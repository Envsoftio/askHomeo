package httpapi

import "testing"

func TestRelationNames(t *testing.T) {
	for _, question := range []string{
		"How is Nash related to Albert Farrington ?",
		"What is the relationship between Nash and Albert Farrington?",
		"Are Dr. Nash and Albert Farrington related?",
	} {
		first, second, ok := relationNames(question)
		if !ok || first != "Nash" || second != "Albert Farrington" {
			t.Fatalf("%q: %q %q %v", question, first, second, ok)
		}
	}
	if _, _, ok := relationNames("How is Nash related to Nash?"); ok {
		t.Fatal("same person is not a two-person relation question")
	}
}
