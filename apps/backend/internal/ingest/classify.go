package ingest

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/classify"
)

func (w *Worker) suggestCategories(ctx context.Context, sourceID, revisionID, assetID uuid.UUID, content string) {
	suggestion := classify.Suggest(content)
	evidence, _ := json.Marshal(suggestion.Evidence)
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO literature_category_suggestions(processing_revision_id,source_id,source_asset_id,classifier_version,categories,state,reason,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb)
 ON CONFLICT(processing_revision_id) DO UPDATE SET classifier_version=EXCLUDED.classifier_version,categories=EXCLUDED.categories,state=EXCLUDED.state,reason=EXCLUDED.reason,evidence=EXCLUDED.evidence`, revisionID, sourceID, assetID, classify.Version, suggestion.Categories, suggestion.State, suggestion.Reason, string(evidence))
	if err != nil {
		return
	}
	if suggestion.State == "suggested" {
		// Prefill only a never-classified current draft. Retries cannot replace a
		// manual choice, an accepted suggestion, or a published classification.
		_, err = tx.Exec(ctx, `WITH previous AS (
   SELECT id,literature_categories,evidence_category FROM sources
   WHERE id=$1 AND current_revision_id=$2 AND status='review' AND literature_category_origin='fallback' FOR UPDATE
  ), changed AS (
   UPDATE sources s SET literature_categories=$3,literature_category_origin='automatic',updated_at=now()
   FROM previous p WHERE s.id=p.id RETURNING s.id
  ) INSERT INTO literature_category_decisions(id,source_id,processing_revision_id,previous_categories,chosen_categories,previous_evidence_category,chosen_evidence_category,origin,rationale)
   SELECT $4,p.id,$2,p.literature_categories,$3,p.evidence_category,p.evidence_category,'detected',$5 FROM previous p JOIN changed c ON c.id=p.id`, sourceID, revisionID, suggestion.Categories, uuid.New(), classify.Version+": "+suggestion.Reason)
		if err != nil {
			return
		}
	}
	_ = tx.Commit(ctx)
}

func (w *Worker) suggestPDFCategories(ctx context.Context, sourceID uuid.UUID) {
	var revisionID, assetID uuid.UUID
	var content string
	err := w.Store.DB.QueryRow(ctx, `SELECT s.current_revision_id,s.primary_asset_id,coalesce((SELECT string_agg(p.text_raw,E'\n' ORDER BY p.pdf_page_index) FROM (SELECT text_raw,pdf_page_index FROM pages WHERE source_id=s.id AND page_kind='text' ORDER BY pdf_page_index LIMIT 30) p),'') FROM sources s WHERE s.id=$1`, sourceID).Scan(&revisionID, &assetID, &content)
	if err == nil {
		w.suggestCategories(ctx, sourceID, revisionID, assetID, strings.TrimSpace(content))
	}
}
