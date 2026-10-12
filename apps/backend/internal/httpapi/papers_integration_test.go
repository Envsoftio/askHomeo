package httpapi

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/doi"
	"homeopath-poc/backend/internal/paperjobs"
	"homeopath-poc/backend/internal/scholarly"
)

type paperTransport func(*http.Request) (*http.Response, error)

func (f paperTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPaperReferenceIsolationAndAuthorizationIntegration(t *testing.T) {
	dbURL := os.Getenv("PAPER_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set PAPER_TEST_DATABASE_URL to a disposable database")
	}
	t.Setenv("PAPER_SEARCH_ENABLED", "1")
	t.Setenv("PAPER_ACQUISITION_ENABLED", "1")
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	client := &scholarly.Client{PubMedBase: "https://provider.test", EuropeBase: "https://provider.test", HTTP: &http.Client{Transport: paperTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/esearch.fcgi":
			body = `{"esearchresult":{"count":"1","idlist":["7654321"]}}`
		case "/efetch.fcgi":
			body = `<PubmedArticleSet><PubmedArticle><MedlineCitation><PMID>7654321</PMID><Article><ArticleTitle>Fixture India protocol</ArticleTitle></Article></MedlineCitation><PubmedData><ArticleIdList><ArticleId IdType="pmc">PMC7654321</ArticleId></ArticleIdList></PubmedData></PubmedArticle></PubmedArticleSet>`
		case "/PMC7654321/fullTextXML":
			body = `<article><front><article-meta><article-id pub-id-type="pmcid">PMC7654321</article-id><article-id pub-id-type="pmid">7654321</article-id><permissions><license><license-p><ext-link href="https://creativecommons.org/licenses/by/4.0/">CC BY</ext-link></license-p></license></permissions><title-group><article-title>Fixture India protocol</article-title></title-group></article-meta></front><body><sec><title>Methods</title><p>Protocol methods and planned outcome only.</p></sec></body></article>`
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}}
	api := (&API{Store: store, Token: "paper-admin-test-token-123456789", ReviewerToken: "paper-reviewer-test-token-123456789", Scholarly: client}).Handler()
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		api.ServeHTTP(res, req)
		return res
	}
	admin, reviewer := "paper-admin-test-token-123456789", "paper-reviewer-test-token-123456789"
	res := request("GET", "/api/v1/papers/search?q=India%20trial&provider=pubmed", "", admin)
	if res.Code != 200 {
		t.Fatalf("search %d %s", res.Code, res.Body.String())
	}
	var search struct {
		ID      string           `json:"id"`
		Results []scholarly.Work `json:"results"`
	}
	if json.Unmarshal(res.Body.Bytes(), &search) != nil || len(search.Results) != 1 {
		t.Fatalf("bad search %s", res.Body.String())
	}
	body := `{"search_id":"` + search.ID + `","provider":"pubmed","native_id":"7654321"}`
	res = request("POST", "/api/v1/papers/references", body, reviewer)
	if res.Code != 404 {
		t.Fatalf("cross-user search save %d", res.Code)
	}
	res = request("POST", "/api/v1/papers/references", body, admin)
	if res.Code != 201 {
		t.Fatalf("save %d %s", res.Code, res.Body.String())
	}
	var saved struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &saved)
	res = request("POST", "/api/v1/papers/references", body, admin)
	if res.Code != 201 {
		t.Fatalf("idempotent save %d %s", res.Code, res.Body.String())
	}
	res = request("GET", "/api/v1/papers/references", "", reviewer)
	if res.Code != 200 || strings.Contains(res.Body.String(), "Fixture India protocol") {
		t.Fatalf("reference leaked to reviewer %d %s", res.Code, res.Body.String())
	}
	locationBody := `{"study_country":"IN","evidence_block_id":"00000000-0000-0000-0000-000000000000","reason":"Fixture site claim without a saved methods passage"}`
	res = request("PUT", "/api/v1/papers/references/"+saved.ID+"/study-location", locationBody, reviewer)
	if res.Code != 403 {
		t.Fatalf("reviewer changed location %d", res.Code)
	}
	res = request("PUT", "/api/v1/papers/references/"+saved.ID+"/study-location", locationBody, admin)
	if res.Code != 409 {
		t.Fatalf("reference-only location was approved %d", res.Code)
	}
	res = request("POST", "/api/v1/papers/references/"+saved.ID+"/acquire", "", reviewer)
	if res.Code != 403 {
		t.Fatalf("reviewer acquired source: %d", res.Code)
	}
	res = request("GET", "/api/v1/papers/references/"+saved.ID+"/locations", "", admin)
	if res.Code != 200 || !strings.Contains(res.Body.String(), "No DOI") {
		t.Fatalf("missing DOI location state %d %s", res.Code, res.Body.String())
	}
	res = request("POST", "/api/v1/papers/references/"+saved.ID+"/acquire", "", admin)
	if res.Code != 202 {
		t.Fatalf("admin acquisition %d %s", res.Code, res.Body.String())
	}
	var job struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.Unmarshal(res.Body.Bytes(), &job) != nil || job.Status != "queued" {
		t.Fatalf("bad queued job %s", res.Body.String())
	}
	worked, err := (&paperjobs.Worker{Store: store, Client: client}).ProcessOne(ctx)
	if err != nil || !worked {
		t.Fatalf("paper worker %v %v", worked, err)
	}
	res = request("GET", "/api/v1/papers/acquisitions/"+job.ID, "", admin)
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"status":"review"`) {
		t.Fatalf("job status %d %s", res.Code, res.Body.String())
	}
	res = request("POST", "/api/v1/papers/references/"+saved.ID+"/acquire", "", admin)
	if res.Code != 200 || !strings.Contains(res.Body.String(), "already_imported") {
		t.Fatalf("repeat acquisition %d %s", res.Code, res.Body.String())
	}
	other := uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO scholarly_works(id,pmcid,title,authors) VALUES($1,'PMC88776655','Cancellation fixture','Fixture Author')`, other)
	if err != nil {
		t.Fatal(err)
	}
	res = request("POST", "/api/v1/papers/references/"+other.String()+"/acquire", "", admin)
	if res.Code != 202 {
		t.Fatalf("queue cancel fixture %d %s", res.Code, res.Body.String())
	}
	var queued struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &queued)
	res = request("POST", "/api/v1/papers/acquisitions/"+queued.ID+"/cancel", "", admin)
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("cancel %d %s", res.Code, res.Body.String())
	}
	res = request("POST", "/api/v1/papers/acquisitions/"+queued.ID+"/retry", "", admin)
	if res.Code != 202 || !strings.Contains(res.Body.String(), `"status":"queued"`) {
		t.Fatalf("retry %d %s", res.Code, res.Body.String())
	}
	res = request("POST", "/api/v1/papers/acquisitions/"+queued.ID+"/cancel", "", admin)
	if res.Code != 200 {
		t.Fatalf("repeat cancel %d %s", res.Code, res.Body.String())
	}
}

