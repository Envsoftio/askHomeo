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
	_, _ = w.Store.DB.Exec(ctx, `INSERT INTO literature_category_suggestions(processing_revision_id,source_id,source_asset_id,classifier_version,categories,state,reason,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb)
 ON CONFLICT(processing_revision_id) DO UPDATE SET classifier_version=EXCLUDED.classifier_version,categories=EXCLUDED.categories,state=EXCLUDED.state,reason=EXCLUDED.reason,evidence=EXCLUDED.evidence`, revisionID, sourceID, assetID, classify.Version, suggestion.Categories, suggestion.State, suggestion.Reason, string(evidence))
}

func (w *Worker) suggestPDFCategories(ctx context.Context, sourceID uuid.UUID) {
	var revisionID, assetID uuid.UUID
	var content string
	err := w.Store.DB.QueryRow(ctx, `SELECT s.current_revision_id,s.primary_asset_id,coalesce((SELECT string_agg(p.text_raw,E'\n' ORDER BY p.pdf_page_index) FROM (SELECT text_raw,pdf_page_index FROM pages WHERE source_id=s.id AND page_kind='text' ORDER BY pdf_page_index LIMIT 30) p),'') FROM sources s WHERE s.id=$1`, sourceID).Scan(&revisionID, &assetID, &content)
	if err == nil {
		w.suggestCategories(ctx, sourceID, revisionID, assetID, strings.TrimSpace(content))
	}
}
