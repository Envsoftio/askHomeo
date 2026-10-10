package httpapi

import (
	"strings"
	"testing"
)

func TestSavedOriginalReaderPreservesTypographyWithoutActiveContent(t *testing.T) {
	input := `<html><head><style>body{background:url(https://tracker.example/x)}</style></head><body><h2 id="rubric">Mind &amp; body</h2><dir><li><font color="red"><i>Bell.</i></font> ordinary <b>bold</b></li></dir><a href="https://tracker.example/click" name="next">Continue</a><img src="https://tracker.example/pixel"><script>fetch('https://tracker.example/x')</script><iframe src="https://tracker.example/frame"></iframe></body></html>`
	got, err := safeReaderHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<h2 id="rubric">Mind &amp; body</h2>`, `<ul><li><font style="color:red"><i>Bell.</i></font> ordinary <b>bold</b></li></ul>`, `id="next"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing preserved markup %q in %q", want, got)
		}
	}
	for _, forbidden := range []string{"tracker.example", "<script", "<iframe", "<img", "<style", "href="} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("active markup %q escaped sanitization: %q", forbidden, got)
		}
	}
}

func TestReaderPassageTargetUnicodeAndCorrections(t *testing.T) {
	got, err := readerPassageTarget("original", "é猫 <script> & next", 1, 2)
	if err != nil || !strings.Contains(got, "é<mark>猫</mark> &lt;script&gt; &amp; next") || !strings.Contains(got, "differs from the extracted original") || strings.Contains(got, "<script>") {
		t.Fatalf("incorrect or unsafe exact target: %q %v", got, err)
	}
	for _, span := range [][2]int{{-1, 2}, {1, 1}, {2, 1}, {0, 100}} {
		if _, err := readerPassageTarget("é猫", "é猫", span[0], span[1]); err == nil {
			t.Fatalf("accepted invalid span %v", span)
		}
	}
	got, err = readerPassageTarget("é猫", "é猫", 0, 2)
	if err != nil || strings.Contains(got, "differs from") || !strings.Contains(got, "<mark>é猫</mark>") {
		t.Fatalf("unchanged target %q: %v", got, err)
	}
}
