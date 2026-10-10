// Package document extracts reviewable blocks from a single HTML or text asset.
// It never follows links or runs document code. Offsets refer to immutable raw bytes.
package document

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

const MaxBytes = 10 << 20

type Block struct {
	ExclusionReason string `json:"exclusion_reason,omitempty"`
	Key             string `json:"key"`
	Kind            string `json:"kind"`
	Heading         string `json:"heading"`
	Text            string `json:"text"`
	StartByte       int    `json:"start_byte"`
	EndByte         int    `json:"end_byte"`
}

type Result struct {
	Title    string   `json:"title,omitempty"`
	Format   string   `json:"format"`
	Charset  string   `json:"charset"`
	Blocks   []Block  `json:"blocks"`
	Warnings []string `json:"warnings"`
}

var whitespace = regexp.MustCompile(`\s+`)
var htmlStart = regexp.MustCompile(`(?is)^\s*(?:<!doctype\s+html\b|<html\b|<head\b|<body\b|<h[1-6]\b|<p\b)`)
var htmlTag = regexp.MustCompile(`(?i)<(?:article|main|section|div|h[1-6]|p|table|ul|ol|li|blockquote)\b[^>]*>`)
var paragraphBreak = regexp.MustCompile(`\r?\n[ \t]*\r?\n`)

// Detect uses bytes first, then MIME type. A filename alone cannot select a format.
func Detect(raw []byte, contentType string) (string, error) {
	if len(raw) == 0 || len(bytes.TrimSpace(raw)) == 0 {
		return "", errors.New("document is empty")
	}
	if len(raw) > MaxBytes {
		return "", fmt.Errorf("document exceeds %d byte limit", MaxBytes)
	}
	mimeType, _, _ := mime.ParseMediaType(contentType)
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("%PDF-")) {
		return "", errors.New("PDF bytes require PDF intake")
	}
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("<?xml")) || mimeType == "application/xml" || mimeType == "text/xml" {
		return "", errors.New("XML is not supported by single-document HTML/TXT intake")
	}
	if htmlStart.Match(raw) || htmlTag.Match(raw) || mimeType == "text/html" {
		return "html", nil
	}
	if mimeType != "" && mimeType != "text/plain" && mimeType != "application/octet-stream" {
		return "", fmt.Errorf("unsupported content type %q", mimeType)
	}
	if regexp.MustCompile(`<[/!?][A-Za-z][^>]*>`).Match(raw) {
		return "", errors.New("markup is not recognized as static HTML")
	}
	return "txt", nil
}

// Extract rejects malformed character data. charsetOverride is an explicit review
// choice for documents whose encoding cannot be determined from their bytes.
func Extract(raw []byte, contentType, charsetOverride string) (Result, error) {
	format, err := Detect(raw, contentType)
	if err != nil {
		return Result{}, err
	}
	decoded, charset, err := decode(raw, contentType, charsetOverride)
	if err != nil {
		return Result{}, err
	}
	var out Result
	if format == "html" {
		out, err = extractHTML(decoded)
		if err == nil {
			assessHTMLBlocks(decoded, out.Blocks)
			out.Title = htmlTitle(decoded)
		}
	} else {
		out, err = extractText(decoded, charset)
	}
	if err != nil {
		return Result{}, err
	}
	out.Format, out.Charset = format, charset
	if format == "html" && charset == "windows-1252" && charsetOverride == "" {
		out.Warnings = append(out.Warnings, "Decoded legacy HTML as Windows-1252; verify accented characters and notation during content review")
	}
	for i := range out.Blocks {
		start, e1 := originalOffset(decoded, charset, out.Blocks[i].StartByte, raw)
		end, e2 := originalOffset(decoded, charset, out.Blocks[i].EndByte, raw)
		if e1 != nil || e2 != nil {
			return Result{}, errors.New("could not map extracted text to original document bytes")
		}
		out.Blocks[i].StartByte, out.Blocks[i].EndByte = start, end
	}
	if len(out.Blocks) == 0 {
		return Result{}, errors.New("document has no substantive text; it may be a JavaScript shell or login page")
	}
	return out, nil
}

