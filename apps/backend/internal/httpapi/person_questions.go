package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var personQuestion = regexp.MustCompile(`(?i)^\s*(?:who is|who was|who's|tell me about|what do you know about)\s+(.+?)\s*\??\s*$`)
var personTitle = regexp.MustCompile(`(?i)^(?:(?:dr|doctor|professor|prof)\.?\s+)+`)
var personContextSuffix = regexp.MustCompile(`(?i)\s+in\s+(?:homeopathy|homoeopathy|medicine|the medical field)$`)
var singleAuthorWithGivenNames = regexp.MustCompile(`^([^,]+),\s*[A-Z](?:\.\s*[A-Z])?\.\s*\(([^)]+)\)`)
var authorLifeYears = regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})-(1[0-9]{3}|20[0-9]{2})\b`)

func personNameFromQuestion(question string) string {
	match := personQuestion.FindStringSubmatch(question)
	if len(match) < 2 {
		return ""
	}
	name := strings.TrimSpace(strings.Trim(match[1], `?"'“”‘’ `))
	name = strings.TrimSpace(personTitle.ReplaceAllString(name, ""))
	name = strings.TrimSpace(personContextSuffix.ReplaceAllString(name, ""))
	words := strings.Fields(normalizeSourceName(name))
	if len(words) == 0 || len(words) > 4 || strings.Contains(" "+normalizeSourceName(name)+" ", " author of ") || sourceQuestion.MatchString(name) {
		return ""
	}
	return name
}

func authorMatchesPerson(author, person string) bool {
	nameWords := strings.Fields(normalizeSourceName(person))
	if len(nameWords) == 0 {
		return false
	}
	authorWords := strings.Fields(normalizeSourceName(author))
	set := make(map[string]bool, len(authorWords))
	for _, word := range authorWords {
		set[word] = true
	}
	for _, word := range nameWords {
		if !set[word] {
			return false
		}
	}
	return len(nameWords[len(nameWords)-1]) >= 4
}

func authorDisplayName(author, person string) string {
	if match := singleAuthorWithGivenNames.FindStringSubmatch(author); len(match) == 3 {
		return match[2] + " " + match[1]
	}
	if !strings.Contains(author, ",") {
		return author
	}
	return person
}

func physicianOnPage(passage, surname string) bool {
	name := regexp.QuoteMeta(surname)
	return regexp.MustCompile(`(?is)\b` + name + `\b.{0,20}\bm\s*\.?\s*d\.?\b|\bdr\.?\s+[a-z. ]{0,40}\b` + name + `\b`).MatchString(passage)
}

func (a *API) answerPersonQuestion(w http.ResponseWriter, r *http.Request, question, mode string, runID, configID uuid.UUID) bool {
	person := personNameFromQuestion(question)
	if person == "" {
		return false
	}
	source, found, err := a.matchReadyAuthor(r.Context(), person, configID)
	if err != nil {
		fail(w, 500, "Could not check prepared source authors.")
		return true
	}
	if !found {
		a.insufficient(w, r.Context(), question, runID, mode, "I do not have enough verified biographical information about "+person+" in the prepared sources. Try a source author in this library, or add a reliable source about this person.", nil)
		return true
	}
	surname := strings.Fields(normalizeSourceName(person))
	h, found, err := a.authorEvidence(r.Context(), source.ID, surname[len(surname)-1], configID)
	if err != nil {
		fail(w, 500, "Could not read the author's source pages.")
		return true
	}
	if !found {
		a.insufficient(w, r.Context(), question, runID, mode, "The prepared source lists "+person+" as an author, but I could not verify a biographical answer against an indexed page.", nil)
		return true
	}
	name := authorDisplayName(source.Author, person)
	answer := fmt.Sprintf("%s is identified as an author of %s [E1].", name, source.Title)
	if physicianOnPage(h.Text, surname[len(surname)-1]) {
		life := ""
		if years := authorLifeYears.FindStringSubmatch(source.Author); len(years) == 3 && strings.Contains(h.Text, years[1]) && strings.Contains(h.Text, years[2]) {
			life = fmt.Sprintf(" (%s–%s)", years[1], years[2])
		}
		answer = fmt.Sprintf("%s%s was a physician and an author of %s [E1].", name, life, source.Title)
	}
	a.saveFocusedAnswer(w, r, question, mode, answer, h, source.Title, configID)
	return true
}

func (a *API) matchReadyAuthor(ctx context.Context, person string, configID uuid.UUID) (readySource, bool, error) {
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
		if authorMatchesPerson(source.Author, person) {
			return source, true, nil
		}
	}
	return readySource{}, false, rows.Err()
}

func (a *API) authorEvidence(ctx context.Context, sourceID uuid.UUID, surname string, configID uuid.UUID) (hit, bool, error) {
	var h hit
	err := a.Store.DB.QueryRow(ctx, `SELECT c.id,c.source_id,c.text_exact,s.title,s.author,coalesce(p.printed_label,''),coalesce(p.scan_page_index+1,0),p.image_url
FROM chunks c JOIN evidence_locations p ON p.chunk_id=c.id JOIN sources s ON s.id=c.source_id
JOIN active_indexes ai ON ai.source_id=s.id JOIN index_runs ir ON ir.id=ai.index_run_id
JOIN chunk_embeddings ce ON ce.chunk_id=c.id AND ce.embedding_config_id=ir.embedding_config_id
WHERE s.id=$1 AND ir.status='ready' AND ir.embedding_config_id=$3 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND collection_source_retrieval_eligible(s.id) AND p.page_kind='text'
 AND p.scan_page_index<100 AND strpos(lower(c.text_exact),lower($2))>0
ORDER BY CASE WHEN strpos(lower(c.text_exact),'subject of this sketch')>0 THEN 0 ELSE 1 END,p.scan_page_index,c.start_character LIMIT 1`, sourceID, surname, configID).Scan(&h.ID, &h.SourceID, &h.Text, &h.Title, &h.Author, &h.Page, &h.Scan, &h.ImageURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return hit{}, false, nil
		}
		return hit{}, false, err
	}
	h.Score = 1
	return h, true, nil
}
