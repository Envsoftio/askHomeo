package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/archive"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/doi"
	"homeopath-poc/backend/internal/localllm"
)

type API struct {
	Store         *core.Store
	Token         string
	ReviewerToken string
	Principals    []Principal
	Model         *localllm.Client
	DOI           *doi.Client
	Archive       *archive.Client
}
type roleKey struct{}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/v1/session", a.openSession)
	mux.HandleFunc("GET /api/v1/session", a.currentSession)
	mux.HandleFunc("DELETE /api/v1/session", a.closeSession)
	mux.HandleFunc("POST /api/v1/imports/nash", a.importNash)
	mux.HandleFunc("POST /api/v1/imports/farrington", a.importFarrington)
	mux.HandleFunc("POST /api/v1/sources/upload", a.uploadPDF)
	mux.HandleFunc("POST /api/v1/sources/import-url", a.importPDFURL)
	mux.HandleFunc("POST /api/v1/sources/{id}/reprocess", a.reprocess)
	mux.HandleFunc("POST /api/v1/sources/{id}/disable", a.disableSource)
	mux.HandleFunc("POST /api/v1/sources/{id}/enable", a.enableSource)
	mux.HandleFunc("GET /api/v1/repositories/archive/search", a.searchArchive)
	mux.HandleFunc("GET /api/v1/repositories/archive/items/{id}", a.archiveItem)
	mux.HandleFunc("POST /api/v1/doi-references", a.addDOIReference)
	mux.HandleFunc("GET /api/v1/doi-references", a.doiReferences)
	mux.HandleFunc("DELETE /api/v1/doi-references/{id}", a.deleteDOIReference)
	mux.HandleFunc("POST /api/v1/doi-references/{id}/import", a.importDOIReference)
	mux.HandleFunc("PATCH /api/v1/sources/{id}/metadata", a.updateMetadata)
	mux.HandleFunc("PATCH /api/v1/pages/{id}/label", a.updatePageLabel)
	mux.HandleFunc("GET /api/v1/sources/{id}/pages/{index}/image", a.pageImage)
	mux.HandleFunc("GET /api/v1/sources", a.sources)
	mux.HandleFunc("GET /api/v1/sources/{id}", a.source)
	mux.HandleFunc("GET /api/v1/sources/{id}/pages", a.pages)
	mux.HandleFunc("GET /api/v1/sources/{id}/pdf", a.pdf)
	mux.HandleFunc("POST /api/v1/sources/{id}/rights", a.rights)
	mux.HandleFunc("POST /api/v1/sources/{id}/publish", a.publish)
	mux.HandleFunc("POST /api/v1/pages/{id}/review", a.reviewPage)
	mux.HandleFunc("POST /api/v1/research/questions", a.question)
	mux.HandleFunc("POST /api/v1/research/answer-jobs", a.queueAnswer)
	mux.HandleFunc("GET /api/v1/research/answer-jobs/{id}", a.answerJob)
	mux.HandleFunc("GET /api/v1/admin/answer-jobs/{id}/evaluation", a.answerEvaluation)
	mux.HandleFunc("POST /api/v1/research/answer-jobs/{id}/retry", a.retryAnswerJob)
	mux.HandleFunc("GET /api/v1/activity", a.activity)
	mux.HandleFunc("POST /api/v1/activity/{id}/read", a.readActivity)
	mux.HandleFunc("GET /api/v1/sources/{id}/index-status", a.indexStatus)
	mux.HandleFunc("POST /api/v1/sources/{id}/reindex", a.reindex)
	mux.HandleFunc("GET /api/v1/research/answers/{id}", a.savedAnswer)
	mux.HandleFunc("GET /api/v1/research/answers/{id}/claims", a.answerClaims)
	mux.HandleFunc("GET /api/v1/citations/{id}", a.citation)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		fromCookie := false
		if provided == "" {
			if cookie, err := r.Cookie("homeopath_session"); err == nil {
				provided = cookie.Value
				fromCookie = true
			}
		}
		principal, authenticated := a.authenticate(provided)
		if r.URL.Path != "/api/v1/health" && r.URL.Path != "/api/v1/session" && !authenticated {
			fail(w, 401, "authentication required")
			return
		}
		if authenticated && fromCookie && !(r.URL.Path == "/api/v1/session" && (r.Method == http.MethodPost || r.Method == http.MethodDelete)) {
			setSessionCookie(w, principal.Token)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 300*time.Second)
		defer cancel()
		ctx = context.WithValue(ctx, roleKey{}, principal.Role)
		ctx = context.WithValue(ctx, principalKey{}, principal)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]string{"error": msg})
}
func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		fail(w, 400, "invalid ID")
		return uuid.Nil, false
	}
	return id, true
}
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if r.Context().Value(roleKey{}) != "admin" {
		fail(w, 403, "admin access required")
		return false
	}
	return true
}
func (a *API) importNash(w http.ResponseWriter, r *http.Request) {
	a.importStarter(w, r, "nash")
}
func (a *API) importFarrington(w http.ResponseWriter, r *http.Request) {
	a.importStarter(w, r, "farrington")
}
func (a *API) importStarter(w http.ResponseWriter, r *http.Request, key string) {
	if !requireAdmin(w, r) {
		return
	}
	id, err := a.Store.ImportStarter(r.Context(), key)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var status string
	if err = a.Store.DB.QueryRow(r.Context(), `SELECT status FROM sources WHERE id=$1`, id).Scan(&status); err != nil {
		fail(w, 500, "could not read source status")
		return
	}
	write(w, 202, map[string]any{"source_id": id, "status": status})
}
func (a *API) uploadPDF(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 251<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		fail(w, 400, "invalid upload or file exceeds 250 MB")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "choose a PDF file")
		return
	}
	defer f.Close()
	id, err := a.Store.ImportPDF(r.Context(), f, r.FormValue("title"), r.FormValue("author"), r.FormValue("edition"), r.FormValue("publication_info"), r.FormValue("repository"), r.FormValue("source_url"), r.FormValue("rights_statement"))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.writeImportedSource(w, r, id)
}