func decode(raw []byte, contentType, override string) (string, string, error) {
	_, params, _ := mime.ParseMediaType(contentType)
	label := strings.ToLower(strings.TrimSpace(override))
	if label == "" {
		label = strings.ToLower(strings.TrimSpace(params["charset"]))
	}
	if bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		if label != "" && label != "utf-8" && label != "utf8" {
			return "", "", errors.New("charset conflicts with UTF-8 BOM")
		}
		raw, label = raw[3:], "utf-8"
	} else if len(raw) >= 2 && (bytes.Equal(raw[:2], []byte{0xff, 0xfe}) || bytes.Equal(raw[:2], []byte{0xfe, 0xff})) {
		le := raw[0] == 0xff
		if label != "" && label != "utf-16" && label != "utf-16le" && label != "utf-16be" {
			return "", "", errors.New("charset conflicts with UTF-16 BOM")
		}
		if (label == "utf-16le" && !le) || (label == "utf-16be" && le) {
			return "", "", errors.New("charset conflicts with UTF-16 byte order")
		}
		label = "utf-16be"
		if le {
			label = "utf-16le"
		}
		return decodeUTF16(raw[2:], le)
	}
	if label == "" {
		// A meta charset declaration is usable only when it appears in the first
		// kilobyte; otherwise the administrator must choose an override.
		prefix := raw
		if len(prefix) > 1024 {
			prefix = prefix[:1024]
		}
		if match := regexp.MustCompile(`(?i)<meta\s+[^>]*charset\s*=\s*["']?([a-z0-9_-]+)`).FindSubmatch(prefix); len(match) == 2 {
			label = strings.ToLower(string(match[1]))
		}
	}
	if label == "" {
		label = "utf-8"
		// Legacy HTML commonly omits its encoding. Use the browser-compatible
		// Western fallback only for unlabeled HTML, never over an explicit
		// declaration/BOM/override or for plain text. Extraction remains reviewed.
		if format, err := Detect(raw, contentType); err == nil && format == "html" && !utf8.Valid(raw) {
			label = "windows-1252"
		}
	}
	var decoded string
	switch label {
	case "utf-8", "utf8":
		if !utf8.Valid(raw) {
			return "", "", errors.New("invalid UTF-8; select an encoding override")
		}
		decoded, label = string(raw), "utf-8"
	case "windows-1252", "cp1252", "iso-8859-1", "latin1":
		enc := charmap.Windows1252
		if label == "iso-8859-1" || label == "latin1" {
			enc = charmap.ISO8859_1
		}
		converted, _, err := transform.String(enc.NewDecoder(), string(raw))
		if err != nil || strings.ContainsRune(converted, utf8.RuneError) {
			return "", "", errors.New("document contains undecodable characters")
		}
		decoded = converted
	default:
		return "", "", fmt.Errorf("unsupported charset %q; select UTF-8, UTF-16, Windows-1252 or ISO-8859-1", label)
	}
	for _, r := range decoded {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
			return "", "", errors.New("document contains binary or control characters")
		}
	}
	return decoded, label, nil
}

func decodeUTF16(raw []byte, little bool) (string, string, error) {
	if len(raw)%2 != 0 {
		return "", "", errors.New("truncated UTF-16 document")
	}
	var b strings.Builder
	for i := 0; i < len(raw); i += 2 {
		var v uint16
		if little {
			v = binary.LittleEndian.Uint16(raw[i:])
		} else {
			v = binary.BigEndian.Uint16(raw[i:])
		}
		if v >= 0xd800 && v <= 0xdbff {
			if i+4 > len(raw) {
				return "", "", errors.New("truncated UTF-16 surrogate")
			}
			i += 2
			var low uint16
			if little {
				low = binary.LittleEndian.Uint16(raw[i:])
			} else {
				low = binary.BigEndian.Uint16(raw[i:])
			}
			if low < 0xdc00 || low > 0xdfff {
				return "", "", errors.New("invalid UTF-16 surrogate")
			}
			b.WriteRune(rune(0x10000 + (uint32(v)-0xd800)*0x400 + uint32(low) - 0xdc00))
		} else if v >= 0xdc00 && v <= 0xdfff {
			return "", "", errors.New("invalid UTF-16 surrogate")
		} else {
			b.WriteRune(rune(v))
		}
	}
	decoded := b.String()
	for _, r := range decoded {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
			return "", "", errors.New("document contains binary or control characters")
		}
	}
	label := "utf-16be"
	if little {
		label = "utf-16le"
	}
	return decoded, label, nil
}

