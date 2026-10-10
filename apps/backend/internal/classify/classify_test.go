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

func TestGenericStructuralClassification(t *testing.T) {
	repertory := Suggest("Cold -- Abc., Def., Ghi.\nHeat -- Jkl., Mno., Pqr.\nMotion -- Stu., Vwx., Yza.\nAggravation and amelioration are modalities.")
	if !reflect.DeepEqual(repertory.Categories, []string{"repertory"}) {
		t.Fatalf("missed structure or confused modalities: %+v", repertory)
	}
	colonRows := Suggest("ABRUPT : Nat-m., tarent., Bell.\nABSENT-MINDED : Acon., act-sp., agar., alum.\nMorning : Guai., nat-c., ph-ac., phos.")
	if !reflect.DeepEqual(colonRows.Categories, []string{"repertory"}) {
		t.Fatalf("missed colon-delimited rubric lists: %+v", colonRows)
	}
	cases := Suggest("Case 1\nThe patient was aged forty and recovered.\nCase 2\nCalled to a patient suffering at night.")
	if !reflect.DeepEqual(cases.Categories, []string{"clinical_cases"}) {
		t.Fatalf("missed numbered narratives: %+v", cases)
	}
	for _, text := range []string{"Case 1 * Case 2 * Case 3", "A -- apples, oranges, pears\nB -- fruit, berries, nuts\nC -- red, blue, green", "Cold -- Abc., Def., Ghi."} {
		if got := Suggest(text); got.State != "uncertain" {
			t.Fatalf("overconfident: %+v", got)
		}
	}
}