func TestPaperCorrectionNoticeIntegration(t *testing.T) {
	dbURL := os.Getenv("PAPER_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set PAPER_TEST_DATABASE_URL to a fresh disposable database")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	workID := uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO scholarly_works(id,doi,title,authors) VALUES($1,'10.1234/corrected-paper','Correction fixture','Fixture Author')`, workID)
	if err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"DOI":"10.1234/corrected-paper","title":["Correction fixture"],"updated-by":[{"DOI":"10.1234/correction-notice","type":"correction"}]}}`))
	}))
	defer provider.Close()
	t.Setenv("PAPER_ACQUISITION_ENABLED", "1")
	api := (&API{Store: store, Token: "paper-correction-admin-token-12345", ReviewerToken: "paper-correction-reviewer-token-12345", DOI: &doi.Client{BaseURL: provider.URL, HTTP: provider.Client()}}).Handler()
	request := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		api.ServeHTTP(res, req)
		return res
	}
	path := "/api/v1/papers/references/" + workID.String() + "/refresh-notice"
	if res := request(path, "paper-correction-reviewer-token-12345"); res.Code != 403 {
		t.Fatalf("reviewer refreshed notice: %d", res.Code)
	}
	if res := request(path, "paper-correction-admin-token-12345"); res.Code != 200 || !strings.Contains(res.Body.String(), `"notice_type":"correction"`) {
		t.Fatalf("correction refresh: %d %s", res.Code, res.Body.String())
	}
	var count int
	if err := store.DB.QueryRow(ctx, `SELECT count(*) FROM scholarly_notices WHERE work_id=$1 AND notice_type='correction'`, workID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("correction provenance count=%d err=%v", count, err)
	}
	if res := request("/api/v1/papers/references/"+workID.String()+"/acquire", "paper-correction-admin-token-12345"); res.Code != 409 {
		t.Fatalf("correction did not hold acquisition: %d %s", res.Code, res.Body.String())
	}
	_, err = store.DB.Exec(ctx, `UPDATE scholarly_works SET notice_type='retraction',notice_url='https://example.org/earlier-retraction' WHERE id=$1`, workID)
	if err != nil {
		t.Fatal(err)
	}
	if res := request(path, "paper-correction-admin-token-12345"); res.Code != 200 || !strings.Contains(res.Body.String(), `"notice_type":"retraction"`) || !strings.Contains(res.Body.String(), "earlier-retraction") {
		t.Fatalf("weaker update replaced prior retraction: %d %s", res.Code, res.Body.String())
	}
}

