package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/classify"
)

const maxCollectionReviewBytes = 30 << 20

type collectionReviewBlock struct {
	id                         uuid.UUID
	index                      int
	original, reviewed, status string
}

type collectionReviewSource struct {
	id, revisionID uuid.UUID
	url, status    string
	blocks         []collectionReviewBlock
}

func collectionPageHeader(page int, source collectionReviewSource) string {
	if source.status == "published" {
		return fmt.Sprintf("\n===== PAGE %d | %s | PUBLISHED READ ONLY =====\n", page+1, source.url)
	}
	return fmt.Sprintf("\n===== PAGE %d | %s =====\n", page+1, source.url)
}

// The page and block markers form a fixed frame. Only the text between a pair
// of block markers may change. A blank block excludes it; a contiguous excerpt
// of its original text is a correction. This keeps every citation tied to one
// saved original and prevents an edited page from borrowing another page's URL.
func renderCollectionReview(sources []collectionReviewSource) string {
	var out strings.Builder
	out.WriteString("BOOK TEXT REVIEW — keep PAGE and BLOCK markers unchanged. Delete unwanted block text; retain only exact source wording.\n")
	for page, source := range sources {
		out.WriteString(collectionPageHeader(page, source))
		for _, block := range source.blocks {
			fmt.Fprintf(&out, "[[BLOCK %s]]\n%s\n[[END %s]]\n", block.id, block.reviewed, block.id)
		}
	}
	return out.String()
}

func parseCollectionReview(edited string, sources []collectionReviewSource) ([]string, error) {
	const preface = "BOOK TEXT REVIEW — keep PAGE and BLOCK markers unchanged. Delete unwanted block text; retain only exact source wording.\n"
	if !strings.HasPrefix(edited, preface) {
		return nil, errors.New("the review header changed; reload and keep markers unchanged")
	}
	rest := strings.TrimPrefix(edited, preface)
	texts := make([]string, 0)
	for page, source := range sources {
		header := collectionPageHeader(page, source)
		if !strings.HasPrefix(rest, header) {
			return nil, fmt.Errorf("page %d marker changed or moved", page+1)
		}
		rest = strings.TrimPrefix(rest, header)
		for _, block := range source.blocks {
			start := fmt.Sprintf("[[BLOCK %s]]\n", block.id)
			end := fmt.Sprintf("\n[[END %s]]\n", block.id)
			if !strings.HasPrefix(rest, start) {
				return nil, fmt.Errorf("block marker changed or moved on page %d", page+1)
			}
			rest = strings.TrimPrefix(rest, start)
			body, tail, ok := strings.Cut(rest, end)
			if !ok {
				return nil, fmt.Errorf("end marker missing on page %d", page+1)
			}
			texts = append(texts, strings.TrimSpace(body))
			rest = tail
		}
	}
	if rest != "" {
		return nil, errors.New("unexpected text after the last block marker")
	}
	return texts, nil
}

