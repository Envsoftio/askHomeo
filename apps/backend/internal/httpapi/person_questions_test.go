package httpapi

import "testing"

func TestPersonQuestionAndAuthorMatch(t *testing.T) {
	for _, item := range []struct{ question, author, display string }{
		{"Who is NASH?", "E. B. Nash", "E. B. Nash"},
		{"Tell me about Dr. Nash", "E. B. Nash", "E. B. Nash"},
		{"Who is Nash in homeopathy?", "E. B. Nash", "E. B. Nash"},
		{"Who was Dr. Ernest Albert Farrington?", "Farrington, E. A. (Ernest Albert), 1847-1885", "Ernest Albert Farrington"},
		{"Who is H. J. Hamre?", "H. J. Hamre, A. Glockmann, K. von Ammon", "H. J. Hamre"},
	} {
		person := personNameFromQuestion(item.question)
		if !authorMatchesPerson(item.author, person) {
			t.Fatalf("%q should match %q (parsed %q)", item.question, item.author, person)
		}
		if got := authorDisplayName(item.author, person); got != item.display {
			t.Fatalf("%q: display %q", item.question, got)
		}
	}
	if authorMatchesPerson("E. B. Nash", "Kent") {
		t.Fatal("unrelated person matched source author")
	}
	if got := personNameFromQuestion("Who is the author of Nash's book?"); got != "" {
		t.Fatalf("book authorship question parsed as a person: %q", got)
	}
}

func TestPhysicianRequiresCredentialNearPerson(t *testing.T) {
	if !physicianOnPage("BY E. B. NASH, M. D.", "nash") {
		t.Fatal("Nash title page identifies him as M.D.")
	}
	if !physicianOnPage("The subject of this sketch, Dr. Ernest A. Farrington, was born January 1, 1847.", "farrington") {
		t.Fatal("Farrington memorial identifies him as Dr.")
	}
	if physicianOnPage("Nash wrote this book. Dr. Smith helped him.", "nash") {
		t.Fatal("another person's title cannot establish Nash's credential")
	}
}
