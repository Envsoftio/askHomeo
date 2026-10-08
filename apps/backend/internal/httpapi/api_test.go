package httpapi

import (
	"strings"
	"testing"
)

func TestQuestionTermsDoNotSpecialCaseSourceOrTopic(t *testing.T) {
	got := strings.Join(queryTerms("Which remedies does Nash compare with Carbo vegetabilis on the inspected page?"), " | ")
	if got != "remedies | nash | compare | carbo | vegetabilis" {
		t.Fatalf("unexpected search terms: %s", got)
	}
	if got := queryTerms("restlessness"); len(got) != 1 || got[0] != "restlessness" {
		t.Fatalf("single user term was dropped: %v", got)
	}
}

func TestQuestionTermsKeepLaterResearchTopics(t *testing.T) {
	got := strings.Join(queryTerms("In the selected study, what condition and homeopathic intervention were tested, how was the comparison group treated, and what were the main results? Cite the pages supporting each point and note any limitations the authors report."), " | ")
	if !strings.Contains(got, "results") || !strings.Contains(got, "limitations") || strings.Contains(got, "selected") || strings.Contains(got, "cite") {
		t.Fatalf("research terms lost or cluttered: %s", got)
	}
}
