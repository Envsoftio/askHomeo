package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

func (a *API) permanentlyDeleteSource(w http.ResponseWriter, r *http.Request) {
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
		fail(w, 400, "Type the source title to confirm permanent deletion.")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not delete source.")
		return
	}
	defer tx.Rollback(r.Context())
	// Match the stored title and serialize publication/metadata changes.
	var title string
	err = tx.QueryRow(r.Context(), `SELECT title FROM sources WHERE id=$1 FOR UPDATE`, id).Scan(&title)
	if err != nil {
		fail(w, 404, "Source not found.")
		return
	}
	if body.ConfirmTitle != title {
		fail(w, 400, "The confirmation title does not match. Reload the source and try again.")
		return
	}
	_, err = tx.Exec(r.Context(), `SELECT delete_unused_source($1)`, id)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "P0001" {
			fail(w, 409, pe.Message)
		} else {
			fail(w, 409, "Source still has dependent records or active work; no data was deleted.")
		}
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "Could not complete source deletion.")
		return
	}
	// The durable worker queue owns cleanup. Never report files erased before it finishes.
	write(w, 202, map[string]string{"status": "deleted_cleanup_pending", "message": "Source data deleted. Unshared original files are queued for permanent cleanup."})
}
func (a *API) deletedFileStatus(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var pending, failed int
	err := a.Store.DB.QueryRow(r.Context(), `SELECT count(DISTINCT source_id),count(DISTINCT source_id) FILTER(WHERE error<>'') FROM deleted_source_files`).Scan(&pending, &failed)
	if err != nil {
		fail(w, 500, "Could not check file cleanup.")
		return
	}
	write(w, 200, map[string]int{"pending": pending, "failed": failed})
}
func (a *API) retryDeletedFiles(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if _, err := a.Store.DB.Exec(r.Context(), `UPDATE deleted_source_files SET retry_after=now() WHERE error<>''`); err != nil {
		fail(w, 500, "Could not queue file cleanup.")
		return
	}
	write(w, 202, map[string]string{"status": "queued"})
}
