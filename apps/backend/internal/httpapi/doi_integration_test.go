package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/doi"
)

func TestDOIReferenceOnlyIntegration(t *testing.T) {
	dbURL := os.Getenv("INTAKE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set INTAKE_TEST_DATABASE_URL to an isolated database")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"DOI":"10.1371/journal.pone.0134657","title":["Homeopathy study"],"author":[{"given":"A","family":"Author"}],"publisher":"PLOS","type":"journal-article","published":{"date-parts":[[2015]]},"license":[{"URL":"https://creativecommons.org/licenses/by/4.0/"}]}}`))
	}))
	defer provider.Close()
	handler := (&API{Store: store, Token: "integration-test-admin-token-12345", DOI: &doi.Client{BaseURL: provider.URL, HTTP: provider.Client()}}).Handler()
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/doi-references", bytes.NewBufferString(`{"doi":"https://doi.org/10.1371/journal.pone.0134657"}`))
		req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != 200 {
			t.Fatalf("add DOI %d: %s", res.Code, res.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/doi-references", nil)
	req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("list DOI %d: %s", res.Code, res.Body.String())
	}
	var rows []struct {
		DOI          string `json:"doi"`
		PDFAvailable bool   `json:"pdf_available"`
		SourceID     string `json:"source_id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DOI != "10.1371/journal.pone.0134657" || rows[0].PDFAvailable || rows[0].SourceID != "" {
		t.Fatalf("reference-only state: %#v", rows)
	}
	var id uuid.UUID
	if err := store.DB.QueryRow(ctx, `SELECT id FROM doi_references WHERE doi=$1`, rows[0].DOI).Scan(&id); err != nil {
		t.Fatal(err)
	}
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/doi-references/"+id.String(), nil)
	deleteReq.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != 200 {
		t.Fatalf("delete DOI %d: %s", deleteRes.Code, deleteRes.Body.String())
	}
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if err := json.Unmarshal(res.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("deleted DOI still listed: %#v", rows)
	}
}
