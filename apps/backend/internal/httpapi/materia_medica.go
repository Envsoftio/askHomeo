package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func (a *API) mmUnits(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	var rev uuid.UUID
	if err := a.Store.DB.QueryRow(r.Context(), `SELECT current_revision_id FROM sources WHERE id=$1 AND removed_at IS NULL AND status IN ('review','published')`, id).Scan(&rev); err != nil {
		fail(w, 409, "Source is unavailable")
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT id,kind,position,label,text,ready FROM (
 SELECT p.id,'page'::text AS kind,p.pdf_page_index AS position,coalesce(p.printed_label,'') AS label,p.text_raw AS text,(p.page_kind='text' AND p.text_qa_status IN ('passed','accepted')) AS ready FROM pages p WHERE source_id=$1 AND processing_revision_id=$2 AND scan_page_index>=0
 UNION ALL SELECT b.id,'block',b.block_index,coalesce(nullif(b.heading,''),b.section_key),b.reviewed_text,b.review_status IN ('accepted','corrected') FROM document_blocks b WHERE source_id=$1 AND processing_revision_id=$2
 ) units ORDER BY position,id LIMIT 10 OFFSET $3`, id, rev, offset)
	if err != nil {
		fail(w, 500, "Could not read source text")
		return
	}
	defer rows.Close()
	units := []any{}
	for rows.Next() {
		var uid uuid.UUID
		var kind, label, text string
		var pos int
		var ready bool
		if err = rows.Scan(&uid, &kind, &pos, &label, &text, &ready); err != nil {
			fail(w, 500, "Could not read source text")
			return
		}
		suggestions := []rubricSuggestion{}
		if ready {
			suggestions = suggestRubricRows(text)
		}
		units = append(units, map[string]any{"id": uid, "kind": kind, "position": pos, "label": label, "text": text, "text_sha256": mmTextHash(text), "ready": ready, "rubric_suggestions": suggestions})
	}
	if rows.Err() != nil {
		fail(w, 500, "Could not read source text")
		return
	}
	write(w, 200, map[string]any{"revision_id": rev, "units": units, "suggestion_adapter": repertorySuggestionVersion})
}

func (a *API) listRemedies(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT id,canonical_name,preparation_key FROM remedies WHERE canonical_name ILIKE '%'||$1||'%' ORDER BY canonical_name,preparation_key LIMIT 100`, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		fail(w, 500, "Could not read remedies")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id uuid.UUID
		var name, key string
		if err = rows.Scan(&id, &name, &key); err != nil {
			fail(w, 500, "Could not read remedies")
			return
		}
		out = append(out, map[string]any{"id": id, "canonical_name": name, "preparation_key": key})
	}
	if rows.Err() != nil {
		fail(w, 500, "Could not read remedies")
		return
	}
	write(w, 200, out)
}

