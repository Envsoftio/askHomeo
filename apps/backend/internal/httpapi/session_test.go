package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdminPasswordSession(t *testing.T) {
	for _, named := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "named reviewers"}[named], func(t *testing.T) {
			api := &API{}
			if named {
				api.Token = "unused-default-admin-token-123456"
				api.Principals = []Principal{{ID: uuid.New(), Name: "Reviewer", Role: "reviewer", Token: "named-reviewer-secret-token"}}
			}
			if err := api.ConfigureAdminLogin("admin@example.com", "test-password"); err != nil {
				t.Fatal(err)
			}
			if named {
				if _, ok := api.authenticate(api.Token); ok {
					t.Fatal("named users must continue to replace the default bearer token")
				}
			}
			h := api.Handler()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"username":"admin@example.com","password":"test-password"}`)))
			cookies := w.Result().Cookies()
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"role":"admin"`) || len(cookies) != 1 {
				t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
			}
			cookie := cookies[0]
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Value == "test-password" || len(cookie.Value) < 32 {
				t.Fatal("expected an opaque HttpOnly session cookie")
			}
			if _, ok := api.authenticate(cookie.Value); ok {
				t.Fatal("browser cookie must not be a bearer token")
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
			req.AddCookie(cookie)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusOK || len(w.Result().Cookies()) != 0 {
				t.Fatal("administrator session should remain valid without extending its expiry")
			}
			req = httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
			req.AddCookie(cookie)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusOK || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge >= 0 {
				t.Fatal("sign out must clear the administrator cookie")
			}
			req = httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
			req.AddCookie(cookie)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatal("signed-out browser token must be revoked")
			}
			if err := api.ConfigureAdminLogin("admin@example.com", "new-password"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAdminLoginRejectsInvalidCredentialsAndTokens(t *testing.T) {
	api := &API{Token: "admin-local-demo-token-123456"}
	if err := api.ConfigureAdminLogin("admin@example.com", "test-password"); err != nil {
		t.Fatal(err)
	}
	h := api.Handler()
	for _, body := range []map[string]string{
		{},
		{"username": "admin@example.com"},
		{"password": "test-password"},
		{"username": "wrong@example.com", "password": "test-password"},
		{"username": "admin@example.com", "password": "wrong-password"},
		{"username": "admin@example.com", "password": "test-password "},
		{"token": api.Token},
		{"token": api.adminSessionToken},
		{"username": "admin@example.com", "password": "test-password", "token": api.Token},
	} {
		payload, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewReader(payload)))
		if w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
			t.Fatalf("invalid credentials accepted: status=%d", w.Code)
		}
	}
	// Existing automation can still authenticate using an explicitly configured bearer token.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.Header.Set("Authorization", "Bearer "+api.Token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bearer API access status=%d", w.Code)
	}
}

func TestAdminLoginConfiguration(t *testing.T) {
	for _, credentials := range [][2]string{{"", ""}, {"admin", ""}, {"", "password"}, {"admin", "   "}} {
		if err := (&API{}).ConfigureAdminLogin(credentials[0], credentials[1]); err == nil {
			t.Fatal("missing administrator credentials must fail startup")
		}
	}
	if err := (&API{Token: "short"}).ConfigureAdminLogin("admin", "password"); err == nil {
		t.Fatal("short optional API token must fail startup")
	}
	api := &API{Principals: []Principal{{ID: legacyAdminID, Role: "reviewer"}}}
	if err := api.ConfigureAdminLogin("admin", "password"); err == nil {
		t.Fatal("reviewer cannot use the reserved administrator identity")
	}
}

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
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].MaxAge != browserSessionMaxAge || cookies[0].Value == principal.Token {
		t.Fatalf("unexpected session cookie: %+v", cookies)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"reviewer"`) {
		t.Fatalf("session status=%d body=%s", w.Code, w.Body.String())
	}
	if refreshed := w.Result().Cookies(); len(refreshed) != 0 {
		t.Fatalf("session lifetime must not be extended by an ordinary request: %+v", refreshed)
	}
	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	logout.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, logout)
	if cleared := w.Result().Cookies(); w.Code != http.StatusOK || len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("session cookie was not cleared: status=%d cookies=%+v", w.Code, cleared)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked reviewer session status=%d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/session", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned session status=%d", w.Code)
	}
}

func TestAdministratorSwitchRejectsReviewerToken(t *testing.T) {
	api := &API{Token: "admin-local-demo-token-123456", ReviewerToken: "reviewer-local-demo-token-123456"}
	if err := api.ConfigureAdminLogin("admin@example.com", "test-password"); err != nil {
		t.Fatal(err)
	}
	h := api.Handler()
	wrong := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewBufferString(`{"token":"reviewer-local-demo-token-123456","required_role":"admin"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, wrong)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "reviewer access") || len(w.Result().Cookies()) != 0 {
		t.Fatalf("reviewer switch status=%d body=%s cookies=%v", w.Code, w.Body.String(), w.Result().Cookies())
	}
	correct := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewBufferString(`{"username":"admin@example.com","password":"test-password","required_role":"admin"}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, correct)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"role":"admin"`) || len(w.Result().Cookies()) != 1 {
		t.Fatalf("admin switch status=%d body=%s cookies=%v", w.Code, w.Body.String(), w.Result().Cookies())
	}
}

func TestProductionBrowserSessionRequiresOriginAndSecureCookie(t *testing.T) {
	api := &API{SecureCookies: true, PublicOrigin: "https://research.example.org"}
	if err := api.ConfigureAdminLogin("admin@example.com", "test-password"); err != nil {
		t.Fatal(err)
	}
	h := api.Handler()
	login := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"username":"admin@example.com","password":"test-password"}`))
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := login(""); w.Code != http.StatusForbidden {
		t.Fatalf("missing origin status=%d", w.Code)
	}
	if w := login("https://other.example.org"); w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin status=%d", w.Code)
	}
	w := login(api.PublicOrigin)
	if w.Code != http.StatusOK || len(w.Result().Cookies()) != 1 || !w.Result().Cookies()[0].Secure {
		t.Fatalf("same-origin login status=%d cookies=%v", w.Code, w.Result().Cookies())
	}
	cookie := w.Result().Cookies()[0]
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	req.AddCookie(cookie)
	req.Header.Set("Origin", "https://other.example.org")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin logout status=%d", w.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rejected logout must not revoke session: status=%d", w.Code)
	}
}

func TestExpiredBrowserSessionIsRejected(t *testing.T) {
	api := &API{ReviewerToken: "reviewer-local-demo-token-123456"}
	h := api.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"token":"reviewer-local-demo-token-123456"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("login status=%d", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	api.browserSessions.Lock()
	row := api.browserSessions.rows[hashBrowserToken(cookie.Value)]
	row.expiresAt = time.Now().Add(-time.Second)
	api.browserSessions.rows[hashBrowserToken(cookie.Value)] = row
	api.browserSessions.Unlock()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expired session status=%d", w.Code)
	}
}
