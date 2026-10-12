package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	doiclient "homeopath-poc/backend/internal/doi"
	"homeopath-poc/backend/internal/httpapi"
	"homeopath-poc/backend/internal/localllm"
	"homeopath-poc/backend/internal/paperjobs"
	"homeopath-poc/backend/internal/scholarly"
)

type paperFixtureTransport func(*http.Request) (*http.Response, error)

func (f paperFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestJATSReviewPublicationCitationIntegration(t *testing.T) {
	dbURL, path := os.Getenv("PAPER_TEST_DATABASE_URL"), os.Getenv("LIVE_JATS_FIXTURE")
	if dbURL == "" || path == "" {
		t.Skip("set PAPER_TEST_DATABASE_URL and LIVE_JATS_FIXTURE for disposable OA fixture")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		IDs []struct {
			Type  string `xml:"pub-id-type,attr"`
			Value string `xml:",chardata"`
		} `xml:"front>article-meta>article-id"`
	}
	if err = xml.Unmarshal(raw, &identity); err != nil {
		t.Fatal(err)
	}
	var pmcid, pmid, doi string
	for _, v := range identity.IDs {
		switch v.Type {
		case "pmcid":
			pmcid = v.Value
		case "pmid":
			pmid = v.Value
		case "doi":
			doi = v.Value
		}
	}
	if pmcid == "" || pmid == "" || doi == "" {
		t.Fatal("fixture lacks durable identifiers")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Input any `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		count := 1
		if list, ok := request.Input.([]any); ok {
			count = len(list)
		}
		vectors := make([]map[string]any, count)
		for i := range vectors {
			vectors[i] = map[string]any{"index": i, "embedding": []float64{1, 0, 0}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "fixture-embedding", "data": vectors})
	}))
	defer modelServer.Close()
	model := localllm.New(localllm.Config{Provider: "openai", EmbeddingProvider: "openai", ChatProvider: "openai", EmbeddingBaseURL: modelServer.URL, ChatBaseURL: modelServer.URL, EmbeddingModel: "fixture-embedding", EmbeddingRevision: "paper-fixture-v1", AnswerModel: "fixture-answer", Dimensions: 3})
	client := &scholarly.Client{PubMedBase: "https://paper-fixture.invalid", EuropeBase: "https://paper-fixture.invalid", HTTP: &http.Client{Transport: paperFixtureTransport(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch r.URL.Path {
		case "/esearch.fcgi":
			body = []byte(fmt.Sprintf(`{"esearchresult":{"count":"1","idlist":["%s"]}}`, pmid))
		case "/efetch.fcgi":
			body = []byte(fmt.Sprintf(`<PubmedArticleSet><PubmedArticle><MedlineCitation><PMID>%s</PMID><Article><ArticleTitle>Indian pilot article fixture</ArticleTitle><AuthorList><Author><LastName>Fixture</LastName><ForeName>Researcher</ForeName></Author></AuthorList></Article></MedlineCitation><PubmedData><ArticleIdList><ArticleId IdType="doi">%s</ArticleId><ArticleId IdType="pmc">%s</ArticleId></ArticleIdList></PubmedData></PubmedArticle></PubmedArticleSet>`, pmid, doi, pmcid))
		case "/search":
			body = []byte(fmt.Sprintf(`{"hitCount":1,"resultList":{"result":[{"id":"%s","source":"MED","title":"Indian pilot article fixture","doi":"%s","pmid":"%s","pmcid":"%s"}]}}`, pmid, doi, pmid, pmcid))
		case "/" + pmcid + "/fullTextXML":
			body = raw
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})}}
	t.Setenv("PAPER_SEARCH_ENABLED", "1")
	t.Setenv("PAPER_ACQUISITION_ENABLED", "1")
	handler := (&httpapi.API{Store: store, Token: "paper-fixture-admin-token-12345", Model: model, Scholarly: client}).Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer paper-fixture-admin-token-12345")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	search := request("GET", "/api/v1/papers/search?q=India%20trial&provider=all", "")
	if search.Code != 200 {
		t.Fatalf("search %d %s", search.Code, search.Body.String())
	}
	var found struct {
		ID      string           `json:"id"`
		Results []scholarly.Work `json:"results"`
	}
	if json.Unmarshal(search.Body.Bytes(), &found) != nil || len(found.Results) != 1 {
		t.Fatalf("deduplicated search %s", search.Body.String())
	}
	save := request("POST", "/api/v1/papers/references", fmt.Sprintf(`{"search_id":"%s","provider":"pubmed","native_id":"%s"}`, found.ID, pmid))
	if save.Code != 201 {
		t.Fatalf("save reference %d %s", save.Code, save.Body.String())
	}
	var saved struct {
		ID uuid.UUID `json:"id"`
	}
	if json.Unmarshal(save.Body.Bytes(), &saved) != nil {
		t.Fatal("bad saved reference")
	}
	listed := request("GET", "/api/v1/papers/references", "")
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), `"paper_search_id":"`+found.ID+`"`) {
		t.Fatalf("saved reference lost owning search: %d %s", listed.Code, listed.Body.String())
	}
	acquire := request("POST", "/api/v1/papers/references/"+saved.ID.String()+"/acquire", "")
	if acquire.Code != 202 {
		t.Fatalf("acquire %d %s", acquire.Code, acquire.Body.String())
	}
	worked, err := (&paperjobs.Worker{Store: store, Client: client}).ProcessOne(ctx)
	if err != nil || !worked {
		t.Fatalf("paper acquisition worker %v %v", worked, err)
	}
	id := saved.ID
	returned, err := store.ImportDocument(ctx, bytes.NewReader(raw), core.DocumentImport{Title: "Indian pilot JATS fixture", Author: "Saved article authors", Transport: "upload", ContentType: "application/xml", SourceID: &id})
	if err != nil || returned != id {
		t.Fatalf("repeat import created duplicate: %s %v", returned, err)
	}
	var status string
	var revision, asset uuid.UUID
	for attempt := 0; attempt < 50; attempt++ {
		if err = New(store, model).once(ctx); err != nil {
			t.Fatal(err)
		}
		err = store.DB.QueryRow(ctx, `SELECT status,current_revision_id,primary_asset_id FROM sources WHERE id=$1`, id).Scan(&status, &revision, &asset)
		if err != nil || status == "review" {
			break
		}
	}
	if err != nil || status != "review" {
		t.Fatalf("extraction %s %v", status, err)
	}
	// All decisions here are test automation, not human text or rights review.
	rows, err := store.DB.Query(ctx, `SELECT id FROM document_blocks WHERE source_id=$1 ORDER BY block_index`, id)
	if err != nil {
		t.Fatal(err)
	}
	blockIDs := []uuid.UUID{}
	for rows.Next() {
		var bid uuid.UUID
		if rows.Scan(&bid) != nil {
			t.Fatal("scan block")
		}
		blockIDs = append(blockIDs, bid)
	}
	rows.Close()
	if len(blockIDs) < 20 {
		t.Fatalf("too few blocks: %d", len(blockIDs))
	}
	var prepublicationChunks int
	_ = store.DB.QueryRow(ctx, `SELECT count(*) FROM chunks WHERE source_id=$1`, id).Scan(&prepublicationChunks)
	if prepublicationChunks != 0 {
		t.Fatal("unreviewed JATS entered evidence")
	}
	for _, bid := range blockIDs {
		res := request("POST", "/api/v1/document-blocks/"+bid.String()+"/review", `{"decision":"accepted"}`)
		if res.Code != 200 {
			t.Fatalf("review %d %s", res.Code, res.Body.String())
		}
	}
	if pmcid == "PMC10650991" {
		res := request("PUT", "/api/v1/papers/references/"+id.String()+"/study-location", fmt.Sprintf(`{"study_country":"IN","evidence_block_id":"%s","reason":"Fixture reviewer selected the saved Mumbai trial methods passage"}`, blockIDs[25]))
		if res.Code != 200 {
			t.Fatalf("study location %d %s", res.Code, res.Body.String())
		}
	}
	res := request("PUT", "/api/v1/sources/"+id.String()+"/categories", `{"categories":["research"],"origin":"manual","rationale":"Fixture category for path verification","evidence_category":"trial_study"}`)
	if res.Code != 200 {
		t.Fatalf("category %d %s", res.Code, res.Body.String())
	}
	res = request("POST", "/api/v1/sources/"+id.String()+"/rights", `{"decision":"allowed","note":"Fixture-only test decision after article license check"}`)
	if res.Code != 200 {
		t.Fatalf("rights %d %s", res.Code, res.Body.String())
	}
	res = request("POST", "/api/v1/sources/"+id.String()+"/publish", "")
	if res.Code != 200 {
		t.Fatalf("publish %d %s", res.Code, res.Body.String())
	}
	for i := 0; i < 20; i++ {
		if err = New(store, model).once(ctx); err != nil {
			t.Fatal(err)
		}
		var ready bool
		_ = store.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM active_indexes ai JOIN index_runs ir ON ir.id=ai.index_run_id WHERE ai.source_id=$1 AND ir.status='ready')`, id).Scan(&ready)
		if ready {
			break
		}
	}
	var chunk uuid.UUID
	needle := os.Getenv("PAPER_EXPECTED_PASSAGE")
	if needle == "" {
		needle = "standard care"
	}
	err = store.DB.QueryRow(ctx, `SELECT c.id FROM chunks c JOIN document_blocks b ON b.id=c.document_block_id WHERE c.source_id=$1 AND b.review_status='accepted' AND lower(c.text_exact) LIKE '%'||lower($2)||'%' ORDER BY c.created_at LIMIT 1`, id, needle).Scan(&chunk)
	if err != nil {
		t.Fatal("no saved expected passage: ", err)
	}
	var ready bool
	_ = store.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM active_indexes ai JOIN index_runs ir ON ir.id=ai.index_run_id WHERE ai.source_id=$1 AND ir.status='ready')`, id).Scan(&ready)
	if !ready {
		t.Fatal("compatible index not ready")
	}
	res = request("POST", "/api/v1/research/answer-jobs", fmt.Sprintf(`{"question":"What does the selected paper report?","mode":"quick","source_ids":["%s"],"paper_search_id":"%s"}`, id, found.ID))
	if res.Code != 202 {
		t.Fatalf("paper-scoped answer job %d %s", res.Code, res.Body.String())
	}
	var queued struct {
		JobID uuid.UUID `json:"job_id"`
	}
	if json.Unmarshal(res.Body.Bytes(), &queued) != nil || queued.JobID == uuid.Nil {
		t.Fatal("paper-scoped answer job missing ID")
	}
	var storedSearch uuid.UUID
	if err = store.DB.QueryRow(ctx, `SELECT paper_search_id FROM answer_jobs WHERE id=$1`, queued.JobID).Scan(&storedSearch); err != nil || storedSearch.String() != found.ID {
		t.Fatalf("answer job lost saved search provenance: %s %v", storedSearch, err)
	}
	res = request("POST", "/api/v1/research/answer-jobs", fmt.Sprintf(`{"question":"What does the selected paper report?","mode":"quick","source_ids":[],"paper_search_id":"%s"}`, found.ID))
	if res.Code != 409 {
		t.Fatalf("empty paper selection widened research scope: %d", res.Code)
	}
	res = request("POST", "/api/v1/research/answer-jobs", fmt.Sprintf(`{"question":"What does the selected paper report?","mode":"quick","source_ids":["%s"],"paper_search_id":"%s"}`, id, uuid.New()))
	if res.Code != 409 {
		t.Fatalf("unowned paper search accepted: %d", res.Code)
	}
	answerID, citationID := uuid.New(), uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO answers(id,question,status,answer_text,owner_principal_id,literature_category_scope,paper_search_id) VALUES($1,'fixture standard-care question','answered','Fixture answer','00000000-0000-4000-8000-000000000001',ARRAY['research']::text[],$2)`, answerID, found.ID)
	if err != nil {
		t.Fatal(err)
	}
	res = request("GET", "/api/v1/research/answers/"+answerID.String(), "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"paper_search"`) || !strings.Contains(res.Body.String(), found.ID) {
		t.Fatalf("saved answer lost search snapshot linkage: %d %s", res.Code, res.Body.String())
	}
	_, err = store.DB.Exec(ctx, `INSERT INTO answer_citations(id,answer_id,chunk_id,ordinal,evidence_label) VALUES($1,$2,$3,1,'E1')`, citationID, answerID, chunk)
	if err != nil {
		t.Fatal(err)
	}
	res = request("GET", "/api/v1/citations/"+citationID.String(), "")
	if res.Code != 200 {
		t.Fatalf("citation %d %s", res.Code, res.Body.String())
	}
	var cited struct {
		ReaderURL string `json:"reader_url"`
		Format    string `json:"format"`
		Passage   string `json:"passage"`
		AssetSHA  string `json:"asset_sha256"`
	}
	if json.Unmarshal(res.Body.Bytes(), &cited) != nil || cited.Format != "jats" || cited.Passage == "" || cited.AssetSHA == "" {
		t.Fatalf("bad citation %s", res.Body.String())
	}
	res = request("GET", strings.Split(cited.ReaderURL, "#")[0], "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), "<mark>") {
		t.Fatalf("saved reader %d %s", res.Code, res.Body.String()[:min(300, res.Body.Len())])
	}
	res = request("POST", "/api/v1/sources/"+id.String()+"/disable", `{"reason":"Temporary fixture eligibility test"}`)
	if res.Code != 200 {
		t.Fatalf("disable %d %s", res.Code, res.Body.String())
	}
	res = request("GET", "/api/v1/citations/"+citationID.String(), "")
	if res.Code == 200 {
		t.Fatal("disabled JATS citation remained accessible")
	}
	res = request("POST", "/api/v1/sources/"+id.String()+"/enable", `{"reason":"Restore after fixture eligibility test"}`)
	if res.Code != 200 {
		t.Fatalf("enable %d %s", res.Code, res.Body.String())
	}
	res = request("GET", "/api/v1/citations/"+citationID.String(), "")
	if res.Code != 200 {
		t.Fatalf("historical citation did not reopen: %d %s", res.Code, res.Body.String())
	}
	noticeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"message":{"DOI":%q,"title":["Indian pilot article fixture"],"updated-by":[{"DOI":"10.1234/fixture-retraction","type":"retraction"}]}}`, doi)
	}))
	defer noticeServer.Close()
	handler = (&httpapi.API{Store: store, Token: "paper-fixture-admin-token-12345", Model: model, Scholarly: client, DOI: &doiclient.Client{BaseURL: noticeServer.URL, HTTP: noticeServer.Client()}}).Handler()
	res = request("POST", "/api/v1/papers/references/"+id.String()+"/refresh-notice", "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), "fixture-retraction") {
		t.Fatalf("notice refresh %d %s", res.Code, res.Body.String())
	}
	var afterNotice string
	if err = store.DB.QueryRow(ctx, `SELECT status FROM sources WHERE id=$1`, id).Scan(&afterNotice); err != nil || afterNotice != "disabled" {
		t.Fatalf("notice did not disable source: %s %v", afterNotice, err)
	}
	res = request("GET", "/api/v1/citations/"+citationID.String(), "")
	if res.Code == 200 {
		t.Fatal("notice-disabled citation remained accessible")
	}
	res = request("GET", "/api/v1/research/answers/"+answerID.String(), "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"affected_sources"`) || !strings.Contains(res.Body.String(), "fixture-retraction") {
		t.Fatalf("historical answer did not show source notice: %d %s", res.Code, res.Body.String())
	}
	res = request("POST", "/api/v1/sources/"+id.String()+"/enable", `{"reason":"Fixture attempt to restore a noticed source"}`)
	if res.Code != 409 {
		t.Fatalf("noticed source restored: %d %s", res.Code, res.Body.String())
	}
	t.Logf("source=%s asset=%s revision=%s citation=%s blocks=%d sha=%s", id, asset, revision, citationID, len(blockIDs), cited.AssetSHA)
}
