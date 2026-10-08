package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"homeopath-poc/backend/internal/localllm"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type hit struct {
	ID                                  uuid.UUID
	SourceID                            uuid.UUID
	Text, Title, Author, Page, ImageURL string
	Scan                                int
	Score                               float64
}
type searchTrace struct {
	Query          string `json:"query"`
	SourceScope    string `json:"source_scope"`
	CandidateCount int    `json:"candidate_count"`
}

var questionTermPattern = regexp.MustCompile(`[\p{L}\p{N}]{3,}`)
var quoteWordPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)
var splitOCRWordPattern = regexp.MustCompile(`-\s*\n\s*`)

const answerPromptRevision = "historical-evidence-first-v2"
const claimPromptRevision = "quote-relevance-v5-heading-quote"

func validResearchQuestion(q string) bool {
	return len(q) <= 1000 && questionTermPattern.MatchString(q)
}

func (a *API) indexStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var status, model, revision, problem, candidateID, activeID string
	var done, total, dimensions int
	err := a.Store.DB.QueryRow(r.Context(), `SELECT coalesce(ir.status,'unpublished'),coalesce(ec.model_id,''),coalesce(ec.model_revision,''),coalesce(ec.dimensions,0),coalesce(ir.error,''),coalesce(ir.indexed_chunk_count,0),coalesce(ir.expected_chunk_count,0),coalesce(ir.id::text,''),coalesce(ai.index_run_id::text,'') FROM sources s LEFT JOIN publications p ON p.source_id=s.id LEFT JOIN index_runs ir ON ir.publication_id=p.id LEFT JOIN embedding_configs ec ON ec.id=ir.embedding_config_id LEFT JOIN active_indexes ai ON ai.source_id=s.id WHERE s.id=$1 ORDER BY ir.created_at DESC LIMIT 1`, id).Scan(&status, &model, &revision, &dimensions, &problem, &done, &total, &candidateID, &activeID)
	if err != nil {
		fail(w, 404, "source not found")
		return
	}
	matchesConfig := a.Model != nil && model == a.Model.Config.EmbeddingModel && revision == a.Model.Config.EmbeddingRevision && dimensions == a.Model.Config.Dimensions
	write(w, 200, map[string]any{"status": status, "model": model, "model_revision": revision, "dimensions": dimensions, "matches_config": matchesConfig, "candidate_run_id": candidateID, "active_run_id": activeID, "completed": done, "total": total, "error": problem, "can_retry": status == "failed"})
}
func (a *API) reindex(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if a.Model == nil {
		fail(w, 503, "Model configuration is missing.")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not prepare index.")
		return
	}
	defer tx.Rollback(r.Context())
	var status, rights string
	err = tx.QueryRow(r.Context(), `SELECT status,rights_status FROM sources WHERE id=$1 FOR UPDATE`, id).Scan(&status, &rights)
	if err != nil {
		fail(w, 404, "source not found")
		return
	}
	if status != "published" || rights != "allowed" {
		fail(w, 409, "source is not published and allowed")
		return
	}
	var configID uuid.UUID
	cfg := a.Model.Config
	err = tx.QueryRow(r.Context(), `INSERT INTO embedding_configs(id,model_id,model_revision,dimensions,preprocessing_version) VALUES($1,$2,$3,$4,'nash-v1') ON CONFLICT(model_id,model_revision,dimensions,preprocessing_version) DO UPDATE SET model_id=EXCLUDED.model_id RETURNING id`, uuid.New(), cfg.EmbeddingModel, cfg.EmbeddingRevision, cfg.Dimensions).Scan(&configID)
	if err != nil {
		fail(w, 500, "Could not save model configuration.")
		return
	}
	var publicationID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO publications(id,source_id) VALUES($1,$2) ON CONFLICT(source_id) DO UPDATE SET source_id=EXCLUDED.source_id RETURNING id`, uuid.New(), id).Scan(&publicationID)
	if err != nil {
		fail(w, 500, "Could not save publication.")
		return
	}
	var total int
	err = tx.QueryRow(r.Context(), `SELECT count(*) FROM chunks c JOIN pages p ON p.id=c.page_id WHERE c.source_id=$1 AND p.page_kind='text' AND p.text_qa_status IN ('passed','accepted') AND length(c.text_exact)<=2800`, id).Scan(&total)
	if err != nil || total == 0 {
		fail(w, 409, "No eligible passages found.")
		return
	}
	var runID uuid.UUID
	var runStatus string
	err = tx.QueryRow(r.Context(), `INSERT INTO index_runs(id,publication_id,embedding_config_id,status,expected_chunk_count) VALUES($1,$2,$3,'pending',$4) ON CONFLICT(publication_id,embedding_config_id) DO UPDATE SET status=CASE WHEN index_runs.status='failed' THEN 'pending' ELSE index_runs.status END,error='',expected_chunk_count=EXCLUDED.expected_chunk_count RETURNING id,status`, uuid.New(), publicationID, configID, total).Scan(&runID, &runStatus)
	if err != nil {
		fail(w, 500, "Could not save index run.")
		return
	}
	if runStatus == "ready" || runStatus == "running" {
		fail(w, 409, "index is already ready or running")
		return
	}
	var jobID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO jobs(id,source_id,kind,status,current_step,total) VALUES($1,$2,'embed','queued','Preparing passages',$3) ON CONFLICT(source_id,kind) DO UPDATE SET status='queued',total=EXCLUDED.total,attempts=0,run_after=now(),error=NULL RETURNING id`, uuid.New(), id, total).Scan(&jobID)
	if err != nil {
		fail(w, 500, "Could not queue preparation.")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO embedding_jobs(job_id,index_run_id) VALUES($1,$2) ON CONFLICT(job_id) DO UPDATE SET index_run_id=EXCLUDED.index_run_id`, jobID, runID)
	if err != nil {
		fail(w, 500, "Could not queue preparation.")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "Could not queue preparation.")
		return
	}
	write(w, 202, map[string]string{"status": "pending"})
}
func (a *API) savedAnswer(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if !a.canReadAnswer(r.Context(), id) {
		fail(w, 404, "Answer not found.")
		return
	}
	var q, status, answer, model, revision, promptRevision, mode string
	var omittedClaims int
	var sectionsJSON []byte
	err := a.Store.DB.QueryRow(r.Context(), `SELECT question,status,answer_text,answer_model,answer_revision,prompt_revision,research_mode,omitted_claim_count,research_sections FROM answers WHERE id=$1`, id).Scan(&q, &status, &answer, &model, &revision, &promptRevision, &mode, &omittedClaims, &sectionsJSON)
	if err != nil {
		fail(w, 404, "answer not found")
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT ac.id,ac.evidence_label,s.title,s.author,coalesce(p.printed_label,''),p.scan_page_index+1 FROM answer_citations ac JOIN chunks c ON c.id=ac.chunk_id JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id WHERE ac.answer_id=$1 AND s.status='published' AND s.rights_status='allowed' ORDER BY ac.ordinal`, id)
	if err != nil {
		fail(w, 500, "could not read citations")
		return
	}
	defer rows.Close()
	citations := []any{}
	for rows.Next() {
		var cid uuid.UUID
		var label, title, author, page string
		var scan int
		if err = rows.Scan(&cid, &label, &title, &author, &page, &scan); err != nil {
			fail(w, 500, "could not read citations")
			return
		}
		citations = append(citations, map[string]any{"id": cid, "label": label, "title": title, "author": author, "printed_page": page, "scan_position": scan})
	}
	if rows.Err() != nil {
		fail(w, 500, "could not read citations")
		return
	}
	searchRows, err := a.Store.DB.Query(r.Context(), `SELECT query,source_scope,candidate_count FROM answer_searches WHERE answer_id=$1 ORDER BY ordinal`, id)
	if err != nil {
		fail(w, 500, "could not read search record")
		return
	}
	filterRows, err := a.Store.DB.Query(r.Context(), `SELECT source_id FROM answer_source_filters WHERE answer_id=$1 ORDER BY ordinal`, id)
	if err != nil {
		fail(w, 500, "could not read source selection")
		return
	}
	selectedSources := []uuid.UUID{}
	for filterRows.Next() {
		var sourceID uuid.UUID
		if err = filterRows.Scan(&sourceID); err != nil {
			filterRows.Close()
			fail(w, 500, "could not read source selection")
			return
		}
		selectedSources = append(selectedSources, sourceID)
	}
	err = filterRows.Err()
	filterRows.Close()
	if err != nil {
		fail(w, 500, "could not read source selection")
		return
	}
	searches := []searchTrace{}
	for searchRows.Next() {
		var st searchTrace
		if err = searchRows.Scan(&st.Query, &st.SourceScope, &st.CandidateCount); err != nil {
			searchRows.Close()
			fail(w, 500, "could not read search record")
			return
		}
		searches = append(searches, st)
	}
	err = searchRows.Err()
	searchRows.Close()
	if err != nil {
		fail(w, 500, "could not read search record")
		return
	}
	candidateRows, err := a.Store.DB.Query(r.Context(), `SELECT ac.ordinal,c.id,s.title,s.author,coalesce(p.printed_label,''),p.scan_page_index+1,p.image_url,left(c.text_exact,500),EXISTS(SELECT 1 FROM answer_citations cite WHERE cite.answer_id=ac.answer_id AND cite.chunk_id=ac.chunk_id) FROM answer_candidates ac JOIN chunks c ON c.id=ac.chunk_id JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id WHERE ac.answer_id=$1 AND s.status='published' AND s.rights_status='allowed' AND p.page_kind='text' ORDER BY ac.ordinal`, id)
	if err != nil {
		fail(w, 500, "could not read evidence record")
		return
	}
	evidence := []any{}
	for candidateRows.Next() {
		var chunkID uuid.UUID
		var rank, scan int
		var title, author, page, imageURL, preview string
		var cited bool
		if err = candidateRows.Scan(&rank, &chunkID, &title, &author, &page, &scan, &imageURL, &preview, &cited); err != nil {
			candidateRows.Close()
			fail(w, 500, "could not read evidence record")
			return
		}
		evidence = append(evidence, map[string]any{"rank": rank, "chunk_id": chunkID, "title": title, "author": author, "printed_page": page, "scan_position": scan, "image_url": imageURL, "preview": preview, "cited": cited})
	}
	err = candidateRows.Err()
	candidateRows.Close()
	if err != nil {
		fail(w, 500, "could not read evidence record")
		return
	}
	sections := []researchSection{}
	if err := json.Unmarshal(sectionsJSON, &sections); err != nil {
		fail(w, 500, "could not read research sections")
		return
	}
	write(w, 200, map[string]any{"answer_id": id, "question": q, "status": status, "answer": answer, "answer_model": model, "answer_revision": revision, "prompt_revision": promptRevision, "research_mode": mode, "omitted_claim_count": omittedClaims, "source_ids": selectedSources, "citations": citations, "searches": searches, "evidence": evidence, "sections": sections})
}
func (a *API) question(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Question  string      `json:"question"`
		Mode      string      `json:"mode"`
		SourceIDs []uuid.UUID `json:"source_ids"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body) != nil {
		fail(w, 400, "invalid question")
		return
	}
	q := strings.TrimSpace(body.Question)
	if !validResearchQuestion(q) {
		fail(w, 400, "Enter a question or topic with at least one word of 3 or more characters (up to 1000 characters).")
		return
	}
	mode := body.Mode
	if mode == "" {
		mode = "quick"
	}
	if mode != "quick" && mode != "deep" {
		fail(w, 400, "mode must be quick or deep")
		return
	}
	if len(body.SourceIDs) > 50 {
		fail(w, 400, "Choose at most 50 sources.")
		return
	}
	selected := make([]uuid.UUID, 0, len(body.SourceIDs))
	seenSelected := map[uuid.UUID]bool{}
	for _, id := range body.SourceIDs {
		if id == uuid.Nil || seenSelected[id] {
			fail(w, 400, "Choose each valid source only once.")
			return
		}
		seenSelected[id] = true
		selected = append(selected, id)
	}
	var runID, configID uuid.UUID
	var modelID, modelRevision string
	var modelDimensions int
	err := a.Store.DB.QueryRow(r.Context(), `SELECT ir.id,ir.embedding_config_id,ec.model_id,ec.model_revision,ec.dimensions FROM active_indexes ai JOIN index_runs ir ON ir.id=ai.index_run_id JOIN embedding_configs ec ON ec.id=ir.embedding_config_id JOIN sources s ON s.id=ai.source_id WHERE s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready' AND (cardinality($1::uuid[])=0 OR s.id=ANY($1::uuid[])) ORDER BY s.created_at LIMIT 1`, selected).Scan(&runID, &configID, &modelID, &modelRevision, &modelDimensions)
	if err != nil {
		if len(selected) > 0 {
			fail(w, 409, "One or more selected sources are not ready. Refresh the source list and choose again.")
			return
		}
		fail(w, 409, "The published book is still being prepared for questions. Check its preparation status.")
		return
	}
	if len(selected) > 0 {
		var readyCount int
		err = a.Store.DB.QueryRow(r.Context(), `SELECT count(DISTINCT s.id) FROM sources s JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id WHERE s.id=ANY($1::uuid[]) AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready' AND ir.embedding_config_id=$2`, selected, configID).Scan(&readyCount)
		if err != nil {
			fail(w, 500, "Could not check selected sources.")
			return
		}
		if readyCount != len(selected) {
			fail(w, 409, "One or more selected sources are not ready. Refresh the source list and choose again.")
			return
		}
	}
	filteredRequest := r.WithContext(context.WithValue(r.Context(), sourceFilterKey{}, selected))
	if a.answerSourceQuestion(w, filteredRequest, q, mode, runID, configID) {
		return
	}
	if a.Model == nil {
		fail(w, 503, "Models are not configured.")
		return
	}
	if modelID != a.Model.Config.EmbeddingModel || modelRevision != a.Model.Config.EmbeddingRevision || modelDimensions != a.Model.Config.Dimensions {
		fail(w, 409, "The active index uses a different embedding configuration. Reindex the source before asking questions with this embedding model.")
		return
	}
	models, err := a.Model.Models(r.Context())
	if err != nil {
		fail(w, 503, err.Error())
		return
	}
	if !models[modelID] || !models[a.Model.Config.AnswerModel] {
		fail(w, 503, "The configured embedding or answer model is unavailable.")
		return
	}
	scopes := []string{authorScope(q)}
	if len(selected) > 0 {
		scopes = []string{""}
	} else if strings.Contains(strings.ToLower(q), "nash") && strings.Contains(strings.ToLower(q), "farrington") {
		scopes = []string{"nash", "farrington"}
	}
	for _, scope := range scopes {
		if scope != "" {
			var ready bool
			err = a.Store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sources s JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id WHERE (s.source_key LIKE $1||'-%' OR s.author ILIKE '%'||$1||'%' OR s.title ILIKE '%'||$1||'%') AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready' AND ir.embedding_config_id=$2)`, scope, configID).Scan(&ready)
			if err != nil {
				fail(w, 500, "Could not check source preparation.")
				return
			}
			if !ready {
				fail(w, 409, scope+" is not prepared for questions yet. Check its source status.")
				return
			}
		}
	}
	researchStarted := time.Now()
	queries := []string{q}
	if mode == "deep" {
		rawPlan, planErr := a.Model.Chat(r.Context(), "You plan searches within a small local library of historical homeopathy books. Break the user's question into two focused searches that cover distinct aspects. Do not invent facts, sources, or answers. Return only JSON: {\"queries\":[\"search one\",\"search two\"]}.", "Question: "+q)
		if planErr != nil {
			fail(w, 503, "Local search planning failed: "+planErr.Error())
			return
		}
		extra, parseErr := parseSearchPlan(rawPlan, q)
		if parseErr != nil {
			rawPlan, planErr = a.Model.Chat(r.Context(), "Repair the malformed search plan. Return only valid JSON with exactly two distinct strings in a queries array. Each string must be a focused search of 8 to 160 characters. No markdown or commentary.", "Question: "+q+"\nPrevious output: "+rawPlan)
			if planErr != nil {
				fail(w, 503, "Local search planning failed: "+planErr.Error())
				return
			}
			extra, parseErr = parseSearchPlan(rawPlan, q)
			if parseErr != nil {
				fail(w, 502, "Local search planner returned unusable queries after retry. Try Quick answer or load a stronger local answer model.")
				return
			}
		}
		queries = append(queries, extra...)
	}
	allEvidence := map[uuid.UUID]hit{}
	searches := make([]searchTrace, 0, len(scopes)*len(queries))
	for _, query := range queries {
		vec, embedErr := a.Model.Embed(r.Context(), "search_query: "+query)
		if embedErr != nil {
			fail(w, 503, embedErr.Error())
			return
		}
		for _, scope := range scopes {
			found, searchErr := a.retrieve(r.Context(), configID, localllm.Vector(vec), query, scope, selected)
			if searchErr != nil {
				fail(w, 500, "Could not search the prepared books.")
				return
			}
			shownScope := scope
			if shownScope == "" {
				shownScope = "all"
				if len(selected) > 0 {
					shownScope = "selected"
				}
			}
			searches = append(searches, searchTrace{Query: query, SourceScope: shownScope, CandidateCount: len(found)})
			for _, h := range found {
				prior := allEvidence[h.ID]
				h.Score += prior.Score
				allEvidence[h.ID] = h
			}
		}
	}
	candidates := make([]hit, 0, len(allEvidence))
	for _, h := range allEvidence {
		candidates = append(candidates, h)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].ID.String() < candidates[j].ID.String()
		}
		return candidates[i].Score > candidates[j].Score
	})
	evidence := selectEvidence(candidates, 10)
	retrievalDuration := time.Since(researchStarted)
	a.logAnswerStageFromContext(r.Context(), "retrieval", map[string]any{"search_count": len(searches), "unique_candidate_count": len(candidates), "selected_candidate_count": len(evidence), "embedding_model": modelID, "embedding_config_id": configID}, &retrievalDuration)
	if len(evidence) == 0 {
		a.insufficientWithFilters(w, r.Context(), q, runID, mode, "I could not find support for this question in the prepared historical books. Try a question about these sources, or consult other sources for topics outside this library.", nil, selected, searches...)
		return
	}
	a.setAnswerJobStage(r.Context(), "Drafting an answer from the passages")
	var pack strings.Builder
	for i, h := range evidence {
		fmt.Fprintf(&pack, "[E%d] %s by %s, scan %d%s\n%s\n\n", i+1, h.Title, h.Author, h.Scan, func() string {
			if h.Page != "" {
				return ", printed page " + h.Page
			}
			return ""
		}(), h.Text)
	}
	var draft struct {
		Answer    string
		Citations []string
	}
	var omittedClaims int
	draftStarted := time.Now()
	draft.Answer, draft.Citations, omittedClaims, err = buildEvidenceFirstDraft(r.Context(), q, mode, pack.String(), evidence, a.Model.Chat)
	draftDuration := time.Since(draftStarted)
	draftLog := map[string]any{"model": a.Model.Config.AnswerModel, "prompt_revision": answerPromptRevision, "omitted_claim_count": omittedClaims, "error": err != nil}
	if err != nil {
		draftLog["error_message"] = err.Error()
		var invalidClaims invalidEvidenceClaimsError
		if errors.As(err, &invalidClaims) {
			output := []rune(invalidClaims.Raw)
			if len(output) > 4000 {
				output = output[:4000]
			}
			draftLog["model_output_excerpt"] = string(output)
		}
	}
	a.logAnswerStageFromContext(r.Context(), "answer_draft", draftLog, &draftDuration)
	if err != nil {
		var invalidClaims invalidEvidenceClaimsError
		if errors.As(err, &invalidClaims) {
			fail(w, 502, "Answer model returned an invalid evidence table.")
		} else {
			fail(w, 503, "Evidence planning failed: "+err.Error())
		}
		return
	}
	used, valid := validateDraft(draft.Answer, draft.Citations, evidence)
	if !valid {
		a.insufficientWithFilters(w, r.Context(), q, runID, mode, "I could not verify an answer in the prepared historical books. Check the considered passages, try another question about these sources, or consult other sources for topics outside this library.", evidence, selected, searches...)
		return
	}
	if err = a.verifyEvidence(r.Context(), used); err != nil {
		fail(w, 409, "The selected passages no longer match their original pages.")
		return
	}
	claimChecks := []claimCheck{}
	checkCtx := context.WithValue(r.Context(), claimCollectorKey{}, &claimChecks)
	a.setAnswerJobStage(r.Context(), "Checking claims against source quotes")
	verificationStarted := time.Now()
	checkedAnswer, checkedCitations, rejectedClaims, initialChecks, err := verifyClaimSentencesBatched(checkCtx, q, draft.Answer, evidence, a.Model.Chat)
	verificationDuration := time.Since(verificationStarted)
	verificationLog := map[string]any{"checked_claim_count": len(initialChecks), "rejected_claim_count": rejectedClaims, "prompt_revision": claimPromptRevision, "error": err != nil}
	if err != nil {
		verificationLog["error_message"] = err.Error()
	}
	a.logAnswerStageFromContext(r.Context(), "claim_verification", verificationLog, &verificationDuration)
	claimChecks = append(claimChecks, initialChecks...)
	if err != nil {
		fail(w, 503, "Support check failed: "+err.Error())
		return
	}
	omittedClaims += rejectedClaims
	draft.Answer, draft.Citations = checkedAnswer, checkedCitations
	used, valid = validateDraft(draft.Answer, draft.Citations, evidence)
	if !valid {
		reason := "I could not verify the draft claims against the source pages. Check the claim checks and considered passages or try a different question about these sources."
		if len(selected) == 0 && strings.Contains(strings.ToLower(q), "selected study") {
			reason = "I could not verify an answer about a single study while searching all sources. Choose one prepared paper, then ask about its design and results. Check the claim checks and considered passages below."
		}
		a.insufficientWithChecks(w, r.Context(), q, runID, mode, reason, evidence, selected, claimChecks, searches...)
		return
	}
	if err = a.verifyEvidence(r.Context(), used); err != nil {
		fail(w, 409, "The selected passages no longer match their original pages.")
		return
	}
	verifiedChecks := claimChecks[:0]
	for _, check := range claimChecks {
		if check.Decision != "supported" || strings.Contains(draft.Answer, check.Text) {
			verifiedChecks = append(verifiedChecks, check)
		}
	}
	claimChecks = verifiedChecks
	sections := []researchSection{}
	if mode == "deep" {
		sections, err = organizeResearchSections(r.Context(), q, "", draft.Answer, evidence, a.Model.Chat)
		if err != nil {
			fail(w, 503, "Research outline failed: "+err.Error())
			return
		}
	}
	sectionsJSON, err := json.Marshal(sections)
	if err != nil {
		fail(w, 500, "Could not save research outline.")
		return
	}
	a.setAnswerJobStage(r.Context(), "Saving checked answer")
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not save answer.")
		return
	}
	defer tx.Rollback(r.Context())
	answerID := uuid.New()
	ids := make([]uuid.UUID, 0, len(used))
	for _, h := range used {
		ids = append(ids, h.ID)
	}
	answerStatus := "answered"
	if omittedClaims > 0 {
		answerStatus = "partial"
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO answers(id,question,status,answer_text,answer_model,answer_revision,prompt_revision,index_run_id,evidence_ids,research_mode,omitted_claim_count,research_sections,answer_job_id,owner_role,owner_principal_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, answerID, q, answerStatus, draft.Answer, a.Model.Config.AnswerModel, a.Model.Config.AnswerRevision, answerPromptRevision, runID, ids, mode, omittedClaims, sectionsJSON, jobIDFromContext(r.Context()), answerOwner(r.Context()), requestPrincipal(r.Context()).ID)
	if err != nil {
		fail(w, 500, "Could not save answer.")
		return
	}
	for i, check := range claimChecks {
		ordinal := i + 1
		if _, err = tx.Exec(r.Context(), `INSERT INTO answer_claims(answer_id,ordinal,claim_text,decision,check_method,verification_prompt_revision,verification_model) VALUES($1,$2,$3,$4,'exact_quote_and_relevance',$5,$6)`, answerID, ordinal, check.Text, check.Decision, claimPromptRevision, a.Model.Config.AnswerModel); err != nil {
			fail(w, 500, "Could not save claim checks.")
			return
		}
		for _, support := range check.Supports {
			if _, err = tx.Exec(r.Context(), `INSERT INTO answer_claim_support(answer_id,claim_ordinal,chunk_id,evidence_label,excerpt_start,excerpt_end,excerpt_text) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, answerID, ordinal, support.ChunkID, support.Label, support.Start, support.End, support.Text); err != nil {
				fail(w, 500, "Could not save supporting excerpts.")
				return
			}
		}
	}
	for i, sourceID := range selected {
		if _, err = tx.Exec(r.Context(), `INSERT INTO answer_source_filters(answer_id,source_id,ordinal) VALUES($1,$2,$3)`, answerID, sourceID, i+1); err != nil {
			fail(w, 500, "Could not save source selection.")
			return
		}
	}
	for i, search := range searches {
		if _, err = tx.Exec(r.Context(), `INSERT INTO answer_searches(answer_id,ordinal,query,source_scope,candidate_count) VALUES($1,$2,$3,$4,$5)`, answerID, i+1, search.Query, search.SourceScope, search.CandidateCount); err != nil {
			fail(w, 500, "Could not save search record.")
			return
		}
	}
	for i, h := range evidence {
		if _, err = tx.Exec(r.Context(), `INSERT INTO answer_candidates(answer_id,ordinal,chunk_id,retrieval_score) VALUES($1,$2,$3,$4)`, answerID, i+1, h.ID, h.Score); err != nil {
			fail(w, 500, "Could not save evidence record.")
			return
		}
	}
	seenSources := map[uuid.UUID]bool{}
	for _, h := range used {
		if seenSources[h.SourceID] {
			continue
		}
		seenSources[h.SourceID] = true
		tag, saveErr := tx.Exec(r.Context(), `INSERT INTO answer_index_runs(answer_id,index_run_id) SELECT $1,ir.id FROM active_indexes ai JOIN index_runs ir ON ir.id=ai.index_run_id JOIN sources s ON s.id=ai.source_id WHERE s.id=$2 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready' AND ir.embedding_config_id=$3`, answerID, h.SourceID, configID)
		if saveErr != nil || tag.RowsAffected() != 1 {
			fail(w, 409, "A cited source is no longer prepared and allowed.")
			return
		}
	}
	cites := []any{}
	for i, h := range used {
		cid := uuid.New()
		label := fmt.Sprintf("E%d", indexOf(evidence, h.ID)+1)
		_, err = tx.Exec(r.Context(), `INSERT INTO answer_citations(id,answer_id,chunk_id,ordinal,evidence_label) VALUES($1,$2,$3,$4,$5)`, cid, answerID, h.ID, i+1, label)
		if err != nil {
			fail(w, 500, "Could not save citations.")
			return
		}
		cites = append(cites, map[string]any{"id": cid, "label": label, "title": h.Title, "author": h.Author, "printed_page": h.Page, "scan_position": h.Scan})
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "Could not save answer.")
		return
	}
	write(w, 200, map[string]any{"answer_id": answerID, "status": answerStatus, "answer": draft.Answer, "citations": cites, "searches": searches, "source_ids": selected, "research_mode": mode, "omitted_claim_count": omittedClaims, "evidence": evidenceCards(evidence, used), "sections": sections})
}

func selectEvidence(candidates []hit, limit int) []hit {
	sources := map[uuid.UUID]bool{}
	for _, h := range candidates {
		sources[h.SourceID] = true
	}
	selected := make([]hit, 0, limit)
	chosen := map[uuid.UUID]bool{}
	perSource := map[uuid.UUID]int{}
	quota := limit
	if len(sources) > 1 {
		quota = (limit + len(sources) - 1) / len(sources)
	}
	for _, h := range candidates {
		if perSource[h.SourceID] >= quota || chosen[h.ID] {
			continue
		}
		selected = append(selected, h)
		chosen[h.ID] = true
		perSource[h.SourceID]++
		if len(selected) == limit {
			return selected
		}
	}
	for _, h := range candidates {
		if chosen[h.ID] {
			continue
		}
		selected = append(selected, h)
		if len(selected) == limit {
			break
		}
	}
	return selected
}

// Verify each claim with only its own cited passages. Whole-answer checks can
// overlook a reversed comparison when the relevant names occur elsewhere.
type claimSupport struct {
	ChunkID uuid.UUID
	Label   string
	Start   int
	End     int
	Text    string
}

type claimCheck struct {
	Text     string
	Decision string
	Supports []claimSupport
}
type claimCollectorKey struct{}

func verifyClaimSentences(ctx context.Context, question, answer string, hits []hit, chat func(context.Context, string, string) (string, error)) (string, []string, int, error) {
	checked, labels, rejected, checks, err := verifyClaimSentencesDetailed(ctx, question, answer, hits, chat)
	if collector, ok := ctx.Value(claimCollectorKey{}).(*[]claimCheck); ok && err == nil {
		*collector = append(*collector, checks...)
	}
	return checked, labels, rejected, err
}

func verifyClaimSentencesDetailed(ctx context.Context, question, answer string, hits []hit, chat func(context.Context, string, string) (string, error)) (string, []string, int, []claimCheck, error) {
	kept := []string{}
	labels := map[string]bool{}
	rejected := 0
	checks := []claimCheck{}
	for _, sentence := range splitAnswerSentences(answer) {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		checkRecord := claimCheck{Text: sentence, Decision: "missing_excerpt"}
		var passages strings.Builder
		cited := map[string]hit{}
		for _, match := range citePattern.FindAllStringSubmatch(sentence, -1) {
			for i, h := range hits {
				label := fmt.Sprintf("E%d", i+1)
				_, exists := cited[label]
				if label == "E"+match[1] && !exists {
					cited[label] = h
					fmt.Fprintf(&passages, "[E%d] %s\n", i+1, h.Text)
				}
			}
		}
		if passages.Len() == 0 {
			rejected++
			checks = append(checks, checkRecord)
			continue
		}
		check, err := chat(ctx, "For each cited passage, copy one short exact quotation that supports this complete claim and its relevance to the question. A remedy name alone or a nearby different symptom is not support. For a comparison, quote support for each side. If the passage does not support the claim, return an empty quotes array. Use only the provided passages. Return JSON only: {\"quotes\":[{\"id\":\"E1\",\"text\":\"exact words from E1\"}]}. Do not paraphrase quotations.", "Question: "+question+"\nClaim: "+sentence+"\nCited passages:\n"+passages.String())
		if err != nil {
			return "", nil, 0, nil, err
		}
		var anchors struct {
			Quotes []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"quotes"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(check)), &anchors); err != nil || anchors.Quotes == nil {
			return "", nil, 0, nil, fmt.Errorf("answer model returned invalid quotation check")
		}
		var quoted strings.Builder
		seenQuotes := map[string]bool{}
		for _, quote := range anchors.Quotes {
			label := strings.TrimSuffix(strings.TrimPrefix(quote.ID, "["), "]")
			h, ok := cited[label]
			if !ok || seenQuotes[label] {
				continue
			}
			if ambiguousOpeningComparison(h.Text) {
				continue
			}
			exactQuote, ok := recoverPassageQuote(h.Text, quote.Text)
			if !ok {
				continue
			}
			start, end, found := locatePassageQuote(h.Text, exactQuote)
			if !found {
				continue
			}
			start = extendQuoteToSubjectHeading(h.Text, sentence, start)
			seenQuotes[label] = true
			excerpt := string([]rune(h.Text)[start:end])
			checkRecord.Supports = append(checkRecord.Supports, claimSupport{ChunkID: h.ID, Label: label, Start: start, End: end, Text: excerpt})
			fmt.Fprintf(&quoted, "[%s] %s\n", label, excerpt)
		}
		if len(seenQuotes) != len(cited) {
			rejected++
			checks = append(checks, checkRecord)
			continue
		}
		focus, err := chat(ctx, "Judge the entire claim against only the exact source quotations below. Both conditions must hold: every factual part is supported by the quotations, and the claim directly answers the question. A quotation about a named remedy but a different symptom does not support a claim about the question's symptom. Check negation, comparison direction, and named entities. Return only JSON {\"supported\":true,\"relevant\":true} or false values.", "Question: "+question+"\nClaim: "+sentence+"\nExact source quotations:\n"+quoted.String())
		if err != nil {
			return "", nil, 0, nil, err
		}
		var relevance struct {
			Supported *bool `json:"supported"`
			Relevant  *bool `json:"relevant"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(focus)), &relevance); err != nil || relevance.Relevant == nil || relevance.Supported == nil {
			return "", nil, 0, nil, fmt.Errorf("answer model returned invalid support verdict")
		}
		if !*relevance.Supported || !*relevance.Relevant {
			rejected++
			checkRecord.Decision = "unsupported"
			if !*relevance.Relevant {
				checkRecord.Decision = "irrelevant"
			}
			checks = append(checks, checkRecord)
			continue
		}
		checkRecord.Decision = "supported"
		checks = append(checks, checkRecord)
		kept = append(kept, sentence)
		for _, match := range citePattern.FindAllStringSubmatch(sentence, -1) {
			labels["E"+match[1]] = true
		}
	}
	declared := make([]string, 0, len(labels))
	for i := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if labels[label] {
			declared = append(declared, label)
		}
	}
	return strings.Join(kept, " "), declared, rejected, checks, nil
}

// Check every drafted sentence in two provider calls, while validating each
// returned quotation against the stored passage before asking for a verdict.
func verifyClaimSentencesBatched(ctx context.Context, question, answer string, hits []hit, chat func(context.Context, string, string) (string, error)) (string, []string, int, []claimCheck, error) {
	type passageInput struct {
		ID      string `json:"id"`
		Text    string `json:"text"`
		Context string `json:"context,omitempty"`
	}
	type quoteInput struct {
		Claim    int            `json:"claim"`
		Text     string         `json:"text"`
		Passages []passageInput `json:"passages"`
	}
	type verdictInput struct {
		Claim  int            `json:"claim"`
		Text   string         `json:"text"`
		Quotes []passageInput `json:"quotes"`
	}
	sentences := splitAnswerSentences(answer)
	checks := make([]claimCheck, len(sentences))
	requests := make([]quoteInput, 0, len(sentences))
	citedByClaim := make(map[int]map[string]hit)
	rejected := 0
	for i, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		checks[i] = claimCheck{Text: sentence, Decision: "missing_excerpt"}
		if sentence == "" {
			continue
		}
		cited := map[string]hit{}
		passages := []passageInput{}
		for _, match := range citePattern.FindAllStringSubmatch(sentence, -1) {
			for index, h := range hits {
				label := fmt.Sprintf("E%d", index+1)
				_, exists := cited[label]
				if label == "E"+match[1] && !exists {
					cited[label] = h
					passages = append(passages, passageInput{ID: label, Text: h.Text})
				}
			}
		}
		if len(passages) == 0 {
			rejected++
			continue
		}
		citedByClaim[i+1] = cited
		requests = append(requests, quoteInput{Claim: i + 1, Text: sentence, Passages: passages})
	}
	if len(requests) == 0 {
		return "", nil, rejected, checks, nil
	}
	requestJSON, _ := json.Marshal(requests)
	raw, err := chat(ctx, "For each numbered claim, copy one short exact quotation from every cited passage that supports the complete claim and directly answers the question. Return an empty quotes array when support is missing. Do not paraphrase. Return only JSON {\"checks\":[{\"claim\":1,\"quotes\":[{\"id\":\"E1\",\"text\":\"exact words\"}]}]} with one check per claim.", "Question: "+question+"\nClaims and cited passages: "+string(requestJSON))
	if err != nil {
		return "", nil, 0, nil, err
	}
	var quoteResponse struct {
		Checks []struct {
			Claim  int `json:"claim"`
			Quotes []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"quotes"`
		} `json:"checks"`
	}
	if json.NewDecoder(strings.NewReader(strings.TrimSpace(raw))).Decode(&quoteResponse) != nil || quoteResponse.Checks == nil {
		return "", nil, 0, nil, errors.New("answer model returned invalid grouped quotation checks")
	}
	quotesByClaim := map[int]map[string]string{}
	for _, item := range quoteResponse.Checks {
		if quotesByClaim[item.Claim] != nil {
			continue
		}
		quotesByClaim[item.Claim] = map[string]string{}
		for _, quote := range item.Quotes {
			label := strings.TrimSuffix(strings.TrimPrefix(quote.ID, "["), "]")
			quotesByClaim[item.Claim][label] = quote.Text
		}
	}
	verdictRequests := []verdictInput{}
	for _, request := range requests {
		cited := citedByClaim[request.Claim]
		quotes := []passageInput{}
		for _, passage := range request.Passages {
			h := cited[passage.ID]
			if ambiguousOpeningComparison(h.Text) {
				break
			}
			quote, ok := quotesByClaim[request.Claim][passage.ID]
			if !ok {
				break
			}
			exact, ok := recoverPassageQuote(h.Text, quote)
			if !ok {
				break
			}
			start, end, ok := locatePassageQuote(h.Text, exact)
			if !ok {
				break
			}
			start = extendQuoteToSubjectHeading(h.Text, request.Text, start)
			excerpt := string([]rune(h.Text)[start:end])
			checks[request.Claim-1].Supports = append(checks[request.Claim-1].Supports, claimSupport{ChunkID: h.ID, Label: passage.ID, Start: start, End: end, Text: excerpt})
			context := []rune(strings.TrimSpace(h.Text))
			if len(context) > 120 {
				context = context[:120]
			}
			quotes = append(quotes, passageInput{ID: passage.ID, Text: excerpt, Context: string(context)})
		}
		if len(quotes) != len(request.Passages) {
			rejected++
			continue
		}
		verdictRequests = append(verdictRequests, verdictInput{Claim: request.Claim, Text: request.Text, Quotes: quotes})
	}
	if len(verdictRequests) == 0 {
		return "", nil, rejected, checks, nil
	}
	verdictJSON, _ := json.Marshal(verdictRequests)
	raw, err = chat(ctx, "Judge each numbered claim against only its exact source quotations. The short context may identify the source section or subject, but factual details must come from the exact quote. Every factual part must be supported and the claim must directly answer the question. Check negation, comparison direction, and named entities. Return only JSON {\"checks\":[{\"claim\":1,\"supported\":true,\"relevant\":true}]} with one verdict per claim.", "Question: "+question+"\nClaims and exact quotations: "+string(verdictJSON))
	if err != nil {
		return "", nil, 0, nil, err
	}
	var verdictResponse struct {
		Checks []struct {
			Claim     int   `json:"claim"`
			Supported *bool `json:"supported"`
			Relevant  *bool `json:"relevant"`
		} `json:"checks"`
	}
	if json.NewDecoder(strings.NewReader(strings.TrimSpace(raw))).Decode(&verdictResponse) != nil || verdictResponse.Checks == nil {
		return "", nil, 0, nil, errors.New("answer model returned invalid grouped support verdicts")
	}
	verdicts := map[int]struct{ supported, relevant bool }{}
	for _, item := range verdictResponse.Checks {
		if item.Supported != nil && item.Relevant != nil {
			verdicts[item.Claim] = struct{ supported, relevant bool }{*item.Supported, *item.Relevant}
		}
	}
	kept := []string{}
	labels := map[string]bool{}
	for _, request := range verdictRequests {
		verdict := verdicts[request.Claim]
		if !verdict.supported || !verdict.relevant {
			rejected++
			checks[request.Claim-1].Decision = "unsupported"
			if !verdict.relevant {
				checks[request.Claim-1].Decision = "irrelevant"
			}
			continue
		}
		checks[request.Claim-1].Decision = "supported"
		kept = append(kept, request.Text)
		for _, passage := range request.Quotes {
			labels[passage.ID] = true
		}
	}
	declared := []string{}
	for i := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if labels[label] {
			declared = append(declared, label)
		}
	}
	return strings.Join(kept, " "), declared, rejected, checks, nil
}

