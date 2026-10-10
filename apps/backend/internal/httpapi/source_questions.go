package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var definitionQuestion = regexp.MustCompile(`(?i)^\s*(?:(?:what is|define|explain)\s+(?:the\s+)*|what does\s+)([a-z][a-z0-9-]{2,30})(?:\s+(?:mean|stand for|tool))?\s*\??\s*$`)
var sourceQuestion = regexp.MustCompile(`(?i)\b(book|source|paper|article|work)\b`)
var sourceOverviewQuestion = regexp.MustCompile(`(?i)^\s*(?:what is|what's|who wrote|who authored|who is the author of|tell me about|summari[sz]e|describe|give me an overview of)\b`)
var sourceAuthorQuestion = regexp.MustCompile(`(?i)^\s*(?:who wrote|who authored|who is the author of)\b`)
var nonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

type readySource struct {
	ID          uuid.UUID
	Title       string
	Author      string
	Edition     string
	Publication string
	Repository  string
}
type sourceFilterKey struct{}
type categoryFilterKey struct{}

func selectedSourcesFromContext(ctx context.Context) []uuid.UUID {
	selected, _ := ctx.Value(sourceFilterKey{}).([]uuid.UUID)
	return selected
}
func selectedCategoriesFromContext(ctx context.Context) []string {
	values, _ := ctx.Value(categoryFilterKey{}).([]string)
	if values == nil {
		return []string{}
	}
	return values
}

