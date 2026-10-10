package document

import (
	"strings"
	"testing"
)

func TestGenericNavigationFilteringRetainsEvidenceAndOriginals(t *testing.T) {
	raw := []byte(`<html><body><p><a href="/">Home</a></p><p>(advertising)</p><p><a href="a">Mind</a> * <a href="b">Head</a> * <a href="c">Eyes</a></p><h1>Collected observations</h1><p>This substantive introduction must survive.</p><p>Cold -- <a href="a">Abc.</a>, <a href="b">Def.</a>, <a href="c">Ghi.</a></p><p>Copyright © Publisher</p></body></html>`)
	out, err := Extract(raw, "text/html", "")
	if err != nil {
		t.Fatal(err)
	}
	excluded := 0
	for _, b := range out.Blocks {
		if b.ExclusionReason != "" {
			excluded++
			if b.EndByte <= b.StartByte || b.Text == "" {
				t.Fatal("lost exclusion provenance")
			}
		}
	}
	text := EvidenceText(out.Blocks)
	if excluded != 4 || strings.Contains(text, "advertising") || strings.Contains(text, "Copyright") || strings.Contains(text, "Home") {
		t.Fatalf("bad exclusions=%d text=%q", excluded, text)
	}
	if !strings.Contains(text, "substantive introduction") || !strings.Contains(text, "Abc.") {
		t.Fatalf("removed evidence: %s", text)
	}
}

func TestHeadingAncestryNamedAnchorsAndLineBreaks(t *testing.T) {
	raw := []byte(`<html><h1>Symptoms</h1><h2>Head</h2><h3>Cause</h3><a name="cold"></a><p>Cold -- Abc., Def.<br>Heat -- Ghi., Jkl.</p><h2>Mind</h2><p>No fear.</p></html>`)
	out, err := Extract(raw, "text/html", "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Blocks[3].Key != "cold" || out.Blocks[3].Heading != "Symptoms > Head > Cause" || !strings.Contains(out.Blocks[3].Text, "\nHeat") {
		t.Fatalf("structure lost: %+v", out.Blocks)
	}
	if out.Blocks[5].Heading != "Symptoms > Mind" {
		t.Fatalf("stale heading: %+v", out.Blocks[5])
	}
}

func TestIndexFilteringDoesNotRemoveSubstantiveNotes(t *testing.T) {
	raw := []byte(`<html><head><title>Collected cases</title></head><body><p><a href="a">Alpha</a></p><p><a href="b">Beta</a></p><p><a href="c">Gamma</a></p><p><a href="d">Delta</a></p><p><a href="e">Epsilon</a></p><p>These observations describe the author's selection of cases and limitations.</p></body></html>`)
	out, err := Extract(raw, "text/html", "")
	if err != nil {
		t.Fatal(err)
	}
	text := EvidenceText(out.Blocks)
	if out.Title != "Collected cases" || strings.Contains(text, "Alpha") || !strings.Contains(text, "limitations") {
		t.Fatalf("bad index handling: %+v", out)
	}
	standalone, err := Extract([]byte(`<html><p><a href="reference">Related source</a></p><p>Discussion of the reference.</p></html>`), "text/html", "")
	if err != nil || standalone.Blocks[0].ExclusionReason != "" {
		t.Fatalf("removed prose reference: %+v %v", standalone, err)
	}
}
