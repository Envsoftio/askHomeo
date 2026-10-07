package httpapi

import "testing"

func TestSourceAuthorQuestionUsesAuthorLine(t *testing.T) {
	source := readySource{Title: "Efficacy of homoeopathic treatment: systematic review of meta-analyses of randomised placebo-controlled homoeopathy trials for any indication", Author: "H. J. Hamre, A. Glockmann, K. von Ammon, D. S. Riley, H. Kiene"}
	if !sourceMatchesQuestion("Who authored the 2023 systematic review of meta-analyses of homoeopathic trials?", source) {
		t.Fatal("author question did not match the prepared paper")
	}
	if sourceMatchesQuestion("Who authored the repertory of Kent?", source) {
		t.Fatal("unrelated author question matched")
	}
	if !authorLineSupported(source.Author, "H. J. Hamre, A. Glockmann, K. von Ammon, D. S. Riley and H. Kiene") {
		t.Fatal("complete author line was rejected")
	}
	if authorLineSupported(source.Author, "Hamre et al.") {
		t.Fatal("abbreviated author line was accepted as full support")
	}
}
