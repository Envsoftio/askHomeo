package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestPageRepairRequiresAdmin(t *testing.T) {
	handler := (&API{ReviewerToken: "page-repair-reviewer-token"}).Handler()
	for _, tc := range []struct{ method, path string }{{"PUT", "text"}, {"POST", "ocr"}} {
		req := httptest.NewRequest(tc.method, "/api/v1/pages/"+uuid.NewString()+"/"+tc.path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer page-repair-reviewer-token")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusForbidden {
			t.Fatalf("%s returned %d", tc.path, res.Code)
		}
	}
}

func TestPageTextRepairIntegration(t *testing.T) {
	url := os.Getenv("OCR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set OCR_TEST_DATABASE_URL to a disposable database")
	}
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := core.Open(ctx, url, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	a := &API{Store: store, Token: "page-repair-admin-token-123456789"}
	if err = a.SyncPrincipals(ctx); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	sid, pid := uuid.New(), uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO sources(id,source_key,title,author,pdf_path,pdf_sha256,pdf_bytes,page_count,status) VALUES($1,$2,'OCR test','Author','unused.pdf',$3,1,1,'review')`, sid, "repair-"+sid.String(), strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	original := "every other w(U'k on Materia Mcdica"
	digest := sha256.Sum256([]byte(original))
	_, err = store.DB.Exec(ctx, `INSERT INTO pages(id,source_id,pdf_page_index,scan_page_index,canvas_id,alto_url,image_url,text_raw,text_sha256,extraction_method,text_qa_status,page_kind) VALUES($1,$2,0,0,'test','','',$3,$4,'test','suspect','missing_text')`, pid, sid, original, hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer page-repair-admin-token-123456789")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	path := "/api/v1/pages/" + pid.String() + "/text"
	corrected := "every other work on Materia Medica — café " + strings.Repeat("verified text and words ", 80)
	payload := map[string]any{"text": corrected, "note": "Checked all lines against the original scan", "text_revision": 0}
	if res := call("PUT", path, payload); res.Code != 200 {
		t.Fatalf("save %d: %s", res.Code, res.Body.String())
	}
	var raw, hash, qa, kind, old, saved string
	var revision, chunkCount, badOffsets int
	err = store.DB.QueryRow(ctx, `SELECT text_raw,text_sha256,text_qa_status,page_kind,text_revision,(SELECT count(*) FROM chunks WHERE page_id=$1),(SELECT count(*) FROM chunks c WHERE c.page_id=p.id AND substring(p.text_raw from c.start_character+1 for c.end_character-c.start_character)<>c.text_exact) FROM pages p WHERE id=$1`, pid).Scan(&raw, &hash, &qa, &kind, &revision, &chunkCount, &badOffsets)
	sum := sha256.Sum256([]byte(corrected))
	if err != nil || raw != corrected || hash != hex.EncodeToString(sum[:]) || qa != "accepted" || kind != "text" || revision != 1 || chunkCount < 2 || badOffsets != 0 {
		t.Fatalf("repair state: qa=%s kind=%s revision=%d chunks=%d offsets=%d err=%v", qa, kind, revision, chunkCount, badOffsets, err)
	}
	err = store.DB.QueryRow(ctx, `SELECT previous_text,corrected_text FROM page_text_repairs WHERE page_id=$1`, pid).Scan(&old, &saved)
	if err != nil || old != original || saved != corrected {
		t.Fatalf("history: %v", err)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE page_text_repairs SET rationale='overwrite' WHERE page_id=$1`, pid); err == nil {
		t.Fatal("history was mutable")
	}
	if res := call("PUT", path, payload); res.Code != 409 {
		t.Fatalf("stale edit = %d", res.Code)
	}
	payload["text_revision"] = 1
	payload["text"] = "  "
	if res := call("PUT", path, payload); res.Code != 400 {
		t.Fatalf("empty edit = %d", res.Code)
	}
	// A saved correction cannot be overwritten after publication.
	if _, err = store.DB.Exec(ctx, `UPDATE sources SET status='published' WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	payload["text"] = corrected
	if res := call("PUT", path, payload); res.Code != 409 {
		t.Fatalf("published edit = %d: %s", res.Code, res.Body.String())
	}
	if res := call("POST", "/api/v1/pages/"+pid.String()+"/ocr", nil); res.Code != 409 {
		t.Fatalf("published OCR = %d", res.Code)
	}
}
