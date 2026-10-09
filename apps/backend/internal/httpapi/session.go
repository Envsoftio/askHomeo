package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

const testingSessionMaxAge = 400 * 24 * 60 * 60

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: "homeopath_session", Value: token, Path: "/api/", MaxAge: testingSessionMaxAge, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (a *API) openSession(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username     string `json:"username"`
		Password     string `json:"password"`
		Token        string `json:"token"`
		RequiredRole string `json:"required_role"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		fail(w, http.StatusBadRequest, "enter valid sign-in details")
		return
	}
	var principal Principal
	var ok bool
	if input.Token != "" && input.Username == "" && input.Password == "" {
		principal, ok = a.authenticate(strings.TrimSpace(input.Token))
		// Administrator browser sign-in always requires username and password.
		ok = ok && principal.Role == "reviewer"
	} else if input.Token == "" && a.adminSessionToken != "" {
		usernameMatches := credentialsEqual(strings.TrimSpace(input.Username), a.adminUsername)
		passwordMatches := credentialsEqual(input.Password, a.adminPassword)
		if usernameMatches && passwordMatches {
			principal, ok = a.adminSessionPrincipal()
		}
	}
	if !ok {
		fail(w, http.StatusUnauthorized, "sign-in details were not recognized")
		return
	}
	if input.RequiredRole != "" && input.RequiredRole != "admin" && input.RequiredRole != "reviewer" {
		fail(w, http.StatusBadRequest, "invalid requested role")
		return
	}
	if input.RequiredRole != "" && principal.Role != input.RequiredRole {
		fail(w, http.StatusForbidden, "This account grants "+principal.Role+" access. Sign in with the administrator username and password to switch to administrator.")
		return
	}
	setSessionCookie(w, principal.Token)
	w.Header().Set("Cache-Control", "no-store")
	write(w, http.StatusOK, map[string]string{"name": principal.Name, "role": principal.Role})
}

func (a *API) currentSession(w http.ResponseWriter, r *http.Request) {
	principal := requestPrincipal(r.Context())
	if principal.Token == "" {
		fail(w, http.StatusUnauthorized, "sign in required")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, http.StatusOK, map[string]string{"name": principal.Name, "role": principal.Role})
}

func (a *API) closeSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "homeopath_session", Value: "", Path: "/api/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.Header().Set("Cache-Control", "no-store")
	write(w, http.StatusOK, map[string]bool{"signed_out": true})
}
