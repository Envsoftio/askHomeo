package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/httpapi"
)

func TestManualPDFIngestionIntegration(t *testing.T) {
	dbURL := os.Getenv("INTAKE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set INTAKE_TEST_DATABASE_URL for isolated database integration test")
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
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ps := filepath.Join(dir, "book.ps")
	pdf := filepath.Join(dir, "book.pdf")
	content := "%!PS-Adobe-3.0\n/Helvetica findfont 14 scalefont setfont\n72 720 moveto (Aconite is described with sudden fear and restlessness in this historical source.) show\n72 690 moveto (The same page discusses fever and its accompanying symptoms in detail.) show\nshowpage\n"
	if err := os.WriteFile(ps, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("gs", "-q", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pdfwrite", "-sOutputFile="+pdf, ps).CombinedOutput(); err != nil {
		t.Fatalf("make PDF: %v: %s", err, out)
	}
	data, err := os.ReadFile(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "book.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"title": "Test book", "author": "Test author", "edition": "First", "publication_info": "1901", "repository": "Test collection", "rights_statement": "Review required"} {
		if err := form.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	api := (&httpapi.API{Store: store, Token: "integration-test-admin-token-12345"}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/upload", &body)
	req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
	req.Header.Set("Content-Type", form.FormDataContentType())
	res := httptest.NewRecorder()
	api.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("upload HTTP %d: %s", res.Code, res.Body.String())
	}
	var upload struct {
		SourceID uuid.UUID `json:"source_id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	id := upload.SourceID
	if err := New(store).once(ctx); err != nil {
		t.Fatal(err)
	}
	var status, raw, image string
	var chunks int
	err = store.DB.QueryRow(ctx, `SELECT s.status,p.text_raw,p.image_url,(SELECT count(*) FROM chunks WHERE page_id=p.id) FROM sources s JOIN pages p ON p.source_id=s.id WHERE s.id=$1 AND p.pdf_page_index=0`, id).Scan(&status, &raw, &image, &chunks)
	if err != nil {
		t.Fatal(err)
	}
	if status != "review" || chunks < 1 || image != "/api/v1/sources/"+id.String()+"/pages/0/image" || len(raw) < 80 {
		t.Fatalf("status=%s chunks=%d image=%s raw=%q", status, chunks, image, raw)
	}
	var jobID uuid.UUID
	if err := store.DB.QueryRow(ctx, `SELECT id FROM jobs WHERE source_id=$1 AND kind='ingest' AND status='done'`, id).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	imageReq := httptest.NewRequest(http.MethodGet, image, nil)
	imageReq.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
	imageRes := httptest.NewRecorder()
	api.ServeHTTP(imageRes, imageReq)
	if imageRes.Code != http.StatusOK || imageRes.Header().Get("Content-Type") != "image/png" || imageRes.Body.Len() < 100 {
		t.Fatalf("scan image HTTP %d (%d bytes): %s", imageRes.Code, imageRes.Body.Len(), imageRes.Body.String())
	}
}

func TestLiveDOIImportIntegration(t *testing.T) {
	if os.Getenv("LIVE_DOI_TEST") != "1" {
		t.Skip("set LIVE_DOI_TEST=1 for Crossref and publisher PDF integration")
	}
	dbURL := os.Getenv("INTAKE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set INTAKE_TEST_DATABASE_URL to an isolated database")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	handler := (&httpapi.API{Store: store, Token: "integration-test-admin-token-12345"}).Handler()
	request := func(path string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer integration-test-admin-token-12345")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	added := request("/api/v1/doi-references", `{"doi":"10.1371/journal.pone.0134657"}`)
	if added.Code != 200 {
		t.Fatalf("add DOI %d: %s", added.Code, added.Body.String())
	}
	var reference struct {
		ID           string `json:"id"`
		PDFAvailable bool   `json:"pdf_available"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &reference); err != nil {
		t.Fatal(err)
	}
	if !reference.PDFAvailable {
		t.Fatal("PLOS ONE publisher PDF route not offered")
	}
	imported := request("/api/v1/doi-references/"+reference.ID+"/import", "")
	if imported.Code != 202 {
		t.Fatalf("import DOI %d: %s", imported.Code, imported.Body.String())
	}
	var result struct {
		SourceID uuid.UUID `json:"source_id"`
	}
	if err := json.Unmarshal(imported.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if err := New(store).once(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	var count int
	if err := store.DB.QueryRow(ctx, `SELECT status,(SELECT count(*) FROM pages WHERE source_id=s.id) FROM sources s WHERE id=$1`, result.SourceID).Scan(&status, &count); err != nil {
		t.Fatal(err)
	}
	if status != "review" || count < 2 {
		t.Fatalf("DOI source status=%s pages=%d", status, count)
	}
}
