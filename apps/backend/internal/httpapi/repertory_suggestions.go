package httpapi

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const repertorySuggestionVersion = "colon-rows-v1"

// Deliberately narrow: an exact heading followed by a comma/semicolon separated
// list of dotted abbreviations. Prose, cross-references and ambiguous notation
// remain manual review. These suggestions establish no membership or grade.
var repertoryAbbreviation = regexp.MustCompile(`^[\p{L}][\p{L}\p{N}-]{0,29}\.$`)

type rubricSuggestion struct {
	Heading   string   `json:"heading"`
	Notations []string `json:"notations"`
	Start     int      `json:"start_character"`
	End       int      `json:"end_character"`
	ExactText string   `json:"exact_text"`
}

func suggestRubricRows(text string) []rubricSuggestion {
	out := []rubricSuggestion{}
	offset := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		start := offset + utf8.RuneCountInString(line[:len(line)-len(strings.TrimLeftFunc(line, unicode.IsSpace))])
		offset += utf8.RuneCountInString(line) + 1
		heading, list, ok := strings.Cut(trimmed, ":")
		heading = strings.TrimSpace(heading)
		if !ok || heading == "" || utf8.RuneCountInString(heading) > 200 || len(strings.Fields(heading)) > 12 || strings.ContainsAny(heading, ".;:") {
			continue
		}
		notations := []string{}
		seen := map[string]bool{}
		valid := true
		for _, part := range strings.Split(strings.ReplaceAll(list, ";", ","), ",") {
			part = strings.TrimSpace(part)
			if !repertoryAbbreviation.MatchString(part) || seen[part] {
				valid = false
				break
			}
			seen[part] = true
			notations = append(notations, part)
		}
		if valid && len(notations) > 0 && len(notations) <= 200 {
			out = append(out, rubricSuggestion{heading, notations, start, start + utf8.RuneCountInString(trimmed), trimmed})
		}
		if len(out) == 50 {
			break
		}
	}
	return out
}
