package document

import (
	"bytes"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Version identifies generic extraction/filter rules, never a book or hostname.
const Version = "document-structure-v2"

// EvidenceText excludes navigation from classification and search preparation.
// Excluded blocks remain available, with original offsets, for review/restoration.
func EvidenceText(blocks []Block) string {
	var b strings.Builder
	for _, block := range blocks {
		if block.ExclusionReason != "" {
			continue
		}
		b.WriteString(block.Heading)
		b.WriteByte('\n')
		b.WriteString(block.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func assessHTMLBlocks(decoded string, blocks []Block) {
	linkOnly := make([]bool, len(blocks))
	navigationCandidates := 0
	for i := range blocks {
		block := &blocks[i]
		if block.StartByte < 0 || block.EndByte > len(decoded) || block.EndByte <= block.StartByte {
			continue
		}
		raw := decoded[block.StartByte:block.EndByte]
		root, err := html.Parse(bytes.NewBufferString(raw))
		if err != nil {
			continue
		}
		links, linkedRunes, linkedImages := 0, 0, 0
		var walk func(*html.Node, bool)
		walk = func(n *html.Node, linked bool) {
			if n.Type == html.ElementNode && n.Data == "a" {
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						links++
						linked = true
						break
					}
				}
			}
			if linked && n.Type == html.ElementNode && n.Data == "img" {
				linkedImages++
			}
			if linked && n.Type == html.TextNode {
				linkedRunes += utf8.RuneCountInString(strings.TrimSpace(n.Data))
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walk(child, linked)
			}
		}
		walk(root, false)
		text := strings.ToLower(strings.TrimSpace(block.Text))
		letters, uppercase := 0, 0
		for _, r := range block.Text {
			if unicode.IsLetter(r) {
				letters++
				if unicode.IsUpper(r) {
					uppercase++
				}
			}
		}
		isList := strings.Contains(text, "--") || strings.Contains(text, "—")
		linkOnly[i] = links > 0 && !isList && linkedRunes*100 >= utf8.RuneCountInString(text)*65
		if linkOnly[i] {
			navigationCandidates++
		}
		switch {
		case links > 0 && (text == "main" || text == "home" || text == "back" || text == "next" || text == "previous" || text == "back to top" || text == "buy a copy"):
			block.ExclusionReason = "Navigation or purchase link"
		case text == "(advertising)" || text == "(advertisement)" || text == "(adversting)" || text == "advertisement":
			block.ExclusionReason = "Advertisement label"
		case strings.HasPrefix(text, "copyright ©") && utf8.RuneCountInString(text) < 160:
			block.ExclusionReason = "Copyright footer (retained in original asset)"
		case linkedImages >= 3 && !isList && letters > 0 && uppercase*100 >= letters*90:
			block.ExclusionReason = "Image-linked uppercase navigation list"
		case strings.Contains(text, "presented by") && strings.Contains(text, "by ") && utf8.RuneCountInString(text) < 200:
			block.ExclusionReason = "Presentation/attribution banner (retained in original asset)"
		case links >= 2 && strings.ContainsAny(text, "*|") && !strings.Contains(text, "--") && linkedRunes*100 >= utf8.RuneCountInString(text)*65:
			block.ExclusionReason = "Link-dense navigation menu"
		}
	}
	// Standalone link labels on index-like pages are navigation. Ordinary
	// prose with occasional references does not meet this page-level rule.
	if navigationCandidates >= 5 && navigationCandidates*4 >= len(blocks) {
		for i := range blocks {
			if linkOnly[i] && utf8.RuneCountInString(blocks[i].Text) < 160 && blocks[i].ExclusionReason == "" {
				blocks[i].ExclusionReason = "Link-only entry on a navigation-dominated page"
			}
		}
	}

}

// The title is metadata, never evidence inserted into the exact passage text.
func htmlTitle(decoded string) string {
	z := html.NewTokenizer(strings.NewReader(decoded))
	inTitle := false
	var title strings.Builder
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() == io.EOF {
				break
			}
			return ""
		}
		token := z.Token()
		if tt == html.StartTagToken && token.Data == "title" {
			inTitle = true
		}
		if tt == html.EndTagToken && token.Data == "title" {
			break
		}
		if inTitle && tt == html.TextToken {
			title.WriteString(token.Data)
		}
		if title.Len() > 2000 {
			return ""
		}
	}
	return strings.Join(strings.Fields(title.String()), " ")
}
