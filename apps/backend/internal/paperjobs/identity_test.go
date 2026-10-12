package paperjobs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"homeopath-poc/backend/internal/scholarly"
)

func TestAlternateCopyCandidateAndHTMLIdentity(t *testing.T) {
	for _, tc := range []struct {
		license, version string
		eligible bool
	}{{"cc-by", "publishedVersion", true}, {"cc0", "acceptedVersion", true}, {"cc-by-nc", "publishedVersion", false}, {"cc-by", "unknown", false}} {
		_, eligible := CandidateFromLocation(scholarly.Location{URL: "https://journal.example/article.pdf", Format: "pdf", License: tc.license, Version: tc.version})
		if eligible != tc.eligible {
			t.Fatalf("candidate %s %s eligible=%v", tc.license, tc.version, eligible)
		}
	}
	title, doi := "Effects of homeopathic treatment in trial", "10.1234/trial.01"
	article := `<html><head><meta name="citation_doi" content="` + doi + `"><meta name="citation_title" content="` + title + `"><link rel="license" href="https://creativecommons.org/licenses/by/4.0/"></head><body><article><h1>` + title + `</h1><p>` + strings.Repeat("The reviewed methods and results are saved here. ", 20) + `</p></article></body></html>`
	if err := validateHTMLIdentity([]byte(article), doi, title); err != nil {
		t.Fatal(err)
	}
	if err := validateHTMLIdentity([]byte(article), "10.1234/wrong", title); err == nil {
		t.Fatal("wrong DOI HTML accepted")
	}
	if err := validateHTMLIdentity([]byte(strings.Replace(article, "creativecommons.org/licenses/by/4.0/", "creativecommons.org/licenses/by-nc/4.0/", 1)), doi, title); err == nil {
		t.Fatal("noncommercial HTML accepted")
	}
	if err := validateHTMLIdentity([]byte(`<html><body><form>Sign in</form></body></html>`), doi, title); err == nil {
		t.Fatal("login page accepted")
	}
}

func TestAlternatePDFIdentity(t *testing.T) {
	raw := makeArticlePDF(t)
	if err := validatePDFIdentity(context.Background(), raw, "10.1234/trial.01", "Effects of homeopathic treatment in trial"); err != nil {
		t.Fatal(err)
	}
	if err := validatePDFIdentity(context.Background(), raw, "10.1234/wrong", "Effects of homeopathic treatment in trial"); err == nil {
		t.Fatal("wrong DOI PDF accepted")
	}
	if err := validatePDFIdentity(context.Background(), []byte("<html>login</html>"), "10.1234/trial.01", "Effects of homeopathic treatment in trial"); err == nil {
		t.Fatal("HTML login body accepted as PDF")
	}
}

func makeArticlePDF(t *testing.T) []byte {
	t.Helper()
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skip("Ghostscript unavailable")
	}
	dir := t.TempDir()
	ps := filepath.Join(dir, "article.ps")
	pdf := filepath.Join(dir, "article.pdf")
	content := `%!PS-Adobe-3.0
/Helvetica findfont 12 scalefont setfont
72 720 moveto (Effects of homeopathic treatment in trial) show
72 690 moveto (DOI: 10.1234/trial.01) show
72 660 moveto (https://creativecommons.org/licenses/by/4.0/) show
showpage
`
	if err := os.WriteFile(ps, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("gs", "-q", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pdfwrite", "-sOutputFile="+pdf, ps).CombinedOutput(); err != nil {
		t.Fatalf("fixture PDF generation: %v %s", err, output)
	}
	raw, err := os.ReadFile(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
