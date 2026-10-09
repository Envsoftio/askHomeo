package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const browserSessionLifetime = 12 * time.Hour

type memoryBrowserSession struct {
	principalID uuid.UUID
	expiresAt   time.Time
}

// The in-memory path supports API handlers in isolated tests. The server always
// supplies a Store, so deployed sessions are kept in PostgreSQL.
type browserSessionMemory struct {
	sync.Mutex
	rows map[string]memoryBrowserSession
}

func hashBrowserToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (a *API) createBrowserSession(ctx context.Context, principalID uuid.UUID) (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	token := hex.EncodeToString(secret)
	expiresAt := time.Now().Add(browserSessionLifetime)
	if a.Store != nil && a.Store.DB != nil {
		_, err := a.Store.DB.Exec(ctx, `INSERT INTO browser_sessions(token_hash,principal_id,expires_at) VALUES($1,$2,$3)`, hashBrowserToken(token), principalID, expiresAt)
		return token, err
	}
	a.browserSessions.Lock()
	defer a.browserSessions.Unlock()
	if a.browserSessions.rows == nil {
		a.browserSessions.rows = make(map[string]memoryBrowserSession)
	}
	a.browserSessions.rows[hashBrowserToken(token)] = memoryBrowserSession{principalID: principalID, expiresAt: expiresAt}
	return token, nil
}

func (a *API) browserSessionPrincipal(ctx context.Context, token string) (Principal, bool, error) {
	if len(token) != 64 {
		return Principal{}, false, nil
	}
	var id uuid.UUID
	if a.Store != nil && a.Store.DB != nil {
		err := a.Store.DB.QueryRow(ctx, `SELECT principal_id FROM browser_sessions WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>now()`, hashBrowserToken(token)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, false, nil
		}
		if err != nil {
			return Principal{}, false, err
		}
	} else {
		a.browserSessions.Lock()
		row, ok := a.browserSessions.rows[hashBrowserToken(token)]
		a.browserSessions.Unlock()
		if !ok || !time.Now().Before(row.expiresAt) {
			return Principal{}, false, nil
		}
		id = row.principalID
	}
	for _, principal := range a.authPrincipals() {
		if principal.ID == id {
			principal.Token = ""
			return principal, true, nil
		}
	}
	return Principal{}, false, nil
}

func (a *API) revokeBrowserSession(ctx context.Context, token string) error {
	if len(token) != 64 {
		return nil
	}
	if a.Store != nil && a.Store.DB != nil {
		_, err := a.Store.DB.Exec(ctx, `UPDATE browser_sessions SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL`, hashBrowserToken(token))
		return err
	}
	a.browserSessions.Lock()
	delete(a.browserSessions.rows, hashBrowserToken(token))
	a.browserSessions.Unlock()
	return nil
}
