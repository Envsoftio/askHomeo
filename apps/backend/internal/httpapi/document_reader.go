package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	xhtml "golang.org/x/net/html"
	"homeopath-poc/backend/internal/document"
)

var readerColor = regexp.MustCompile(`^(?:#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?|[a-zA-Z]{3,20})$`)
var readerID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,100}$`)

// documentReader renders a saved revision with a strict allowlist. The raw
// download remains available for byte inspection; this view preserves visible
// emphasis without loading document code, images, stylesheets or fonts.
func (a *API) documentReader(w http.ResponseWriter, r *http.Request) {
	sourceID, ok := parseID(w, r)
	if !ok {
		return
	}
	revisionID, err := uuid.Parse(r.PathValue("revision"))
	if err != nil {
		fail(w, 400, "invalid revision ID")
		return
	}
	var sha, relative, format, status, rights, contentType, charset string
	err = a.Store.DB.QueryRow(r.Context(), `SELECT sa.sha256,sa.object_locator,sa.kind,s.status,s.rights_status,s.document_content_type,s.document_charset_override FROM processing_revisions rev JOIN source_assets sa ON sa.id=rev.source_asset_id JOIN sources s ON s.id=rev.source_id WHERE rev.id=$1 AND rev.source_id=$2 AND sa.kind IN ('html','txt')`, revisionID, sourceID).Scan(&sha, &relative, &format, &status, &rights, &contentType, &charset)
	if err != nil {
		fail(w, 404, "document not found")
		return
	}
	if requestRole(r.Context()) != "admin" && (status != "published" || rights != "allowed") {
		fail(w, 403, "document unavailable")
		return
	}
	if relative != filepath.Join("data/runtime/assets", sha+"."+format) {
		fail(w, 409, "saved document location differs from checksum")
		return
	}
	raw, err := os.ReadFile(filepath.Join(a.Store.Root, relative))
	if err != nil {
		fail(w, 503, "saved document unavailable")
		return
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != sha {
		fail(w, 409, "saved document checksum differs")
		return
	}
	decoded, err := document.DecodeOriginal(raw, contentType, charset)
	if err != nil {
		fail(w, 409, "saved document encoding needs review")
		return
	}
	var body string
	if format == "html" {
		body, err = safeReaderHTML(decoded)
		if err != nil {
			fail(w, 409, "saved markup needs review")
			return
		}
	} else {
		body = "<pre>" + html.EscapeString(decoded) + "</pre>"
	}
	if r.URL.Query().Has("block") || r.URL.Query().Has("start") || r.URL.Query().Has("end") {
		blockID, blockErr := uuid.Parse(r.URL.Query().Get("block"))
		start, startErr := strconv.Atoi(r.URL.Query().Get("start"))
		end, endErr := strconv.Atoi(r.URL.Query().Get("end"))
		if blockErr != nil || startErr != nil || endErr != nil || start < 0 || end <= start {
			fail(w, 400, "invalid saved passage target")
			return
		}
		var original, reviewed, reviewStatus string
		err = a.Store.DB.QueryRow(r.Context(), `SELECT original_text,reviewed_text,review_status FROM document_blocks WHERE id=$1 AND source_id=$2 AND processing_revision_id=$3`, blockID, sourceID, revisionID).Scan(&original, &reviewed, &reviewStatus)
		if err != nil || (requestRole(r.Context()) != "admin" && reviewStatus != "accepted" && reviewStatus != "corrected") {
			fail(w, 404, "saved passage target unavailable in this revision")
			return
		}
		target, targetErr := readerPassageTarget(original, reviewed, start, end)
		if targetErr != nil {
			fail(w, 409, "saved passage target differs from reviewed text")
			return
		}
		body = target + "<h2>Saved original document</h2>" + body
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = fmt.Fprint(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>Saved original</title><style>body{max-width:80ch;margin:2rem auto;padding:0 1rem;font:16px/1.6 Georgia,serif;color:#20252a}table{border-collapse:collapse}td,th{border:1px solid #aaa;padding:.3rem}pre{white-space:pre-wrap}a[id]{scroll-margin-top:1rem}</style></head><body>", body, "</body></html>")
}

// Target offsets are Unicode character offsets in the immutable reviewed block,
// not DOM offsets. Keep the exact selection separate from original typography:
// a reviewed correction must never be presented as original source markup.
func readerPassageTarget(original, reviewed string, start, end int) (string, error) {
	runes := []rune(reviewed)
	if start < 0 || end <= start || end > len(runes) {
		return "", fmt.Errorf("invalid passage offsets")
	}
	note := ""
	if original != reviewed {
		note = "<p>This reviewed block differs from the extracted original. Compare it with the saved original below.</p>"
	}
	return `<section id="saved-passage"><h2>Exact reviewed passage</h2>` + note +
		"<pre>" + html.EscapeString(string(runes[:start])) + "<mark>" + html.EscapeString(string(runes[start:end])) +
		"</mark>" + html.EscapeString(string(runes[end:])) + "</pre></section>", nil
}

func documentReaderURL(sourceID, revisionID, blockID uuid.UUID, start, end int) string {
	return fmt.Sprintf("/api/v1/sources/%s/document/%s/reader?block=%s&start=%d&end=%d#saved-passage", sourceID, revisionID, blockID, start, end)
}

func safeReaderHTML(decoded string) (string, error) {
	root, err := xhtml.Parse(strings.NewReader(decoded))
	if err != nil {
		return "", err
	}
	var body *xhtml.Node
	var findBody func(*xhtml.Node)
	findBody = func(n *xhtml.Node) {
		if body != nil {
			return
		}
		if n.Type == xhtml.ElementNode && n.Data == "body" {
			body = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			findBody(child)
		}
	}
	findBody(root)
	if body == nil {
		return "", fmt.Errorf("HTML body missing")
	}
	var output bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		writeSafeNode(&output, child)
	}
	return output.String(), nil
}

func writeSafeNode(out *bytes.Buffer, node *xhtml.Node) {
	if node.Type == xhtml.TextNode {
		out.WriteString(html.EscapeString(node.Data))
		return
	}
	if node.Type != xhtml.ElementNode {
		return
	}
	tag := strings.ToLower(node.Data)
	switch tag {
	case "script", "style", "link", "iframe", "frame", "frameset", "object", "embed", "svg", "math", "img", "video", "audio", "source", "picture", "form", "input", "button", "textarea", "select", "template", "noscript", "canvas":
		return
	}
	if tag == "dir" {
		tag = "ul"
	}
	allowed := false
	switch tag {
	case "p", "div", "span", "section", "article", "main", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "dl", "dt", "dd", "table", "thead", "tbody", "tfoot", "tr", "td", "th", "blockquote", "pre", "code", "b", "strong", "i", "em", "u", "s", "small", "big", "sup", "sub", "font", "a", "br", "hr":
		allowed = true
	}
	if allowed {
		out.WriteByte('<')
		out.WriteString(tag)
		for _, attr := range node.Attr {
			if (attr.Key == "id" || (tag == "a" && attr.Key == "name")) && readerID.MatchString(attr.Val) {
				out.WriteString(` id="`)
				out.WriteString(html.EscapeString(attr.Val))
				out.WriteByte('"')
				break
			}
		}
		if tag == "font" {
			for _, attr := range node.Attr {
				if attr.Key == "color" && readerColor.MatchString(attr.Val) {
					out.WriteString(` style="color:`)
					out.WriteString(attr.Val)
					out.WriteByte('"')
					break
				}
			}
		}
		out.WriteByte('>')
	}
	if tag != "br" && tag != "hr" {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			writeSafeNode(out, child)
		}
		if allowed {
			out.WriteString("</")
			out.WriteString(tag)
			out.WriteByte('>')
		}
	}
}
