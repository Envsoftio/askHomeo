package paperjobs

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
	"homeopath-poc/backend/internal/document"
)

var wordToken = regexp.MustCompile(`[\p{L}\p{N}]+`)

func titleMatches(expected, observed string) bool {
	expected = strings.ToLower(expected)
	observed = strings.ToLower(observed)
	tokens := wordToken.FindAllString(expected, -1)
	if len(tokens) == 0 {
		return false
	}
	seen := map[string]bool{}
	long, hits := 0, 0
	for _, token := range tokens {
		if len(token) < 4 || seen[token] {
			continue
		}
		seen[token] = true
		long++
		if strings.Contains(observed, token) {
			hits++
		}
	}
	return long >= 3 && hits*5 >= long*4
}

func hasArticleLicense(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "creativecommons.org/licenses/by/4.0/") || strings.Contains(s, "creativecommons.org/publicdomain/zero/1.0/")
}

func validatePDFIdentity(ctx context.Context, raw []byte, doi, title string) error {
	if len(raw) < 16 || len(raw) > 25<<20 || !bytes.HasPrefix(raw, []byte("%PDF-")) {
		return errors.New("candidate is not a bounded PDF")
	}
	if doi == "" {
		return errors.New("DOI is required to verify alternate PDF identity")
	}
	step, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if _, err := exec.LookPath("pdftotext"); err == nil {
		cmd = exec.CommandContext(step, "pdftotext", "-f", "1", "-l", "2", "-enc", "UTF-8", "-", "-")
	} else {
		cmd = exec.CommandContext(step, "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=txtwrite", "-dFirstPage=1", "-dLastPage=2", "-sOutputFile=-", "-")
	}
	cmd.Stdin = bytes.NewReader(raw)
	text, err := cmd.Output()
	if err != nil || len(text) > 2<<20 {
		return errors.New("PDF opening pages could not be checked for article identity")
	}
	opening := strings.ToLower(string(text))
	if !strings.Contains(opening, strings.ToLower(doi)) || !titleMatches(title, opening) {
		return errors.New("PDF opening pages do not verify the selected DOI and title")
	}
	if !hasArticleLicense(opening) {
		return errors.New("PDF opening pages lack an article-level CC BY 4.0 or CC0 statement")
	}
	return nil
}

func validateHTMLIdentity(raw []byte, doi, title string) error {
	if doi == "" {
		return errors.New("DOI is required to verify alternate HTML identity")
	}
	extracted, err := document.Extract(raw, "text/html", "")
	if err != nil || extracted.Format != "html" {
		return errors.New("candidate is not a reviewable HTML article")
	}
	root, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return errors.New("candidate HTML could not be parsed")
	}
	metaDOI, metaTitle, license := "", "", ""
	var articleText strings.Builder
	tooDeep, tooLong := false, false
	var walk func(*html.Node, bool, int)
	walk = func(n *html.Node, inArticle bool, depth int) {
		if depth > 128 {
			tooDeep = true
			return
		}
		if n.Type == html.ElementNode {
			if n.Data == "article" {
				inArticle = true
			}
			if n.Data == "meta" || n.Data == "link" {
				attrs := map[string]string{}
				for _, a := range n.Attr {
					attrs[strings.ToLower(a.Key)] = strings.TrimSpace(a.Val)
				}
				key := strings.ToLower(attrs["name"])
				if key == "citation_doi" || key == "dc.identifier.doi" {
					metaDOI = strings.ToLower(strings.TrimPrefix(attrs["content"], "https://doi.org/"))
				}
				if key == "citation_title" {
					metaTitle = attrs["content"]
				}
				if strings.EqualFold(attrs["rel"], "license") {
					license = attrs["href"]
				}
			}
		}
		if n.Type == html.TextNode && inArticle && !tooLong {
			if articleText.Len()+len(n.Data) > 2<<20 {
				tooLong = true
			} else {
				articleText.WriteString(n.Data)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inArticle, depth+1)
		}
	}
	walk(root, false, 0)
	if metaDOI != strings.ToLower(doi) || !titleMatches(title, metaTitle) {
		return errors.New("HTML article metadata differs from selected DOI and title")
	}
	if !hasArticleLicense(license) {
		return errors.New("HTML article lacks an article-level CC BY 4.0 or CC0 license link")
	}
	letters := 0
	for _, r := range articleText.String() {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 500 || tooLong || tooDeep {
		return errors.New("HTML candidate lacks bounded article full text")
	}
	return nil
}
