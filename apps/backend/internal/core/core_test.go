package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStarterFarringtonUsesItsOwnPageMap(t *testing.T) {
	// Keep this unit test independent of the optional downloaded corpus.
	root := t.TempDir()
	corpus := filepath.Join(root, "data", "starter-corpus")
	if err := os.MkdirAll(corpus, 0700); err != nil {
		t.Fatal(err)
	}
	writeJSON := func(name string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(corpus, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("manifest.json", Manifest{Sources: []Seed{
		{SourceKey: "nash-1899", Author: "E. B. Nash", PageMapPath: "data/starter-corpus/nash-pages.json"},
		{SourceKey: "farrington-1890-b21118838", Author: "E. A. Farrington", PDFPageCount: 779, PageMapPath: "data/starter-corpus/farrington-pages.json"},
	}})
	// A different map for Nash catches accidentally selecting the first source.
	writeJSON("nash-pages.json", PageMap{Pages: []MappedPage{{ScanPageIndex: 0, PDFPageIndex: 0}}})
	pages := make([]MappedPage, 778)
	for i := range pages {
		pages[i] = MappedPage{ScanPageIndex: i, PDFPageIndex: i + 1}
	}
	writeJSON("farrington-pages.json", PageMap{Pages: pages})
	s := &Store{Root: root}
	seed, err := s.Starter("farrington")
	if err != nil {
		t.Fatal(err)
	}
	exact, err := s.Starter("farrington-1890-b21118838")
	if err != nil || exact.SourceKey != seed.SourceKey {
		t.Fatalf("exact source key lookup failed: %v", err)
	}
	if seed.PDFPageCount != 779 || seed.Author == "" {
		t.Fatalf("unexpected Farrington metadata: %#v", seed)
	}
	mapping, err := s.PageMap(seed)
	if err != nil {
		t.Fatal(err)
	}
	if len(mapping.Pages) != 778 {
		t.Fatalf("got %d mapped scans", len(mapping.Pages))
	}
	if mapping.Pages[48].PDFPageIndex != 49 || mapping.Pages[48].ScanPageIndex != 48 {
		t.Fatalf("unexpected inspected page mapping: %#v", mapping.Pages[48])
	}
}
