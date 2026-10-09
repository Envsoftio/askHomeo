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

func TestHTMLTXTDocumentFlowIntegration(t *testing.T) {
	dbURL := os.Getenv("INTAKE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set INTAKE_TEST_DATABASE_URL to a disposable database")
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
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	modelServer:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if r.URL.Path!="/embeddings"{http.NotFound(w,r);return}
		var input struct{Input any `json:"input"`};if json.NewDecoder(r.Body).Decode(&input)!=nil{http.Error(w,"bad input",400);return}
		count:=1;if values,ok:=input.Input.([]any);ok{count=len(values)}
		data:=make([]map[string]any,count);for i:=range data{data[i]=map[string]any{"index":i,"embedding":[]float64{1,0,0}}}
		_ = json.NewEncoder(w).Encode(map[string]any{"model":"fixture-embedding","data":data})
	}));defer modelServer.Close()
	model:=localllm.New(localllm.Config{Provider:"openai",EmbeddingProvider:"openai",ChatProvider:"openai",EmbeddingBaseURL:modelServer.URL,ChatBaseURL:modelServer.URL,EmbeddingModel:"fixture-embedding",EmbeddingRevision:"v1",AnswerModel:"fixture-answer",Dimensions:3})
	imported:=map[string]uuid.UUID{}
	for _, fixture := range []struct{ format, content, contentType string }{
		{"html", `<html><body><nav><p>Menu</p></nav><h1 id="arnica">Materia Medica</h1><p>Arnica remedy picture includes modalities and aggravation from bruised soreness.</p><h2 id="appendix">Repertory appendix</h2><p>Rubrics list grade of remedies for the selected symptom.</p><script>alert(1)</script></body></html>`, "text/html; charset=utf-8"},
		{"txt", "Arnica\n\nThis historical materia medica describes bruised soreness and its modalities.", "text/plain; charset=utf-8"},
	} {
		id, e := store.ImportDocument(ctx, strings.NewReader(fixture.content), core.DocumentImport{Title: "Test " + fixture.format, Author: "Test author", Transport: "upload", ContentType: fixture.contentType, RightsStatement: "Fixture permitted for testing"})
		if e != nil {
			t.Fatal(e)
		}
		imported[fixture.format]=id
		for attempt := 0; attempt < 5; attempt++ {
			if err = New(store,model).once(ctx); err != nil {
				t.Fatal(err)
			}
			var current string
			if err = store.DB.QueryRow(ctx, `SELECT status FROM sources WHERE id=$1`, id).Scan(&current); err != nil {
				t.Fatal(err)
			}
			if current == "review" {
				break
			}
		}
		var status, kind string
		var blocks int
		var pageCount *int
		err = store.DB.QueryRow(ctx, `SELECT s.status,sa.kind,(SELECT count(*) FROM document_blocks WHERE source_id=s.id),s.page_count FROM sources s JOIN source_assets sa ON sa.id=s.primary_asset_id WHERE s.id=$1`, id).Scan(&status, &kind, &blocks, &pageCount)
		if err != nil || status != "review" || kind != fixture.format || blocks == 0 || pageCount != nil {
			t.Fatalf("status=%s kind=%s blocks=%d pages=%v err=%v", status, kind, blocks, pageCount, err)
		}
		api := (&httpapi.API{Store: store, Token: "integration-test-admin-token-12345", Model: model}).Handler()
		request := func(method, path, body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			api.ServeHTTP(res, req)
			return res
		}
		res := request("GET", "/api/v1/sources/"+id.String()+"/blocks", "")
		if res.Code != 200 {
			t.Fatalf("blocks %d %s", res.Code, res.Body.String())
		}
		var list []struct {
			ID       uuid.UUID `json:"id"`
			Original string    `json:"original_text"`
		}
		if err = json.Unmarshal(res.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		for _, b := range list {
			if strings.Contains(b.Original, "Menu") || strings.Contains(b.Original, "alert") {
				t.Fatalf("active/navigation text leaked: %q", b.Original)
			}
			res = request("POST", "/api/v1/document-blocks/"+b.ID.String()+"/review", `{"decision":"accepted"}`)
			if res.Code != 200 {
				t.Fatalf("review %d %s", res.Code, res.Body.String())
			}
		}
		chosen:=`["materia_medica"]`
		if fixture.format=="html"{chosen=`["materia_medica","repertory"]`}
		res=request("PUT","/api/v1/sources/"+id.String()+"/categories",`{"categories":`+chosen+`,"origin":"accepted_suggestion","rationale":"Reviewed extracted headings and passages","evidence_category":"classical_reference"}`)
		if res.Code!=200{t.Fatalf("categories %d %s",res.Code,res.Body.String())}
		if fixture.format=="html"{
			res=request("PUT","/api/v1/document-blocks/"+list[len(list)-1].ID.String()+"/categories",`{"categories":["repertory"],"rationale":"Reviewed repertory appendix"}`)
			if res.Code!=200{t.Fatalf("section override %d %s",res.Code,res.Body.String())}
		}
		res = request("POST", "/api/v1/sources/"+id.String()+"/rights", `{"decision":"allowed","note":"Fixture is permitted for testing"}`)
		if res.Code != 200 {
			t.Fatalf("rights %d %s", res.Code, res.Body.String())
		}
		res = request("POST", "/api/v1/sources/"+id.String()+"/publish", "")
		if res.Code != 200 {
			t.Fatalf("publish %d %s", res.Code, res.Body.String())
		}
		var passages int
		err = store.DB.QueryRow(ctx, `SELECT count(*) FROM chunks WHERE source_id=$1 AND document_block_id IS NOT NULL`, id).Scan(&passages)
		if err != nil || passages == 0 {
			t.Fatalf("passages=%d err=%v", passages, err)
		}
		for attempt:=0;attempt<10;attempt++{if err=New(store,model).once(ctx);err!=nil{t.Fatal(err)};var indexStatus string;_ = store.DB.QueryRow(ctx,`SELECT ir.status FROM active_indexes ai JOIN index_runs ir ON ir.id=ai.index_run_id WHERE ai.source_id=$1`,id).Scan(&indexStatus);if indexStatus=="ready"{break}}
		var readyCount int;err=store.DB.QueryRow(ctx,`SELECT count(*) FROM active_indexes ai JOIN index_runs ir ON ir.id=ai.index_run_id WHERE ai.source_id=$1 AND ir.status='ready'`,id).Scan(&readyCount)
		if err!=nil||readyCount!=1{t.Fatalf("READY index=%d err=%v",readyCount,err)}
		if fixture.format=="html"{
			var category string
			err=store.DB.QueryRow(ctx,`SELECT categories[1] FROM evidence_categories ec JOIN chunks c ON c.id=ec.chunk_id WHERE c.document_block_id=$1`,list[len(list)-1].ID).Scan(&category)
			if err!=nil||category!="repertory"{t.Fatalf("section category=%q err=%v",category,err)}
		}
	}
	var repertorySources int
	err=store.DB.QueryRow(ctx,`SELECT count(DISTINCT ec.source_id) FROM evidence_categories ec JOIN chunk_embeddings ce ON ce.chunk_id=ec.chunk_id JOIN active_indexes ai ON ai.source_id=ec.source_id JOIN index_runs ir ON ir.id=ai.index_run_id WHERE ec.source_id=ANY($1::uuid[]) AND ir.status='ready' AND ec.categories && ARRAY['repertory']::text[]`,[]uuid.UUID{imported["html"],imported["txt"]}).Scan(&repertorySources)
	if err!=nil||repertorySources!=1{t.Fatalf("repertory source intersection=%d err=%v",repertorySources,err)}
}
