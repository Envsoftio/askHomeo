package httpapi

import (
	"fmt"
	"strings"
)

func plainPageHeader(page int, source collectionReviewSource) string {
	status := ""
	if source.status == "published" {
		status = " (published, read only)"
	}
	return fmt.Sprintf("===== PAGE %d%s =====\nSource URL: %s\n\n", page+1, status, source.url)
}

// The user edits one readable book. Page URLs remain visible; block identities
// stay on the server so deleting prose cannot move evidence to another URL.
func renderPlainCollectionReview(sources []collectionReviewSource) string {
	var out strings.Builder
	for page, source := range sources {
		out.WriteString(plainPageHeader(page, source))
		for _, block := range source.blocks {
			out.WriteString(strings.TrimSpace(block.reviewed))
			out.WriteString("\n\n")
		}
	}
	return out.String()
}

func parsePlainCollectionReview(edited string, sources []collectionReviewSource) ([]string, error) {
	// Browsers and pasted text may use CRLF. The extracted source uses LF.
	rest := strings.ReplaceAll(edited, "\r\n", "\n")
	texts := []string{}
	for page, source := range sources {
		header := plainPageHeader(page, source)
		if !strings.HasPrefix(rest, header) {
			return nil, fmt.Errorf("keep the page %d heading and source URL unchanged", page+1)
		}
		rest = strings.TrimPrefix(rest, header)
		body := rest
		if page+1 < len(sources) {
			var found bool
			body, rest, found = strings.Cut(rest, plainPageHeader(page+1, sources[page+1]))
			if !found {
				return nil, fmt.Errorf("keep the page %d heading and source URL unchanged", page+2)
			}
			rest = plainPageHeader(page+1, sources[page+1]) + rest
		} else {
			rest = ""
		}
		selected, err := matchPlainPage(strings.TrimSpace(body), source.blocks)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page+1, err)
		}
		texts = append(texts, selected...)
	}
	if rest != "" {
		return nil, fmt.Errorf("unexpected text outside the book pages")
	}
	return texts, nil
}

// Match retained paragraphs to their original blocks in order. A whole
// paragraph may be removed or its ends trimmed. Embedded blank lines are
// preserved by choosing the longest matching prefix. Unmatched text is an
// error, never silently discarded or assigned to a different source page.
func matchPlainPage(rest string, blocks []collectionReviewBlock) ([]string, error) {
	texts := make([]string, len(blocks))
	for i, block := range blocks {
		if rest == "" {
			break
		}
		// No retained excerpt can be longer than its source block.
		end := min(len(rest), len(block.original))
		for end > 0 {
			if end == len(rest) || strings.HasPrefix(rest[end:], "\n\n") {
				candidate := strings.TrimSpace(rest[:end])
				if candidate != "" && strings.Contains(block.original, candidate) {
					texts[i] = candidate
					rest = strings.TrimSpace(rest[end:])
					break
				}
			}
			// Only paragraph boundaries can divide blocks, avoiding a partial
			// word match that would incorrectly consume a later passage.
			end = strings.LastIndex(rest[:end], "\n\n")
		}
	}
	if rest != "" {
		return nil, fmt.Errorf("keep retained paragraphs in their original order and wording, separated by a blank line; remove whole paragraphs or trim their ends")
	}
	return texts, nil
}
