package doi

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeClient(body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

func TestNormalizeDOI(t *testing.T) {
	for _, s := range []string{"10.1371/journal.pone.0134657", "https://doi.org/10.1371/journal.pone.0134657", "doi:10.1371/journal.pone.0134657"} {
		got, err := Normalize(s)
		if err != nil || got != "10.1371/journal.pone.0134657" {
			t.Fatalf("%q: %q %v", s, got, err)
		}
	}
	for _, s := range []string{"https://example.com/10.1371/x", "10.1371/x?foo=1", "10.1371/../secret", "not-a-doi"} {
		if _, err := Normalize(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

func TestLookupCrossrefKeepsReferenceOnlyWithoutEligiblePDF(t *testing.T) {
	record, err := (&Client{HTTP: fakeClient(`{"message":{"DOI":"10.1234/referenceonly","title":["A homeopathy trial"],"author":[{"given":"A","family":"Author"}],"publisher":"PLOS","type":"journal-article","published":{"date-parts":[[2015,3,13]]},"license":[{"URL":"http://creativecommons.org/licenses/by/4.0/"}],"link":[{"URL":"https://example.org/article","content-type":"text/html"}]}}`)}).Lookup(context.Background(), "10.1234/referenceonly")
	if err != nil {
		t.Fatal(err)
	}
	if record.Title != "A homeopathy trial" || record.Authors != "A Author" || record.Year != 2015 || record.PDFURL != "" || record.LicenseURL == "" {
		t.Fatalf("unexpected metadata: %#v", record)
	}
}

func TestLookupRequiresLicenseAndDirectPDF(t *testing.T) {
	record, err := (&Client{HTTP: fakeClient(`{"message":{"DOI":"10.1234/eligible","title":["Paper"],"license":[{"URL":"https://creativecommons.org/licenses/by/4.0/"}],"link":[{"URL":"https://journal.example/paper.pdf","content-type":"application/pdf"}]}}`)}).Lookup(context.Background(), "10.1234/eligible")
	if err != nil || record.PDFURL != "https://journal.example/paper.pdf" {
		t.Fatalf("PDF eligibility: %#v %v", record, err)
	}
	if _, err := FetchPDF(context.Background(), "https://127.0.0.1/private.pdf"); err == nil {
		t.Fatal("private PDF address accepted")
	}
}
func TestLookupFlagsRetractionBeforePDFImport(t *testing.T) {
	body := `{"message":{"DOI":"10.1234/retracted","title":["Retracted trial"],"license":[{"URL":"https://creativecommons.org/licenses/by/4.0/"}],"link":[{"URL":"https://journal.example/paper.pdf","content-type":"application/pdf"}],"updated-by":[{"DOI":"10.1234/retraction-notice","type":"retraction"}]}}`
	record, err := (&Client{HTTP: fakeClient(body)}).Lookup(context.Background(), "10.1234/retracted")
	if err != nil || record.RetractionNoticeURL != "https://doi.org/10.1234/retraction-notice" {
		t.Fatalf("retraction metadata: %#v %v", record, err)
	}
}
func TestLookupFallsBackToDataCiteMetadata(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/works/") {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"attributes":{"doi":"10.5555/example","titles":[{"title":"A data paper"}],"creators":[{"name":"A. Author"}],"publisher":"Repository","publicationYear":2024,"types":{"resourceTypeGeneral":"Text"}}}}`)), Header: make(http.Header)}, nil
	})}
	record, err := (&Client{HTTP: client}).Lookup(context.Background(), "10.5555/example")
	if err != nil || record.Provider != "DataCite" || record.Title != "A data paper" || record.PDFURL != "" {
		t.Fatalf("DataCite fallback: %#v %v", record, err)
	}
}
func TestLookupRejectsMismatchedPDFVersion(t *testing.T) {
	body := `{"message":{"DOI":"10.1234/version","title":["Paper"],"license":[{"URL":"https://creativecommons.org/licenses/by/4.0/","content-version":"am"}],"link":[{"URL":"https://journal.example/vor.pdf","content-type":"application/pdf","content-version":"vor"}]}}`
	record, err := (&Client{HTTP: fakeClient(body)}).Lookup(context.Background(), "10.1234/version")
	if err != nil || record.PDFURL != "" {
		t.Fatalf("version mismatch: %#v %v", record, err)
	}
}
func TestLiveHomeopathyDOI(t *testing.T) {
	if os.Getenv("LIVE_DOI_TEST") != "1" {
		t.Skip("set LIVE_DOI_TEST=1 for live Crossref lookup")
	}
	record, err := (&Client{}).Lookup(context.Background(), "10.1371/journal.pone.0134657")
	if err != nil {
		t.Fatal(err)
	}
	if record.Provider != "Crossref" || !strings.Contains(strings.ToLower(record.Title), "homeopath") || record.Year != 2015 || record.RetractionNoticeURL != "" || !strings.Contains(record.PDFURL, "journals.plos.org/plosone/article/file") {
		t.Fatalf("unexpected homeopathy DOI metadata: %#v", record)
	}
}
