package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

func (a *API) reviewStudyLocation(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var b struct {
		Country string `json:"study_country"`
		BlockID string `json:"evidence_block_id"`
		Reason  string `json:"reason"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil {
		fail(w, 400, "invalid study-location review")
		return
	}
	b.Country = strings.TrimSpace(b.Country)
	b.Reason = strings.TrimSpace(b.Reason)
	if b.Country != "unknown" && b.Country != "multinational" && !countryCode.MatchString(b.Country) {
		fail(w, 400, "use an ISO country code, multinational or unknown")
		return
	}
	if len(b.Reason) < 10 {
		fail(w, 400, "record the study-location review reason")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var sourceID *uuid.UUID
	err = tx.QueryRow(r.Context(), `SELECT source_id FROM scholarly_works WHERE id=$1 FOR UPDATE`, id).Scan(&sourceID)
	if err != nil {
		fail(w, 404, "paper reference unavailable")
		return
	}
	var evidence any
	if b.Country != "unknown" {
		if sourceID == nil {
			fail(w, 409, "study location needs a saved full-text source or registry record")
			return
		}
		blockID, parseErr := uuid.Parse(b.BlockID)
		if parseErr != nil {
			fail(w, 400, "select an exact saved methods or registry block")
			return
		}
		var kind, status string
		err = tx.QueryRow(r.Context(), `SELECT b.kind,b.review_status FROM document_blocks b JOIN sources s ON s.id=b.source_id WHERE b.id=$1 AND b.source_id=$2 AND b.processing_revision_id=s.current_revision_id AND s.removed_at IS NULL`, blockID, *sourceID).Scan(&kind, &status)
		if err != nil || status != "accepted" && status != "corrected" || kind == "reference" {
			fail(w, 409, "evidence must be a reviewed block in the current saved source revision")
			return
		}
		evidence = blockID
	}
	_, err = tx.Exec(r.Context(), `UPDATE scholarly_works SET study_country=$2,study_country_review_status='reviewed',study_country_evidence_block_id=$3,study_country_reviewed_by=$4,study_country_reviewed_at=now(),study_country_reason=$5,updated_at=now() WHERE id=$1`, id, b.Country, evidence, requestPrincipal(r.Context()).ID, b.Reason)
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "could not save study-location review")
		return
	}
	write(w, 200, map[string]any{"study_country": b.Country, "review_status": "reviewed", "evidence_block_id": evidence})
}
