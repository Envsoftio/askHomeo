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