// A passage that begins with a comparison has lost the preceding subject at
// the chunk boundary. Its listed remedies must not become the claim's subject.
func ambiguousOpeningComparison(passage string) bool {
	words := strings.Fields(strings.ToLower(passage))
	if len(words) == 0 {
		return false
	}
	return words[0] == "like" || (len(words) > 1 && ((words[0] == "as" && words[1] == "with") || (words[0] == "similar" && words[1] == "to")))
}

// If a short quotation omits the nearby section heading naming its subject,
// include the heading in the same contiguous source excerpt. This lets the
// verifier resolve pronouns such as "the headache" without relying on an
// uncited description of the passage.
func extendQuoteToSubjectHeading(passage, claim string, start int) int {
	runes := []rune(passage)
	if start < 1 || start > len(runes) {
		return start
	}
	prefix := string(runes[:start])
	lineStart := 0
	for _, line := range strings.Split(prefix, "\n") {
		trimmed := strings.TrimSpace(line)
		letters, uppercase := 0, 0
		for _, ch := range trimmed {
			if unicode.IsLetter(ch) {
				letters++
				if unicode.IsUpper(ch) {
					uppercase++
				}
			}
		}
		if letters >= 5 && uppercase*5 >= letters*4 && start-lineStart <= 220 {
			for _, word := range quoteWordPattern.FindAllString(strings.ToLower(trimmed), -1) {
				if len([]rune(word)) >= 5 && strings.Contains(strings.ToLower(claim), word) {
					return lineStart
				}
			}
		}
		lineStart += utf8.RuneCountInString(line) + 1
	}
	return start
}

