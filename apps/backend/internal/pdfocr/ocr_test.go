package pdfocr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuspiciousText(t *testing.T) {
	for _, text := range []string{"every other w(U'k on Materia Mcdica", "broken � text", "w]rk"} {
		if SuspiciousText(text) == "" {
			t.Errorf("missed broken token: %q", text)
		}
	}
	for _, text := range []string{"Materia Medica", "Nux vomica", "patient's symptoms (worse at night)", "work — historical spelling"} {
		if SuspiciousText(text) != "" {
			t.Errorf("flagged clean text: %q", text)
		}
	}
}
func TestWordConfidence(t *testing.T) {
	for _, tc := range []struct {
		tsv  string
		want bool
	}{
		{"5\t1\t1\t1\t1\t1\t0\t0\t10\t10\t96.2\tMedica", false},
		{"5\t1\t1\t1\t1\t1\t0\t0\t10\t10\t79.9\tMcdica", true},
		{"5\t1\t1\t1\t1\t1\t0\t0\t10\t10\tbroken\tword", true},
		{"level\tpage_num\tblock_num", true},
	} {
		if got := lowConfidence(tc.tsv); got != tc.want {
			t.Errorf("confidence = %v, want %v", got, tc.want)
		}
	}
}

func TestCheckRenderedPage(t *testing.T) {
	for _, tool := range []string{"gs", "tesseract"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " unavailable")
		}
	}
	dir := t.TempDir()
	ps, pdf := filepath.Join(dir, "page.ps"), filepath.Join(dir, "page.pdf")
	content := "%!PS-Adobe-3.0\n/Helvetica findfont 16 scalefont setfont\n72 720 moveto (Every other work on Materia Medica.) show\n"
	for i := 0; i < 8; i++ {
		content += fmt.Sprintf("72 %d moveto (The patient reports pain and fever during the night.) show\n", 690-i*24)
	}
	content += "showpage\n"
	if err := os.WriteFile(ps, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("gs", "-q", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pdfwrite", "-sOutputFile="+pdf, ps).CombinedOutput(); err != nil {
		t.Fatalf("PDF fixture: %v %s", err, out)
	}
	text, warning, err := Check(context.Background(), pdf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Materia Medica") {
		t.Fatalf("OCR text: %s", text)
	}
	if strings.Contains(warning, "unavailable") {
		t.Fatalf("TSV confidence missing: %s", warning)
	}
}
