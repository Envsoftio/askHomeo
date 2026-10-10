package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/pdfocr"
)

// retryPageOCR produces a draft. An administrator must check it against the scan
// before saving; an OCR retry alone cannot approve evidence.
func (a *API) retryPageOCR(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var sha string
	var index, revision int
	err := a.Store.DB.QueryRow(r.Context(), `SELECT s.pdf_sha256,p.pdf_page_index,p.text_revision FROM pages p JOIN sources s ON s.id=p.source_id WHERE p.id=$1 AND s.status='review' AND s.removed_at IS NULL AND p.scan_page_index>=0`, id).Scan(&sha, &index, &revision)
	if err != nil {
		fail(w, 409, "Page is unavailable for repair.")
		return
	}
	pdf, release, err := a.Store.AcquirePDF(r.Context(), sha)
	if err != nil {
		fail(w, 503, "PDF storage unavailable.")
		return
	}
	defer release()
	text, warning, err := pdfocr.Check(r.Context(), pdf, index)
	if err != nil {
		fail(w, 422, "OCR could not read this scan. Try again or enter the text manually.")
		return
	}
	if strings.TrimSpace(text) == "" {
		fail(w, 422, "OCR found no text. Check the scan and enter readable text manually, or classify it as blank or illustrated.")
		return
	}
	write(w, 200, map[string]any{"text": text, "warning": warning, "text_revision": revision})
}

func (a *API) correctPageText(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		Text     string `json:"text"`
		Note     string `json:"note"`
		Revision *int   `json:"text_revision"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body) != nil || body.Revision == nil || *body.Revision < 0 || strings.TrimSpace(body.Text) == "" || len(body.Text) > 1<<20 || !utf8.ValidString(body.Text) || strings.ContainsRune(body.Text, 0) || strings.TrimSpace(body.Note) == "" || len(body.Note) > 4000 {
		fail(w, 400, "Provide nonempty corrected text, its revision and a review note.")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not save correction.")
		return
	}
	defer tx.Rollback(r.Context())
	var sourceID, revisionID uuid.UUID
	var previous, kind string
	var revision int
	// Lock source as publication also locks it, then protect the page against concurrent edits.
	err = tx.QueryRow(r.Context(), `SELECT s.id FROM sources s JOIN pages p ON p.source_id=s.id WHERE p.id=$1 AND s.status='review' AND s.removed_at IS NULL FOR UPDATE OF s`, id).Scan(&sourceID)
	if err != nil {
		fail(w, 409, "Only unpublished pages in review can be repaired.")
		return
	}
	err = tx.QueryRow(r.Context(), `SELECT text_raw,page_kind,processing_revision_id,text_revision FROM pages WHERE id=$1 AND scan_page_index>=0 FOR UPDATE`, id).Scan(&previous, &kind, &revisionID, &revision)
	if err != nil || revision != *body.Revision {
		fail(w, 409, "This page changed. Reload it before saving your correction.")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO page_text_repairs(id,page_id,processing_revision_id,actor_principal_id,revision,previous_text,corrected_text,rationale) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, uuid.New(), id, revisionID, requestPrincipal(r.Context()).ID, revision+1, previous, body.Text, strings.TrimSpace(body.Note))
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM chunks WHERE page_id=$1`, id)
	}
	digest := sha256.Sum256([]byte(body.Text))
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE pages SET text_raw=$2,text_sha256=$3,text_revision=text_revision+1,extraction_method='Admin checked transcription',page_kind='text',review_status='reviewed',review_note=$4,text_qa_status='accepted',text_qa_at=now(),text_qa_reason='Administrator corrected and checked text against the original scan',triage_reason='' WHERE id=$1`, id, body.Text, hex.EncodeToString(digest[:]), strings.TrimSpace(body.Note))
	}
	if err == nil {
		runes := []rune(body.Text)
		index := 0
		for start := 0; start < len(runes); {
			for start < len(runes) && unicode.IsSpace(runes[start]) {
				start++
			}
			if start == len(runes) {
				break
			}
			end := start + 700
			if end >= len(runes) {
				end = len(runes)
			} else {
				for end > start+350 && !unicode.IsSpace(runes[end-1]) {
					end--
				}
			}
			for end > start && unicode.IsSpace(runes[end-1]) {
				end--
			}
			_, err = tx.Exec(r.Context(), `INSERT INTO chunks(id,page_id,source_id,chunk_index,text_exact,start_character,end_character) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), id, sourceID, index, string(runes[start:end]), start, end)
			if err != nil {
				break
			}
			start = end
			index++
		}
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO page_review_decisions(id,page_id,processing_revision_id,actor_principal_id,previous_kind,decision_kind,rationale) VALUES($1,$2,$3,$4,$5,'text',$6)`, uuid.New(), id, revisionID, requestPrincipal(r.Context()).ID, kind, strings.TrimSpace(body.Note))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "Could not save correction; no text was changed.")
		return
	}
	write(w, 200, map[string]any{"text_revision": revision + 1, "status": "accepted"})
}
