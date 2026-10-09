package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreviewURLRequiresAdminAndDoesNotNeedStore(t *testing.T) {
	api := &API{Token: "test-admin"}
	request := func(token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/preview-url", strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		api.Handler().ServeHTTP(res, req)
		return res
	}
	if got := request("", `{"url":"http://example.org"}`); got.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous preview: %d", got.Code)
	}
	if got := request("test-admin", `{"url":"http://127.0.0.1/private"}`); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blocked preview: %d: %s", got.Code, got.Body.String())
	}
	if got := request("test-admin", `{"url":"https://user:pass@example.org/"}`); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("credential URL: %d: %s", got.Code, got.Body.String())
	}
}

func TestPreviewExtractReportsBlocksAndEncodingFailure(t *testing.T) {
	readiness, sample, count, charset, warnings := previewExtract([]byte(`<html><body><nav>Skip</nav><h1>Chapter</h1><p>Useful passage.</p><script>bad()</script></body></html>`), "text/html", "")
	if count != 2 || charset != "utf-8" || !strings.Contains(sample, "Useful passage") || strings.Contains(sample, "Skip") || len(warnings) == 0 || !strings.Contains(readiness, "review") {
		t.Fatalf("preview: %q %q %d %q %v", readiness, sample, count, charset, warnings)
	}
	readiness, _, count, _, _ = previewExtract([]byte{'C', 'a', 'f', 0xe9}, "text/plain", "")
	if count != 0 || !strings.Contains(readiness, "override") {
		t.Fatalf("encoding failure: %q %d", readiness, count)
	}
}
