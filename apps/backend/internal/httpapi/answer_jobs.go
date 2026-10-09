package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/localllm"
)

type answerJobKey struct{}

func requestRole(ctx context.Context) string { role, _ := ctx.Value(roleKey{}).(string); return role }
func answerOwner(ctx context.Context) string {
	role := requestRole(ctx)
	if role == "reviewer" {
		return role
	}
	return "admin"
}
func (a *API) canReadAnswer(ctx context.Context, id uuid.UUID) bool {
	principal := requestPrincipal(ctx)
	if principal.Role == "admin" {
		return true
	}
	if principal.Role != "reviewer" {
		return false
	}
	var allowed bool
	_ = a.Store.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM answers WHERE id=$1 AND owner_principal_id=$2)`, id, principal.ID).Scan(&allowed)
	return allowed
}

func jobIDFromContext(ctx context.Context) *uuid.UUID {
	id, ok := ctx.Value(answerJobKey{}).(uuid.UUID)
	if !ok {
		return nil
	}
	return &id
}

func (a *API) queueAnswer(w http.ResponseWriter, r *http.Request) {
	role := requestRole(r.Context())
	if role != "admin" && role != "reviewer" {
		fail(w, 401, "Authentication required.")
		return
	}
	var input struct {
		Question  string      `json:"question"`
		Mode      string      `json:"mode"`
		SourceIDs []uuid.UUID `json:"source_ids"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
		fail(w, 400, "Invalid question request.")
		return
	}
	rawQuestion := input.Question
	input.Question = strings.TrimSpace(input.Question)
	if !validResearchQuestion(input.Question) {
		fail(w, 400, "Enter a question with a word of at least three characters (up to 1000 characters).")
		return
	}
	if input.Mode == "" {
		input.Mode = "quick"
	}
	if input.Mode != "quick" && input.Mode != "deep" {
		fail(w, 400, "Research depth must be quick or deep.")
		return
	}
	if len(input.SourceIDs) > 50 {
		fail(w, 400, "Choose at most 50 sources.")
		return
	}
	if input.SourceIDs == nil {
		input.SourceIDs = []uuid.UUID{}
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range input.SourceIDs {
		if id == uuid.Nil || seen[id] {
			fail(w, 400, "Choose each valid source once.")
			return
		}
		seen[id] = true
	}
	if len(input.SourceIDs) > 0 {
		var count int
		err := a.Store.DB.QueryRow(r.Context(), `SELECT count(*) FROM sources s JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id WHERE s.id=ANY($1::uuid[]) AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready'`, input.SourceIDs).Scan(&count)
		if err != nil || count != len(input.SourceIDs) {
			fail(w, 409, "One or more selected sources are not ready. Refresh sources and try again.")
			return
		}
	}
	id := uuid.New()
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not save question.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO answer_jobs(id,question,question_raw,research_mode,source_ids,status,owner_role,owner_principal_id) VALUES($1,$2,$3,$4,$5,'waiting',$6,$7)`, id, input.Question, rawQuestion, input.Mode, input.SourceIDs, role, requestPrincipal(r.Context()).ID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO activity_events(id,answer_job_id,kind,message) VALUES($1,$2,'waiting','Question saved and waiting for research')`, uuid.New(), id)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO answer_job_logs(answer_job_id,attempt,stage,detail) VALUES($1,0,'queued',jsonb_build_object('research_mode',$2::text,'selected_source_count',$3::integer))`, id, input.Mode, len(input.SourceIDs))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		log.Printf("save question job %s: %v", id, err)
		fail(w, 500, "Could not save question.")
		return
	}
	write(w, 202, map[string]any{"job_id": id, "status": "waiting"})
}

type answerJobView struct {
	ID           uuid.UUID  `json:"id"`
	Question     string     `json:"question"`
	Mode         string     `json:"research_mode"`
	Status       string     `json:"status"`
	Stage        string     `json:"stage"`
	Attempts     int        `json:"attempts"`
	AnswerID     *uuid.UUID `json:"answer_id"`
	ErrorCode    *string    `json:"error_code"`
	ErrorMessage *string    `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type sourceJobView struct {
	ID        uuid.UUID `json:"id"`
	SourceID  uuid.UUID `json:"source_id"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Stage     string    `json:"stage"`
	Completed int       `json:"completed"`
	Total     int       `json:"total"`
	Attempts  int       `json:"attempts"`
	Error     *string   `json:"error"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (a *API) answerJob(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var j answerJobView
	principal := requestPrincipal(r.Context())
	err := a.Store.DB.QueryRow(r.Context(), `SELECT id,question,research_mode,status,stage,attempts,answer_id,error_code,error_message,created_at,updated_at FROM answer_jobs WHERE id=$1 AND ($2='admin' OR owner_principal_id=$3)`, id, principal.Role, principal.ID).Scan(&j.ID, &j.Question, &j.Mode, &j.Status, &j.Stage, &j.Attempts, &j.AnswerID, &j.ErrorCode, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Question job not found.")
		return
	}
	if err != nil {
		fail(w, 500, "Could not load question job.")
		return
	}
	write(w, 200, j)
}

func (a *API) retryAnswerJob(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not retry question.")
		return
	}
	defer tx.Rollback(r.Context())
	principal := requestPrincipal(r.Context())
	tag, err := tx.Exec(r.Context(), `UPDATE answer_jobs SET status='waiting',stage='Waiting for research',attempts=0,run_after=now(),error_code=NULL,error_message=NULL,updated_at=now() WHERE id=$1 AND status='failed' AND coalesce(error_code,'')<>'source_unavailable' AND ($2='admin' OR owner_principal_id=$3)`, id, principal.Role, principal.ID)
	if err != nil {
		fail(w, 500, "Could not retry question.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Only failed questions can be retried.")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO activity_events(id,answer_job_id,kind,message) VALUES($1,$2,'waiting','Question retry requested')`, uuid.New(), id)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO answer_job_logs(answer_job_id,attempt,stage,detail) SELECT id,0,'retry_requested','{}'::jsonb FROM answer_jobs WHERE id=$1`, id)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "Could not retry question.")
		return
	}
	write(w, 202, map[string]any{"job_id": id, "status": "waiting"})
}

