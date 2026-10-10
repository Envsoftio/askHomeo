package classify

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const Version = "literature-rules-v3"

var rubricSeparator = regexp.MustCompile(`\s+(?:--|—|:)\s*`)

type Evidence struct {
	Category string `json:"category"`
	Cue      string `json:"cue"`
}
type Suggestion struct {
	Categories []string   `json:"categories"`
	State      string     `json:"state"`
	Reason     string     `json:"reason"`
	Evidence   []Evidence `json:"evidence"`
}

// Suggest uses extracted content, including headings. Bibliographic labels are
// intentionally excluded from the confidence threshold so a misleading title
// cannot make unrelated prose a repertory or a clinical study.
func Suggest(content string) Suggestion {
	threshold := 2
	if configured, err := strconv.Atoi(os.Getenv("LITERATURE_CLASSIFIER_MIN_CUES")); err == nil && configured >= 1 && configured <= 7 {
		threshold = configured
	}
	return SuggestWithThreshold(content, threshold)
}

// SuggestWithThreshold allows the cue threshold to be checked against reviewed
// examples before changing it for a corpus. It is not a probability estimate.
func SuggestWithThreshold(content string, threshold int) Suggestion {
	if threshold < 1 {
		threshold = 2
	}
	text := strings.ToLower(content)
	rules := map[string][]string{
		"materia_medica":     {"materia medica", "modalities", "aggravation", "amelioration", "mental symptoms", "remedy picture", "leading symptoms"},
		"repertory":          {"repertory", "rubric", "rubrics", "remedies under", "grade of remedies", "mind; fear", "generalities;"},
		"organon_philosophy": {"aphorism", "organon", "vital force", "similia similibus", "philosophy of homoeopathy"},
		"therapeutics":       {"therapeutics", "indications for", "treatment of", "clinical indications", "differential diagnosis"},
		"provings":           {"proving", "prover", "pathogenetic", "healthy volunteers", "drug proving"},
		"clinical_cases":     {"case report", "case history", "patient presented", "follow-up visit", "clinical case"},
		"research":           {"randomized", "randomised", "placebo-controlled", "systematic review", "meta-analysis", "confidence interval", "trial registration"},
	}
	scores := map[string]int{}
	evidence := []Evidence{}
	for category, cues := range rules {
		for _, cue := range cues {
			if strings.Contains(text, cue) {
				scores[category]++
				evidence = append(evidence, Evidence{category, cue})
			}
		}
	}
	// Layout-independent signals complement vocabulary. A title alone is not
	// enough: repeated label/list rows or numbered case narratives are required.
	rubricRows := 0
	for _, row := range strings.Split(content, "\n") {
		// Legacy repertories commonly use "rubric : remedy, remedy" rather
		// than a dash. Require several list rows before trusting the shape.
		parts := rubricSeparator.Split(strings.TrimSpace(row), 2)
		if len(parts) != 2 || len(parts[0]) < 3 || len(parts[0]) > 120 {
			continue
		}
		items := strings.Split(parts[1], ",")
		shortNames := 0
		for _, item := range items {
			item = strings.TrimSpace(item)
			if len(item) > 0 && len(item) <= 28 && len(strings.Fields(item)) <= 4 && strings.Contains(item, ".") {
				shortNames++
			}
		}
		if shortNames >= 3 {
			rubricRows++
		}
	}
	if rubricRows >= 3 {
		scores["repertory"] += 2
		evidence = append(evidence, Evidence{"repertory", "repeated labeled rows with abbreviated remedy lists"}, Evidence{"repertory", "multiple distinct list entries in document content"})
		// Modalities occur in repertories too; those shared terms alone do not
		// establish a separate materia medica section.
		if !strings.Contains(text, "materia medica") && !strings.Contains(text, "remedy picture") && !strings.Contains(text, "leading symptoms") {
			delete(scores, "materia_medica")
		}
	}
	if len(regexp.MustCompile(`(?i)\bcase\s+[0-9]+\b`).FindAllString(content, -1)) >= 2 {
		scores["clinical_cases"]++
		evidence = append(evidence, Evidence{"clinical_cases", "multiple numbered cases"})
		narrative := 0
		for _, cue := range []string{"patient", "called to", "prescribed", "recovery", "aged", "suffering"} {
			if strings.Contains(text, cue) {
				narrative++
			}
		}
		if narrative >= 2 {
			scores["clinical_cases"]++
			evidence = append(evidence, Evidence{"clinical_cases", "patient and treatment narrative within numbered cases"})
		}
	}
	chosen := []string{}
	for category, score := range scores {
		if score >= threshold {
			chosen = append(chosen, category)
		}
	}
	sort.Strings(chosen)
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].Category == evidence[j].Category {
			return evidence[i].Cue < evidence[j].Cue
		}
		return evidence[i].Category < evidence[j].Category
	})
	if len(chosen) == 0 {
		return Suggestion{[]string{"unclassified"}, "uncertain", "Could not confidently identify literature type from extracted content; select a category.", evidence}
	}
	reasons := []string{}
	for _, category := range chosen {
		cues := []string{}
		for _, hit := range evidence {
			if hit.Category == category {
				cues = append(cues, hit.Cue)
			}
		}
		reasons = append(reasons, category+": "+strings.Join(cues, ", "))
	}
	return Suggestion{chosen, "suggested", strings.Join(reasons, "; "), evidence}
}
