package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/localllm"
)

func TestCollectionActivationCompatibilityIntegration(t *testing.T) {
	db := os.Getenv("COLLECTION_API_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set COLLECTION_API_TEST_DATABASE_URL to a disposable database")
	}
	ctx := context.Background()
	root, _ := filepath.Abs("../../../..")
	store, err := core.Open(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	model := localllm.New(localllm.Config{EmbeddingModel: "activation-fixture", EmbeddingRevision: "v1", Dimensions: 3})
	a := &API{Store: store, Token: "activation-admin-fixture-12345", Model: model}
	if err = a.SyncPrincipals(ctx); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	collectionID, configID := uuid.New(), uuid.New()
	exec(`INSERT INTO linked_collections(id,title,author) VALUES($1,'Activation fixture','Fixture author')`, collectionID)
	exec(`INSERT INTO embedding_configs(id,model_id,model_revision,dimensions,preprocessing_version) VALUES($1,'activation-fixture','v1',3,$2)`, configID, uuid.NewString())
	fixture := func(generation int) (uuid.UUID, uuid.UUID) {
		t.Helper()
		snapshot, source, pub, run := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		exec(`INSERT INTO collection_snapshots(id,collection_id,generation,scope_json,state) VALUES($1,$2,$3,'{}','review')`, snapshot, collectionID, generation)
		exec(`INSERT INTO sources(id,source_key,title,author,pdf_path,pdf_sha256,pdf_bytes,page_count,status) VALUES($1,$2,'Activation fixture','Author','fixture.pdf',repeat('a',64),1,1,'review')`, source, uuid.NewString())
		exec(`INSERT INTO publications(id,source_id,processing_revision_id,source_asset_id) SELECT $2,id,current_revision_id,primary_asset_id FROM sources WHERE id=$1`, source, pub)
		exec(`INSERT INTO index_runs(id,publication_id,embedding_config_id,status,expected_chunk_count,indexed_chunk_count) VALUES($1,$2,$3,'ready',1,1)`, run, pub, configID)
		exec(`INSERT INTO active_indexes(source_id,index_run_id) VALUES($1,$2)`, source, run)
		exec(`UPDATE sources SET status='published',rights_status='allowed',published_revision_id=current_revision_id WHERE id=$1`, source)
		exec(`INSERT INTO collection_items(id,snapshot_id,requested_url,final_url,sha256,byte_size,object_locator,depth,discovery_ordinal,state,role,source_id,preparation_state) VALUES($1,$2,'https://fixture.example/chapter','https://fixture.example/chapter',repeat('a',64),1,'fixture',0,0,'fetched','content_candidate',$3,'done')`, uuid.New(), snapshot, source)
		return snapshot, source
	}
	activate := func(partial bool, want int) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"rationale": "Reviewed synthetic activation fixture", "allow_partial": partial})
		req := httptest.NewRequest("POST", "/api/v1/collections/"+collectionID.String()+"/activate", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+a.Token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("activation %d want %d: %s", res.Code, want, res.Body.String())
		}
	}
	assertActive := func(snapshot uuid.UUID, decisions int) {
		t.Helper()
		var actual uuid.UUID
		var count int
		if err := store.DB.QueryRow(ctx, `SELECT snapshot_id,(SELECT count(*) FROM collection_activation_decisions WHERE collection_id=$1) FROM active_collection_snapshots WHERE collection_id=$1`, collectionID).Scan(&actual, &count); err != nil || actual != snapshot || count != decisions {
			t.Fatalf("active snapshot/decision count %s/%d want %s/%d: %v", actual, count, snapshot, decisions, err)
		}
	}
	first, firstSource := fixture(1)
	activate(false, 200)
	assertActive(first, 1)
	second, secondSource := fixture(2)
	for _, field := range []string{"model", "revision", "dimensions"} {
		original := model.Config
		switch field {
		case "model":
			model.Config.EmbeddingModel = "incompatible"
		case "revision":
			model.Config.EmbeddingRevision = "incompatible"
		case "dimensions":
			model.Config.Dimensions = 4
		}
		activate(false, 409)
		assertActive(first, 1)
		model.Config = original
	}
	a.Model = nil
	activate(false, 503)
	a.Model = model
	exec(`UPDATE sources SET rights_status='denied' WHERE id=$1`, secondSource)
	activate(false, 409)
	assertActive(first, 1)
	exec(`UPDATE sources SET rights_status='allowed' WHERE id=$1`, secondSource)
	exec(`UPDATE collection_snapshots SET incomplete_reasons=ARRAY['fixture fetch failure'] WHERE id=$1`, second)
	activate(false, 409)
	assertActive(first, 1)
	activate(true, 200)
	assertActive(second, 2)
	var oldEligible, newEligible, partial bool
	if err := store.DB.QueryRow(ctx, `SELECT collection_source_retrieval_eligible($1),collection_source_retrieval_eligible($2),(SELECT partial FROM collection_activation_decisions WHERE snapshot_id=$3)`, firstSource, secondSource, second).Scan(&oldEligible, &newEligible, &partial); err != nil || oldEligible || !newEligible || !partial {
		t.Fatalf("replacement or partial audit failed: %t/%t/%t %v", oldEligible, newEligible, partial, err)
	}
	third, _ := fixture(3)
	exec(`UPDATE collection_snapshots SET incomplete_reasons=ARRAY['link limit reached','failed documents'] WHERE id=$1`, third)
	req := httptest.NewRequest("POST", "/api/v1/collections/"+collectionID.String()+"/resume", strings.NewReader(`{"rationale":"Retry synthetic failed pages"}`))
	req.Header.Set("Authorization", "Bearer "+a.Token)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != 202 {
		t.Fatalf("resume %d: %s", res.Code, res.Body.String())
	}
	var reasons []string
	if err := store.DB.QueryRow(ctx, `SELECT incomplete_reasons FROM collection_snapshots WHERE id=$1`, third).Scan(&reasons); err != nil || len(reasons) != 1 || reasons[0] != "link limit reached" {
		t.Fatalf("resume erased undiscovered-link warning: %v %v", reasons, err)
	}
	assertActive(second, 2)
}
