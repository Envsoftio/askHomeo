package paperjobs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestDOIAlternateCopyAcquisitionIntegration(t *testing.T) {
	dbURL := os.Getenv("PAPER_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set PAPER_TEST_DATABASE_URL to a fresh disposable database")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	title := "Effects of homeopathic treatment in trial"
	makeHTML := func(doi string) []byte {
		return []byte(`<html><head><meta name="citation_doi" content="` + doi + `"><meta name="citation_title" content="` + title + `"><link rel="license" href="https://creativecommons.org/licenses/by/4.0/"></head><body><article><h1>` + title + `</h1><p>` + strings.Repeat("Methods and results of this original article are presented here. ", 20) + `</p></article></body></html>`)
	}
	pdf := makeArticlePDF(t)
	fixtures := []struct {
		name, doi, format string
		raw []byte
		want string
	}{
		{"html", "10.1234/html.01", "html", makeHTML("10.1234/html.01"), "review"},
		{"pdf", "10.1234/trial.01", "pdf", pdf, "review"},
		{"wrong_pdf", "10.1234/wrong.01", "pdf", pdf, "failed"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			workID, jobID := uuid.New(), uuid.New()
			_, err := store.DB.Exec(ctx, `INSERT INTO scholarly_works(id,doi,title,authors) VALUES($1,$2,$3,'Fixture Researcher')`, workID, fixture.doi, title)
			if err != nil {
				t.Fatal(err)
			}
			candidate := Candidate{Resolver: "unpaywall", Format: fixture.format, URL: "https://journal.example/" + fixture.name, Version: "publishedVersion", License: "cc-by"}
			payload, _ := json.Marshal([]Candidate{candidate})
			_, err = store.DB.Exec(ctx, `INSERT INTO paper_acquisition_jobs(id,work_id,policy_version,candidate_urls_json,effective_limits_json) VALUES($1,$2,$3,$4,'{}')`, jobID, workID, PolicyVersion, payload)
			if err != nil {
				t.Fatal(err)
			}
			worker := &Worker{Store: store, fetchAlternate: func(_ context.Context, got Candidate) ([]byte, string, error) {
				if got.URL != candidate.URL {
					return nil, "", errors.New("wrong candidate fetched")
				}
				return fixture.raw, candidate.URL, nil
			}}
			worked, err := worker.ProcessOne(ctx)
			if err != nil || !worked {
				t.Fatalf("worker: %v %v", worked, err)
			}
			var status, reason, linked string
			if err := store.DB.QueryRow(ctx, `SELECT status,reason,coalesce(source_id::text,'') FROM paper_acquisition_jobs WHERE id=$1`, jobID).Scan(&status, &reason, &linked); err != nil {
				t.Fatal(err)
			}
			if status != fixture.want {
				t.Fatalf("job %s, wanted %s: %s", status, fixture.want, reason)
			}
			if fixture.want == "failed" {
				if linked != "" || !strings.Contains(reason, "DOI and title") {
					t.Fatalf("wrong article imported: %s %s", linked, reason)
				}
				return
			}
			if linked != workID.String() {
				t.Fatalf("source identity differs from work: %s", linked)
			}
			var format, rights string
			if err := store.DB.QueryRow(ctx, `SELECT document_format,rights_status FROM sources WHERE id=$1`, workID).Scan(&format, &rights); err != nil || format != fixture.format || rights == "allowed" {
				t.Fatalf("import format=%s rights=%s err=%v", format, rights, err)
			}
		})
	}
}
