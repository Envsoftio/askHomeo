package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBrowserSessionUsesNamedPrincipal(t *testing.T) {
	principal := Principal{ID: uuid.New(), Name: "Reviewer Alpha", Role: "reviewer", Token: "reviewer-alpha-local-demo-token"}
	h := (&API{Principals: []Principal{principal}}).Handler()
	login := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewBufferString(`{"token":"`+principal.Token+`"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, login)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"Reviewer Alpha"`) {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].MaxAge != testingSessionMaxAge {
		t.Fatalf("unexpected session cookie: %+v", cookies)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"reviewer"`) {
		t.Fatalf("session status=%d body=%s", w.Code, w.Body.String())
	}
	if refreshed := w.Result().Cookies(); len(refreshed) != 1 || refreshed[0].MaxAge != testingSessionMaxAge {
		t.Fatalf("session cookie was not renewed: %+v", refreshed)
	}
	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	logout.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, logout)
	if cleared := w.Result().Cookies(); w.Code != http.StatusOK || len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("session cookie was not cleared: status=%d cookies=%+v", w.Code, cleared)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/session", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned session status=%d", w.Code)
	}
}

func TestAdministratorSwitchRejectsReviewerToken(t *testing.T) {
	api := &API{Token: "admin-local-demo-token-123456", ReviewerToken: "reviewer-local-demo-token-123456"}
	h := api.Handler()
	wrong := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewBufferString(`{"token":"reviewer-local-demo-token-123456","required_role":"admin"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, wrong)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "reviewer access") || len(w.Result().Cookies()) != 0 {
		t.Fatalf("reviewer switch status=%d body=%s cookies=%v", w.Code, w.Body.String(), w.Result().Cookies())
	}
	correct := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewBufferString(`{"token":"admin-local-demo-token-123456","required_role":"admin"}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, correct)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"role":"admin"`) || len(w.Result().Cookies()) != 1 {
		t.Fatalf("admin switch status=%d body=%s cookies=%v", w.Code, w.Body.String(), w.Result().Cookies())
	}
}
