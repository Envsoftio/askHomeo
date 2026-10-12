package scholarly

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

func TestProviderContractsAndContinuation(t *testing.T) {
	calls := 0
	c := &Client{PubMedBase: "https://provider.test", EuropeBase: "https://provider.test", HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/esearch.fcgi" {
			if !strings.Contains(r.URL.Query().Get("term"), "homoeopathy[All Fields]") {
				t.Error("missing synonym")
			}
			return response(`{"esearchresult":{"count":"2","idlist":["123"]}}`), nil
		}
		if r.URL.Path == "/efetch.fcgi" {
			return response(`<PubmedArticleSet><PubmedArticle><MedlineCitation><PMID>123</PMID><Article><ArticleTitle>Trial</ArticleTitle><Journal><JournalIssue><PubDate><Year>2024</Year></PubDate></JournalIssue></Journal></Article></MedlineCitation><PubmedData><ArticleIdList><ArticleId IdType="doi">10.1/X</ArticleId><ArticleId IdType="pmc">PMC999</ArticleId></ArticleIdList></PubmedData></PubmedArticle></PubmedArticleSet>`), nil
		}
		if r.URL.Path == "/search" {
			if r.URL.Query().Get("cursorMark") != "*" {
				t.Error("bad cursor")
			}
			return response(`{"hitCount":2,"nextCursorMark":"NEXT","resultList":{"result":[{"id":"123","source":"MED","title":"Trial","doi":"10.1/X","pmid":"123","pmcid":"PMC999","pubYear":"2024"}]}}`), nil
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
		return nil, nil
	})}}
	p, e := c.SearchPubMed(context.Background(), "covid", 0, 1)
	if e != nil {
		t.Fatal(e)
	}
	if p.Total != 2 || p.Next != "1" || len(p.Results) != 1 || p.Results[0].DOI != "10.1/x" {
		t.Fatalf("wrong PubMed page %+v", p)
	}
	ep, e := c.SearchEurope(context.Background(), "covid", "", 1)
	if e != nil {
		t.Fatal(e)
	}
	if ep.Next != "NEXT" || ep.Results[0].PMCID != "PMC999" {
		t.Fatalf("wrong Europe page %+v", ep)
	}
	if calls != 3 {
		t.Fatalf("calls %d", calls)
	}
}

func TestOptionalResolversFailClosed(t *testing.T) {
	t.Setenv("UNPAYWALL_CONTACT_EMAIL", "")
	c := &Client{}
	locations, err := c.Unpaywall(context.Background(), "10.1/example")
	if err != nil || locations.Available || !strings.Contains(locations.Reason, "disabled") {
		t.Fatalf("Unpaywall missing-config state: %+v %v", locations, err)
	}
	t.Setenv("SEMANTIC_SCHOLAR_ENABLED", "1")
	t.Setenv("SEMANTIC_SCHOLAR_API_KEY", "")
	t.Setenv("SEMANTIC_SCHOLAR_COMMERCIAL_PERMISSION_CONFIRMED", "0")
	if _, err = c.SearchSemantic(context.Background(), "homeopathy", 0, 10); err == nil {
		t.Fatal("Semantic Scholar used without credentials and permission")
	}
}

func TestPMCAndUnpaywallContracts(t *testing.T) {
	t.Setenv("UNPAYWALL_CONTACT_EMAIL", "test@example.invalid")
	c := &Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "unpaywall") {
			if r.URL.Query().Get("email") != "test@example.invalid" {
				t.Error("missing contact email")
			}
			return response(`{"doi":"10.1/example","is_oa":true,"best_oa_location":{"url_for_pdf":"https://repository.example/paper.pdf","version":"acceptedVersion","license":"cc-by"},"oa_locations":[]}`), nil
		}
		if r.URL.Query().Get("identifier") != "oai:pubmedcentral.nih.gov:7654321" || r.URL.Query().Get("metadataPrefix") != "pmc" {
			t.Error("wrong PMC OAI contract")
		}
		return response(`<OAI-PMH><GetRecord><record><metadata><article><p>Full text.</p></article></metadata></record></GetRecord></OAI-PMH>`), nil
	})}}
	result, err := c.Unpaywall(context.Background(), "10.1/example")
	if err != nil || len(result.Candidates) != 1 || result.Candidates[0].Version != "acceptedVersion" {
		t.Fatalf("locations %+v %v", result, err)
	}
	raw, err := c.PMC(context.Background(), "PMC7654321")
	if err != nil || !strings.Contains(string(raw), "Full text") {
		t.Fatalf("PMC %s %v", raw, err)
	}
}

func TestProviderRetryAfterIsTyped(t *testing.T){
 c:=&Client{PubMedBase:"https://provider.test",HTTP:&http.Client{Transport:roundTrip(func(*http.Request)(*http.Response,error){return &http.Response{StatusCode:429,Header:http.Header{"Retry-After":[]string{"7"}},Body:io.NopCloser(strings.NewReader("busy"))},nil})}}
 _,err:=c.SearchPubMed(context.Background(),"trial",0,1);var providerErr *ProviderError;if !errors.As(err,&providerErr)||providerErr.Status!=429||providerErr.RetryAfter.Seconds()!=7 {t.Fatalf("retry contract %v",err)}
}
