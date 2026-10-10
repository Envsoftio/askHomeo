package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/localllm"
)

// Reviewed page/block fixtures isolate the structure review and reference
// libraries from OCR/network/model variability. Intake adapters have their own tests.
func TestReferenceLibrariesIntegration(t *testing.T) {
	db := os.Getenv("REFERENCE_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set REFERENCE_TEST_DATABASE_URL to a disposable database")
	}
	ctx := context.Background()
	root, _ := filepath.Abs("../../../..")
	store, err := core.Open(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store.Root = t.TempDir()
	model := localllm.New(localllm.Config{EmbeddingModel: "reference-fixture", EmbeddingRevision: "v1", Dimensions: 3})
	a := &API{Store: store, Token: "reference-admin-test-12345", ReviewerToken: "reference-reviewer-test-12345", Model: model}
	if err = a.SyncPrincipals(ctx); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err = store.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(raw)))
		req.Header.Set("Authorization", "Bearer "+a.Token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: %d expected %d: %s", method, path, res.Code, want, res.Body.String())
		}
		out := map[string]any{}
		_ = json.Unmarshal(res.Body.Bytes(), &out)
		return out
	}
	config := uuid.New()
	exec(`INSERT INTO embedding_configs(id,model_id,model_revision,dimensions,preprocessing_version) VALUES($1,'reference-fixture','v1',3,'fixture')`, config)
	var sources, mmIDs, remedyIDs []string
	for index, format := range []string{"pdf", "html", "txt"} {
		sid, uid := uuid.New(), uuid.New()
		provider := "Provider A"
		if index == 1 {
			provider = "Provider B"
		}
		text := "ACONITE — café. MIND FEAR dark Acon. Bold = grade 2."
		if format == "pdf" {
			exec(`INSERT INTO sources(id,source_key,title,author,edition,repository,pdf_path,pdf_sha256,pdf_bytes,page_count,status,literature_categories) VALUES($1,$2,$3,'Author A','Edition 1',$4,'fixture.pdf',repeat('a',64),1,1,'review',ARRAY['materia_medica','repertory'])`, sid, uuid.NewString(), "Reference "+format, provider)
		} else {
			content, contentType := text, "text/plain"
			if format == "html" {
				content = "<p>" + text + "</p>"
				contentType = "text/html"
			}
			sid, err = store.ImportDocument(ctx, strings.NewReader(content), core.DocumentImport{Title: "Reference " + format, Author: "Author B", Edition: "Edition 2", Repository: provider, ContentType: contentType, Transport: "upload"})
			if err != nil {
				t.Fatal(err)
			}
			exec(`UPDATE sources SET status='review',literature_categories=ARRAY['materia_medica','repertory'] WHERE id=$1`, sid)
		}
		var rev uuid.UUID
		if err = store.DB.QueryRow(ctx, `SELECT current_revision_id FROM sources WHERE id=$1`, sid).Scan(&rev); err != nil {
			t.Fatal(err)
		}
		loc := structuredLocationInput{Start: 0, End: len([]rune(text)), ExactText: text, TextSHA256: mmTextHash(text)}
		if format == "pdf" {
			loc.PageID = &uid
			exec(`INSERT INTO pages(id,source_id,pdf_page_index,scan_page_index,canvas_id,alto_url,image_url,text_raw,text_sha256,extraction_method,text_qa_status,page_kind) VALUES($1,$2,0,0,$3,'','',$4,repeat('a',64),'fixture','accepted','text')`, uid, sid, uuid.NewString(), text)
		} else {
			loc.DocumentBlockID = &uid
			exec(`INSERT INTO document_blocks(id,source_id,source_asset_id,processing_revision_id,block_index,section_key,kind,original_text,reviewed_text,start_byte,end_byte,review_status) SELECT $2,id,primary_asset_id,current_revision_id,0,'fixture','paragraph',$3,$3,0,$4,'accepted' FROM sources WHERE id=$1`, sid, uid, text, len(text))
		}
		body := func(parent any, heading string, members any) map[string]any {
			return map[string]any{"revision_id": rev, "parent_id": parent, "heading": heading, "rationale": "Checked synthetic source", "locations": []structuredLocationInput{loc}, "remedies": members}
		}
		rootID := call("POST", "/sources/"+sid.String()+"/repertory-entries", body(nil, "MIND", []any{}), 201)["id"].(string)
		prep := "whole plant"
		if format == "txt" {
			prep = "distinct preparation"
		}
		membership := map[string]any{"canonical_name": "Fixture aconite", "preparation_key": prep, "source_notation": "Acon.", "grade": 2, "grade_scheme": "Bold = grade 2."}
		wrong := body(rootID, "dark", []any{map[string]any{"canonical_name": "Fixture aconite", "preparation_key": prep, "source_notation": "Acon.", "grade": 2, "grade_scheme": "Not supported"}})
		call("POST", "/sources/"+sid.String()+"/repertory-entries", wrong, 400)
		stale := body(rootID, "dark", []any{})
		stale["revision_id"] = uuid.New()
		call("POST", "/sources/"+sid.String()+"/repertory-entries", stale, 409)
		hashStale := loc
		hashStale.TextSHA256 = "stale"
		stale = body(rootID, "dark", []any{})
		stale["locations"] = []structuredLocationInput{hashStale}
		call("POST", "/sources/"+sid.String()+"/repertory-entries", stale, 409)
		child := call("POST", "/sources/"+sid.String()+"/repertory-entries", body(rootID, "dark", []any{membership}), 201)["id"].(string)
		call("POST", "/sources/"+sid.String()+"/repertory-entries", body(rootID, "dark", []any{membership}), 409)
		// Text correction invalidates both the root and its dependent hierarchy.
		if format == "pdf" {
			exec(`UPDATE pages SET text_raw=text_raw||' correction' WHERE id=$1`, uid)
		} else {
			exec(`UPDATE document_blocks SET reviewed_text=reviewed_text||' correction' WHERE id=$1`, uid)
		}
		var count int
		if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM structured_entries WHERE source_id=$1 AND review_status='accepted'`, sid).Scan(&count); err != nil || count != 0 {
			t.Fatalf("correction retained rubric approval %d %v", count, err)
		}
		if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM rubric_remedies WHERE rubric_id=$1 AND review_status='accepted'`, child).Scan(&count); err != nil || count != 0 {
			t.Fatal("correction retained membership")
		}
		call("POST", "/sources/"+sid.String()+"/repertory-entries", body(rootID, "dark", []any{}), 409)
		text += " correction"
		loc.ExactText = text
		loc.End = len([]rune(text))
		loc.TextSHA256 = mmTextHash(text)
		rootID = call("POST", "/sources/"+sid.String()+"/repertory-entries", body(nil, "MIND", []any{}), 201)["id"].(string)
		child = call("POST", "/sources/"+sid.String()+"/repertory-entries", body(rootID, "dark", []any{membership}), 201)["id"].(string)
		// Explicit draft revocation also cascades without touching other sources.
		call("POST", "/structured-entries/"+rootID+"/review", map[string]any{"decision": "rejected", "rationale": "Replace synthetic hierarchy"}, 200)
		if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM structured_entries WHERE id=$1 AND review_status='accepted'`, child).Scan(&count); err != nil || count != 0 {
			t.Fatal("parent revocation retained child")
		}
		rootID = call("POST", "/sources/"+sid.String()+"/repertory-entries", body(nil, "MIND", []any{}), 201)["id"].(string)
		membership["grade"] = nil
		membership["grade_scheme"] = ""
		child = call("POST", "/sources/"+sid.String()+"/repertory-entries", body(rootID, "dark", []any{membership}), 201)["id"].(string)
		mm := call("POST", "/sources/"+sid.String()+"/mm-entries", map[string]any{"revision_id": rev, "spelling": "ACONITE", "canonical_name": "Fixture aconite", "preparation_key": prep, "rationale": "Reviewed source remedy identity", "locations": []structuredLocationInput{loc}}, 201)
		mmIDs = append(mmIDs, mm["id"].(string))
		remedyIDs = append(remedyIDs, mm["remedy_id"].(string))
		sources = append(sources, sid.String())
		call("GET", "/materia-medica/entries/"+mm["id"].(string), nil, 404)
		if format != "pdf" {
			exec(`INSERT INTO chunks(id,source_id,document_block_id,chunk_index,text_exact,start_character,end_character) VALUES($1,$2,$3,0,$4,0,$5)`, uuid.New(), sid, uid, text, loc.End)
		}
		pub, run := uuid.New(), uuid.New()
		exec(`INSERT INTO publications(id,source_id,processing_revision_id,source_asset_id) SELECT $2,id,current_revision_id,primary_asset_id FROM sources WHERE id=$1`, sid, pub)
		exec(`INSERT INTO index_runs(id,publication_id,embedding_config_id,status,expected_chunk_count,indexed_chunk_count) VALUES($1,$2,$3,'ready',1,1)`, run, pub, config)
		exec(`INSERT INTO chunk_embeddings(chunk_id,embedding_config_id,embedding,input_hash) SELECT id,$2,'[1,0,0]',repeat('a',64) FROM chunks WHERE source_id=$1`, sid, config)
		exec(`INSERT INTO active_indexes(source_id,index_run_id) VALUES($1,$2)`, sid, run)
		exec(`UPDATE sources SET status='published',rights_status='allowed',published_revision_id=current_revision_id WHERE id=$1`, sid)
		detail := call("GET", "/materia-medica/entries/"+mm["id"].(string), nil, 200)
		support := detail["locations"].([]any)[0].(map[string]any)
		if support["exact_text"] != text {
			t.Fatal("entry text changed")
		}
		if format != "pdf" {
			call("GET", strings.TrimPrefix(support["original_url"].(string), "/api/v1"), nil, 200)
		}
		call("POST", "/sources/"+sid.String()+"/repertory-entries", body(nil, "MIND", []any{}), 409)
	}
	if remedyIDs[0] != remedyIDs[1] || remedyIDs[0] == remedyIDs[2] {
		t.Fatal("identity/preparation separation failed")
	}
	catalog := call("GET", "/materia-medica/catalog", nil, 200)
	if len(catalog["sources"].([]any)) != 3 || len(catalog["remedies"].([]any)) != 2 {
		t.Fatal("multi-provider catalog wrong")
	}
	repCatalog := call("GET", "/repertory/catalog", nil, 200)
	if len(repCatalog["sources"].([]any)) != 3 {
		t.Fatal("multi-provider repertory catalog wrong")
	}
	for _, x := range catalog["sources"].([]any) {
		if x.(map[string]any)["repository"] == "" {
			t.Fatal("provider attribution lost")
		}
	}
	if call("GET", "/materia-medica/entries?remedy_id="+remedyIDs[0], nil, 200)["total"] != float64(2) {
		t.Fatal("same identity did not span authors")
	}
	if call("GET", "/materia-medica/entries?q=ACONITE&scope=selected&source_id="+sources[1], nil, 200)["total"] != float64(1) {
		t.Fatal("source filter failed")
	}
	if call("GET", "/materia-medica/entries?scope=selected", nil, 200)["total"] != float64(0) {
		t.Fatal("empty selection broadened")
	}
	if call("GET", "/materia-medica/entries?remedy_id="+remedyIDs[2]+"&scope=selected&source_id="+sources[0], nil, 200)["total"] != float64(0) {
		t.Fatal("preparation filter broadened")
	}
	model.Config.EmbeddingRevision = "wrong"
	if call("GET", "/materia-medica/entries", nil, 200)["total"] != float64(0) {
		t.Fatal("incompatible index leaked")
	}
	call("GET", "/materia-medica/entries/"+mmIDs[0], nil, 404)
	model.Config.EmbeddingRevision = "v1"
	exec(`UPDATE sources SET status='disabled' WHERE id=$1`, sources[0])
	call("GET", "/materia-medica/entries/"+mmIDs[0], nil, 404)
	exec(`UPDATE sources SET rights_status='denied' WHERE id=$1`, sources[1])
	call("GET", "/materia-medica/entries/"+mmIDs[1], nil, 404)
	if call("GET", "/materia-medica/entries", nil, 200)["total"] != float64(1) {
		t.Fatal("access restrictions leaked")
	}
	for _, path := range []string{"/sources/" + sources[0] + "/repertory-entries", "/sources/" + sources[0] + "/review-units"} {
		method := "POST"
		if strings.HasSuffix(path, "units") {
			method = "GET"
		}
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+a.ReviewerToken)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != 403 {
			t.Fatal("reviewer wrote structure")
		}
	}
}
