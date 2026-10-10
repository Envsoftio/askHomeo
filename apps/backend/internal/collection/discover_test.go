package collection

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"homeopath-poc/backend/internal/safefetch"
)

type fixtureFetcher struct {
	pages map[string]string
	calls []string
}

func (f *fixtureFetcher) FetchScoped(_ context.Context, raw string, _ int64, _ bool, allowed func(*url.URL) error) (*safefetch.Result, error) {
	u, err := url.Parse(raw)
	if err != nil || allowed(u) != nil {
		return nil, errors.New("fixture fetch outside scope")
	}
	f.calls = append(f.calls, raw)
	body, ok := f.pages[raw]
	if !ok {
		return nil, errors.New("fixture missing")
	}
	return &safefetch.Result{Body: io.NopCloser(strings.NewReader(body)), RequestedURL: raw, FinalURL: raw, ContentType: "text/html"}, nil
}

func fixtureScope() Scope {
	return Scope{SeedURL: "https://books.example/book/index.html", AllowedHosts: []string{"books.example"}, AllowedPathPrefixes: []string{"/book/", "/sibling/"}, MaxDocuments: 5, MaxDepth: 3, MaxTotalBytes: 1 << 20, MaxDurationSeconds: 10}
}

func TestLinkedVolumesStayWithinBookFamily(t *testing.T) {
	s := fixtureScope()
	s.AllowedPathPrefixes = []string{"/books/kentrep/"}
	s.SeedURL = "https://books.example/books/kentrep/index.htm"
	s.IncludeLinkedVolumes = true
	s.MaxDocuments, s.MaxDepth, s.MaxTotalBytes, s.MaxDurationSeconds = 0, 0, 0, 0
	if _, err := s.ValidateCapture(); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/books/kentrep/kent0300.htm", "/books/kentrep1/kent0305.htm", "/books/kentrep3/kent1420.htm"} {
		u, _ := url.Parse("https://books.example" + target)
		if err := s.AllowURL(u); err != nil {
			t.Fatalf("lost linked volume %s: %v", target, err)
		}
	}
	for _, target := range []string{"/books/other/index.htm", "/books/kentrep-ad/index.htm", "/books/kentrep3/../other/index.htm", "/books/kentrep3evil/a.htm", "/books/kentrep3%2fother/a.htm"} {
		u, _ := url.Parse("https://books.example" + target)
		if s.AllowURL(u) == nil {
			t.Fatalf("allowed unrelated/ambiguous page %s", target)
		}
	}
	s.IncludeLinkedVolumes = false
	u, _ := url.Parse("https://books.example/books/kentrep3/kent1420.htm")
	if s.AllowURL(u) == nil {
		t.Fatal("explicit scope was broadened without linked volumes enabled")
	}
}

