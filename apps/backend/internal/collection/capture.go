package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/document"
	"homeopath-poc/backend/internal/safefetch"
)

type CaptureWorker struct {
	Store   *core.Store
	Fetcher Fetcher
}

func (w *CaptureWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := w.Once(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Once processes at most one item. The frontier and job lease live in
// PostgreSQL, so another worker can continue after an interrupted fetch.
func (w *CaptureWorker) Once(ctx context.Context) error {
	var snapshotID uuid.UUID
	claimToken := uuid.New()
	err := w.Store.DB.QueryRow(ctx, `UPDATE collection_capture_jobs SET state='running',attempts=attempts+1,lease_until=now()+interval '90 seconds',lease_token=$1,updated_at=now()
 WHERE snapshot_id=(SELECT snapshot_id FROM collection_capture_jobs WHERE next_run_at<=now() AND (state='queued' OR (state='running' AND lease_until<now())) ORDER BY next_run_at FOR UPDATE SKIP LOCKED LIMIT 1)
 RETURNING snapshot_id`, claimToken).Scan(&snapshotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return w.prepareOne(ctx)
	}
	if err != nil {
		return fmt.Errorf("claim collection capture: %w", err)
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = w.Store.DB.Exec(ctx, `UPDATE collection_capture_jobs SET lease_until=now()+interval '90 seconds',updated_at=now() WHERE snapshot_id=$1 AND state='running' AND lease_token=$2`, snapshotID, claimToken)
			}
		}
	}()
	err = w.captureOne(ctx, snapshotID, claimToken)
	close(stop)
	if err != nil {
		tag, _ := w.Store.DB.Exec(ctx, `UPDATE collection_capture_jobs SET state='failed',lease_until=NULL,lease_token=NULL,last_error=$2,updated_at=now() WHERE snapshot_id=$1 AND state='running' AND lease_token=$3`, snapshotID, err.Error(), claimToken)
		if tag.RowsAffected() > 0 {
			_, _ = w.Store.DB.Exec(ctx, `UPDATE collection_snapshots SET state='failed',incomplete_reasons=array_append(incomplete_reasons,$2),finished_at=now() WHERE id=$1 AND state='capturing'`, snapshotID, err.Error())
		}
		return nil
	}
	return nil
}

