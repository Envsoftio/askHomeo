package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/localllm"
	"time"
)

func (w *Worker) embed(ctx context.Context, jobID, sourceID uuid.UUID) error {
	if w.Model == nil {
		return errors.New("model configuration is missing")
	}
	var runID, configID uuid.UUID
	var expected int
	var modelID, modelRevision string
	var modelDimensions int
	if e := w.Store.DB.QueryRow(ctx, `SELECT ir.id,ir.embedding_config_id,ir.expected_chunk_count,ec.model_id,ec.model_revision,ec.dimensions FROM embedding_jobs ej JOIN index_runs ir ON ir.id=ej.index_run_id JOIN embedding_configs ec ON ec.id=ir.embedding_config_id WHERE ej.job_id=$1`, jobID).Scan(&runID, &configID, &expected, &modelID, &modelRevision, &modelDimensions); e != nil {
		return e
	}
	if modelID != w.Model.Config.EmbeddingModel || modelRevision != w.Model.Config.EmbeddingRevision || modelDimensions != w.Model.Config.Dimensions {
		return errors.New("configured embedding model or revision differs from index snapshot")
	}
	models, e := w.Model.Models(ctx)
	if e != nil {
		return e
	}
	if !models[modelID] {
		return fmt.Errorf("embedding model %q is not loaded", modelID)
	}
	_, e = w.Store.DB.Exec(ctx, `UPDATE index_runs SET status='running',started_at=coalesce(started_at,now()),error='' WHERE id=$1`, runID)
	if e != nil {
		return e
	}
	batchSize := 1
	if w.Model.Config.EmbeddingProvider != "lmstudio" {
		batchSize = 16
	}
	rows, e := w.Store.DB.Query(ctx, `SELECT c.id,c.text_exact FROM chunks c JOIN pages p ON p.id=c.page_id WHERE c.source_id=$1 AND p.page_kind='text' AND p.text_qa_status IN ('passed','accepted') AND length(c.text_exact)<=2800 AND NOT EXISTS(SELECT 1 FROM chunk_embeddings ce WHERE ce.chunk_id=c.id AND ce.embedding_config_id=$2) ORDER BY p.pdf_page_index,c.chunk_index LIMIT $3`, sourceID, configID, batchSize)
	if e != nil {
		return e
	}
	var chunkIDs []uuid.UUID
	var passages []string
	for rows.Next() {
		var chunkID uuid.UUID
		var passage string
		if e = rows.Scan(&chunkID, &passage); e != nil {
			rows.Close()
			return e
		}
		chunkIDs = append(chunkIDs, chunkID)
		passages = append(passages, "search_document: "+passage)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if len(passages) > 0 {
		step, cancel := context.WithTimeout(ctx, 60*time.Second)
		vectors, err := w.Model.EmbedBatch(step, passages)
		cancel()
		if err != nil {
			return err
		}
		for i, vector := range vectors {
			h := sha256.Sum256([]byte(w.Model.EmbeddingInput(passages[i])))
			_, err = w.Store.DB.Exec(ctx, `INSERT INTO chunk_embeddings(chunk_id,embedding_config_id,embedding,input_hash) VALUES($1,$2,$3::vector,$4) ON CONFLICT(chunk_id,embedding_config_id) DO NOTHING`, chunkIDs[i], configID, localllm.Vector(vector), hex.EncodeToString(h[:]))
			if err != nil {
				return err
			}
		}
	}
	var done, actual int
	e = w.Store.DB.QueryRow(ctx, `SELECT count(*) FROM chunk_embeddings ce JOIN chunks c ON c.id=ce.chunk_id JOIN pages p ON p.id=c.page_id WHERE c.source_id=$1 AND ce.embedding_config_id=$2 AND p.page_kind='text' AND p.text_qa_status IN ('passed','accepted') AND length(c.text_exact)<=2800`, sourceID, configID).Scan(&done)
	if e != nil {
		return e
	}
	e = w.Store.DB.QueryRow(ctx, `SELECT count(*) FROM chunks c JOIN pages p ON p.id=c.page_id WHERE c.source_id=$1 AND p.page_kind='text' AND p.text_qa_status IN ('passed','accepted') AND length(c.text_exact)<=2800`, sourceID).Scan(&actual)
	if e != nil {
		return e
	}
	if actual != expected || done > expected {
		return fmt.Errorf("passage snapshot changed: expected %d, found %d", expected, actual)
	}
	_, e = w.Store.DB.Exec(ctx, `UPDATE index_runs SET indexed_chunk_count=$2 WHERE id=$1`, runID, done)
	if e != nil {
		return e
	}
	if done < expected {
		_, e = w.Store.DB.Exec(ctx, `UPDATE jobs SET completed=$2,lease_until=NULL,status='queued',attempts=0,current_step='Preparing passages',updated_at=now() WHERE id=$1`, jobID, done)
		return e
	}
	tx, e := w.Store.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE index_runs SET status='ready',indexed_chunk_count=$2,finished_at=now(),error='' WHERE id=$1`, runID, done); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO active_indexes(source_id,index_run_id) VALUES($1,$2) ON CONFLICT(source_id) DO UPDATE SET index_run_id=EXCLUDED.index_run_id`, sourceID, runID); e != nil {
		return e
	}
	if e = activateReplacement(ctx, tx, sourceID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE jobs SET status='done',completed=$2,current_step='Ready to ask',lease_until=NULL,error=NULL,updated_at=now() WHERE id=$1`, jobID, done); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func activateReplacement(ctx context.Context, tx pgx.Tx, sourceID uuid.UUID) error {
	var replaces *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT supersedes_source_id FROM sources WHERE id=$1`, sourceID).Scan(&replaces); err != nil {
		return err
	}
	if replaces == nil {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE sources SET superseded_at=now(),updated_at=now() WHERE id=$1 AND status='published' AND superseded_at IS NULL`, *replaces)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("the prior publication is no longer current")
	}
	return nil
}