func (a *API) readCollectionReview(ctx context.Context, tx pgx.Tx, collectionID uuid.UUID) (uuid.UUID, []collectionReviewSource, error) {
	var snapshotID uuid.UUID
	var state string
	err := tx.QueryRow(ctx, `SELECT s.id,s.state FROM collection_snapshots s WHERE s.collection_id=$1 ORDER BY s.generation DESC LIMIT 1 FOR SHARE`, collectionID).Scan(&snapshotID, &state)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if state != "review" {
		return uuid.Nil, nil, errors.New("collection capture must finish before text review")
	}
	rows, err := tx.Query(ctx, `SELECT i.source_id,coalesce(i.final_url,i.requested_url) FROM collection_items i WHERE i.snapshot_id=$1 AND i.state='fetched' AND i.role='content_candidate' ORDER BY i.discovery_ordinal`, snapshotID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	type item struct {
		id  *uuid.UUID
		url string
	}
	items := []item{}
	for rows.Next() {
		var it item
		if err = rows.Scan(&it.id, &it.url); err != nil {
			break
		}
		items = append(items, it)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return uuid.Nil, nil, err
	}
	if len(items) == 0 {
		return uuid.Nil, nil, errors.New("collection has no content pages")
	}
	sources := make([]collectionReviewSource, 0, len(items))
	for _, it := range items {
		if it.id == nil {
			return uuid.Nil, nil, errors.New("prepare all content pages before opening book review")
		}
		var source collectionReviewSource
		source.id, source.url = *it.id, it.url
		var status string
		err = tx.QueryRow(ctx, `SELECT current_revision_id,status FROM sources WHERE id=$1 AND removed_at IS NULL AND document_format IN ('html','txt') FOR UPDATE`, source.id).Scan(&source.revisionID, &status)
		if err != nil || (status != "review" && status != "published") {
			return uuid.Nil, nil, errors.New("all content pages must finish processing or be published before book review")
		}
		source.status = status
		blocks, qerr := tx.Query(ctx, `SELECT id,block_index,original_text,reviewed_text,review_status FROM document_blocks WHERE processing_revision_id=$1 AND review_status<>'excluded' ORDER BY block_index`, source.revisionID)
		if qerr != nil {
			return uuid.Nil, nil, qerr
		}
		for blocks.Next() {
			var b collectionReviewBlock
			if err = blocks.Scan(&b.id, &b.index, &b.original, &b.reviewed, &b.status); err != nil {
				break
			}
			source.blocks = append(source.blocks, b)
		}
		if err == nil {
			err = blocks.Err()
		}
		blocks.Close()
		if err != nil {
			return uuid.Nil, nil, err
		}
		sources = append(sources, source)
	}
	return snapshotID, sources, nil
}

func (a *API) collectionReviewText(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "could not open book review")
		return
	}
	defer tx.Rollback(r.Context())
	snapshotID, sources, err := a.readCollectionReview(r.Context(), tx, id)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	result := renderCollectionReview(sources)
	if len(result) > maxCollectionReviewBytes {
		fail(w, 409, "book text exceeds the editor limit")
		return
	}
	blocks := 0
	pending := 0
	draftPages := 0
	pages := make([]map[string]any, 0, len(sources))
	for _, source := range sources {
		pageBlocks := make([]map[string]any, 0, len(source.blocks))
		if source.status == "review" {
			draftPages++
		}
		blocks += len(source.blocks)
		for _, block := range source.blocks {
			pageBlocks = append(pageBlocks, map[string]any{"id": block.id, "text": block.reviewed})
			if source.status == "review" && block.status == "pending" {
				pending++
			}
		}
		pages = append(pages, map[string]any{"url": source.url, "published": source.status == "published", "blocks": pageBlocks})
	}
	write(w, 200, map[string]any{"snapshot_id": snapshotID, "text": result, "plain_text": renderPlainCollectionReview(sources), "pages": len(sources), "review_pages": pages, "draft_pages": draftPages, "blocks": blocks, "pending_blocks": pending})
}