func originalOffset(decoded, charset string, position int, raw []byte) (int, error) {
	if position < 0 || position > len(decoded) || !utf8.ValidString(decoded[:position]) {
		return 0, errors.New("invalid decoded offset")
	}
	bom := 0
	if bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		bom = 3
	}
	if bytes.HasPrefix(raw, []byte{0xff, 0xfe}) || bytes.HasPrefix(raw, []byte{0xfe, 0xff}) {
		bom = 2
	}
	prefix := decoded[:position]
	switch charset {
	case "utf-8":
		return bom + len(prefix), nil
	case "windows-1252", "cp1252":
		encoded, _, err := transform.String(charmap.Windows1252.NewEncoder(), prefix)
		return bom + len(encoded), err
	case "iso-8859-1", "latin1":
		encoded, _, err := transform.String(charmap.ISO8859_1.NewEncoder(), prefix)
		return bom + len(encoded), err
	case "utf-16le", "utf-16be":
		n := bom
		for _, r := range prefix {
			if r > 0xffff {
				n += 4
			} else {
				n += 2
			}
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unsupported offset charset %q", charset)
	}
}

func extractText(decoded string, charset string) (Result, error) {
	result := Result{}
	start := 0
	for start < len(decoded) {
		breakAt := paragraphBreak.FindStringIndex(decoded[start:])
		end := len(decoded)
		if breakAt != nil {
			end = start + breakAt[0]
		}
		part := decoded[start:end]
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			key := fmt.Sprintf("paragraph-%d", len(result.Blocks)+1)
			left := strings.Index(part, trimmed)
			block := Block{Key: key, Kind: "paragraph", Text: trimmed, StartByte: start + left, EndByte: start + left + len(trimmed)}
			result.Blocks = append(result.Blocks, block)
		}
		if breakAt == nil {
			break
		}
		start += breakAt[1]
	}
	return result, nil
}