func (a *API) activity(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 1000 {
		fail(w, 400, "Activity page is too large.")
		return
	}
	principal := requestPrincipal(r.Context())
	rows, err := a.Store.DB.Query(r.Context(), `SELECT id,question,research_mode,status,stage,attempts,answer_id,error_code,error_message,created_at,updated_at FROM answer_jobs WHERE $2='admin' OR owner_principal_id=$3 ORDER BY created_at DESC LIMIT 20 OFFSET $1`, (page-1)*20, principal.Role, principal.ID)
	if err != nil {
		fail(w, 500, "Could not load activity.")
		return
	}
	defer rows.Close()
	jobs := []answerJobView{}
	for rows.Next() {
		var j answerJobView
		if err = rows.Scan(&j.ID, &j.Question, &j.Mode, &j.Status, &j.Stage, &j.Attempts, &j.AnswerID, &j.ErrorCode, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt); err != nil {
			fail(w, 500, "Could not load activity.")
			return
		}
		jobs = append(jobs, j)
	}
	if rows.Err() != nil {
		fail(w, 500, "Could not load activity.")
		return
	}
	sourceRows, err := a.Store.DB.Query(r.Context(), `SELECT j.id,j.source_id,s.title,j.kind,j.status,j.current_step,j.completed,j.total,j.attempts,j.error,j.updated_at FROM jobs j JOIN sources s ON s.id=j.source_id ORDER BY j.updated_at DESC LIMIT 20 OFFSET $1`, (page-1)*20)
	if err != nil {
		fail(w, 500, "Could not load source activity.")
		return
	}
	sourceJobs := []sourceJobView{}
	for sourceRows.Next() {
		var job sourceJobView
		if err = sourceRows.Scan(&job.ID, &job.SourceID, &job.Title, &job.Kind, &job.Status, &job.Stage, &job.Completed, &job.Total, &job.Attempts, &job.Error, &job.UpdatedAt); err != nil {
			sourceRows.Close()
			fail(w, 500, "Could not load source activity.")
			return
		}
		sourceJobs = append(sourceJobs, job)
	}
	err = sourceRows.Err()
	sourceRows.Close()
	if err != nil {
		fail(w, 500, "Could not load source activity.")
		return
	}
	var unread int
	_ = a.Store.DB.QueryRow(r.Context(), `SELECT count(*) FROM activity_events e LEFT JOIN answer_jobs j ON j.id=e.answer_job_id WHERE e.kind IN ('finished','failed') AND (e.source_job_id IS NOT NULL OR $1='admin' OR j.owner_principal_id=$2) AND NOT EXISTS(SELECT 1 FROM activity_event_reads ar WHERE ar.event_id=e.id AND ar.actor_principal_id=$2)`, principal.Role, principal.ID).Scan(&unread)
	write(w, 200, map[string]any{"jobs": jobs, "source_jobs": sourceJobs, "page": page, "unread_count": unread})
}

