package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var relationQuestions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*how (?:is|was|are|were) (.+?) related to (.+?)\s*\??\s*$`),
	regexp.MustCompile(`(?i)^\s*what (?:is|was) the relationship between (.+?) and (.+?)\s*\??\s*$`),
	regexp.MustCompile(`(?i)^\s*are (.+?) and (.+?) related\s*\??\s*$`),
}

func relationNames(question string) (string, string, bool) {
	for _, pattern := range relationQuestions {
		match := pattern.FindStringSubmatch(question)
		if len(match) != 3 {
			continue
		}
		first := strings.TrimSpace(strings.Trim(match[1], `?"'“”‘’ `))
		second := strings.TrimSpace(strings.Trim(match[2], `?"'“”‘’ `))
		first = strings.TrimSpace(personTitle.ReplaceAllString(first, ""))
		second = strings.TrimSpace(personTitle.ReplaceAllString(second, ""))
		if len(strings.Fields(first)) > 4 || len(strings.Fields(second)) > 4 || first == "" || second == "" || normalizeSourceName(first) == normalizeSourceName(second) {
			return "", "", false
		}
		return first, second, true
	}
	return "", "", false
}

func (a *API) answerPersonRelation(w http.ResponseWriter, r *http.Request, question, mode string, runID, configID uuid.UUID) bool {
	first, second, recognized := relationNames(question)
	if !recognized {
		return false
	}
	firstSource, firstFound, err := a.matchReadyAuthor(r.Context(), first, configID)
	if err != nil {
		fail(w, 500, "Could not check prepared source authors.")
		return true
	}
	secondSource, secondFound, err := a.matchReadyAuthor(r.Context(), second, configID)
	if err != nil {
		fail(w, 500, "Could not check prepared source authors.")
		return true
	}
	if !firstFound || !secondFound {
		a.insufficient(w, r.Context(), question, runID, mode, "I cannot verify how "+first+" and "+second+" are related from the prepared sources. Add a reliable source about the people or ask about authors already in this library.", nil)
		return true
	}
	firstWords, secondWords := strings.Fields(normalizeSourceName(first)), strings.Fields(normalizeSourceName(second))
	firstSurname, secondSurname := firstWords[len(firstWords)-1], secondWords[len(secondWords)-1]
	firstHit, firstPage, err := a.authorEvidence(r.Context(), firstSource.ID, firstSurname, configID)
	if err != nil {
		fail(w, 500, "Could not read the first author's source pages.")
		return true
	}
	secondHit, secondPage, err := a.authorEvidence(r.Context(), secondSource.ID, secondSurname, configID)
	if err != nil {
		fail(w, 500, "Could not read the second author's source pages.")
		return true
	}
	if !firstPage || !secondPage {
		a.insufficient(w, r.Context(), question, runID, mode, "I found both author records, but could not verify their identities against indexed pages.", nil)
		return true
	}
	firstName, secondName := authorDisplayName(firstSource.Author, first), authorDisplayName(secondSource.Author, second)
	if firstSource.ID == secondSource.ID {
		answer := fmt.Sprintf("%s and %s are listed as authors of %s [E1]. This page does not establish a personal or family relationship between them [E1].", firstName, secondName, firstSource.Title)
		a.saveFocusedAnswer(w, r, question, mode, answer, firstHit, firstSource.Title, configID)
		return true
	}
	hits := []hit{firstHit, secondHit}
	answer := fmt.Sprintf("%s and %s are authors of separate works in this collection: %s and %s, respectively [E1][E2]. The cited pages do not establish a personal or family relationship [E1][E2].", firstName, secondName, firstSource.Title, secondSource.Title)
	firstCross, foundFirstCross, err := a.crossMentionEvidence(r.Context(), firstSource.ID, secondSurname, configID)
	if err != nil {
		fail(w, 500, "Could not check the authors' cross-references.")
		return true
	}
	secondCross, foundSecondCross, err := a.crossMentionEvidence(r.Context(), secondSource.ID, firstSurname, configID)
	if err != nil {
		fail(w, 500, "Could not check the authors' cross-references.")
		return true
	}
	if foundFirstCross || foundSecondCross {
		from, to, cross := firstName, secondName, firstCross
		if !foundFirstCross {
			from, to, cross = secondName, firstName, secondCross
		}
		if cross.ID != firstHit.ID && cross.ID != secondHit.ID {
			hits = append(hits, cross)
			answer = fmt.Sprintf("The documented connection is through their writings: %s refers to %s by name in %s [E3]. %s and %s authored separate works in this collection [E1][E2]. The cited pages do not establish a personal or family relationship [E1][E2][E3].", from, to, cross.Title, firstName, secondName)
		}
	}
	a.saveFocusedAnswerHits(w, r, question, mode, answer, hits, "all", configID)
	return true
}

func (a *API) crossMentionEvidence(ctx context.Context, sourceID uuid.UUID, otherSurname string, configID uuid.UUID) (hit, bool, error) {
	rows, err := a.Store.DB.Query(ctx, `SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),coalesce(p.scan_page_index+1,0),p.image_url
FROM chunks c JOIN evidence_locations p ON p.chunk_id=c.id JOIN sources s ON s.id=c.source_id
JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id
JOIN chunk_embeddings ce ON ce.chunk_id=c.id AND ce.embedding_config_id=ir.embedding_config_id
WHERE s.id=$1 AND ir.status='ready' AND ir.embedding_config_id=$3 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND collection_source_retrieval_eligible(s.id) AND p.page_kind='text'
 AND strpos(lower(c.text_exact),lower($2))>0 ORDER BY p.scan_page_index,c.start_character LIMIT 80`, sourceID, otherSurname, configID)
	if err != nil {
		return hit{}, false, err
	}
	defer rows.Close()
	wholeName := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(otherSurname) + `\b`)
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.ID, &h.SourceID, &h.Text, &h.Title, &h.Author, &h.Page, &h.Scan, &h.ImageURL); err != nil {
			return hit{}, false, err
		}
		if wholeName.MatchString(h.Text) {
			h.Score = 1
			return h, true, nil
		}
	}
	return hit{}, false, rows.Err()
}
