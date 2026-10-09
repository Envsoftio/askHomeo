package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Store structured diagnostics without prompts, tokens, or unbounded model output.
func (a *API) logAnswerStage(ctx context.Context, id uuid.UUID, stage string, detail map[string]any, duration *time.Duration) {
	if id == uuid.Nil {
		return
	}
	data, err := json.Marshal(detail)
	if err != nil {
		log.Printf("answer evaluation encode stage %s: %v", stage, err)
		return
	}
	var durationMS any
	if duration != nil {
		durationMS = duration.Milliseconds()
	}
	_, err = a.Store.DB.Exec(ctx, `INSERT INTO answer_job_logs(answer_job_id,attempt,stage,detail,duration_ms) SELECT id,attempts,$2,$3::jsonb,$4 FROM answer_jobs WHERE id=$1`, id, stage, string(data), durationMS)
	if err != nil {
		log.Printf("answer evaluation save stage %s for job %s: %v", stage, id, err)
	}
}

func (a *API) logAnswerStageFromContext(ctx context.Context, stage string, detail map[string]any, duration *time.Duration) {
	if id := jobIDFromContext(ctx); id != nil {
		a.logAnswerStage(ctx, *id, stage, detail, duration)
	}
}

func (a *API) setAnswerJobStage(ctx context.Context, stage string) {
	if id := jobIDFromContext(ctx); id != nil {
		_, _ = a.Store.DB.Exec(ctx, `UPDATE answer_jobs SET stage=$2,updated_at=now() WHERE id=$1 AND status='working'`, *id, stage)
	}
}

