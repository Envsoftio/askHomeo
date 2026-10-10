package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Page sources in the same captured book count as one user selection. The
// underlying IDs still scope retrieval and preserve each page's citations.
func (a *API) validBookSelectionSize(r *http.Request, ids []uuid.UUID) bool {
	if len(ids) <= 50 {
		return true
	}
	var count int
	err := a.Store.DB.QueryRow(r.Context(), `SELECT count(DISTINCT coalesce(book.collection_id,s.id)) FROM sources s
 LEFT JOIN LATERAL (SELECT cs.collection_id FROM collection_items i JOIN collection_snapshots cs ON cs.id=i.snapshot_id WHERE i.source_id=s.id LIMIT 1) book ON true
 WHERE s.id=ANY($1::uuid[])`, ids).Scan(&count)
	return err == nil && count <= 50
}

func (a *API) listCollections(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.DB.Query(r.Context(), `SELECT c.id,c.title,c.author,s.state,
 (SELECT count(*) FROM collection_items WHERE snapshot_id=s.id),
 (SELECT count(*) FROM collection_items WHERE snapshot_id=s.id AND state='fetched'),
 (SELECT count(DISTINCT i.source_id) FROM collection_items i JOIN collection_snapshots cs ON cs.id=i.snapshot_id WHERE cs.collection_id=c.id),
 cardinality(s.incomplete_reasons)=0 AND s.state IN ('review','active')
 FROM linked_collections c JOIN LATERAL (SELECT * FROM collection_snapshots WHERE collection_id=c.id ORDER BY generation DESC LIMIT 1) s ON true
 ORDER BY c.created_at DESC`)
	if err != nil {
		fail(w, 500, "Could not load linked books.")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id uuid.UUID
		var title, author, state string
		var total, fetched, sourceCount int
		var complete bool
		if err = rows.Scan(&id, &title, &author, &state, &total, &fetched, &sourceCount, &complete); err != nil {
			fail(w, 500, "Could not read linked books.")
			return
		}
		out = append(out, map[string]any{"id": id, "title": title, "author": author, "state": state, "total": total, "fetched": fetched, "source_count": sourceCount, "complete": complete})
	}
	if rows.Err() != nil {
		fail(w, 500, "Could not read linked books.")
		return
	}
	write(w, 200, out)
}

// Delete every unused page source in one transaction, including previous
// generations. The immutable crawl manifest and saved originals remain for
// inspection. The existing deletion function protects published/cited data.
func (a *API) deleteCollectionSources(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		ConfirmTitle string `json:"confirm_title"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body) != nil || strings.TrimSpace(body.ConfirmTitle) == "" {
		fail(w, 400, "Type the book title to confirm deletion of all its page sources.")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not delete book page sources.")
		return
	}
	defer tx.Rollback(r.Context())
	var title string
	if err = tx.QueryRow(r.Context(), `SELECT title FROM linked_collections WHERE id=$1 FOR UPDATE`, id).Scan(&title); err != nil {
		fail(w, 404, "Book not found.")
		return
	}
	if body.ConfirmTitle != title {
		fail(w, 400, "The confirmation title does not match this book.")
		return
	}
	// Lock snapshots before items, matching capture/review. Reject work that
	// could recreate a source after this transaction commits.
	if _, err = tx.Exec(r.Context(), `SELECT id FROM collection_snapshots WHERE collection_id=$1 FOR UPDATE`, id); err != nil {
		fail(w, 409, "Could not lock this book for deletion.")
		return
	}
	if _, err = tx.Exec(r.Context(), `SELECT id FROM collection_items WHERE snapshot_id IN(SELECT id FROM collection_snapshots WHERE collection_id=$1) FOR UPDATE`, id); err != nil {
		fail(w, 409, "Could not lock book pages for deletion.")
		return
	}
	var working bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM collection_snapshots WHERE collection_id=$1 AND state IN ('queued','capturing')) OR EXISTS(SELECT 1 FROM collection_items i JOIN collection_snapshots s ON s.id=i.snapshot_id WHERE s.collection_id=$1 AND i.preparation_state='running')`, id).Scan(&working)
	if err != nil || working {
		fail(w, 409, "Cancel capture and wait for page preparation to finish before deleting book sources.")
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT DISTINCT i.source_id FROM collection_items i JOIN collection_snapshots s ON s.id=i.snapshot_id WHERE s.collection_id=$1 AND i.source_id IS NOT NULL ORDER BY i.source_id`, id)
	if err != nil {
		fail(w, 500, "Could not read book page sources.")
		return
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var sid uuid.UUID
		if err = rows.Scan(&sid); err != nil {
			break
		}
		ids = append(ids, sid)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "Could not read book page sources.")
		return
	}
	for _, sid := range ids {
		if _, err = tx.Exec(r.Context(), `SELECT delete_unused_source($1)`, sid); err != nil {
			var pe *pgconn.PgError
			message := "Book contains sources with active work or dependent records. No sources were deleted."
			if errors.As(err, &pe) && pe.Code == "P0001" {
				message = pe.Message + " No sources were deleted."
			}
			fail(w, 409, message)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `UPDATE collection_items SET preparation_state='done',preparation_lease_until=NULL WHERE snapshot_id IN(SELECT id FROM collection_snapshots WHERE collection_id=$1) AND source_id IS NULL`, id)
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "Could not finish deleting book page sources.")
		return
	}
	write(w, 200, map[string]any{"deleted_sources": len(ids), "message": "All unused page sources deleted. The saved capture remains available for inspection."})
}