func (w *CaptureWorker) captureOne(ctx context.Context, snapshotID, claimToken uuid.UUID) error {
	var rawScope []byte
	var total int64
	var started time.Time
	err := w.Store.DB.QueryRow(ctx, `UPDATE collection_snapshots SET state='capturing',started_at=coalesce(started_at,now()) WHERE id=$1 AND state IN ('queued','capturing') RETURNING scope_json,bytes_fetched,started_at`, snapshotID).Scan(&rawScope, &total, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var scope Scope
	if err = json.Unmarshal(rawScope, &scope); err != nil {
		return err
	}
	if _, err = scope.ValidateCapture(); err != nil {
		return err
	}
	if time.Since(started) > time.Duration(scope.MaxDurationSeconds)*time.Second || total >= scope.MaxTotalBytes {
		reason := "capture time limit reached"
		if total >= scope.MaxTotalBytes {
			reason = "capture byte limit reached"
		}
		return w.finishLimit(ctx, snapshotID, claimToken, reason)
	}
	// An expired lease may have left one item in fetching. Its file is
	// content-addressed; repeating this item is safe and does not add a source.
	_, err = w.Store.DB.Exec(ctx, `UPDATE collection_items SET state='queued' WHERE snapshot_id=$1 AND state='fetching'`, snapshotID)
	if err != nil {
		return err
	}
	var itemID uuid.UUID
	var target string
	var depth, attempts int
	err = w.Store.DB.QueryRow(ctx, `UPDATE collection_items SET state='fetching',attempts=attempts+1 WHERE id=(SELECT id FROM collection_items WHERE snapshot_id=$1 AND state='queued' ORDER BY discovery_ordinal FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,requested_url,depth,attempts`, snapshotID).Scan(&itemID, &target, &depth, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return w.finishCapture(ctx, snapshotID, claimToken)
	}
	if err != nil {
		return err
	}
	fetcher := w.Fetcher
	if fetcher == nil {
		fetcher = &safefetch.Fetcher{}
	}
	limit := int64(document.MaxBytes)
	if remaining := scope.MaxTotalBytes - total; remaining < limit {
		limit = remaining
	}
	allowURL := func(u *url.URL) error {
		if !scope.AllowUnencryptedHTTP && u.Scheme == "http" {
			return errors.New("unencrypted HTTP was not selected for this capture")
		}
		return scope.AllowURL(u)
	}
	result, fetchErr := fetcher.FetchScoped(ctx, target, limit, scope.AllowHTTPRedirect, allowURL)
	if fetchErr != nil {
		return w.itemFailed(ctx, snapshotID, claimToken, itemID, attempts, scope, fetchErr)
	}
	data, readErr := io.ReadAll(result.Body)
	result.Body.Close()
	if readErr != nil {
		return w.itemFailed(ctx, snapshotID, claimToken, itemID, attempts, scope, readErr)
	}
	inspected, links, inspectErr := Inspect(data, result.ContentType, result.FinalURL)
	if inspectErr != nil {
		return w.itemFailed(ctx, snapshotID, claimToken, itemID, attempts, scope, inspectErr)
	}
	tx, err := w.Store.BeginAssetWrite(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	sha := sha256.Sum256(data)
	shaText := hex.EncodeToString(sha[:])
	locator, err := saveAsset(w.Store.Root, data, inspected.Format, shaText)
	if err != nil {
		return err
	}
	var stillRunning bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_snapshots WHERE id=$1 AND state='capturing' FOR UPDATE)`, snapshotID).Scan(&stillRunning)
	if err != nil {
		return err
	}
	if !stillRunning {
		return nil
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_capture_jobs WHERE snapshot_id=$1 AND state='running' AND lease_token=$2 FOR UPDATE)`, snapshotID, claimToken).Scan(&stillRunning)
	if err != nil {
		return err
	}
	if !stillRunning {
		return nil
	}
	var duplicate bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_items WHERE snapshot_id=$1 AND final_url=$2 AND state='fetched' AND id<>$3)`, snapshotID, result.FinalURL, itemID).Scan(&duplicate)
	if err != nil {
		return err
	}
	state := "fetched"
	if duplicate {
		state = "duplicate_redirect"
	}
	_, err = tx.Exec(ctx, `UPDATE collection_items SET state=$2,final_url=$3,role=$4,content_type=$5,format=$6,sha256=$7,byte_size=$8,object_locator=$9,anchors=$10,block_count=$11,warnings=$12,error='',fetched_at=now() WHERE id=$1`, itemID, state, result.FinalURL, inspected.Role, result.ContentType, inspected.Format, shaText, len(data), locator, inspected.Anchors, inspected.BlockCount, inspected.Warnings)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE collection_snapshots SET bytes_fetched=bytes_fetched+$2 WHERE id=$1`, snapshotID, len(data))
	if err != nil {
		return err
	}
	if !duplicate {
		var count int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM collection_items WHERE snapshot_id=$1`, snapshotID).Scan(&count)
		if err != nil {
			return err
		}
		for i, link := range links {
			if i >= 10000 {
				_, err = tx.Exec(ctx, `UPDATE collection_snapshots SET incomplete_reasons=array_append(incomplete_reasons,'link limit reached') WHERE id=$1`, snapshotID)
				if err != nil {
					return err
				}
				break
			}
			linkState := "queued"
			u := mustURL(link.To)
			if scope.AllowURL(u) != nil || (!scope.AllowUnencryptedHTTP && u.Scheme == "http") {
				linkState = "excluded_scope"
			} else {
				var exists bool
				err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_items WHERE snapshot_id=$1 AND requested_url=$2)`, snapshotID, link.To).Scan(&exists)
				if err != nil {
					return err
				}
				if exists {
					linkState = "duplicate_or_cycle"
				} else if scope.MaxDepth > 0 && depth >= scope.MaxDepth {
					linkState = "excluded_depth"
				} else if scope.MaxDocuments > 0 && count >= scope.MaxDocuments {
					linkState = "excluded_document_limit"
				} else {
					_, err = tx.Exec(ctx, `INSERT INTO collection_items(id,snapshot_id,requested_url,depth,discovery_ordinal) VALUES($1,$2,$3,$4,$5) ON CONFLICT(snapshot_id,requested_url) DO NOTHING`, uuid.New(), snapshotID, link.To, depth+1, count)
					if err != nil {
						return err
					}
					count++
				}
			}
			_, err = tx.Exec(ctx, `INSERT INTO collection_links(id,snapshot_id,from_item_id,ordinal,target_url,fragment,label,relation,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(from_item_id,ordinal) DO NOTHING`, uuid.New(), snapshotID, itemID, i, link.To, link.Fragment, link.Text, link.Relation, linkState)
			if err != nil {
				return err
			}
			if linkState == "excluded_depth" || linkState == "excluded_document_limit" {
				_, err = tx.Exec(ctx, `UPDATE collection_snapshots SET incomplete_reasons=array_append(incomplete_reasons,$2) WHERE id=$1`, snapshotID, linkState)
				if err != nil {
					return err
				}
			}
		}
	}
	_, err = tx.Exec(ctx, `UPDATE collection_capture_jobs SET state='queued',lease_until=NULL,lease_token=NULL,next_run_at=now()+($2::int*interval '1 millisecond'),last_error='',updated_at=now() WHERE snapshot_id=$1 AND lease_token=$3`, snapshotID, max(scope.RequestDelayMillis, 200), claimToken)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *CaptureWorker) finishLimit(ctx context.Context, snapshotID, claimToken uuid.UUID, reason string) error {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owned bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_snapshots WHERE id=$1 AND state='capturing' FOR UPDATE)`, snapshotID).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return nil
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_capture_jobs WHERE snapshot_id=$1 AND state='running' AND lease_token=$2 FOR UPDATE)`, snapshotID, claimToken).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return nil
	}
	if _, err = tx.Exec(ctx, `UPDATE collection_snapshots SET state='review',incomplete_reasons=array_append(incomplete_reasons,$2),finished_at=now() WHERE id=$1`, snapshotID, reason); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE collection_capture_jobs SET state='done',lease_until=NULL,lease_token=NULL,updated_at=now() WHERE snapshot_id=$1 AND lease_token=$2`, snapshotID, claimToken); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *CaptureWorker) itemFailed(ctx context.Context, snapshotID, claimToken, itemID uuid.UUID, attempts int, scope Scope, problem error) error {
	state := "queued"
	if attempts >= 3 {
		state = "failed"
	}
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owned bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_snapshots WHERE id=$1 AND state='capturing' FOR UPDATE)`, snapshotID).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return nil
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_capture_jobs WHERE snapshot_id=$1 AND state='running' AND lease_token=$2 FOR UPDATE)`, snapshotID, claimToken).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE collection_items SET state=$2,error=$3 WHERE id=$1 AND state='fetching'`, itemID, state, problem.Error())
	if err != nil {
		return err
	}
	delay := max(scope.RequestDelayMillis, 200)
	if state == "queued" {
		delay = max(delay, attempts*attempts*1000)
	}
	_, err = tx.Exec(ctx, `UPDATE collection_capture_jobs SET state='queued',lease_until=NULL,lease_token=NULL,next_run_at=now()+($2::int*interval '1 millisecond'),last_error=$3,updated_at=now() WHERE snapshot_id=$1 AND state='running' AND lease_token=$4`, snapshotID, delay, problem.Error(), claimToken)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *CaptureWorker) finishCapture(ctx context.Context, snapshotID, claimToken uuid.UUID) error {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owned bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_snapshots WHERE id=$1 AND state='capturing' FOR UPDATE)`, snapshotID).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return nil
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM collection_capture_jobs WHERE snapshot_id=$1 AND state='running' AND lease_token=$2 FOR UPDATE)`, snapshotID, claimToken).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE collection_links l SET state=CASE WHEN i.state='failed' THEN 'target_failed' WHEN l.fragment<>'' AND NOT l.fragment=ANY(i.anchors) THEN 'missing_anchor' ELSE 'resolved' END FROM collection_items i WHERE l.snapshot_id=$1 AND i.snapshot_id=l.snapshot_id AND i.requested_url=l.target_url AND l.state IN ('queued','duplicate_or_cycle','target_failed','missing_anchor')`, snapshotID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE collection_snapshots SET state='review',finished_at=now(),incomplete_reasons=incomplete_reasons
		||CASE WHEN EXISTS(SELECT 1 FROM collection_items WHERE snapshot_id=$1 AND state='failed') THEN ARRAY['failed documents']::text[] ELSE ARRAY[]::text[] END
		||CASE WHEN EXISTS(SELECT 1 FROM collection_links WHERE snapshot_id=$1 AND state='missing_anchor') THEN ARRAY['missing anchors']::text[] ELSE ARRAY[]::text[] END
		||CASE WHEN EXISTS(SELECT 1 FROM collection_links WHERE snapshot_id=$1 AND state='excluded_depth') THEN ARRAY['depth limit reached']::text[] ELSE ARRAY[]::text[] END
		||CASE WHEN EXISTS(SELECT 1 FROM collection_links WHERE snapshot_id=$1 AND state='excluded_document_limit') THEN ARRAY['document limit reached']::text[] ELSE ARRAY[]::text[] END
		||CASE WHEN EXISTS(SELECT 1 FROM collection_items WHERE snapshot_id=$1 AND state='fetched' AND role='unresolved') THEN ARRAY['unresolved content extraction']::text[] ELSE ARRAY[]::text[] END
		WHERE id=$1 AND state='capturing'`, snapshotID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE collection_capture_jobs SET state='done',lease_until=NULL,lease_token=NULL,last_error='',updated_at=now() WHERE snapshot_id=$1 AND state='running' AND lease_token=$2`, snapshotID, claimToken)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func saveAsset(root string, raw []byte, format, sha string) (string, error) {
	relative := filepath.Join("data/runtime/assets", sha+"."+format)
	full := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		return "", err
	}
	if existing, err := os.ReadFile(full); err == nil {
		if !bytes.Equal(existing, raw) {
			return "", errors.New("collection asset checksum collision")
		}
		return relative, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".collection-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(raw); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Chmod(0400); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(tmp.Name(), full); err != nil {
		return "", err
	}
	return relative, nil
}
