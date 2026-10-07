package ingest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestMeasurePGMRejectsIncompleteRender(t *testing.T) {
	if _, err := measurePGM([]byte("P5\n2 2\n255\n\x80")); err == nil {
		t.Fatal("truncated render was accepted")
	}
}

func TestNashScanTriage(t *testing.T) {
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skip("Ghostscript not installed")
	}
	pdf := filepath.Join("..", "..", "..", "..", "data", "starter-corpus", "originals", "nash-1899.pdf")
	if _, err := os.Stat(pdf); err != nil {
		t.Skip("Nash starter PDF not available")
	}
	for _, tc := range []struct {
		scan  int
		blank bool
	}{
		{1, false},   // patterned cover: OCR is empty
		{4, true},    // empty paper
		{5, false},   // handwriting at the top
		{397, false}, // patterned inside cover
	} {
		t.Run(strconv.Itoa(tc.scan), func(t *testing.T) {
			m, err := renderMetrics(context.Background(), pdf, tc.scan)
			if err != nil {
				t.Fatal(err)
			}
			if m.blank() != tc.blank {
				t.Fatalf("scan %d blank=%v, metrics=%+v", tc.scan, m.blank(), m)
			}
		})
	}
}