func (a *API) readActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	principal := requestPrincipal(r.Context())
	_, err := a.Store.DB.Exec(r.Context(), `INSERT INTO activity_event_reads(event_id,actor_role,actor_principal_id) SELECT e.id,$2,$3 FROM activity_events e LEFT JOIN answer_jobs j ON j.id=e.answer_job_id WHERE (e.source_job_id=$1 OR e.answer_job_id=$1) AND (e.source_job_id IS NOT NULL OR $2='admin' OR j.owner_principal_id=$3) ON CONFLICT DO NOTHING`, id, principal.Role, principal.ID)
	if err != nil {
		fail(w, 500, "Could not mark activity read.")
		return
	}
	write(w, 200, map[string]string{"status": "read"})
}

func (a *API) answerClaims(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if !a.canReadAnswer(r.Context(), id) {
		fail(w, 404, "Answer not found.")
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT c.ordinal,c.claim_text,c.decision,c.check_method,c.verification_prompt_revision,c.verification_model,s.chunk_id,s.evidence_label,s.excerpt_start,s.excerpt_end,s.excerpt_text FROM answer_claims c LEFT JOIN answer_claim_support s ON s.answer_id=c.answer_id AND s.claim_ordinal=c.ordinal WHERE c.answer_id=$1 ORDER BY c.ordinal,s.evidence_label`, id)
	if err != nil {
		fail(w, 500, "Could not load claim checks.")
		return
	}
	defer rows.Close()
	type support struct {
		ChunkID uuid.UUID `json:"chunk_id"`
		Label   string    `json:"label"`
		Start   int       `json:"excerpt_start"`
		End     int       `json:"excerpt_end"`
		Text    string    `json:"excerpt"`
	}
	type claim struct {
		Ordinal        int       `json:"ordinal"`
		Text           string    `json:"claim"`
		Decision       string    `json:"decision"`
		Method         string    `json:"check_method"`
		PromptRevision string    `json:"verification_prompt_revision"`
		Model          string    `json:"verification_model"`
		Supports       []support `json:"supports"`
	}
	claims := []claim{}
	for rows.Next() {
		var ordinal int
		var text, decision, method, promptRevision, model string
		var chunkID *uuid.UUID
		var label, excerpt *string
		var start, end *int
		if err = rows.Scan(&ordinal, &text, &decision, &method, &promptRevision, &model, &chunkID, &label, &start, &end, &excerpt); err != nil {
			fail(w, 500, "Could not load claim checks.")
			return
		}
		if len(claims) == 0 || claims[len(claims)-1].Ordinal != ordinal {
			claims = append(claims, claim{Ordinal: ordinal, Text: text, Decision: decision, Method: method, PromptRevision: promptRevision, Model: model, Supports: []support{}})
		}
		if chunkID != nil && label != nil && start != nil && end != nil && excerpt != nil {
			claims[len(claims)-1].Supports = append(claims[len(claims)-1].Supports, support{ChunkID: *chunkID, Label: *label, Start: *start, End: *end, Text: *excerpt})
		}
	}
	if rows.Err() != nil {
		fail(w, 500, "Could not load claim checks.")
		return
	}
	write(w, 200, map[string]any{"answer_id": id, "claims": claims})
}

// RunAnswerWorker persists each transition. An expired lease is picked up after
// restart; an answer already committed for that job is reused rather than rebuilt.
func RunAnswerWorker(ctx context.Context, store *core.Store, model *localllm.Client) error {
	a := &API{Store: store, Model: model}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := a.runOneAnswerJob(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *API) runOneAnswerJob(ctx context.Context) error {
	// Exhausted jobs left working by a worker crash must become actionable.
	_, sweepErr := a.Store.DB.Exec(ctx, `WITH stale AS (UPDATE answer_jobs SET status='failed',stage='Needs your attention',error_code='worker_interrupted',error_message='The research worker stopped before the answer finished.',lease_until=NULL,updated_at=now() WHERE status='working' AND attempts>=3 AND lease_until<now() RETURNING id) INSERT INTO activity_events(id,answer_job_id,kind,message) SELECT gen_random_uuid(),id,'failed','The research worker stopped before the answer finished.' FROM stale`)
	if sweepErr != nil {
		return fmt.Errorf("reconcile answer jobs: %w", sweepErr)
	}
	var id uuid.UUID
	var ownerPrincipalID uuid.UUID
	var question, mode, ownerRole string
	var sourceIDs []uuid.UUID
	err := a.Store.DB.QueryRow(ctx, `UPDATE answer_jobs SET status='working',stage='Searching and checking sources',attempts=attempts+1,lease_until=now()+interval '90 seconds',heartbeat_at=now(),updated_at=now() WHERE id=(SELECT id FROM answer_jobs WHERE run_after<=now() AND attempts<3 AND (status IN ('waiting','retrying') OR (status='working' AND lease_until<now())) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,question,research_mode,source_ids,owner_role,owner_principal_id`).Scan(&id, &question, &mode, &sourceIDs, &ownerRole, &ownerPrincipalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("claim answer job: %w", err)
	}
	_, _ = a.Store.DB.Exec(ctx, `INSERT INTO activity_events(id,answer_job_id,kind,message) VALUES($1,$2,'working','Research started')`, uuid.New(), id)
	a.logAnswerStage(ctx, id, "started", map[string]any{"research_mode": mode, "selected_source_count": len(sourceIDs)}, nil)
	var prior uuid.UUID
	if err = a.Store.DB.QueryRow(ctx, `SELECT id FROM answers WHERE answer_job_id=$1`, id).Scan(&prior); err == nil {
		return a.finishAnswerJob(ctx, id, prior)
	}
	jobCtx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				_, _ = a.Store.DB.Exec(jobCtx, `UPDATE answer_jobs SET heartbeat_at=now(),lease_until=now()+interval '90 seconds',updated_at=now() WHERE id=$1 AND status='working'`, id)
			}
		}
	}()
	input, _ := json.Marshal(map[string]any{"question": question, "mode": mode, "source_ids": sourceIDs})
	requestCtx := context.WithValue(jobCtx, answerJobKey{}, id)
	requestCtx = localllm.WithOwner(requestCtx, "answer_job", id.String())
	requestCtx = context.WithValue(requestCtx, roleKey{}, ownerRole)
	requestCtx = context.WithValue(requestCtx, principalKey{}, Principal{ID: ownerPrincipalID, Role: ownerRole})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/research/questions", bytes.NewReader(input)).WithContext(requestCtx)
	rec := httptest.NewRecorder()
	startedAt := time.Now()
	a.question(rec, req)
	close(stopHeartbeat)
	duration := time.Since(startedAt)
	if rec.Code >= 200 && rec.Code < 300 {
		var result struct {
			AnswerID uuid.UUID `json:"answer_id"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &result) == nil && result.AnswerID != uuid.Nil {
			a.logAnswerStage(ctx, id, "research_completed", map[string]any{"http_status": rec.Code, "answer_id": result.AnswerID}, &duration)
			return a.finishAnswerJob(ctx, id, result.AnswerID)
		}
	}
	var response struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &response)
	message := strings.TrimSpace(response.Error)
	if message == "" {
		message = "The answer could not be completed."
	}
	code := "answer_failed"
	retry := rec.Code >= 500 || jobCtx.Err() != nil
	if rec.Code == 503 {
		code = "model_unavailable"
		message = "The configured AI connection or model is unavailable. Check the model provider and try again."
		if strings.Contains(response.Error, "HTTP 401") || strings.Contains(response.Error, "HTTP 403") || strings.Contains(response.Error, "HTTP 400") || strings.Contains(response.Error, "HTTP 404") {
			code = "model_configuration"
			message = "The model provider rejected the configured credentials, model, or request. Check the server configuration."
			retry = false
		}
		if strings.Contains(response.Error, `finish_reason="length"`) {
			code = "model_output_limit"
			message = "The model ran out of output tokens before completing its answer. Reduce reasoning effort or increase CHAT_MAX_TOKENS."
			retry = false
		}
		if strings.Contains(strings.ToLower(response.Error), "temporarily overloaded") || strings.Contains(strings.ToLower(response.Error), "rate limit") || strings.Contains(response.Error, "HTTP 429") {
			code = "provider_busy"
			message = "The hosted model is busy or rate limited. Retrying automatically."
		}
	}
	if rec.Code == 409 {
		code = "source_unavailable"
		retry = false
	}
	if jobCtx.Err() != nil {
		code = "answer_timeout"
		message = "Answer generation timed out."
	}
	a.logAnswerStage(ctx, id, "research_failed", map[string]any{"http_status": rec.Code, "error_code": code, "message": message}, &duration)
	return a.failAnswerJob(ctx, id, code, message, retry)
}

