package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/httpapi"
	"homeopath-poc/backend/internal/localllm"
)

func TestPublishProvenanceIntegration(t *testing.T) {
	dbURL := os.Getenv("INTAKE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set INTAKE_TEST_DATABASE_URL to a disposable one-page intake database")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var sourceID, pageID uuid.UUID
	err = store.DB.QueryRow(ctx, `SELECT s.id,p.id FROM sources s JOIN pages p ON p.source_id=s.id WHERE s.status='review' AND p.pdf_page_index=0 AND EXISTS(SELECT 1 FROM jobs j WHERE j.source_id=s.id AND j.kind='text_qa' AND j.status='queued') LIMIT 1`).Scan(&sourceID, &pageID)
	if err != nil {
		t.Skipf("no one-page review fixture with queued QA: %v", err)
	}
	if err = New(store).once(ctx); err != nil {
		t.Fatal(err)
	}
	api := (&httpapi.API{Store: store, Token: "integration-test-admin-token-12345", Model: localllm.New(localllm.Config{EmbeddingModel: "test-embedding", EmbeddingRevision: "test-v1", Dimensions: 3})}).Handler()
	send := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
		res := httptest.NewRecorder()
		api.ServeHTTP(res, req)
		return res
	}
	var qaStatus string
	if err = store.DB.QueryRow(ctx, `SELECT text_qa_status FROM pages WHERE id=$1`, pageID).Scan(&qaStatus); err != nil {
		t.Fatal(err)
	}
	if qaStatus == "suspect" {
		res := send(http.MethodPost, "/api/v1/pages/"+pageID.String()+"/review", `{"outcome":"text","note":"Checked generated fixture scan against extracted text."}`)
		if res.Code != http.StatusOK {
			t.Fatalf("page review %d: %s", res.Code, res.Body.String())
		}
	}
	rights := send(http.MethodPost, "/api/v1/sources/"+sourceID.String()+"/rights", `{"decision":"allowed","note":"Generated fixture is permitted for this local integration test."}`)
	if rights.Code != http.StatusOK {
		t.Fatalf("rights %d: %s", rights.Code, rights.Body.String())
	}
	published := send(http.MethodPost, "/api/v1/sources/"+sourceID.String()+"/publish", "")
	if published.Code != http.StatusOK {
		t.Fatalf("publish %d: %s", published.Code, published.Body.String())
	}
	var decisionID, publicationDecisionID, revisionID, publicationRevisionID uuid.UUID
	var metadata, labels []byte
	err = store.DB.QueryRow(ctx, `SELECT s.rights_decision_id,p.rights_decision_id,s.published_revision_id,p.processing_revision_id,p.metadata_snapshot,p.page_labels_snapshot FROM sources s JOIN publications p ON p.source_id=s.id WHERE s.id=$1`, sourceID).Scan(&decisionID, &publicationDecisionID, &revisionID, &publicationRevisionID, &metadata, &labels)
	if err != nil || decisionID == uuid.Nil || decisionID != publicationDecisionID || revisionID != publicationRevisionID {
		t.Fatalf("publication lineage mismatch: %v %v %v %v %v", decisionID, publicationDecisionID, revisionID, publicationRevisionID, err)
	}
	var snapshot map[string]any
	if err = json.Unmarshal(metadata, &snapshot); err != nil || snapshot["title"] != "Test book" {
		t.Fatalf("metadata snapshot: %s %v", metadata, err)
	}
	if string(labels) == "{}" {
		t.Fatalf("missing page label snapshot: %s", labels)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE publications SET metadata_snapshot='{}'::jsonb WHERE source_id=$1`, sourceID); err == nil {
		t.Fatal("publication snapshot was mutable")
	}
}
