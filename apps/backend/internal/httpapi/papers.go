package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/scholarly"
)

var sharedPaperClient = &scholarly.Client{}

func (a *API) paperClient() *scholarly.Client {
	if a.Scholarly != nil {
		return a.Scholarly
	}
	return sharedPaperClient
}
func papersEnabled() bool { return os.Getenv("PAPER_SEARCH_ENABLED") == "1" }

func (a *API) searchPapers(w http.ResponseWriter, r *http.Request) {
	if !papersEnabled() {
		fail(w, 503, "paper search is disabled by PAPER_SEARCH_ENABLED")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 3 || len(query) > 250 {
		fail(w, 400, "enter a 3–250 character query")
		return
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	if provider == "" {
		provider = "all"
	}
	if provider != "all" && provider != "pubmed" && provider != "europepmc" && provider != "semantic_scholar" {
		fail(w, 400, "unsupported paper provider")
		return
	}
	pages := []scholarly.Page{}
	failures := map[string]string{}
	client := a.paperClient()
	offset := 0
	if value := r.URL.Query().Get("offset"); value != "" {
		var e error
		offset, e = strconv.Atoi(value)
		if e != nil || offset < 0 || offset > 9970 {
			fail(w, 400, "invalid paper search offset")
			return
		}
	}
	if provider == "all" || provider == "pubmed" {
		p, e := client.SearchPubMed(r.Context(), query, offset, 20)
		if e != nil {
			failures["pubmed"] = e.Error()
		} else {
			pages = append(pages, p)
		}
	}
	if provider == "semantic_scholar" {
		p, e := client.SearchSemantic(r.Context(), query, offset, 20)
		if e != nil {
			failures["semantic_scholar"] = e.Error()
		} else {
			pages = append(pages, p)
		}
	}
	if provider == "all" || provider == "europepmc" {
		p, e := client.SearchEurope(r.Context(), query, r.URL.Query().Get("cursor"), 20)
		if e != nil {
			failures["europepmc"] = e.Error()
		} else {
			pages = append(pages, p)
		}
	}
	if len(pages) == 0 {
		write(w, 502, map[string]any{"state": "failed", "failures": failures})
		return
	}
	seen := map[string]int{}
	results := []scholarly.Work{}
	for _, p := range pages {
		for _, v := range p.Results {
			keys := []string{}
			if v.DOI != "" {
				keys = append(keys, "doi:"+v.DOI)
			}
			if v.PMID != "" {
				keys = append(keys, "pmid:"+v.PMID)
			}
			if v.PMCID != "" {
				keys = append(keys, "pmcid:"+v.PMCID)
			}
			if len(keys) == 0 {
				keys = append(keys, v.Provider+":"+v.NativeID)
			}
			duplicate := -1
			for _, key := range keys {
				if prior, ok := seen[key]; ok {
					duplicate = prior
					break
				}
			}
			if duplicate >= 0 {
				old := results[duplicate]
				if old.DOI != "" && v.DOI != "" && old.DOI != v.DOI || old.PMID != "" && v.PMID != "" && old.PMID != v.PMID || old.PMCID != "" && v.PMCID != "" && old.PMCID != v.PMCID {
					v.IdentityConflict = true
					results[duplicate].IdentityConflict = true
					results = append(results, v)
				}
				continue
			}
			for _, key := range keys {
				seen[key] = len(results)
			}
			results = append(results, v)
		}
	}
	state := "complete"
	if len(failures) > 0 {
		state = "partial"
	}
	snapshot := map[string]any{"pages": pages, "results": results, "failures": failures, "requested_provider": provider, "query": query}
	payload, _ := json.Marshal(snapshot)
	id := uuid.New()
	actor := requestPrincipal(r.Context()).ID
	_, err := a.Store.DB.Exec(r.Context(), `INSERT INTO paper_searches(id,owner_principal_id,query,providers,state,cap,results_json,provider_status_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, actor, query, []string{provider}, state, 40, payload, payload)
	if err != nil {
		fail(w, 500, "could not save search provenance")
		return
	}
	write(w, 200, map[string]any{"id": id, "state": state, "results": results, "pages": pages, "failures": failures})
}

func (a *API) savePaperReference(w http.ResponseWriter, r *http.Request) {
	if !papersEnabled() {
		fail(w, 503, "paper search is disabled")
		return
	}
	var b struct {
		SearchID uuid.UUID `json:"search_id"`
		Provider string    `json:"provider"`
		NativeID string    `json:"native_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil || b.SearchID == uuid.Nil {
		fail(w, 400, "select a saved search result")
		return
	}
	var snapshot []byte
	err := a.Store.DB.QueryRow(r.Context(), `SELECT results_json FROM paper_searches WHERE id=$1 AND owner_principal_id=$2`, b.SearchID, requestPrincipal(r.Context()).ID).Scan(&snapshot)
	if err != nil {
		fail(w, 404, "search snapshot unavailable")
		return
	}
	var saved struct {
		Results []scholarly.Work `json:"results"`
	}
	if json.Unmarshal(snapshot, &saved) != nil {
		fail(w, 500, "search snapshot is invalid")
		return
	}
	var work scholarly.Work
	found := false
	for _, v := range saved.Results {
		if v.Provider == b.Provider && v.NativeID == b.NativeID {
			work = v
			found = true
			break
		}
	}
	if !found {
		fail(w, 404, "result absent from search snapshot")
		return
	}
	if work.IdentityConflict {
		fail(w, 409, "contradictory provider identifiers require review")
		return
	}
	if work.DOI == "" && work.PMID == "" && work.PMCID == "" {
		fail(w, 409, "result has no verified identifier; manual review required")
		return
	}
	tx, err := a.Store.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `SELECT id FROM scholarly_works WHERE ($1<>'' AND doi=$1) OR ($2<>'' AND pmid=$2) OR ($3<>'' AND pmcid=$3) FOR UPDATE`, work.DOI, work.PMID, work.PMCID).Scan(&id)
	if err == pgx.ErrNoRows {
		id = uuid.New()
		_, err = tx.Exec(r.Context(), `INSERT INTO scholarly_works(id,doi,pmid,pmcid,title,authors,publication_year,publication_type,abstract) VALUES($1,nullif($2,''),nullif($3,''),nullif($4,''),$5,$6,nullif($7,0),$8,$9)`, id, work.DOI, work.PMID, work.PMCID, work.Title, work.Authors, work.Year, work.Type, work.Abstract)
	}
	if err != nil {
		fail(w, 409, "identifier conflict requires review")
		return
	}
	var doi, pmid, pmcid string
	err = tx.QueryRow(r.Context(), `SELECT coalesce(doi,''),coalesce(pmid,''),coalesce(pmcid,'') FROM scholarly_works WHERE id=$1`, id).Scan(&doi, &pmid, &pmcid)
	if err != nil || doi != "" && work.DOI != "" && doi != work.DOI || pmid != "" && work.PMID != "" && pmid != work.PMID || pmcid != "" && work.PMCID != "" && pmcid != work.PMCID {
		fail(w, 409, "contradictory identifiers require review")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE scholarly_works SET doi=coalesce(doi,nullif($2,'')),pmid=coalesce(pmid,nullif($3,'')),pmcid=coalesce(pmcid,nullif($4,'')),updated_at=now() WHERE id=$1`, id, work.DOI, work.PMID, work.PMCID)
	if err != nil {
		fail(w, 409, "identifier linkage conflicts with another saved work; review required")
		return
	}
	raw := work.Raw
	if len(raw) == 0 {
		raw, _ = json.Marshal(work)
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO scholarly_observations(id,work_id,provider,native_id,raw_metadata_json,search_id,result_order) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(search_id,provider,native_id) DO NOTHING`, uuid.New(), id, work.Provider, work.NativeID, raw, b.SearchID, 0)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO paper_saved_references(owner_principal_id,work_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, requestPrincipal(r.Context()).ID, id)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "could not save paper reference")
		return
	}
	write(w, 201, map[string]any{"id": id, "reference_only": true})
}

