package ingest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestComparePageTextFindsMissingOpening(t *testing.T) {
	opening := "bryonia opening symptoms motion aggravation characteristic remedy fever cough pain stitching joints swelling"
	body := strings.Repeat("patient remedy symptoms motion pain fever cough treatment joints swelling ", 8)
	if got := comparePageText(body, opening+" "+body); !got.suspect {
		t.Fatalf("missing opening passed: %+v", got)
	}
	if got := comparePageText(opening+" "+body, opening+" "+body); got.suspect {
		t.Fatalf("matching text was flagged: %+v", got)
	}
}

func TestFreshOCRNashPage(t *testing.T) {
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skip("Ghostscript unavailable")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("Tesseract unavailable")
	}
	pdf := filepath.Join("..", "..", "..", "..", "data", "starter-corpus", "originals", "nash-1899.pdf")
	if _, err := os.Stat(pdf); err != nil {
		t.Skip("Nash PDF unavailable")
	}
	text, err := freshOCR(context.Background(), pdf, 30)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "BRYONIA ALBA") || !strings.Contains(text, "flamed parts") {
		t.Fatalf("fresh OCR missed scan opening: %.120s", text)
	}
}
func TestFreshOCRFarringtonMarginFallback(t *testing.T) {
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skip("Ghostscript unavailable")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("Tesseract unavailable")
	}
	pdf := filepath.Join("..", "..", "..", "..", "data", "starter-corpus", "originals", "farrington-1890.pdf")
	if _, err := os.Stat(pdf); err != nil {
		t.Skip("Farrington PDF unavailable")
	}
	text, err := freshOCR(context.Background(), pdf, 28)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Ignatia") || !strings.Contains(text, "Nux vomica") {
		t.Fatalf("crop fallback missed Farrington's text: %.120s", text)
	}
}
