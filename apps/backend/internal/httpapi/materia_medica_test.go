package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
)

func TestMMReviewRequiresAdmin(t *testing.T) {
	a := (&API{Token: "mm-admin", ReviewerToken: "mm-reviewer"}).Handler()
	for _, path := range []string{"/api/v1/sources/" + uuid.NewString() + "/mm-entries", "/api/v1/sources/" + uuid.NewString() + "/mm-units", "/api/v1/remedies"} {
		method := "GET"
		if strings.HasSuffix(path, "mm-entries") {
			method = "POST"
		}
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer mm-reviewer")
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

func TestMMContinuityIntegration(t *testing.T) {
	db := os.Getenv("MM_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set MM_TEST_DATABASE_URL to a disposable database")
	}
	ctx := context.Background()
	root, _ := filepath.Abs("../../../..")
	s, err := core.Open(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	s.Root = t.TempDir()
	a := &API{Store: s, Token: "mm-admin-token-123456789"}
	if err = a.SyncPrincipals(ctx); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := s.DB.Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+a.Token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	type fixture struct {
		sid, rev uuid.UUID
		locs     []structuredLocationInput
		chunks   []uuid.UUID
	}
	makeSource := func(format string) fixture {
		t.Helper()
		f := fixture{sid: uuid.New()}
		texts := []string{"BELLADONNA\nMind.— Restless.", "Head.— Throbbing; worse noise. Café.", "NUX VOMICA\nStomach.— Nausea."}
		if format == "pdf" {
			exec(`INSERT INTO sources(id,source_key,title,author,pdf_path,pdf_sha256,pdf_bytes,page_count,status,literature_categories) VALUES($1,$2,'MM fixture','Fixture author','unused.pdf',$3,1,3,'review',ARRAY['materia_medica'])`, f.sid, "mm-"+f.sid.String(), strings.Repeat("a", 64))
		} else {
			f.sid, err = s.ImportDocument(ctx, strings.NewReader(strings.Join(texts, "\n\n")), core.DocumentImport{Title: "MM fixture", Author: "Fixture author", Transport: "upload", ContentType: "text/plain"})
			if err != nil {
				t.Fatal(err)
			}
			exec(`UPDATE sources SET status='review',literature_categories=ARRAY['materia_medica'] WHERE id=$1`, f.sid)
		}
		if err = s.DB.QueryRow(ctx, `SELECT current_revision_id FROM sources WHERE id=$1`, f.sid).Scan(&f.rev); err != nil {
			t.Fatal(err)
		}
		for i, text := range texts {
			uid := uuid.New()
			loc := structuredLocationInput{Start: 0, End: len([]rune(text)), ExactText: text, TextSHA256: mmTextHash(text)}
			if format == "pdf" {
				loc.PageID = &uid
				h := sha256.Sum256([]byte(text))
				exec(`INSERT INTO pages(id,source_id,pdf_page_index,scan_page_index,canvas_id,alto_url,image_url,text_raw,text_sha256,extraction_method,text_qa_status,page_kind) VALUES($1,$2,$3,$3,$4,'','',$5,$6,'fixture','accepted','text')`, uid, f.sid, i, uuid.NewString(), text, hex.EncodeToString(h[:]))
			} else {
				loc.DocumentBlockID = &uid
				exec(`INSERT INTO document_blocks(id,source_id,source_asset_id,processing_revision_id,block_index,section_key,kind,original_text,reviewed_text,start_byte,end_byte,review_status) SELECT $2,id,primary_asset_id,current_revision_id,$3,$4,'paragraph',$5,$5,0,$6,'accepted' FROM sources WHERE id=$1`, f.sid, uid, i, uuid.NewString(), text, len(text))
			}
			cid := uuid.New()
			exec(`INSERT INTO chunks(id,page_id,document_block_id,source_id,chunk_index,text_exact,start_character,end_character) VALUES($1,$2,$3,$4,0,$5,0,$6)`, cid, loc.PageID, loc.DocumentBlockID, f.sid, text, loc.End)
			f.locs = append(f.locs, loc)
			f.chunks = append(f.chunks, cid)
		}
		return f
	}
	request := func(f fixture, prep string) map[string]any {
		return map[string]any{"revision_id": f.rev, "spelling": "BELLADONNA", "canonical_name": "Belladonna", "preparation_key": prep, "rationale": "Checked heading and continuation against both originals", "locations": f.locs[:2]}
	}
	approve := func(f fixture, prep string) (uuid.UUID, uuid.UUID) {
		t.Helper()
		w := call("POST", "/sources/"+f.sid.String()+"/mm-entries", request(f, prep))
		if w.Code != 201 {
			t.Fatalf("approve %d %s", w.Code, w.Body)
		}
		var result struct {
			ID       uuid.UUID
			RemedyID uuid.UUID `json:"remedy_id"`
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		return result.ID, result.RemedyID
	}
	pdf := makeSource("pdf")
	eid, rid := approve(pdf, "fixture plant preparation")
	for i, l := range pdf.locs {
		if err = s.DB.QueryRow(ctx, `SELECT id FROM chunks WHERE page_id=$1 ORDER BY chunk_index LIMIT 1`, l.PageID).Scan(&pdf.chunks[i]); err != nil {
			t.Fatal(err)
		}
	}
	if w := call("POST", "/sources/"+pdf.sid.String()+"/mm-entries", request(pdf, "fixture plant preparation")); w.Code != 409 {
		t.Fatalf("overlap accepted: %d %s", w.Code, w.Body)
	}
	txt := makeSource("txt")
	_, other := approve(txt, "fixture distinct preparation")
	if other == rid {
		t.Fatal("distinct preparations merged")
	}
	// Corrections invalidate the previous mapping without erasing the review record.
	exec(`UPDATE document_blocks SET reviewed_text=reviewed_text||' Changed.' WHERE id=$1`, txt.locs[1].DocumentBlockID)
	var status string
	if err = s.DB.QueryRow(ctx, `SELECT review_status FROM structured_entries WHERE source_id=$1`, txt.sid).Scan(&status); err != nil || status != "rejected" {
		t.Fatalf("text correction left approval: %s %v", status, err)
	}
	stale := request(txt, "fixture distinct preparation")
	if w := call("POST", "/sources/"+txt.sid.String()+"/mm-entries", stale); w.Code != 409 {
		t.Fatalf("stale text accepted: %d %s", w.Code, w.Body)
	}
	missing := request(txt, "fixture distinct preparation")
	missing["locations"] = txt.locs[1:2]
	if w := call("POST", "/sources/"+txt.sid.String()+"/mm-entries", missing); w.Code != 400 {
		t.Fatal("heading-free identity accepted")
	}
	// An entry beginning mid-page gets its own exact passages, not the
	// preceding text or following remedy's name.
	boundary := makeSource("pdf")
	prefix, suffix := "Editorial preface.\n", "\nNUX VOMICA\nAnother entry."
	mixed := prefix + boundary.locs[0].ExactText + suffix
	exec(`UPDATE pages SET text_raw=$2 WHERE id=$1`, boundary.locs[0].PageID, mixed)
	boundary.locs[0].Start = len([]rune(prefix))
	boundary.locs[0].End = boundary.locs[0].Start + len([]rune(boundary.locs[0].ExactText))
	boundary.locs[0].TextSHA256 = mmTextHash(mixed)
	boundaryID, _ := approve(boundary, "fixture plant preparation")
	var exactChunk string
	if err = s.DB.QueryRow(ctx, `SELECT text_exact FROM chunks WHERE page_id=$1 AND start_character=$2 AND end_character=$3`, boundary.locs[0].PageID, boundary.locs[0].Start, boundary.locs[0].End).Scan(&exactChunk); err != nil || exactChunk != boundary.locs[0].ExactText {
		t.Fatalf("entry boundary chunk: %q %v", exactChunk, err)
	}
	exec(`UPDATE pages SET text_qa_status='suspect' WHERE id=$1`, boundary.locs[1].PageID)
	if err = s.DB.QueryRow(ctx, `SELECT review_status FROM structured_entries WHERE id=$1`, boundaryID).Scan(&status); err != nil || status != "rejected" {
		t.Fatal("QA downgrade retained approval")
	}
	units := call("GET", "/sources/"+pdf.sid.String()+"/mm-units", nil)
	if units.Code != 200 || !strings.Contains(units.Body.String(), "text_sha256") {
		t.Fatalf("review units: %d %s", units.Code, units.Body)
	}
	// Publication/index fixture: test exact continuation retrieval and all gates.
	config, pub, run := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO embedding_configs(id,model_id,model_revision,dimensions,preprocessing_version) VALUES($1,$2,'v1',3,'fixture')`, config, uuid.NewString())
	exec(`INSERT INTO publications(id,source_id,processing_revision_id,source_asset_id) SELECT $2,id,current_revision_id,primary_asset_id FROM sources WHERE id=$1`, pdf.sid, pub)
	exec(`INSERT INTO index_runs(id,publication_id,embedding_config_id,status,expected_chunk_count,indexed_chunk_count) VALUES($1,$2,$3,'ready',3,3)`, run, pub, config)
	for _, cid := range pdf.chunks {
		exec(`INSERT INTO chunk_embeddings(chunk_id,embedding_config_id,embedding,input_hash) VALUES($1,$2,'[1,0,0]',repeat('a',64))`, cid, config)
	}
	exec(`INSERT INTO active_indexes(source_id,index_run_id) VALUES($1,$2)`, pdf.sid, run)
	exec(`UPDATE sources SET status='published',rights_status='allowed',published_revision_id=current_revision_id WHERE id=$1`, pdf.sid)
	var n int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM eligible_mm_chunks WHERE entry_id=$1`, eid).Scan(&n); err != nil || n != 2 {
		t.Fatalf("linked chunk count %d: %v", n, err)
	}
	hits, e := a.retrieve(ctx, config, "[1,0,0]", "What does Belladonna say about the head?", "", []uuid.UUID{pdf.sid})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, h := range hits {
		if h.ID == pdf.chunks[1] {
			found = true
			if !strings.Contains(h.RemedyContext, "Belladonna") || h.Text != pdf.locs[1].ExactText {
				t.Fatalf("lost continuation context: %+v", h)
			}
		}
		if h.ID == pdf.chunks[2] && h.RemedyContext != "" {
			t.Fatal("next remedy inherited identity")
		}
	}
	if !found {
		t.Fatal("continuation was not retrieved")
	}
	scoped, e := a.retrieve(context.WithValue(ctx, categoryFilterKey{}, []string{"research"}), config, "[1,0,0]", "Belladonna", "", []uuid.UUID{pdf.sid})
	if e != nil || len(scoped) != 0 {
		t.Fatalf("category leak %d %v", len(scoped), e)
	}
	scoped, e = a.retrieve(ctx, config, "[1,0,0]", "Belladonna", "", []uuid.UUID{txt.sid})
	if e != nil || len(scoped) != 0 {
		t.Fatalf("source scope leak %d %v", len(scoped), e)
	}
	if w := call("POST", "/sources/"+pdf.sid.String()+"/mm-entries", request(pdf, "fixture plant preparation")); w.Code != 409 {
		t.Fatal("published mapping editable")
	}
	exec(`UPDATE sources SET status='disabled' WHERE id=$1`, pdf.sid)
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM eligible_mm_chunks WHERE entry_id=$1`, eid).Scan(&n); err != nil || n != 0 {
		t.Fatal("disabled mapping leaked")
	}
}

func TestMMIdentityContextDoesNotBecomeQuotation(t *testing.T) {
	h := hit{ID: uuid.New(), Text: "Head.— The headache is worse from noise.", RemedyContext: "Reviewed entry: Belladonna; preparation: fixture."}
	for _, quote := range []string{"The headache is worse from noise.", "Reviewed entry: Belladonna; preparation: fixture."} {
		calls := 0
		_, _, rejected, checks, err := verifyClaimSentencesBatched(context.Background(), "What does this entry say?", "Belladonna is described as worse from noise [E1].", []hit{h}, func(_ context.Context, _ string, user string) (string, error) {
			calls++
			if !strings.Contains(user, h.RemedyContext) {
				t.Fatal("reviewed identity context missing")
			}
			if calls == 1 {
				raw, _ := json.Marshal(map[string]any{"checks": []any{map[string]any{"claim": 1, "quotes": []any{map[string]any{"id": "E1", "text": quote}}}}})
				return string(raw), nil
			}
			return `{"checks":[{"claim":1,"supported":true,"relevant":true}]}`, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if quote == h.RemedyContext {
			if rejected != 1 || calls != 1 {
				t.Fatal("identity metadata was accepted as a quotation")
			}
		} else {
			if rejected != 0 || len(checks) != 1 || strings.TrimSuffix(checks[0].Supports[0].Text, ".") != "The headache is worse from noise" {
				t.Fatalf("exact continuation quote failed: %+v", checks)
			}
		}
	}
}
