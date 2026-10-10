package httpapi

import (
	"strings"
	"testing"
)

func TestRubricSuggestionsPreserveExactUnicodeRows(t *testing.T) {
	text := "Introductory notes\n\u2003 Peur, aggravation: Acon., Bell.; Nux-v. \r\nFear, amelioration: Coff.\nSee also: nerves\nDose: 2 mg\n"
	got := suggestRubricRows(text)
	if len(got) != 2 || got[0].Heading != "Peur, aggravation" || len(got[0].Notations) != 3 || got[1].Heading != "Fear, amelioration" {
		t.Fatalf("unexpected suggestions: %+v", got)
	}
	for _, row := range got {
		if string([]rune(text)[row.Start:row.End]) != row.ExactText {
			t.Fatalf("lost exact source offsets: %+v", row)
		}
	}
}

func TestRubricSuggestionsLeaveAmbiguityForReview(t *testing.T) {
	for _, text := range []string{"Fear: Acon.,,Bell.", "Fear: Acon.,", "Fear: Acon., Acon.", "Fear: Acon. (see anxiety)", "Fear: Acon. Bell.", "Fear:", ": Acon.", "https://example.test: Acon.", "He reported: improvement after treatment."} {
		if rows := suggestRubricRows(text); len(rows) != 0 {
			t.Fatalf("ambiguous row recognized: %q %+v", text, rows)
		}
	}
	if got := suggestRubricRows(strings.Repeat("Fear: Acon.\n", 60)); len(got) != 50 {
		t.Fatalf("suggestion bound failed: %d", len(got))
	}
}

func TestSourceNotationDoesNotImplyNumericGrade(t *testing.T) {
	for _, style := range []string{"unknown", "ordinary", "italic", "bold", "bold_italic", "other"} {
		if !validSourceStyle(style) {
			t.Fatal(style)
		}
	}
	if validSourceStyle("grade 3") || conventionSupported("", nil) || conventionSupported("italic is grade 2", []structuredLocationInput{{ExactText: "Italic indicates emphasis."}}) {
		t.Fatal("invented grade convention accepted")
	}
}
