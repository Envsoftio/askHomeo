package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"github.com/google/uuid"
)

var legacyAdminID = uuid.MustParse("00000000-0000-4000-8000-000000000001")
var legacyReviewerID = uuid.MustParse("00000000-0000-4000-8000-000000000002")

type Principal struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Role  string    `json:"role"`
	Token string    `json:"token"`
}

type principalKey struct{}

func requestPrincipal(ctx context.Context) Principal {
	if principal, ok := ctx.Value(principalKey{}).(Principal); ok {
		return principal
	}
	role := requestRole(ctx)
	if role == "reviewer" {
		return Principal{ID: legacyReviewerID, Name: "Legacy reviewer", Role: role}
	}
	return Principal{ID: legacyAdminID, Name: "Legacy administrator", Role: "admin"}
}

func (a *API) authPrincipals() []Principal {
	adminName := strings.TrimSpace(os.Getenv("ADMIN_NAME"))
	if adminName == "" {
		adminName = "Local administrator"
	}
	token := a.Token
	if token == "" {
		token = a.adminSessionToken
	}
	admin := Principal{ID: legacyAdminID, Name: adminName, Role: "admin", Token: token}
	if len(a.Principals) > 0 {
		users := append([]Principal(nil), a.Principals...)
		if a.adminSessionToken != "" {
			for _, principal := range users {
				if principal.ID == legacyAdminID {
					return users
				}
			}
			admin.Token = a.adminSessionToken
			users = append(users, admin)
		}
		return users
	}
	users := []Principal{admin}
	if a.ReviewerToken != "" {
		reviewerName := strings.TrimSpace(os.Getenv("REVIEWER_NAME"))
		if reviewerName == "" {
			reviewerName = "Local reviewer"
		}
		users = append(users, Principal{ID: legacyReviewerID, Name: reviewerName, Role: "reviewer", Token: a.ReviewerToken})
	}
	return users
}

func (a *API) authenticate(token string) (Principal, bool) {
	if a.adminSessionToken != "" && credentialsEqual(token, a.adminSessionToken) {
		return a.adminSessionPrincipal()
	}
	for _, principal := range a.authPrincipals() {
		if principal.Token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(principal.Token)) == 1 {
			return principal, true
		}
	}
	return Principal{}, false
}

// ConfigureAdminLogin is called once at startup, before serving requests.
// Browser sessions use a random secret, never the configured password.
func (a *API) ConfigureAdminLogin(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || strings.TrimSpace(password) == "" {
		return errors.New("ADMIN_USERNAME and ADMIN_PASSWORD are required")
	}
	if a.Token != "" && len(a.Token) < 24 {
		return errors.New("API_TOKEN must have at least 24 characters when set")
	}
	for _, principal := range a.Principals {
		if principal.ID == legacyAdminID && principal.Role != "admin" {
			return errors.New("the default administrator ID is reserved for an admin principal")
		}
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	a.adminUsername, a.adminPassword = username, password
	a.adminSessionToken = hex.EncodeToString(secret)
	return nil
}

func credentialsEqual(provided, expected string) bool {
	providedHash, expectedHash := sha256.Sum256([]byte(provided)), sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}

func (a *API) adminSessionPrincipal() (Principal, bool) {
	for _, principal := range a.authPrincipals() {
		if principal.ID == legacyAdminID && principal.Role == "admin" {
			principal.Token = a.adminSessionToken
			return principal, true
		}
	}
	return Principal{}, false
}

func (a *API) SyncPrincipals(ctx context.Context) error {
	seenIDs := map[uuid.UUID]bool{}
	seenTokens := map[string]bool{}
	for _, principal := range a.authPrincipals() {
		if principal.ID == uuid.Nil || len(strings.TrimSpace(principal.Name)) < 2 || len(principal.Name) > 120 || (principal.Role != "admin" && principal.Role != "reviewer") || len(principal.Token) < 24 || seenIDs[principal.ID] || seenTokens[principal.Token] {
			return errors.New("invalid or duplicate configured principal")
		}
		seenIDs[principal.ID] = true
		seenTokens[principal.Token] = true
		_, err := a.Store.DB.Exec(ctx, `INSERT INTO app_principals(id,display_name,role) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET display_name=EXCLUDED.display_name WHERE app_principals.role=EXCLUDED.role`, principal.ID, strings.TrimSpace(principal.Name), principal.Role)
		if err != nil {
			return err
		}
	}
	return nil
}