// Character offsets use Unicode code points, matching chunk passage text.
func locatePassageQuote(passage, quote string) (int, int, bool) {
	// Keep a map from dehyphenated OCR characters back to the original passage.
	// A quotation can differ in whitespace or a line-break hyphen and still
	// resolve to a precise contiguous excerpt of the saved source text.
	skips := splitOCRWordPattern.FindAllStringIndex(passage, -1)
	clean := make([]rune, 0, utf8.RuneCountInString(passage))
	original := make([]int, 0, cap(clean))
	skipIndex, runeOffset := 0, 0
	for byteOffset, ch := range passage {
		for skipIndex < len(skips) && byteOffset >= skips[skipIndex][1] {
			skipIndex++
		}
		if skipIndex >= len(skips) || byteOffset < skips[skipIndex][0] {
			clean = append(clean, ch)
			original = append(original, runeOffset)
		}
		runeOffset++
	}
	cleanText := string(clean)
	wordSpans := quoteWordPattern.FindAllStringIndex(cleanText, -1)
	quoteWords := quoteWordPattern.FindAllString(strings.ToLower(splitOCRWordPattern.ReplaceAllString(quote, "")), -1)
	if len(quoteWords) < 4 || len(quoteWords) > len(wordSpans) {
		return 0, 0, false
	}
	for i := 0; i+len(quoteWords) <= len(wordSpans); i++ {
		match := true
		for j, word := range quoteWords {
			if strings.ToLower(cleanText[wordSpans[i+j][0]:wordSpans[i+j][1]]) != word {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		startClean := utf8.RuneCountInString(cleanText[:wordSpans[i][0]])
		endClean := utf8.RuneCountInString(cleanText[:wordSpans[i+len(quoteWords)-1][1]])
		return original[startClean], original[endClean-1] + 1, true
	}
	return 0, 0, false
}

func exactPassageQuote(passage, quote string) bool {
	if len([]rune(quote)) > 500 {
		return false
	}
	normalize := func(value string) []string {
		value = splitOCRWordPattern.ReplaceAllString(value, "")
		return quoteWordPattern.FindAllString(strings.ToLower(value), -1)
	}
	words := normalize(quote)
	if len(words) < 4 {
		return false
	}
	return strings.Contains(" "+strings.Join(normalize(passage), " ")+" ", " "+strings.Join(words, " ")+" ")
}

// OCR can corrupt a word inside an otherwise copied quotation. Keep only the
// longest contiguous part that exists in the saved passage; the later claim
// verdict still checks whether that narrower excerpt supports the claim.
func recoverPassageQuote(passage, quote string) (string, bool) {
	if exactPassageQuote(passage, quote) {
		return quote, true
	}
	if utf8.RuneCountInString(quote) > 500 {
		return "", false
	}
	quoteWords := quoteWordPattern.FindAllString(strings.ToLower(splitOCRWordPattern.ReplaceAllString(quote, "")), -1)
	passageWords := quoteWordPattern.FindAllString(strings.ToLower(splitOCRWordPattern.ReplaceAllString(passage, "")), -1)
	if len(quoteWords) < 4 {
		return "", false
	}
	bestStart, bestLength := 0, 0
	for i, word := range quoteWords {
		for j, passageWord := range passageWords {
			if word != passageWord {
				continue
			}
			length := 0
			for i+length < len(quoteWords) && j+length < len(passageWords) && quoteWords[i+length] == passageWords[j+length] {
				length++
			}
			if length > bestLength {
				bestStart, bestLength = i, length
			}
		}
	}
	if bestLength < 4 || bestLength*2 < len(quoteWords) {
		return "", false
	}
	matched := strings.Join(quoteWords[bestStart:bestStart+bestLength], " ")
	start, end, ok := locatePassageQuote(passage, matched)
	if !ok {
		return "", false
	}
	return string([]rune(passage)[start:end]), true
}

// Give a source omitted by the first draft a focused chance to contribute.
// Added claims pass the same structural, passage and relevance checks.
func supplementUnrepresentedSources(ctx context.Context, question, answer string, cites []string, hits []hit, chat func(context.Context, string, string) (string, error)) (string, []string, int, error) {
	used := map[string]bool{}
	for _, label := range cites {
		used[label] = true
	}
	represented := map[uuid.UUID]bool{}
	for i, h := range hits {
		if used[fmt.Sprintf("E%d", i+1)] {
			represented[h.SourceID] = true
		}
	}
	omitted := 0
	visited := map[uuid.UUID]bool{}
	for _, source := range hits {
		if visited[source.SourceID] || represented[source.SourceID] {
			continue
		}
		visited[source.SourceID] = true
		var pack strings.Builder
		for i, h := range hits {
			if h.SourceID == source.SourceID {
				fmt.Fprintf(&pack, "[E%d] %s by %s\n%s\n\n", i+1, h.Title, h.Author, h.Text)
			}
		}
		raw, err := chat(ctx, "You are a research guide. Add two or three distinct, concise findings from this author that directly address the user's question. Use only the supplied passages; if they do not directly address the question, return an empty answer. Start each sentence with the author or remedy name, make one narrow claim per sentence, and cite that same sentence inline with [E#]. Do not include uncited introductions, medical advice, or unsupported comparisons. Return only JSON {\"answer\":\"...\",\"citations\":[\"E1\"]} with the exact IDs used.", "Question: "+question+"\nAuthor: "+source.Author+"\nPassages:\n"+pack.String())
		if err != nil {
			return "", nil, 0, err
		}
		var draft struct {
			Answer    string   `json:"answer"`
			Citations []string `json:"citations"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &draft); err != nil {
			return "", nil, 0, fmt.Errorf("answer model returned invalid source supplement: %w", err)
		}
		if strings.TrimSpace(draft.Answer) == "" {
			continue
		}
		draft.Answer = normalizeDraftCitations(draft.Answer, draft.Citations)
		if _, ok := validateDraft(draft.Answer, draft.Citations, hits); !ok {
			var dropped int
			draft.Answer, draft.Citations, dropped = retainSupportedSentences(draft.Answer, hits)
			omitted += dropped
		}
		if _, ok := validateDraft(draft.Answer, draft.Citations, hits); !ok {
			continue
		}
		checked, labels, dropped, err := verifyClaimSentences(ctx, question, draft.Answer, hits, chat)
		if err != nil {
			return "", nil, 0, err
		}
		omitted += dropped
		if _, ok := validateDraft(checked, labels, hits); !ok {
			continue
		}
		ownSource := true
		for _, label := range labels {
			for i, h := range hits {
				if label == fmt.Sprintf("E%d", i+1) && h.SourceID != source.SourceID {
					ownSource = false
				}
			}
		}
		if !ownSource {
			continue
		}
		answer = strings.TrimSpace(answer + " " + checked)
		for _, label := range labels {
			used[label] = true
		}
	}
	allLabels := make([]string, 0, len(used))
	for i := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if used[label] {
			allLabels = append(allLabels, label)
		}
	}
	return answer, allLabels, omitted, nil
}

func extendAnswerFromUnusedEvidence(ctx context.Context, question, answer string, cites []string, hits []hit, target int, chat func(context.Context, string, string) (string, error)) (string, []string, int, error) {
	used := map[string]bool{}
	for _, label := range cites {
		used[label] = true
	}
	omitted, attempts := 0, 0
	for i, h := range hits {
		if len(splitAnswerSentences(answer)) >= target || attempts >= 6 {
			break
		}
		label := fmt.Sprintf("E%d", i+1)
		if used[label] {
			continue
		}
		attempts++
		raw, err := chat(ctx, "Write exactly one new, concise finding that directly answers the question from the single passage supplied. It must add information absent from the existing answer. Use no outside knowledge or medical advice. Name the author or remedy, and cite the sentence inline with the supplied ID. If the passage offers no distinct finding, return an empty answer. Return only JSON {\"answer\":\"...\",\"citations\":[\"E1\"]} using the supplied ID.", "Question: "+question+"\nExisting answer: "+answer+"\nPassage: ["+label+"] "+h.Title+" by "+h.Author+"\n"+h.Text)
		if err != nil {
			return "", nil, 0, err
		}
		var draft struct {
			Answer    string   `json:"answer"`
			Citations []string `json:"citations"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &draft); err != nil {
			return "", nil, 0, fmt.Errorf("answer model returned invalid expansion: %w", err)
		}
		if strings.TrimSpace(draft.Answer) == "" {
			continue
		}
		if len(draft.Citations) != 1 || draft.Citations[0] != label {
			omitted++
			continue
		}
		draft.Answer = normalizeDraftCitations(draft.Answer, draft.Citations)
		if _, ok := validateDraft(draft.Answer, draft.Citations, hits); !ok {
			omitted++
			continue
		}
		checked, labels, rejected, err := verifyClaimSentences(ctx, question, draft.Answer, hits, chat)
		if err != nil {
			return "", nil, 0, err
		}
		omitted += rejected
		if len(labels) != 1 || labels[0] != label {
			continue
		}
		if _, ok := validateDraft(checked, labels, hits); !ok {
			continue
		}
		answer = strings.TrimSpace(answer + " " + checked)
		used[label] = true
	}
	allLabels := make([]string, 0, len(used))
	for i := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if used[label] {
			allLabels = append(allLabels, label)
		}
	}
	return answer, allLabels, omitted, nil
}

func synthesizeResearchLead(ctx context.Context, question, answer string, cites []string, hits []hit, chat func(context.Context, string, string) (string, error)) (string, []string, error) {
	if len(splitAnswerSentences(answer)) < 2 || len(cites) < 2 {
		return "", nil, nil
	}
	used := map[string]bool{}
	for _, label := range cites {
		used[label] = true
	}
	var sourceLabels strings.Builder
	for i, h := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if used[label] {
			fmt.Fprintf(&sourceLabels, "[%s] %s\n", label, h.Author)
		}
	}
	raw, err := chat(ctx, "Write one research thesis of at most 30 words. Begin with the cited authors' surnames. Summarize how their accounts frame the question, using only the verified findings. Do not name or list individual remedies, repeat a detail, use a semicolon, or claim clinical effectiveness. End with two or three distinct citation IDs; if both authors are represented, cite one passage from each. If no faithful thesis is possible, return an empty answer. Return only JSON {\"answer\":\"...\",\"citations\":[\"E1\",\"E2\"]}.", "Question: "+question+"\nVerified findings:\n"+answer+"\nCitation authors:\n"+sourceLabels.String())
	if err != nil {
		return "", nil, err
	}
	if strings.Contains(raw, ";") || regexp.MustCompile(`\[E[0-9]+,\s*E[0-9]+`).MatchString(raw) {
		selected := []string{}
		seenSources := map[uuid.UUID]bool{}
		for i, h := range hits {
			label := fmt.Sprintf("E%d", i+1)
			if used[label] && !seenSources[h.SourceID] {
				selected = append(selected, label)
				seenSources[h.SourceID] = true
			}
		}
		for _, label := range cites {
			if len(selected) >= 2 {
				break
			}
			if !slices.Contains(selected, label) {
				selected = append(selected, label)
			}
		}
		if len(selected) >= 2 && len(seenSources) >= 2 {
			facts := []string{}
			authors := []string{}
			for _, label := range selected[:2] {
				for i, h := range hits {
					if label != fmt.Sprintf("E%d", i+1) {
						continue
					}
					name := h.Author
					if strings.Contains(strings.ToLower(name), "farrington") {
						name = "Farrington"
					} else if strings.Contains(strings.ToLower(name), "nash") {
						name = "Nash"
					}
					authors = append(authors, name)
					for _, sentence := range splitAnswerSentences(answer) {
						if strings.Contains(sentence, "["+label+"]") {
							facts = append(facts, "["+label+"] "+name+": "+strings.TrimSpace(sentence))
							break
						}
					}
					break
				}
			}
			if len(facts) == 2 && len(authors) == 2 {
				topic := ""
				if len(strings.Fields(question)) <= 3 {
					topic = strings.TrimSpace(question)
				}
				shape := fmt.Sprintf("%s describes ___, whereas %s describes ___ [%s][%s].", authors[0], authors[1], selected[0], selected[1])
				if topic != "" {
					shape = fmt.Sprintf("%s describes %s alongside ___, whereas %s describes ___ [%s][%s].", authors[0], topic, authors[1], selected[0], selected[1])
				}
				raw, err = chat(ctx, "Write one research comparison sentence of at most 28 words. Use this sentence shape: "+shape+" Fill the blanks with symptom patterns from only the two verified findings, without remedy names. Do not use a semicolon or add another claim. Return only JSON {\"answer\":\"...\",\"citations\":[\""+selected[0]+"\",\""+selected[1]+"\"]}.", "Question: "+question+"\n"+strings.Join(facts, "\n"))
			}
			if err != nil {
				return "", nil, err
			}
		}
	}
	var draft struct {
		Answer    string   `json:"answer"`
		Sentence  string   `json:"sentence"`
		Citations []string `json:"citations"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &draft); err != nil {
		return "", nil, fmt.Errorf("answer model returned invalid synthesis: %w", err)
	}
	if draft.Answer == "" {
		draft.Answer = draft.Sentence
	}
	for i, label := range draft.Citations {
		draft.Citations[i] = strings.TrimSuffix(strings.TrimPrefix(label, "["), "]")
	}
	if strings.TrimSpace(draft.Answer) == "" {
		return "", nil, nil
	}
	draft.Answer = normalizeDraftCitations(draft.Answer, draft.Citations)
	if len(strings.Fields(citationLikePattern.ReplaceAllString(draft.Answer, ""))) > 32 || strings.Contains(draft.Answer, ";") || len(draft.Citations) < 2 || len(draft.Citations) > 3 {
		return "", nil, nil
	}
	if citationLikePattern.MatchString(draft.Answer) && !citePattern.MatchString(draft.Answer) {
		return "", nil, nil
	}
	if !citePattern.MatchString(draft.Answer) && len(splitAnswerSentences(draft.Answer)) == 1 {
		body := strings.TrimSpace(draft.Answer)
		punctuation := ""
		if strings.ContainsAny(body[len(body)-1:], ".!?") {
			punctuation = body[len(body)-1:]
			body = strings.TrimSpace(body[:len(body)-1])
		}
		for _, label := range draft.Citations {
			body += " [" + label + "]"
		}
		draft.Answer = body + punctuation
	}
	if len(splitAnswerSentences(draft.Answer)) != 1 || len(draft.Citations) < 2 {
		return "", nil, nil
	}
	for _, label := range draft.Citations {
		if !used[label] {
			return "", nil, nil
		}
	}
	availableSources := map[uuid.UUID]bool{}
	leadSources := map[uuid.UUID]bool{}
	for i, h := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if used[label] {
			availableSources[h.SourceID] = true
		}
		for _, cited := range draft.Citations {
			if cited == label {
				leadSources[h.SourceID] = true
			}
		}
	}
	if len(availableSources) > 1 && len(leadSources) < 2 {
		return "", nil, nil
	}
	if _, ok := validateDraft(draft.Answer, draft.Citations, hits); !ok {
		return "", nil, nil
	}
	checked, labels, rejected, err := verifyClaimSentences(ctx, question, draft.Answer, hits, chat)
	if err != nil {
		return "", nil, err
	}
	if rejected != 0 || len(labels) < 2 {
		return "", nil, nil
	}
	if !strings.ContainsAny(checked[len(checked)-1:], ".!?") {
		checked += "."
	}
	return checked, labels, nil
}

func evidenceCards(hits, cited []hit) []any {
	used := map[uuid.UUID]bool{}
	for _, h := range cited {
		used[h.ID] = true
	}
	out := make([]any, 0, len(hits))
	for i, h := range hits {
		preview := []rune(h.Text)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		out = append(out, map[string]any{"rank": i + 1, "chunk_id": h.ID, "title": h.Title, "author": h.Author, "printed_page": h.Page, "scan_position": h.Scan, "image_url": h.ImageURL, "preview": string(preview), "cited": used[h.ID]})
	}
	return out
}
func indexOf(h []hit, id uuid.UUID) int {
	for i, x := range h {
		if x.ID == id {
			return i
		}
	}
	return -1
}

var citePattern = regexp.MustCompile(`\[E([1-9]|10)\]`)
var anyCitationPattern = regexp.MustCompile(`\[E[0-9]+\]`)
var citationLikePattern = regexp.MustCompile(`\[[^\]]*E[0-9]+[^\]]*\]`)

// Keep abbreviated remedy names and personal initials inside their sentence.
// The model may write "Natrum mur. produces..." or "E. B. Nash says...".
func answerSentenceBoundaries(answer string) [][]int {
	candidates := regexp.MustCompile(`[.!?][ \t]+`).FindAllStringIndex(answer, -1)
	boundaries := make([][]int, 0, len(candidates))
	for _, loc := range candidates {
		if loc[1] >= len(answer) {
			continue
		}
		next, _ := utf8.DecodeRuneInString(answer[loc[1]:])
		if !unicode.IsUpper(next) && next != '“' && next != '"' && next != '\'' {
			continue
		}
		before := answer[:loc[0]]
		start := strings.LastIndexAny(before, " \t\n") + 1
		word := strings.ToLower(before[start:])
		if len(word) == 1 || word == "dr" || word == "mr" || word == "mrs" || word == "prof" || word == "etc" || word == "vs" {
			continue
		}
		boundaries = append(boundaries, loc)
	}
	return boundaries
}
func splitAnswerSentences(answer string) []string {
	boundaries := answerSentenceBoundaries(answer)
	parts := make([]string, 0, len(boundaries)+1)
	start := 0
	for _, loc := range boundaries {
		parts = append(parts, answer[start:loc[0]+1])
		start = loc[1]
	}
	parts = append(parts, answer[start:])
	return parts
}

// Salvage independently supported claims when a small model ignores a repair
// request. Every retained sentence still passes the complete answer checks.
func retainSupportedSentences(answer string, hits []hit) (string, []string, int) {
	parts := splitAnswerSentences(answer)
	kept := make([]string, 0, len(parts))
	labels := map[string]bool{}
	omitted := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !citePattern.MatchString(part) {
			omitted++
			continue
		}
		declared := []string{}
		for i := range hits {
			label := fmt.Sprintf("E%d", i+1)
			if strings.Contains(part, "["+label+"]") {
				declared = append(declared, label)
			}
		}
		if _, ok := validateDraft(part, declared, hits); !ok {
			omitted++
			continue
		}
		if len(kept) == 0 && (strings.HasPrefix(part, "He ") || strings.HasPrefix(part, "She ")) {
			if match := citePattern.FindStringSubmatch(part); match != nil {
				for i, h := range hits {
					if fmt.Sprintf("%d", i+1) == match[1] {
						name := h.Author
						if strings.Contains(strings.ToLower(name), "farrington") {
							name = "Farrington"
						}
						if strings.Contains(strings.ToLower(name), "nash") {
							name = "Nash"
						}
						part = name + part[strings.IndexByte(part, ' '):]
						break
					}
				}
			}
		}
		if !strings.ContainsAny(part[len(part)-1:], ".!?") {
			part += "."
		}
		kept = append(kept, part)
		for _, match := range citePattern.FindAllStringSubmatch(part, -1) {
			labels["E"+match[1]] = true
		}
	}
	declared := make([]string, 0, len(labels))
	for i := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if labels[label] {
			declared = append(declared, label)
		}
	}
	return strings.Join(kept, " "), declared, omitted
}

// Normalize citation placement and fill only unambiguous sentence attribution.
// A citation between different passages is left unresolved for the repair pass.
func normalizeDraftCitations(answer string, cites []string) string {
	answer = regexp.MustCompile(`([.!?])\s*(\[E(?:[1-9]|10)\])`).ReplaceAllString(answer, " $2$1")
	paragraphs := strings.Split(answer, "\n")
	for i, paragraph := range paragraphs {
		paragraphs[i] = normalizeParagraphCitations(paragraph, cites)
	}
	return strings.Join(paragraphs, "\n")
}

type citationSentence struct {
	body string
	gap  string
}

func normalizeParagraphCitations(paragraph string, declared []string) string {
	locations := answerSentenceBoundaries(paragraph)
	parts := make([]citationSentence, 0, len(locations)+1)
	start := 0
	for _, loc := range locations {
		end := loc[0] + 1
		parts = append(parts, citationSentence{body: paragraph[start:end], gap: paragraph[end:loc[1]]})
		start = loc[1]
	}
	parts = append(parts, citationSentence{body: paragraph[start:]})
	if len(parts) == 1 && len(anyCitationPattern.FindAllString(paragraph, -1)) > 0 {
		return paragraph
	}
	allLabels := map[string]bool{}
	for _, part := range parts {
		for _, label := range anyCitationPattern.FindAllString(part.body, -1) {
			allLabels[label] = true
		}
	}
	single := ""
	if len(allLabels) == 1 {
		for label := range allLabels {
			single = label
		}
	}
	if len(allLabels) == 0 && len(declared) == 1 && regexp.MustCompile(`^E(?:[1-9]|10)$`).MatchString(declared[0]) {
		single = "[" + declared[0] + "]"
	}
	for i := range parts {
		if strings.TrimSpace(parts[i].body) == "" || anyCitationPattern.MatchString(parts[i].body) {
			continue
		}
		label := single
		if label == "" {
			before, after := "", ""
			for j := i - 1; j >= 0; j-- {
				if m := anyCitationPattern.FindAllString(parts[j].body, -1); len(m) > 0 {
					if len(m) == 1 {
						before = m[0]
					}
					break
				}
			}
			for j := i + 1; j < len(parts); j++ {
				if m := anyCitationPattern.FindAllString(parts[j].body, -1); len(m) > 0 {
					if len(m) == 1 {
						after = m[0]
					}
					break
				}
			}
			if before != "" && before == after {
				label = before
			}
		}
		if label == "" {
			continue
		}
		body := strings.TrimRight(parts[i].body, " \t")
		if len(body) > 0 && strings.ContainsRune(".!?", rune(body[len(body)-1])) {
			body = body[:len(body)-1] + " " + label + body[len(body)-1:]
		} else {
			body += " " + label
		}
		parts[i].body = body
	}
	var out strings.Builder
	for _, part := range parts {
		out.WriteString(part.body)
		out.WriteString(part.gap)
	}
	return out.String()
}

func validateDraft(answer string, cites []string, hits []hit) ([]hit, bool) {
	if strings.TrimSpace(answer) == "" || len(answer) > 5000 {
		return nil, false
	}
	seen := map[string]bool{}
	for _, c := range cites {
		seen[c] = true
	}
	matches := citePattern.FindAllStringSubmatch(answer, -1)
	if len(matches) != len(citationLikePattern.FindAllString(answer, -1)) {
		return nil, false
	}
	if len(matches) == 0 {
		return nil, false
	}
	used := []hit{}
	for i, h := range hits {
		label := fmt.Sprintf("E%d", i+1)
		if seen[label] {
			found := false
			for _, m := range matches {
				if m[0] == "["+label+"]" {
					found = true
				}
			}
			if !found {
				return nil, false
			}
			used = append(used, h)
		}
	}
	if len(used) != len(seen) {
		return nil, false
	}
	for _, m := range matches {
		if !seen["E"+m[1]] {
			return nil, false
		}
	}
	// Proper names must occur in the exact passages cited by the sentence.
	sentences := splitAnswerSentences(answer)
	for _, sentence := range sentences {
		if !regexp.MustCompile(`(?:\s*\[E(?:[1-9]|10)\])+\s*[.!?]?\s*$`).MatchString(sentence) {
			return nil, false
		}
		labels := citePattern.FindAllStringSubmatch(sentence, -1)
		var cited strings.Builder
		for _, m := range labels {
			for i, h := range hits {
				if fmt.Sprintf("E%d", i+1) == "E"+m[1] {
					cited.WriteString(" " + h.Text + " " + h.Title + " " + h.Author)
				}
			}
		}
		for _, quote := range regexp.MustCompile(`[“"]([^”"]{8,})[”"]`).FindAllStringSubmatch(sentence, -1) {
			if !strings.Contains(cited.String(), quote[1]) {
				return nil, false
			}
		}
		names := regexp.MustCompile(`\b[A-Z][a-z]{3,}\b`).FindAllStringIndex(sentence, -1)
		for _, span := range names {
			word := sentence[span[0]:span[1]]
			prefix := strings.TrimSpace(sentence[:span[0]])
			if prefix == "" || strings.HasSuffix(prefix, "[") {
				continue
			}
			if !strings.Contains(strings.ToLower(cited.String()), strings.ToLower(word)) {
				return nil, false
			}
		}
	}
	for _, sentence := range splitAnswerSentences(answer) {
		if strings.TrimSpace(sentence) != "" && !citePattern.MatchString(sentence) {
			return nil, false
		}
	}
	return used, true
}
func (a *API) insufficient(w http.ResponseWriter, ctx context.Context, q string, run uuid.UUID, mode, reason string, candidates []hit, searches ...searchTrace) {
	a.insufficientWithFilters(w, ctx, q, run, mode, reason, candidates, selectedSourcesFromContext(ctx), searches...)
}
func (a *API) insufficientWithFilters(w http.ResponseWriter, ctx context.Context, q string, run uuid.UUID, mode, reason string, candidates []hit, selected []uuid.UUID, searches ...searchTrace) {
	a.insufficientWithChecks(w, ctx, q, run, mode, reason, candidates, selected, nil, searches...)
}
func (a *API) insufficientWithChecks(w http.ResponseWriter, ctx context.Context, q string, run uuid.UUID, mode, reason string, candidates []hit, selected []uuid.UUID, checks []claimCheck, searches ...searchTrace) {
	id := uuid.New()
	tx, err := a.Store.DB.Begin(ctx)
	if err != nil {
		fail(w, 500, "Could not save research result.")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO answers(id,question,status,answer_text,index_run_id,research_mode,answer_job_id,prompt_revision,owner_role,owner_principal_id) VALUES($1,$2,'insufficient_evidence',$3,$4,$5,$6,$7,$8,$9)`, id, q, reason, run, mode, jobIDFromContext(ctx), answerPromptRevision, answerOwner(ctx), requestPrincipal(ctx).ID); err != nil {
		fail(w, 500, "Could not save research result.")
		return
	}
	for i, check := range checks {
		ordinal := i + 1
		if _, err = tx.Exec(ctx, `INSERT INTO answer_claims(answer_id,ordinal,claim_text,decision,check_method,verification_prompt_revision,verification_model) VALUES($1,$2,$3,$4,'exact_quote_and_relevance',$5,$6)`, id, ordinal, check.Text, check.Decision, claimPromptRevision, a.Model.Config.AnswerModel); err != nil {
			fail(w, 500, "Could not save claim checks.")
			return
		}
		for _, support := range check.Supports {
			if _, err = tx.Exec(ctx, `INSERT INTO answer_claim_support(answer_id,claim_ordinal,chunk_id,evidence_label,excerpt_start,excerpt_end,excerpt_text) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, ordinal, support.ChunkID, support.Label, support.Start, support.End, support.Text); err != nil {
				fail(w, 500, "Could not save supporting excerpts.")
				return
			}
		}
	}
	for i, sourceID := range selected {
		if _, err = tx.Exec(ctx, `INSERT INTO answer_source_filters(answer_id,source_id,ordinal) VALUES($1,$2,$3)`, id, sourceID, i+1); err != nil {
			fail(w, 500, "Could not save source selection.")
			return
		}
	}
	for i, search := range searches {
		if _, err = tx.Exec(ctx, `INSERT INTO answer_searches(answer_id,ordinal,query,source_scope,candidate_count) VALUES($1,$2,$3,$4,$5)`, id, i+1, search.Query, search.SourceScope, search.CandidateCount); err != nil {
			fail(w, 500, "Could not save search record.")
			return
		}
	}
	for i, h := range candidates {
		if _, err = tx.Exec(ctx, `INSERT INTO answer_candidates(answer_id,ordinal,chunk_id,retrieval_score) VALUES($1,$2,$3,$4)`, id, i+1, h.ID, h.Score); err != nil {
			fail(w, 500, "Could not save evidence record.")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 500, "Could not save research result.")
		return
	}
	write(w, 200, map[string]any{"answer_id": id, "status": "insufficient_evidence", "answer": reason, "citations": []any{}, "searches": searches, "source_ids": selected, "research_mode": mode, "omitted_claim_count": 0, "evidence": evidenceCards(candidates, nil)})
}
func authorScope(q string) string {
	q = strings.ToLower(q)
	nash, farrington := strings.Contains(q, "nash"), strings.Contains(q, "farrington")
	if nash && !farrington {
		return "nash"
	}
	if farrington && !nash {
		return "farrington"
	}
	return ""
}
func parseSearchPlan(raw, original string) ([]string, error) {
	var plan struct {
		Queries []string `json:"queries"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &plan); err != nil {
		return nil, err
	}
	if len(plan.Queries) != 2 {
		return nil, fmt.Errorf("expected two focused searches")
	}
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(original)): true}
	out := make([]string, 0, 2)
	for _, query := range plan.Queries {
		query = strings.TrimSpace(query)
		key := strings.ToLower(query)
		if len(query) < 8 || len(query) > 160 || strings.ContainsAny(query, "\r\n") || seen[key] {
			return nil, fmt.Errorf("invalid or repeated search")
		}
		seen[key] = true
		out = append(out, query)
	}
	return out, nil
}
func (a *API) retrieve(ctx context.Context, config uuid.UUID, vector, q, scope string, selected []uuid.UUID) ([]hit, error) {
	type ranked struct {
		hit
		rank int
	}
	all := map[uuid.UUID]*hit{}
	terms := queryTerms(q)
	lex := strings.Join(terms, " | ")
	if selected == nil {
		selected = []uuid.UUID{}
	}
	queries := []struct {
		sql string
		arg any
	}{{`SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),p.scan_page_index+1,p.image_url FROM chunk_embeddings ce JOIN chunks c ON c.id=ce.chunk_id JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id JOIN publications pub ON pub.source_id=s.id JOIN index_runs ir ON ir.publication_id=pub.id AND ir.embedding_config_id=ce.embedding_config_id JOIN active_indexes ai ON ai.index_run_id=ir.id WHERE ce.embedding_config_id=$1 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready' AND p.page_kind='text' AND ($3='' OR (s.source_key LIKE $3||'-%' OR s.author ILIKE '%'||$3||'%' OR s.title ILIKE '%'||$3||'%')) AND (cardinality($4::uuid[])=0 OR s.id=ANY($4::uuid[])) ORDER BY ce.embedding <=> $2::vector LIMIT 40`, vector}, {`SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),p.scan_page_index+1,p.image_url FROM chunks c JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id JOIN publications pub ON pub.source_id=s.id JOIN index_runs ir ON ir.publication_id=pub.id JOIN active_indexes ai ON ai.index_run_id=ir.id JOIN chunk_embeddings ce ON ce.chunk_id=c.id AND ce.embedding_config_id=ir.embedding_config_id WHERE ir.embedding_config_id=$1 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready' AND p.page_kind='text' AND ($3='' OR (s.source_key LIKE $3||'-%' OR s.author ILIKE '%'||$3||'%' OR s.title ILIKE '%'||$3||'%')) AND (cardinality($4::uuid[])=0 OR s.id=ANY($4::uuid[])) AND c.search_vector @@ to_tsquery('english',$2) ORDER BY ts_rank_cd(c.search_vector,to_tsquery('english',$2)) DESC LIMIT 40`, lex}}
	if overviewQuestion(q) {
		queries = append(queries, struct {
			sql string
			arg any
		}{`SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),p.scan_page_index+1,p.image_url FROM chunks c JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id JOIN chunk_embeddings ce ON ce.chunk_id=c.id AND ce.embedding_config_id=ir.embedding_config_id WHERE ir.embedding_config_id=$1 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready' AND p.page_kind='text' AND p.scan_page_index<=1 AND (s.title ILIKE '%review%' OR s.title ILIKE '%trial%' OR s.title ILIKE '%study%' OR s.title ILIKE '%meta-analys%') AND ($3='' OR (s.source_key LIKE $3||'-%' OR s.author ILIKE '%'||$3||'%' OR s.title ILIKE '%'||$3||'%')) AND (cardinality($4::uuid[])=0 OR s.id=ANY($4::uuid[])) AND c.search_vector @@ to_tsquery('english',$2) ORDER BY ts_rank_cd(c.search_vector,to_tsquery('english',$2)) DESC LIMIT 40`, lex})
		if strings.Contains(strings.ToLower(q), "result") || strings.Contains(strings.ToLower(q), "finding") || strings.Contains(strings.ToLower(q), "conclusion") {
			queries = append(queries, struct {
				sql string
				arg any
			}{`SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),p.scan_page_index+1,p.image_url FROM chunks c JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id JOIN chunk_embeddings ce ON ce.chunk_id=c.id AND ce.embedding_config_id=ir.embedding_config_id WHERE ir.embedding_config_id=$1 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND ir.status='ready' AND p.page_kind='text' AND p.scan_page_index<=1 AND (s.title ILIKE '%review%' OR s.title ILIKE '%trial%' OR s.title ILIKE '%study%' OR s.title ILIKE '%meta-analys%') AND ($3='' OR (s.source_key LIKE $3||'-%' OR s.author ILIKE '%'||$3||'%' OR s.title ILIKE '%'||$3||'%')) AND (cardinality($4::uuid[])=0 OR s.id=ANY($4::uuid[])) AND c.search_vector @@ to_tsquery('english',$2) ORDER BY ts_rank_cd(c.search_vector,to_tsquery('english',$2)) DESC LIMIT 40`, "result | conclusion | effect | placebo"})
		}
	}
	for _, query := range queries {
		rows, err := a.Store.DB.Query(ctx, query.sql, config, query.arg, scope, selected)
		if err != nil {
			return nil, err
		}
		rank := 0
		for rows.Next() {
			var h hit
			if err = rows.Scan(&h.ID, &h.SourceID, &h.Text, &h.Title, &h.Author, &h.Page, &h.Scan, &h.ImageURL); err != nil {
				rows.Close()
				return nil, err
			}
			rank++
			if all[h.ID] == nil {
				all[h.ID] = &h
			}
			all[h.ID].Score += 1 / float64(60+rank)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	out := make([]hit, 0, len(all))
	for _, h := range all {
		// Overview questions need the paper's abstract and opening results.
		// Otherwise long bibliographies and tables can outrank those pages.
		if overviewQuestion(q) && researchPaperTitle(h.Title) && h.Scan <= 2 {
			if h.Scan == 1 {
				h.Score += 0.012
			} else {
				h.Score += 0.008
			}
		}
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].Score > out[j].Score
	})
	chosenHits := make([]hit, 0, 24)
	perSource := map[uuid.UUID]int{}
	for _, h := range out {
		if perSource[h.SourceID] >= 12 {
			continue
		}
		chosenHits = append(chosenHits, h)
		perSource[h.SourceID]++
		if len(chosenHits) == 24 {
			break
		}
	}
	return chosenHits, nil
}

