package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/document"
	"homeopath-poc/backend/internal/safefetch"
)

func (a *API) uploadDocument(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, document.MaxBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		fail(w, 400, "invalid upload or document exceeds 10 MiB")
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "choose one HTML or TXT document")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, document.MaxBytes+1))
	if err != nil || len(raw) > document.MaxBytes {
		fail(w, 400, "document exceeds 10 MiB")
		return
	}
	contentType := http.DetectContentType(raw)
	format, err := document.Detect(raw, contentType)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if (ext == ".html" || ext == ".htm") && format != "html" || ext == ".txt" && format != "txt" {
		fail(w, 400, "file extension and detected document format differ")
		return
	}
	if ext != ".html" && ext != ".htm" && ext != ".txt" {
		fail(w, 400, "choose a .html, .htm or .txt file")
		return
	}
	categories := r.MultipartForm.Value["literature_categories"]
	if len(categories) > 0 && !validLiteratureCategories(categories) {
		fail(w, 400, "choose valid literature categories")
		return
	}
	info := core.DocumentImport{Title: r.FormValue("title"), Author: r.FormValue("author"), Edition: r.FormValue("edition"), PublicationInfo: r.FormValue("publication_info"), Repository: r.FormValue("repository"), SourceURL: r.FormValue("source_url"), RightsStatement: r.FormValue("rights_statement"), Transport: "upload", ContentType: contentType, CharsetOverride: r.FormValue("charset_override")}
	if len(categories) > 0 {
		actor := requestPrincipal(r.Context()).ID
		info.LiteratureCategories, info.EvidenceCategory, info.LiteratureCategoryOrigin, info.CategoryActorID = categories, "unknown", "manual", &actor
	}
	id, err := a.Store.ImportDocument(r.Context(), strings.NewReader(string(raw)), info)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.writeImportedSource(w, r, id)
}

