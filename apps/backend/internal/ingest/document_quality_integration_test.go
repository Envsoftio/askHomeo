package ingest

import (
	"context"
	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/document"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenericDocumentPreparationIntegration(t *testing.T) {
	dbURL := os.Getenv("INTAKE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("set INTAKE_TEST_DATABASE_URL to a disposable database")
	}
	ctx := context.Background()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store.Root = t.TempDir()
	raw := `<html><p><a href="/">Home</a></p><p>(advertising)</p><h1>Symptoms</h1><h2>Head</h2><p>Cold -- Abc., Def., Ghi.<br>Heat -- Jkl., Mno., Pqr.<br>Motion -- Stu., Vwx., Yza.</p></html>`
	id, err := store.ImportDocument(ctx, strings.NewReader(raw), core.DocumentImport{Title: "Generic fixture", Author: "Fixture author", Transport: "upload", ContentType: "text/html"})
	if err != nil {
		t.Fatal(err)
	}
	var job, revision, asset uuid.UUID
	if err = store.DB.QueryRow(ctx, `SELECT j.id,s.current_revision_id,s.primary_asset_id FROM jobs j JOIN sources s ON s.id=j.source_id WHERE s.id=$1 AND j.kind='ingest'`, id).Scan(&job, &revision, &asset); err != nil {
		t.Fatal(err)
	}
	worker := New(store, nil)
	if err = worker.processDocument(ctx, job, id); err != nil {
		t.Fatal(err)
	}
	var category, origin, evidence, version string
	if err = store.DB.QueryRow(ctx, `SELECT literature_categories[1],literature_category_origin,evidence_category,r.component_versions_json->>'document_extractor' FROM sources s JOIN processing_revisions r ON r.id=s.current_revision_id WHERE s.id=$1`, id).Scan(&category, &origin, &evidence, &version); err != nil {
		t.Fatal(err)
	}
	if category != "repertory" || origin != "automatic" || evidence != "unknown" || version != document.Version {
		t.Fatalf("wrong automatic metadata %s %s %s %s", category, origin, evidence, version)
	}
	var excluded, decisions int
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM document_blocks WHERE source_id=$1 AND review_status='excluded' AND review_note LIKE 'document-structure-v2:%'`, id).Scan(&excluded); err != nil {
		t.Fatal(err)
	}
	if err = store.DB.QueryRow(ctx, `SELECT count(*) FROM literature_category_decisions WHERE source_id=$1 AND origin='detected' AND actor_principal_id IS NULL`, id).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if excluded != 2 || decisions != 1 {
		t.Fatalf("exclusions=%d decisions=%d", excluded, decisions)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE sources SET literature_categories=ARRAY['other'],literature_category_origin='manual',evidence_category='classical_reference' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	worker.suggestCategories(ctx, id, revision, asset, "Materia medica: remedy picture and modalities.")
	if err = store.DB.QueryRow(ctx, `SELECT literature_categories[1],evidence_category FROM sources WHERE id=$1`, id).Scan(&category, &evidence); err != nil {
		t.Fatal(err)
	}
	if category != "other" || evidence != "classical_reference" {
		t.Fatal("detection overwrote manual metadata")
	}
}
