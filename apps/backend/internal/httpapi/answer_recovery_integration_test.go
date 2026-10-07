package httpapi

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestExhaustedAnswerJobBecomesFailedIntegration(t *testing.T) {
	url := os.Getenv("PROVENANCE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PROVENANCE_TEST_DATABASE_URL to a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := core.Open(ctx, url, "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	id := uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO answer_jobs(id,question,research_mode,source_ids,status,attempts,lease_until,owner_role,owner_principal_id)
	VALUES($1,'Who is Nash?','quick','{}','working',3,now()-interval '1 minute','admin',$2)`, id, legacyAdminID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Exec(context.Background(), `DELETE FROM answer_jobs WHERE id=$1`, id)
	defer store.DB.Exec(context.Background(), `DELETE FROM activity_events WHERE answer_job_id=$1`, id)
	if err = (&API{Store: store}).runOneAnswerJob(ctx); err != nil {
		t.Fatal(err)
	}
	var status, code string
	if err = store.DB.QueryRow(ctx, `SELECT status,error_code FROM answer_jobs WHERE id=$1`, id).Scan(&status, &code); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || code != "worker_interrupted" {
		t.Fatalf("stale job status=%q code=%q", status, code)
	}
}
