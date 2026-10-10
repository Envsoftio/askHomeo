// Package collection previews a bounded set of linked public documents. A
// preview is a manifest only: it does not publish or index fetched content.
package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"homeopath-poc/backend/internal/classify"
	"homeopath-poc/backend/internal/document"
	"homeopath-poc/backend/internal/safefetch"
)

type Fetcher interface {
	FetchScoped(context.Context, string, int64, bool, func(*url.URL) error) (*safefetch.Result, error)
}

type Scope struct {
	SeedURL              string   `json:"seed_url"`
	AllowedHosts         []string `json:"allowed_hosts"`
	AllowedPathPrefixes  []string `json:"allowed_path_prefixes"`
	MaxDocuments         int      `json:"max_documents"`
	MaxDepth             int      `json:"max_depth"`
	MaxTotalBytes        int64    `json:"max_total_bytes"`
	MaxDurationSeconds   int      `json:"max_duration_seconds"`
	RequestDelayMillis   int      `json:"request_delay_millis"`
	AllowHTTPRedirect    bool     `json:"allow_https_to_http_redirect"`
	AllowUnencryptedHTTP bool     `json:"allow_unencrypted_http"`
}

type Entry struct {
	ExcludedBlocks       int      `json:"excluded_blocks,omitempty"`
	SuggestedCategories  []string `json:"suggested_categories,omitempty"`
	ClassificationReason string   `json:"classification_reason,omitempty"`
	URL                  string   `json:"url"`
	FinalURL             string   `json:"final_url,omitempty"`
	Depth                int      `json:"depth"`
	State                string   `json:"state"`
	Role                 string   `json:"role,omitempty"`
	Format               string   `json:"format,omitempty"`
	ContentType          string   `json:"content_type,omitempty"`
	ByteSize             int      `json:"byte_size,omitempty"`
	SHA256               string   `json:"sha256,omitempty"`
	BlockCount           int      `json:"block_count,omitempty"`
	Charset              string   `json:"charset,omitempty"`
	Sample               string   `json:"sample,omitempty"`
	Anchors              []string `json:"anchors,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
	Error                string   `json:"error,omitempty"`
}

type Link struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Fragment string `json:"fragment,omitempty"`
	Text     string `json:"text,omitempty"`
	Relation string `json:"relation"`
	State    string `json:"state"`
}

type Manifest struct {
	Entries      []Entry  `json:"entries"`
	Links        []Link   `json:"links"`
	Fetched      int      `json:"fetched"`
	Failed       int      `json:"failed"`
	TotalBytes   int64    `json:"total_bytes"`
	Complete     bool     `json:"complete"`
	LimitReasons []string `json:"limit_reasons"`
}

func (s Scope) Validate() (*url.URL, error) {
	return s.validateLimits(100, 30<<20, 180)
}

// ValidateCapture permits a larger, resumable selection than an interactive
// preview while retaining the same exact host/path and transport rules.
func (s Scope) ValidateCapture() (*url.URL, error) {
	u, err := s.validateLimits(100, 100<<20, 3600)
	if err != nil {
		return nil, err
	}
	if (u.Scheme == "http" || s.AllowHTTPRedirect) && !s.AllowUnencryptedHTTP {
		return nil, errors.New("explicit unencrypted HTTP selection is required for capture")
	}
	return u, nil
}

func (s Scope) validateLimits(maxDocuments int, maxBytes int64, maxSeconds int) (*url.URL, error) {
	u, err := url.Parse(s.SeedURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Fragment != "" || u.User != nil {
		return nil, errors.New("seed must be a public HTTP or HTTPS document URL without credentials or fragment")
	}
	if len(s.AllowedHosts) == 0 || len(s.AllowedPathPrefixes) == 0 || s.MaxDocuments < 1 || s.MaxDocuments > maxDocuments ||
		s.MaxDepth < 0 || s.MaxDepth > 5 || s.MaxTotalBytes < 1 || s.MaxTotalBytes > maxBytes ||
		s.MaxDurationSeconds < 1 || s.MaxDurationSeconds > maxSeconds || s.RequestDelayMillis < 0 || s.RequestDelayMillis > 5000 {
		return nil, errors.New("collection scope needs explicit hosts/paths and bounded document, depth, byte, time and delay limits")
	}
	for _, host := range s.AllowedHosts {
		if host == "" || strings.ContainsAny(host, "/:@*?#") || strings.ToLower(host) != host {
			return nil, errors.New("allowed hosts must be exact lowercase hostnames")
		}
	}
	for _, prefix := range s.AllowedPathPrefixes {
		if !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "?#%") ||
			(prefix != "/" && path.Clean(prefix) != strings.TrimSuffix(prefix, "/")) {
			return nil, errors.New("allowed paths must be clean absolute URL path prefixes")
		}
	}
	if err := s.AllowURL(u); err != nil {
		return nil, fmt.Errorf("seed is outside collection scope: %w", err)
	}
	return u, nil
}

func (s Scope) AllowURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return errors.New("URL is outside collection scope")
	}
	escaped := strings.ToLower(u.EscapedPath())
	cleanPath := u.Path
	if cleanPath == "" {
		cleanPath = "/"
	}
	if cleanPath != "/" && path.Clean(cleanPath) != strings.TrimSuffix(cleanPath, "/") ||
		strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") || strings.Contains(escaped, "%2e") || strings.Contains(escaped, "%25") {
		return errors.New("ambiguous URL path is outside collection scope")
	}
	hostOK := false
	for _, host := range s.AllowedHosts {
		if strings.EqualFold(u.Hostname(), host) {
			hostOK = true
			break
		}
	}
	if !hostOK {
		return errors.New("host is outside collection scope")
	}
	for _, prefix := range s.AllowedPathPrefixes {
		if u.EscapedPath() == prefix || strings.HasPrefix(u.EscapedPath(), strings.TrimSuffix(prefix, "/")+"/") ||
			(strings.HasSuffix(prefix, "/") && strings.HasPrefix(u.EscapedPath(), prefix)) {
			return nil
		}
	}
	return errors.New("path is outside collection scope")
}

type queued struct {
	url   string
	depth int
}

func Preview(ctx context.Context, s Scope, fetcher Fetcher) (Manifest, error) {
	seed, err := s.Validate()
	if err != nil {
		return Manifest{}, err
	}
	if fetcher == nil {
		fetcher = &safefetch.Fetcher{}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.MaxDurationSeconds)*time.Second)
	defer cancel()
	manifest := Manifest{Entries: []Entry{}, Links: []Link{}, LimitReasons: []string{}}
	queue := []queued{{seed.String(), 0}}
	seen := map[string]bool{seed.String(): true}
	finalSeen := map[string]bool{}
	attempts := 0
previewLoop:
	for len(queue) > 0 {
		if ctx.Err() != nil {
			manifest.LimitReasons = appendUnique(manifest.LimitReasons, "time limit reached")
			break
		}
		if len(manifest.Entries) >= s.MaxDocuments {
			manifest.LimitReasons = appendUnique(manifest.LimitReasons, "document limit reached")
			break
		}
		if manifest.TotalBytes >= s.MaxTotalBytes {
			manifest.LimitReasons = appendUnique(manifest.LimitReasons, "byte limit reached")
			break
		}
		item := queue[0]
		queue = queue[1:]
		if attempts > 0 && s.RequestDelayMillis > 0 {
			select {
			case <-ctx.Done():
				manifest.LimitReasons = appendUnique(manifest.LimitReasons, "time limit reached")
				break previewLoop
			case <-time.After(time.Duration(s.RequestDelayMillis) * time.Millisecond):
			}
		}
		attempts++
		entry := Entry{URL: item.url, Depth: item.depth, State: "failed"}
		remaining := s.MaxTotalBytes - manifest.TotalBytes
		limit := int64(document.MaxBytes)
		if remaining < limit {
			limit = remaining
		}
		result, fetchErr := fetcher.FetchScoped(ctx, item.url, limit, s.AllowHTTPRedirect, s.AllowURL)
		if fetchErr != nil {
			entry.Error = fetchErr.Error()
			manifest.Failed++
			manifest.Entries = append(manifest.Entries, entry)
			continue
		}
		data, readErr := io.ReadAll(result.Body)
		result.Body.Close()
		entry.FinalURL, entry.ContentType = result.FinalURL, result.ContentType
		manifest.TotalBytes += int64(len(data))
		entry.ByteSize = len(data)
		if readErr != nil {
			entry.Error = readErr.Error()
			manifest.Failed++
			manifest.Entries = append(manifest.Entries, entry)
			continue
		}
		hash := sha256.Sum256(data)
		entry.SHA256 = hex.EncodeToString(hash[:])
		if finalSeen[result.FinalURL] {
			entry.State = "duplicate_redirect"
			manifest.Entries = append(manifest.Entries, entry)
			continue
		}
		finalSeen[result.FinalURL] = true
		if final, parseErr := url.Parse(result.FinalURL); parseErr == nil {
			seen[final.String()] = true
		}
		inspected, links, inspectErr := Inspect(data, result.ContentType, result.FinalURL)
		if inspectErr != nil {
			entry.Error = inspectErr.Error()
			manifest.Failed++
			manifest.Entries = append(manifest.Entries, entry)
			continue
		}
		entry.Format = inspected.Format
		entry.Charset, entry.Sample = inspected.Charset, inspected.Sample
		entry.ExcludedBlocks, entry.SuggestedCategories, entry.ClassificationReason = inspected.ExcludedBlocks, inspected.SuggestedCategories, inspected.ClassificationReason
		entry.State = "fetched"
		entry.BlockCount, entry.Warnings, entry.Role, entry.Anchors = inspected.BlockCount, inspected.Warnings, inspected.Role, inspected.Anchors
		if entry.Format == "html" {
			for _, link := range links {
				if len(manifest.Links) >= 10000 {
					manifest.LimitReasons = appendUnique(manifest.LimitReasons, "link limit reached")
					break
				}
				if err := s.AllowURL(mustURL(link.To)); err != nil {
					link.State = "excluded_scope"
				} else if seen[link.To] {
					link.State = "duplicate_or_cycle"
				} else if item.depth >= s.MaxDepth {
					link.State = "excluded_depth"
					manifest.LimitReasons = appendUnique(manifest.LimitReasons, "depth limit reached")
				} else if len(seen) >= s.MaxDocuments {
					link.State = "excluded_document_limit"
					manifest.LimitReasons = appendUnique(manifest.LimitReasons, "document limit reached")
				} else {
					link.State = "queued"
					seen[link.To] = true
					queue = append(queue, queued{link.To, item.depth + 1})
				}
				manifest.Links = append(manifest.Links, link)
			}
		}
		if entry.Role == "unresolved" {
			manifest.LimitReasons = appendUnique(manifest.LimitReasons, "unresolved content extraction")
		}
		manifest.Fetched++
		manifest.Entries = append(manifest.Entries, entry)
	}
	byURL := map[string]Entry{}
	for _, entry := range manifest.Entries {
		byURL[entry.URL] = entry
		if entry.FinalURL != "" {
			byURL[entry.FinalURL] = entry
		}
	}
	for i := range manifest.Links {
		link := &manifest.Links[i]
		if link.State != "queued" && link.State != "duplicate_or_cycle" {
			continue
		}
		target, ok := byURL[link.To]
		if !ok {
			link.State = "unfetched"
			continue
		}
		if target.State != "fetched" {
			link.State = "target_" + target.State
			continue
		}
		if link.Fragment != "" && !contains(target.Anchors, link.Fragment) {
			link.State = "missing_anchor"
			manifest.LimitReasons = appendUnique(manifest.LimitReasons, "missing anchors")
			continue
		}
		if link.State == "duplicate_or_cycle" && link.Fragment == "" {
			continue
		}
		link.State = "resolved"
	}
	manifest.Complete = len(manifest.LimitReasons) == 0 && len(queue) == 0 && manifest.Failed == 0
	return manifest, nil
}

// Inspect keeps navigation metadata distinct from reviewable content. It does
// not approve either the extracted blocks or any structural interpretation.
func Inspect(data []byte, contentType, finalURL string) (Entry, []Link, error) {
	format, err := document.Detect(data, contentType)
	if err != nil {
		return Entry{}, nil, err
	}
	entry := Entry{Format: format, Role: "unresolved", Warnings: []string{}}
	noText := false
	if extracted, extractionErr := document.Extract(data, contentType, ""); extractionErr == nil {
		suggestion := classify.Suggest(document.EvidenceText(extracted.Blocks))
		entry.SuggestedCategories, entry.ClassificationReason = suggestion.Categories, suggestion.Reason
		for _, block := range extracted.Blocks {
			if block.ExclusionReason != "" {
				entry.ExcludedBlocks++
			} else {
				entry.BlockCount++
			}
		}
		entry.Charset = extracted.Charset
		for _, block := range extracted.Blocks {
			if block.ExclusionReason != "" {
				continue
			}
			if entry.Sample != "" {
				entry.Sample += "\n"
			}
			entry.Sample += block.Text
			if len([]rune(entry.Sample)) >= 600 {
				entry.Sample = string([]rune(entry.Sample)[:600])
				break
			}
		}
		entry.Warnings = append(entry.Warnings, extracted.Warnings...)
		entry.Role = "content_candidate"
		if entry.BlockCount == 0 {
			noText = true
			entry.Role = "unresolved"
		}
	} else {
		noText = strings.Contains(extractionErr.Error(), "document has no substantive text")
		entry.Warnings = append(entry.Warnings, "No reviewable text was extracted: "+extractionErr.Error())
	}
	if format != "html" {
		return entry, nil, nil
	}
	linkData := data
	if entry.Charset != "" {
		reader, decodeErr := charset.NewReaderLabel(entry.Charset, bytes.NewReader(data))
		if decodeErr != nil {
			return Entry{}, nil, decodeErr
		}
		linkData, err = io.ReadAll(reader)
		if err != nil {
			return Entry{}, nil, err
		}
	}
	anchors, links, err := parseLinks(linkData, finalURL)
	if err != nil {
		return Entry{}, nil, err
	}
	entry.Anchors = anchors
	if noText && len(links) > 0 {
		entry.Role = "index_only"
	}
	return entry, links, nil
}

func mustURL(raw string) *url.URL { u, _ := url.Parse(raw); return u }
func appendUnique(items []string, value string) []string {
	if !contains(items, value) {
		return append(items, value)
	}
	return items
}
func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func parseLinks(raw []byte, baseRaw string) ([]string, []Link, error) {
	base, err := url.Parse(baseRaw)
	if err != nil {
		return nil, nil, err
	}
	root, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}
	anchors := []string{}
	links := []Link{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if (a.Key == "id" || (n.Data == "a" && a.Key == "name")) && a.Val != "" {
					anchors = appendUnique(anchors, a.Val)
				}
			}
			if n.Data == "a" || n.Data == "link" {
				href := ""
				rel := ""
				for _, a := range n.Attr {
					if a.Key == "href" {
						href = a.Val
					}
					if a.Key == "rel" {
						rel = a.Val
					}
				}
				if href != "" && (n.Data == "a" || strings.Contains(strings.ToLower(rel), "next")) {
					ref, e := url.Parse(strings.TrimSpace(href))
					if e == nil && (ref.Scheme == "" || ref.Scheme == "http" || ref.Scheme == "https") && ref.User == nil {
						resolved := base.ResolveReference(ref)
						fragment := resolved.Fragment
						resolved.Fragment = ""
						if resolved.Hostname() != "" && likelyDocument(resolved.Path) {
							text := nodeText(n)
							links = append(links, Link{From: base.String(), To: resolved.String(), Fragment: fragment, Text: text, Relation: relation(text, rel)})
						}
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return anchors, links, nil
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
			b.WriteByte(' ')
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
func likelyDocument(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	switch ext {
	case "", ".htm", ".html", ".txt":
		return true
	}
	return false
}
func relation(text, rel string) string {
	v := strings.ToLower(text + " " + rel)
	switch {
	case strings.Contains(v, "next") || strings.Contains(v, "previous") || strings.Contains(v, "prev"):
		return "pagination"
	case strings.Contains(v, "index") || strings.Contains(v, "contents"):
		return "index"
	case strings.Contains(v, "chapter") || strings.Contains(v, "section"):
		return "section"
	default:
		return "linked_document"
	}
}
