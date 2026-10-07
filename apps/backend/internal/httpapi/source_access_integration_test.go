package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestDisableAndRestoreCitationIntegration(t *testing.T) {
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
	var sourceID, citationID uuid.UUID
	err = store.DB.QueryRow(ctx, `SELECT s.id,ac.id FROM sources s JOIN chunks c ON c.source_id=s.id JOIN answer_citations ac ON ac.chunk_id=c.id WHERE s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL LIMIT 1`).Scan(&sourceID, &citationID)
	if err != nil {
		t.Skipf("disposable database has no published cited source: %v", err)
	}
	handler := (&API{Store: store, Token: "integration-test-admin-token-12345"}).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	path := "/api/v1/sources/" + sourceID.String()
	if res := call(http.MethodPost, path+"/disable", `{"reason":"Source temporarily withdrawn for review"}`); res.Code != http.StatusOK {
		t.Fatalf("disable HTTP %d: %s", res.Code, res.Body.String())
	}
	defer func() {
		_ = call(http.MethodPost, path+"/enable", `{"reason":"Integration test restore after citation check"}`)
	}()
	if res := call(http.MethodGet, "/api/v1/citations/"+citationID.String(), ""); res.Code != http.StatusNotFound {
		t.Fatalf("disabled citation HTTP %d", res.Code)
	}
	if res := call(http.MethodPost, path+"/enable", `{"reason":"Review completed and source restored"}`); res.Code != http.StatusOK {
		t.Fatalf("enable HTTP %d: %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodGet, "/api/v1/citations/"+citationID.String(), ""); res.Code != http.StatusOK {
		t.Fatalf("restored citation HTTP %d: %s", res.Code, res.Body.String())
	}
}