func extractHTML(decoded string) (Result, error) {
	z := html.NewTokenizer(strings.NewReader(decoded))
	result := Result{}
	hasTable := false
	type frame struct {
		tag    string
		hidden bool
	}
	stack := []frame{}
	var current *Block
	var content strings.Builder
	heading := ""
	headings := make([]string, 6)
	headingLevel := 0
	pendingAnchor := ""
	keys := map[string]int{}
	offset := 0
	flush := func() {
		if current == nil {
			return
		}
		lines := strings.Split(content.String(), "\n")
		for i := range lines {
			lines[i] = whitespace.ReplaceAllString(strings.TrimSpace(lines[i]), " ")
		}
		current.Text = strings.TrimSpace(strings.Join(lines, "\n"))
		if current.Text != "" {
			if pendingAnchor != "" {
				current.Key = pendingAnchor
				pendingAnchor = ""
			}
			base := current.Key
			keys[base]++
			if keys[base] > 1 {
				current.Key = fmt.Sprintf("%s-%d", base, keys[base])
			}
			if current.Kind == "heading" {
				if headingLevel > 0 {
					headings[headingLevel-1] = current.Text
					for i := headingLevel; i < len(headings); i++ {
						headings[i] = ""
					}
					path := []string{}
					for _, part := range headings {
						if part != "" {
							path = append(path, part)
						}
					}
					heading = strings.Join(path, " > ")
				} else {
					heading = current.Text
				}
			} else {
				current.Heading = heading
			}
			result.Blocks = append(result.Blocks, *current)
		}
		current = nil
		content.Reset()
	}
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if err := z.Err(); err != nil && err != io.EOF {
				return Result{}, err
			}
			break
		}
		raw := z.Raw()
		begin := offset
		offset += len(raw)
		tok := z.Token()
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			tag := strings.ToLower(tok.Data)
			if tag == "table" {
				hasTable = true
			}
			parentHidden := len(stack) > 0 && stack[len(stack)-1].hidden
			hidden := parentHidden || isHiddenTag(tag, tok.Attr)
			if !hidden && blockKind(tag) != "" {
				// Legacy HTML often omits closing paragraph tags. Finish the
				// previous span before starting another block, rather than saving
				// an invalid zero end offset that prevents document import.
				if current != nil {
					current.EndByte = begin
				}
				flush()
				key := fmt.Sprintf("block-%d", len(result.Blocks)+1)
				for _, a := range tok.Attr {
					if a.Key == "id" && a.Val != "" {
						key = a.Val
					}
				}
				headingLevel = 0
				if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
					headingLevel = int(tag[1] - '0')
				}
				current = &Block{Key: key, Kind: blockKind(tag), StartByte: begin}
			}
			if !hidden && tag == "a" {
				for _, attr := range tok.Attr {
					if (attr.Key == "name" || attr.Key == "id") && attr.Val != "" && (current == nil || strings.TrimSpace(content.String()) == "") {
						pendingAnchor = attr.Val
					}
				}
			}
			if tt == html.StartTagToken && !isVoid(tag) {
				stack = append(stack, frame{tag, hidden})
			}
			if tag == "br" && current != nil {
				content.WriteString("\n")
			}
		} else if tt == html.EndTagToken {
			tag := strings.ToLower(tok.Data)
			if current != nil && blockKind(tag) == current.Kind {
				current.EndByte = offset
				flush()
			}
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].tag == tag {
					stack = stack[:i]
					break
				}
			}
		} else if tt == html.TextToken && current != nil && !(len(stack) > 0 && stack[len(stack)-1].hidden) {
			content.WriteString(" ")
			content.WriteString(whitespace.ReplaceAllString(tok.Data, " "))
		}
	}
	if current != nil {
		current.EndByte = offset
		flush()
	}
	if len(result.Blocks) > 0 {
		result.Warnings = append(result.Warnings, "HTML block boundaries require review before exact citations")
	}
	if hasTable {
		result.Warnings = append(result.Warnings, "Table cells are extracted in reading order; row and column relationships, repertory rubrics, and grades are not verified")
	}
	return result, nil
}

func blockKind(tag string) string {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	case "p", "li", "blockquote", "pre", "td", "th", "caption":
		return "paragraph"
	}
	return ""
}
func isVoid(tag string) bool {
	switch tag {
	case "br", "meta", "link", "img", "input", "hr", "source", "area", "base", "embed", "wbr":
		return true
	}
	return false
}
func isHiddenTag(tag string, attrs []html.Attribute) bool {
	switch tag {
	case "script", "style", "noscript", "template", "svg", "form", "nav", "footer", "header", "aside", "iframe":
		return true
	}
	for _, a := range attrs {
		if a.Key == "hidden" || (a.Key == "aria-hidden" && strings.EqualFold(a.Val, "true")) {
			return true
		}
		if a.Key == "role" && strings.EqualFold(a.Val, "navigation") {
			return true
		}
		if a.Key == "class" || a.Key == "id" {
			for _, part := range strings.FieldsFunc(strings.ToLower(a.Val), func(r rune) bool { return r == ' ' || r == '-' || r == '_' }) {
				switch part {
				case "nav", "navbar", "menu", "sidebar", "breadcrumbs", "cookiebanner":
					return true
				}
			}
		}
	}
	return false
}
