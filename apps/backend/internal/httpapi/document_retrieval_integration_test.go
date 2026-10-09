package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/localllm"
	"homeopath-poc/backend/internal/safefetch"
)

// Run after TestHTMLTXTDocumentFlowIntegration against the same disposable DB.
func TestDocumentRetrievalAndSavedCitationIntegration(t *testing.T) {
	dbURL := os.Getenv("INTAKE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set INTAKE_TEST_DATABASE_URL for disposable fixture")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	var htmlID, txtID, configID uuid.UUID
	err = store.DB.QueryRow(ctx, `SELECT id FROM sources WHERE title='Test html' AND status='published' ORDER BY created_at DESC LIMIT 1`).Scan(&htmlID)
	if err != nil {
		t.Skip("run the HTML/TXT ingestion fixture first")
	}
	err = store.DB.QueryRow(ctx, `SELECT id FROM sources WHERE title='Test txt' AND status='published' ORDER BY created_at DESC LIMIT 1`).Scan(&txtID)
	if err != nil {
		t.Skip("run the HTML/TXT ingestion fixture first")
	}
	err = store.DB.QueryRow(ctx, `SELECT ir.embedding_config_id FROM active_indexes ai JOIN index_runs ir ON ir.id=ai.index_run_id WHERE ai.source_id=$1 AND ir.status='ready'`, htmlID).Scan(&configID)
	if err != nil {
		t.Fatal(err)
	}
	a := &API{Store: store, Token: "integration-test-admin-token-12345"}
	contextWithCategories := context.WithValue(ctx, categoryFilterKey{}, []string{"repertory"})
	for _, question := range []string{"rubrics grade remedies", "repertory appendix"} {
		hits, e := a.retrieve(contextWithCategories, configID, localllm.Vector([]float32{1, 0, 0}), question, "", []uuid.UUID{htmlID, txtID})
		if e != nil || len(hits) == 0 {
			t.Fatalf("category retrieval %q: %d hits, %v", question, len(hits), e)
		}
		for _, hit := range hits {
			if hit.SourceID != htmlID {
				t.Fatalf("category filter broadened into TXT: %+v", hit)
			}
		}
	}
	empty, e := a.retrieve(contextWithCategories, configID, localllm.Vector([]float32{1, 0, 0}), "rubrics", "", []uuid.UUID{txtID})
	if e != nil || len(empty) != 0 {
		t.Fatalf("empty source/category intersection broadened: %d hits, %v", len(empty), e)
	}
	empty, e = a.retrieve(context.WithValue(ctx, categoryFilterKey{}, []string{"research"}), configID, localllm.Vector([]float32{1, 0, 0}), "rubrics", "", []uuid.UUID{htmlID, txtID})
	if e != nil || len(empty) != 0 {
		t.Fatalf("unavailable category broadened: %d hits, %v", len(empty), e)
	}
	var chunkID uuid.UUID
	err = store.DB.QueryRow(ctx, `SELECT c.id FROM chunks c JOIN document_blocks b ON b.id=c.document_block_id WHERE c.source_id=$1 AND b.section_key='appendix' LIMIT 1`, htmlID).Scan(&chunkID)
	if err != nil {
		t.Fatal(err)
	}
	answerID, citationID := uuid.New(), uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO answers(id,question,status,answer_text,owner_principal_id,literature_category_scope,literature_category_snapshot) VALUES($1,'fixture question','answered','Fixture answer','00000000-0000-4000-8000-000000000001',$2,$3::jsonb)`, answerID, []string{"repertory"}, `{"`+chunkID.String()+`":{"categories":["repertory"],"evidence_category":"classical_reference"}}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(ctx, `INSERT INTO answer_citations(id,answer_id,chunk_id,ordinal,evidence_label) VALUES($1,$2,$3,1,'E1')`, citationID, answerID, chunkID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(ctx, `UPDATE sources SET literature_categories=ARRAY['other']::text[] WHERE id=$1`, htmlID)
	if err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	res := request("/api/v1/citations/" + citationID.String())
	if res.Code != 200 {
		t.Fatalf("saved citation %d %s", res.Code, res.Body.String())
	}
	var citation struct {
		Passage      string   `json:"passage"`
		SectionKey   string   `json:"section_key"`
		DocumentURL  string   `json:"document_url"`
		Format       string   `json:"format"`
		Categories   []string `json:"literature_categories"`
		PDFPageIndex *int     `json:"pdf_page_index"`
	}
	if err = json.Unmarshal(res.Body.Bytes(), &citation); err != nil {
		t.Fatal(err)
	}
	if citation.Format != "html" || citation.SectionKey != "appendix" || citation.PDFPageIndex != nil || len(citation.Categories) != 1 || citation.Categories[0] != "repertory" {
		t.Fatalf("saved citation changed after category edit: %+v", citation)
	}
	res = request(citation.DocumentURL)
	if res.Code != 200 || res.Header().Get("Content-Type") != "text/plain; charset=utf-8" || res.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("raw HTML was not inert: %d %v", res.Code, res.Header())
	}
	for _, path := range []string{citation.DocumentURL, "/api/v1/sources/" + htmlID.String() + "/blocks"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		unauthorized := httptest.NewRecorder()
		handler.ServeHTTP(unauthorized, req)
		if unauthorized.Code < 400 {
			t.Fatalf("unauthorized document read %s returned %d", path, unauthorized.Code)
		}
	}
}

