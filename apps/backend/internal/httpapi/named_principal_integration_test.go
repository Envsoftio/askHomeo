package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestNamedReviewersHaveSeparateActivityIntegration(t *testing.T) {
	url := os.Getenv("PROVENANCE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PROVENANCE_TEST_DATABASE_URL to a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := core.Open(ctx, url, "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	first := Principal{ID: uuid.New(), Name: "Reviewer Alpha", Role: "reviewer", Token: "integration-reviewer-alpha-token-12345"}
	second := Principal{ID: uuid.New(), Name: "Reviewer Beta", Role: "reviewer", Token: "integration-reviewer-beta-token-12345"}
	a := &API{Store: store, Principals: []Principal{first, second}}
	if err := a.SyncPrincipals(ctx); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	call := func(user Principal, method, path string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+user.Token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	res := call(first, http.MethodPost, "/api/v1/research/answer-jobs", map[string]any{"question": "Who is Nash?", "mode": "quick"})
	if res.Code != http.StatusAccepted {
		t.Fatalf("queue HTTP %d: %s", res.Code, res.Body.String())
	}
	var queued struct {
		JobID uuid.UUID `json:"job_id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &queued); err != nil {
		t.Fatal(err)
	}
	if res = call(second, http.MethodGet, "/api/v1/research/answer-jobs/"+queued.JobID.String(), nil); res.Code != http.StatusNotFound {
		t.Fatalf("second reviewer read first reviewer's job: HTTP %d", res.Code)
	}
	res = call(second, http.MethodGet, "/api/v1/activity", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("activity HTTP %d", res.Code)
	}
	var activity struct {
		Jobs []struct {
			ID uuid.UUID `json:"id"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &activity); err != nil {
		t.Fatal(err)
	}
	for _, job := range activity.Jobs {
		if job.ID == queued.JobID {
			t.Fatal("second reviewer saw first reviewer's activity")
		}
	}
	res = call(second, http.MethodPost, "/api/v1/activity/"+queued.JobID.String()+"/read", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("mark-read HTTP %d", res.Code)
	}
	var count int
	if err := store.DB.QueryRow(ctx, `SELECT count(*) FROM activity_event_reads ar JOIN activity_events e ON e.id=ar.event_id WHERE e.answer_job_id=$1 AND ar.actor_principal_id=$2`, queued.JobID, second.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("second reviewer marked first reviewer's event read: %d, %v", count, err)
	}
}

func TestNamedReviewerCanApproveRightsButCannotPublishIntegration(t *testing.T) {
	url := os.Getenv("PROVENANCE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PROVENANCE_TEST_DATABASE_URL to a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := core.Open(ctx, url, "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	var sourceID uuid.UUID
	if err = store.DB.QueryRow(ctx, `SELECT id FROM sources WHERE status='review' LIMIT 1`).Scan(&sourceID); err != nil {
		t.Skipf("disposable database has no review source: %v", err)
	}
	person := Principal{ID: uuid.New(), Name: "Rights reviewer", Role: "reviewer", Token: "integration-rights-reviewer-token-12345"}
	a := &API{Store: store, Principals: []Principal{person}}
	if err = a.SyncPrincipals(ctx); err != nil {
		t.Fatal(err)
	}
	h := a.Handler()
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+person.Token)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	res := post("/api/v1/sources/"+sourceID.String()+"/rights", `{"decision":"allowed","note":"Reviewed source record and permission for local use."}`)
	if res.Code != http.StatusOK {
		t.Fatalf("reviewer rights HTTP %d: %s", res.Code, res.Body.String())
	}
	var recorded uuid.UUID
	var recordedRole string
	if err = store.DB.QueryRow(ctx, `SELECT reviewer_principal_id,reviewer_role FROM rights_decisions WHERE id=(SELECT rights_decision_id FROM sources WHERE id=$1)`, sourceID).Scan(&recorded, &recordedRole); err != nil || recorded != person.ID || recordedRole != "reviewer" {
		t.Fatalf("rights reviewer %s role %s, error %v", recorded, recordedRole, err)
	}
	res = post("/api/v1/sources/"+sourceID.String()+"/publish", `{}`)
	if res.Code != http.StatusForbidden {
		t.Fatalf("reviewer published source: HTTP %d", res.Code)
	}
}