func TestDOIAlternateCandidateQueueIntegration(t *testing.T) {
	dbURL := os.Getenv("PAPER_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set PAPER_TEST_DATABASE_URL to a disposable database")
	}
	t.Setenv("PAPER_ACQUISITION_ENABLED", "1")
	t.Setenv("UNPAYWALL_CONTACT_EMAIL", "test@example.invalid")
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	workID := uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO scholarly_works(id,doi,title,authors) VALUES($1,'10.1234/alternate-queue','Alternate queue fixture','Fixture Author')`, workID)
	if err != nil {
		t.Fatal(err)
	}
	client := &scholarly.Client{HTTP: &http.Client{Transport: paperTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "api.unpaywall.org" {
			t.Errorf("unexpected resolver host %s", r.URL.Hostname())
		}
		body := `{"doi":"10.1234/alternate-queue","is_oa":true,"oa_locations":[{"url_for_pdf":"https://journal.example/restricted.pdf","version":"publishedVersion","license":"cc-by-nc"},{"url_for_pdf":"https://journal.example/article.pdf","version":"publishedVersion","license":"cc-by"}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}}
	api := (&API{Store: store, Token: "paper-alternate-admin-token-12345", Scholarly: client}).Handler()
	req := httptest.NewRequest("POST", "/api/v1/papers/references/"+workID.String()+"/acquire", nil)
	req.Header.Set("Authorization", "Bearer paper-alternate-admin-token-12345")
	res := httptest.NewRecorder()
	api.ServeHTTP(res, req)
	if res.Code != 202 {
		t.Fatalf("DOI candidate queue %d %s", res.Code, res.Body.String())
	}
	var candidates []paperjobs.Candidate
	var saved []byte
	if err := store.DB.QueryRow(ctx, `SELECT candidate_urls_json FROM paper_acquisition_jobs WHERE work_id=$1`, workID).Scan(&saved); err != nil || json.Unmarshal(saved, &candidates) != nil {
		t.Fatalf("candidate snapshot: %v", err)
	}
	if len(candidates) != 1 || candidates[0].URL != "https://journal.example/article.pdf" {
		t.Fatalf("restrictive candidate was queued: %#v", candidates)
	}
}
