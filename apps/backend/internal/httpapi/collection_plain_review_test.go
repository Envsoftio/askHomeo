package httpapi

import (
	"strings"
	"testing"
)

func TestPlainBookReviewMapsCleanTextToOriginalPages(t *testing.T) {
	sources := []collectionReviewSource{
		{url: "https://example.test/book/one", blocks: []collectionReviewBlock{
			{original: "Heading", reviewed: "Heading"},
			{original: "Keep this passage. Remove this tail.", reviewed: "Keep this passage. Remove this tail."},
			{original: "Unwanted footer", reviewed: "Unwanted footer"},
			{original: "First paragraph.\n\nSecond paragraph.", reviewed: "First paragraph.\n\nSecond paragraph."},
		}},
		{url: "https://example.test/book2/two", status: "published", blocks: []collectionReviewBlock{{original: "Published evidence.", reviewed: "Published evidence."}}},
	}
	text := renderPlainCollectionReview(sources)
	if strings.Contains(text, "[[BLOCK") || !strings.Contains(text, "Source URL:") {
		t.Fatal("editor is not clean, URL-backed text")
	}
	texts, err := parsePlainCollectionReview(text, sources)
	if err != nil || len(texts) != 5 || texts[3] != sources[0].blocks[3].original {
		t.Fatalf("unchanged book did not round trip: %#v %v", texts, err)
	}
	text = strings.Replace(text, "Keep this passage. Remove this tail.", "Keep this passage.", 1)
	text = strings.Replace(text, "Unwanted footer\n\n", "", 1)
	texts, err = parsePlainCollectionReview(text, sources)
	if err != nil || texts[1] != "Keep this passage." || texts[2] != "" || texts[4] != "Published evidence." {
		t.Fatalf("clean edits lost their source mapping: %#v %v", texts, err)
	}
	for _, invalid := range []string{
		strings.Replace(text, "Keep this passage.", "Invented claim.", 1),
		strings.Replace(text, "/book2/two", "/book2/other", 1),
		strings.Replace(text, "Heading\n\nKeep this passage.", "Keep this passage.\n\nHeading", 1),
		text + "Injected trailing content",
	} {
		if _, err := parsePlainCollectionReview(invalid, sources); err == nil {
			t.Fatal("unmapped or reordered content was accepted")
		}
	}
}
