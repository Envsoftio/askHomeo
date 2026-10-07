package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/doi"
)

func (a *API) doiClient() *doi.Client {
	if a.DOI != nil {
		return a.DOI
	}
	return &doi.Client{}
}

func (a *API) addDOIReference(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var b struct {
		DOI string `json:"doi"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&b); err != nil {
		fail(w, 400, "enter a DOI")
		return
	}
	if _, err := doi.Normalize(b.DOI); err != nil {
		fail(w, 400, err.Error())
		return
	}
	metadata, err := a.doiClient().Lookup(r.Context(), b.DOI)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	var id uuid.UUID
	err = a.Store.DB.QueryRow(r.Context(), `INSERT INTO doi_references(id,doi,title,authors,publication_year,publisher,work_type,doi_url,pdf_url,license_url,metadata_provider,retraction_notice_url)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT(doi) DO UPDATE SET title=CASE WHEN doi_references.source_id IS NULL THEN EXCLUDED.title ELSE doi_references.title END,
 authors=CASE WHEN doi_references.source_id IS NULL THEN EXCLUDED.authors ELSE doi_references.authors END,
 publication_year=CASE WHEN doi_references.source_id IS NULL THEN EXCLUDED.publication_year ELSE doi_references.publication_year END,
 publisher=CASE WHEN doi_references.source_id IS NULL THEN EXCLUDED.publisher ELSE doi_references.publisher END,
 work_type=CASE WHEN doi_references.source_id IS NULL THEN EXCLUDED.work_type ELSE doi_references.work_type END,
 pdf_url=CASE WHEN doi_references.source_id IS NULL THEN EXCLUDED.pdf_url ELSE doi_references.pdf_url END,
 license_url=CASE WHEN doi_references.source_id IS NULL THEN EXCLUDED.license_url ELSE doi_references.license_url END,
 retraction_notice_url=CASE WHEN EXCLUDED.retraction_notice_url<>'' THEN EXCLUDED.retraction_notice_url ELSE doi_references.retraction_notice_url END,
 hidden_at=NULL,updated_at=now() RETURNING id`, uuid.New(), metadata.DOI, metadata.Title, metadata.Authors, metadata.Year, metadata.Publisher, metadata.WorkType, metadata.DOIURL, metadata.PDFURL, metadata.LicenseURL, metadata.Provider, metadata.RetractionNoticeURL).Scan(&id)
	if err != nil {
		fail(w, 500, "could not save DOI reference")
		return
	}
	var notice string
	if err := a.Store.DB.QueryRow(r.Context(), `SELECT retraction_notice_url FROM doi_references WHERE id=$1`, id).Scan(&notice); err != nil {
		fail(w, 500, "could not read DOI status")
		return
	}
	if notice != "" {
		if _, err := a.Store.DB.Exec(r.Context(), `WITH RECURSIVE affected(id) AS (SELECT source_id FROM doi_references WHERE id=$1 AND source_id IS NOT NULL UNION ALL SELECT s.id FROM sources s JOIN affected a ON s.supersedes_source_id=a.id) UPDATE sources SET status='disabled',updated_at=now() WHERE id IN (SELECT id FROM affected) AND status='published'`, id); err != nil {
			fail(w, 500, "could not disable retracted source")
			return
		}
	}
	write(w, 200, map[string]any{"id": id, "doi": metadata.DOI, "title": metadata.Title, "authors": metadata.Authors, "doi_url": metadata.DOIURL, "pdf_available": metadata.PDFURL != "" && notice == "", "retraction_notice_url": notice, "reference_only": true})
}

func (a *API) doiReferences(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.DB.Query(r.Context(), `SELECT id,doi,title,authors,coalesce(publication_year,0),publisher,work_type,doi_url,pdf_url,license_url,metadata_provider,coalesce(source_id::text,''),retraction_notice_url FROM doi_references WHERE hidden_at IS NULL ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		fail(w, 500, "could not list DOI references")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id uuid.UUID
		var name, title, authors, publisher, kind, doiURL, pdfURL, licenseURL, provider, sourceID, notice string
		var year int
		if err := rows.Scan(&id, &name, &title, &authors, &year, &publisher, &kind, &doiURL, &pdfURL, &licenseURL, &provider, &sourceID, &notice); err != nil {
			fail(w, 500, "could not read DOI reference")
			return
		}
		out = append(out, map[string]any{"id": id, "doi": name, "title": title, "authors": authors, "publication_year": year, "publisher": publisher, "work_type": kind, "doi_url": doiURL, "pdf_available": pdfURL != "" && notice == "", "license_url": licenseURL, "metadata_provider": provider, "source_id": sourceID, "retraction_notice_url": notice})
	}
	if rows.Err() != nil {
		fail(w, 500, "could not finish DOI references")
		return
	}
	write(w, 200, out)
}

