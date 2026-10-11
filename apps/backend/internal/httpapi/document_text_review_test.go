package httpapi

import (
	"strings"
	"testing"
)

func TestDocumentTextReviewMapsCleanedTextToOriginalBlocks(t *testing.T) {
	blocks := []collectionReviewBlock{
		{original: "PREFACE.", reviewed: "PREFACE.", status: "pending"},
		{original: "Unwanted library stamp", reviewed: "Unwanted library stamp", status: "pending"},
		{original: "BELLADONNA.—(Atropa Belladonna.)", reviewed: "BELLADONNA.—(Atropa Belladonna.)", status: "pending"},
		{original: "Keep this symptom. Remove this tail.", reviewed: "Keep this symptom. Remove this tail.", status: "pending"},
	}
	text := renderDocumentReview(blocks)
	selected, err := matchDocumentText(text, blocks)
	if err != nil || len(selected) != len(blocks) {
		t.Fatalf("unchanged text did not round trip: %#v %v", selected, err)
	}
	cleaned := strings.Replace(text, "Unwanted library stamp\n\n", "", 1)
	cleaned = strings.Replace(cleaned, "Keep this symptom. Remove this tail.", "Keep this symptom.", 1)
	selected, err = matchDocumentText(cleaned, blocks)
	if err != nil || selected[1] != "" || selected[2] != blocks[2].original || selected[3] != "Keep this symptom." {
		t.Fatalf("cleaned text lost block identity: %#v %v", selected, err)
	}
	for _, invalid := range []string{
		strings.Replace(cleaned, "Keep this symptom.", "Invented claim.", 1),
		strings.Replace(cleaned, "PREFACE.\n\nBELLADONNA", "BELLADONNA", 1) + "\n\nPREFACE.",
	} {
		if _, err := matchDocumentText(invalid, blocks); err == nil {
			t.Fatal("unmapped or reordered text was accepted")
		}
	}
}

func TestDocumentTextReviewPreservesCRLFOriginal(t *testing.T) {
	blocks := []collectionReviewBlock{{original: "BELLADONNA.\r\nMind: Restless.\r\nHead: Pain.", reviewed: "BELLADONNA.\r\nMind: Restless.\r\nHead: Pain.", status: "pending"}}
	text := renderDocumentReview(blocks)
	if strings.Contains(text, "\r") {
		t.Fatal("editor text still has CRLF")
	}
	selected, err := matchDocumentText(text, blocks)
	if err != nil || selected[0] != blocks[0].original {
		t.Fatalf("original line endings lost: %#v %v", selected, err)
	}
	selected, err = matchDocumentText("Mind: Restless.\nHead: Pain.\n", blocks)
	if err != nil || selected[0] != "Mind: Restless.\r\nHead: Pain." {
		t.Fatalf("trimmed excerpt lost original line endings: %#v %v", selected, err)
	}
}

func TestDocumentReviewHashChangesAfterIndividualReview(t *testing.T) {
	blocks := []collectionReviewBlock{{original: "Original", reviewed: "Original", status: "pending"}}
	before := documentReviewHash(blocks)
	blocks[0].status = "accepted"
	if before == documentReviewHash(blocks) {
		t.Fatal("stale editor would overwrite a newer block decision")
	}
}
