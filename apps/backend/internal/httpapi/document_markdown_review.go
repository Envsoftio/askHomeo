package httpapi

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type documentReviewEdit struct {
	ID   uuid.UUID `json:"id"`
	Text string    `json:"text"`
}

func matchDocumentReviewEdits(edits []documentReviewEdit, blocks []collectionReviewBlock) ([]string, error) {
	if len(edits) != len(blocks) {
		return nil, errors.New("document blocks changed; reload the review")
	}
	selected := make([]string, len(blocks))
	for i, block := range blocks {
		if edits[i].ID != block.id {
			return nil, fmt.Errorf("block %d changed or moved; reload the review", i+1)
		}
		body := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(edits[i].Text, "\r\n", "\n"), "\r", "\n"))
		if body == "" {
			continue
		}
		if block.kind == "figure" || block.kind == "reference" {
			return nil, fmt.Errorf("block %d is not article text evidence", i+1)
		}
		normalized, mapping := normalizeEditorLines(block.original)
		if block.kind == "table" && body != normalized {
			return nil, fmt.Errorf("block %d table must match the saved original", i+1)
		}
		offset := strings.Index(normalized, body)
		if offset < 0 {
			return nil, fmt.Errorf("block %d must retain exact source wording", i+1)
		}
		selected[i] = block.original[mapping[offset]:mapping[offset+len(body)]]
	}
	return selected, nil
}

// Markdown is a review surface. Block markers bind every edit to immutable
// original text; formatting is never indexed or treated as article evidence.
func renderMarkdownDocumentReview(blocks []collectionReviewBlock) string {
	var out strings.Builder
	out.WriteString("# Document review\n\nKeep BLOCK markers and source wording. Remove a block's content to exclude it; trim prose only at its ends. Tables must remain intact.\n\n")
	for _, block := range blocks {
		fmt.Fprintf(&out, "<!-- BLOCK %s -->\n", block.id)
		if block.status != "excluded" {
			reviewed, _ := normalizeEditorLines(block.reviewed)
			switch block.kind {
			case "heading":
				fmt.Fprintf(&out, "## %s\n", reviewed)
			case "table":
				out.WriteString(markdownTable(reviewed))
				out.WriteByte('\n')
			default:
				out.WriteString(reviewed)
				out.WriteByte('\n')
			}
		}
		fmt.Fprintf(&out, "<!-- END %s -->\n\n", block.id)
	}
	return out.String()
}

func markdownTable(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	var out strings.Builder
	for i, line := range lines {
		cells := strings.Split(line, " | ")
		out.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			out.WriteString("| " + strings.TrimSuffix(strings.Repeat("--- | ", len(cells)), " ") + "\n")
		}
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func matchMarkdownDocumentReview(edited string, blocks []collectionReviewBlock) ([]string, error) {
	const preface = "# Document review\n\nKeep BLOCK markers and source wording. Remove a block's content to exclude it; trim prose only at its ends. Tables must remain intact.\n\n"
	if !strings.HasPrefix(edited, preface) {
		return nil, errors.New("Markdown review instructions changed; reload the editor")
	}
	rest := strings.TrimPrefix(edited, preface)
	selected := make([]string, len(blocks))
	for i, block := range blocks {
		start := fmt.Sprintf("<!-- BLOCK %s -->\n", block.id)
		end := fmt.Sprintf("\n<!-- END %s -->\n", block.id)
		if !strings.HasPrefix(rest, start) {
			return nil, fmt.Errorf("block %d marker changed or moved", i+1)
		}
		rest = strings.TrimPrefix(rest, start)
		body, tail, ok := strings.Cut(rest, end)
		if !ok {
			return nil, fmt.Errorf("block %d end marker missing", i+1)
		}
		rest = strings.TrimPrefix(tail, "\n")
		body = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n"))
		if body == "" {
			continue
		}
		if block.kind == "heading" {
			if !strings.HasPrefix(body, "## ") {
				return nil, fmt.Errorf("block %d heading syntax changed", i+1)
			}
			body = strings.TrimPrefix(body, "## ")
		}
		if block.kind == "table" {
			normalized, _ := normalizeEditorLines(block.original)
			if body != markdownTable(normalized) {
				return nil, fmt.Errorf("block %d table differs from the saved original; review cells in the original", i+1)
			}
			selected[i] = block.original
			continue
		}
		normalized, mapping := normalizeEditorLines(block.original)
		offset := strings.Index(normalized, body)
		if offset < 0 {
			return nil, fmt.Errorf("block %d must retain an exact excerpt of its saved original", i+1)
		}
		selected[i] = block.original[mapping[offset]:mapping[offset+len(body)]]
	}
	if strings.TrimSpace(rest) != "" {
		return nil, errors.New("unexpected Markdown after the last block")
	}
	return selected, nil
}
