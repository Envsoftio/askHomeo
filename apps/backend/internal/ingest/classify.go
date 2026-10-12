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
	evidenceSuggestion := classify.SuggestEvidence(content)
	evidence, _ := json.Marshal(suggestion.Evidence)
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO literature_category_suggestions(processing_revision_id,source_id,source_asset_id,classifier_version,categories,state,reason,evidence,evidence_category,evidence_state,evidence_reason,evidence_classifier_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12)
 ON CONFLICT(processing_revision_id) DO UPDATE SET classifier_version=EXCLUDED.classifier_version,categories=EXCLUDED.categories,state=EXCLUDED.state,reason=EXCLUDED.reason,evidence=EXCLUDED.evidence,evidence_category=EXCLUDED.evidence_category,evidence_state=EXCLUDED.evidence_state,evidence_reason=EXCLUDED.evidence_reason,evidence_classifier_version=EXCLUDED.evidence_classifier_version`, revisionID, sourceID, assetID, classify.Version, suggestion.Categories, suggestion.State, suggestion.Reason, string(evidence), evidenceSuggestion.Category, evidenceSuggestion.State, evidenceSuggestion.Reason, classify.EvidenceVersion)
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
	if evidenceSuggestion.State == "suggested" {
		// Evidence review is independent of literature review. A previous
		// administrator decision, including explicit unknown, stays in force.
		_, err = tx.Exec(ctx, `WITH previous AS (
   SELECT id,literature_categories,evidence_category FROM sources
   WHERE id=$1 AND current_revision_id=$2 AND status='review' AND evidence_category_origin='fallback' FOR UPDATE
  ), changed AS (
   UPDATE sources s SET evidence_category=$3,evidence_category_origin='automatic',updated_at=now()
   FROM previous p WHERE s.id=p.id RETURNING s.id
  ) INSERT INTO literature_category_decisions(id,source_id,processing_revision_id,previous_categories,chosen_categories,previous_evidence_category,chosen_evidence_category,origin,rationale)
   SELECT $4,p.id,$2,p.literature_categories,p.literature_categories,p.evidence_category,$3,'detected',$5 FROM previous p JOIN changed c ON c.id=p.id`, sourceID, revisionID, evidenceSuggestion.Category, uuid.New(), classify.EvidenceVersion+": "+evidenceSuggestion.Reason)
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