func (a *API) importDocumentURL(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var b struct {
		URL                  string   `json:"url"`
		Title                string   `json:"title"`
		Author               string   `json:"author"`
		Edition              string   `json:"edition"`
		PublicationInfo      string   `json:"publication_info"`
		Repository           string   `json:"repository"`
		SourceURL            string   `json:"source_url"`
		RightsStatement      string   `json:"rights_statement"`
		CharsetOverride      string   `json:"charset_override"`
		AllowHTTPRedirect    bool     `json:"allow_https_to_http_redirect"`
		LiteratureCategories []string `json:"literature_categories"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b); err != nil {
		fail(w, 400, "invalid document import request")
		return
	}
	if len(b.LiteratureCategories) > 0 && !validLiteratureCategories(b.LiteratureCategories) {
		fail(w, 400, "choose valid literature categories")
		return
	}
	fetcher := a.Fetcher
	if fetcher == nil {
		fetcher = &safefetch.Fetcher{}
	}
	result, err := fetcher.Fetch(r.Context(), strings.TrimSpace(b.URL), document.MaxBytes, b.AllowHTTPRedirect)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	defer result.Body.Close()
	transport := "https"
	if result.Unencrypted {
		transport = "http"
	}
	sourceURL := strings.TrimSpace(b.SourceURL)
	if sourceURL == "" {
		sourceURL = result.FinalURL
	}
	info := core.DocumentImport{Title: b.Title, Author: b.Author, Edition: b.Edition, PublicationInfo: b.PublicationInfo, Repository: b.Repository, SourceURL: sourceURL, RightsStatement: b.RightsStatement, RequestedURL: result.RequestedURL, FinalURL: result.FinalURL, Transport: transport, ContentType: result.ContentType, CharsetOverride: b.CharsetOverride}
	if len(b.LiteratureCategories) > 0 {
		actor := requestPrincipal(r.Context()).ID
		info.LiteratureCategories, info.EvidenceCategory, info.LiteratureCategoryOrigin, info.CategoryActorID = b.LiteratureCategories, "unknown", "manual", &actor
	}
	id, err := a.Store.ImportDocument(r.Context(), result.Body, info)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.writeImportedSource(w, r, id)
}

func (a *API) documentBlocks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if role := requestRole(r.Context()); role != "admin" && role != "reviewer" {
		fail(w, 403, "reviewer access required")
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT b.id,b.block_index,b.section_key,b.kind,b.heading,b.original_text,b.reviewed_text,b.start_byte,b.end_byte,b.review_status,b.review_note,b.warnings,coalesce(sc.categories,s.literature_categories),sc.id IS NOT NULL FROM document_blocks b JOIN sources s ON s.id=b.source_id LEFT JOIN literature_section_categories sc ON sc.document_block_id=b.id WHERE b.source_id=$1 AND b.processing_revision_id=s.current_revision_id ORDER BY b.block_index`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var bid uuid.UUID
		var index, start, end int
		var key, kind, heading, original, reviewed, status, note string
		var warnings, categories []string
		var categoryOverride bool
		if err = rows.Scan(&bid, &index, &key, &kind, &heading, &original, &reviewed, &start, &end, &status, &note, &warnings, &categories, &categoryOverride); err != nil {
			fail(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{"id": bid, "block_index": index, "section_key": key, "kind": kind, "heading": heading, "original_text": original, "reviewed_text": reviewed, "start_byte": start, "end_byte": end, "review_status": status, "review_note": note, "warnings": warnings, "literature_categories": categories, "category_override": categoryOverride})
	}
	if err = rows.Err(); err != nil {
		fail(w, 500, err.Error())
		return
	}
	write(w, 200, out)
}

func (a *API) reviewDocumentBlock(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		fail(w, 400, "invalid block ID")
		return
	}
	var b struct {
		Decision string `json:"decision"`
		Text     string `json:"text"`
		Note     string `json:"note"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b); err != nil {
		fail(w, 400, "invalid block review")
		return
	}
	if b.Decision != "accepted" && b.Decision != "corrected" && b.Decision != "excluded" {
		fail(w, 400, "choose accept, correct or exclude")
		return
	}
	if b.Decision == "corrected" && strings.TrimSpace(b.Text) == "" {
		fail(w, 400, "corrected text is required")
		return
	}
	if b.Decision != "accepted" && strings.TrimSpace(b.Note) == "" {
		fail(w, 400, "review note is required")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var sourceID, assetID, oldRevision uuid.UUID
	var blockIndex int
	err = tx.QueryRow(r.Context(), `SELECT b.source_id,b.source_asset_id,b.processing_revision_id,b.block_index FROM document_blocks b JOIN sources s ON s.id=b.source_id WHERE b.id=$1 AND b.processing_revision_id=s.current_revision_id AND s.status='review' FOR UPDATE OF s`, id).Scan(&sourceID, &assetID, &oldRevision, &blockIndex)
	if err != nil {
		fail(w, 409, "block is unavailable for review")
		return
	}
	targetRevision, targetBlock := oldRevision, id
	if b.Decision == "corrected" {
		targetRevision = uuid.New()
		_, err = tx.Exec(r.Context(), `INSERT INTO processing_revisions(id,source_id,source_asset_id,status,component_versions_json,configuration_json,completed_at) SELECT $1,source_id,source_asset_id,'review',component_versions_json,configuration_json,now() FROM processing_revisions WHERE id=$2`, targetRevision, oldRevision)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO document_blocks(id,source_id,source_asset_id,processing_revision_id,block_index,section_key,kind,heading,original_text,reviewed_text,start_byte,end_byte,review_status,review_note,warnings) SELECT gen_random_uuid(),source_id,source_asset_id,$1,block_index,section_key,kind,heading,original_text,reviewed_text,start_byte,end_byte,review_status,review_note,warnings FROM document_blocks WHERE processing_revision_id=$2`, targetRevision, oldRevision)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO document_locations(id,source_id,source_asset_id,processing_revision_id,kind,section_key) SELECT gen_random_uuid(),source_id,source_asset_id,$1,'section',section_key FROM document_blocks WHERE processing_revision_id=$1 ON CONFLICT DO NOTHING`, targetRevision)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO literature_section_categories(id,source_id,processing_revision_id,document_block_id,categories,actor_principal_id,rationale)
 SELECT gen_random_uuid(),next.source_id,$1,next.id,sc.categories,sc.actor_principal_id,sc.rationale FROM literature_section_categories sc JOIN document_blocks old ON old.id=sc.document_block_id JOIN document_blocks next ON next.processing_revision_id=$1 AND next.block_index=old.block_index WHERE old.processing_revision_id=$2 AND old.block_index<>$3`, targetRevision, oldRevision, blockIndex)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		err = tx.QueryRow(r.Context(), `SELECT id FROM document_blocks WHERE processing_revision_id=$1 AND block_index=$2`, targetRevision, blockIndex).Scan(&targetBlock)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE sources SET current_revision_id=$2,rights_status='needs_review',rights_decision_id=NULL,reviewed_at=NULL,updated_at=now() WHERE id=$1`, sourceID, targetRevision)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
	}
	text := strings.TrimSpace(b.Text)
	if b.Decision != "corrected" {
		text = ""
	}
	_, err = tx.Exec(r.Context(), `UPDATE document_blocks SET review_status=$2,review_note=$3,reviewed_text=CASE WHEN $2='corrected' THEN $4 ELSE original_text END WHERE id=$1`, targetBlock, b.Decision, strings.TrimSpace(b.Note), text)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO document_block_decisions(id,source_id,block_id,processing_revision_id,actor_principal_id,decision,rationale) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), sourceID, targetBlock, targetRevision, requestPrincipal(r.Context()).ID, b.Decision, strings.TrimSpace(b.Note))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"status": b.Decision, "processing_revision_id": targetRevision})
}
