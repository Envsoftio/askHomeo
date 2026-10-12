package httpapi

import (
	"net/http"
	"time"
)

// refreshPaperNotice records the provider observation and applies the existing
// source-disable gate when Crossref reports an editorial update. An absent notice does
// not clear a previously recorded notice or re-enable a source.
func (a *API) refreshPaperNotice(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var name string
	if err := a.Store.DB.QueryRow(r.Context(), `SELECT coalesce(doi,'') FROM scholarly_works WHERE id=$1`, id).Scan(&name); err != nil {
		fail(w, 404, "paper reference unavailable")
		return
	}
	if name == "" {
		fail(w, 409, "a DOI is required for the current notice check")
		return
	}
	metadata, err := a.doiClient().Lookup(r.Context(), name)
	if err != nil {
		fail(w, 502, "Crossref notice check failed; saved status was not changed")
		return
	}
	if metadata.DOI != name {
		fail(w, 502, "notice response DOI differs from saved paper")
		return
	}
	if metadata.Provider != "Crossref" {
		fail(w, 502, "Crossref has no record for this DOI; notice status was not changed")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	selectedURL, selectedType, rank := "", "", -1
	var priorURL, priorType string
	if err = tx.QueryRow(r.Context(), `SELECT notice_url,notice_type FROM scholarly_works WHERE id=$1 FOR UPDATE`, id).Scan(&priorURL, &priorType); err != nil {
		fail(w, 409, "paper reference unavailable")
		return
	}
	if priorURL != "" {
		selectedURL, selectedType, rank = priorURL, priorType, paperNoticePriority(priorType)
	}
	for _, update := range metadata.Updates {
		_, err = tx.Exec(r.Context(), `INSERT INTO scholarly_notices(work_id,notice_doi,notice_type,notice_url,provider) VALUES($1,$2,$3,$4,'crossref') ON CONFLICT(work_id,notice_doi,notice_type) DO UPDATE SET notice_url=$4,last_checked_at=now()`, id, update.DOI, update.Type, update.URL)
		if err != nil {
			fail(w, 500, "could not save article update")
			return
		}
		priority := paperNoticePriority(update.Type)
		if priority > rank {
			selectedURL, selectedType, rank = update.URL, update.Type, priority
		}
	}
	var notice, noticeType, source string
	err = tx.QueryRow(r.Context(), `UPDATE scholarly_works SET notice_url=CASE WHEN $2<>'' THEN $2 ELSE notice_url END,notice_type=CASE WHEN $4<>'' THEN $4 ELSE notice_type END,notice_checked_at=now(),notice_provider='crossref',updated_at=now() WHERE id=$1 AND doi=$3 RETURNING notice_url,notice_type,coalesce(source_id::text,'')`, id, selectedURL, name, selectedType).Scan(&notice, &noticeType, &source)
	if err != nil {
		fail(w, 409, "paper identifier changed during notice check")
		return
	}
	if notice != "" {
		if metadata.RetractionNoticeURL != "" {
			_, err = tx.Exec(r.Context(), `UPDATE doi_references SET retraction_notice_url=$2,updated_at=now() WHERE doi=$1 AND retraction_notice_url=''`, name, metadata.RetractionNoticeURL)
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), `WITH RECURSIVE affected(id) AS (
			 SELECT source_id FROM scholarly_works WHERE id=$1 AND source_id IS NOT NULL
			 UNION SELECT source_id FROM doi_references WHERE doi=$2 AND source_id IS NOT NULL
			 UNION ALL SELECT s.id FROM sources s JOIN affected a ON s.supersedes_source_id=a.id
			) UPDATE sources SET status='disabled',updated_at=now() WHERE id IN (SELECT id FROM affected) AND status='published'`, id, name)
		}
		if err != nil {
			fail(w, 500, "could not apply notice eligibility gate")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "could not save notice check")
		return
	}
	write(w, 200, map[string]any{"notice_url": notice, "notice_type": noticeType, "updates": metadata.Updates, "checked_at": time.Now().UTC(), "provider": "crossref", "source_id": source})
}

func paperNoticePriority(kind string) int {
	switch kind {
	case "retraction", "withdrawal", "removal":
		return 4
	case "partial_retraction", "expression_of_concern":
		return 3
	case "correction", "corrigendum", "erratum":
		return 2
	default:
		return 1
	}
}
