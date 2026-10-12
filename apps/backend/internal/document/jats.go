package document

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// JATSVersion changes whenever the element-to-block mapping changes. Byte
// offsets below address the immutable UTF-8 XML asset, not decoded characters.
const JATSVersion = "jats-v3"

var jatsSpace = regexp.MustCompile(`\s+`)
var jatsDoctype = regexp.MustCompile(`(?is)<!DOCTYPE\s+article\s*(?:SYSTEM\s+"[^"]*"|PUBLIC\s+"[^"]*"\s+"[^"]*")?\s*>`)

type jatsFrame struct {
	name, key, heading string
	start              int
	text               strings.Builder
	inRefs             bool
	imageRef           string
}

func isJATS(raw []byte, contentType string) bool {
	head := raw
	if len(head) > 4096 {
		head = head[:4096]
	}
	s := string(head)
	return strings.Contains(s, "<article") && (strings.Contains(contentType, "xml") || strings.HasPrefix(strings.TrimSpace(s), "<?xml") || strings.HasPrefix(strings.TrimSpace(s), "<!DOCTYPE"))
}

// ExtractJATS deliberately accepts only the JATS article root. Go's XML
// decoder never fetches DTDs; custom entities and internal subsets are denied.
func ExtractJATS(raw []byte) (Result, error) {
	if len(raw) == 0 || len(raw) > MaxBytes {
		return Result{}, errors.New("JATS document is empty or exceeds 10 MiB")
	}
	if !utf8.Valid(raw) {
		return Result{}, errors.New("JATS must be UTF-8")
	}
	if bytes.Contains(bytes.ToUpper(raw), []byte("<!ENTITY")) {
		return Result{}, errors.New("JATS entity declarations are forbidden")
	}
	clean := jatsDoctype.ReplaceAll(raw, []byte{})
	if bytes.Contains(bytes.ToUpper(clean), []byte("<!DOCTYPE")) {
		return Result{}, errors.New("unsupported JATS DOCTYPE")
	}
	// Replacement preserves offsets only when no DOCTYPE was present. Map
	// decoder offsets back through the removed declaration below.
	removedAt, removedLen := 0, 0
	if loc := jatsDoctype.FindIndex(raw); loc != nil {
		removedAt, removedLen = loc[0], loc[1]-loc[0]
	}
	originalOffset := func(n int) int {
		if removedLen > 0 && n >= removedAt {
			return n + removedLen
		}
		return n
	}
	dec := xml.NewDecoder(bytes.NewReader(clean))
	dec.Strict = true
	stack := []*jatsFrame{}
	out := Result{Format: "jats", Charset: "utf-8", Blocks: []Block{}}
	counter := 0
	rootSeen := false
	emit := func(f *jatsFrame, end int) {
		if f.inRefs && f.name != "ref" {
			return
		}
		kind := "paragraph"
		switch f.name {
		case "article-title", "title":
			kind = "heading"
		case "abstract":
			kind = "abstract"
		case "list-item":
			kind = "list"
		case "caption":
			kind = "caption"
		case "table-wrap-foot":
			kind = "table_footnote"
		case "ref":
			kind = "reference"
		case "funding-group":
			kind = "funding"
		case "ack":
			kind = "acknowledgment"
		case "fn":
			kind = "footnote"
		case "table":
			kind = "table"
		case "fig":
			kind = "figure"
		}
		value := strings.TrimSpace(jatsSpace.ReplaceAllString(f.text.String(), " "))
		if kind == "table" {
			lines := []string{}
			for _, line := range strings.Split(f.text.String(), "\n") {
				cells := []string{}
				for _, cell := range strings.Split(line, "\t") {
					cells = append(cells, strings.TrimSpace(jatsSpace.ReplaceAllString(cell, " ")))
				}
				joined := strings.TrimSpace(strings.Join(cells, " | "))
				if joined != "" {
					lines = append(lines, joined)
				}
			}
			value = strings.Join(lines, "\n")
		}
		if value == "" && kind != "figure" {
			return
		}
		counter++
		key := fmt.Sprintf("jats-%05d-%s", counter, f.name)
		heading := f.heading
		if kind == "heading" {
			heading = value
			if out.Title == "" && f.name == "article-title" {
				out.Title = value
			}
		}
		block := Block{Key: key, Kind: kind, Heading: heading, Text: value, StartByte: originalOffset(f.start), EndByte: originalOffset(end)}
		if kind == "reference" {
			block.ExclusionReason = "bibliographic reference, not article findings"
		}
		if kind == "figure" {
			block.Text = strings.TrimSpace(value + " [image reference: " + f.imageRef + "]")
			block.ExclusionReason = "figure image requires separate asset and visual review"
		}
		out.Blocks = append(out.Blocks, block)
	}
	for {
		before := int(dec.InputOffset())
		tok, err := dec.Token()
		after := int(dec.InputOffset())
		if err == io.EOF {
			break
		}
		if err != nil {
			return Result{}, fmt.Errorf("malformed JATS: %w", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return Result{}, errors.New("unsupported JATS directive")
		case xml.StartElement:
			if t.Name.Local == "graphic" || t.Name.Local == "inline-graphic" || t.Name.Local == "supplementary-material" || t.Name.Local == "disp-formula" {
				out.Warnings = append(out.Warnings, "Unextracted "+t.Name.Local+" requires separate review")
			}
			if t.Name.Local == "graphic" || t.Name.Local == "inline-graphic" {
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].name == "fig" {
						for _, attr := range t.Attr {
							if attr.Name.Local == "href" {
								stack[i].imageRef = attr.Value
								break
							}
						}
						break
					}
				}
			}
			if len(stack) >= 64 {
				return Result{}, errors.New("JATS nesting exceeds 64 elements")
			}
			if len(stack) == 0 {
				if rootSeen || t.Name.Local != "article" {
					return Result{}, errors.New("JATS article root required")
				}
				rootSeen = true
			}
			heading := ""
			refs := false
			if len(stack) > 0 {
				heading = stack[len(stack)-1].heading
				refs = stack[len(stack)-1].inRefs
			}
			if t.Name.Local == "ref-list" {
				refs = true
			}
			stack = append(stack, &jatsFrame{name: t.Name.Local, start: before, heading: heading, inRefs: refs})
		case xml.CharData:
			for _, f := range stack {
				if f.text.Len() > MaxBytes {
					return Result{}, errors.New("JATS text exceeds limit")
				}
				f.text.Write([]byte(t))
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return Result{}, errors.New("unexpected JATS closing element")
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if f.name == "td" || f.name == "th" || f.name == "tr" {
				separator := "\t"
				if f.name == "tr" {
					separator = "\n"
				}
				for _, ancestor := range stack {
					if ancestor.name == "table" {
						ancestor.text.WriteString(separator)
					}
				}
			}
			if f.name != t.Name.Local {
				return Result{}, errors.New("JATS closing element differs")
			}
			switch f.name {
			case "article-title", "title", "p", "list-item", "caption", "table-wrap-foot", "ref", "ack", "funding-group", "fn", "table", "fig":
				// A table is retained as one context block, including its headers and
				// footnotes. Its child paragraphs are not separately treated as findings.
				if f.name == "fig" && f.imageRef == "" {
					break
				}
				if f.name != "p" || !hasAncestor(stack, "table") {
					emit(f, after)
				}
			}
			if f.name == "title" && len(stack) > 0 && stack[len(stack)-1].name == "sec" {
				path := strings.TrimSpace(f.text.String())
				if parent := stack[len(stack)-1].heading; parent != "" {
					path = parent + " / " + path
				}
				stack[len(stack)-1].heading = path
			}
		}
	}
	if !rootSeen || len(stack) != 0 || len(out.Blocks) == 0 {
		return Result{}, errors.New("JATS has no complete reviewable article text")
	}
	return out, nil
}

func hasAncestor(stack []*jatsFrame, name string) bool {
	for _, f := range stack {
		if f.name == name {
			return true
		}
	}
	return false
}
