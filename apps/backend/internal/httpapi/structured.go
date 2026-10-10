package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type structuredLocationInput struct {
	TextSHA256      string     `json:"text_sha256,omitempty"`
	PageID          *uuid.UUID `json:"page_id"`
	DocumentBlockID *uuid.UUID `json:"document_block_id"`
	Start           int        `json:"start_character"`
	End             int        `json:"end_character"`
	ExactText       string     `json:"exact_text"`
}

func (a *API) createRemedy(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var b struct {
		CanonicalName  string `json:"canonical_name"`
		PreparationKey string `json:"preparation_key"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil || strings.TrimSpace(b.CanonicalName) == "" || strings.TrimSpace(b.PreparationKey) == "" {
		fail(w, 400, "canonical name and preparation key are required")
		return
	}
	var id uuid.UUID
	err := a.Store.DB.QueryRow(r.Context(), `INSERT INTO remedies(id,canonical_name,preparation_key) VALUES($1,$2,$3)
 ON CONFLICT(canonical_name,preparation_key) DO UPDATE SET canonical_name=EXCLUDED.canonical_name RETURNING id`,
		uuid.New(), strings.TrimSpace(b.CanonicalName), strings.TrimSpace(b.PreparationKey)).Scan(&id)
	if err != nil {
		fail(w, 500, "could not save remedy")
		return
	}
	write(w, 200, map[string]any{"id": id, "canonical_name": strings.TrimSpace(b.CanonicalName), "preparation_key": strings.TrimSpace(b.PreparationKey)})
}

func (a *API) createStructuredEntry(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	sourceID, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Kind                 string                    `json:"kind"`
		ParentID             *uuid.UUID                `json:"parent_id"`
		Ordinal              int                       `json:"ordinal"`
		Heading              string                    `json:"heading"`
		FullPath             []string                  `json:"full_path"`
		SourceRemedySpelling string                    `json:"source_remedy_spelling"`
		RemedyID             *uuid.UUID                `json:"remedy_id"`
		Subsection           string                    `json:"subsection"`
		Locations            []structuredLocationInput `json:"locations"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&b) != nil ||
		(b.Kind != "materia_medica" && b.Kind != "repertory_rubric") || b.Ordinal < 0 ||
		strings.TrimSpace(b.Heading) == "" || len(b.FullPath) == 0 || len(b.Locations) == 0 || len(b.Locations) > 20 {
		fail(w, 400, "invalid structured entry or missing supporting locations")
		return
	}
	for i := range b.FullPath {
		b.FullPath[i] = strings.TrimSpace(b.FullPath[i])
		if b.FullPath[i] == "" {
			fail(w, 400, "entry path has an empty heading")
			return
		}
	}
	if b.FullPath[len(b.FullPath)-1] != strings.TrimSpace(b.Heading) ||
		(b.Kind == "materia_medica" && (b.ParentID != nil || strings.TrimSpace(b.SourceRemedySpelling) == "")) ||
		(b.Kind == "repertory_rubric" && strings.TrimSpace(b.SourceRemedySpelling) != "") {
		fail(w, 400, "entry heading, path or kind is inconsistent")
		return
	}
	for _, loc := range b.Locations {
		if (loc.PageID == nil) == (loc.DocumentBlockID == nil) || loc.Start < 0 || loc.End <= loc.Start || loc.ExactText == "" {
			fail(w, 400, "invalid exact supporting location")
			return
		}
	}
	ctx := r.Context()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, "could not start structured review")
		return
	}
	defer tx.Rollback(ctx)
	var revisionID, assetID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT current_revision_id,primary_asset_id FROM sources WHERE id=$1 AND status='review' FOR UPDATE`, sourceID).Scan(&revisionID, &assetID)
	if err != nil {
		fail(w, 409, "source is not in review")
		return
	}
	if b.ParentID != nil {
		var parentPath []string
		err = tx.QueryRow(ctx, `SELECT full_path FROM structured_entries WHERE id=$1 AND source_id=$2 AND processing_revision_id=$3 AND kind='repertory_rubric'`, b.ParentID, sourceID, revisionID).Scan(&parentPath)
		if err != nil || len(parentPath)+1 != len(b.FullPath) {
			fail(w, 400, "rubric parent is unavailable or path is inconsistent")
			return
		}
		for i := range parentPath {
			if parentPath[i] != b.FullPath[i] {
				fail(w, 400, "rubric path does not match its parent")
				return
			}
		}
	} else if b.Kind == "repertory_rubric" && len(b.FullPath) != 1 {
		fail(w, 400, "nested rubric needs a parent")
		return
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO structured_entries(id,source_id,source_asset_id,processing_revision_id,kind,parent_id,ordinal,heading,full_path,source_remedy_spelling,remedy_id,subsection,adapter_version)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'manual-v1')`, id, sourceID, assetID, revisionID, b.Kind, b.ParentID, b.Ordinal, strings.TrimSpace(b.Heading), b.FullPath, strings.TrimSpace(b.SourceRemedySpelling), b.RemedyID, strings.TrimSpace(b.Subsection))
	if err != nil {
		fail(w, 409, "entry conflicts with this revision or has invalid identity")
		return
	}
	for _, loc := range b.Locations {
		_, err = tx.Exec(ctx, `INSERT INTO structured_entry_locations(entry_id,source_id,processing_revision_id,page_id,document_block_id,start_character,end_character,exact_text)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, sourceID, revisionID, loc.PageID, loc.DocumentBlockID, loc.Start, loc.End, loc.ExactText)
		if err != nil {
			fail(w, 400, "supporting location does not match reviewed source text")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, "could not save structured entry")
		return
	}
	write(w, 201, map[string]any{"id": id, "processing_revision_id": revisionID, "review_status": "pending"})
}

func (a *API) reviewStructuredEntry(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Decision       string     `json:"decision"`
		Rationale      string     `json:"rationale"`
		RemedyID       *uuid.UUID `json:"remedy_id"`
		ValidationNote string     `json:"validation_note"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b) != nil ||
		(b.Decision != "accepted" && b.Decision != "rejected") || strings.TrimSpace(b.Rationale) == "" {
		fail(w, 400, "review decision and rationale are required")
		return
	}
	ctx := r.Context()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, "could not start review")
		return
	}
	defer tx.Rollback(ctx)
	var sourceID, revisionID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT e.source_id,e.processing_revision_id FROM structured_entries e JOIN sources s ON s.id=e.source_id
 WHERE e.id=$1 AND e.processing_revision_id=s.current_revision_id AND s.status='review' AND s.removed_at IS NULL AND (e.review_status='pending' OR ($2='rejected' AND e.review_status IN ('accepted','corrected'))) FOR UPDATE OF s,e`, id, b.Decision).Scan(&sourceID, &revisionID)
	if err != nil {
		fail(w, 409, "entry is unavailable for review")
		return
	}
	_, err = tx.Exec(ctx, `UPDATE structured_entries SET review_status=$2,remedy_id=coalesce($3,remedy_id),validation_note=$4 WHERE id=$1`, id, b.Decision, b.RemedyID, strings.TrimSpace(b.ValidationNote))
	if err != nil {
		fail(w, 409, "entry cannot be approved without a resolved remedy and exact support")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO structured_review_decisions(id,source_id,processing_revision_id,entry_id,actor_principal_id,decision,rationale)
 VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), sourceID, revisionID, id, requestPrincipal(ctx).ID, b.Decision, strings.TrimSpace(b.Rationale))
	if err != nil {
		fail(w, 500, "could not save review decision")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, "could not commit review")
		return
	}
	write(w, 200, map[string]any{"id": id, "review_status": b.Decision})
}

func (a *API) structuredEntries(w http.ResponseWriter, r *http.Request) {
	if role := requestRole(r.Context()); role != "admin" && role != "reviewer" {
		fail(w, 403, "reviewer access required")
		return
	}
	sourceID, ok := parseID(w, r)
	if !ok {
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT e.id,e.processing_revision_id,e.kind,e.parent_id,e.ordinal,e.heading,e.full_path,e.source_remedy_spelling,e.remedy_id,e.subsection,e.review_status,e.adapter_version,e.validation_note,
	coalesce((SELECT jsonb_agg(jsonb_build_object('page_id',l.page_id,'document_block_id',l.document_block_id,'start_character',l.start_character,'end_character',l.end_character,'exact_text',l.exact_text) ORDER BY l.start_character) FROM structured_entry_locations l WHERE l.entry_id=e.id),'[]'::jsonb)
 FROM structured_entries e JOIN sources s ON s.id=e.source_id WHERE e.source_id=$1 AND e.processing_revision_id=s.current_revision_id ORDER BY e.kind,e.ordinal`, sourceID)
	if err != nil {
		fail(w, 500, "could not read structured entries")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, rev uuid.UUID
		var parent, remedy *uuid.UUID
		var kind, heading, spelling, subsection, status, adapter, note string
		var path []string
		var ordinal int
		var locations []byte
		if err = rows.Scan(&id, &rev, &kind, &parent, &ordinal, &heading, &path, &spelling, &remedy, &subsection, &status, &adapter, &note, &locations); err != nil {
			fail(w, 500, "could not read structured entries")
			return
		}
		out = append(out, map[string]any{"id": id, "processing_revision_id": rev, "kind": kind, "parent_id": parent, "ordinal": ordinal, "heading": heading, "full_path": path, "source_remedy_spelling": spelling, "remedy_id": remedy, "subsection": subsection, "review_status": status, "adapter_version": adapter, "validation_note": note, "locations": json.RawMessage(locations)})
	}
	if rows.Err() != nil {
		fail(w, 500, "could not read structured entries")
		return
	}
	write(w, 200, out)
}

