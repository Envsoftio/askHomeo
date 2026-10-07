package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEvidenceFirstDraftRequiresExactSourceQuotes(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "The review was written by Hamre and colleagues and assessed six meta-analyses."}}
	chat := func(_ context.Context, system, user string) (string, error) {
		if !strings.Contains(system, "direct answer") || !strings.Contains(user, "Question: Who wrote the review?") {
			t.Fatal("question and direct-answer instruction were not supplied")
		}
		return `{"claims":[{"text":"Hamre and colleagues wrote the review.","supports":[{"id":"E1","quote":"The review was written by Hamre and colleagues"}]},{"text":"The review included ten meta-analyses.","supports":[{"id":"E1","quote":"ten meta-analyses were included"}]}]}`, nil
	}
	answer, labels, omitted, err := buildEvidenceFirstDraft(context.Background(), "Who wrote the review?", "quick", "[E1]", hits, chat)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Hamre and colleagues wrote the review [E1]." || len(labels) != 1 || labels[0] != "E1" || omitted != 1 {
		t.Fatalf("unexpected evidence draft: %q labels=%v omitted=%d", answer, labels, omitted)
	}
}

func TestEvidenceFirstDraftAcceptsExtraClosingBrace(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "The throat is swollen and the tonsils are enlarged and redder than normal."}}
	chat := func(context.Context, string, string) (string, error) {
		return `{"claims":[{"text":"The tonsils are enlarged and red.","supports":[{"id":"E1","quote":"The throat is swollen and the tonsils are enlarged and redder than normal."}]}]}}`, nil
	}
	answer, labels, omitted, err := buildEvidenceFirstDraft(context.Background(), "What does the source say about the tonsils?", "quick", "[E1]", hits, chat)
	if err != nil || answer != "The tonsils are enlarged and red [E1]." || len(labels) != 1 || labels[0] != "E1" || omitted != 0 {
		t.Fatalf("unexpected repaired draft: %q labels=%v omitted=%d err=%v", answer, labels, omitted, err)
	}
}

func TestEvidenceFirstDraftIgnoresTrailingCommentary(t *testing.T) {
	hits := []hit{{ID: uuid.New(), Text: "The throat is swollen and the tonsils are enlarged and redder than normal."}}
	chat := func(context.Context, string, string) (string, error) {
		return "{\"claims\":[{\"text\":\"The tonsils are enlarged and red.\",\"supports\":[{\"id\":\"E1\",\"quote\":\"The throat is swollen and the tonsils are enlarged and redder than normal.\"}]}]}\n\nAdditional commentary that must not become an answer.", nil
	}
	answer, labels, omitted, err := buildEvidenceFirstDraft(context.Background(), "What does the source say about the tonsils?", "quick", "[E1]", hits, chat)
	if err != nil || answer != "The tonsils are enlarged and red [E1]." || len(labels) != 1 || labels[0] != "E1" || omitted != 0 {
		t.Fatalf("unexpected draft: %q labels=%v omitted=%d err=%v", answer, labels, omitted, err)
	}
}
