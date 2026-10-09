package collection

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

// Run against a migrated disposable database only. Each Once call uses a new
// worker instance to exercise persistent frontier and job state.
func TestCaptureResumesAcrossWorkerInstancesIntegration(t *testing.T) {
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
	scope := fixtureScope()
	scope.SeedURL = "https://books.example/book/index.html"
	scope.RequestDelayMillis = 200
	raw, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	collectionID, snapshotID, seedID := uuid.New(), uuid.New(), uuid.New()
	if _, err = store.DB.Exec(ctx, `INSERT INTO linked_collections(id,title,author) VALUES($1,'Fixture book','Fixture author')`, collectionID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.DB.Exec(ctx, `DELETE FROM collection_links WHERE snapshot_id=$1`, snapshotID)
		_, _ = store.DB.Exec(ctx, `DELETE FROM collection_capture_jobs WHERE snapshot_id=$1`, snapshotID)
		_, _ = store.DB.Exec(ctx, `DELETE FROM collection_items WHERE snapshot_id=$1`, snapshotID)
		_, _ = store.DB.Exec(ctx, `DELETE FROM collection_snapshots WHERE id=$1`, snapshotID)
		_, _ = store.DB.Exec(ctx, `DELETE FROM linked_collections WHERE id=$1`, collectionID)
	})
	if _, err = store.DB.Exec(ctx, `INSERT INTO collection_snapshots(id,collection_id,generation,scope_json) VALUES($1,$2,1,$3)`, snapshotID, collectionID, raw); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `INSERT INTO collection_items(id,snapshot_id,requested_url,depth,discovery_ordinal) VALUES($1,$2,$3,0,0)`, seedID, snapshotID, scope.SeedURL); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `INSERT INTO collection_capture_jobs(snapshot_id) VALUES($1)`, snapshotID); err != nil {
		t.Fatal(err)
	}
	fetcher := &fixtureFetcher{pages: map[string]string{
		"https://books.example/book/index.html":      `<html><body><a href="section.html">Section</a><a href="/outside/ad.html">Outside</a></body></html>`,
		"https://books.example/book/section.html":    `<html><body><a href="../sibling/content.html#one">One</a><a href="../sibling/content.html#missing">Missing</a></body></html>`,
		"https://books.example/sibling/content.html": `<html><body><h2 id="one">Remedy</h2><p>Substantive source text</p></body></html>`,
	}}
	var state string
	for i := 0; i < 20; i++ {
		if err = (&CaptureWorker{Store: store, Fetcher: fetcher}).Once(ctx); err != nil {
			t.Fatal(err)
		}
		if err = store.DB.QueryRow(ctx, `SELECT state FROM collection_capture_jobs WHERE snapshot_id=$1`, snapshotID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "done" {
			break
		}
		time.Sleep(220 * time.Millisecond)
	}
	if state != "done" {
		t.Fatalf("capture did not finish: %s", state)
	}
	var itemCount, fetched, missing, excluded, sourceCount int
	if err = store.DB.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE state='fetched'),count(source_id) FROM collection_items WHERE snapshot_id=$1`, snapshotID).Scan(&itemCount, &fetched, &sourceCount); err != nil {
		t.Fatal(err)
	}
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='missing_anchor'),count(*) FILTER(WHERE state='excluded_scope') FROM collection_links WHERE snapshot_id=$1`, snapshotID).Scan(&missing, &excluded); err != nil {
		t.Fatal(err)
	}
	if itemCount != 3 || fetched != 3 || sourceCount != 0 || missing != 1 || excluded != 1 || len(fetcher.calls) != 3 {
		t.Fatalf("unexpected persisted coverage: items=%d fetched=%d sources=%d missing=%d excluded=%d calls=%v", itemCount, fetched, sourceCount, missing, excluded, fetcher.calls)
	}
	var reasons []string
	if err = store.DB.QueryRow(ctx, `SELECT incomplete_reasons FROM collection_snapshots WHERE id=$1`, snapshotID).Scan(&reasons); err != nil {
		t.Fatal(err)
	}
	if !contains(reasons, "missing anchors") {
		t.Fatalf("partial coverage lost: %v", reasons)
	}
}
