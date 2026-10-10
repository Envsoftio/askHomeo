package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestPermanentDeleteRequiresAdminAndConfirmation(t *testing.T) {
	h := (&API{Token: "delete-admin-token-123456789", ReviewerToken: "delete-reviewer-token-123456789"}).Handler()
	for _, tc := range []struct {
		token, body string
		status      int
	}{{"delete-reviewer-token-123456789", `{}`, 403}, {"delete-admin-token-123456789", `{}`, 400}} {
		r := httptest.NewRequest("DELETE", "/api/v1/sources/"+uuid.NewString()+"/permanent", strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
	}
}
func TestPermanentDeleteDraftIntegration(t *testing.T) {
	url := os.Getenv("DELETE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set DELETE_TEST_DATABASE_URL to a disposable database")
	}
	ctx := context.Background()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	s, err := core.Open(ctx, url, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	s.Root = t.TempDir()
	a := &API{Store: s, Token: "delete-admin-token-123456789"}
	if err = a.SyncPrincipals(ctx); err != nil {
		t.Fatal(err)
	}
	h := a.Handler()
	raw := "<html><title>Draft book</title><body><h1>Draft book</h1><p>Every other work on Materia Medica.</p></body></html><!--" + uuid.NewString() + "-->"
	importDraft := func() uuid.UUID {
		id, e := s.ImportDocument(ctx, strings.NewReader(raw), core.DocumentImport{Title: "Draft book", Author: "Test author", Transport: "upload", ContentType: "text/html"})
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	call := func(id uuid.UUID, title string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"confirm_title": title})
		r := httptest.NewRequest("DELETE", "/api/v1/sources/"+id.String()+"/permanent", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+a.Token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	first, second := importDraft(), importDraft()
	var locator string
	if err = s.DB.QueryRow(ctx, `SELECT document_object_locator FROM sources WHERE id=$1`, first).Scan(&locator); err != nil {
		t.Fatal(err)
	}
	// Include extracted and manually reviewed evidence to exercise dependency order.
	block := uuid.New()
	_, err = s.DB.Exec(ctx, `INSERT INTO document_blocks(id,source_id,source_asset_id,processing_revision_id,block_index,section_key,kind,original_text,reviewed_text,start_byte,end_byte,review_status) SELECT $2,id,primary_asset_id,current_revision_id,0,'section-1','paragraph','abc','abc',0,3,'accepted' FROM sources WHERE id=$1`, first, block)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO chunks(id,source_id,document_block_id,chunk_index,text_exact,start_character,end_character) VALUES($1,$2,$3,0,'abc',0,3)`, uuid.New(), first, block)
	if err != nil {
		t.Fatal(err)
	}
	if res := call(first, "wrong title"); res.Code != 400 {
		t.Fatalf("wrong confirmation: %d", res.Code)
	}
	if res := call(first, "Draft book"); res.Code != 202 {
		t.Fatalf("delete: %d %s", res.Code, res.Body.String())
	}
	var count int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM sources WHERE id=$1`, first).Scan(&count); err != nil || count != 0 {
		t.Fatalf("source retained: %d %v", count, err)
	}
	if err = s.CleanupDeletedFile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, locator)); err != nil {
		t.Fatalf("shared original removed: %v", err)
	}
	// Running work and publication history must block the API even for administrators.
	if _, err = s.DB.Exec(ctx, `UPDATE jobs SET status='running' WHERE source_id=$1`, second); err != nil {
		t.Fatal(err)
	}
	if res := call(second, "Draft book"); res.Code != 409 {
		t.Fatalf("running work delete: %d", res.Code)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE jobs SET status='failed' WHERE source_id=$1`, second); err != nil {
		t.Fatal(err)
	}
	// A removed, unused draft is still eligible for actual deletion.
	if _, err = s.DB.Exec(ctx, `UPDATE sources SET status='disabled',removed_at=now() WHERE id=$1`, second); err != nil {
		t.Fatal(err)
	}
	if res := call(second, "Draft book"); res.Code != 202 {
		t.Fatalf("removed draft delete: %d %s", res.Code, res.Body.String())
	}
	// Re-import before cleanup must retain the newly registered shared asset.
	third := importDraft()
	if err = s.CleanupDeletedFile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, locator)); err != nil {
		t.Fatal("re-import lost its file")
	}
	if res := call(third, "Draft book"); res.Code != 202 {
		t.Fatalf("third delete: %d %s", res.Code, res.Body.String())
	}
	if err = s.CleanupDeletedFile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, locator)); !os.IsNotExist(err) {
		t.Fatalf("unshared file remains: %v", err)
	}
	fourth := importDraft()
	if _, err = s.DB.Exec(ctx, `UPDATE sources SET status='published',published_revision_id=current_revision_id WHERE id=$1`, fourth); err != nil {
		t.Fatal(err)
	}
	if res := call(fourth, "Draft book"); res.Code != 409 {
		t.Fatalf("published delete: %d", res.Code)
	}
	if _, err = s.DB.Exec(ctx, `DELETE FROM source_assets WHERE source_id=$1`, fourth); err == nil {
		t.Fatal("ordinary provenance deletion bypassed guard")
	}
	// PDF repairs and review history are deleted too; failed storage cleanup is durable.
	pid, pageID := uuid.New(), uuid.New()
	sum := sha256.Sum256([]byte("PDF deletion fixture" + uuid.NewString()))
	sha := hex.EncodeToString(sum[:])
	fake := &deleteTestObjects{fail: true}
	s.PDFObjects = fake
	_, err = s.DB.Exec(ctx, `INSERT INTO sources(id,source_key,title,author,pdf_path,pdf_sha256,pdf_bytes,page_count,status) VALUES($1,$2,'PDF draft','Author',$3,$4,20,1,'review')`, pid, "delete-"+pid.String(), fake.Location(sha), sha)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO pages(id,source_id,pdf_page_index,scan_page_index,canvas_id,alto_url,image_url,text_raw,text_sha256,extraction_method) VALUES($1,$2,0,0,'test','','','abc',$3,'test')`, pageID, pid, sha)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO page_text_repairs(id,page_id,processing_revision_id,actor_principal_id,revision,previous_text,corrected_text,rationale) SELECT $2,p.id,p.processing_revision_id,$3,1,'abx','abc','Checked scan' FROM pages p WHERE id=$1`, pageID, uuid.New(), legacyAdminID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO page_review_decisions(id,page_id,processing_revision_id,actor_principal_id,previous_kind,decision_kind,rationale) SELECT $2,p.id,p.processing_revision_id,$3,'text','text','Checked scan' FROM pages p WHERE id=$1`, pageID, uuid.New(), legacyAdminID)
	if err != nil {
		t.Fatal(err)
	}
	if res := call(pid, "PDF draft"); res.Code != 202 {
		t.Fatalf("PDF delete: %d %s", res.Code, res.Body.String())
	}
	if err = s.CleanupDeletedFile(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM deleted_source_files WHERE source_id=$1 AND error<>''`, pid).Scan(&count); err != nil || count != 1 {
		t.Fatalf("lost failed cleanup: %d %v", count, err)
	}
	fake.fail = false
	if _, err = s.DB.Exec(ctx, `UPDATE deleted_source_files SET retry_after=now() WHERE source_id=$1`, pid); err != nil {
		t.Fatal(err)
	}
	if err = s.CleanupDeletedFile(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM deleted_source_files WHERE source_id=$1`, pid).Scan(&count); err != nil || count != 0 || fake.deleted != 1 {
		t.Fatalf("cleanup retry: %d %v", count, err)
	}

}

type deleteTestObjects struct {
	fail    bool
	deleted int
}

func (d *deleteTestObjects) Put(context.Context, string, *os.File) error { return nil }
func (d *deleteTestObjects) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("unused")
}
func (d *deleteTestObjects) Check(context.Context) error { return nil }
func (d *deleteTestObjects) Location(sha string) string {
	return "s3://test-bucket/pdf/" + sha + ".pdf"
}
func (d *deleteTestObjects) DeleteAllVersions(context.Context, string) error {
	if d.fail {
		return errors.New("storage outage")
	}
	d.deleted++
	return nil
}