func (a *API) writeImportedSource(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var title, author, edition, publication, repository, sourceURL string
	if err := a.Store.DB.QueryRow(r.Context(), `SELECT title,author,edition,publication_info,repository,coalesce(source_url,'') FROM sources WHERE id=$1`, id).Scan(&title, &author, &edition, &publication, &repository, &sourceURL); err != nil {
		fail(w, 500, "PDF was queued but its details could not be read; source ID: "+id.String())
		return
	}
	write(w, 202, map[string]any{"source_id": id, "status": "queued", "title": title, "author": author, "edition": edition, "publication_info": publication, "repository": repository, "source_url": sourceURL})
}
func (a *API) updateMetadata(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Title           string `json:"title"`
		Author          string `json:"author"`
		Edition         string `json:"edition"`
		PublicationInfo string `json:"publication_info"`
		Repository      string `json:"repository"`
		SourceURL       string `json:"source_url"`
		RightsStatement string `json:"rights_statement"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b); err != nil || strings.TrimSpace(b.Title) == "" || strings.TrimSpace(b.Author) == "" {
		fail(w, 400, "title and author are required")
		return
	}
	if b.SourceURL != "" {
		u, err := url.Parse(b.SourceURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			fail(w, 400, "source URL must be an HTTP or HTTPS address")
			return
		}
	}
	tag, err := a.Store.DB.Exec(r.Context(), `UPDATE sources SET title=$2,author=$3,edition=$4,publication_info=$5,repository=$6,source_url=$7,rights_statement=$8,rights_status=CASE WHEN coalesce(source_url,'')<>$7 OR rights_statement<>$8 THEN 'needs_review' ELSE rights_status END,rights_decision_id=CASE WHEN coalesce(source_url,'')<>$7 OR rights_statement<>$8 THEN NULL ELSE rights_decision_id END,updated_at=now() WHERE id=$1 AND status='review'`, id, strings.TrimSpace(b.Title), strings.TrimSpace(b.Author), strings.TrimSpace(b.Edition), strings.TrimSpace(b.PublicationInfo), strings.TrimSpace(b.Repository), strings.TrimSpace(b.SourceURL), strings.TrimSpace(b.RightsStatement))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "source must be in review")
		return
	}
	write(w, 200, map[string]string{"status": "saved"})
}
func (a *API) updatePageLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Label string `json:"printed_label"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&b); err != nil || len(b.Label) > 80 {
		fail(w, 400, "invalid printed page label")
		return
	}
	tag, err := a.Store.DB.Exec(r.Context(), `UPDATE pages p SET printed_label=$2 FROM sources s WHERE p.id=$1 AND p.source_id=s.id AND s.status='review' AND p.scan_page_index>=0`, id, strings.TrimSpace(b.Label))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "page must be in review")
		return
	}
	write(w, 200, map[string]string{"printed_label": strings.TrimSpace(b.Label)})
}
func (a *API) pageImage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || index < 0 {
		fail(w, 400, "invalid page")
		return
	}
	var sha, status, key string
	var count int
	if err = a.Store.DB.QueryRow(r.Context(), `SELECT pdf_sha256,status,source_key,page_count-1 FROM sources WHERE id=$1`, id).Scan(&sha, &status, &key, &count); err != nil {
		fail(w, 404, "source not found")
		return
	}
	if !strings.HasPrefix(key, "upload-") || index >= count || status == "disabled" {
		fail(w, 404, "page unavailable")
		return
	}
	dir, err := os.MkdirTemp("", "pdf-preview-*")
	if err != nil {
		fail(w, 500, "preview unavailable")
		return
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "page.png")
	page := strconv.Itoa(index + 1)
	cmd := exec.CommandContext(r.Context(), "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=png16m", "-r110", "-dFirstPage="+page, "-dLastPage="+page, "-sOutputFile="+output, "-f", a.Store.PDFPath(sha))
	if _, err = cmd.CombinedOutput(); err != nil {
		fail(w, 500, "preview unavailable")
		return
	}
	data, err := os.ReadFile(output)
	if err != nil || len(data) > 16<<20 {
		fail(w, 500, "preview unavailable")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}