func TestPreviewFollowsSiblingPathsAndRetainsMultipleAnchors(t *testing.T) {
	f := &fixtureFetcher{pages: map[string]string{
		"https://books.example/book/index.html":     `<html><body><a href="section.html">Section</a><a href="/outside/a.html">Advertisement</a></body></html>`,
		"https://books.example/book/section.html":   `<html><body><a href="../sibling/remedy.html#one">A</a><a href="../sibling/remedy.html#two">B</a><a href="../sibling/remedy.html#absent">Missing</a></body></html>`,
		"https://books.example/sibling/remedy.html": `<html><body><h2 id="one">Fear</h2><p id="two">Aconite</p><a href="/book/index.html">Index</a></body></html>`,
	}}
	m, err := Preview(context.Background(), fixtureScope(), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 || m.Fetched != 3 || len(m.Entries) != 3 || m.Complete {
		t.Fatalf("unexpected coverage: %+v calls=%v", m, f.calls)
	}
	var resolved, missing, excluded, cycle int
	for _, link := range m.Links {
		switch link.State {
		case "resolved":
			resolved++
		case "missing_anchor":
			missing++
		case "excluded_scope":
			excluded++
		case "duplicate_or_cycle":
			cycle++
		}
	}
	if resolved != 3 || missing != 1 || excluded != 1 || cycle != 1 {
		t.Fatalf("unexpected link states: %+v", m.Links)
	}
	if !contains(m.Entries[2].Anchors, "one") || !contains(m.Entries[2].Anchors, "two") {
		t.Fatalf("anchors lost: %+v", m.Entries[2])
	}
}

func TestPreviewReportsLimitsAndFailedTargets(t *testing.T) {
	f := &fixtureFetcher{pages: map[string]string{
		"https://books.example/book/index.html": `<html><body><a href="a.html">A</a><a href="b.html">B</a></body></html>`,
		"https://books.example/book/a.html":     `<html><body><p>Content</p></body></html>`,
	}}
	s := fixtureScope()
	s.MaxDocuments = 2
	m, err := Preview(context.Background(), s, f)
	if err != nil {
		t.Fatal(err)
	}
	if m.Complete || !contains(m.LimitReasons, "document limit reached") || len(f.calls) != 2 {
		t.Fatalf("document limit failed: %+v calls=%v", m, f.calls)
	}
	s.MaxDocuments = 5
	f.calls = nil
	m, err = Preview(context.Background(), s, f)
	if err != nil {
		t.Fatal(err)
	}
	if m.Complete || m.Failed != 1 {
		t.Fatalf("missing target treated as complete: %+v", m)
	}
}

func TestPreviewFollowsMoreThanOneHundredPagesBeyondFiveLevels(t *testing.T) {
	const count = 110
	pages := make(map[string]string, count)
	for i := 0; i < count; i++ {
		url := "https://books.example/book/page" + strconv.Itoa(i) + ".html"
		body := "<html><body><p>Book content</p>"
		if i+1 < count {
			body += `<a href="page` + strconv.Itoa(i+1) + `.html">Next</a>`
		}
		pages[url] = body + "</body></html>"
	}
	f := &fixtureFetcher{pages: pages}
	s := fixtureScope()
	s.SeedURL = "https://books.example/book/page0.html"
	s.MaxDocuments, s.MaxDepth = 0, 0
	m, err := Preview(context.Background(), s, f)
	if err != nil || !m.Complete || m.Fetched != count || len(f.calls) != count {
		t.Fatalf("in-scope chain was truncated: fetched=%d calls=%d reasons=%v err=%v", m.Fetched, len(f.calls), m.LimitReasons, err)
	}
}

func TestPreviewPaginationLayoutAndRepeatedAnchorTarget(t *testing.T) {
	f := &fixtureFetcher{pages: map[string]string{
		"https://books.example/book/page1.htm": `<html><head><link rel="next" href="page2.htm"></head><body><a href="page2.htm#r1">First rubric</a><a href="page2.htm#r2">Second rubric</a></body></html>`,
		"https://books.example/book/page2.htm": `<html><body><p id="r1">Short rubric</p><p id="r2">Another rubric</p><a href="page1.htm">Previous</a></body></html>`,
	}}
	s := fixtureScope()
	s.SeedURL = "https://books.example/book/page1.htm"
	m, err := Preview(context.Background(), s, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || m.Fetched != 2 || !m.Complete {
		t.Fatalf("pagination coverage: %+v calls=%v", m, f.calls)
	}
	var pagination, anchored int
	for _, link := range m.Links {
		if link.Relation == "pagination" {
			pagination++
		}
		if link.Fragment != "" && link.State == "resolved" {
			anchored++
		}
	}
	if pagination < 1 || anchored != 2 {
		t.Fatalf("pagination or anchor references lost: %+v", m.Links)
	}
}

func TestScopeRequiresExplicitCleanPaths(t *testing.T) {
	s := fixtureScope()
	for _, raw := range []string{"https://books.example/bookish/a.html", "https://other.example/book/a.html", "https://books.example/book/%2e%2e/outside/a.html", "https://books.example/book/%252e%252e/outside/a.html"} {
		u, _ := url.Parse(raw)
		if err := s.AllowURL(u); err == nil {
			t.Fatalf("accepted out-of-scope URL %s", raw)
		}
	}
	s.AllowedPathPrefixes = []string{"/book/../outside/"}
	if _, err := s.Validate(); err == nil {
		t.Fatal("accepted ambiguous path prefix")
	}
}

func TestPreviewDoesNotCallEmptyShellComplete(t *testing.T) {
	f := &fixtureFetcher{pages: map[string]string{
		"https://books.example/book/index.html": `<html><body><script>renderBook()</script></body></html>`,
	}}
	m, err := Preview(context.Background(), fixtureScope(), f)
	if err != nil {
		t.Fatal(err)
	}
	if m.Complete || len(m.Entries) != 1 || m.Entries[0].Role != "unresolved" || !contains(m.LimitReasons, "unresolved content extraction") {
		t.Fatalf("empty shell appeared complete: %+v", m)
	}
}

func TestLegacyContentAndBrokenEncodingAreNotIndexPages(t *testing.T) {
	raw := []byte("<html><p>M\xe9di-T repertory</p><a href='chapter.htm'>M\xe9moire</a></html>")
	entry, links, err := Inspect(raw, "text/html", "https://books.example/book/index.htm")
	if err != nil || entry.Role != "content_candidate" || entry.BlockCount == 0 || entry.Charset != "windows-1252" || !strings.Contains(entry.Sample, "Médi-T") {
		t.Fatalf("lost legacy content: %+v %v", entry, err)
	}
	if len(links) != 1 || links[0].Text != "Mémoire" {
		t.Fatalf("broken link decoding: %+v", links)
	}
	entry, _, err = Inspect(raw, "text/html; charset=utf-8", "https://books.example/book/index.htm")
	if err != nil || entry.Role != "unresolved" {
		t.Fatalf("encoding failure became navigation: %+v %v", entry, err)
	}
}

func TestPreviewDiscoversContinuationAfterManyAnchorLinks(t *testing.T) {
	var body strings.Builder
	body.WriteString("<html><p>Index</p>")
	for i := 0; i < 510; i++ {
		body.WriteString(`<a href="#section">Section</a>`)
	}
	body.WriteString(`<a name="section"></a><a href="continuation.htm">Next</a></html>`)
	f := &fixtureFetcher{pages: map[string]string{
		"https://books.example/book/index.html":       body.String(),
		"https://books.example/book/continuation.htm": "<html><p>Continuation evidence.</p></html>",
	}}
	s := fixtureScope()
	s.MaxDocuments = 50
	m, err := Preview(context.Background(), s, f)
	if err != nil || !m.Complete || m.Fetched != 2 {
		t.Fatalf("lost continuation beyond repeated links: %+v %v", m, err)
	}
}

func TestNavigationOnlyIndexDoesNotBecomeEvidence(t *testing.T) {
	raw := []byte(`<html><p><a href="a">Alpha</a></p><p><a href="b">Beta</a></p><p><a href="c">Gamma</a></p><p><a href="d">Delta</a></p><p><a href="e">Epsilon</a></p></html>`)
	e, links, err := Inspect(raw, "text/html", "https://books.example/index.htm")
	if err != nil || e.Role != "index_only" || e.BlockCount != 0 || e.ExcludedBlocks != 5 || e.Sample != "" || len(links) != 5 {
		t.Fatalf("navigation became evidence: %+v links=%d err=%v", e, len(links), err)
	}
}
