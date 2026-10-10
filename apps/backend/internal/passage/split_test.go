package passage

import (
	"strings"
	"testing"
	"unicode"
)

func TestEntriesRemainSeparateAndSectionsStayWithShortProfile(t *testing.T) {
	text := "BELLADONNA\nMind.— Restless.\nHead.— Throbbing.\n\nNUX VOMICA\nMind.— Irritable.\nStomach.— Nausea."
	spans := Split(text, 1800)
	if len(spans) != 2 || !strings.HasPrefix(spans[0].Text, "BELLADONNA") || !strings.HasPrefix(spans[1].Text, "NUX VOMICA") {
		t.Fatalf("mixed entries or orphan heading: %+v", spans)
	}
}

func TestExactUnicodeCoverageAndBounds(t *testing.T) {
	for _, text := range []string{"", " \t\r\n", "BELLADONNA\r\nMind.— café; élève.\n" + strings.Repeat("Worse from motion; better at rest. ", 150), strings.Repeat("界", 200), "A\u00a0B\tC\nD"} {
		for _, limit := range []int{1, 7, 70, 1800} {
			runes := []rune(text)
			covered := make([]bool, len(runes))
			previous := 0
			for _, span := range Split(text, limit) {
				if span.Start < previous || span.End <= span.Start || span.End > len(runes) || span.End-span.Start > limit || span.Text != string(runes[span.Start:span.End]) {
					t.Fatalf("invalid exact span: %+v", span)
				}
				for i := span.Start; i < span.End; i++ {
					covered[i] = true
				}
				previous = span.End
			}
			for i, r := range runes {
				if !unicode.IsSpace(r) && !covered[i] {
					t.Fatalf("lost character at %d, limit %d", i, limit)
				}
			}
		}
	}
}

func TestSectionLabelsAreNotRunningProse(t *testing.T) {
	for _, s := range []string{"Headache after motion", "Mindful of others", "Skin is dry", "The mind is restless"} {
		if SectionLabel(s) != "" {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"Mind.— restless", "Head: pain", "MODALITIES", "Relationship.-- Compare."} {
		if SectionLabel(s) == "" {
			t.Fatal(s)
		}
	}
}
