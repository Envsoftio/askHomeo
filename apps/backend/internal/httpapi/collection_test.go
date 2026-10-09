package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectionPreviewRequiresAdminAndExplicitScope(t *testing.T) {
	a := &API{}
	for _, tc := range []struct {
		role   string
		status int
	}{{"reviewer", 403}, {"admin", 400}} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/collections/preview", strings.NewReader(`{"seed_url":"https://books.example/index.html"}`))
		req = req.WithContext(context.WithValue(req.Context(), roleKey{}, tc.role))
		res := httptest.NewRecorder()
		a.previewCollection(res, req)
		if res.Code != tc.status {
			t.Fatalf("role %s got %d, want %d: %s", tc.role, res.Code, tc.status, res.Body.String())
		}
	}
}
