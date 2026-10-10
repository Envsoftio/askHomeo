package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Manual rubric recognition works against reviewed PDF pages and HTML/TXT
// blocks. It deliberately does not infer hierarchy, aliases or grades from OCR.
func (a *API) approveRepertoryEntry(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	sid, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		RevisionID uuid.UUID                 `json:"revision_id"`
		ParentID   *uuid.UUID                `json:"parent_id"`
		Heading    string                    `json:"heading"`
		Rationale  string                    `json:"rationale"`
		Locations  []structuredLocationInput `json:"locations"`
		Remedies   []struct {
			RemedyID       *uuid.UUID `json:"remedy_id"`
			CanonicalName  string     `json:"canonical_name"`
			PreparationKey string     `json:"preparation_key"`
			Notation       string     `json:"source_notation"`
			Grade          *int       `json:"grade"`
			Scheme         string     `json:"grade_scheme"`
		} `json:"remedies"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&b) != nil || b.RevisionID == uuid.Nil || strings.TrimSpace(b.Heading) == "" || len(b.Heading) > 500 || strings.TrimSpace(b.Rationale) == "" || len(b.Rationale) > 4000 || len(b.Locations) == 0 || len(b.Locations) > 200 || len(b.Remedies) > 200 {
		fail(w, 400, "provide the rubric heading, revision, reviewed selections and review reason")
		return
	}
	b.Heading = strings.TrimSpace(b.Heading)
	supports := func(text string) bool {
		for _, l := range b.Locations {
			if strings.Contains(l.ExactText, text) {
				return true
			}
		}
		return false
	}
	if !supports(b.Heading) {
		fail(w, 400, "include the exact rubric heading in the selected text")
		return
	}
	for i := range b.Remedies {
		m := &b.Remedies[i]
		m.Notation = strings.TrimSpace(m.Notation)
		m.Scheme = strings.TrimSpace(m.Scheme)
		if m.Notation == "" || len(m.Notation) > 300 || !supports(m.Notation) || (m.RemedyID == nil && (strings.TrimSpace(m.CanonicalName) == "" || strings.TrimSpace(m.PreparationKey) == "" || len(m.CanonicalName) > 300 || len(m.PreparationKey) > 300)) {
			fail(w, 400, "each membership needs exact source notation and a verified remedy/preparation identity")
			return
		}
		if m.Grade != nil && (*m.Grade < 1 || *m.Grade > 32767 || m.Scheme == "" || len(m.Scheme) > 2000 || !supports(m.Scheme)) {
			fail(w, 400, "known numeric grades require the exact source convention in the selected evidence; otherwise leave the grade unknown")
			return
		}
	}
	ctx := r.Context()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, "could not start repertory review")
		return
	}
	defer tx.Rollback(ctx)
	var rev, asset uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT current_revision_id,primary_asset_id FROM sources WHERE id=$1 AND status='review' AND removed_at IS NULL FOR UPDATE`, sid).Scan(&rev, &asset); err != nil || rev != b.RevisionID {
		fail(w, 409, "source changed or is published; reload or create a review candidate")
		return
	}
	path := []string{}
	if b.ParentID != nil {
		if err = tx.QueryRow(ctx, `SELECT full_path FROM structured_entries WHERE id=$1 AND source_id=$2 AND processing_revision_id=$3 AND kind='repertory_rubric' AND review_status IN ('accepted','corrected')`, b.ParentID, sid, rev).Scan(&path); err != nil || len(path) >= 100 {
			fail(w, 409, "choose a verified parent rubric from this source revision")
			return
		}
	}
	path = append(path, b.Heading)
	for i, l := range b.Locations {
		if (l.PageID == nil) == (l.DocumentBlockID == nil) || l.Start < 0 || l.End <= l.Start || len([]rune(l.ExactText)) != l.End-l.Start {
			fail(w, 400, "invalid supporting text selection")
			return
		}
		var current string
		err = tx.QueryRow(ctx, `SELECT text_raw FROM pages WHERE id=$1 AND source_id=$3 AND processing_revision_id=$4 AND page_kind='text' AND text_qa_status IN ('accepted','passed')
 UNION ALL SELECT reviewed_text FROM document_blocks WHERE id=$2 AND source_id=$3 AND processing_revision_id=$4 AND review_status IN ('accepted','corrected')`, l.PageID, l.DocumentBlockID, sid, rev).Scan(&current)
		if err != nil || l.TextSHA256 == "" || mmTextHash(current) != l.TextSHA256 {
			fail(w, 409, "selected text changed or is not reviewed; reload and select it again")
			return
		}
		for _, prior := range b.Locations[:i] {
			if sameMMLocation(prior, l) && prior.Start < l.End && l.Start < prior.End {
				fail(w, 400, "supporting selections overlap")
				return
			}
		}
	}
	var duplicate bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM structured_entries WHERE source_id=$1 AND processing_revision_id=$2 AND kind='repertory_rubric' AND full_path=$3 AND review_status IN ('pending','accepted','corrected'))`, sid, rev, path).Scan(&duplicate); err != nil {
		fail(w, 500, "could not check rubric identity")
		return
	}
	if duplicate {
		fail(w, 409, "this rubric path already exists; revoke the draft mapping before replacing it")
		return
	}
	eid := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO structured_entries(id,source_id,source_asset_id,processing_revision_id,kind,parent_id,ordinal,heading,full_path,adapter_version)
 SELECT $1,$2,$3,$4,'repertory_rubric',$5,coalesce(max(ordinal)+1,0),$6,$7,'manual-repertory-v1' FROM structured_entries WHERE processing_revision_id=$4 AND kind='repertory_rubric'`, eid, sid, asset, rev, b.ParentID, b.Heading, path)
	if err != nil {
		fail(w, 409, "could not create rubric in this revision")
		return
	}
	for _, l := range b.Locations {
		_, err = tx.Exec(ctx, `INSERT INTO structured_entry_locations(entry_id,source_id,processing_revision_id,page_id,document_block_id,start_character,end_character,exact_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, eid, sid, rev, l.PageID, l.DocumentBlockID, l.Start, l.End, l.ExactText)
		if err != nil {
			fail(w, 409, "support does not match reviewed source text")
			return
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE structured_entries SET review_status='accepted',validation_note=$2 WHERE id=$1`, eid, b.Rationale); err != nil {
		fail(w, 409, "rubric could not be verified")
		return
	}
	if _, err = tx.Exec(ctx, `INSERT INTO structured_review_decisions(id,source_id,processing_revision_id,entry_id,actor_principal_id,decision,rationale) VALUES($1,$2,$3,$4,$5,'accepted',$6)`, uuid.New(), sid, rev, eid, requestPrincipal(ctx).ID, b.Rationale); err != nil {
		fail(w, 500, "could not record rubric review")
		return
	}
	for _, m := range b.Remedies {
		var rid uuid.UUID
		if m.RemedyID != nil {
			err = tx.QueryRow(ctx, `SELECT id FROM remedies WHERE id=$1`, m.RemedyID).Scan(&rid)
		} else {
			err = tx.QueryRow(ctx, `INSERT INTO remedies(id,canonical_name,preparation_key) VALUES($1,$2,$3) ON CONFLICT(canonical_name,preparation_key) DO UPDATE SET canonical_name=EXCLUDED.canonical_name RETURNING id`, uuid.New(), strings.TrimSpace(m.CanonicalName), strings.TrimSpace(m.PreparationKey)).Scan(&rid)
		}
		if err != nil {
			fail(w, 400, "invalid remedy identity")
			return
		}
		aid := uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO rubric_remedies(id,rubric_id,source_id,processing_revision_id,remedy_id,source_notation,source_remedy_spelling,grade,grade_scheme) VALUES($1,$2,$3,$4,$5,$6,$6,$7,$8)`, aid, eid, sid, rev, rid, m.Notation, m.Grade, m.Scheme)
		if err != nil {
			fail(w, 409, "duplicate or invalid membership")
			return
		}
		for _, l := range b.Locations {
			_, err = tx.Exec(ctx, `INSERT INTO rubric_remedy_locations(association_id,source_id,processing_revision_id,page_id,document_block_id,start_character,end_character,exact_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, aid, sid, rev, l.PageID, l.DocumentBlockID, l.Start, l.End, l.ExactText)
			if err != nil {
				fail(w, 409, "membership evidence changed")
				return
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE rubric_remedies SET review_status='accepted',validation_note=$2 WHERE id=$1`, aid, b.Rationale); err != nil {
			fail(w, 409, "membership could not be verified")
			return
		}
		if _, err = tx.Exec(ctx, `INSERT INTO structured_review_decisions(id,source_id,processing_revision_id,rubric_remedy_id,actor_principal_id,decision,rationale) VALUES($1,$2,$3,$4,$5,'accepted',$6)`, uuid.New(), sid, rev, aid, requestPrincipal(ctx).ID, b.Rationale); err != nil {
			fail(w, 500, "could not record membership review")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, "could not commit rubric review")
		return
	}
	write(w, 201, map[string]any{"id": eid, "review_status": "accepted", "full_path": path})
}
