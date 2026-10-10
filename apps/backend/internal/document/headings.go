package document

import (
	"golang.org/x/net/html"
	"homeopath-poc/backend/internal/passage"
	"strings"
)

func legacyHeading(raw, text string) bool {
	if strings.Contains(text, "\n") || len([]rune(text)) > 80 || !passage.Heading(text) {
		return false
	}
	if label := passage.SectionLabel(text); label != "" && strings.Trim(strings.TrimSpace(text[len(label):]), ".:—- ") != "" {
		return false
	}
	z := html.NewTokenizer(strings.NewReader(raw))
	emphasis := 0
	seen := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return seen
		case html.StartTagToken:
			tag := z.Token().Data
			if tag == "a" {
				return false
			}
			if tag == "b" || tag == "strong" {
				emphasis++
			}
		case html.EndTagToken:
			tag := z.Token().Data
			if (tag == "b" || tag == "strong") && emphasis > 0 {
				emphasis--
			}
		case html.TextToken:
			if strings.TrimSpace(string(z.Text())) != "" {
				if emphasis == 0 {
					return false
				}
				seen = true
			}
		}
	}
}
