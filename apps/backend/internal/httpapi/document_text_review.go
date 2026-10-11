package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// A single plain editor uses the same exact-substring rules as linked-book
// review. The saved original blocks and byte offsets remain immutable.
func (a *API) documentReviewText(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	revision, blocks, err := a.readDocumentReview(r, id)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	text := renderDocumentReview(blocks)
	if len(text) > maxCollectionReviewBytes {
		fail(w, 409, "document text exceeds the editor limit")
		return
	}
	pending := 0
	for _, block := range blocks {
		if block.status == "pending" {
			pending++
		}
	}
	write(w, 200, map[string]any{"revision_id": revision, "review_hash": documentReviewHash(blocks), "plain_text": text, "blocks": len(blocks), "pending_blocks": pending})
}

func documentReviewHash(blocks []collectionReviewBlock) string {
	h := sha256.New()
	for _, block := range blocks {
		h.Write([]byte(block.id.String()))
		h.Write([]byte{0})
		h.Write([]byte(block.status))
		h.Write([]byte{0})
		h.Write([]byte(block.reviewed))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (a *API) readDocumentReview(r *http.Request, id uuid.UUID) (uuid.UUID, []collectionReviewBlock, error) {
	query := `SELECT current_revision_id FROM sources WHERE id=$1 AND document_format IN ('html','txt') AND status='review' AND removed_at IS NULL`
	var revision uuid.UUID
	if err := a.Store.DB.QueryRow(r.Context(), query, id).Scan(&revision); err != nil {
		return uuid.Nil, nil, errors.New("document is not available for text review")
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT id,block_index,original_text,reviewed_text,review_status FROM document_blocks WHERE processing_revision_id=$1 ORDER BY block_index`, revision)
	if err != nil {
		return uuid.Nil, nil, err
	}
	defer rows.Close()
	blocks := []collectionReviewBlock{}
	for rows.Next() {
		var block collectionReviewBlock
		if err = rows.Scan(&block.id, &block.index, &block.original, &block.reviewed, &block.status); err != nil {
			return uuid.Nil, nil, err
		}
		blocks = append(blocks, block)
	}
	if err = rows.Err(); err != nil {
		return uuid.Nil, nil, err
	}
	if len(blocks) == 0 {
		return uuid.Nil, nil, errors.New("document extraction has not produced reviewable text")
	}
	return revision, blocks, nil
}

func renderDocumentReview(blocks []collectionReviewBlock) string {
	var out strings.Builder
	for _, block := range blocks {
		if block.status != "excluded" {
			out.WriteString(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(block.reviewed, "\r\n", "\n"), "\r", "\n")))
			out.WriteString("\n\n")
		}
	}
	return out.String()
}

func matchDocumentText(edited string, blocks []collectionReviewBlock) ([]string, error) {
	normalized := make([]collectionReviewBlock, len(blocks))
	mappings := make([][]int, len(blocks))
	for i, block := range blocks {
		normalized[i] = block
		normalized[i].original, mappings[i] = normalizeEditorLines(block.original)
	}
	selected, err := matchPlainPage(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(edited, "\r\n", "\n"), "\r", "\n")), normalized)
	if err != nil {
		return nil, err
	}
	for i, excerpt := range selected {
		if excerpt == "" {
			continue
		}
		start := strings.Index(normalized[i].original, excerpt)
		if start < 0 {
			return nil, errors.New("reviewed text no longer matches its original block")
		}
		selected[i] = blocks[i].original[mappings[i][start]:mappings[i][start+len(excerpt)]]
	}
	return selected, nil
}

func normalizeEditorLines(original string) (string, []int) {
	var out strings.Builder
	mapping := []int{0}
	for i := 0; i < len(original); {
		if original[i] == '\r' {
			if i+1 < len(original) && original[i+1] == '\n' {
				i++
			}
			out.WriteByte('\n')
		} else {
			out.WriteByte(original[i])
		}
		i++
		mapping = append(mapping, i)
	}
	return out.String(), mapping
}

func (a *API) saveDocumentReviewText(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		RevisionID uuid.UUID `json:"revision_id"`
		ReviewHash string    `json:"review_hash"`
		PlainText  string    `json:"plain_text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCollectionReviewBytes+4096)).Decode(&body); err != nil || body.RevisionID == uuid.Nil || len(body.ReviewHash) != 64 || len(body.PlainText) > maxCollectionReviewBytes {
		fail(w, 400, "invalid document review text")
		return
	}
	ctx := r.Context()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, "could not start document review")
		return
	}
	defer tx.Rollback(ctx)
	var revision, assetID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT current_revision_id,primary_asset_id FROM sources WHERE id=$1 AND document_format IN ('html','txt') AND status='review' AND removed_at IS NULL FOR UPDATE`, id).Scan(&revision, &assetID)
	if err != nil || revision != body.RevisionID {
		fail(w, 409, "document changed; reload the text editor")
		return
	}
	rows, err := tx.Query(ctx, `SELECT id,block_index,original_text,reviewed_text,review_status FROM document_blocks WHERE processing_revision_id=$1 ORDER BY block_index`, revision)
	if err != nil {
		fail(w, 500, "could not read document blocks")
		return
	}
	blocks := []collectionReviewBlock{}
	for rows.Next() {
		var block collectionReviewBlock
		if err = rows.Scan(&block.id, &block.index, &block.original, &block.reviewed, &block.status); err != nil {
			break
		}
		blocks = append(blocks, block)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(blocks) == 0 {
		fail(w, 500, "could not read document blocks")
		return
	}
	if body.ReviewHash != documentReviewHash(blocks) {
		fail(w, 409, "document review changed; reload the text editor")
		return
	}
	selected, err := matchDocumentText(body.PlainText, blocks)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	retained, excluded, trimmed, needsRevision := 0, 0, 0, false
	for i, block := range blocks {
		if selected[i] == "" {
			excluded++
			continue
		}
		retained++
		if selected[i] != block.original {
			trimmed++
		}
		if selected[i] != block.reviewed {
			needsRevision = true
		}
	}
	if retained == 0 {
		fail(w, 400, "retain at least one text block")
		return
	}
	var entries int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM structured_entries WHERE processing_revision_id=$1`, revision).Scan(&entries); err != nil {
		fail(w, 500, "could not check reviewed entries")
		return
	}
	if entries > 0 {
		fail(w, 409, "reviewed remedy entries already exist; complete text cleanup before mapping entries")
		return
	}
	targetRevision := revision
	if needsRevision {
		targetRevision = uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO processing_revisions(id,source_id,source_asset_id,status,component_versions_json,configuration_json,completed_at) SELECT $1,source_id,source_asset_id,'review',component_versions_json,configuration_json,now() FROM processing_revisions WHERE id=$2`, targetRevision, revision)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO document_blocks(id,source_id,source_asset_id,processing_revision_id,block_index,section_key,kind,heading,original_text,reviewed_text,start_byte,end_byte,review_status,review_note,warnings) SELECT gen_random_uuid(),source_id,source_asset_id,$1,block_index,section_key,kind,heading,original_text,reviewed_text,start_byte,end_byte,review_status,review_note,warnings FROM document_blocks WHERE processing_revision_id=$2`, targetRevision, revision)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO document_locations(id,source_id,source_asset_id,processing_revision_id,kind,section_key) SELECT gen_random_uuid(),source_id,source_asset_id,$1,'section',section_key FROM document_blocks WHERE processing_revision_id=$1 ON CONFLICT DO NOTHING`, targetRevision)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO literature_section_categories(id,source_id,processing_revision_id,document_block_id,categories,actor_principal_id,rationale) SELECT gen_random_uuid(),next.source_id,$1,next.id,sc.categories,sc.actor_principal_id,sc.rationale FROM literature_section_categories sc JOIN document_blocks old ON old.id=sc.document_block_id JOIN document_blocks next ON next.processing_revision_id=$1 AND next.block_index=old.block_index WHERE old.processing_revision_id=$2`, targetRevision, revision)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE sources SET current_revision_id=$2,rights_status='needs_review',rights_decision_id=NULL,reviewed_at=NULL,updated_at=now() WHERE id=$1`, id, targetRevision)
		}
		if err != nil {
			fail(w, 500, "could not save a corrected document revision")
			return
		}
	}
	actor := requestPrincipal(ctx).ID
	for i, block := range blocks {
		decision, note := "accepted", "Reviewed in document text editor"
		if selected[i] == "" {
			decision, note = "excluded", "Removed in document text editor"
		} else if selected[i] != block.original {
			decision, note = "corrected", "Trimmed to an exact original excerpt in document text editor"
		}
		if !needsRevision && block.status == decision && (decision != "corrected" || selected[i] == block.reviewed) && (decision != "excluded" || block.reviewed == "") {
			continue
		}
		var targetBlock uuid.UUID
		err = tx.QueryRow(ctx, `UPDATE document_blocks SET review_status=$3,review_note=$4,reviewed_text=CASE WHEN $3='excluded' THEN '' WHEN $3='corrected' THEN $5 ELSE original_text END WHERE processing_revision_id=$1 AND block_index=$2 RETURNING id`, targetRevision, block.index, decision, note, selected[i]).Scan(&targetBlock)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO document_block_decisions(id,source_id,block_id,processing_revision_id,actor_principal_id,decision,rationale) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), id, targetBlock, targetRevision, actor, decision, note)
		}
		if err != nil {
			fail(w, 500, "could not save document block review")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, "could not commit document review")
		return
	}
	write(w, 200, map[string]any{"revision_id": targetRevision, "retained_blocks": retained, "excluded_blocks": excluded, "trimmed_blocks": trimmed})
}
