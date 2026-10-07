package ingest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestExhaustedSourceJobBecomesFailedIntegration(t *testing.T) {
	dbURL := os.Getenv("PROVENANCE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set PROVENANCE_TEST_DATABASE_URL to a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	var id uuid.UUID
	if err = store.DB.QueryRow(ctx, `SELECT id FROM jobs WHERE kind='text_qa' AND status='done' LIMIT 1`).Scan(&id); err != nil {
		t.Skipf("no completed text check job: %v", err)
	}
	_, err = store.DB.Exec(ctx, `UPDATE jobs SET status='running',attempts=5,lease_until=now()-interval '1 minute' WHERE id=$1`, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = New(store).reconcileExpiredJobs(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	var events int
	if err = store.DB.QueryRow(ctx, `SELECT status FROM jobs WHERE id=$1`, id).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("stale job status=%q err=%v", status, err)
	}
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM activity_events WHERE source_job_id=$1 AND kind='failed'`, id).Scan(&events); err != nil || events == 0 {
		t.Fatalf("missing durable failure event: count=%d err=%v", events, err)
	}
}
