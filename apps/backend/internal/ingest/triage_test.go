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

func TestScanTriageOutcome(t *testing.T) {
	for _, tc := range []struct {
		metrics      scanMetrics
		kind, status string
	}{
		{scanMetrics{}, "blank", "auto_checked"},
		{scanMetrics{dark: 0.0003, edges: 0.001}, "unclassified", "needs_review"},
		{scanMetrics{dark: 0.0001, edges: 0.007}, "unclassified", "needs_review"},
	} {
		kind, status, _ := tc.metrics.outcome()
		if kind != tc.kind || status != tc.status {
			t.Errorf("metrics %+v => %s/%s, want %s/%s", tc.metrics, kind, status, tc.kind, tc.status)
		}
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
