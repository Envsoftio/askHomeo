package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// A comparison of a finished trial with a protocol requires different kinds
// of evidence. General semantic rank alone can favor methods and discussion
// paragraphs over the observed outcome or explicit non-initiation statement.
func studyProtocolComparison(question string) bool {
	q := strings.ToLower(question)
	return strings.Contains(q, "protocol") && (strings.Contains(q, "trial") || strings.Contains(q, "study")) &&
		(strings.Contains(q, "outcome") || strings.Contains(q, "result"))
}

// The ordinary claim check asks whether each displayed sentence is supported.
// This second check asks whether the answer omitted a requested study fact.
// It is deliberately restricted to trial/protocol comparisons and can trigger
// one more evidence-first draft; the new sentences still pass normal checks.
func auditStudyCoverage(ctx context.Context, question, answer string, hits []hit, chat func(context.Context, string, string) (string, error)) ([]string, error) {
	if !studyProtocolComparison(question) {
		return nil, nil
	}
	var anchors strings.Builder
	for i, h := range hits {
		if i >= 10 {
			break
		}
		fmt.Fprintf(&anchors, "[E%d] %s: %s\n", i+1, h.Title, h.Text)
	}
	raw, err := chat(ctx, "Check coverage, not truth. Compare the user's requested items with the answer and supplied evidence. Return JSON only: {\"missing\":[\"primary_outcome\",\"observed_result\"]}. Allowed missing labels: completed_trial, protocol_identity, treatment_groups, primary_outcome, observed_result, protocol_results. Include a label only when the question requests that item, the supplied evidence supports it, and the answer does not state it. For observed_result, a source with numerical outcomes requires the relevant numerical comparison in the answer; an author's general efficacy conclusion alone is incomplete. A protocol's planned outcomes are not observed results. Return an empty array when all answerable requested items are covered.", "Question: "+question+"\nAnswer: "+answer+"\nEvidence:\n"+anchors.String())
	if err != nil {
		return nil, err
	}
	var response struct {
		Missing []string `json:"missing"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &response); err != nil || response.Missing == nil {
		return nil, fmt.Errorf("answer model returned invalid coverage check")
	}
	allowed := map[string]bool{"completed_trial": true, "protocol_identity": true, "treatment_groups": true, "primary_outcome": true, "observed_result": true, "protocol_results": true}
	seen := map[string]bool{}
	missing := make([]string, 0, len(response.Missing))
	for _, item := range response.Missing {
		if !allowed[item] || seen[item] {
			return nil, fmt.Errorf("answer model returned invalid coverage item")
		}
		seen[item] = true
		missing = append(missing, item)
	}
	return missing, nil
}

func studyAnchorKinds(title string) []string {
	if strings.Contains(strings.ToLower(title), "protocol") {
		return []string{"not_started"}
	}
	return []string{"primary_outcome", "observed_result"}
}

func sourceIdentityContext(h hit) string {
	if h.Title == "" && h.Author == "" {
		return ""
	}
	title, author := []rune(h.Title), []rune(h.Author)
	if len(title) > 350 {
		title = title[:350]
	}
	if len(author) > 200 {
		author = author[:200]
	}
	return fmt.Sprintf("\nSource bibliographic identity: title %q, author %q.\n", string(title), string(author))
}

// Only sources already found by the ordinary search are eligible. The query
// repeats the active-index, rights, publication and category gates used by
// retrieve, so an anchor cannot introduce an unprepared source.
func (a *API) prioritizeStudyAnchors(ctx context.Context, config uuid.UUID, question string, candidates []hit) ([]hit, error) {
	if !studyProtocolComparison(question) {
		return candidates, nil
	}
	papers := make([]hit, 0, len(candidates))
	hasProtocol, hasTrial := false, false
	for _, h := range candidates {
		if !researchPaperTitle(h.Title) {
			continue
		}
		papers = append(papers, h)
		if strings.Contains(strings.ToLower(h.Title), "protocol") {
			hasProtocol = true
		} else {
			hasTrial = true
		}
	}
	if hasProtocol && hasTrial {
		candidates = papers
	}
	const anchorSQL = `SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),coalesce(p.scan_page_index+1,0),p.image_url,cat.categories,p.format,s.evidence_category
FROM chunks c JOIN evidence_locations p ON p.chunk_id=c.id
JOIN sources s ON s.id=c.source_id JOIN evidence_categories cat ON cat.chunk_id=c.id
JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id
JOIN chunk_embeddings ce ON ce.chunk_id=c.id AND ce.embedding_config_id=ir.embedding_config_id
LEFT JOIN document_blocks b ON b.id=c.document_block_id
WHERE s.id=$1 AND ir.embedding_config_id=$2 AND ir.status='ready' AND s.status='published'
AND s.rights_status='allowed' AND s.superseded_at IS NULL AND collection_source_retrieval_eligible(s.id)
AND p.page_kind='text' AND (cardinality($3::text[])=0 OR cat.categories && $3::text[])
AND (($4='primary_outcome' AND c.text_exact ILIKE '%primary outcome%')
 OR ($4='observed_result' AND c.text_exact ~* '([0-9]+[.][0-9]+.{0,100}(days|weeks|months|%|mm|points)|p[[:space:]]*[<=>]|confidence interval|95%[[:space:]]*ci)')
 OR ($4='not_started' AND (c.text_exact ILIKE '%not initiated%' OR c.text_exact ILIKE '%not started%' OR c.text_exact ILIKE '%no observed results%')))
ORDER BY CASE WHEN b.block_index IS NULL THEN 1 ELSE 0 END,b.block_index,c.chunk_index LIMIT 1`
	seenSources := map[uuid.UUID]bool{}
	seenHits := map[uuid.UUID]bool{}
	anchors := make([]hit, 0, 8)
	for _, candidate := range candidates {
		if len(seenSources) >= 4 {
			break
		}
		if seenSources[candidate.SourceID] || !researchPaperTitle(candidate.Title) {
			continue
		}
		seenSources[candidate.SourceID] = true
		for _, kind := range studyAnchorKinds(candidate.Title) {
			var h hit
			err := a.Store.DB.QueryRow(ctx, anchorSQL, candidate.SourceID, config, selectedCategoriesFromContext(ctx), kind).Scan(&h.ID, &h.SourceID, &h.Text, &h.Title, &h.Author, &h.Page, &h.Scan, &h.ImageURL, &h.Categories, &h.Format, &h.EvidenceCategory)
			if errors.Is(err, pgx.ErrNoRows) {
				// A source may not contain this kind of passage. Other kinds and
				// ordinary retrieval remain available.
				continue
			}
			if err != nil {
				return nil, err
			}
			if !seenHits[h.ID] {
				seenHits[h.ID] = true
				anchors = append(anchors, h)
			}
		}
	}
	if len(anchors) == 0 {
		return candidates, nil
	}
	// Anchors receive the first slots. The ordinary ranked passages fill the
	// rest without changing the source-balancing and citation checks.
	for _, h := range candidates {
		if !seenHits[h.ID] {
			anchors = append(anchors, h)
		}
	}
	return anchors, nil
}
