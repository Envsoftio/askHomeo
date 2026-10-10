package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

// Requires a disposable, fully migrated database. It checks the restart-safe
// item identity and the active-pointer gate with real PostgreSQL constraints.
func TestPreparedCollectionReplacementIntegration(t *testing.T) {
	dbURL := os.Getenv("COLLECTION_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set COLLECTION_TEST_DATABASE_URL to a migrated disposable database")
	}
	ctx := context.Background()
	store, err := core.Open(ctx, dbURL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	collectionID, firstSnapshot, secondSnapshot := uuid.New(), uuid.New(), uuid.New()
	firstItem, secondItem := uuid.New(), uuid.New()
	if _, err = store.DB.Exec(ctx, `INSERT INTO linked_collections(id,title,author) VALUES($1,'Integration book','Integration author')`, collectionID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `INSERT INTO collection_snapshots(id,collection_id,generation,scope_json,state) VALUES($1,$2,1,'{}','review')`, firstSnapshot, collectionID); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`<html><head><title>Fixture chapter</title></head><body><h1>Chapter</h1><p>Distinctive reviewable passage in this chapter.</p></body></html>`)
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	locator := filepath.Join("data/runtime/assets", sha+".html")
	if err = os.MkdirAll(filepath.Dir(filepath.Join(store.Root, locator)), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(store.Root, locator), raw, 0600); err != nil {
		t.Fatal(err)
	}
	firstURL := "https://books.example/book/first.html"
	if _, err = store.DB.Exec(ctx, `INSERT INTO collection_items(id,snapshot_id,requested_url,final_url,depth,discovery_ordinal,state,role,format,content_type,sha256,byte_size,object_locator)
 VALUES($1,$2,$3,$3,0,0,'fetched','content_candidate','html','text/html',$4,$5,$6)`, firstItem, firstSnapshot, firstURL, sha, len(raw), locator); err != nil {
		t.Fatal(err)
	}
	if err = (&CaptureWorker{Store: store}).prepareOne(ctx); err != nil {
		t.Fatal(err)
	}
	var preparedSource uuid.UUID
	var preparationState string
	if err = store.DB.QueryRow(ctx, `SELECT source_id,preparation_state FROM collection_items WHERE id=$1`, firstItem).Scan(&preparedSource, &preparationState); err != nil {
		t.Fatal(err)
	}
	if preparedSource != firstItem || preparationState != "done" {
		t.Fatalf("preparation identity/state = %s/%s", preparedSource, preparationState)
	}
	if err = (&CaptureWorker{Store: store}).prepareOne(ctx); err != nil {
		t.Fatal(err)
	}
	var sourceCount int
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM sources WHERE id=$1`, firstItem).Scan(&sourceCount); err != nil || sourceCount != 1 {
		t.Fatalf("idempotent source count = %d: %v", sourceCount, err)
	}
	if _, err = store.DB.Exec(ctx, `INSERT INTO collection_snapshots(id,collection_id,generation,scope_json,state) VALUES($1,$2,2,'{}','review')`, secondSnapshot, collectionID); err != nil {
		t.Fatal(err)
	}
	secondURL := "https://books.example/book/second.html"
	if _, err = store.ImportDocument(ctx, bytes.NewReader(raw), core.DocumentImport{SourceID: &secondItem, Title: "Second chapter", Author: "Integration author", SourceURL: secondURL, RequestedURL: secondURL, FinalURL: secondURL, Transport: "https", ContentType: "text/html"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `INSERT INTO collection_items(id,snapshot_id,requested_url,final_url,depth,discovery_ordinal,state,role,format,content_type,sha256,byte_size,object_locator,source_id,preparation_state)
 VALUES($1,$2,$3,$3,0,0,'fetched','content_candidate','html','text/html',$4,$5,$6,$1,'done')`, secondItem, secondSnapshot, secondURL, sha, len(raw), locator); err != nil {
		t.Fatal(err)
	}
	assertEligibility := func(first, second bool) {
		t.Helper()
		var gotFirst, gotSecond bool
		if err = store.DB.QueryRow(ctx, `SELECT collection_source_retrieval_eligible($1),collection_source_retrieval_eligible($2)`, firstItem, secondItem).Scan(&gotFirst, &gotSecond); err != nil {
			t.Fatal(err)
		}
		if gotFirst != first || gotSecond != second {
			t.Fatalf("eligibility = %t/%t, want %t/%t", gotFirst, gotSecond, first, second)
		}
	}
	assertEligibility(true, true)
	if _, err = store.DB.Exec(ctx, `INSERT INTO active_collection_snapshots(collection_id,snapshot_id) VALUES($1,$2)`, collectionID, firstSnapshot); err != nil {
		t.Fatal(err)
	}
	assertEligibility(true, false)
	if _, err = store.DB.Exec(ctx, `UPDATE active_collection_snapshots SET snapshot_id=$2 WHERE collection_id=$1`, collectionID, secondSnapshot); err != nil {
		t.Fatal(err)
	}
	assertEligibility(false, true)
}