func (a *API) paperReferences(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.DB.Query(r.Context(), `SELECT sw.id,coalesce(sw.doi,''),coalesce(sw.pmid,''),coalesce(sw.pmcid,''),sw.title,sw.authors,coalesce(sw.publication_year,0),sw.publication_type,sw.access_status,coalesce(s.rights_status,sw.rights_status),coalesce(s.status,sw.processing_status),coalesce(sw.source_id::text,''),sw.study_country,CASE WHEN sw.study_country_evidence_block_id IS NOT NULL AND (lb.processing_revision_id IS DISTINCT FROM s.current_revision_id OR lb.review_status NOT IN ('accepted','corrected')) THEN 'stale' ELSE sw.study_country_review_status END,sw.notice_url,coalesce(sw.notice_checked_at::text,''),coalesce(paj.id::text,''),coalesce(paj.status,''),coalesce(paj.reason,''),coalesce(sw.study_country_evidence_block_id::text,''),sw.study_country_reason,coalesce((SELECT so.search_id::text FROM scholarly_observations so JOIN paper_searches ps ON ps.id=so.search_id WHERE so.work_id=sw.id AND ps.owner_principal_id=pr.owner_principal_id ORDER BY so.retrieved_at DESC LIMIT 1),''),sw.notice_type FROM scholarly_works sw JOIN paper_saved_references pr ON pr.work_id=sw.id LEFT JOIN sources s ON s.id=sw.source_id LEFT JOIN document_blocks lb ON lb.id=sw.study_country_evidence_block_id LEFT JOIN paper_acquisition_jobs paj ON paj.work_id=sw.id WHERE pr.owner_principal_id=$1 ORDER BY pr.saved_at DESC LIMIT 100`, requestPrincipal(r.Context()).ID)
	if err != nil {
		fail(w, 500, "could not list paper references")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var doi, pmid, pmcid, title, authors, typ, access, rights, processing, source, country, countryReview, notice, noticeChecked, jobID, jobStatus, jobReason, locationBlock, locationReason, searchID, noticeType string
		var year int
		if rows.Scan(&id, &doi, &pmid, &pmcid, &title, &authors, &year, &typ, &access, &rights, &processing, &source, &country, &countryReview, &notice, &noticeChecked, &jobID, &jobStatus, &jobReason, &locationBlock, &locationReason, &searchID, &noticeType) != nil {
			fail(w, 500, "could not read paper reference")
			return
		}
		out = append(out, map[string]any{"id": id, "doi": doi, "pmid": pmid, "pmcid": pmcid, "title": title, "authors": authors, "year": year, "publication_type": typ, "access_status": access, "rights_status": rights, "processing_status": processing, "source_id": source, "study_country": country, "study_country_review_status": countryReview, "study_country_evidence_block_id": locationBlock, "study_country_reason": locationReason, "notice_url": notice, "notice_type": noticeType, "notice_checked_at": noticeChecked, "paper_search_id": searchID, "acquisition_job_id": jobID, "acquisition_status": jobStatus, "acquisition_reason": jobReason})
	}
	write(w, 200, out)
}

func (a *API) paperLocations(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var doi string
	err := a.Store.DB.QueryRow(r.Context(), `SELECT coalesce(sw.doi,'') FROM scholarly_works sw JOIN paper_saved_references pr ON pr.work_id=sw.id WHERE sw.id=$1 AND (pr.owner_principal_id=$2 OR $3='admin') LIMIT 1`, id, requestPrincipal(r.Context()).ID, requestRole(r.Context())).Scan(&doi)
	if err != nil {
		fail(w, 404, "saved reference unavailable")
		return
	}
	if doi == "" {
		write(w, 200, map[string]any{"available": false, "reason": "No DOI for Unpaywall lookup"})
		return
	}
	locations, err := a.paperClient().Unpaywall(r.Context(), doi)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	write(w, 200, locations)
}
