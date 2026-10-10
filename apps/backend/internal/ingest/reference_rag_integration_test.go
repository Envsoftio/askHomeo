package ingest

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/httpapi"
	"homeopath-poc/backend/internal/localllm"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Uses reviewed TXT and a deterministic embedding fixture, with the real
// publication and embedding worker rather than a pre-inserted ready index.
func TestReferenceSourceRAGPreparationIntegration(t *testing.T) {
	db := os.Getenv("REFERENCE_RAG_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set REFERENCE_RAG_TEST_DATABASE_URL to a disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := t.TempDir()
	migrations, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(root, "apps/backend"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(migrations, filepath.Join(root, "apps/backend/migrations")); err != nil {
		t.Fatal(err)
	}
	store, err := core.Open(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Input any `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		n := 1
		if xs, ok := body.Input.([]any); ok {
			n = len(xs)
		}
		data := make([]any, n)
		for i := range data {
			data[i] = map[string]any{"index": i, "embedding": []float64{1, 0, 0}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "reference-rag-fixture", "data": data})
	}))
	defer fixture.Close()
	model := localllm.New(localllm.Config{EmbeddingProvider: "openai", EmbeddingBaseURL: fixture.URL, EmbeddingModel: "reference-rag-fixture", EmbeddingRevision: "v1", Dimensions: 3})
	api := &httpapi.API{Store: store, Token: "reference-rag-admin-12345", Model: model}
	handler := api.Handler()
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(raw)))
		req.Header.Set("Authorization", "Bearer "+api.Token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		out := map[string]any{}
		_ = json.Unmarshal(res.Body.Bytes(), &out)
		return out
	}
	text := "ACONITE MIND FEAR dark Acon. Bold = grade 2."
	sid, err := store.ImportDocument(ctx, strings.NewReader(text), core.DocumentImport{Title: "Reference RAG fixture", Author: "Fixture", Edition: "First", Repository: "Provider A", ContentType: "text/plain", Transport: "upload", RightsStatement: "Synthetic test content"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if err = New(store, model).once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var block uuid.UUID
	if err = store.DB.QueryRow(ctx, `SELECT id FROM document_blocks WHERE source_id=$1 ORDER BY block_index LIMIT 1`, sid).Scan(&block); err != nil {
		t.Fatal(err)
	}
	call("POST", "/document-blocks/"+block.String()+"/review", map[string]any{"decision": "accepted"}, 200)
	call("PUT", "/sources/"+sid.String()+"/categories", map[string]any{"categories": []string{"materia_medica", "repertory"}, "origin": "manual", "rationale": "Synthetic reference book", "evidence_category": "classical_reference"}, 200)
	units := call("GET", "/sources/"+sid.String()+"/review-units", nil, 200)
	rev := units["revision_id"].(string)
	u := units["units"].([]any)[0].(map[string]any)
	if u["text"] != text || u["ready"] != true {
		t.Fatalf("reviewed unit not ready: %+v", u)
	}
	loc := map[string]any{"document_block_id": block, "start_character": 0, "end_character": len([]rune(text)), "exact_text": text, "text_sha256": u["text_sha256"]}
	call("POST", "/sources/"+sid.String()+"/mm-entries", map[string]any{"revision_id": rev, "spelling": "ACONITE", "canonical_name": "Fixture aconite", "preparation_key": "whole plant", "rationale": "Verified synthetic identity", "locations": []any{loc}}, 201)
	parent := call("POST", "/sources/"+sid.String()+"/repertory-entries", map[string]any{"revision_id": rev, "heading": "MIND", "rationale": "Verified synthetic rubric", "locations": []any{loc}, "remedies": []any{}}, 201)["id"].(string)
	call("POST", "/sources/"+sid.String()+"/repertory-entries", map[string]any{"revision_id": rev, "parent_id": parent, "heading": "FEAR", "rationale": "Verified synthetic membership", "locations": []any{loc}, "remedies": []any{map[string]any{"canonical_name": "Fixture aconite", "preparation_key": "whole plant", "source_notation": "Acon.", "grade": 2, "grade_scheme": "Bold = grade 2."}}}, 201)
	if call("GET", "/repertory/rubrics?q=FEAR", nil, 200)["total"] != float64(0) {
		t.Fatal("unpublished rubric entered search")
	}
	call("POST", "/sources/"+sid.String()+"/rights", map[string]any{"decision": "allowed", "note": "Synthetic permitted fixture"}, 200)
	call("POST", "/sources/"+sid.String()+"/publish", nil, 200)
	if got := call("GET", "/sources/"+sid.String()+"/index-status", nil, 200)["status"]; got != "pending" {
		t.Fatalf("publish index status %v", got)
	}
	for i := 0; i < 12; i++ {
		if err = New(store, model).once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	index := call("GET", "/sources/"+sid.String()+"/index-status", nil, 200)
	if index["status"] != "ready" || index["completed"] != index["total"] || index["total"].(float64) < 1 {
		t.Fatalf("passages not RAG ready: %+v", index)
	}
	if call("GET", "/repertory/rubrics?q=FEAR", nil, 200)["total"] != float64(1) {
		t.Fatal("verified rubric unavailable after embedding")
	}
	if call("GET", "/materia-medica/entries?q=ACONITE", nil, 200)["total"] != float64(1) {
		t.Fatal("verified remedy unavailable after embedding")
	}
	var passages int
	err = store.DB.QueryRow(ctx, `SELECT count(*) FROM chunks c JOIN chunk_embeddings ce ON ce.chunk_id=c.id JOIN active_indexes ai ON ai.source_id=c.source_id JOIN index_runs ir ON ir.id=ai.index_run_id AND ir.embedding_config_id=ce.embedding_config_id JOIN evidence_categories cat ON cat.chunk_id=c.id WHERE c.source_id=$1 AND ir.status='ready' AND cat.categories @> ARRAY['materia_medica','repertory']::text[]`, sid).Scan(&passages)
	if err != nil || passages < 1 {
		t.Fatalf("category-tagged RAG passages=%d err=%v", passages, err)
	}
}