func (a *API) saveCollectionReviewText(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		SnapshotID uuid.UUID `json:"snapshot_id"`
		Text       string    `json:"text"`
		PlainText  *string   `json:"plain_text"`
		Edits      *[]struct {
			ID   uuid.UUID `json:"id"`
			Text string    `json:"text"`
		} `json:"edits"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*maxCollectionReviewBytes+4096)).Decode(&body); err != nil {
		fail(w, 400, "invalid book review text")
		return
	}
	if len(body.Text) > maxCollectionReviewBytes || (body.PlainText != nil && len(*body.PlainText) > maxCollectionReviewBytes) {
		fail(w, 400, "book text exceeds the editor limit")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "could not save book review")
		return
	}
	defer tx.Rollback(r.Context())
	snapshotID, sources, err := a.readCollectionReview(r.Context(), tx, id)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	if body.SnapshotID != snapshotID {
		fail(w, 409, "collection snapshot changed; reload book review")
		return
	}
	var texts []string
	if body.PlainText != nil {
		texts, err = parsePlainCollectionReview(*body.PlainText, sources)
	} else if body.Edits != nil {
		position := 0
		for _, source := range sources {
			for _, block := range source.blocks {
				if position >= len(*body.Edits) || (*body.Edits)[position].ID != block.id {
					fail(w, 400, "book page or block order changed; reload the editor")
					return
				}
				texts = append(texts, strings.TrimSpace((*body.Edits)[position].Text))
				position++
			}
		}
		if position != len(*body.Edits) {
			fail(w, 400, "book page or block count changed; reload the editor")
			return
		}
	} else {
		texts, err = parseCollectionReview(body.Text, sources)
	}
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	position, kept, excluded, corrected := 0, 0, 0, 0
	draftKept := 0
	for _, source := range sources {
		for _, block := range source.blocks {
			selected := texts[position]
			position++
			if source.status == "published" {
				if selected != strings.TrimSpace(block.reviewed) {
					fail(w, 409, "published page text is read only; reprocess it as a candidate instead")
					return
				}
				kept++
				continue
			}
			if selected == "" {
				excluded++
				continue
			}
			if !strings.Contains(block.original, selected) {
				fail(w, 400, "edited text must be a contiguous excerpt of its original block; remove whole blocks or trim their ends")
				return
			}
			kept++
			draftKept++
			if selected != block.original {
				corrected++
			}
		}
	}
	if draftKept == 0 {
		fail(w, 400, "retain at least one draft text block")
		return
	}
	position = 0
	actor := requestPrincipal(r.Context()).ID
	for _, source := range sources {
		if source.status == "published" {
			position += len(source.blocks)
			continue
		}
		needsRevision := false
		for i, block := range source.blocks {
			selected := texts[position+i]
			if selected != "" && selected != block.reviewed {
				needsRevision = true
			}
		}
		targetRevision := source.revisionID
		if needsRevision {
			targetRevision = uuid.New()
			_, err = tx.Exec(r.Context(), `INSERT INTO processing_revisions(id,source_id,source_asset_id,status,component_versions_json,configuration_json,completed_at) SELECT $1,source_id,source_asset_id,'review',component_versions_json,configuration_json,now() FROM processing_revisions WHERE id=$2`, targetRevision, source.revisionID)
			if err == nil {
				_, err = tx.Exec(r.Context(), `INSERT INTO document_blocks(id,source_id,source_asset_id,processing_revision_id,block_index,section_key,kind,heading,original_text,reviewed_text,start_byte,end_byte,review_status,review_note,warnings) SELECT gen_random_uuid(),source_id,source_asset_id,$1,block_index,section_key,kind,heading,original_text,reviewed_text,start_byte,end_byte,review_status,review_note,warnings FROM document_blocks WHERE processing_revision_id=$2`, targetRevision, source.revisionID)
			}
			if err == nil {
				_, err = tx.Exec(r.Context(), `INSERT INTO document_locations(id,source_id,source_asset_id,processing_revision_id,kind,section_key) SELECT gen_random_uuid(),source_id,source_asset_id,$1,'section',section_key FROM document_blocks WHERE processing_revision_id=$1 ON CONFLICT DO NOTHING`, targetRevision)
			}
			if err == nil {
				_, err = tx.Exec(r.Context(), `INSERT INTO literature_section_categories(id,source_id,processing_revision_id,document_block_id,categories,actor_principal_id,rationale) SELECT gen_random_uuid(),next.source_id,$1,next.id,sc.categories,sc.actor_principal_id,sc.rationale FROM literature_section_categories sc JOIN document_blocks old ON old.id=sc.document_block_id JOIN document_blocks next ON next.processing_revision_id=$1 AND next.block_index=old.block_index WHERE old.processing_revision_id=$2`, targetRevision, source.revisionID)
			}
			if err == nil {
				_, err = tx.Exec(r.Context(), `UPDATE sources SET current_revision_id=$2,rights_status='needs_review',rights_decision_id=NULL,reviewed_at=NULL,updated_at=now() WHERE id=$1`, source.id, targetRevision)
			}
			if err != nil {
				fail(w, 500, "could not create corrected source revision")
				return
			}
		}
		for _, block := range source.blocks {
			selected := texts[position]
			position++
			decision, note := "accepted", "Reviewed in collection text editor"
			if selected == "" {
				decision, note = "excluded", "Removed in collection text editor"
			} else if selected != block.original {
				decision, note = "corrected", "Trimmed to an exact source excerpt in collection text editor"
			}
			if !needsRevision && block.status == decision && (decision != "corrected" || selected == block.reviewed) {
				continue
			}
			var targetBlock uuid.UUID
			err = tx.QueryRow(r.Context(), `UPDATE document_blocks SET review_status=$3,review_note=$4,reviewed_text=CASE WHEN $3='corrected' THEN $5 ELSE original_text END WHERE processing_revision_id=$1 AND block_index=$2 RETURNING id`, targetRevision, block.index, decision, note, selected).Scan(&targetBlock)
			if err == nil {
				_, err = tx.Exec(r.Context(), `INSERT INTO document_block_decisions(id,source_id,block_id,processing_revision_id,actor_principal_id,decision,rationale) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), source.id, targetBlock, targetRevision, actor, decision, note)
			}
			if err != nil {
				fail(w, 500, "could not save block review")
				return
			}
		}
		var hasRetained bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM document_blocks WHERE processing_revision_id=$1 AND review_status IN ('accepted','corrected') AND length(trim(reviewed_text))>0)`, targetRevision).Scan(&hasRetained); err != nil {
			fail(w, 500, "could not check retained page text")
			return
		}
		var itemID uuid.UUID
		var previouslyExcluded bool
		if err = tx.QueryRow(r.Context(), `SELECT id,review_excluded FROM collection_items WHERE snapshot_id=$1 AND source_id=$2 FOR UPDATE`, snapshotID, source.id).Scan(&itemID, &previouslyExcluded); err != nil {
			fail(w, 500, "could not locate reviewed collection page")
			return
		}
		if previouslyExcluded != !hasRetained {
			rationale := "Reviewed page has retained text"
			if !hasRetained {
				rationale = "All reviewable page text was removed in the collection editor"
			}
			_, err = tx.Exec(r.Context(), `UPDATE collection_items SET review_excluded=$2 WHERE id=$1`, itemID, !hasRetained)
			if err == nil {
				_, err = tx.Exec(r.Context(), `INSERT INTO collection_item_review_decisions(id,item_id,snapshot_id,actor_principal_id,excluded,rationale) VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), itemID, snapshotID, actor, !hasRetained, rationale)
			}
			if err != nil {
				fail(w, 500, "could not record page exclusion decision")
				return
			}
		}
		if err = refreshCollectionCategorySuggestion(r.Context(), tx, source.id, targetRevision); err != nil {
			fail(w, 500, "could not refresh category suggestion")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "could not commit book review")
		return
	}
	write(w, 200, map[string]any{"snapshot_id": snapshotID, "pages": len(sources), "retained_blocks": kept, "excluded_blocks": excluded, "trimmed_blocks": corrected})
}

