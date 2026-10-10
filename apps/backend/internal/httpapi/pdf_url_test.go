package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImportPDFURLRejectsUnsafeLinks(t *testing.T) {
	api := &API{Token: "test-admin"}
	for _, raw := range []string{
		`{"pdf_url":"https://127.0.0.1/book.pdf","title":"Book","author":"Author"}`,
		`{"pdf_url":"https://user:pass@example.org/book.pdf","title":"Book","author":"Author"}`,
		`{"pdf_url":"http://example.org:8080/book.pdf","title":"Book","author":"Author"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/import-url", strings.NewReader(raw))
		req.Header.Set("Authorization", "Bearer test-admin")
		res := httptest.NewRecorder()
		api.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusBadGateway {
			t.Fatalf("unsafe link: HTTP %d: %s", res.Code, res.Body.String())
		}
	}
}

func TestPDFURLCategoryRejectedBeforeFetch(t *testing.T) {
	api := &API{Token: "test-admin"}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/import-url", strings.NewReader(`{"pdf_url":"https://example.org/book.pdf","literature_categories":["invented"]}`))
	req.Header.Set("Authorization", "Bearer test-admin")
	res := httptest.NewRecorder()
	api.Handler().ServeHTTP(res, req)
	if res.Code != 400 {
		t.Fatalf("invalid category: %d %s", res.Code, res.Body.String())
	}
}
