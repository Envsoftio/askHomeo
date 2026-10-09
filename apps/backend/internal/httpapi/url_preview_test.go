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
