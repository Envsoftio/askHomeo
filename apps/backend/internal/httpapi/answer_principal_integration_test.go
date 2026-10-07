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

func TestReviewerCannotReadAdminQuestionActivityIntegration(t *testing.T) {
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
	api := (&API{Store: store, Token: "integration-test-admin-token-12345", ReviewerToken: "integration-test-reviewer-token-12345"}).Handler()
	call := func(role, method, path string, body any) *httptest.ResponseRecorder {
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer integration-test-"+role+"-token-12345")
		res := httptest.NewRecorder()
		api.ServeHTTP(res, req)
		return res
	}
	queue := func(role string) uuid.UUID {
		res := call(role, http.MethodPost, "/api/v1/research/answer-jobs", map[string]any{"question": "Who is Nash?", "mode": "quick"})
		if res.Code != http.StatusAccepted {
			t.Fatalf("%s queue HTTP %d: %s", role, res.Code, res.Body.String())
		}
		var body struct {
			JobID uuid.UUID `json:"job_id"`
		}
		if err = json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.JobID
	}
	adminID := queue("admin")
	reviewerID := queue("reviewer")
	var selectedCount int
	if err = store.DB.QueryRow(ctx, `SELECT cardinality(source_ids) FROM answer_jobs WHERE id=$1`, adminID).Scan(&selectedCount); err != nil || selectedCount != 0 {
		t.Fatalf("optional source filter was not saved as empty: count=%d err=%v", selectedCount, err)
	}
	if res := call("reviewer", http.MethodGet, "/api/v1/research/answer-jobs/"+adminID.String(), nil); res.Code != http.StatusNotFound {
		t.Fatalf("reviewer read admin job HTTP %d", res.Code)
	}
	if res := call("reviewer", http.MethodPost, "/api/v1/activity/"+adminID.String()+"/read", nil); res.Code != http.StatusOK {
		t.Fatalf("reviewer mark-read HTTP %d", res.Code)
	}
	var foreignReads int
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM activity_event_reads ar JOIN activity_events e ON e.id=ar.event_id WHERE e.answer_job_id=$1 AND ar.actor_role='reviewer'`, adminID).Scan(&foreignReads); err != nil || foreignReads != 0 {
		t.Fatalf("reviewer marked admin activity read: count=%d err=%v", foreignReads, err)
	}
	res := call("reviewer", http.MethodGet, "/api/v1/activity", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("activity HTTP %d", res.Code)
	}
	var activity struct {
		Jobs []struct {
			ID uuid.UUID `json:"id"`
		} `json:"jobs"`
	}
	if err = json.Unmarshal(res.Body.Bytes(), &activity); err != nil {
		t.Fatal(err)
	}
	foundReviewer := false
	for _, job := range activity.Jobs {
		if job.ID == adminID {
			t.Fatal("reviewer saw admin question")
		}
		if job.ID == reviewerID {
			foundReviewer = true
		}
	}
	if !foundReviewer {
		t.Fatal("reviewer question missing from own activity")
	}
	var adminAnswer, adminCitation uuid.UUID
	err = store.DB.QueryRow(ctx, `SELECT a.id,ac.id FROM answers a JOIN answer_citations ac ON ac.answer_id=a.id WHERE a.owner_role='admin' LIMIT 1`).Scan(&adminAnswer, &adminCitation)
	if err == nil {
		if res = call("reviewer", http.MethodGet, "/api/v1/research/answers/"+adminAnswer.String(), nil); res.Code != http.StatusNotFound {
			t.Fatalf("reviewer read admin answer HTTP %d", res.Code)
		}
		if res = call("reviewer", http.MethodGet, "/api/v1/citations/"+adminCitation.String(), nil); res.Code != http.StatusNotFound {
			t.Fatalf("reviewer read admin citation HTTP %d", res.Code)
		}
	}
}
