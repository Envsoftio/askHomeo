package collection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

// Synthetic graph sizes match the task's collection boundaries, not the
// contents or review status of the named historical books.
func TestCollectionCoverageAndRetryIntegration(t *testing.T) {
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, size := range []int{27, 33} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			scope := fixtureScope()
			scope.MaxDocuments, scope.MaxDurationSeconds = size, 180
			scope.RequestDelayMillis = 200
			fetcher := &fixtureFetcher{pages: map[string]string{}}
			var index strings.Builder
			index.WriteString("<html><body>")
			for i := 1; i < size; i++ {
				fmt.Fprintf(&index, `<a href="page%d.html#rubric">Chapter %d</a>`, i, i)
				fetcher.pages[fmt.Sprintf("https://books.example/book/page%d.html", i)] = fmt.Sprintf(`<html><body><h2 id="rubric">Chapter %d</h2><p>Exact substantive fixture passage.</p><a href="index.html">Index</a></body></html>`, i)
			}
			index.WriteString("</body></html>")
			fetcher.pages[scope.SeedURL] = index.String()
			if size == 33 {
				previewScope := scope
				previewScope.MaxDocuments = 30
				preview, err := Preview(ctx, previewScope, fetcher)
				if err != nil || preview.Complete || preview.Fetched != 30 || !contains(preview.LimitReasons, "document limit reached") {
					t.Fatalf("truncated preview: %+v %v", preview, err)
				}
			}
			// Missing page fails after bounded retries; the frontier survives.
			failedURL := "https://books.example/book/page1.html"
			savedPage := fetcher.pages[failedURL]
			delete(fetcher.pages, failedURL)
			collection, snapshot, seed := uuid.New(), uuid.New(), uuid.New()
			raw, _ := json.Marshal(scope)
			exec(`INSERT INTO linked_collections(id,title,author) VALUES($1,'Synthetic graph','Fixture')`, collection)
			exec(`INSERT INTO collection_snapshots(id,collection_id,generation,scope_json) VALUES($1,$2,1,$3)`, snapshot, collection, raw)
			exec(`INSERT INTO collection_items(id,snapshot_id,requested_url,depth,discovery_ordinal) VALUES($1,$2,$3,0,0)`, seed, snapshot, scope.SeedURL)
			exec(`INSERT INTO collection_capture_jobs(snapshot_id) VALUES($1)`, snapshot)
			run := func() {
				t.Helper()
				for i := 0; i < size+10; i++ {
					// Advance persisted scheduling for a network-free fixture. Each
					// pass constructs a fresh worker to test restart continuity.
					exec(`UPDATE collection_capture_jobs SET next_run_at=now() WHERE snapshot_id=$1`, snapshot)
					if err := (&CaptureWorker{Store: store, Fetcher: fetcher}).Once(ctx); err != nil {
						t.Fatal(err)
					}
					var state string
					if err := store.DB.QueryRow(ctx, `SELECT state FROM collection_capture_jobs WHERE snapshot_id=$1`, snapshot).Scan(&state); err != nil {
						t.Fatal(err)
					}
					if state == "done" {
						return
					}
				}
				t.Fatal("capture did not finish within bounded attempts")
			}
			run()
			var failed, targetFailed int
			var reasons []string
			if err := store.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM collection_items WHERE snapshot_id=$1 AND state='failed'),(SELECT count(*) FROM collection_links WHERE snapshot_id=$1 AND state='target_failed'),incomplete_reasons FROM collection_snapshots WHERE id=$1`, snapshot).Scan(&failed, &targetFailed, &reasons); err != nil || failed != 1 || targetFailed != 1 || !contains(reasons, "failed documents") {
				t.Fatalf("lost failure coverage: %d/%d %v %v", failed, targetFailed, reasons, err)
			}
			fetcher.pages[failedURL] = savedPage
			exec(`UPDATE collection_items SET state='queued',attempts=0 WHERE snapshot_id=$1 AND state='failed'`, snapshot)
			exec(`UPDATE collection_snapshots SET state='queued',incomplete_reasons='{}',started_at=NULL WHERE id=$1`, snapshot)
			exec(`UPDATE collection_capture_jobs SET state='queued',next_run_at=now() WHERE snapshot_id=$1`, snapshot)
			run()
			var fetched, unique, unresolved int
			if err := store.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM collection_items WHERE snapshot_id=$1 AND state='fetched'),(SELECT count(DISTINCT requested_url) FROM collection_items WHERE snapshot_id=$1),(SELECT count(*) FROM collection_links WHERE snapshot_id=$1 AND state<>'resolved'),incomplete_reasons FROM collection_snapshots WHERE id=$1`, snapshot).Scan(&fetched, &unique, &unresolved, &reasons); err != nil || fetched != size || unique != size || unresolved != 0 || len(reasons) != 0 {
				t.Fatalf("retry did not reconcile complete graph: %d/%d/%d %v %v", fetched, unique, unresolved, reasons, err)
			}
		})
	}
}
