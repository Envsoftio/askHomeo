package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/collection"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/document"
)

func (a *API) createCollection(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var b struct {
		Title, Author, Edition, Rationale string
		Scope                             collection.Scope
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&b); err != nil {
		fail(w, 400, "invalid collection request")
		return
	}
	if strings.TrimSpace(b.Title) == "" || strings.TrimSpace(b.Author) == "" || strings.TrimSpace(b.Rationale) == "" {
		fail(w, 400, "title, author and scope rationale are required")
		return
	}
	if b.Scope.RequestDelayMillis < 200 {
		b.Scope.RequestDelayMillis = 200
	}
	seed, err := b.Scope.ValidateCapture()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	scopeJSON, err := json.Marshal(b.Scope)
	if err != nil {
		fail(w, 400, "invalid scope")
		return
	}
	collectionID, snapshotID, itemID := uuid.New(), uuid.New(), uuid.New()
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "collection store unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	actor := requestPrincipal(r.Context()).ID
	_, err = tx.Exec(r.Context(), `INSERT INTO linked_collections(id,title,author,edition,created_by_principal_id) VALUES($1,$2,$3,$4,$5)`, collectionID, strings.TrimSpace(b.Title), strings.TrimSpace(b.Author), strings.TrimSpace(b.Edition), actor)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_snapshots(id,collection_id,generation,scope_json) VALUES($1,$2,1,$3)`, snapshotID, collectionID, scopeJSON)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_items(id,snapshot_id,requested_url,depth,discovery_ordinal) VALUES($1,$2,$3,0,0)`, itemID, snapshotID, seed.String())
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_capture_jobs(snapshot_id) VALUES($1)`, snapshotID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_scope_decisions(id,snapshot_id,actor_principal_id,decision,rationale,scope_json) VALUES($1,$2,$3,'created',$4,$5)`, uuid.New(), snapshotID, actor, strings.TrimSpace(b.Rationale), scopeJSON)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "could not create collection")
		return
	}
	write(w, 202, map[string]any{"collection_id": collectionID, "snapshot_id": snapshotID, "state": "queued"})
}

