package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestRightsDecisionHistoryIntegration(t *testing.T) {
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
	var title, author, edition, publication, repository, sourceURL, statement string
	err = store.DB.QueryRow(ctx, `SELECT id,title,author,edition,publication_info,repository,coalesce(source_url,''),rights_statement FROM sources WHERE status='review' LIMIT 1`).Scan(&id, &title, &author, &edition, &publication, &repository, &sourceURL, &statement)
	if err != nil {
		t.Skipf("disposable database has no review source: %v", err)
	}
	handler := (&API{Store: store, Token: "integration-test-admin-token-12345"}).Handler()
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	rightsPath := "/api/v1/sources/" + id.String() + "/rights"
	first := post(rightsPath, `{"decision":"allowed","note":"Reviewed the catalogue and local use terms."}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first decision %d: %s", first.Code, first.Body.String())
	}
	var firstID uuid.UUID
	if err = store.DB.QueryRow(ctx, `SELECT rights_decision_id FROM sources WHERE id=$1`, id).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE rights_decisions SET rationale='changed' WHERE id=$1`, firstID); err == nil {
		t.Fatal("rights decision was mutable")
	}
	metadata, _ := json.Marshal(map[string]string{"title": title, "author": author, "edition": edition, "publication_info": publication, "repository": repository, "source_url": sourceURL, "rights_statement": statement + " (corrected)"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sources/"+id.String()+"/metadata", strings.NewReader(string(metadata)))
	req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("metadata correction %d: %s", res.Code, res.Body.String())
	}
	var current *uuid.UUID
	var status string
	if err = store.DB.QueryRow(ctx, `SELECT rights_decision_id,rights_status FROM sources WHERE id=$1`, id).Scan(&current, &status); err != nil || current != nil || status != "needs_review" {
		t.Fatalf("stale decision remained: %v %s %v", current, status, err)
	}
	second := post(rightsPath, `{"decision":"allowed","note":"Reviewed corrected statement for local research."}`)
	if second.Code != http.StatusOK {
		t.Fatalf("second decision %d: %s", second.Code, second.Body.String())
	}
	var secondID uuid.UUID
	var count int
	if err = store.DB.QueryRow(ctx, `SELECT rights_decision_id,(SELECT count(*) FROM rights_decisions WHERE source_id=$1) FROM sources WHERE id=$1`, id).Scan(&secondID, &count); err != nil || firstID == secondID || count < 2 {
		t.Fatalf("decision history missing: %v %v %d %v", firstID, secondID, count, err)
	}
}
