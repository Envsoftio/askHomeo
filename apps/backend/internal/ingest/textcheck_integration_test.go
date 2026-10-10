package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestFailedScanChecksRemainVisibleIntegration(t *testing.T) {
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
	store.Root = t.TempDir()
	// A checksum-valid stored asset whose rendering fails must leave every page
	// visible for review, even if it had been reviewed before the new QA pass.
	data := []byte("invalid PDF fixture")
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	path := store.PDFPath(sha)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	sid, jid := uuid.New(), uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO sources(id,source_key,title,author,pdf_path,pdf_sha256,pdf_bytes,page_count,status) VALUES($1,$2,'Scan QA test','Author',$3,$4,$5,2,'review')`, sid, "qa-"+sid.String(), path, sha, len(data))
	if err != nil {
		t.Fatal(err)
	}
	for i, status := range []string{"reviewed", "needs_review"} {
		_, err = store.DB.Exec(ctx, `INSERT INTO pages(id,source_id,pdf_page_index,scan_page_index,canvas_id,alto_url,image_url,text_raw,text_sha256,extraction_method,review_status) VALUES($1,$2,$3,$3,$4,'','','old transcription',$5,'test',$6)`, uuid.New(), sid, i, uuid.NewString(), strings.Repeat("a", 64), status)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.DB.Exec(ctx, `INSERT INTO jobs(id,source_id,kind,status,current_step,total) VALUES($1,$2,'text_qa','running','test',2)`, jid, sid)
	if err != nil {
		t.Fatal(err)
	}
	if err = New(store).checkText(ctx, jid, sid); err != nil {
		t.Fatal(err)
	}
	var flagged int
	var status string
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM pages WHERE source_id=$1 AND text_qa_status='suspect' AND review_status='needs_review' AND text_qa_at IS NOT NULL`, sid).Scan(&flagged); err != nil || flagged != 2 {
		t.Fatalf("flagged=%d err=%v", flagged, err)
	}
	if err = store.DB.QueryRow(ctx, `SELECT status FROM jobs WHERE id=$1`, jid).Scan(&status); err != nil || status != "done" {
		t.Fatalf("job=%s err=%v", status, err)
	}
}