func (a *API) collectionDetail(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var title, author, edition string
	var snapshotID uuid.UUID
	var generation int
	var scope json.RawMessage
	var state string
	var reasons []string
	var total int64
	err := a.Store.DB.QueryRow(r.Context(), `SELECT c.title,c.author,c.edition,s.id,s.generation,s.scope_json,s.state,s.incomplete_reasons,s.bytes_fetched FROM linked_collections c JOIN collection_snapshots s ON s.collection_id=c.id WHERE c.id=$1 ORDER BY s.generation DESC LIMIT 1`, id).Scan(&title, &author, &edition, &snapshotID, &generation, &scope, &state, &reasons, &total)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "collection not found")
		return
	}
	if err != nil {
		fail(w, 500, "could not read collection")
		return
	}
	items := []map[string]any{}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT i.id,i.requested_url,i.final_url,i.depth,i.discovery_ordinal,i.state,i.role,i.format,i.content_type,coalesce(i.sha256,''),coalesce(i.byte_size,0),i.anchors,i.block_count,i.warnings,i.error,i.attempts,i.source_id,coalesce(s.status,''),coalesce(s.rights_status,''),coalesce(ir.status,'') FROM collection_items i LEFT JOIN sources s ON s.id=i.source_id LEFT JOIN active_indexes ai ON ai.source_id=s.id LEFT JOIN index_runs ir ON ir.id=ai.index_run_id WHERE i.snapshot_id=$1 ORDER BY i.discovery_ordinal`, snapshotID)
	if err != nil {
		fail(w, 500, "could not read collection items")
		return
	}
	for rows.Next() {
		var itemID uuid.UUID
		var requested, final, state, role, format, contentType, sha, problem, sourceStatus, rightsStatus, indexStatus string
		var depth, ordinal, blocks, attempts int
		var size int64
		var anchors, warnings []string
		var sourceID *uuid.UUID
		if err = rows.Scan(&itemID, &requested, &final, &depth, &ordinal, &state, &role, &format, &contentType, &sha, &size, &anchors, &blocks, &warnings, &problem, &attempts, &sourceID, &sourceStatus, &rightsStatus, &indexStatus); err != nil {
			break
		}
		items = append(items, map[string]any{"id": itemID, "requested_url": requested, "final_url": final, "depth": depth, "ordinal": ordinal, "state": state, "role": role, "format": format, "content_type": contentType, "sha256": sha, "byte_size": size, "anchors": anchors, "block_count": blocks, "warnings": warnings, "error": problem, "attempts": attempts, "source_id": sourceID, "source_status": sourceStatus, "rights_status": rightsStatus, "index_status": indexStatus})
	}
	rows.Close()
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		fail(w, 500, "could not read collection items")
		return
	}
	links := []map[string]any{}
	rows, err = a.Store.DB.Query(r.Context(), `SELECT from_item_id,target_url,fragment,label,relation,state FROM collection_links WHERE snapshot_id=$1 ORDER BY from_item_id,ordinal`, snapshotID)
	if err != nil {
		fail(w, 500, "could not read collection links")
		return
	}
	for rows.Next() {
		var from uuid.UUID
		var target, fragment, label, relation, linkState string
		if err = rows.Scan(&from, &target, &fragment, &label, &relation, &linkState); err != nil {
			break
		}
		links = append(links, map[string]any{"from_item_id": from, "target_url": target, "fragment": fragment, "label": label, "relation": relation, "state": linkState})
	}
	rows.Close()
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		fail(w, 500, "could not read collection links")
		return
	}
	var activeID *uuid.UUID
	_ = a.Store.DB.QueryRow(r.Context(), `SELECT snapshot_id FROM active_collection_snapshots WHERE collection_id=$1`, id).Scan(&activeID)
	write(w, 200, map[string]any{"collection_id": id, "title": title, "author": author, "edition": edition, "snapshot_id": snapshotID, "generation": generation, "scope": scope, "state": state, "incomplete_reasons": reasons, "bytes_fetched": total, "items": items, "links": links, "active_snapshot_id": activeID, "capture_complete": state == "review" && len(reasons) == 0})
}

func (a *API) resumeCollection(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Rationale string `json:"rationale"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&b); err != nil || strings.TrimSpace(b.Rationale) == "" {
		fail(w, 400, "resume rationale required")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "collection store unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var snapshotID uuid.UUID
	var scope []byte
	var state string
	err = tx.QueryRow(r.Context(), `SELECT id,scope_json,state FROM collection_snapshots WHERE collection_id=$1 ORDER BY generation DESC LIMIT 1 FOR UPDATE`, id).Scan(&snapshotID, &scope, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "collection not found")
		return
	}
	if err != nil {
		fail(w, 500, "could not read collection")
		return
	}
	if state != "failed" && state != "cancelled" && state != "review" {
		fail(w, 409, "collection is not resumable")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE collection_items SET state='queued',attempts=0,error='' WHERE snapshot_id=$1 AND state='failed'`, snapshotID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE collection_snapshots SET state='queued',started_at=NULL,finished_at=NULL,incomplete_reasons='{}' WHERE id=$1`, snapshotID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_capture_jobs(snapshot_id,state,next_run_at,lease_until,lease_token,last_error) VALUES($1,'queued',now(),NULL,NULL,'') ON CONFLICT(snapshot_id) DO UPDATE SET state='queued',next_run_at=now(),lease_until=NULL,lease_token=NULL,last_error='',updated_at=now()`, snapshotID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_scope_decisions(id,snapshot_id,actor_principal_id,decision,rationale,scope_json) VALUES($1,$2,$3,'resumed',$4,$5)`, uuid.New(), snapshotID, requestPrincipal(r.Context()).ID, strings.TrimSpace(b.Rationale), scope)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "could not resume collection")
		return
	}
	write(w, 202, map[string]any{"snapshot_id": snapshotID, "state": "queued"})
}

func (a *API) cancelCollection(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Rationale string `json:"rationale"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&b); err != nil || strings.TrimSpace(b.Rationale) == "" {
		fail(w, 400, "cancellation rationale required")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "collection store unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var snapshotID uuid.UUID
	var scope []byte
	var state string
	err = tx.QueryRow(r.Context(), `SELECT id,scope_json,state FROM collection_snapshots WHERE collection_id=$1 ORDER BY generation DESC LIMIT 1 FOR UPDATE`, id).Scan(&snapshotID, &scope, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "collection not found")
		return
	}
	if err != nil {
		fail(w, 500, "could not read collection")
		return
	}
	if state != "queued" && state != "capturing" {
		fail(w, 409, "collection is not capturing")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE collection_snapshots SET state='cancelled',finished_at=now() WHERE id=$1`, snapshotID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE collection_capture_jobs SET state='cancelled',lease_until=NULL,lease_token=NULL,updated_at=now() WHERE snapshot_id=$1`, snapshotID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_scope_decisions(id,snapshot_id,actor_principal_id,decision,rationale,scope_json) VALUES($1,$2,$3,'cancelled',$4,$5)`, uuid.New(), snapshotID, requestPrincipal(r.Context()).ID, strings.TrimSpace(b.Rationale), scope)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "could not cancel collection")
		return
	}
	write(w, 200, map[string]any{"snapshot_id": snapshotID, "state": "cancelled"})
}

