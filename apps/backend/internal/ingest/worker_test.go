package ingest

import "testing"

func TestParseALTO(t *testing.T) {
	// Exercise namespaces, word spacing, line breaks, and escaped characters
	// without depending on an OCR download outside the repository.
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<alto xmlns="http://www.loc.gov/standards/alto/ns-v4#">
  <Layout><Page><PrintSpace><TextBlock>
    <String CONTENT="outside a text line"/>
    <TextLine>
      <String CONTENT="CARBO"/><SP/><String CONTENT="VEGETABILIS"/>
    </TextLine>
    <TextLine>
      <String CONTENT="A"/><String CONTENT="&amp;"/><String CONTENT="B"/>
    </TextLine>
  </TextBlock></PrintSpace></Page></Layout>
</alto>`)
	text, err := ParseALTO(data)
	if err != nil {
		t.Fatal(err)
	}
	if want := "CARBO VEGETABILIS\nA & B"; text != want {
		t.Fatalf("OCR text = %q, want %q", text, want)
	}
}

func TestParseALTORejectsMalformedXML(t *testing.T) {
	if _, err := ParseALTO([]byte(`<alto><TextLine><String CONTENT="unfinished"/>`)); err == nil {
		t.Fatal("expected malformed XML to fail")
	}
}
