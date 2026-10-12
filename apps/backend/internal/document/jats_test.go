package document

import (
	"os"
	"strings"
	"testing"
)

func TestJATSLiveFixtureWhenProvided(t *testing.T) {
	path := os.Getenv("LIVE_JATS_FIXTURE")
	if path == "" {
		t.Skip("set LIVE_JATS_FIXTURE to a temporary saved OA JATS article")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ExtractJATS(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Blocks) < 20 {
		t.Fatalf("too few reviewable blocks: %d", len(result.Blocks))
	}
	for _, b := range result.Blocks {
		if b.StartByte < 0 || b.EndByte > len(raw) || b.EndByte <= b.StartByte {
			t.Fatalf("invalid original span %+v", b)
		}
	}
}

func TestJATSBlocksAndOriginalOffsets(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><!DOCTYPE article SYSTEM "https://example.org/never-fetch.dtd"><article><front><article-meta><title-group><article-title>Indian trial</article-title></title-group><abstract><p>Planned outcome only.</p></abstract></article-meta></front><body><sec><title>Results</title><p>Twenty participants; no benefit.</p><table-wrap><caption><p>Change (mm Hg)</p></caption><table><thead><tr><th>Arm</th><th>Difference (mm Hg)</th></tr></thead><tbody><tr><td>A</td><td>−2 (95% CI −5 to 1)</td></tr></tbody></table><table-wrap-foot><p>n=10 per arm.</p></table-wrap-foot></table-wrap></sec></body><back><ref-list><ref>Author 2020</ref></ref-list></back></article>`)
	out, err := ExtractJATS(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Indian trial" || out.Format != "jats" {
		t.Fatalf("wrong metadata: %+v", out)
	}
	kinds := map[string]bool{}
	for _, b := range out.Blocks {
		kinds[b.Kind] = true
		if b.StartByte < 0 || b.EndByte > len(raw) || b.StartByte >= b.EndByte {
			t.Fatalf("bad raw offset %+v", b)
		}
		if !strings.Contains(string(raw[b.StartByte:b.EndByte]), strings.Split(b.Text, " ")[0]) {
			t.Fatalf("raw span misses block %q", b.Text)
		}
	}
	for _, kind := range []string{"heading", "paragraph", "table", "table_footnote", "reference"} {
		if !kinds[kind] {
			t.Errorf("missing %s", kind)
		}
	}
}
func TestJATSRejectsEntitiesAndWrongRoot(t *testing.T) {
	for _, raw := range []string{`<!DOCTYPE article [<!ENTITY x SYSTEM "file:///etc/passwd">]><article><p>&x;</p></article>`, `<metadata><p>Not an article</p></metadata>`, `<article><p>broken</article>`, `<article><p>empty</p>`} {
		if _, err := ExtractJATS([]byte(raw)); err == nil {
			t.Errorf("accepted unsafe/malformed XML %q", raw)
		}
	}
}
