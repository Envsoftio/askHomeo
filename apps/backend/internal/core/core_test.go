package core

import (
	"path/filepath"
	"testing"
)

func TestStarterFarringtonUsesItsOwnPageMap(t *testing.T) {
	s := &Store{Root: filepath.Join("..", "..", "..", "..")}
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