func (a *API) finishAnswerJob(ctx context.Context, id, answerID uuid.UUID) error {
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE answer_jobs SET status='finished',stage='Answer ready',answer_id=$2,lease_until=NULL,error_code=NULL,error_message=NULL,finished_at=now(),updated_at=now() WHERE id=$1`, id, answerID)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO activity_events(id,answer_job_id,kind,message) VALUES($1,$2,'finished','Answer ready to open')`, uuid.New(), id)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *API) failAnswerJob(ctx context.Context, id uuid.UUID, code, message string, retry bool) error {
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var attempts int
	if err = tx.QueryRow(ctx, `SELECT attempts FROM answer_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&attempts); err != nil {
		return err
	}
	status, stage, kind := "failed", "Needs your attention", "failed"
	if retry && attempts < 3 {
		status, stage, kind = "retrying", "Waiting to retry", "retrying"
	}
	if status == "failed" && code == "provider_busy" {
		message = "The hosted model is busy or rate limited. Try again when it is available."
	}
	_, err = tx.Exec(ctx, `UPDATE answer_jobs SET status=$2,stage=$3,error_code=$4,error_message=$5,run_after=now()+($6::int * interval '1 minute'),lease_until=NULL,updated_at=now() WHERE id=$1`, id, status, stage, code, message, attempts*attempts)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO activity_events(id,answer_job_id,kind,message) VALUES($1,$2,$3,$4)`, uuid.New(), id, kind, stage+": "+message)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
