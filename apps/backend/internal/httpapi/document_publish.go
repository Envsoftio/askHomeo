package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/passage"
)

func (a *API) publishDocument(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if a.Model == nil {
		fail(w, 503, "model configuration is missing")
		return
	}
	ctx := r.Context()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	var status, rights, title, author string
	var revisionID, assetID, rightsID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT status,rights_status,title,author,current_revision_id,primary_asset_id FROM sources WHERE id=$1 AND document_format IN ('html','txt') FOR UPDATE`, id).Scan(&status, &rights, &title, &author, &revisionID, &assetID)
	if err != nil {
		fail(w, 404, "document source not found")
		return
	}
	if status != "review" || rights != "allowed" {
		fail(w, 409, "document review and an allowed rights decision are required")
		return
	}
	if strings.HasPrefix(title, "Untitled document (verify details)") || strings.HasPrefix(author, "Unknown author (verify details)") {
		fail(w, 409, "Confirm the document title and author before publication")
		return
	}
	err = tx.QueryRow(ctx, `SELECT id FROM rights_decisions WHERE id=(SELECT rights_decision_id FROM sources WHERE id=$1) AND processing_revision_id=$2 AND source_asset_id=$3 AND decision='allowed'`, id, revisionID, assetID).Scan(&rightsID)
	if err != nil {
		fail(w, 409, "Save an allowed rights decision for this revision")
		return
	}
	var pending, eligible int
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE review_status='pending'),count(*) FILTER(WHERE review_status IN ('accepted','corrected') AND length(trim(reviewed_text))>0) FROM document_blocks WHERE processing_revision_id=$1`, revisionID).Scan(&pending, &eligible)
	if err != nil || pending > 0 || eligible == 0 {
		fail(w, 409, "Review every document block and retain at least one text block")
		return
	}
	rows, err := tx.Query(ctx, `SELECT id,section_key,reviewed_text FROM document_blocks WHERE processing_revision_id=$1 AND review_status IN ('accepted','corrected') ORDER BY block_index`, revisionID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	type block struct {
		id         uuid.UUID
		sectionKey string
		text       string
	}
	blocks := []block{}
	for rows.Next() {
		var b block
		if err = rows.Scan(&b.id, &b.sectionKey, &b.text); err != nil {
			rows.Close()
			fail(w, 500, err.Error())
			return
		}
		blocks = append(blocks, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	passages := 0
	for _, b := range blocks {
		spans, splitErr := core.EntryPassages(ctx, tx, b.text, revisionID, nil, &b.id)
		if splitErr != nil {
			fail(w, 500, "Could not split reviewed entry passages")
			return
		}
		for n, span := range spans {
			_, err = tx.Exec(ctx, `INSERT INTO chunks(id,page_id,document_block_id,source_id,chunk_index,text_exact,start_character,end_character) VALUES($1,NULL,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, uuid.New(), b.id, id, n, span.Text, span.Start, span.End)
			if err != nil {
				fail(w, 500, err.Error())
				return
			}
			_, err = tx.Exec(ctx, `INSERT INTO document_locations(id,source_id,source_asset_id,processing_revision_id,kind,section_key,start_character,end_character) VALUES($1,$2,$3,$4,'text_span',$5,$6,$7) ON CONFLICT DO NOTHING`, uuid.New(), id, assetID, revisionID, b.sectionKey, span.Start, span.End)
			if err != nil {
				fail(w, 500, err.Error())
				return
			}
			passages++
		}
	}
	if passages == 0 {
		fail(w, 409, "No usable passages remain")
		return
	}
	cfg := a.Model.Config
	var configID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO embedding_configs(id,model_id,model_revision,dimensions,preprocessing_version) VALUES($1,$2,$3,$4,'nash-v1') ON CONFLICT(model_id,model_revision,dimensions,preprocessing_version) DO UPDATE SET model_id=EXCLUDED.model_id RETURNING id`, uuid.New(), cfg.EmbeddingModel, cfg.EmbeddingRevision, cfg.Dimensions).Scan(&configID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var publicationID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO publications(id,source_id) VALUES($1,$2) ON CONFLICT(source_id) DO UPDATE SET source_id=EXCLUDED.source_id RETURNING id`, uuid.New(), id).Scan(&publicationID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(ctx, `UPDATE publications p SET processing_revision_id=s.current_revision_id,edition_id=s.edition_id,source_asset_id=s.primary_asset_id,
 metadata_snapshot=jsonb_build_object('title',s.title,'author',s.author,'edition',s.edition,'publication_info',s.publication_info,'repository',s.repository,'source_url',coalesce(s.source_url,''),'document_format',s.document_format,'literature_categories',s.literature_categories,'evidence_category',s.evidence_category),
 rights_snapshot=jsonb_build_object('decision',s.rights_status,'statement',s.rights_statement,'note',coalesce(s.review_note,''),'reviewed_at',s.reviewed_at),
 rights_decision_id=$2,published_by_role='admin',approved_by_principal_id=(SELECT reviewer_principal_id FROM rights_decisions WHERE id=$2),approved_at=(SELECT created_at FROM rights_decisions WHERE id=$2),published_by_principal_id=$3,published_at=now()
 FROM sources s WHERE p.id=$1 AND s.id=p.source_id`, publicationID, rightsID, requestPrincipal(ctx).ID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var runID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO index_runs(id,publication_id,embedding_config_id,status,expected_chunk_count) VALUES($1,$2,$3,'pending',$4) ON CONFLICT(publication_id,embedding_config_id) DO UPDATE SET expected_chunk_count=EXCLUDED.expected_chunk_count RETURNING id`, uuid.New(), publicationID, configID, passages).Scan(&runID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var jobID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO jobs(id,source_id,kind,status,current_step,total) VALUES($1,$2,'embed','queued','Preparing document passages',$3) ON CONFLICT(source_id,kind) DO UPDATE SET status='queued',total=EXCLUDED.total,attempts=0,error=NULL RETURNING id`, uuid.New(), id, passages).Scan(&jobID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO embedding_jobs(job_id,index_run_id) VALUES($1,$2) ON CONFLICT(job_id) DO UPDATE SET index_run_id=EXCLUDED.index_run_id`, jobID, runID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(ctx, `UPDATE sources SET status='published',published_revision_id=current_revision_id,updated_at=now() WHERE id=$1`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	_, err = tx.Exec(ctx, `UPDATE processing_revisions SET status='published' WHERE id=$1`, revisionID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]string{"status": "published"})
}

type documentSpan struct {
	start, end int
	text       string
}

func documentSpans(text string, maxRunes int) []documentSpan {
	out := []documentSpan{}
	for _, span := range passage.Split(text, maxRunes) {
		out = append(out, documentSpan{span.Start, span.End, span.Text})
	}
	return out
}
