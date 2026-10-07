package ingest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestActivateReplacementIntegration(t *testing.T) {
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
	var oldID, candidateID uuid.UUID
	err = store.DB.QueryRow(ctx, `SELECT old.id,candidate.id FROM sources candidate JOIN sources old ON old.id=candidate.supersedes_source_id WHERE old.status='published' AND old.superseded_at IS NULL LIMIT 1`).Scan(&oldID, &candidateID)
	if err != nil {
		t.Skipf("disposable database has no current source with a candidate: %v", err)
	}
	tx, err := store.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = activateReplacement(ctx, tx, candidateID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var replaced bool
	if err = store.DB.QueryRow(ctx, `SELECT superseded_at IS NOT NULL FROM sources WHERE id=$1`, oldID).Scan(&replaced); err != nil || !replaced {
		t.Fatalf("old source was not superseded: %v %v", replaced, err)
	}
	second, err := store.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Rollback(ctx)
	if err = activateReplacement(ctx, second, candidateID); err == nil {
		t.Fatal("second activation should fail")
	}
}
