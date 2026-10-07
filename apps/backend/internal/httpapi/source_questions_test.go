package httpapi

import "testing"

func TestExpandedTermFromExactPassage(t *testing.T) {
	passage := "The risk of bias for each MA was assessed by the ROBIS (Risk Of Bias In Systematic reviews) tool."
	if got := expandedTerm(passage, "robis"); got != "Risk Of Bias In Systematic reviews" {
		t.Fatalf("unexpected expansion: %q", got)
	}
	if got := expandedTerm("ROBIS assessments of individual items and domains", "robis"); got != "" {
		t.Fatalf("invented expansion: %q", got)
	}
}

func TestDefinitionQuestionPhrasings(t *testing.T) {
	for _, question := range []string{"What is the ROBIS?", "What is the the robis?", "What does ROBIS stand for?", "What is the ROBIS tool?"} {
		match := definitionQuestion.FindStringSubmatch(question)
		if len(match) < 2 || match[1] != "ROBIS" && match[1] != "robis" {
			t.Fatalf("definition question %q: %v", question, match)
		}
	}
}

func TestSourceQuestionMatchesTitleOrAuthorWithBookIntent(t *testing.T) {
	nash := readySource{Title: "Leaders in Homoeopathic Therapeutics", Author: "E. B. Nash"}
	if !sourceMatchesQuestion("What is Leaders in Homoeopathic Therapeutics about?", nash) {
		t.Fatal("full title should match")
	}
	if !sourceMatchesQuestion("Tell me about Nash's book", nash) {
		t.Fatal("named book should match")
	}
	if sourceMatchesQuestion("What does Nash say about Aconite?", nash) {
		t.Fatal("topic question should use passage research")
	}
}

func TestSourceOverviewIntentDoesNotCatchBookContentQuestions(t *testing.T) {
	if !sourceOverviewQuestion.MatchString("Tell me about Nash's book") {
		t.Fatal("book overview should be recognized")
	}
	if sourceOverviewQuestion.MatchString("What does Nash's book say about Aconite?") {
		t.Fatal("book content question should use passage research")
	}
}
