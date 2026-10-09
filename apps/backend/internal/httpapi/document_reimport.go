package httpapi

import (
	"encoding/json"
	"net/http"

	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/document"
	"homeopath-poc/backend/internal/safefetch"
)

func (a *API) reimportDocumentURL(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		AllowHTTPRedirect bool   `json:"allow_https_to_http_redirect"`
		CharsetOverride   string `json:"charset_override"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			fail(w, 400, "invalid reimport options")
			return
		}
	}
	var title, author, edition, publication, repository, sourceURL, rightsStatement, requestedURL, format, previousCharset string
	var categories []string
	var evidenceCategory, categoryOrigin string
	var candidateCount int
	err := a.Store.DB.QueryRow(r.Context(), `SELECT s.title,s.author,s.edition,s.publication_info,s.repository,coalesce(s.source_url,''),s.rights_statement,a.requested_url,s.document_format,coalesce(s.document_charset_override,''),s.literature_categories,s.evidence_category,s.literature_category_origin,(SELECT count(*) FROM sources next WHERE next.supersedes_source_id=s.id AND next.status NOT IN ('failed','disabled')) FROM sources s JOIN document_acquisitions a ON a.source_id=s.id WHERE s.id=$1 AND s.status='published' AND s.superseded_at IS NULL`, id).Scan(&title, &author, &edition, &publication, &repository, &sourceURL, &rightsStatement, &requestedURL, &format, &previousCharset, &categories, &evidenceCategory, &categoryOrigin, &candidateCount)
	if err != nil || requestedURL == "" || format == "pdf" {
		fail(w, 409, "published URL document is unavailable for reimport")
		return
	}
	if candidateCount > 0 {
		fail(w, 409, "a replacement candidate already exists")
		return
	}
	fetcher := a.Fetcher
	if fetcher == nil {
		fetcher = &safefetch.Fetcher{}
	}
	result, err := fetcher.Fetch(r.Context(), requestedURL, document.MaxBytes, body.AllowHTTPRedirect)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	defer result.Body.Close()
	if body.CharsetOverride == "" {
		body.CharsetOverride = previousCharset
	}
	transport := "https"
	if result.Unencrypted {
		transport = "http"
	}
	info := core.DocumentImport{Title: title, Author: author, Edition: edition, PublicationInfo: publication, Repository: repository, SourceURL: sourceURL, RightsStatement: rightsStatement, RequestedURL: result.RequestedURL, FinalURL: result.FinalURL, Transport: transport, ContentType: result.ContentType, CharsetOverride: body.CharsetOverride, SupersedesSourceID: &id, LiteratureCategories: categories, EvidenceCategory: evidenceCategory, LiteratureCategoryOrigin: categoryOrigin}
	candidateID, err := a.Store.ImportDocument(r.Context(), result.Body, info)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.writeImportedSource(w, r, candidateID)
}