func TestDocumentURLImportRefetchIntegration(t *testing.T) {
	dbURL := os.Getenv("INTAKE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set INTAKE_TEST_DATABASE_URL to a disposable database")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		word := "Preview"
		switch calls.Add(1) {
		case 2:
			word = "Imported"
		case 3:
			word = "Reimported"
		}
		_, _ = w.Write([]byte(`<html><body><h1>Materia Medica</h1><p>` + word + ` snapshot with modalities and remedy picture.</p></body></html>`))
	}))
	defer server.Close()
	fetcher := &safefetch.Fetcher{Resolve: func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	handler := (&API{Store: store, Token: "integration-test-admin-token-12345", Fetcher: fetcher}).Handler()
	request := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	var before, after int
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM sources`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	res := request("/api/v1/sources/preview-url", `{"url":"http://public.example/page"}`)
	if res.Code != 200 {
		t.Fatalf("preview %d %s", res.Code, res.Body.String())
	}
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM sources`).Scan(&after); err != nil || after != before {
		t.Fatalf("preview created source: %d -> %d, %v", before, after, err)
	}
	res = request("/api/v1/sources/import-document-url", `{"url":"http://public.example/page","title":"URL fixture","author":"Fixture author","rights_statement":"Review required","literature_categories":["materia_medica"]}`)
	if res.Code != 202 {
		t.Fatalf("URL import %d %s", res.Code, res.Body.String())
	}
	var imported struct {
		SourceID uuid.UUID `json:"source_id"`
	}
	if err = json.Unmarshal(res.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	var sha, requested, final, transport string
	err = store.DB.QueryRow(ctx, `SELECT s.document_sha256,a.requested_url,a.final_url,a.transport FROM sources s JOIN document_acquisitions a ON a.source_id=s.id WHERE s.id=$1`, imported.SourceID).Scan(&sha, &requested, &final, &transport)
	if err != nil || requested != "http://public.example/page" || final != requested || transport != "http" || calls.Load() != 2 {
		t.Fatalf("acquisition: %s %s %s calls=%d err=%v", requested, final, transport, calls.Load(), err)
	}
	importedBytes := []byte(`<html><body><h1>Materia Medica</h1><p>Imported snapshot with modalities and remedy picture.</p></body></html>`)
	hash := sha256.Sum256(importedBytes)
	if sha != hex.EncodeToString(hash[:]) {
		t.Fatalf("import used preview bytes: %s", sha)
	}
	var intakeDecisions int
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM literature_category_decisions WHERE source_id=$1 AND origin='manual'`, imported.SourceID).Scan(&intakeDecisions); err != nil || intakeDecisions != 1 {
		t.Fatalf("intake category decision count=%d err=%v", intakeDecisions, err)
	}
	_, err = store.DB.Exec(ctx, `UPDATE sources SET status='published' WHERE id=$1`, imported.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	res = request("/api/v1/sources/"+imported.SourceID.String()+"/reimport-document", `{}`)
	if res.Code != 202 {
		t.Fatalf("URL reimport %d %s", res.Code, res.Body.String())
	}
	var candidate struct {
		SourceID uuid.UUID `json:"source_id"`
	}
	if err = json.Unmarshal(res.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	var candidateSHA string
	var parentID uuid.UUID
	var candidateCategories []string
	err = store.DB.QueryRow(ctx, `SELECT document_sha256,supersedes_source_id,literature_categories FROM sources WHERE id=$1`, candidate.SourceID).Scan(&candidateSHA, &parentID, &candidateCategories)
	if err != nil || parentID != imported.SourceID || candidateSHA == sha || len(candidateCategories) != 1 || candidateCategories[0] != "materia_medica" || calls.Load() != 3 {
		t.Fatalf("reimport candidate sha=%s parent=%s categories=%v calls=%d err=%v", candidateSHA, parentID, candidateCategories, calls.Load(), err)
	}
	var originalSHA string
	if err = store.DB.QueryRow(ctx, `SELECT document_sha256 FROM sources WHERE id=$1`, imported.SourceID).Scan(&originalSHA); err != nil || originalSHA != sha {
		t.Fatalf("reimport changed original snapshot: %s -> %s, %v", sha, originalSHA, err)
	}
}