func (a *API) deleteDOIReference(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	conn, err := a.Store.DB.Acquire(r.Context())
	if err != nil {
		fail(w, 500, "database unavailable")
		return
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(r.Context(), `SELECT pg_try_advisory_lock(77321,hashtext($1))`, id.String()).Scan(&locked); err != nil {
		fail(w, 500, "could not lock reference")
		return
	}
	if !locked {
		fail(w, 409, "this PDF is being imported; try again when import finishes")
		return
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(77321,hashtext($1))`, id.String())
	}()
	var linked bool
	err = conn.QueryRow(r.Context(), `SELECT source_id IS NOT NULL FROM doi_references WHERE id=$1 AND hidden_at IS NULL`, id).Scan(&linked)
	if err != nil {
		fail(w, 404, "DOI reference not found")
		return
	}
	if linked {
		_, err = conn.Exec(r.Context(), `UPDATE doi_references SET hidden_at=now(),updated_at=now() WHERE id=$1`, id)
	} else {
		_, err = conn.Exec(r.Context(), `DELETE FROM doi_references WHERE id=$1 AND source_id IS NULL`, id)
	}
	if err != nil {
		fail(w, 500, "could not delete DOI reference")
		return
	}
	write(w, 200, map[string]any{"deleted": true, "source_retained": linked})
}

func (a *API) importDOIReference(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	conn, err := a.Store.DB.Acquire(r.Context())
	if err != nil {
		fail(w, 500, "database unavailable")
		return
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(r.Context(), `SELECT pg_try_advisory_lock(77321,hashtext($1))`, id.String()).Scan(&locked); err != nil {
		fail(w, 500, "could not lock reference")
		return
	}
	if !locked {
		fail(w, 409, "this PDF is already being imported")
		return
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(77321,hashtext($1))`, id.String())
	}()
	var name, title, authors, publisher, pdfURL, licenseURL, sourceID, notice string
	var year int
	err = conn.QueryRow(r.Context(), `SELECT doi,title,authors,publisher,coalesce(publication_year,0),pdf_url,license_url,coalesce(source_id::text,''),retraction_notice_url FROM doi_references WHERE id=$1 AND hidden_at IS NULL`, id).Scan(&name, &title, &authors, &publisher, &year, &pdfURL, &licenseURL, &sourceID, &notice)
	if err != nil {
		fail(w, 404, "DOI reference not found")
		return
	}
	if notice != "" {
		fail(w, 409, "This article was retracted and cannot be imported for answers. See "+notice)
		return
	}
	if sourceID != "" {
		write(w, 200, map[string]any{"source_id": sourceID, "status": "already_imported"})
		return
	}
	if pdfURL == "" || licenseURL == "" {
		fail(w, 409, "No eligible PDF link is recorded for this DOI; upload a permitted PDF if available.")
		return
	}
	file, err := doi.FetchPDF(r.Context(), pdfURL)
	if err != nil {
		fail(w, 502, "PDF could not be downloaded: "+err.Error())
		return
	}
	defer file.Close()
	publication := strings.TrimSpace(fmt.Sprintf("%s (%d)", publisher, year))
	newID, err := a.Store.ImportPDFWithOrigin(r.Context(), file, title, authors, "", publication, "Crossref DOI", "https://doi.org/"+name, licenseURL, pdfURL)
	if err != nil {
		fail(w, 502, "Downloaded file could not be imported: "+err.Error())
		return
	}
	tag, err := conn.Exec(r.Context(), `UPDATE doi_references SET source_id=$2,updated_at=now() WHERE id=$1 AND source_id IS NULL`, id, newID)
	if err != nil {
		fail(w, 500, "PDF saved but reference could not be linked; source ID: "+newID.String())
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 409, "PDF saved but the reference was linked concurrently; source ID: "+newID.String())
		return
	}
	write(w, 202, map[string]any{"source_id": newID, "status": "queued"})
}
