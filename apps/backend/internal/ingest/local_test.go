package ingest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractLocalPDFPage(t *testing.T) {
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skip("Ghostscript unavailable")
	}
	dir := t.TempDir()
	ps := filepath.Join(dir, "source.ps")
	pdf := filepath.Join(dir, "source.pdf")
	content := "%!PS-Adobe-3.0\n/Helvetica findfont 14 scalefont setfont\n72 720 moveto (Aconite is described with sudden fear and restlessness in this historical source.) show\n72 690 moveto (The same page discusses fever and its accompanying symptoms in detail.) show\nshowpage\n"
	if err := os.WriteFile(ps, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("gs", "-q", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pdfwrite", "-sOutputFile="+pdf, ps)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make PDF: %v: %s", err, out)
	}
	text, method, err := extractLocalPage(context.Background(), pdf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if method != "PDF text" || !strings.Contains(text, "Aconite") || !strings.Contains(text, "fever") {
		t.Fatalf("unexpected extraction %q: %q", method, text)
	}
}
func TestGuessPrintedLabelOnlyAtPageEdges(t *testing.T) {
	if got := guessPrintedLabel("xiv\nTitle\nBody text"); got == nil || *got != "xiv" {
		t.Fatalf("roman front matter: %v", got)
	}
	if got := guessPrintedLabel("Heading\n12\nBody text"); got != nil {
		t.Fatalf("middle number should not be label: %v", *got)
	}
	if got := guessPrintedLabel("Heading\nBody text\n42"); got == nil || *got != "42" {
		t.Fatalf("footer number: %v", got)
	}
}
