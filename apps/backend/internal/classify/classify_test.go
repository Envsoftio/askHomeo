package classify

import (
	"reflect"
	"testing"
)

func TestContentControlsSuggestionAndMixedWork(t *testing.T) {
	got := Suggest("Repertory by author: a randomized placebo-controlled trial with confidence interval and trial registration.")
	if !reflect.DeepEqual(got.Categories, []string{"research"}) {
		t.Fatalf("misleading title controlled result: %+v", got)
	}
	mixed := Suggest("Materia medica. Remedy picture and modalities. Repertory appendix with rubrics and grade of remedies.")
	if !reflect.DeepEqual(mixed.Categories, []string{"materia_medica", "repertory"}) {
		t.Fatalf("mixed classification: %+v", mixed)
	}
	unknown := Suggest("A short page with plain text.")
	if unknown.State != "uncertain" || !reflect.DeepEqual(unknown.Categories, []string{"unclassified"}) {
		t.Fatalf("uncertain input: %+v", unknown)
	}
}

func TestConfigurableCueThreshold(t *testing.T) {
	content := "Materia medica with modalities and remedy picture."
	if got := SuggestWithThreshold(content, 3); !reflect.DeepEqual(got.Categories, []string{"materia_medica"}) {
		t.Fatalf("three reviewed cues: %+v", got)
	}
	if got := SuggestWithThreshold(content, 4); got.State != "uncertain" {
		t.Fatalf("threshold should leave material for manual review: %+v", got)
	}
	t.Setenv("LITERATURE_CLASSIFIER_MIN_CUES", "4")
	if got := Suggest(content); got.State != "uncertain" {
		t.Fatalf("configured threshold: %+v", got)
	}
}
