package httpapi

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMarkdownDocumentReviewPreservesBlockIdentity(t *testing.T) {
	blocks := []collectionReviewBlock{
		{id: uuid.New(), kind: "heading", original: "Results", reviewed: "Results", status: "pending"},
		{id: uuid.New(), kind: "paragraph", original: "Ten patients improved. No adverse events reported.", reviewed: "Ten patients improved. No adverse events reported.", status: "pending"},
		{id: uuid.New(), kind: "table", original: "Arm | Change\nA | −2\nB | 0", reviewed: "Arm | Change\nA | −2\nB | 0", status: "pending"},
	}
	md := renderMarkdownDocumentReview(blocks)
	if !strings.Contains(md, "| Arm | Change |") || !strings.Contains(md, "## Results") {
		t.Fatal("Markdown structure missing")
	}
	got, err := matchMarkdownDocumentReview(md, blocks)
	if err != nil || got[2] != blocks[2].original {
		t.Fatalf("round trip failed: %q %v", got, err)
	}
	cleaned := strings.Replace(md, "Ten patients improved. No adverse events reported.", "Ten patients improved.", 1)
	got, err = matchMarkdownDocumentReview(cleaned, blocks)
	if err != nil || got[1] != "Ten patients improved." {
		t.Fatalf("exact trim failed: %q %v", got, err)
	}
	for _, bad := range []string{
		strings.Replace(md, "−2", "−20", 1),
		strings.Replace(md, blocks[0].id.String(), uuid.New().String(), 1),
		strings.Replace(md, "Ten patients improved.", "Twenty patients improved.", 1),
	} {
		if _, err := matchMarkdownDocumentReview(bad, blocks); err == nil {
			t.Fatal("accepted changed evidence or marker")
		}
	}
}

func TestRenderJATSTableEscapesCells(t *testing.T) {
	html := renderJATSTable("Arm | Result\nA | <script>x</script>")
	if !strings.Contains(html, "<td>Arm</td>") || !strings.Contains(html, "&lt;script&gt;") || strings.Contains(html, "<script>") {
		t.Fatal(html)
	}
	original, err := renderJATSOriginalTable([]byte(`<table><thead><tr><th rowspan="2">Arm</th><th>Result</th></tr></thead><tbody><tr><td>A</td><td>&lt;script&gt;</td></tr></tbody></table>`))
	if err != nil || !strings.Contains(original, `<th rowspan="2">Arm</th>`) || !strings.Contains(original, "&lt;script&gt;") || strings.Contains(original, "<script>") {
		t.Fatalf("original table structure lost: %s %v", original, err)
	}
}

func TestStructuredDocumentReviewEditsKeepSourceMapping(t *testing.T) {
	blocks := []collectionReviewBlock{
		{id: uuid.New(), kind: "heading", original: "Human Subject Ethics Review Approvals or Exemptions", reviewed: "Human Subject Ethics Review Approvals or Exemptions", status: "pending"},
		{id: uuid.New(), kind: "paragraph", original: "First line.\r\nSecond line.", reviewed: "First line.\r\nSecond line.", status: "pending"},
		{id: uuid.New(), kind: "table", original: "Arm | Result\nA | 1", reviewed: "Arm | Result\nA | 1", status: "pending"},
		{id: uuid.New(), kind: "figure", original: "Figure 1 [image reference: fig1.png]", reviewed: "Figure 1 [image reference: fig1.png]", status: "excluded"},
	}
	edits := []documentReviewEdit{{blocks[0].id, blocks[0].original}, {blocks[1].id, "Second line."}, {blocks[2].id, blocks[2].original}, {blocks[3].id, ""}}
	selected, err := matchDocumentReviewEdits(edits, blocks)
	if err != nil || selected[1] != "Second line." || selected[3] != "" {
		t.Fatalf("structured edits failed: %#v %v", selected, err)
	}
	edits[1].ID = uuid.New()
	if _, err = matchDocumentReviewEdits(edits, blocks); err == nil {
		t.Fatal("accepted moved block")
	}
	edits[1].ID = blocks[1].id
	edits[2].Text = "Arm | Result\nA | 10"
	if _, err = matchDocumentReviewEdits(edits, blocks); err == nil {
		t.Fatal("accepted changed table cell")
	}
	edits[2].Text = blocks[2].original
	edits[3].Text = blocks[3].original
	if _, err = matchDocumentReviewEdits(edits, blocks); err == nil {
		t.Fatal("accepted figure as text evidence")
	}
}