func (a *API) answerEvaluation(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var question, rawQuestion, mode, jobStatus, stage, owner, answer, answerStatus, answerModel, answerRevision, promptRevision, embeddingModel, embeddingRevision string
	var answerID *uuid.UUID
	var attempts int
	var createdAt time.Time
	var finishedAt *time.Time
	var selectedSources []uuid.UUID
	err := a.Store.DB.QueryRow(r.Context(), `SELECT j.question,j.question_raw,j.research_mode,j.status,j.stage,j.attempts,p.display_name,j.source_ids,j.answer_id,j.created_at,j.finished_at,coalesce(a.answer_text,''),coalesce(a.status,''),coalesce(a.answer_model,''),coalesce(a.answer_revision,''),coalesce(a.prompt_revision,''),coalesce(ec.model_id,''),coalesce(ec.model_revision,'')
		FROM answer_jobs j JOIN app_principals p ON p.id=j.owner_principal_id LEFT JOIN answers a ON a.id=j.answer_id LEFT JOIN index_runs ir ON ir.id=a.index_run_id LEFT JOIN embedding_configs ec ON ec.id=ir.embedding_config_id WHERE j.id=$1`, id).Scan(&question, &rawQuestion, &mode, &jobStatus, &stage, &attempts, &owner, &selectedSources, &answerID, &createdAt, &finishedAt, &answer, &answerStatus, &answerModel, &answerRevision, &promptRevision, &embeddingModel, &embeddingRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Question job not found.")
		return
	}
	if err != nil {
		fail(w, 500, "Could not load evaluation report.")
		return
	}
	metrics := map[string]any{}
	searches := []map[string]any{}
	candidates := []map[string]any{}
	claims := []map[string]any{}
	if answerID != nil {
		var searchCount, retrievedCount, candidateCount, citedCount, supportedCount, rejectedCount int
		var topScore, meanScore float64
		err = a.Store.DB.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM answer_searches WHERE answer_id=$1),(SELECT coalesce(sum(candidate_count),0) FROM answer_searches WHERE answer_id=$1),(SELECT count(*) FROM answer_candidates WHERE answer_id=$1),(SELECT count(DISTINCT chunk_id) FROM answer_citations WHERE answer_id=$1),(SELECT count(*) FROM answer_claims WHERE answer_id=$1 AND decision='supported'),(SELECT count(*) FROM answer_claims WHERE answer_id=$1 AND decision<>'supported'),(SELECT coalesce(max(retrieval_score),0) FROM answer_candidates WHERE answer_id=$1),(SELECT coalesce(avg(retrieval_score),0) FROM answer_candidates WHERE answer_id=$1)`, answerID).Scan(&searchCount, &retrievedCount, &candidateCount, &citedCount, &supportedCount, &rejectedCount, &topScore, &meanScore)
		if err != nil {
			fail(w, 500, "Could not load evaluation metrics.")
			return
		}
		metrics = map[string]any{"search_count": searchCount, "retrieved_passages_across_searches": retrievedCount, "selected_candidate_count": candidateCount, "cited_passage_count": citedCount, "citation_coverage": 0.0, "supported_claim_count": supportedCount, "rejected_claim_count": rejectedCount, "top_retrieval_score": topScore, "mean_retrieval_score": meanScore}
		if candidateCount > 0 {
			metrics["citation_coverage"] = float64(citedCount) / float64(candidateCount)
		}
		searchRows, queryErr := a.Store.DB.Query(r.Context(), `SELECT ordinal,query,source_scope,candidate_count FROM answer_searches WHERE answer_id=$1 ORDER BY ordinal`, answerID)
		if queryErr != nil {
			fail(w, 500, "Could not load search trace.")
			return
		}
		for searchRows.Next() {
			var ordinal, count int
			var query, scope string
			if err = searchRows.Scan(&ordinal, &query, &scope, &count); err != nil {
				break
			}
			searches = append(searches, map[string]any{"ordinal": ordinal, "query": query, "scope": scope, "candidate_count": count})
		}
		if err == nil {
			err = searchRows.Err()
		}
		searchRows.Close()
		if err != nil {
			fail(w, 500, "Could not load search trace.")
			return
		}
		candidateRows, queryErr := a.Store.DB.Query(r.Context(), `SELECT ac.ordinal,ac.retrieval_score,c.id,s.title,s.author,p.scan_page_index+1,coalesce(p.printed_label,''),left(c.text_exact,500),EXISTS(SELECT 1 FROM answer_citations cite WHERE cite.answer_id=ac.answer_id AND cite.chunk_id=ac.chunk_id) FROM answer_candidates ac JOIN chunks c ON c.id=ac.chunk_id JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id WHERE ac.answer_id=$1 ORDER BY ac.ordinal`, answerID)
		if queryErr != nil {
			fail(w, 500, "Could not load retrieved passages.")
			return
		}
		for candidateRows.Next() {
			var ordinal, scan int
			var score float64
			var chunkID uuid.UUID
			var title, author, printedPage, preview string
			var cited bool
			if err = candidateRows.Scan(&ordinal, &score, &chunkID, &title, &author, &scan, &printedPage, &preview, &cited); err != nil {
				break
			}
			candidates = append(candidates, map[string]any{"rank": ordinal, "score": score, "chunk_id": chunkID, "title": title, "author": author, "scan": scan, "printed_page": printedPage, "preview": preview, "cited": cited})
		}
		if err == nil {
			err = candidateRows.Err()
		}
		candidateRows.Close()
		if err != nil {
			fail(w, 500, "Could not load retrieved passages.")
			return
		}
		claimRows, queryErr := a.Store.DB.Query(r.Context(), `SELECT ordinal,claim_text,decision,check_method FROM answer_claims WHERE answer_id=$1 ORDER BY ordinal`, answerID)
		if queryErr != nil {
			fail(w, 500, "Could not load claim checks.")
			return
		}
		for claimRows.Next() {
			var ordinal int
			var claim, decision, method string
			if err = claimRows.Scan(&ordinal, &claim, &decision, &method); err != nil {
				break
			}
			claims = append(claims, map[string]any{"ordinal": ordinal, "claim": claim, "decision": decision, "check_method": method})
		}
		if err == nil {
			err = claimRows.Err()
		}
		claimRows.Close()
		if err != nil {
			fail(w, 500, "Could not load claim checks.")
			return
		}
	}
	logs := []map[string]any{}
	logRows, err := a.Store.DB.Query(r.Context(), `SELECT attempt,stage,detail,duration_ms,created_at FROM answer_job_logs WHERE answer_job_id=$1 ORDER BY id`, id)
	if err != nil {
		fail(w, 500, "Could not load technical log.")
		return
	}
	for logRows.Next() {
		var attempt int
		var logStage string
		var detail json.RawMessage
		var duration *int64
		var at time.Time
		if err = logRows.Scan(&attempt, &logStage, &detail, &duration, &at); err != nil {
			break
		}
		logs = append(logs, map[string]any{"attempt": attempt, "stage": logStage, "detail": detail, "duration_ms": duration, "created_at": at})
	}
	if err == nil {
		err = logRows.Err()
	}
	logRows.Close()
	if err != nil {
		fail(w, 500, "Could not load technical log.")
		return
	}
	modelCalls := []map[string]any{}
	callRows, err := a.Store.DB.Query(r.Context(), `SELECT kind,provider,requested_model,returned_model,provider_request_id,outcome,prompt_tokens,completion_tokens,total_tokens,reasoning_tokens,estimated_cost_usd::float8,cost_origin,duration_ms,created_at FROM model_calls WHERE owner_kind='answer_job' AND owner_id=$1 ORDER BY id`, id)
	if err != nil {
		fail(w, 500, "Could not load model usage.")
		return
	}
	var knownCost float64
	unknownCostCalls := 0
	for callRows.Next() {
		var kind, provider, requested, returned, requestID, outcome, origin string
		var prompt, completion, total, reasoning *int64
		var cost *float64
		var duration int64
		var at time.Time
		if err = callRows.Scan(&kind, &provider, &requested, &returned, &requestID, &outcome, &prompt, &completion, &total, &reasoning, &cost, &origin, &duration, &at); err != nil {
			break
		}
		if cost != nil {
			knownCost += *cost
		} else {
			unknownCostCalls++
		}
		modelCalls = append(modelCalls, map[string]any{"kind": kind, "provider": provider, "requested_model": requested, "returned_model": returned, "request_id": requestID, "outcome": outcome, "prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": total, "reasoning_tokens": reasoning, "estimated_cost_usd": cost, "cost_origin": origin, "duration_ms": duration, "created_at": at})
	}
	if err == nil {
		err = callRows.Err()
	}
	callRows.Close()
	if err != nil {
		fail(w, 500, "Could not load model usage.")
		return
	}
	write(w, 200, map[string]any{"job_id": id, "question": question, "question_raw": rawQuestion, "research_mode": mode, "job_status": jobStatus, "stage": stage, "attempts": attempts, "owner": owner, "selected_source_ids": selectedSources, "created_at": createdAt, "finished_at": finishedAt, "answer_id": answerID, "answer": answer, "answer_status": answerStatus, "answer_model": answerModel, "answer_revision": answerRevision, "prompt_revision": promptRevision, "embedding_model": embeddingModel, "embedding_revision": embeddingRevision, "metrics": metrics, "searches": searches, "candidates": candidates, "claims": claims, "logs": logs, "model_calls": modelCalls, "model_usage_summary": map[string]any{"call_count": len(modelCalls), "known_estimated_cost_usd": knownCost, "unknown_cost_call_count": unknownCostCalls}})
}
