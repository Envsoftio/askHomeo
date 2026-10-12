package paperjobs

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/scholarly"
)

type noNetworkTransport struct{ Called *bool }

func (n noNetworkTransport) RoundTrip(*http.Request) (*http.Response, error) {
	*n.Called = true
	return nil, context.DeadlineExceeded
}

func TestCheckpointLeaseRecoveryIntegration(t *testing.T) {
	dbURL := os.Getenv("PAPER_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set PAPER_TEST_DATABASE_URL to a disposable database")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`<article><front><article-meta><article-id pub-id-type="pmcid">PMC99887766</article-id><article-id pub-id-type="pmid">99887766</article-id><article-id pub-id-type="doi">10.1234/checkpoint</article-id><permissions><license><ext-link href="https://creativecommons.org/licenses/by/4.0/">CC BY</ext-link></license></permissions><title-group><article-title>Checkpoint trial</article-title></title-group></article-meta></front><body><sec><title>Results</title><p>Saved result after worker restart.</p></sec></body></article>`)
	called := false
	worker := &Worker{Store: store, Client: &scholarly.Client{HTTP: &http.Client{Transport: noNetworkTransport{Called: &called}}}}
	sha, path, err := worker.saveCheckpoint(raw)
	if err != nil {
		t.Fatal(err)
	}
	workID, jobID := uuid.New(), uuid.New()
	_, err = store.DB.Exec(ctx, `INSERT INTO scholarly_works(id,doi,pmid,pmcid,title,authors) VALUES($1,'10.1234/checkpoint','99887766','PMC99887766','Checkpoint trial','Fixture Author')`, workID)
	if err != nil {
		t.Fatal(err)
	}
	candidates, _ := json.Marshal([]string{"https://www.ebi.ac.uk/europepmc/webservices/rest/PMC99887766/fullTextXML"})
	limits, _ := json.Marshal(map[string]int{"response_bytes": 10 << 20})
	_, err = store.DB.Exec(ctx, `INSERT INTO paper_acquisition_jobs(id,work_id,status,stage,policy_version,candidate_urls_json,effective_limits_json,checkpoint_sha256,checkpoint_locator,checkpoint_source_url,checkpoint_resolver,lease_until,lease_token) VALUES($1,$2,'running','validated',$3,$4,$5,$6,$7,$8,'europepmc',now()-interval '1 minute',$9)`, jobID, workID, PolicyVersion, candidates, limits, sha, path, "https://www.ebi.ac.uk/europepmc/webservices/rest/PMC99887766/fullTextXML", uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	worked, err := worker.ProcessOne(ctx)
	if err != nil || !worked {
		t.Fatalf("recovery %v %v", worked, err)
	}
	if called {
		t.Fatal("checkpoint recovery contacted provider")
	}
	var status, source string
	err = store.DB.QueryRow(ctx, `SELECT status,coalesce(source_id::text,'') FROM paper_acquisition_jobs WHERE id=$1`, jobID).Scan(&status, &source)
	if err != nil || status != "review" || source != workID.String() {
		t.Fatalf("job %s source %s %v", status, source, err)
	}
	var count int
	_ = store.DB.QueryRow(ctx, `SELECT count(*) FROM sources WHERE id=$1`, workID).Scan(&count)
	if count != 1 {
		t.Fatalf("duplicate or missing source %d", count)
	}
	_, err = store.DB.Exec(ctx, `UPDATE paper_acquisition_jobs SET status='running',stage='importing',lease_until=now()-interval '1 minute',lease_token=$2 WHERE id=$1`, jobID, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	worked, err = worker.ProcessOne(ctx)
	if err != nil || !worked {
		t.Fatalf("post-import crash recovery %v %v", worked, err)
	}
	_ = store.DB.QueryRow(ctx, `SELECT count(*) FROM sources WHERE id=$1`, workID).Scan(&count)
	if count != 1 || called {
		t.Fatalf("post-import recovery duplicated source or fetched again: count=%d network=%v", count, called)
	}
	// A terminal job is not selected again, even though its saved checkpoint remains.
	worked, err = worker.ProcessOne(ctx)
	if err != nil || worked {
		t.Fatalf("terminal job reprocessed %v %v", worked, err)
	}
	if !strings.HasSuffix(path, sha+".jats") {
		t.Fatal("checkpoint path is not content-addressed")
	}
}
