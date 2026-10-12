package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/paperjobs"
)

func (a *API) acquirePaper(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if os.Getenv("PAPER_ACQUISITION_ENABLED") != "1" {
		fail(w, 503, "paper acquisition is disabled")
		return
	}
	workID, ok := parseID(w, r)
	if !ok {
		return
	}
	var pmcid, doi, source, notice string
	err := a.Store.DB.QueryRow(r.Context(), `SELECT coalesce(pmcid,''),coalesce(doi,''),coalesce(source_id::text,''),notice_url FROM scholarly_works WHERE id=$1`, workID).Scan(&pmcid, &doi, &source, &notice)
	if err != nil {
		fail(w, 404, "paper reference unavailable")
		return
	}
	if source != "" {
		write(w, 200, map[string]any{"source_id": source, "status": "already_imported"})
		return
	}
	if notice != "" {
		fail(w, 409, "a recorded paper notice blocks acquisition; see "+notice)
		return
	}
	candidates := []paperjobs.Candidate{}
	if pmcid != "" {
		candidates = append(candidates, paperjobs.Candidate{Resolver: "europepmc", Format: "jats", URL: "https://www.ebi.ac.uk/europepmc/webservices/rest/" + pmcid + "/fullTextXML"})
		candidates = append(candidates, paperjobs.Candidate{Resolver: "pmc_oai", Format: "jats", URL: "https://pmc.ncbi.nlm.nih.gov/api/oai/v1/mh/?verb=GetRecord&identifier=oai:pubmedcentral.nih.gov:" + strings.TrimPrefix(pmcid, "PMC") + "&metadataPrefix=pmc"})
	}
	resolverReason := ""
	if doi != "" {
		locations, lookupErr := a.paperClient().Unpaywall(r.Context(), doi)
		if lookupErr != nil {
			resolverReason = "Unpaywall lookup failed; DOI copy was not queued"
		} else {
			resolverReason = locations.Reason
			for _, loc := range locations.Candidates {
				candidate, eligible := paperjobs.CandidateFromLocation(loc)
				if eligible && len(candidates) < 5 {
					candidates = append(candidates, candidate)
				}
			}
		}
	}
	if len(candidates) == 0 {
		if resolverReason == "" {
			resolverReason = "No PMCID or eligible, licensed DOI candidate"
		}
		fail(w, 409, resolverReason+"; keep reference-only")
		return
	}
	urls, _ := json.Marshal(candidates)
	limits, _ := json.Marshal(map[string]any{"candidate_locations": len(candidates), "transient_retries": 3, "request_seconds": 30, "structured_response_bytes": 10 << 20, "pdf_response_bytes": 25 << 20, "jats_nesting": 64, "resolver_note": resolverReason})
	id := uuid.New()
	err = a.Store.DB.QueryRow(r.Context(), `INSERT INTO paper_acquisition_jobs(id,work_id,policy_version,candidate_urls_json,effective_limits_json) VALUES($1,$2,$3,$4,$5) ON CONFLICT(work_id) DO UPDATE SET updated_at=paper_acquisition_jobs.updated_at RETURNING id`, id, workID, paperjobs.PolicyVersion, urls, limits).Scan(&id)
	if err != nil {
		fail(w, 500, "could not queue paper acquisition")
		return
	}
	_, _ = a.Store.DB.Exec(r.Context(), `UPDATE scholarly_works SET processing_status='queued',updated_at=now() WHERE id=$1 AND processing_status='not_started'`, workID)
	a.paperAcquisitionResponse(w, r, id, 202)
}

func (a *API) paperAcquisitionResponse(w http.ResponseWriter, r *http.Request, id uuid.UUID, code int) {
	var workID uuid.UUID
	var status, stage, reason, errorCode, source string
	var attempts int
	err := a.Store.DB.QueryRow(r.Context(), `SELECT work_id,status,stage,reason,error_code,coalesce(source_id::text,''),attempts FROM paper_acquisition_jobs WHERE id=$1`, id).Scan(&workID, &status, &stage, &reason, &errorCode, &source, &attempts)
	if err != nil {
		fail(w, 404, "acquisition job unavailable")
		return
	}
	write(w, code, map[string]any{"id": id, "work_id": workID, "status": status, "stage": stage, "reason": reason, "error_code": errorCode, "source_id": source, "attempts": attempts})
}

func (a *API) paperAcquisition(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	a.paperAcquisitionResponse(w, r, id, 200)
}

func (a *API) cancelPaperAcquisition(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	tag, err := a.Store.DB.Exec(r.Context(), `UPDATE paper_acquisition_jobs SET status='cancelled',stage='stopped',reason='Cancelled by administrator',error_code='cancelled',lease_token=NULL,lease_until=NULL,updated_at=now(),completed_at=now() WHERE id=$1 AND status IN ('queued','running') AND stage<>'importing'`, id)
	if err != nil {
		fail(w, 500, "could not cancel acquisition")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 409, "acquisition cannot be cancelled in its current stage")
		return
	}
	a.paperAcquisitionResponse(w, r, id, 200)
}

func (a *API) retryPaperAcquisition(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	tag, err := a.Store.DB.Exec(r.Context(), `UPDATE paper_acquisition_jobs SET status='queued',stage=CASE WHEN checkpoint_sha256 IS NULL THEN 'resolve' ELSE 'validated' END,attempts=0,next_retry_at=NULL,error_code='',reason='',lease_until=NULL,lease_token=NULL,completed_at=NULL,updated_at=now() WHERE id=$1 AND status IN ('failed','cancelled')`, id)
	if err != nil {
		fail(w, 500, "could not retry acquisition")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, 409, "only a failed or cancelled acquisition can be retried")
		return
	}
	a.paperAcquisitionResponse(w, r, id, 202)
}
