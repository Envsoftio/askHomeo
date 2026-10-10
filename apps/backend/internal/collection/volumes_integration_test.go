package collection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestCaptureLinkedVolumesWithoutTotalLimitsIntegration(t *testing.T) {
	db := os.Getenv("COLLECTION_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set COLLECTION_TEST_DATABASE_URL to a migrated disposable database")
	}
	ctx := context.Background()
	store, err := core.Open(ctx, db, t.TempDir())
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
	const count = 310
	urls := make([]string, count)
	for i := range urls {
		folder := "kentrep"
		if i/80 > 0 {
			folder += fmt.Sprint(i / 80)
		}
		urls[i] = fmt.Sprintf("https://books.example/books/%s/page%d.htm", folder, i)
	}
	scope := Scope{SeedURL: urls[0], AllowedHosts: []string{"books.example"}, AllowedPathPrefixes: []string{"/books/kentrep/"}, IncludeLinkedVolumes: true, RequestDelayMillis: 200}
	fetcher := &fixtureFetcher{pages: map[string]string{}}
	for i, u := range urls {
		body := `<html><p id="page">Exact book evidence for this page.</p>`
		if i+1 < count {
			body += fmt.Sprintf(`<a href="%s#page">Next page</a>`, urls[i+1])
		}
		fetcher.pages[u] = body + "</html>"
	}
	book, snapshot := uuid.New(), uuid.New()
	raw, _ := json.Marshal(scope)
	exec(`INSERT INTO linked_collections(id,title,author) VALUES($1,'Numbered volumes','Fixture')`, book)
	exec(`INSERT INTO collection_snapshots(id,collection_id,generation,scope_json,started_at,bytes_fetched) VALUES($1,$2,1,$3,now()-interval '2 hours',209715200)`, snapshot, book, raw)
	exec(`INSERT INTO collection_items(id,snapshot_id,requested_url,depth,discovery_ordinal) VALUES($1,$2,$3,0,0)`, uuid.New(), snapshot, scope.SeedURL)
	exec(`INSERT INTO collection_capture_jobs(snapshot_id) VALUES($1)`, snapshot)
	for i := 0; i < count+1; i++ {
		exec(`UPDATE collection_capture_jobs SET next_run_at=now() WHERE snapshot_id=$1`, snapshot)
		if err := (&CaptureWorker{Store: store, Fetcher: fetcher}).Once(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var fetched int
	var state string
	var reasons []string
	err = store.DB.QueryRow(ctx, `SELECT state,incomplete_reasons,(SELECT count(*) FROM collection_items WHERE snapshot_id=$1 AND state='fetched') FROM collection_snapshots WHERE id=$1`, snapshot).Scan(&state, &reasons, &fetched)
	if err != nil || state != "review" || fetched != count || len(reasons) > 0 || len(fetcher.calls) != count {
		t.Fatalf("lost book coverage: state=%s fetched=%d calls=%d reasons=%v err=%v", state, fetched, len(fetcher.calls), reasons, err)
	}
}
