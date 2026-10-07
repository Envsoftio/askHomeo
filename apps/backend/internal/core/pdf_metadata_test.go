package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWellcomeCoverMetadata(t *testing.T) {
	raw := `Leaders in homoeopathic therapeutics / by E.B. Nash.
Contributors
Nash, E. B. 1838-1917.
Publication/Creation
Philadelphia : Boericke & Tafel, 1899.
Persistent URL
https://wellcomecollection.org/works/sh3awcy7`
	got := parsePDFMetadataText(raw)
	if got.Title != "Leaders in homoeopathic therapeutics" || got.Author != "E. B. Nash" || got.Publication != "Philadelphia : Boericke & Tafel, 1899" || got.Repository != "Wellcome Collection" || got.SourceURL != "https://wellcomecollection.org/works/sh3awcy7" {
		t.Fatalf("unexpected cover metadata: %#v", got)
	}
}

func TestParseScannedTitlePage(t *testing.T) {
	raw := `ORGANON OF MEDICINE
BY
SAMUEL HAHNEMANN
Fifth Edition`
	got := parsePDFMetadataText(raw)
	if got.Title != "ORGANON OF MEDICINE" || got.Author != "SAMUEL HAHNEMANN" || got.Edition != "Fifth Edition" {
		t.Fatalf("unexpected title page metadata: %#v", got)
	}
}

func TestParseVolumeFromTitlePage(t *testing.T) {
	got := parsePDFMetadataText("MATERIA MEDICA PURA\nVOLUME II\nBY SAMUEL HAHNEMANN")
	if got.Edition != "Volume II" {
		t.Fatalf("volume missing: %#v", got)
	}
}

func TestParseJournalCoverAvoidsBodyTextAsAuthor(t *testing.T) {
	raw := `Hamre et al. Systematic Reviews (2023) 12:191
https://doi.org/10.1186/s13643-023-02313-2
RESEARCH Open Access
Efficacy of homoeopathic treatment:
Systematic review of meta-analyses
of randomised placebo-controlled trials
H. J. Hamre, A. Glockmann, K. von Ammon, D. S. Riley and H. Kiene
Abstract
The risk of bias was assessed by the ROBIS tool.`
	got := parsePDFMetadataText(raw)
	if got.Title != "Efficacy of homoeopathic treatment: Systematic review of meta-analyses of randomised placebo-controlled trials" || got.Author != "H. J. Hamre, A. Glockmann, K. von Ammon, D. S. Riley and H. Kiene" || got.SourceURL != "https://doi.org/10.1186/s13643-023-02313-2" {
		t.Fatalf("journal cover: %#v", got)
	}
}

func TestStarterPDFCatalogueCover(t *testing.T) {
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skip("Ghostscript is not installed")
	}
	root := filepath.Join("..", "..", "..", "..")
	for _, item := range []struct{ file, title, author string }{
		{"nash-1899.pdf", "Leaders in homoeopathic therapeutics", "E. B. Nash"},
		{"farrington-1890.pdf", "A clinical materia medica", "E. A. Farrington"},
	} {
		pdf := filepath.Join(root, "data", "starter-corpus", "originals", item.file)
		if _, err := os.Stat(pdf); err != nil {
			t.Skip("starter PDFs are not available")
		}
		output := filepath.Join(t.TempDir(), "cover.txt")
		cmd := exec.Command("gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=txtwrite", "-dFirstPage=1", "-dLastPage=1", "-sOutputFile="+output, "-f", pdf)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("extract %s cover: %v: %s", item.file, err, out)
		}
		b, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		got := parsePDFMetadataText(string(b))
		if !strings.HasPrefix(got.Title, item.title) || got.Author != item.author {
			t.Fatalf("%s: %#v", item.file, got)
		}
	}
}
