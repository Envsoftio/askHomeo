package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func (a *API) disableSource(w http.ResponseWriter, r *http.Request) { a.setSourceAccess(w, r, false) }
func (a *API) enableSource(w http.ResponseWriter, r *http.Request)  { a.setSourceAccess(w, r, true) }

func (a *API) setSourceAccess(w http.ResponseWriter, r *http.Request, enable bool) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
		fail(w, 400, "A reason is required.")
		return
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if len(body.Reason) < 8 || len(body.Reason) > 1000 {
		fail(w, 400, "Give a reason of 8 to 1000 characters.")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Could not change source access.")
		return
	}
	defer tx.Rollback(r.Context())
	var status, rights string
	var superseded bool
	err = tx.QueryRow(r.Context(), `SELECT status,rights_status,superseded_at IS NOT NULL FROM sources WHERE id=$1 FOR UPDATE`, id).Scan(&status, &rights, &superseded)
	if err != nil {
		fail(w, 404, "Source not found.")
		return
	}
	action, next := "disable", "disabled"
	if enable {
		action, next = "enable", "published"
		if status != "disabled" || rights != "allowed" || superseded {
			fail(w, 409, "Only an allowed, current disabled source can be restored.")
			return
		}
		var prior string
		err = tx.QueryRow(r.Context(), `SELECT previous_status FROM source_access_decisions WHERE source_id=$1 AND action='disable' ORDER BY created_at DESC LIMIT 1`, id).Scan(&prior)
		if err != nil || prior != "published" {
			fail(w, 409, "A prior published state was not recorded for this source.")
			return
		}
	} else if status != "published" {
		fail(w, 409, "Only a published source can be disabled.")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE sources SET status=$2,updated_at=now() WHERE id=$1`, id, next)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO source_access_decisions(id,source_id,action,previous_status,reason,actor_role,actor_principal_id) VALUES($1,$2,$3,$4,$5,'admin',$6)`, uuid.New(), id, action, status, body.Reason, requestPrincipal(r.Context()).ID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "Could not change source access.")
		return
	}
	write(w, 200, map[string]string{"status": next})
}
