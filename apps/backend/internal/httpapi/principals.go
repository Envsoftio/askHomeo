package httpapi

import (
	"context"
	"crypto/subtle"
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
	if len(a.Principals) > 0 {
		return a.Principals
	}
	adminName := strings.TrimSpace(os.Getenv("ADMIN_NAME"))
	if adminName == "" {
		adminName = "Local administrator"
	}
	users := []Principal{{ID: legacyAdminID, Name: adminName, Role: "admin", Token: a.Token}}
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
	for _, principal := range a.authPrincipals() {
		if principal.Token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(principal.Token)) == 1 {
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