func refreshCollectionCategorySuggestion(ctx context.Context, tx pgx.Tx, sourceID, revisionID uuid.UUID) error {
	var assetID uuid.UUID
	var content string
	if err := tx.QueryRow(ctx, `SELECT s.primary_asset_id,coalesce((SELECT string_agg(b.heading||E'\n'||b.reviewed_text,E'\n' ORDER BY b.block_index) FROM document_blocks b WHERE b.processing_revision_id=$2 AND b.review_status IN ('accepted','corrected')),'') FROM sources s WHERE s.id=$1`, sourceID, revisionID).Scan(&assetID, &content); err != nil {
		return err
	}
	suggestion := classify.Suggest(content)
	evidence, err := json.Marshal(suggestion.Evidence)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO literature_category_suggestions(processing_revision_id,source_id,source_asset_id,classifier_version,categories,state,reason,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb) ON CONFLICT(processing_revision_id) DO UPDATE SET classifier_version=EXCLUDED.classifier_version,categories=EXCLUDED.categories,state=EXCLUDED.state,reason=EXCLUDED.reason,evidence=EXCLUDED.evidence`, revisionID, sourceID, assetID, classify.Version, suggestion.Categories, suggestion.State, suggestion.Reason, string(evidence))
	if err != nil {
		return err
	}
	var previous []string
	var previousEvidence, origin string
	if err = tx.QueryRow(ctx, `SELECT literature_categories,evidence_category,literature_category_origin FROM sources WHERE id=$1`, sourceID).Scan(&previous, &previousEvidence, &origin); err != nil {
		return err
	}
	if origin == "manual" {
		return nil
	}
	chosen, nextOrigin := suggestion.Categories, "automatic"
	if suggestion.State != "suggested" {
		chosen, nextOrigin = []string{"unclassified"}, "fallback"
	}
	if equalCategories(previous, chosen) && origin == nextOrigin {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE sources SET literature_categories=$2,literature_category_origin=$3,updated_at=now() WHERE id=$1`, sourceID, chosen, nextOrigin)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO literature_category_decisions(id,source_id,processing_revision_id,previous_categories,chosen_categories,previous_evidence_category,chosen_evidence_category,origin,rationale) VALUES($1,$2,$3,$4,$5,$6,$6,'detected',$7)`, uuid.New(), sourceID, revisionID, previous, chosen, previousEvidence, classify.Version+": refreshed after collection text review: "+suggestion.Reason)
	return err
}

// One explicit rights decision is applied to each eligible current source
// revision. Each page still receives its own immutable decision record.
func (a *API) approveCollectionRights(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		SnapshotID   uuid.UUID `json:"snapshot_id"`
		Statement    string    `json:"statement"`
		Note         string    `json:"note"`
		EvidenceURL  string    `json:"evidence_url"`
		Jurisdiction string    `json:"jurisdiction"`
		Restrictions string    `json:"restrictions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		fail(w, 400, "invalid collection rights decision")
		return
	}
	if strings.TrimSpace(body.Statement) == "" || strings.TrimSpace(body.Note) == "" {
		fail(w, 400, "rights statement and review reason are required")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "could not save collection rights")
		return
	}
	defer tx.Rollback(r.Context())
	snapshotID, sources, err := a.readCollectionReview(r.Context(), tx, id)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	if snapshotID != body.SnapshotID {
		fail(w, 409, "collection snapshot changed; reload book review")
		return
	}
	var incomplete []string
	if err = tx.QueryRow(r.Context(), `SELECT incomplete_reasons FROM collection_snapshots WHERE id=$1`, snapshotID).Scan(&incomplete); err != nil {
		fail(w, 500, "could not check collection coverage")
		return
	}
	if len(incomplete) > 0 {
		fail(w, 409, "bulk publication requires complete capture within the selected scope; resolve the listed missing pages or publish reviewed sources individually with explicit partial-coverage limits")
		return
	}
	approved, skipped := 0, 0
	eligibleIDs := []uuid.UUID{}
	for _, source := range sources {
		if source.status == "published" {
			continue
		}
		var pending, eligible int
		err = tx.QueryRow(r.Context(), `SELECT count(*) FILTER (WHERE review_status='pending'), count(*) FILTER (WHERE review_status IN ('accepted','corrected') AND length(trim(reviewed_text))>0) FROM document_blocks WHERE processing_revision_id=$1`, source.revisionID).Scan(&pending, &eligible)
		if err != nil {
			fail(w, 500, "could not check page review")
			return
		}
		if pending > 0 {
			fail(w, 409, "save book text review before approving rights")
			return
		}
		if eligible == 0 {
			skipped++
			continue
		}
		var assetID uuid.UUID
		err = tx.QueryRow(r.Context(), `SELECT primary_asset_id FROM sources WHERE id=$1`, source.id).Scan(&assetID)
		if err != nil {
			fail(w, 500, "could not check source asset")
			return
		}
		decisionID := uuid.New()
		_, err = tx.Exec(r.Context(), `INSERT INTO rights_decisions(id,source_id,processing_revision_id,source_asset_id,raw_statement,decision,reviewer_role,rationale,evidence_url,jurisdiction,restrictions,reviewer_principal_id) VALUES($1,$2,$3,$4,$5,'allowed','admin',$6,$7,$8,$9,$10)`, decisionID, source.id, source.revisionID, assetID, strings.TrimSpace(body.Statement), strings.TrimSpace(body.Note), strings.TrimSpace(body.EvidenceURL), strings.TrimSpace(body.Jurisdiction), strings.TrimSpace(body.Restrictions), requestPrincipal(r.Context()).ID)
		if err == nil {
			_, err = tx.Exec(r.Context(), `UPDATE sources SET rights_statement=$2,rights_status='allowed',rights_decision_id=$3,review_note=$4,reviewed_at=now(),updated_at=now() WHERE id=$1`, source.id, strings.TrimSpace(body.Statement), decisionID, strings.TrimSpace(body.Note))
		}
		if err != nil {
			fail(w, 500, "could not save page rights decision")
			return
		}
		approved++
		eligibleIDs = append(eligibleIDs, source.id)
	}
	if approved == 0 {
		fail(w, 409, "no retained pages are ready for rights approval")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "could not commit collection rights")
		return
	}
	write(w, 200, map[string]any{"approved_pages": approved, "empty_pages": skipped, "source_ids": eligibleIDs})
}