// approveMMEntry is a human verification operation, not automatic name matching.
// The submitted revision and exact spans prevent stale selections and invented text.
func (a *API) approveMMEntry(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	sid, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		RevisionID     uuid.UUID                 `json:"revision_id"`
		Spelling       string                    `json:"spelling"`
		RemedyID       *uuid.UUID                `json:"remedy_id"`
		CanonicalName  string                    `json:"canonical_name"`
		PreparationKey string                    `json:"preparation_key"`
		Rationale      string                    `json:"rationale"`
		Locations      []structuredLocationInput `json:"locations"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&b) != nil || b.RevisionID == uuid.Nil || strings.TrimSpace(b.Spelling) == "" || len(b.Spelling) > 300 || strings.TrimSpace(b.Rationale) == "" || len(b.Rationale) > 4000 || len(b.Locations) == 0 || len(b.Locations) > 200 {
		fail(w, 400, "Provide the source spelling, revision, exact text selections and review reason")
		return
	}
	b.Spelling = strings.TrimSpace(b.Spelling)
	if b.RemedyID == nil && (strings.TrimSpace(b.CanonicalName) == "" || strings.TrimSpace(b.PreparationKey) == "" || len(b.CanonicalName) > 300 || len(b.PreparationKey) > 300) {
		fail(w, 400, "Choose a remedy or enter its name and preparation identity")
		return
	}
	supported := false
	for _, l := range b.Locations {
		if (l.PageID == nil) == (l.DocumentBlockID == nil) || l.Start < 0 || l.End <= l.Start || len([]rune(l.ExactText)) != l.End-l.Start {
			fail(w, 400, "Invalid text selection")
			return
		}
		if strings.Contains(l.ExactText, b.Spelling) {
			supported = true
		}
	}
	if !supported {
		fail(w, 400, "Include the original remedy heading with the exact source spelling")
		return
	}
	ctx := r.Context()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, "Could not start review")
		return
	}
	defer tx.Rollback(ctx)
	var rev, asset uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT current_revision_id,primary_asset_id FROM sources WHERE id=$1 AND status='review' AND removed_at IS NULL FOR UPDATE`, sid).Scan(&rev, &asset); err != nil || rev != b.RevisionID {
		fail(w, 409, "Source changed or is published; reload or create a review candidate")
		return
	}
	// Reject overlapping accepted mappings, including two names for the same text.
	for i, l := range b.Locations {
		var current string
		err = tx.QueryRow(ctx, `SELECT text_raw FROM pages WHERE id=$1 AND source_id=$3 AND processing_revision_id=$4
 UNION ALL SELECT reviewed_text FROM document_blocks WHERE id=$2 AND source_id=$3 AND processing_revision_id=$4`, l.PageID, l.DocumentBlockID, sid, rev).Scan(&current)
		if err != nil || l.TextSHA256 == "" || mmTextHash(current) != l.TextSHA256 {
			fail(w, 409, "Selected page or block changed; refresh and select its text again")
			return
		}

		for _, prior := range b.Locations[:i] {
			if sameMMLocation(prior, l) && prior.Start < l.End && l.Start < prior.End {
				fail(w, 400, "Selections overlap")
				return
			}
		}
		var overlap bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM structured_entry_locations l JOIN structured_entries e ON e.id=l.entry_id WHERE e.source_id=$1 AND e.processing_revision_id=$2 AND e.kind='materia_medica' AND e.review_status IN ('accepted','corrected') AND (l.page_id=$3 OR l.document_block_id=$4) AND l.start_character<$6 AND $5<l.end_character)`, sid, rev, l.PageID, l.DocumentBlockID, l.Start, l.End).Scan(&overlap)
		if err != nil {
			fail(w, 500, "Could not check existing mappings")
			return
		}
		if overlap {
			fail(w, 409, "Text already belongs to an approved remedy entry; revoke that mapping first")
			return
		}
	}
	var rid uuid.UUID
	if b.RemedyID != nil {
		err = tx.QueryRow(ctx, `SELECT id FROM remedies WHERE id=$1`, b.RemedyID).Scan(&rid)
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO remedies(id,canonical_name,preparation_key) VALUES($1,$2,$3) ON CONFLICT(canonical_name,preparation_key) DO UPDATE SET canonical_name=EXCLUDED.canonical_name RETURNING id`, uuid.New(), strings.TrimSpace(b.CanonicalName), strings.TrimSpace(b.PreparationKey)).Scan(&rid)
	}
	if err != nil {
		fail(w, 400, "Invalid remedy identity")
		return
	}
	eid := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO structured_entries(id,source_id,source_asset_id,processing_revision_id,kind,ordinal,heading,full_path,source_remedy_spelling,remedy_id,adapter_version)
 SELECT $1,$2,$3,$4,'materia_medica',coalesce(max(ordinal)+1,0),$5,ARRAY[$5]::text[],$5,$6,'reviewed-continuity-v1' FROM structured_entries WHERE processing_revision_id=$4 AND kind='materia_medica'`, eid, sid, asset, rev, b.Spelling, rid)
	if err != nil {
		fail(w, 500, "Could not create entry")
		return
	}
	for _, l := range b.Locations {
		_, err = tx.Exec(ctx, `INSERT INTO structured_entry_locations(entry_id,source_id,processing_revision_id,page_id,document_block_id,start_character,end_character,exact_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, eid, sid, rev, l.PageID, l.DocumentBlockID, l.Start, l.End, l.ExactText)
		if err != nil {
			fail(w, 409, "Selected text changed, is unreviewed, or belongs to another revision")
			return
		}
	}
	_, err = tx.Exec(ctx, `UPDATE structured_entries SET review_status='accepted',validation_note=$2 WHERE id=$1`, eid, strings.TrimSpace(b.Rationale))
	if err != nil {
		fail(w, 409, "Entry could not be verified")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO structured_review_decisions(id,source_id,processing_revision_id,entry_id,actor_principal_id,decision,rationale) VALUES($1,$2,$3,$4,$5,'accepted',$6)`, uuid.New(), sid, rev, eid, requestPrincipal(ctx).ID, strings.TrimSpace(b.Rationale))
	if err != nil {
		fail(w, 500, "Could not record review")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO remedy_aliases(id,spelling,remedy_id,status,source_id,processing_revision_id,review_note,reviewed_by_principal_id) VALUES($1,$2,$3,'reviewed',$4,$5,$6,$7)`, uuid.New(), b.Spelling, rid, sid, rev, b.Rationale, requestPrincipal(ctx).ID)
	if err != nil {
		fail(w, 500, "Could not record source spelling")
		return
	}
	// Recut draft PDF passages at approved entry boundaries. HTML/TXT does
	// this at publication, when its chunks are first created.
	seenPages := map[uuid.UUID]bool{}
	for _, l := range b.Locations {
		if l.PageID == nil || seenPages[*l.PageID] {
			continue
		}
		seenPages[*l.PageID] = true
		var raw string
		if err = tx.QueryRow(ctx, `SELECT text_raw FROM pages WHERE id=$1`, l.PageID).Scan(&raw); err != nil {
			fail(w, 500, "Could not prepare entry passages")
			return
		}
		spans, e := core.EntryPassages(ctx, tx, raw, rev, l.PageID, nil)
		if e != nil {
			fail(w, 500, "Could not split entry passages")
			return
		}
		if _, err = tx.Exec(ctx, `DELETE FROM chunks WHERE page_id=$1`, l.PageID); err != nil {
			fail(w, 409, "Draft passages are already in use")
			return
		}
		for i, span := range spans {
			if _, err = tx.Exec(ctx, `INSERT INTO chunks(id,page_id,source_id,chunk_index,text_exact,start_character,end_character) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), l.PageID, sid, i, span.Text, span.Start, span.End); err != nil {
				fail(w, 500, "Could not save entry passages")
				return
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, "Could not commit review")
		return
	}
	write(w, 201, map[string]any{"id": eid, "remedy_id": rid, "review_status": "accepted"})
}
func sameMMLocation(a, b structuredLocationInput) bool {
	return a.PageID != nil && b.PageID != nil && *a.PageID == *b.PageID || a.DocumentBlockID != nil && b.DocumentBlockID != nil && *a.DocumentBlockID == *b.DocumentBlockID
}

func mmTextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
