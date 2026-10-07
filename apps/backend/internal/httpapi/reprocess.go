package httpapi

import (
	"github.com/google/uuid"
	"net/http"
)

// reprocess keeps the published source and all its evidence intact while a new
// candidate source goes through extraction, QA, rights review and indexing.
func (a *API) reprocess(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	oldID, ok := parseID(w, r)
	if !ok {
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not create candidate revision.")
		return
	}
	defer tx.Rollback(r.Context())
	var status, rights string
	var superseded bool
	err = tx.QueryRow(r.Context(), `SELECT status,rights_status,superseded_at IS NOT NULL FROM sources WHERE id=$1 FOR UPDATE`, oldID).Scan(&status, &rights, &superseded)
	if err != nil {
		fail(w, 404, "source not found")
		return
	}
	if status != "published" || rights != "allowed" || superseded {
		fail(w, 409, "Choose a current published source to reprocess.")
		return
	}
	var pending bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sources WHERE supersedes_source_id=$1 AND status NOT IN ('failed','disabled'))`, oldID).Scan(&pending)
	if err != nil {
		fail(w, 500, "Could not check candidate revisions.")
		return
	}
	if pending {
		fail(w, 409, "This source already has a candidate revision. Open it in Sources.")
		return
	}
	id := uuid.New()
	_, err = tx.Exec(r.Context(), `INSERT INTO sources(id,source_key,title,author,publication_year,source_url,pdf_path,pdf_sha256,pdf_bytes,page_count,status,edition,publication_info,repository,rights_statement,pdf_origin_url,supersedes_source_id)
 SELECT $1,$3,title,author,publication_year,source_url,pdf_path,pdf_sha256,pdf_bytes,
 CASE WHEN source_key LIKE 'upload-%' THEN page_count ELSE page_count+1 END,
 'queued',edition,publication_info,repository,rights_statement,pdf_origin_url,id
 FROM sources WHERE id=$2`, id, oldID, "upload-"+id.String())
	if err != nil {
		fail(w, 500, "Could not create candidate revision.")
		return
	}
	var total int
	err = tx.QueryRow(r.Context(), `SELECT page_count-1 FROM sources WHERE id=$1`, id).Scan(&total)
	if err != nil {
		fail(w, 500, "Could not create candidate revision.")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO jobs(id,source_id,kind,status,current_step,total) VALUES($1,$2,'ingest','queued','Reading PDF pages',$3)`, uuid.New(), id, total)
	if err != nil {
		fail(w, 500, "Could not queue candidate revision.")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "Could not save candidate revision.")
		return
	}
	write(w, 202, map[string]any{"source_id": id, "supersedes_source_id": oldID, "status": "queued"})
}
