package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestReprocessPreservesPublishedCitationIntegration(t *testing.T) {
	dbURL := os.Getenv("PROVENANCE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set PROVENANCE_TEST_DATABASE_URL to a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	var oldID, citationID uuid.UUID
	err = store.DB.QueryRow(ctx, `SELECT s.id,ac.id FROM sources s JOIN chunks c ON c.source_id=s.id JOIN answer_citations ac ON ac.chunk_id=c.id WHERE s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL AND NOT EXISTS(SELECT 1 FROM sources candidate WHERE candidate.supersedes_source_id=s.id AND candidate.status NOT IN ('failed','disabled')) ORDER BY s.created_at LIMIT 1`).Scan(&oldID, &citationID)
	if err != nil {
		t.Skipf("disposable database has no published cited source: %v", err)
	}
	handler := (&API{Store: store, Token: "integration-test-admin-token-12345"}).Handler()
	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	res := request(http.MethodPost, "/api/v1/sources/"+oldID.String()+"/reprocess")
	if res.Code != http.StatusAccepted {
		t.Fatalf("reprocess HTTP %d: %s", res.Code, res.Body.String())
	}
	var result struct {
		SourceID uuid.UUID `json:"source_id"`
	}
	if err = json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var sourceAsset, jobRevision, sourceRevision uuid.UUID
	var previousCurrent bool
	err = store.DB.QueryRow(ctx, `SELECT s.primary_asset_id,s.current_revision_id,j.processing_revision_id,(SELECT superseded_at IS NULL FROM sources WHERE id=$2) FROM sources s JOIN jobs j ON j.source_id=s.id AND j.kind='ingest' WHERE s.id=$1`, result.SourceID, oldID).Scan(&sourceAsset, &sourceRevision, &jobRevision, &previousCurrent)
	if err != nil || sourceAsset == uuid.Nil || sourceRevision == uuid.Nil || sourceRevision != jobRevision || !previousCurrent {
		t.Fatalf("candidate lineage: asset=%v revision=%v job=%v oldCurrent=%v err=%v", sourceAsset, sourceRevision, jobRevision, previousCurrent, err)
	}
	citation := request(http.MethodGet, "/api/v1/citations/"+citationID.String())
	if citation.Code != http.StatusOK {
		t.Fatalf("prior citation HTTP %d: %s", citation.Code, citation.Body.String())
	}
	var body map[string]json.RawMessage
	if err = json.Unmarshal(citation.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"edition_id", "source_asset_id", "processing_revision_id", "publication_id", "pdf_sha256"} {
		if len(body[field]) == 0 {
			t.Fatalf("citation missing %s", field)
		}
	}
	repeat := request(http.MethodPost, "/api/v1/sources/"+oldID.String()+"/reprocess")
	if repeat.Code != http.StatusConflict {
		t.Fatalf("duplicate candidate HTTP %d: %s", repeat.Code, repeat.Body.String())
	}
	// Simulate the atomic READY-index switch after a candidate has completed.
	if _, err = store.DB.Exec(ctx, `UPDATE sources SET superseded_at=now() WHERE id=$1`, oldID); err != nil {
		t.Fatal(err)
	}
	if citation = request(http.MethodGet, "/api/v1/citations/"+citationID.String()); citation.Code != http.StatusOK {
		t.Fatalf("citation after replacement HTTP %d: %s", citation.Code, citation.Body.String())
	}
	var eligible bool
	if err = store.DB.QueryRow(ctx, `SELECT status='published' AND rights_status='allowed' AND superseded_at IS NULL FROM sources WHERE id=$1`, oldID).Scan(&eligible); err != nil || eligible {
		t.Fatalf("old source still eligible for new retrieval: %v, %v", eligible, err)
	}
}
