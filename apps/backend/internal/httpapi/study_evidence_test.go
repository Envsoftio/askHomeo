package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestStudyProtocolComparisonOnlyForRelevantQuestions(t *testing.T) {
	if !studyProtocolComparison("Which paper reports a completed trial, and which is a protocol? Describe the primary outcome and reported result.") {
		t.Fatal("missed trial/protocol comparison")
	}
	for _, q := range []string{"What does the Organon say about a protocol?", "What were the trial results?"} {
		if studyProtocolComparison(q) {
			t.Fatalf("applied comparison path to %q", q)
		}
	}
	if got := studyAnchorKinds("Protocol for a randomized trial"); len(got) != 1 || got[0] != "not_started" {
		t.Fatalf("protocol anchors = %v", got)
	}
}

func TestStudyComparisonDraftPrioritizesObservedOutcome(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "The primary outcome was duration of oxygen support. Oxygen support was required for 9.84 and 14.92 days in the two arms."}}
	question := "Which trial and protocol report the primary outcome and results?"
	chat := func(_ context.Context, system, _ string) (string, error) {
		if !strings.Contains(system, "observed numerical result") || !strings.Contains(system, "Attribute any efficacy interpretation to the authors") || !strings.Contains(system, "at most 7 claims") {
			t.Fatalf("study coverage instructions missing: %s", system)
		}
		return `{"claims":[{"text":"The primary outcome was duration of oxygen support.","supports":[{"id":"E1","quote":"primary outcome was duration of oxygen support"}]}]}`, nil
	}
	answer, _, _, err := buildEvidenceFirstDraft(context.Background(), question, "quick", "[E1]", hits, chat)
	if err != nil || !strings.Contains(answer, "duration of oxygen support") {
		t.Fatalf("draft=%q err=%v", answer, err)
	}
}

func TestStudyCoverageFlagsMissingNumericalResult(t *testing.T) {
	question := "Which trial and protocol report the primary outcome and result?"
	hits := []hit{{Title: "Completed trial", Text: "The primary outcome was duration of oxygen support. Oxygen support was required for 9.84 and 14.92 days."}}
	missing, err := auditStudyCoverage(context.Background(), question, "The trial reported efficacy.", hits, func(_ context.Context, system, user string) (string, error) {
		if !strings.Contains(system, "numerical comparison") || !strings.Contains(user, "9.84 and 14.92 days") {
			t.Fatal("coverage check lost numerical evidence")
		}
		return `{"missing":["primary_outcome","observed_result"]}`, nil
	})
	if err != nil || len(missing) != 2 || missing[0] != "primary_outcome" || missing[1] != "observed_result" {
		t.Fatalf("missing=%v err=%v", missing, err)
	}
}

func TestStudyIdentityIsAvailableToClaimCheck(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Title: "Trial of treatment A", Author: "Researcher A", Text: "A randomized trial was conducted with 100 participants."}}
	call := 0
	answer, _, rejected, _, err := verifyClaimSentencesBatched(context.Background(), "Which trial was completed?", "Trial of treatment A reports a completed randomized trial [E1].", hits, func(_ context.Context, _, user string) (string, error) {
		call++
		if !strings.Contains(user, "Source bibliographic identity") || !strings.Contains(user, "Trial of treatment A") {
			t.Fatal("claim check did not receive stored source identity")
		}
		if call == 1 {
			return `{"checks":[{"claim":1,"quotes":[{"id":"E1","text":"A randomized trial was conducted with 100 participants"}]}]}`, nil
		}
		return `{"checks":[{"claim":1,"supported":true,"relevant":true}]}`, nil
	})
	if err != nil || rejected != 0 || call != 2 || !strings.Contains(answer, "Trial of treatment A") {
		t.Fatalf("answer=%q rejected=%d calls=%d err=%v", answer, rejected, call, err)
	}
}
