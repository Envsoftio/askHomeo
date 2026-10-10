package httpapi

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCollectionReviewFrameKeepsSourceBoundaries(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	sources := []collectionReviewSource{
		{url: "https://example.test/book/one", blocks: []collectionReviewBlock{{id: a, original: "Keep this passage. Remove this tail.", reviewed: "Keep this passage. Remove this tail."}}},
		{url: "https://example.test/book/two", status: "published", blocks: []collectionReviewBlock{{id: b, original: "Second page evidence.", reviewed: "Second page evidence."}}},
	}
	original := renderCollectionReview(sources)
	texts, err := parseCollectionReview(original, sources)
	if err != nil || len(texts) != 2 || texts[0] != sources[0].blocks[0].original || texts[1] != sources[1].blocks[0].original {
		t.Fatalf("round trip: %v %#v", err, texts)
	}
	trimmed := strings.Replace(original, sources[0].blocks[0].reviewed, "Keep this passage.", 1)
	texts, err = parseCollectionReview(trimmed, sources)
	if err != nil || texts[0] != "Keep this passage." {
		t.Fatalf("trimmed block: %v %#v", err, texts)
	}
	removed := strings.Replace(original, sources[1].blocks[0].reviewed, "", 1)
	texts, err = parseCollectionReview(removed, sources)
	if err != nil || texts[1] != "" {
		t.Fatalf("removed block: %v %#v", err, texts)
	}
	changedURL := strings.Replace(original, "/book/two", "/book/other", 1)
	if _, err = parseCollectionReview(changedURL, sources); err == nil {
		t.Fatal("changed page source was accepted")
	}
	changedMarker := strings.Replace(original, b.String(), a.String(), 1)
	if _, err = parseCollectionReview(changedMarker, sources); err == nil {
		t.Fatal("changed block marker was accepted")
	}
	if !strings.Contains(original, "PUBLISHED READ ONLY") {
		t.Fatal("published page was not marked read only")
	}
}
