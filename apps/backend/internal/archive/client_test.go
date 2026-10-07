package archive

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchAndItem(t *testing.T) {
	client := Client{HTTP: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/advancedsearch.php":
			if got := r.URL.Query().Get("q"); got != `mediatype:texts AND (title:"Boericke" OR creator:"Boericke")` {
				t.Errorf("search query %q", got)
			}
			body = `{"response":{"numFound":1,"docs":[{"identifier":"pocketmanualhom00boergoog","title":"Pocket manual","creator":["William Boericke","Oscar E. Boericke"],"year":1906}]}}`
		case "/metadata/pocketmanualhom00boergoog":
			body = `{"metadata":{"title":"Pocket manual","creator":["William Boericke","Oscar E. Boericke"],"date":"1906","rights":"Public domain"},"files":[{"name":"pocketmanualhom00boergoog.pdf","size":"35359102","source":"original"},{"name":"oversized.pdf","size":"300000000"},{"name":"private.pdf","size":"2000","private":true},{"name":"readme.txt","size":"100"}]}`
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	result, err := client.Search(context.Background(), "Boericke", 1)
	if err != nil || len(result.Works) != 1 || result.Works[0].Creator != "William Boericke, Oscar E. Boericke" {
		t.Fatalf("search: %#v %v", result, err)
	}
	item, err := client.Item(context.Background(), result.Works[0].Identifier)
	if err != nil || len(item.PDFs) != 1 || item.PDFs[0].Name != "pocketmanualhom00boergoog.pdf" || !strings.HasPrefix(item.PDFs[0].URL, "https://archive.org/download/") {
		t.Fatalf("item: %#v %v", item, err)
	}
}

func TestRejectsInvalidInputs(t *testing.T) {
	client := Client{}
	if _, err := client.Search(context.Background(), `a`, 1); err == nil {
		t.Fatal("short search accepted")
	}
	if _, err := client.Item(context.Background(), `../private`); err == nil {
		t.Fatal("invalid item identifier accepted")
	}
}
