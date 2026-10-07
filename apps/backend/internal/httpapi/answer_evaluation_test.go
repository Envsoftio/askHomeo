package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestEvaluationReportRequiresAdmin(t *testing.T) {
	api := (&API{Token: "integration-test-admin-token-12345", ReviewerToken: "integration-test-reviewer-token-12345"}).Handler()
	path := "/api/v1/admin/answer-jobs/" + uuid.NewString() + "/evaluation"
	for _, test := range []struct {
		name, token string
		status      int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"reviewer", "integration-test-reviewer-token-12345", http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("HTTP %d, want %d", response.Code, test.status)
			}
		})
	}
}
