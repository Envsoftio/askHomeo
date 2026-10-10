package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/document"
)

// prepareOne creates one review source from an immutable captured item. It is
// deliberately separate from fetching: incomplete captures cannot silently
// become bulk review candidates, and a restart resumes from database state.
func (w *CaptureWorker) prepareOne(ctx context.Context) error {
	var itemID uuid.UUID
	var title, author, edition, sha, format, contentType, requested, final, locator string
	var attempts int
	err := w.Store.DB.QueryRow(ctx, `UPDATE collection_items SET preparation_state='running',preparation_attempts=preparation_attempts+1,preparation_lease_until=now()+interval '5 minutes',preparation_error=''
 WHERE id=(SELECT i.id FROM collection_items i JOIN collection_snapshots s ON s.id=i.snapshot_id
 WHERE i.state='fetched' AND i.role='content_candidate' AND i.source_id IS NULL
 AND (i.preparation_state='pending' OR (i.preparation_state='running' AND i.preparation_lease_until<now()))
 AND i.preparation_next_run_at<=now() AND s.state='review' AND cardinality(s.incomplete_reasons)=0
 AND s.generation=(SELECT max(generation) FROM collection_snapshots WHERE collection_id=s.collection_id)
 ORDER BY i.discovery_ordinal FOR UPDATE OF i SKIP LOCKED LIMIT 1)
 RETURNING id,preparation_attempts`).Scan(&itemID, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("claim collection preparation: %w", err)
	}
	err = w.Store.DB.QueryRow(ctx, `SELECT c.title,c.author,c.edition,i.sha256,i.format,i.content_type,i.requested_url,i.final_url,i.object_locator
 FROM collection_items i JOIN collection_snapshots s ON s.id=i.snapshot_id JOIN linked_collections c ON c.id=s.collection_id WHERE i.id=$1`, itemID).Scan(&title, &author, &edition, &sha, &format, &contentType, &requested, &final, &locator)
	if err == nil {
		err = w.prepareItem(ctx, itemID, title, author, edition, sha, format, contentType, requested, final, locator)
	}
	if err != nil {
		state := "pending"
		if attempts >= 3 {
			state = "failed"
		}
		_, updateErr := w.Store.DB.Exec(ctx, `UPDATE collection_items SET preparation_state=$2,preparation_error=$3,preparation_lease_until=NULL,preparation_next_run_at=now()+($4::int*interval '1 second') WHERE id=$1 AND preparation_state='running'`, itemID, state, err.Error(), attempts*attempts)
		if updateErr != nil {
			return updateErr
		}
		return nil
	}
	_, err = w.Store.DB.Exec(ctx, `UPDATE collection_items SET source_id=$2,preparation_state='done',preparation_error='',preparation_lease_until=NULL WHERE id=$1 AND preparation_state='running'`, itemID, itemID)
	return err
}

func (w *CaptureWorker) prepareItem(ctx context.Context, itemID uuid.UUID, title, author, edition, sha, format, contentType, requested, final, locator string) error {
	if sha == "" || (format != "html" && format != "txt") || locator != filepath.Join("data/runtime/assets", sha+"."+format) {
		return errors.New("captured original has invalid asset identity")
	}
	start := time.Now()
	raw, err := os.ReadFile(filepath.Join(w.Store.Root, locator))
	if err != nil {
		return err
	}
	if time.Since(start) > time.Minute {
		return errors.New("saved asset read exceeded preparation limit")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != sha {
		return errors.New("captured original checksum changed")
	}
	if extracted, extractErr := document.Extract(raw, contentType, ""); extractErr == nil && extracted.Title != "" {
		title = extracted.Title
	}
	transport := "https"
	if strings.HasPrefix(final, "http:") {
		transport = "http"
	}
	_, err = w.Store.ImportDocument(ctx, bytes.NewReader(raw), core.DocumentImport{SourceID: &itemID, Title: title, Author: author, Edition: edition, Repository: "linked collection", SourceURL: final, RequestedURL: requested, FinalURL: final, Transport: transport, ContentType: contentType})
	return err
}