func (a *API) answerSourceQuestion(w http.ResponseWriter, r *http.Request, question, mode string, runID, configID uuid.UUID) bool {
	if a.answerPersonRelation(w, r, question, mode, runID, configID) {
		return true
	}
	if a.answerPersonQuestion(w, r, question, mode, runID, configID) {
		return true
	}
	if match := definitionQuestion.FindStringSubmatch(question); len(match) > 1 {
		term := match[1]
		if len(term) >= 4 {
			h, expansion, err := a.findDefinition(r.Context(), term, configID)
			if err != nil {
				fail(w, 500, "Could not check source definitions.")
				return true
			}
			if expansion != "" {
				answer := fmt.Sprintf("%s stands for %s; the authors of %s used this tool to assess risk of bias in each meta-analysis [E1].", strings.ToUpper(term), expansion, h.Title)
				a.saveFocusedAnswer(w, r, question, mode, answer, h, "all", configID)
				return true
			}
		}
	}
	if !sourceOverviewQuestion.MatchString(question) {
		return false
	}
	source, found, err := a.matchReadySource(r.Context(), question, configID)
	if err != nil {
		fail(w, 500, "Could not inspect prepared sources.")
		return true
	}
	if !found {
		return false
	}
	hits, err := a.openingEvidence(r.Context(), source.ID)
	if err != nil {
		fail(w, 500, "Could not read this source's opening pages.")
		return true
	}
	if len(hits) == 0 {
		return false
	}
	if sourceAuthorQuestion.MatchString(question) {
		for _, candidate := range hits {
			if authorLineSupported(source.Author, candidate.Text) {
				a.saveFocusedAnswer(w, r, question, mode, fmt.Sprintf("The author line lists %s [E1].", source.Author), candidate, source.Title, configID)
				return true
			}
		}
		return false
	}
	h := hits[0]
	answer := fmt.Sprintf("%s is a prepared source by %s", source.Title, source.Author)
	if source.Edition != "" {
		answer += ", " + source.Edition
	}
	if source.Publication != "" {
		answer += " (" + source.Publication + ")"
	}
	answer += ". Its opening pages are available to inspect [E1]. Ask about a specific topic in the work for a page-supported explanation."
	if a.Model != nil {
		raw, chatErr := a.Model.Chat(r.Context(), "Summarize what this historical source is about in two short sentences using only its bibliographic record and opening passage. Attribute claims to the source, do not assert clinical efficacy, and cite every sentence inline with [E1]. Return only JSON {\"answer\":\"...\",\"citations\":[\"E1\"]}.", "Question: "+question+"\nTitle: "+source.Title+"\nAuthor: "+source.Author+"\nPublication: "+source.Publication+"\nOpening passage [E1], scan "+fmt.Sprint(h.Scan)+":\n"+h.Text)
		if chatErr == nil {
			var draft struct {
				Answer    string   `json:"answer"`
				Citations []string `json:"citations"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(raw)), &draft) == nil {
				draft.Answer = normalizeDraftCitations(draft.Answer, draft.Citations)
				if used, valid := validateDraft(draft.Answer, draft.Citations, []hit{h}); valid && len(used) > 0 {
					checked, _, rejected, checkErr := verifyClaimSentences(r.Context(), question, draft.Answer, []hit{h}, a.Model.Chat)
					if checkErr == nil && rejected == 0 && strings.TrimSpace(checked) != "" {
						answer = checked
						a.saveFocusedAnswer(w, r, question, mode, answer, h, source.Title, configID)
						return true
					}
				}
			}
		}
	}
	a.saveFocusedAnswer(w, r, question, mode, answer, h, source.Title, configID)
	return true
}

func (a *API) findDefinition(ctx context.Context, term string, configID uuid.UUID) (hit, string, error) {
	rows, err := a.Store.DB.Query(ctx, `SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),coalesce(p.scan_page_index+1,0),p.image_url
FROM chunks c JOIN evidence_locations p ON p.chunk_id=c.id JOIN sources s ON s.id=c.source_id
JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id
JOIN chunk_embeddings ce ON ce.chunk_id=c.id AND ce.embedding_config_id=ir.embedding_config_id
WHERE ir.status='ready' AND ir.embedding_config_id=$2 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND collection_source_retrieval_eligible(s.id) AND p.page_kind='text'
 AND (cardinality($3::uuid[])=0 OR s.id=ANY($3::uuid[])) AND strpos(lower(c.text_exact),lower($1))>0 ORDER BY p.scan_page_index LIMIT 100`, term, configID, selectedSourcesFromContext(ctx))
	if err != nil {
		return hit{}, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.ID, &h.SourceID, &h.Text, &h.Title, &h.Author, &h.Page, &h.Scan, &h.ImageURL); err != nil {
			return hit{}, "", err
		}
		if expansion := expandedTerm(h.Text, term); expansion != "" {
			return h, expansion, nil
		}
	}
	return hit{}, "", rows.Err()
}

func expandedTerm(passage, term string) string {
	quoted := regexp.QuoteMeta(term)
	patterns := []string{
		`(?i)\b` + quoted + `\b\s*(?:tool\s*)?\(([^()\n]{5,110})\)`,
		`(?i)([A-Za-z][A-Za-z -]{5,110})\s*\(` + quoted + `\)`,
	}
	for _, pattern := range patterns {
		match := regexp.MustCompile(pattern).FindStringSubmatch(passage)
		if len(match) > 1 {
			value := strings.Trim(strings.Join(strings.Fields(match[1]), " "), " .,;:")
			if len(strings.Fields(value)) >= 2 && len(value) <= 110 {
				return value
			}
		}
	}
	return ""
}

func normalizeSourceName(value string) string {
	return strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(value), " "))
}

func sourceMatchesQuestion(question string, source readySource) bool {
	q := normalizeSourceName(question)
	title := strings.TrimPrefix(normalizeSourceName(source.Title), "a ")
	title = strings.TrimPrefix(title, "the ")
	if len(strings.Fields(title)) >= 2 && strings.Contains(" "+q+" ", " "+title+" ") {
		return true
	}
	if sourceAuthorQuestion.MatchString(question) {
		matched, distinctive := 0, false
		for _, word := range strings.Fields(title) {
			if len(word) < 5 || !strings.Contains(" "+q+" ", " "+word+" ") {
				continue
			}
			matched++
			if len(word) >= 8 {
				distinctive = true
			}
		}
		if matched >= 3 && distinctive {
			return true
		}
	}
	if !sourceQuestion.MatchString(question) {
		return false
	}
	name := strings.TrimSpace(source.Author)
	if i := strings.Index(name, ","); i >= 0 {
		name = name[:i]
	} else if parts := strings.Fields(name); len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	name = normalizeSourceName(name)
	return len(name) >= 4 && strings.Contains(" "+q+" ", " "+name+" ")
}

func authorLineSupported(author, passage string) bool {
	parts := strings.Split(author, ",")
	if len(parts) == 1 {
		parts = strings.Split(author, " and ")
	}
	for _, part := range parts {
		words := strings.Fields(normalizeSourceName(part))
		if len(words) == 0 || !strings.Contains(" "+normalizeSourceName(passage)+" ", " "+words[len(words)-1]+" ") {
			return false
		}
	}
	return true
}

func (a *API) matchReadySource(ctx context.Context, question string, configID uuid.UUID) (readySource, bool, error) {
	rows, err := a.Store.DB.Query(ctx, `SELECT s.id,s.title,s.author,s.edition,s.publication_info,s.repository FROM sources s JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id WHERE ir.status='ready' AND ir.embedding_config_id=$1 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND collection_source_retrieval_eligible(s.id) AND (cardinality($2::uuid[])=0 OR s.id=ANY($2::uuid[])) ORDER BY s.created_at`, configID, selectedSourcesFromContext(ctx))
	if err != nil {
		return readySource{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var source readySource
		if err := rows.Scan(&source.ID, &source.Title, &source.Author, &source.Edition, &source.Publication, &source.Repository); err != nil {
			return readySource{}, false, err
		}
		if sourceMatchesQuestion(question, source) {
			return source, true, nil
		}
	}
	return readySource{}, false, rows.Err()
}

func (a *API) openingEvidence(ctx context.Context, sourceID uuid.UUID) ([]hit, error) {
	rows, err := a.Store.DB.Query(ctx, `SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),coalesce(p.scan_page_index+1,0),p.image_url
FROM chunks c JOIN evidence_locations p ON p.chunk_id=c.id JOIN sources s ON s.id=c.source_id WHERE c.source_id=$1 AND p.page_kind='text'
ORDER BY CASE WHEN p.scan_page_index<30 AND (left(p.text_raw,160) ~* '(preface|introduction|abstract)') THEN 0 ELSE 1 END,p.scan_page_index,c.start_character LIMIT 3`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []hit
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.ID, &h.SourceID, &h.Text, &h.Title, &h.Author, &h.Page, &h.Scan, &h.ImageURL); err != nil {
			return nil, err
		}
		h.Score = 1
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func (a *API) saveFocusedAnswer(w http.ResponseWriter, r *http.Request, question, mode, answer string, h hit, scope string, configID uuid.UUID) {
	a.saveFocusedAnswerHits(w, r, question, mode, answer, []hit{h}, scope, configID)
}

func (a *API) saveFocusedAnswerHits(w http.ResponseWriter, r *http.Request, question, mode, answer string, hits []hit, scope string, configID uuid.UUID) {
	if len(hits) == 0 || len(hits) > 10 {
		fail(w, 500, "Invalid source answer evidence.")
		return
	}
	if err := a.verifyEvidence(r.Context(), hits); err != nil {
		fail(w, 409, "The selected passage no longer matches its original page.")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not save source answer.")
		return
	}
	defer tx.Rollback(r.Context())
	answerID := uuid.New()
	ids := make([]uuid.UUID, 0, len(hits))
	runs := make(map[uuid.UUID]uuid.UUID)
	for _, h := range hits {
		ids = append(ids, h.ID)
		if _, seen := runs[h.SourceID]; seen {
			continue
		}
		var activeRun uuid.UUID
		if err = tx.QueryRow(r.Context(), `SELECT ir.id FROM active_indexes ai JOIN index_runs ir ON ir.id=ai.index_run_id JOIN sources s ON s.id=ai.source_id WHERE s.id=$1 AND ir.embedding_config_id=$2 AND ir.status='ready' AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND collection_source_retrieval_eligible(s.id)`, h.SourceID, configID).Scan(&activeRun); err != nil {
			fail(w, 409, "This source is no longer prepared for questions.")
			return
		}
		runs[h.SourceID] = activeRun
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO answers(id,question,status,answer_text,answer_model,answer_revision,index_run_id,evidence_ids,research_mode,answer_job_id,prompt_revision,owner_role,owner_principal_id) VALUES($1,$2,'answered',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, answerID, question, answer, "source-answer", "source-answer-v1", runs[hits[0].SourceID], ids, mode, jobIDFromContext(r.Context()), "source-identity-v1", answerOwner(r.Context()), requestPrincipal(r.Context()).ID); err != nil {
		fail(w, 500, "Could not save source answer.")
		return
	}
	selected := selectedSourcesFromContext(r.Context())
	for i, sourceID := range selected {
		if _, err = tx.Exec(r.Context(), `INSERT INTO answer_source_filters(answer_id,source_id,ordinal) VALUES($1,$2,$3)`, answerID, sourceID, i+1); err != nil {
			fail(w, 500, "Could not save source selection.")
			return
		}
	}
	for _, runID := range runs {
		if _, err = tx.Exec(r.Context(), `INSERT INTO answer_index_runs(answer_id,index_run_id) VALUES($1,$2)`, answerID, runID); err != nil {
			fail(w, 500, "Could not save source answer.")
			return
		}
	}
	citations := make([]any, 0, len(hits))
	for i, h := range hits {
		label := fmt.Sprintf("E%d", i+1)
		citationID := uuid.New()
		if _, err = tx.Exec(r.Context(), `INSERT INTO answer_citations(id,answer_id,chunk_id,ordinal,evidence_label) VALUES($1,$2,$3,$4,$5)`, citationID, answerID, h.ID, i+1, label); err != nil {
			fail(w, 500, "Could not save source citation.")
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO answer_candidates(answer_id,ordinal,chunk_id,retrieval_score) VALUES($1,$2,$3,1)`, answerID, i+1, h.ID); err != nil {
			fail(w, 500, "Could not save source evidence.")
			return
		}
		citations = append(citations, map[string]any{"id": citationID, "label": label, "title": h.Title, "author": h.Author, "printed_page": h.Page, "scan_position": h.Scan})
	}
	trace := searchTrace{Query: question, SourceScope: scope, CandidateCount: len(hits)}
	if _, err = tx.Exec(r.Context(), `INSERT INTO answer_searches(answer_id,ordinal,query,source_scope,candidate_count) VALUES($1,1,$2,$3,$4)`, answerID, trace.Query, trace.SourceScope, trace.CandidateCount); err != nil {
		fail(w, 500, "Could not save source search.")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 500, "Could not save source answer.")
		return
	}
	write(w, 200, map[string]any{"answer_id": answerID, "status": "answered", "answer": answer, "answer_model": "source-answer", "research_mode": mode, "source_ids": selected, "citations": citations, "searches": []searchTrace{trace}, "evidence": evidenceCards(hits, hits), "sections": []any{}, "omitted_claim_count": 0})
}