func (a *API) createRubricRemedy(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	rubricID, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		RemedyID             *uuid.UUID                `json:"remedy_id"`
		SourceNotation       string                    `json:"source_notation"`
		SourceRemedySpelling string                    `json:"source_remedy_spelling"`
		Grade                *int                      `json:"grade"`
		GradeScheme          string                    `json:"grade_scheme"`
		SourceStyle          string                    `json:"source_style"`
		CategoricalGrade     string                    `json:"categorical_grade"`
		Locations            []structuredLocationInput `json:"locations"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&b) != nil ||
		strings.TrimSpace(b.SourceNotation) == "" || strings.TrimSpace(b.SourceRemedySpelling) == "" ||
		len(b.Locations) == 0 || len(b.Locations) > 20 ||
		(b.Grade != nil && (*b.Grade <= 0 || strings.TrimSpace(b.GradeScheme) == "")) {
		fail(w, 400, "notation, spelling, exact support and a scheme for known grades are required")
		return
	}
	if b.SourceStyle == "" {
		b.SourceStyle = "unknown"
	}
	b.CategoricalGrade = strings.TrimSpace(b.CategoricalGrade)
	b.GradeScheme = strings.TrimSpace(b.GradeScheme)
	if !validSourceStyle(b.SourceStyle) || len(b.CategoricalGrade) > 300 || len(b.GradeScheme) > 2000 || (b.CategoricalGrade != "" && !conventionSupported(b.GradeScheme, b.Locations)) {
		fail(w, 400, "invalid source style or missing exact categorical convention evidence")
		return
	}
	for _, loc := range b.Locations {
		if (loc.PageID == nil) == (loc.DocumentBlockID == nil) || loc.Start < 0 || loc.End <= loc.Start ||
			!strings.Contains(loc.ExactText, strings.TrimSpace(b.SourceNotation)) {
			fail(w, 400, "supporting location must contain the source notation")
			return
		}
	}
	ctx := r.Context()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, "could not start rubric review")
		return
	}
	defer tx.Rollback(ctx)
	var sourceID, revisionID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT e.source_id,e.processing_revision_id FROM structured_entries e JOIN sources s ON s.id=e.source_id
 WHERE e.id=$1 AND e.kind='repertory_rubric' AND e.processing_revision_id=s.current_revision_id AND s.status='review' FOR UPDATE OF s`, rubricID).Scan(&sourceID, &revisionID)
	if err != nil {
		fail(w, 409, "rubric is unavailable for review")
		return
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO rubric_remedies(id,rubric_id,source_id,processing_revision_id,remedy_id,source_notation,source_remedy_spelling,grade,grade_scheme,source_style,categorical_grade)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, rubricID, sourceID, revisionID, b.RemedyID,
		strings.TrimSpace(b.SourceNotation), strings.TrimSpace(b.SourceRemedySpelling), b.Grade, strings.TrimSpace(b.GradeScheme), b.SourceStyle, b.CategoricalGrade)
	if err != nil {
		fail(w, 400, "invalid rubric association")
		return
	}
	for _, loc := range b.Locations {
		_, err = tx.Exec(ctx, `INSERT INTO rubric_remedy_locations(association_id,source_id,processing_revision_id,page_id,document_block_id,start_character,end_character,exact_text)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, sourceID, revisionID, loc.PageID, loc.DocumentBlockID, loc.Start, loc.End, loc.ExactText)
		if err != nil {
			fail(w, 400, "association support does not match reviewed source text")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, "could not save rubric association")
		return
	}
	write(w, 201, map[string]any{"id": id, "processing_revision_id": revisionID, "review_status": "pending"})
}

func (a *API) reviewRubricRemedy(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Decision       string     `json:"decision"`
		Rationale      string     `json:"rationale"`
		RemedyID       *uuid.UUID `json:"remedy_id"`
		ValidationNote string     `json:"validation_note"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b) != nil ||
		(b.Decision != "accepted" && b.Decision != "rejected" && b.Decision != "unresolved") || strings.TrimSpace(b.Rationale) == "" {
		fail(w, 400, "review decision and rationale are required")
		return
	}
	ctx := r.Context()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, "could not start association review")
		return
	}
	defer tx.Rollback(ctx)
	var sourceID, revisionID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT rr.source_id,rr.processing_revision_id FROM rubric_remedies rr JOIN sources s ON s.id=rr.source_id
 WHERE rr.id=$1 AND rr.processing_revision_id=s.current_revision_id AND s.status='review' AND rr.review_status='pending' FOR UPDATE OF rr,s`, id).Scan(&sourceID, &revisionID)
	if err != nil {
		fail(w, 409, "association is unavailable for review")
		return
	}
	_, err = tx.Exec(ctx, `UPDATE rubric_remedies SET review_status=$2,remedy_id=CASE WHEN $2='unresolved' THEN NULL ELSE coalesce($3,remedy_id) END,validation_note=$4 WHERE id=$1`,
		id, b.Decision, b.RemedyID, strings.TrimSpace(b.ValidationNote))
	if err != nil {
		fail(w, 409, "association cannot be approved without a resolved remedy and exact support")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO structured_review_decisions(id,source_id,processing_revision_id,rubric_remedy_id,actor_principal_id,decision,rationale)
 VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), sourceID, revisionID, id, requestPrincipal(ctx).ID, b.Decision, strings.TrimSpace(b.Rationale))
	if err != nil {
		fail(w, 500, "could not save association decision")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, "could not commit association review")
		return
	}
	write(w, 200, map[string]any{"id": id, "review_status": b.Decision})
}

func (a *API) rubricRemedies(w http.ResponseWriter, r *http.Request) {
	if role := requestRole(r.Context()); role != "admin" && role != "reviewer" {
		fail(w, 403, "reviewer access required")
		return
	}
	rubricID, ok := parseID(w, r)
	if !ok {
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT rr.id,rr.processing_revision_id,rr.remedy_id,rr.source_notation,rr.source_remedy_spelling,rr.grade,rr.grade_scheme,rr.review_status,rr.validation_note,rr.source_style,rr.categorical_grade,
	coalesce((SELECT jsonb_agg(jsonb_build_object('page_id',l.page_id,'document_block_id',l.document_block_id,'start_character',l.start_character,'end_character',l.end_character,'exact_text',l.exact_text) ORDER BY l.start_character) FROM rubric_remedy_locations l WHERE l.association_id=rr.id),'[]'::jsonb)
 FROM rubric_remedies rr JOIN structured_entries e ON e.id=rr.rubric_id JOIN sources s ON s.id=e.source_id
 WHERE rr.rubric_id=$1 AND rr.processing_revision_id=s.current_revision_id ORDER BY rr.created_at,rr.id`, rubricID)
	if err != nil {
		fail(w, 500, "could not read rubric associations")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, revision uuid.UUID
		var remedy *uuid.UUID
		var grade *int
		var notation, spelling, scheme, status, note, style, categorical string
		var locations []byte
		if err = rows.Scan(&id, &revision, &remedy, &notation, &spelling, &grade, &scheme, &status, &note, &style, &categorical, &locations); err != nil {
			fail(w, 500, "could not read rubric associations")
			return
		}
		out = append(out, map[string]any{"id": id, "processing_revision_id": revision, "remedy_id": remedy, "source_notation": notation, "source_remedy_spelling": spelling, "grade": grade, "grade_scheme": scheme, "source_style": style, "categorical_grade": categorical, "review_status": status, "validation_note": note, "locations": json.RawMessage(locations)})
	}
	if rows.Err() != nil {
		fail(w, 500, "could not read rubric associations")
		return
	}
	write(w, 200, out)
}
