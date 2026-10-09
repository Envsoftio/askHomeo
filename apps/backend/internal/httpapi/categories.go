package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/classify"
)

var literatureValues = map[string]bool{"materia_medica": true, "repertory": true, "organon_philosophy": true, "therapeutics": true, "provings": true, "clinical_cases": true, "research": true, "other": true, "unclassified": true}
var evidenceValues = map[string]bool{"unknown": true, "classical_reference": true, "published_case": true, "trial_study": true, "systematic_review": true, "trial_registration": true, "guideline_safety": true, "other": true}

func validLiteratureCategories(values []string) bool {
	if len(values) == 0 || len(values) > 8 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !literatureValues[value] || seen[value] || value == "unclassified" && len(values) > 1 {
			return false
		}
		seen[value] = true
	}
	return true
}

func (a *API) setLiteratureCategories(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		Categories       []string `json:"categories"`
		Origin           string   `json:"origin"`
		Rationale        string   `json:"rationale"`
		EvidenceCategory string   `json:"evidence_category"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !validLiteratureCategories(body.Categories) {
		fail(w, 400, "choose valid literature categories")
		return
	}
	if body.Origin != "accepted_suggestion" && body.Origin != "manual" {
		fail(w, 400, "choose accepted_suggestion or manual origin")
		return
	}
	if strings.TrimSpace(body.Rationale) == "" {
		fail(w, 400, "record a category decision reason")
		return
	}
	if body.EvidenceCategory != "" && !evidenceValues[body.EvidenceCategory] {
		fail(w, 400, "choose a valid evidence category")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	var revisionID uuid.UUID
	var previous []string
	var status, previousEvidence string
	err = tx.QueryRow(r.Context(), `SELECT current_revision_id,literature_categories,status,evidence_category FROM sources WHERE id=$1 FOR UPDATE`, id).Scan(&revisionID, &previous, &status, &previousEvidence)
	if err != nil {
		fail(w, 404, "source not found")
		return
	}
	if status != "review" && status != "published" && status != "disabled" {
		fail(w, 409, "source is not ready for category review")
		return
	}
	if body.Origin == "accepted_suggestion" {
		var suggested []string
		if err = tx.QueryRow(r.Context(), `SELECT categories FROM literature_category_suggestions WHERE processing_revision_id=$1 AND state='suggested'`, revisionID).Scan(&suggested); err != nil || !equalCategories(suggested, body.Categories) {
			fail(w, 409, "current suggestion differs from chosen categories")
			return
		}
	}
	newOrigin := "manual"
	if body.Origin == "accepted_suggestion" {
		newOrigin = "automatic"
	}
	if body.EvidenceCategory == "" {
		body.EvidenceCategory = previousEvidence
	}
	_, err = tx.Exec(r.Context(), `UPDATE sources SET literature_categories=$2,literature_category_origin=$3,evidence_category=$4,updated_at=now() WHERE id=$1`, id, body.Categories, newOrigin, body.EvidenceCategory)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO literature_category_decisions(id,source_id,processing_revision_id,actor_principal_id,previous_categories,chosen_categories,previous_evidence_category,chosen_evidence_category,origin,rationale) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, uuid.New(), id, revisionID, requestPrincipal(r.Context()).ID, previous, body.Categories, previousEvidence, body.EvidenceCategory, body.Origin, strings.TrimSpace(body.Rationale))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"categories": body.Categories, "origin": newOrigin, "evidence_category": body.EvidenceCategory})
}
func equalCategories(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	found := map[string]bool{}
	for _, v := range a {
		found[v] = true
	}
	for _, v := range b {
		if !found[v] {
			return false
		}
	}
	return true
}

func (a *API) retryLiteratureClassification(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var format, content string
	var revisionID, assetID uuid.UUID
	err := a.Store.DB.QueryRow(r.Context(), `SELECT document_format,current_revision_id,primary_asset_id FROM sources WHERE id=$1`, id).Scan(&format, &revisionID, &assetID)
	if err != nil {
		fail(w, 404, "source not found")
		return
	}
	if format == "pdf" {
		err = a.Store.DB.QueryRow(r.Context(), `SELECT coalesce(string_agg(text_raw,E'\n' ORDER BY pdf_page_index),'') FROM (SELECT text_raw,pdf_page_index FROM pages WHERE source_id=$1 AND page_kind='text' ORDER BY pdf_page_index LIMIT 30) p`, id).Scan(&content)
	} else {
		err = a.Store.DB.QueryRow(r.Context(), `SELECT coalesce(string_agg(heading||E'\n'||original_text,E'\n' ORDER BY block_index),'') FROM document_blocks WHERE processing_revision_id=$1`, revisionID).Scan(&content)
	}
	if err != nil {
		fail(w, 500, "could not read extracted content")
		return
	}
	suggestion := classify.Suggest(content)
	evidence, _ := json.Marshal(suggestion.Evidence)
	_, err = a.Store.DB.Exec(r.Context(), `INSERT INTO literature_category_suggestions(processing_revision_id,source_id,source_asset_id,classifier_version,categories,state,reason,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb) ON CONFLICT(processing_revision_id) DO UPDATE SET classifier_version=EXCLUDED.classifier_version,categories=EXCLUDED.categories,state=EXCLUDED.state,reason=EXCLUDED.reason,evidence=EXCLUDED.evidence`, revisionID, id, assetID, classify.Version, suggestion.Categories, suggestion.State, suggestion.Reason, string(evidence))
	if err != nil {
		fail(w, 500, "could not save classification suggestion")
		return
	}
	write(w, 200, suggestion)
}

func (a *API) setDocumentBlockCategories(w http.ResponseWriter, r *http.Request) {
	a.setSectionCategories(w, r, "document_block_id")
}
func (a *API) setPageCategories(w http.ResponseWriter, r *http.Request) {
	a.setSectionCategories(w, r, "page_id")
}
func (a *API) setSectionCategories(w http.ResponseWriter, r *http.Request, column string) {
	if !requireAdmin(w, r) {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		fail(w, 400, "invalid section ID")
		return
	}
	var body struct {
		Categories []string `json:"categories"`
		Rationale  string   `json:"rationale"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !validLiteratureCategories(body.Categories) || strings.TrimSpace(body.Rationale) == "" {
		fail(w, 400, "categories and review reason are required")
		return
	}
	table := "document_blocks"
	if column == "page_id" {
		table = "pages"
	}
	var sourceID, revisionID uuid.UUID
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	query := `SELECT section.source_id,section.processing_revision_id FROM ` + table + ` section JOIN sources s ON s.id=section.source_id WHERE section.id=$1 AND section.processing_revision_id=s.current_revision_id AND s.status IN ('review','published') FOR UPDATE OF s`
	if err = tx.QueryRow(r.Context(), query, id).Scan(&sourceID, &revisionID); err != nil {
		fail(w, 409, "section unavailable for category review")
		return
	}
	var previous []string
	err = tx.QueryRow(r.Context(), `SELECT categories FROM literature_section_categories WHERE `+column+`=$1`, id).Scan(&previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 500, err.Error())
		return
	}
	statement := `INSERT INTO literature_section_categories(id,source_id,processing_revision_id,` + column + `,categories,actor_principal_id,rationale) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(` + column + `) DO UPDATE SET categories=EXCLUDED.categories,actor_principal_id=EXCLUDED.actor_principal_id,rationale=EXCLUDED.rationale,created_at=now()`
	_, err = tx.Exec(r.Context(), statement, uuid.New(), sourceID, revisionID, id, body.Categories, requestPrincipal(r.Context()).ID, strings.TrimSpace(body.Rationale))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var blockID, pageID any
	if column == "document_block_id" {
		blockID = id
	} else {
		pageID = id
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO literature_section_category_decisions(id,source_id,processing_revision_id,document_block_id,page_id,previous_categories,chosen_categories,actor_principal_id,rationale) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uuid.New(), sourceID, revisionID, blockID, pageID, previous, body.Categories, requestPrincipal(r.Context()).ID, strings.TrimSpace(body.Rationale))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"categories": body.Categories})
}
