package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteManagedFigureOnlyFromContentAddressedPath(t *testing.T) {
	root := t.TempDir()
	sha := strings.Repeat("a", 64)
	rel := filepath.Join("data/runtime/assets/figures", sha+".png")
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("figure"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Root: root}
	if err := s.deleteManagedAsset(context.Background(), "figure_png", sha, "elsewhere.png"); err == nil {
		t.Fatal("accepted unrelated locator")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("rejected locator removed file")
	}
	if err := s.deleteManagedAsset(context.Background(), "figure_png", sha, rel); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("figure remains: %v", err)
	}
}