func (a *API) importCollectionItem(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	collectionID, ok := parseID(w, r)
	if !ok {
		return
	}
	itemID, err := uuid.Parse(r.PathValue("item"))
	if err != nil {
		fail(w, 400, "invalid item ID")
		return
	}
	var b struct {
		CharsetOverride string `json:"charset_override"`
		Title           string `json:"title"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&b); err != nil {
		fail(w, 400, "invalid item import request")
		return
	}
	var title, author, edition, state, role, sha, format, contentType, requested, final, locator string
	var sourceID *uuid.UUID
	err = a.Store.DB.QueryRow(r.Context(), `SELECT c.title,c.author,c.edition,s.state,i.role,coalesce(i.sha256,''),i.format,i.content_type,i.requested_url,i.final_url,coalesce(i.object_locator,''),i.source_id FROM collection_items i JOIN collection_snapshots s ON s.id=i.snapshot_id JOIN linked_collections c ON c.id=s.collection_id WHERE c.id=$1 AND i.id=$2 AND s.generation=(SELECT max(generation) FROM collection_snapshots WHERE collection_id=c.id)`, collectionID, itemID).Scan(&title, &author, &edition, &state, &role, &sha, &format, &contentType, &requested, &final, &locator, &sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "collection item not found")
		return
	}
	if err != nil {
		fail(w, 500, "could not read collection item")
		return
	}
	if state != "review" || role != "content_candidate" {
		fail(w, 409, "item is not ready for content review")
		return
	}
	if sourceID != nil {
		write(w, 200, map[string]any{"source_id": sourceID, "already_imported": true})
		return
	}
	if sha == "" || format == "" || locator != filepath.Join("data/runtime/assets", sha+"."+format) {
		fail(w, 409, "invalid saved asset identity")
		return
	}
	raw, err := os.ReadFile(filepath.Join(a.Store.Root, locator))
	if err != nil {
		fail(w, 500, "saved asset unavailable")
		return
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != sha {
		fail(w, 500, "saved asset checksum mismatch")
		return
	}
	if strings.TrimSpace(b.Title) != "" {
		title = b.Title
	} else if extracted, extractErr := document.Extract(raw, contentType, b.CharsetOverride); extractErr == nil && extracted.Title != "" {
		title = extracted.Title
	}
	transport := "https"
	if strings.HasPrefix(final, "http:") {
		transport = "http"
	}
	id, err := a.Store.ImportDocument(r.Context(), bytes.NewReader(raw), core.DocumentImport{SourceID: &itemID, Title: title, Author: author, Edition: edition, Repository: "linked collection", SourceURL: final, RequestedURL: requested, FinalURL: final, Transport: transport, ContentType: contentType, CharsetOverride: b.CharsetOverride})
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	_, err = a.Store.DB.Exec(r.Context(), `UPDATE collection_items SET source_id=$2 WHERE id=$1 AND source_id IS NULL`, itemID, id)
	if err != nil {
		fail(w, 500, "source imported; retry to link collection item")
		return
	}
	write(w, 202, map[string]any{"source_id": id, "review_required": true})
}

// A scope change always creates a new generation. The previous active pointer
// and all source/revision identities remain untouched during the refresh.
func (a *API) refreshCollection(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Rationale string           `json:"rationale"`
		Scope     collection.Scope `json:"scope"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&b); err != nil || strings.TrimSpace(b.Rationale) == "" {
		fail(w, 400, "scope and refresh rationale required")
		return
	}
	if b.Scope.RequestDelayMillis < 200 {
		b.Scope.RequestDelayMillis = 200
	}
	seed, err := b.Scope.ValidateCapture()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	raw, err := json.Marshal(b.Scope)
	if err != nil {
		fail(w, 400, "invalid scope")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "collection store unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var generation int
	var previousState string
	err = tx.QueryRow(r.Context(), `SELECT s.generation,s.state FROM linked_collections c JOIN collection_snapshots s ON s.collection_id=c.id WHERE c.id=$1 ORDER BY s.generation DESC LIMIT 1 FOR UPDATE OF c,s`, id).Scan(&generation, &previousState)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "collection not found")
		return
	}
	if err != nil {
		fail(w, 500, "could not read collection")
		return
	}
	if previousState == "queued" || previousState == "capturing" {
		fail(w, 409, "finish or cancel the current capture before refreshing")
		return
	}
	snapshotID, itemID := uuid.New(), uuid.New()
	_, err = tx.Exec(r.Context(), `INSERT INTO collection_snapshots(id,collection_id,generation,scope_json) VALUES($1,$2,$3,$4)`, snapshotID, id, generation+1, raw)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_items(id,snapshot_id,requested_url,depth,discovery_ordinal) VALUES($1,$2,$3,0,0)`, itemID, snapshotID, seed.String())
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_capture_jobs(snapshot_id) VALUES($1)`, snapshotID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO collection_scope_decisions(id,snapshot_id,actor_principal_id,decision,rationale,scope_json) VALUES($1,$2,$3,'replaced',$4,$5)`, uuid.New(), snapshotID, requestPrincipal(r.Context()).ID, strings.TrimSpace(b.Rationale), raw)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "could not start collection refresh")
		return
	}
	write(w, 202, map[string]any{"snapshot_id": snapshotID, "generation": generation + 1, "state": "queued"})
}

func (a *API) activateCollection(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Rationale    string `json:"rationale"`
		AllowPartial bool   `json:"allow_partial"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&b); err != nil || strings.TrimSpace(b.Rationale) == "" {
		fail(w, 400, "activation rationale required")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "collection store unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var snapshotID uuid.UUID
	var state string
	var reasons []string
	err = tx.QueryRow(r.Context(), `SELECT s.id,s.state,s.incomplete_reasons FROM linked_collections c JOIN collection_snapshots s ON s.collection_id=c.id WHERE c.id=$1 ORDER BY s.generation DESC LIMIT 1 FOR UPDATE OF c,s`, id).Scan(&snapshotID, &state, &reasons)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "collection not found")
		return
	}
	if err != nil {
		fail(w, 500, "could not read collection")
		return
	}
	if state != "review" {
		fail(w, 409, "capture must be in review before activation")
		return
	}
	if len(reasons) > 0 && !b.AllowPartial {
		fail(w, 409, "capture has incomplete coverage; acknowledge partial activation")
		return
	}
	var candidates, unready, configCount int
	err = tx.QueryRow(r.Context(), `SELECT count(*) FILTER (WHERE i.state='fetched' AND i.role='content_candidate'),count(*) FILTER (WHERE i.state='fetched' AND i.role='content_candidate' AND (i.source_id IS NULL OR s.status<>'published' OR s.rights_status<>'allowed' OR s.superseded_at IS NOT NULL OR ir.status<>'ready' OR ai.index_run_id IS NULL)),count(DISTINCT ir.embedding_config_id) FILTER (WHERE i.state='fetched' AND i.role='content_candidate') FROM collection_items i LEFT JOIN sources s ON s.id=i.source_id LEFT JOIN active_indexes ai ON ai.source_id=s.id LEFT JOIN index_runs ir ON ir.id=ai.index_run_id WHERE i.snapshot_id=$1`, snapshotID).Scan(&candidates, &unready, &configCount)
	if err != nil {
		fail(w, 500, "could not verify collection readiness")
		return
	}
	if candidates == 0 || unready > 0 || configCount != 1 {
		fail(w, 409, "every candidate needs reviewed content, allowed rights, publication and one compatible ready index configuration")
		return
	}
	var unresolved int
	err = tx.QueryRow(r.Context(), `SELECT count(*) FROM collection_items WHERE snapshot_id=$1 AND state IN ('queued','fetching','failed')`, snapshotID).Scan(&unresolved)
	if err != nil {
		fail(w, 500, "could not verify coverage")
		return
	}
	if unresolved > 0 && !b.AllowPartial {
		fail(w, 409, "unresolved collection items require partial activation acknowledgement")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO active_collection_snapshots(collection_id,snapshot_id) VALUES($1,$2) ON CONFLICT(collection_id) DO UPDATE SET snapshot_id=excluded.snapshot_id,activated_at=now()`, id, snapshotID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE collection_snapshots SET state='active' WHERE id=$1`, snapshotID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "could not activate collection")
		return
	}
	write(w, 200, map[string]any{"active_snapshot_id": snapshotID, "partial": len(reasons) > 0 || unresolved > 0, "rationale": strings.TrimSpace(b.Rationale)})
}
