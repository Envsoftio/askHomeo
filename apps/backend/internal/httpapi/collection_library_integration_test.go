package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestCombinedBookEditingAndBulkDeletionIntegration(t *testing.T) {
	db := os.Getenv("COLLECTION_API_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set COLLECTION_API_TEST_DATABASE_URL to a disposable database")
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	api := &API{Store: store, Token: "combined-book-admin-fixture-12345", ReviewerToken: "combined-book-reviewer-fixture-12345"}
	if err = api.SyncPrincipals(ctx); err != nil {
		t.Fatal(err)
	}
	handler := api.Handler()
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	book, snapshot := uuid.New(), uuid.New()
	exec(`INSERT INTO linked_collections(id,title,author) VALUES($1,'Combined test book','Fixture')`, book)
	exec(`INSERT INTO collection_snapshots(id,collection_id,generation,scope_json,state) VALUES($1,$2,1,'{}','review')`, snapshot, book)
	ids := []uuid.UUID{}
	for i := 0; i < 3; i++ {
		sid, err := store.ImportDocument(ctx, strings.NewReader("<html><p>Book evidence.</p></html><!--"+uuid.NewString()+"-->"), core.DocumentImport{Title: "Chapter", Author: "Fixture", Transport: "upload", ContentType: "text/html"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sid)
		exec(`UPDATE sources SET status='review' WHERE id=$1`, sid)
		exec(`UPDATE jobs SET status='failed' WHERE source_id=$1`, sid)
		if i == 2 {
			continue
		} // A standalone source must survive book deletion.
		exec(`INSERT INTO document_blocks(id,source_id,source_asset_id,processing_revision_id,block_index,section_key,kind,original_text,reviewed_text,start_byte,end_byte,review_status) SELECT $2,id,primary_asset_id,current_revision_id,0,'section-1','paragraph','Keep this passage. Remove the tail.','Keep this passage. Remove the tail.',0,35,'pending' FROM sources WHERE id=$1`, sid, uuid.New())
		exec(`INSERT INTO collection_items(id,snapshot_id,requested_url,final_url,sha256,byte_size,object_locator,format,depth,discovery_ordinal,state,role,source_id,preparation_state) VALUES($1,$2,$3,$3,repeat('a',64),1,'fixture','html',0,$4,'fetched','content_candidate',$5,'done')`, uuid.New(), snapshot, "https://fixture.example/book/"+sid.String(), i, sid)
	}
	path := "/api/v1/collections/" + book.String()
	res := call("GET", path+"/review-text", api.Token, nil)
	if res.Code != 200 {
		t.Fatalf("open editor: %d %s", res.Code, res.Body.String())
	}
	var review struct {
		PlainText string `json:"plain_text"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(review.PlainText, "Keep this passage. Remove the tail.", "Keep this passage.", 1)
	res = call("PUT", path+"/review-text", api.Token, map[string]any{"snapshot_id": snapshot, "plain_text": edited})
	if res.Code != 200 {
		t.Fatalf("save combined text: %d %s", res.Code, res.Body.String())
	}
	var text string
	if err = store.DB.QueryRow(ctx, `SELECT reviewed_text FROM document_blocks b JOIN sources s ON s.current_revision_id=b.processing_revision_id WHERE s.id=$1`, ids[0]).Scan(&text); err != nil || text != "Keep this passage." {
		t.Fatalf("saved passage: %q %v", text, err)
	}
	for _, tc := range []struct {
		token, title string
		status       int
	}{{api.ReviewerToken, "Combined test book", 403}, {api.Token, "Wrong", 400}} {
		res = call("DELETE", path+"/sources", tc.token, map[string]string{"confirm_title": tc.title})
		if res.Code != tc.status {
			t.Fatalf("delete gate: %d want %d", res.Code, tc.status)
		}
	}
	ordered := append([]uuid.UUID{}, ids[:2]...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })
	exec(`UPDATE jobs SET status='running' WHERE source_id=$1`, ordered[1])
	res = call("DELETE", path+"/sources", api.Token, map[string]string{"confirm_title": "Combined test book"})
	if res.Code != 409 {
		t.Fatalf("running work did not block batch: %d %s", res.Code, res.Body.String())
	}
	var count int
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM sources WHERE id=ANY($1::uuid[])`, ids).Scan(&count); err != nil || count != 3 {
		t.Fatalf("batch was not atomic: %d %v", count, err)
	}
	exec(`UPDATE jobs SET status='failed' WHERE source_id=$1`, ordered[1])
	res = call("DELETE", path+"/sources", api.Token, map[string]string{"confirm_title": "Combined test book"})
	if res.Code != 200 {
		t.Fatalf("bulk delete: %d %s", res.Code, res.Body.String())
	}
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM sources WHERE id=ANY($1::uuid[])`, ids).Scan(&count); err != nil || count != 1 {
		t.Fatalf("wrong deletion scope: %d %v", count, err)
	}
}

func TestBookSelectionIncludesMoreThanFiftyPageSourcesIntegration(t *testing.T) {
	db := os.Getenv("COLLECTION_API_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set COLLECTION_API_TEST_DATABASE_URL to a disposable database")
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
	a := &API{Store: store}
	book, snapshot := uuid.New(), uuid.New()
	if _, err = store.DB.Exec(ctx, `INSERT INTO linked_collections(id,title,author) VALUES($1,'Large selection','Fixture')`, book); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `INSERT INTO collection_snapshots(id,collection_id,generation,scope_json,state) VALUES($1,$2,1,'{}','review')`, snapshot, book); err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{}
	for i := 0; i < 51; i++ {
		sid := uuid.New()
		ids = append(ids, sid)
		if _, err = store.DB.Exec(ctx, `INSERT INTO sources(id,source_key,title,author,pdf_path,pdf_sha256,pdf_bytes,page_count,status) VALUES($1,$2,'Chapter','Fixture','fixture.pdf',repeat('a',64),1,1,'review')`, sid, uuid.NewString()); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest("POST", "/", nil)
	if a.validBookSelectionSize(req, ids) {
		t.Fatal("51 independent selections were accepted")
	}
	for i, sid := range ids {
		if _, err = store.DB.Exec(ctx, `INSERT INTO collection_items(id,snapshot_id,requested_url,depth,discovery_ordinal,source_id) VALUES($1,$2,$3,0,$4,$5)`, uuid.New(), snapshot, "https://fixture.example/"+sid.String(), i, sid); err != nil {
			t.Fatal(err)
		}
	}
	if !a.validBookSelectionSize(req, ids) {
		t.Fatal("one book was rejected because it has more than 50 page sources")
	}
}
