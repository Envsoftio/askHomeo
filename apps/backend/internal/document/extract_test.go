package document

import (
	"strings"
	"testing"
)

func TestHTMLExtractionKeepsEvidenceOrderAndRemovesChrome(t *testing.T) {
	raw := []byte(`<!doctype html><html><head><script>bad()</script></head><body><nav><p>Menu</p></nav><h1 id="chapter">Materia Medica</h1><p>First &amp; second.</p><ul><li>Third item</li></ul><table><tr><th>Remedy</th><td>Arnica</td></tr></table><form><p>Submit secret</p></form></body></html>`)
	out, err := Extract(raw, "text/html; charset=utf-8", "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Format != "html" || len(out.Blocks) != 5 {
		t.Fatalf("unexpected extraction: %+v", out)
	}
	if out.Blocks[0].Key != "chapter" || out.Blocks[0].Text != "Materia Medica" || out.Blocks[1].Text != "First & second." || out.Blocks[4].Text != "Arnica" {
		t.Fatalf("unexpected block order: %+v", out.Blocks)
	}
	if !strings.Contains(strings.Join(out.Warnings, " "), "row and column relationships") {
		t.Fatalf("table structure limitation was not exposed: %+v", out.Warnings)
	}
	for _, block := range out.Blocks {
		if strings.Contains(block.Text, "Menu") || strings.Contains(block.Text, "secret") || strings.Contains(block.Text, "bad") {
			t.Fatalf("navigation or active content leaked: %+v", block)
		}
	}
}

func TestTextOffsetsAndBOM(t *testing.T) {
	raw := append([]byte{0xef, 0xbb, 0xbf}, []byte("First line\nsecond line\n\nThird paragraph")...)
	out, err := Extract(raw, "text/plain", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Blocks) != 2 || out.Blocks[0].StartByte != 3 || out.Blocks[1].Text != "Third paragraph" {
		t.Fatalf("unexpected paragraphs: %+v", out.Blocks)
	}
	for _, block := range out.Blocks {
		if string(raw[block.StartByte:block.EndByte]) != block.Text {
			t.Fatalf("offset does not identify passage: %+v", block)
		}
	}
}

func TestUncertainEncodingRequiresOverride(t *testing.T) {
	raw := []byte{'C', 'a', 'f', 0xe9}
	if _, err := Extract(raw, "text/plain", ""); err == nil || !strings.Contains(err.Error(), "override") {
		t.Fatalf("expected encoding guidance, got %v", err)
	}
	out, err := Extract(raw, "text/plain", "windows-1252")
	if err != nil || out.Blocks[0].Text != "Café" || out.Blocks[0].StartByte != 0 || out.Blocks[0].EndByte != len(raw) {
		t.Fatalf("override result: %+v, %v", out, err)
	}
}

func TestUTF16OriginalOffsets(t *testing.T) {
	raw := []byte{0xff, 0xfe, 'A', 0, ' ', 0, 'B', 0, '\n', 0, '\n', 0, 'C', 0}
	out, err := Extract(raw, "text/plain", "")
	if err != nil || len(out.Blocks) != 2 || out.Blocks[0].StartByte != 2 || out.Blocks[0].EndByte != 8 || out.Blocks[1].StartByte != 12 {
		t.Fatalf("UTF-16 offsets: %+v %v", out, err)
	}
}

func TestDuplicateHTMLAnchorsGetUniqueSectionKeys(t *testing.T) {
	out, err := Extract([]byte(`<html><body><p id="repeat">First.</p><p id="repeat">Second.</p></body></html>`), "text/html", "")
	if err != nil || len(out.Blocks) != 2 || out.Blocks[0].Key != "repeat" || out.Blocks[1].Key != "repeat-2" {
		t.Fatalf("duplicate anchors: %+v %v", out, err)
	}
}

func TestRejectBinaryAndScriptShell(t *testing.T) {
	for _, raw := range [][]byte{{'a', 0, 'b'}, []byte("\xff\xfe\x00"), []byte("%PDF-1.7")} {
		if _, err := Extract(raw, "", ""); err == nil {
			t.Fatalf("accepted invalid document %q", raw)
		}
	}
	if _, err := Extract([]byte(`<html><body><script>render()</script></body></html>`), "text/html", ""); err == nil {
		t.Fatal("accepted empty JavaScript shell")
	}
}

func TestLegacyHTMLAutomaticEncodingPreservesOriginalOffsets(t *testing.T) {
	raw := []byte("<html><body><p>M\xe9di-T: \x9cdema</p></body></html>")
	out, err := Extract(raw, "text/html", "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Charset != "windows-1252" || out.Blocks[0].Text != "Médi-T: œdema" {
		t.Fatalf("unexpected decoding: %+v", out)
	}
	b := out.Blocks[0]
	if string(raw[b.StartByte:b.EndByte]) != "<p>M\xe9di-T: \x9cdema</p>" {
		t.Fatalf("wrong original offsets: %+v", b)
	}
	if !strings.Contains(strings.Join(out.Warnings, " "), "Windows-1252") {
		t.Fatal("missing encoding review warning")
	}
	for _, ct := range []string{"text/html; charset=utf-8", "text/plain"} {
		if _, err := Extract(raw, ct, "utf-8"); err == nil {
			t.Fatal("overrode explicit encoding")
		}
	}
	if _, err := Extract([]byte("<html><meta charset=utf-8><p>\xe9</p></html>"), "text/html", ""); err == nil {
		t.Fatal("overrode meta declaration")
	}
	if _, err := Extract([]byte("<html><p>\x81</p></html>"), "text/html", ""); err == nil {
		t.Fatal("accepted undefined legacy byte")
	}
}

func TestImplicitParagraphEndHasOriginalLocation(t *testing.T) {
	raw := []byte(`<html><body><p>First paragraph<p>Second paragraph</p></body></html>`)
	out, err := Extract(raw, "text/html", "")
	if err != nil || len(out.Blocks) != 2 {
		t.Fatalf("extraction: %+v %v", out, err)
	}
	for i, b := range out.Blocks {
		if b.EndByte <= b.StartByte || b.EndByte > len(raw) {
			t.Fatalf("invalid offsets: %+v", b)
		}
		if i == 0 && string(raw[b.StartByte:b.EndByte]) != "<p>First paragraph" {
			t.Fatalf("wrong implicit boundary: %+v", b)
		}
	}
}
