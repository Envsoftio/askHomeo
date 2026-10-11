package ingest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"homeopath-poc/backend/internal/pdfocr"
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

func TestComparePageTextToleratesMinorOCRDifferences(t *testing.T) {
	clean := "every other work on Materia Medica " + strings.Repeat("patient remedy symptoms motion pain fever cough treatment joints swelling ", 10)
	if got := comparePageText(strings.Replace(clean, "on", "no", 1), clean); got.suspect {
		t.Fatalf("minor OCR difference flagged: %+v", got)
	}
	if got := comparePageText(strings.Replace(clean, "work", "w(U'k", 1), clean); !got.suspect {
		t.Fatalf("malformed word passed: %+v", got)
	}
	missing := strings.ReplaceAll(clean, "patient remedy symptoms motion pain ", "")
	if got := comparePageText(missing, clean); !got.suspect {
		t.Fatalf("missing content passed: %+v", got)
	}
}

func TestComparePageTextAllowsTableReadingOrder(t *testing.T) {
	header := "Remedy Dose Finding Observation Result Notes Followup "
	rows := strings.Repeat("Arnica 30C swelling decreased after treatment mild improvement ", 9)
	stored := header + rows
	ocr := "Finding Observation Result Notes Followup Remedy Dose " + rows
	if got := comparePageText(stored, ocr); got.suspect {
		t.Fatalf("table column order flagged: %+v", got)
	}
}

func TestResolveOCRWarningForEmbeddedText(t *testing.T) {
	stored := strings.Repeat("remedy symptoms improved after treatment ", 30)
	match := textCheck{coverage: 0.98}
	if got := resolveOCRWarning(match, pdfocr.ConfidenceWarning, "PDF text", stored, stored); got.suspect {
		t.Fatalf("high agreement PDF text flagged: %+v", got)
	}
	noisyOCR := strings.ReplaceAll(stored, "improved", "impr0ved")
	if got := resolveOCRWarning(comparePageText(stored, noisyOCR), pdfocr.ConfidenceWarning, "PDF text", stored, noisyOCR); got.suspect {
		t.Fatalf("isolated OCR errors flagged despite continuous embedded text: %+v", got)
	}
	for _, tc := range []struct {
		result  textCheck
		method  string
		warning string
		stored  string
		fresh   string
	}{
		{match, "Tesseract OCR", pdfocr.ConfidenceWarning, stored, stored},
		{textCheck{coverage: 0.70}, "PDF text", pdfocr.ConfidenceWarning, stored, stored},
		{match, "PDF text", "OCR required a cropped scan; check all margins for missing text.", stored, stored},
		{match, "PDF text", pdfocr.ConfidenceWarning, "short page", "short page"},
		{match, "PDF text", pdfocr.ConfidenceWarning, stored, "uniquephrase1 uniquephrase2 uniquephrase3 uniquephrase4 uniquephrase5 " + stored},
	} {
		if got := resolveOCRWarning(tc.result, tc.warning, tc.method, tc.stored, tc.fresh); !got.suspect {
			t.Fatalf("uncertain page auto-passed: %+v", tc)
		}
	}
}