func overviewQuestion(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "study") || strings.Contains(q, "review") || strings.Contains(q, "result") || strings.Contains(q, "finding") || strings.Contains(q, "conclusion") || strings.Contains(q, "included")
}

func researchPaperTitle(title string) bool {
	title = strings.ToLower(title)
	return strings.Contains(title, "review") || strings.Contains(title, "trial") || strings.Contains(title, "study") || strings.Contains(title, "meta-analysis") || strings.Contains(title, "meta-analyses")
}

func (a *API) verifyEvidence(ctx context.Context, hits []hit) error {
	for _, h := range hits {
		var exact, raw, storedSHA string
		var start, end int
		err := a.Store.DB.QueryRow(ctx, `SELECT c.text_exact,p.text_raw,p.text_sha256,c.start_character,c.end_character FROM chunks c JOIN pages p ON p.id=c.page_id JOIN sources s ON s.id=c.source_id WHERE c.id=$1 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND p.page_kind='text'`, h.ID).Scan(&exact, &raw, &storedSHA, &start, &end)
		if err != nil {
			return err
		}
		runes := []rune(raw)
		if start < 0 || end > len(runes) || end <= start || string(runes[start:end]) != exact || exact != h.Text {
			return fmt.Errorf("passage offsets differ from page")
		}
		digest := sha256.Sum256([]byte(raw))
		if hex.EncodeToString(digest[:]) != storedSHA {
			return fmt.Errorf("page checksum differs")
		}
	}
	return nil
}
