package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// A section contains only sentences that have already passed citation and
// passage checks. The model may arrange them, but cannot add answer prose.
type researchSection struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type sectionGroup struct {
	Title   string `json:"title"`
	Indices []int  `json:"indices"`
}

func sectionTitleOK(title string) bool {
	if title == "" || len([]rune(title)) > 72 || strings.ContainsAny(title, "\n\r[]{}:;.!?") {
		return false
	}
	for _, r := range title {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsSpace(r) && r != '-' && r != '/' && r != '&' && r != ',' && r != '\'' {
			return false
		}
	}
	return true
}

func sectionsFromGroups(sentences []string, groups []sectionGroup) ([]researchSection, bool) {
	if len(groups) < 2 || len(groups) > 4 {
		return nil, false
	}
	seen := make([]bool, len(sentences))
	sections := make([]researchSection, 0, len(groups))
	for _, group := range groups {
		group.Title = strings.TrimSpace(group.Title)
		if !sectionTitleOK(group.Title) || len(group.Indices) == 0 {
			return nil, false
		}
		parts := make([]string, 0, len(group.Indices))
		for _, index := range group.Indices {
			if index < 1 || index > len(sentences) || seen[index-1] {
				return nil, false
			}
			seen[index-1] = true
			parts = append(parts, strings.TrimSpace(sentences[index-1]))
		}
		sections = append(sections, researchSection{Title: group.Title, Body: strings.Join(parts, " ")})
	}
	for _, included := range seen {
		if !included {
			return nil, false
		}
	}
	return sections, true
}

func sourceSections(sentences []string, hits []hit) []researchSection {
	order := []string{}
	grouped := map[string][]string{}
	for _, sentence := range sentences {
		match := citePattern.FindStringSubmatch(sentence)
		if match == nil {
			return []researchSection{{Title: "Findings in the books", Body: strings.Join(sentences, " ")}}
		}
		var index int
		if _, err := fmt.Sscanf(match[1], "%d", &index); err != nil || index < 1 || index > len(hits) {
			return []researchSection{{Title: "Findings in the books", Body: strings.Join(sentences, " ")}}
		}
		author := hits[index-1].Author
		if _, exists := grouped[author]; !exists {
			order = append(order, author)
		}
		grouped[author] = append(grouped[author], strings.TrimSpace(sentence))
	}
	sections := make([]researchSection, 0, len(order))
	for _, author := range order {
		name := strings.Split(author, ",")[0]
		fields := strings.Fields(name)
		if len(fields) > 0 {
			name = fields[len(fields)-1]
		}
		sections = append(sections, researchSection{Title: "Findings from " + name, Body: strings.Join(grouped[author], " ")})
	}
	return sections
}

func organizeResearchSections(ctx context.Context, question, lead, findings string, hits []hit, chat func(context.Context, string, string) (string, error)) ([]researchSection, error) {
	sections := []researchSection{}
	if lead != "" {
		sections = append(sections, researchSection{Title: "Research overview", Body: lead})
	}
	sentences := splitAnswerSentences(findings)
	if len(sentences) < 4 {
		sections = append(sections, researchSection{Title: "Findings in the books", Body: findings})
		return sections, nil
	}
	var numbered strings.Builder
	for i, sentence := range sentences {
		fmt.Fprintf(&numbered, "%d. %s\n", i+1, strings.TrimSpace(sentence))
	}
	raw, err := chat(ctx, "Organize these already verified research findings into 2 to 4 short thematic sections that match the user's question. Return only JSON {\"groups\":[{\"title\":\"short descriptive noun phrase\",\"indices\":[1,2]}]}. Every numbered finding must appear exactly once. Titles should name a theme present in those findings, without making new factual, clinical, or comparative claims. Do not write or rewrite answer sentences. Do not use a fixed outline across different questions.", "Question: "+question+"\nVerified findings:\n"+numbered.String())
	if err != nil {
		return nil, err
	}
	var plan struct {
		Groups []sectionGroup `json:"groups"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &plan) == nil {
		if grouped, ok := sectionsFromGroups(sentences, plan.Groups); ok {
			return append(sections, grouped...), nil
		}
	}
	// An unusable outline must never remove, repeat, or add a verified claim.
	return append(sections, sourceSections(sentences, hits)...), nil
}