func (a *API) sources(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.DB.Query(r.Context(), `SELECT s.id,s.title,s.author,s.status,s.rights_status,coalesce(j.completed,0),coalesce(j.total,0),coalesce(j.error,''),s.supersedes_source_id,s.superseded_at IS NOT NULL FROM sources s LEFT JOIN jobs j ON j.source_id=s.id AND j.kind='ingest' ORDER BY s.created_at DESC LIMIT 50`)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id uuid.UUID
		var supersedes *uuid.UUID
		var superseded bool
		var title, author, status, rights, problem string
		var completed, total int
		if err = rows.Scan(&id, &title, &author, &status, &rights, &completed, &total, &problem, &supersedes, &superseded); err != nil {
			fail(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{"id": id, "title": title, "author": author, "status": status, "rights_status": rights, "pages_read": completed, "pages_total": total, "error": problem, "supersedes_source_id": supersedes, "superseded": superseded})
	}
	write(w, 200, out)
}
func (a *API) source(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var title, author, status, rights, sha, path string
	var pages, reviewed, unclassified, missing, suspect, checked, textTotal int
	err := a.Store.DB.QueryRow(r.Context(), `SELECT title,author,status,rights_status,pdf_sha256,pdf_path,(SELECT count(*) FROM pages WHERE source_id=s.id AND scan_page_index>=0),(SELECT count(*) FROM pages WHERE source_id=s.id AND review_status='reviewed' AND page_kind='text'),(SELECT count(*) FROM pages WHERE source_id=s.id AND page_kind='unclassified'),(SELECT count(*) FROM pages WHERE source_id=s.id AND page_kind='missing_text'),(SELECT count(*) FROM pages WHERE source_id=s.id AND page_kind='text' AND text_qa_status='suspect'),(SELECT count(*) FROM pages WHERE source_id=s.id AND scan_page_index>=0 AND page_kind='text' AND text_qa_at IS NOT NULL),(SELECT count(*) FROM pages WHERE source_id=s.id AND scan_page_index>=0 AND page_kind='text') FROM sources s WHERE id=$1`, id).Scan(&title, &author, &status, &rights, &sha, &path, &pages, &reviewed, &unclassified, &missing, &suspect, &checked, &textTotal)
	if err != nil {
		fail(w, 404, "source not found")
		return
	}
	var front, beginning, middle, end int
	err = a.Store.DB.QueryRow(r.Context(), `SELECT count(*) FILTER (WHERE scan_page_index BETWEEN 0 AND 9 AND review_status='reviewed' AND page_kind='text'),count(*) FILTER (WHERE scan_page_index BETWEEN 10 AND 99 AND review_status='reviewed' AND page_kind='text'),count(*) FILTER (WHERE scan_page_index BETWEEN 100 AND 299 AND review_status='reviewed' AND page_kind='text'),count(*) FILTER (WHERE scan_page_index>=300 AND review_status='reviewed' AND page_kind='text') FROM pages WHERE source_id=$1`, id).Scan(&front, &beginning, &middle, &end)
	if err != nil {
		fail(w, 500, "could not read page checks")
		return
	}
	var triageStatus, triageError string
	var triageDone, triageTotal, autoBlank int
	err = a.Store.DB.QueryRow(r.Context(), `SELECT coalesce(j.status,''),coalesce(j.error,''),coalesce(j.completed,0),coalesce(j.total,0),
 (SELECT count(*) FROM pages WHERE source_id=$1 AND page_kind='blank' AND review_status='auto_checked')
 FROM sources s LEFT JOIN jobs j ON j.source_id=s.id AND j.kind='triage' WHERE s.id=$1`, id).Scan(&triageStatus, &triageError, &triageDone, &triageTotal, &autoBlank)
	if err != nil {
		fail(w, 500, "could not read scan check progress")
		return
	}
	var textQAStatus, textQAError string
	err = a.Store.DB.QueryRow(r.Context(), `SELECT coalesce(j.status,''),coalesce(j.error,'') FROM sources s LEFT JOIN jobs j ON j.source_id=s.id AND j.kind='text_qa' WHERE s.id=$1`, id).Scan(&textQAStatus, &textQAError)
	if err != nil {
		fail(w, 500, "could not read text check progress")
		return
	}
	var sourceKey, edition, publicationInfo, repository, sourceURL, rightsStatement, pdfOriginURL string
	if err = a.Store.DB.QueryRow(r.Context(), `SELECT source_key,edition,publication_info,repository,coalesce(source_url,''),rights_statement,pdf_origin_url FROM sources WHERE id=$1`, id).Scan(&sourceKey, &edition, &publicationInfo, &repository, &sourceURL, &rightsStatement, &pdfOriginURL); err != nil {
		fail(w, 500, "could not read source key")
		return
	}
	var editionID, recordID, assetID, revisionID, publishedRevisionID, supersedesID, rightsDecisionID *uuid.UUID
	var superseded bool
	if err = a.Store.DB.QueryRow(r.Context(), `SELECT edition_id,source_record_id,primary_asset_id,current_revision_id,published_revision_id,supersedes_source_id,rights_decision_id,superseded_at IS NOT NULL FROM sources WHERE id=$1`, id).Scan(&editionID, &recordID, &assetID, &revisionID, &publishedRevisionID, &supersedesID, &rightsDecisionID, &superseded); err != nil {
		fail(w, 500, "could not read source provenance")
		return
	}
	var retractionNoticeURL string
	if err = a.Store.DB.QueryRow(r.Context(), `WITH RECURSIVE lineage(id,supersedes_source_id) AS (SELECT id,supersedes_source_id FROM sources WHERE id=$1 UNION ALL SELECT s.id,s.supersedes_source_id FROM sources s JOIN lineage l ON s.id=l.supersedes_source_id) SELECT coalesce((SELECT d.retraction_notice_url FROM doi_references d WHERE d.source_id IN (SELECT id FROM lineage) AND d.retraction_notice_url<>'' LIMIT 1),'')`, id).Scan(&retractionNoticeURL); err != nil {
		fail(w, 500, "could not read publication status")
		return
	}
	var rightsMark, rightsEvidenceURL string
	if !strings.HasPrefix(sourceKey, "upload-") {
		seed, seedErr := a.Store.Starter(sourceKey)
		if seedErr != nil {
			fail(w, 500, "could not read source evidence")
			return
		}
		rightsMark, rightsEvidenceURL = seed.Rights.Mark, seed.Rights.RightsSourceURL
	}
	write(w, 200, map[string]any{"id": id, "title": title, "author": author, "status": status, "rights_status": rights, "pdf_sha256": sha, "pages_read": pages, "pages_reviewed": reviewed, "unclassified_pages": unclassified, "missing_text_pages": missing, "suspect_text_pages": suspect, "text_pages_checked": checked, "text_pages_total": textTotal, "text_qa_status": textQAStatus, "text_qa_error": textQAError, "auto_blank_pages": autoBlank, "triage_status": triageStatus, "triage_error": triageError, "triage_completed": triageDone, "triage_total": triageTotal, "review_coverage": map[string]bool{"front": front > 0, "beginning": beginning > 0, "middle": middle > 0, "end": end > 0}, "pdf_url": "/api/v1/sources/" + id.String() + "/pdf", "source_url": sourceURL, "pdf_origin_url": pdfOriginURL, "rights_mark": rightsMark, "rights_evidence_url": rightsEvidenceURL, "edition": edition, "publication_info": publicationInfo, "repository": repository, "rights_statement": rightsStatement, "retraction_notice_url": retractionNoticeURL, "edition_id": editionID, "source_record_id": recordID, "source_asset_id": assetID, "processing_revision_id": revisionID, "published_revision_id": publishedRevisionID, "supersedes_source_id": supersedesID, "rights_decision_id": rightsDecisionID, "superseded": superseded})
}
func (a *API) pages(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("start"))
	var scanCount int
	if err := a.Store.DB.QueryRow(r.Context(), `SELECT page_count-1 FROM sources WHERE id=$1`, id).Scan(&scanCount); err != nil {
		fail(w, 404, "source not found")
		return
	}
	if start < 0 || start >= scanCount {
		fail(w, 400, "scan range is out of bounds")
		return
	}
	showOmitted := r.URL.Query().Get("show_omitted") == "true"
	attention := r.URL.Query().Get("needs_attention") == "true"
	rows, err := a.Store.DB.Query(r.Context(), `SELECT id,pdf_page_index,scan_page_index,coalesce(printed_label,''),review_status,page_kind,coalesce(review_note,''),text_raw,image_url,triage_reason,text_qa_status,text_qa_reason FROM pages WHERE source_id=$1 AND (($4 AND (page_kind IN ('unclassified','missing_text') OR (page_kind='text' AND text_qa_status='suspect'))) OR (NOT $4 AND scan_page_index >= $2 AND scan_page_index < $2+30 AND ($3 OR page_kind NOT IN ('blank','book_info')))) ORDER BY scan_page_index`, id, start, showOmitted, attention)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	a.writePages(w, rows)
}
func (a *API) writePages(w http.ResponseWriter, rows pgx.Rows) {
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var pid uuid.UUID
		var pdf, scan int
		var label, status, kind, note, txt, img, reason, qaStatus, qaReason string
		if err := rows.Scan(&pid, &pdf, &scan, &label, &status, &kind, &note, &txt, &img, &reason, &qaStatus, &qaReason); err != nil {
			fail(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{"id": pid, "pdf_page_index": pdf, "scan_page_index": scan, "printed_label": label, "review_status": status, "page_kind": kind, "review_note": note, "triage_reason": reason, "text_qa_status": qaStatus, "text_qa_reason": qaReason, "text": txt, "image_url": img})
	}
	if err := rows.Err(); err != nil {
		fail(w, 500, "could not finish loading pages")
		return
	}
	write(w, 200, out)
}
func (a *API) pdf(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var sha, status string
	err := a.Store.DB.QueryRow(r.Context(), `SELECT pdf_sha256,status FROM sources WHERE id=$1`, id).Scan(&sha, &status)
	if err != nil {
		fail(w, 404, "source not found")
		return
	}
	if status == "disabled" {
		fail(w, 403, "source disabled")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, a.Store.PDFPath(sha))
}
func (a *API) reviewPage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		Note    string `json:"note"`
		Outcome string `json:"outcome"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		fail(w, 400, "invalid review")
		return
	}
	note := strings.TrimSpace(body.Note)
	status := "reviewed"
	switch body.Outcome {
	case "text", "illustration":
	case "blank":
		if note == "" {
			note = "Blank page; omitted from answers."
		}
	case "book_info":
		if note == "" {
			note = "Library or book information; omitted from answers."
		}
	case "missing_text":
		status = "needs_ocr"
	default:
		fail(w, 400, "choose a page outcome")
		return
	}
	if note == "" {
		fail(w, 400, "review note required")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not save page review.")
		return
	}
	defer tx.Rollback(r.Context())
	var previousKind string
	var revisionID uuid.UUID
	err = tx.QueryRow(r.Context(), `SELECT p.page_kind,p.processing_revision_id FROM pages p JOIN sources s ON s.id=p.source_id WHERE p.id=$1 AND s.status='review' AND p.scan_page_index>=0 FOR UPDATE OF p`, id).Scan(&previousKind, &revisionID)
	if err != nil {
		fail(w, 409, "page is unavailable for review")
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE pages p SET review_status=$3,review_note=$2,page_kind=$4,triage_reason='',text_qa_status=CASE WHEN $4='text' AND p.text_qa_status='suspect' THEN 'accepted' ELSE p.text_qa_status END FROM sources s WHERE p.id=$1 AND p.source_id=s.id AND s.status='review' AND p.scan_page_index>=0 AND ($4<>'text' OR length(trim(p.text_raw))>0)`, id, note, status, body.Outcome)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "page is unavailable for review or has no text")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO page_review_decisions(id,page_id,processing_revision_id,actor_principal_id,previous_kind,decision_kind,rationale) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), id, revisionID, requestPrincipal(r.Context()).ID, previousKind, body.Outcome, note)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "Could not save page review.")
		return
	}
	write(w, 200, map[string]string{"status": status, "page_kind": body.Outcome})
}
func (a *API) rights(w http.ResponseWriter, r *http.Request) {
	if role := requestRole(r.Context()); role != "admin" && role != "reviewer" {
		fail(w, 403, "reviewer access required")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		Decision     string `json:"decision"`
		Note         string `json:"note"`
		EvidenceURL  string `json:"evidence_url"`
		Jurisdiction string `json:"jurisdiction"`
		Restrictions string `json:"restrictions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		fail(w, 400, "invalid decision")
		return
	}
	if body.Decision != "allowed" && body.Decision != "denied" {
		fail(w, 400, "decision must be allowed or denied")
		return
	}
	if strings.TrimSpace(body.Note) == "" {
		fail(w, 400, "review note required")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not save rights decision.")
		return
	}
	defer tx.Rollback(r.Context())
	var status, statement string
	var revisionID, assetID uuid.UUID
	if err = tx.QueryRow(r.Context(), `SELECT status,rights_statement,current_revision_id,primary_asset_id FROM sources WHERE id=$1 FOR UPDATE`, id).Scan(&status, &statement, &revisionID, &assetID); err != nil {
		fail(w, 404, "source not found")
		return
	}
	if status != "review" {
		fail(w, 409, "source is not ready for review")
		return
	}
	decisionID := uuid.New()
	if _, err = tx.Exec(r.Context(), `INSERT INTO rights_decisions(id,source_id,processing_revision_id,source_asset_id,raw_statement,decision,reviewer_role,rationale,evidence_url,jurisdiction,restrictions,reviewer_principal_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, decisionID, id, revisionID, assetID, statement, body.Decision, requestRole(r.Context()), strings.TrimSpace(body.Note), strings.TrimSpace(body.EvidenceURL), strings.TrimSpace(body.Jurisdiction), strings.TrimSpace(body.Restrictions), requestPrincipal(r.Context()).ID); err != nil {
		fail(w, 500, "Could not record rights decision.")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE sources SET rights_status=$2,rights_decision_id=$3,review_note=$4,reviewed_at=now(),updated_at=now() WHERE id=$1`, id, body.Decision, decisionID, strings.TrimSpace(body.Note)); err != nil {
		fail(w, 500, "Could not save rights decision.")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "Could not save rights decision.")
		return
	}
	write(w, 200, map[string]any{"rights_status": body.Decision, "rights_decision_id": decisionID})
}
func (a *API) publish(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var status, rights, title, author string
	var count, unclassified, missing, suspect, unchecked int
	err = tx.QueryRow(r.Context(), `SELECT status,rights_status,title,author,(SELECT count(*) FROM pages WHERE source_id=$1 AND scan_page_index>=0),(SELECT count(*) FROM pages WHERE source_id=$1 AND page_kind='unclassified'),(SELECT count(*) FROM pages WHERE source_id=$1 AND page_kind='missing_text'),(SELECT count(*) FROM pages WHERE source_id=$1 AND page_kind='text' AND text_qa_status='suspect'),(SELECT count(*) FROM pages WHERE source_id=$1 AND scan_page_index>=0 AND page_kind='text' AND text_qa_at IS NULL) FROM sources WHERE id=$1 FOR UPDATE`, id).Scan(&status, &rights, &title, &author, &count, &unclassified, &missing, &suspect, &unchecked)
	if err != nil {
		fail(w, 404, "source not found")
		return
	}
	var expected int
	if err = tx.QueryRow(r.Context(), `SELECT page_count-1 FROM sources WHERE id=$1`, id).Scan(&expected); err != nil {
		fail(w, 500, "could not read expected page count")
		return
	}
	if status != "review" || rights != "allowed" || count != expected {
		fail(w, 409, fmt.Sprintf("publication requires all %d pages and an allowed rights decision (currently %d pages)", expected, count))
		return
	}
	var rightsDecisionID uuid.UUID
	if err = tx.QueryRow(r.Context(), `SELECT rd.id FROM sources s JOIN rights_decisions rd ON rd.id=s.rights_decision_id WHERE s.id=$1 AND rd.decision='allowed' AND rd.processing_revision_id=s.current_revision_id AND rd.source_asset_id=s.primary_asset_id AND rd.raw_statement=s.rights_statement`, id).Scan(&rightsDecisionID); err != nil {
		fail(w, 409, "Save a current allowed rights decision before publication.")
		return
	}
	if strings.HasPrefix(title, "Untitled PDF (verify details)") || strings.HasPrefix(author, "Unknown author (verify details)") {
		fail(w, 409, "Confirm the source title and author before publication.")
		return
	}
	var retractionNoticeURL string
	if err = tx.QueryRow(r.Context(), `WITH RECURSIVE lineage(id,supersedes_source_id) AS (SELECT id,supersedes_source_id FROM sources WHERE id=$1 UNION ALL SELECT s.id,s.supersedes_source_id FROM sources s JOIN lineage l ON s.id=l.supersedes_source_id) SELECT coalesce((SELECT d.retraction_notice_url FROM doi_references d WHERE d.source_id IN (SELECT id FROM lineage) AND d.retraction_notice_url<>'' LIMIT 1),'')`, id).Scan(&retractionNoticeURL); err != nil {
		fail(w, 500, "could not check publication status")
		return
	}
	if retractionNoticeURL != "" {
		fail(w, 409, "This article was retracted and cannot be published for answers. See "+retractionNoticeURL)
		return
	}
	var qaStatus string
	if err = tx.QueryRow(r.Context(), `SELECT status FROM jobs WHERE source_id=$1 AND kind='text_qa'`, id).Scan(&qaStatus); err != nil || qaStatus != "done" {
		fail(w, 409, "Automatic text checking must finish before making this book available.")
		return
	}
	if unclassified+missing+suspect+unchecked > 0 {
		fail(w, 409, fmt.Sprintf("Resolve %d uncertain scans, %d missing-text scans, %d suspected text pages, and %d unchecked text pages first.", unclassified, missing, suspect, unchecked))
		return
	}
	if a.Model == nil {
		fail(w, 503, "model configuration is missing")
		return
	}
	cfg := a.Model.Config
	var configID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO embedding_configs(id,model_id,model_revision,dimensions,preprocessing_version) VALUES($1,$2,$3,$4,'nash-v1') ON CONFLICT(model_id,model_revision,dimensions,preprocessing_version) DO UPDATE SET model_id=EXCLUDED.model_id RETURNING id`, uuid.New(), cfg.EmbeddingModel, cfg.EmbeddingRevision, cfg.Dimensions).Scan(&configID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var passageCount int
	err = tx.QueryRow(r.Context(), `SELECT count(*) FROM chunks c JOIN pages p ON p.id=c.page_id WHERE c.source_id=$1 AND p.page_kind='text' AND p.text_qa_status IN ('passed','accepted') AND length(c.text_exact)<=2800`, id).Scan(&passageCount)
	if err != nil || passageCount == 0 {
		fail(w, 409, "No eligible passages are ready; check the page text and passage length.")
		return
	}
	var publicationID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO publications(id,source_id) VALUES($1,$2) ON CONFLICT(source_id) DO UPDATE SET source_id=EXCLUDED.source_id RETURNING id`, uuid.New(), id).Scan(&publicationID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE publications p SET processing_revision_id=s.current_revision_id,edition_id=s.edition_id,source_asset_id=s.primary_asset_id,
 metadata_snapshot=jsonb_build_object('title',s.title,'author',s.author,'edition',s.edition,'publication_info',s.publication_info,'repository',s.repository,'source_url',coalesce(s.source_url,'')),
 rights_snapshot=jsonb_build_object('decision',s.rights_status,'statement',s.rights_statement,'note',coalesce(s.review_note,''),'reviewed_at',s.reviewed_at),rights_decision_id=$2,published_by_role='admin',approved_by_principal_id=(SELECT reviewer_principal_id FROM rights_decisions WHERE id=$2),approved_at=(SELECT created_at FROM rights_decisions WHERE id=$2),published_by_principal_id=$3,published_at=now()
 ,page_labels_snapshot=(SELECT coalesce(jsonb_object_agg(pg.pdf_page_index,coalesce(pg.printed_label,'')),'{}'::jsonb) FROM pages pg WHERE pg.source_id=s.id)
 FROM sources s WHERE p.id=$1 AND s.id=p.source_id`, publicationID, rightsDecisionID, requestPrincipal(r.Context()).ID)
	if err != nil {
		fail(w, 500, "Could not save publication provenance.")
		return
	}
	var runID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO index_runs(id,publication_id,embedding_config_id,status,expected_chunk_count) VALUES($1,$2,$3,'pending',$4) ON CONFLICT(publication_id,embedding_config_id) DO UPDATE SET expected_chunk_count=EXCLUDED.expected_chunk_count RETURNING id`, uuid.New(), publicationID, configID, passageCount).Scan(&runID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var jobID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO jobs(id,source_id,kind,status,current_step,total) VALUES($1,$2,'embed','queued','Preparing passages',$3) ON CONFLICT(source_id,kind) DO UPDATE SET status='queued',total=EXCLUDED.total,attempts=0,error=NULL RETURNING id`, uuid.New(), id, passageCount).Scan(&jobID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO embedding_jobs(job_id,index_run_id) VALUES($1,$2) ON CONFLICT(job_id) DO UPDATE SET index_run_id=EXCLUDED.index_run_id`, jobID, runID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE sources SET status='published',published_revision_id=current_revision_id,updated_at=now() WHERE id=$1`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE processing_revisions SET status='published',completed_at=coalesce(completed_at,now()) WHERE id=(SELECT current_revision_id FROM sources WHERE id=$1)`, id)
	if err != nil {
		fail(w, 500, "Could not publish processing revision.")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]string{"status": "published"})
}
func (a *API) citation(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var chunk, page, source, editionID, assetID, revisionID, publicationID uuid.UUID
	var passage, title, author, label, sha, img, pageText, pageSHA, sourceURL string
	var pdfIndex, scanIndex int
	principal := requestPrincipal(r.Context())
	err := a.Store.DB.QueryRow(r.Context(), `SELECT c.id,p.id,s.id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),sa.sha256,p.image_url,p.pdf_page_index,p.scan_page_index,p.text_raw,p.text_sha256,coalesce(s.source_url,''),pub.edition_id,sa.id,p.processing_revision_id,pub.id FROM answer_citations ac JOIN answers ans ON ans.id=ac.answer_id JOIN chunks c ON c.id=ac.chunk_id JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id JOIN source_assets sa ON sa.id=p.source_asset_id JOIN publications pub ON pub.source_id=s.id AND pub.processing_revision_id=p.processing_revision_id WHERE ac.id=$1 AND ($2='admin' OR ans.owner_principal_id=$3) AND s.status='published' AND s.rights_status='allowed' AND p.page_kind='text' AND substring(p.text_raw from c.start_character+1 for c.end_character-c.start_character)=c.text_exact`, id, principal.Role, principal.ID).Scan(&chunk, &page, &source, &passage, &title, &author, &label, &sha, &img, &pdfIndex, &scanIndex, &pageText, &pageSHA, &sourceURL, &editionID, &assetID, &revisionID, &publicationID)
	if err != nil {
		fail(w, 404, "citation not available")
		return
	}
	digest := sha256.Sum256([]byte(pageText))
	if hex.EncodeToString(digest[:]) != pageSHA {
		fail(w, 409, "original page checksum does not match")
		return
	}
	write(w, 200, map[string]any{"id": id, "chunk_id": chunk, "page_id": page, "source_id": source, "edition_id": editionID, "source_asset_id": assetID, "processing_revision_id": revisionID, "publication_id": publicationID, "passage": passage, "title": title, "author": author, "printed_page": label, "scan_position": scanIndex + 1, "pdf_page_index": pdfIndex, "pdf_sha256": sha, "image_url": img, "pdf_url": "/api/v1/sources/" + source.String() + "/pdf#page=" + strconv.Itoa(pdfIndex+1), "source_url": sourceURL})
}
func Token() string { return os.Getenv("API_TOKEN") }

var words = regexp.MustCompile(`[A-Za-z]+`)

func queryTerms(q string) []string {
	stop := map[string]bool{"which": true, "what": true, "does": true, "with": true, "about": true, "from": true, "there": true, "these": true, "those": true, "the": true, "and": true, "for": true, "how": true, "page": true, "inspected": true}
	out := []string{}
	seen := map[string]bool{}
	for _, word := range words.FindAllString(strings.ToLower(q), -1) {
		if len(word) < 3 || stop[word] || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
		if len(out) == 8 {
			break
		}
	}
	return out
}
