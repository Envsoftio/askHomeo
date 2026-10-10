package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/httpapi"
	"homeopath-poc/backend/internal/localllm"
)

// Uses only a disposable database and synthetic source text; no live model calls.
func TestRepertoryBrowserIntegration(t *testing.T) {
	dbURL := os.Getenv("REPERTORY_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set REPERTORY_TEST_DATABASE_URL to a disposable database")
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
		var body struct {
			Input any `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		n := 1
		if values, ok := body.Input.([]any); ok {
			n = len(values)
		}
		data := make([]any, n)
		for i := range data {
			data[i] = map[string]any{"index": i, "embedding": []float64{1, 0, 0}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "repertory-fixture", "data": data})
	}))
	defer modelServer.Close()
	model := localllm.New(localllm.Config{EmbeddingProvider: "openai", EmbeddingBaseURL: modelServer.URL, EmbeddingModel: "repertory-fixture", EmbeddingRevision: "v1", Dimensions: 3})
	a := &httpapi.API{Store: store, Token: "repertory-fixture-admin-token-12345", Model: model}
	handler := a.Handler()
	request := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		req.Header.Set("Authorization", "Bearer repertory-fixture-admin-token-12345")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(res.Body.Bytes(), &out)
		return out
	}
	text := "MIND FEAR dark Acon. Bell. Unknown. EYE fear after long pain in dark café"
	source, err := store.ImportDocument(ctx, strings.NewReader(text), core.DocumentImport{Title: "Synthetic repertory", Author: "Fixture", Edition: "Test edition", ContentType: "text/plain", Transport: "upload", RightsStatement: "Synthetic test content"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err = New(store, model).once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var block uuid.UUID
	var original string
	if err = store.DB.QueryRow(ctx, `SELECT id,original_text FROM document_blocks WHERE source_id=$1 ORDER BY block_index LIMIT 1`, source).Scan(&block, &original); err != nil {
		t.Fatal(err)
	}
	request("POST", "/api/v1/document-blocks/"+block.String()+"/review", map[string]any{"decision": "accepted"}, 200)
	location := map[string]any{"document_block_id": block, "start_character": 0, "end_character": len([]rune(original)), "exact_text": original}
	createEntry := func(path []string, parent any, ordinal int, accept bool) string {
		t.Helper()
		out := request("POST", "/api/v1/sources/"+source.String()+"/structured-entries", map[string]any{"kind": "repertory_rubric", "parent_id": parent, "ordinal": ordinal, "heading": path[len(path)-1], "full_path": path, "locations": []any{location}}, 201)
		id := out["id"].(string)
		if accept {
			request("POST", "/api/v1/structured-entries/"+id+"/review", map[string]any{"decision": "accepted", "rationale": "Synthetic reviewed path"}, 200)
		}
		return id
	}
	mind := createEntry([]string{"MIND"}, nil, 0, true)
	fear := createEntry([]string{"MIND", "FEAR"}, mind, 1, true)
	dark := createEntry([]string{"MIND", "FEAR", "dark"}, fear, 2, true)
	createEntry([]string{"EYE fear after long pain in dark"}, nil, 3, true)
	pending := createEntry([]string{"pending dark"}, nil, 4, false)
	remedy := request("POST", "/api/v1/remedies", map[string]any{"canonical_name": "Fixture aconite", "preparation_key": "whole plant"}, 200)["id"].(string)
	for i, notation := range []string{"Acon.", "Bell.", "Unknown."} {
		var grade any
		if i == 1 {
			grade = 2
		}
		out := request("POST", "/api/v1/structured-entries/"+dark+"/remedies", map[string]any{"remedy_id": remedy, "source_notation": notation, "source_remedy_spelling": notation, "grade": grade, "grade_scheme": "fixture convention", "locations": []any{location}}, 201)
		if i < 2 {
			request("POST", "/api/v1/rubric-remedies/"+out["id"].(string)+"/review", map[string]any{"decision": "accepted", "rationale": "Synthetic verified membership"}, 200)
		}
	}
	search := func(suffix string) map[string]any {
		return request("GET", "/api/v1/repertory/rubrics"+suffix, nil, 200)
	}
	if search("")["total"] != float64(0) {
		t.Fatal("unpublished rubric leaked")
	}
	request("PUT", "/api/v1/sources/"+source.String()+"/categories", map[string]any{"categories": []string{"repertory"}, "origin": "manual", "rationale": "Synthetic repertory", "evidence_category": "classical_reference"}, 200)
	request("POST", "/api/v1/sources/"+source.String()+"/rights", map[string]any{"decision": "allowed", "note": "Synthetic fixture"}, 200)
	request("POST", "/api/v1/sources/"+source.String()+"/publish", nil, 200)
	for i := 0; i < 10; i++ {
		if err = New(store, model).once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	result := search("?q=fear+dark")
	rows := result["items"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["id"] != dark {
		t.Fatalf("exact path ranking: %+v", result)
	}
	if rows[0].(map[string]any)["verified_remedy_count"] != float64(1) {
		t.Fatal("distinct verified remedy count is wrong")
	}
	ancestors := rows[0].(map[string]any)["ancestors"].([]any)
	if len(ancestors) != 2 || ancestors[0].(map[string]any)["id"] != mind || ancestors[1].(map[string]any)["id"] != fear {
		t.Fatal("tree ancestry lost")
	}
	for _, suffix := range []string{"?scope=selected", "?scope=selected&source_id=" + uuid.NewString(), "?chapter=ABSENT", "?remedy_id=" + uuid.NewString(), "?q=missing", "?offset=50"} {
		if search(suffix)["total"] != float64(0) && suffix != "?offset=50" {
			t.Fatalf("scope widened: %s", suffix)
		}
	}
	if search("?parent=" + fear)["total"] != float64(1) {
		t.Fatal("child navigation failed")
	}
	if search("?remedy_id=" + remedy)["total"] != float64(1) {
		t.Fatal("reverse lookup failed")
	}
	if search("?scope=selected&source_id=" + source.String() + "&chapter=MIND&remedy_id=" + remedy)["total"] != float64(1) {
		t.Fatal("filter intersection failed")
	}
	if len(search("?offset=50")["items"].([]any)) != 0 {
		t.Fatal("pagination failed")
	}
	request("GET", "/api/v1/repertory/rubrics/"+pending, nil, 404)
	detail := request("GET", "/api/v1/repertory/rubrics/"+dark, nil, 200)
	members := detail["remedies"].([]any)
	if len(members) != 2 {
		t.Fatal("pending membership leaked")
	}
	unknown, known := false, false
	for _, raw := range members {
		m := raw.(map[string]any)
		unknown = unknown || m["grade"] == nil
		known = known || m["grade"] == float64(2)
		loc := m["locations"].([]any)[0].(map[string]any)
		if loc["exact_text"] != original || !strings.Contains(loc["original_url"].(string), "/reader") {
			t.Fatal("support lost")
		}
	}
	if !unknown || !known {
		t.Fatal("unknown/source grade was lost")
	}
	originalURL := detail["locations"].([]any)[0].(map[string]any)["original_url"].(string)
	request("GET", originalURL, nil, 200)
	catalog := request("GET", "/api/v1/repertory/catalog", nil, 200)
	if len(catalog["sources"].([]any)) != 1 {
		t.Fatal("catalog missing eligible source")
	}
	model.Config.EmbeddingRevision = "incompatible"
	if search("")["total"] != float64(0) {
		t.Fatal("incompatible index leaked")
	}
	request("GET", "/api/v1/repertory/rubrics/"+dark, nil, 404)
	model.Config.EmbeddingRevision = "v1"
	if _, err = store.DB.Exec(ctx, `UPDATE sources SET status='disabled' WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	if search("")["total"] != float64(0) {
		t.Fatal("disabled source leaked")
	}
	request("GET", "/api/v1/repertory/rubrics/"+dark, nil, 404)
	if len(request("GET", "/api/v1/repertory/catalog", nil, 200)["sources"].([]any)) != 0 {
		t.Fatal("disabled source catalog leaked")
	}
	if _, err = store.DB.Exec(ctx, `UPDATE sources SET status='published',rights_status='denied' WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	if search("")["total"] != float64(0) {
		t.Fatal("denied source leaked")
	}
	request("GET", "/api/v1/repertory/rubrics/"+dark, nil, 404)
	if _, err = store.DB.Exec(ctx, `UPDATE sources SET rights_status='allowed' WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	if search("")["total"] != float64(4) {
		t.Fatal("restored source did not return")
	}
	collection, oldSnapshot, newSnapshot := uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO linked_collections(id,title,author) VALUES($1,'Fixture collection','Fixture')`, []any{collection}},
		{`INSERT INTO collection_snapshots(id,collection_id,generation,scope_json) VALUES($1,$3,1,'{}'),($2,$3,2,'{}')`, []any{oldSnapshot, newSnapshot, collection}},
		{`INSERT INTO collection_items(id,snapshot_id,requested_url,depth,discovery_ordinal,source_id) VALUES($1,$2,'https://fixture.invalid/book',0,0,$3)`, []any{uuid.New(), oldSnapshot, source}},
		{`INSERT INTO active_collection_snapshots(collection_id,snapshot_id) VALUES($1,$2)`, []any{collection, newSnapshot}},
	} {
		if _, err = store.DB.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if search("")["total"] != float64(0) {
		t.Fatal("retired collection snapshot leaked")
	}
	request("GET", "/api/v1/repertory/rubrics/"+dark, nil, 404)
	if len(request("GET", "/api/v1/repertory/catalog", nil, 200)["sources"].([]any)) != 0 {
		t.Fatal("retired collection catalog leaked")
	}
	request("GET", "/api/v1/repertory/rubrics?source_id=bad", nil, 400)
	req := httptest.NewRequest("GET", "/api/v1/repertory/catalog", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != 401 {
		t.Fatal("unauthenticated catalog allowed")
	}
}
