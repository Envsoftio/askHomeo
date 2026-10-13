package paperjobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/document"
	"homeopath-poc/backend/internal/safefetch"
	"homeopath-poc/backend/internal/scholarly"
)

const PolicyVersion = "paper-acquisition-2"

type Worker struct {
	Store          *core.Store
	Client         *scholarly.Client
	Fetcher        *safefetch.Fetcher
	fetchAlternate func(context.Context, Candidate) ([]byte, string, error)
}
type job struct {
	ID, WorkID, LeaseToken                                           uuid.UUID
	Attempts                                                         int
	CheckpointSHA, CheckpointPath, CheckpointURL, CheckpointResolver *string
	CandidateJSON                                                    []byte
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		worked, err := w.ProcessOne(ctx)
		if err != nil {
			return err
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// ProcessOne uses a durable lease. The validated raw checkpoint is saved before
// source import so a crash after download resumes without contacting a provider.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var j job
	err = tx.QueryRow(ctx, `SELECT id,work_id,attempts,checkpoint_sha256,checkpoint_locator,checkpoint_source_url,checkpoint_resolver,candidate_urls_json FROM paper_acquisition_jobs WHERE (status='queued' AND (next_retry_at IS NULL OR next_retry_at<=now())) OR (status='running' AND lease_until<now()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&j.ID, &j.WorkID, &j.Attempts, &j.CheckpointSHA, &j.CheckpointPath, &j.CheckpointURL, &j.CheckpointResolver, &j.CandidateJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	j.LeaseToken = uuid.New()
	j.Attempts++
	_, err = tx.Exec(ctx, `UPDATE paper_acquisition_jobs SET status='running',stage='resolve',attempts=$2,lease_token=$3,lease_until=now()+interval '2 minutes',next_retry_at=NULL,error_code='',reason='',updated_at=now() WHERE id=$1`, j.ID, j.Attempts, j.LeaseToken)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	if err = w.process(ctx, j); err != nil {
		code := "acquisition_failed"
		status := "failed"
		var retry *time.Time
		var providerErr *scholarly.ProviderError
		transient := errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "request failed") || strings.Contains(err.Error(), "HTTP 429") || strings.Contains(err.Error(), "HTTP 5")
		if errors.As(err, &providerErr) {
			transient = providerErr.Status == 429 || providerErr.Status >= 500
		}
		if j.Attempts < 3 && transient {
			code = "transient_provider_failure"
			status = "queued"
			backoff := time.Duration(j.Attempts*j.Attempts)*time.Minute + time.Duration(rand.Intn(10))*time.Second
			if providerErr != nil && providerErr.RetryAfter > backoff {
				backoff = providerErr.RetryAfter
			}
			next := time.Now().Add(backoff)
			retry = &next
		}
		_, updateErr := w.Store.DB.Exec(context.WithoutCancel(ctx), `UPDATE paper_acquisition_jobs SET status=$3,stage='stopped',error_code=$4,reason=$5,next_retry_at=$6,lease_until=NULL,lease_token=NULL,updated_at=now(),completed_at=CASE WHEN $3='failed' THEN now() ELSE completed_at END WHERE id=$1 AND lease_token=$2`, j.ID, j.LeaseToken, status, code, err.Error(), retry)
		if updateErr != nil {
			return true, updateErr
		}
		_, _ = w.Store.DB.Exec(context.WithoutCancel(ctx), `UPDATE scholarly_works SET processing_status=$2,access_status=CASE WHEN $3 THEN 'full_text_candidate' ELSE access_status END,updated_at=now() WHERE id=$1`, j.WorkID, status, lenString(j.CheckpointSHA) > 0)
	}
	return true, nil
}

func lenString(p *string) int {
	if p == nil {
		return 0
	}
	return len(*p)
}

func (w *Worker) process(ctx context.Context, j job) error {
	var pmcid, pmid, doi, title, authors, source, notice string
	if err := w.Store.DB.QueryRow(ctx, `SELECT coalesce(pmcid,''),coalesce(pmid,''),coalesce(doi,''),title,authors,coalesce(source_id::text,''),notice_url FROM scholarly_works WHERE id=$1`, j.WorkID).Scan(&pmcid, &pmid, &doi, &title, &authors, &source, &notice); err != nil {
		return err
	}
	if notice != "" {
		return errors.New("recorded paper notice blocks acquisition: " + notice)
	}
	if source != "" {
		linked, err := uuid.Parse(source)
		if err != nil {
			return err
		}
		return w.complete(ctx, j, linked)
	}
	var committed bool
	if err := w.Store.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sources WHERE id=$1)`, j.WorkID).Scan(&committed); err != nil {
		return err
	}
	if committed {
		return w.complete(ctx, j, j.WorkID)
	}
	var raw []byte
	sourceURL, resolver, format := "", "", ""
	if lenString(j.CheckpointSHA) > 0 {
		sha, path := *j.CheckpointSHA, *j.CheckpointPath
		format = strings.TrimPrefix(filepath.Ext(path), ".")
		if format != "jats" && format != "pdf" && format != "html" || path != filepath.Join("data/runtime/paper-jobs", sha+"."+format) {
			return errors.New("checkpoint path differs from checksum")
		}
		var err error
		raw, err = os.ReadFile(filepath.Join(w.Store.Root, path))
		if err != nil {
			return fmt.Errorf("saved checkpoint unavailable: %w", err)
		}
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != sha {
			return errors.New("saved checkpoint checksum differs")
		}
		if err = validateCandidate(ctx, raw, format, pmcid, pmid, doi, title); err != nil {
			return err
		}
		sourceURL, resolver = *j.CheckpointURL, *j.CheckpointResolver
	} else {
		candidates := []Candidate{}
		if err := json.Unmarshal(j.CandidateJSON, &candidates); err != nil {
			// Jobs queued before the candidate snapshot upgrade contained URL strings.
			if pmcid != "" {
				candidates = []Candidate{{Resolver: "europepmc", Format: "jats", URL: "https://www.ebi.ac.uk/europepmc/webservices/rest/" + pmcid + "/fullTextXML"}, {Resolver: "pmc_oai", Format: "jats", URL: "https://pmc.ncbi.nlm.nih.gov/api/oai/v1/mh/?verb=GetRecord&identifier=oai:pubmedcentral.nih.gov:" + strings.TrimPrefix(pmcid, "PMC") + "&metadataPrefix=pmc"}}
			}
		}
		if len(candidates) == 0 || len(candidates) > 5 {
			return errors.New("acquisition has no bounded candidate snapshot")
		}
		reasons := []string{}
		candidateErrors := []error{}
		for _, candidate := range candidates {
			data, finalURL, err := w.fetchCandidate(ctx, candidate, pmcid)
			if err == nil && candidate.Resolver == "pmc_oai" {
				start := bytes.Index(data, []byte("<article"))
				end := bytes.LastIndex(data, []byte("</article>"))
				if start < 0 || end < start {
					err = errors.New("PMC OAI contained no article payload")
				} else {
					data = data[start : end+len("</article>")]
				}
			}
			if err == nil {
				err = validateCandidate(ctx, data, candidate.Format, pmcid, pmid, doi, title)
			}
			outcome := "validated"
			reason := ""
			if err != nil {
				outcome = "failed"
				reason = err.Error()
				reasons = append(reasons, candidate.Resolver+": "+reason)
				candidateErrors = append(candidateErrors, err)
			}
			checksum := ""
			if err == nil {
				h := sha256.Sum256(data)
				checksum = hex.EncodeToString(h[:])
			}
			_, _ = w.Store.DB.Exec(ctx, `INSERT INTO paper_acquisition_attempts(id,work_id,resolver,candidate_url,state,reason,response_sha256,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()) ON CONFLICT(work_id,resolver,candidate_url) DO UPDATE SET state=$5,reason=$6,response_sha256=$7,completed_at=now()`, uuid.New(), j.WorkID, candidate.Resolver, candidate.URL, outcome, reason, checksum)
			if err == nil {
				raw = data
				sourceURL = finalURL
				resolver = candidate.Resolver
				format = candidate.Format
				break
			}
		}
		if raw == nil {
			cause := candidateErrors[len(candidateErrors)-1]
			for _, candidateErr := range candidateErrors {
				var providerErr *scholarly.ProviderError
				if errors.As(candidateErr, &providerErr) && (providerErr.Status == 429 || providerErr.Status >= 500) {
					cause = candidateErr
					break
				}
			}
			return fmt.Errorf("no permitted article acquired (%s): %w", strings.Join(reasons, "; "), cause)
		}
		sha, path, err := w.saveCheckpointFormat(raw, format)
		if err != nil {
			return err
		}
		tag, err := w.Store.DB.Exec(ctx, `UPDATE paper_acquisition_jobs SET stage='validated',checkpoint_sha256=$3,checkpoint_locator=$4,checkpoint_source_url=$5,checkpoint_resolver=$6,updated_at=now() WHERE id=$1 AND lease_token=$2 AND status='running'`, j.ID, j.LeaseToken, sha, path, sourceURL, resolver)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("acquisition cancelled before checkpoint")
		}
	}
	tag, err := w.Store.DB.Exec(ctx, `UPDATE paper_acquisition_jobs SET stage='importing',updated_at=now() WHERE id=$1 AND lease_token=$2 AND status='running'`, j.ID, j.LeaseToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("acquisition cancelled before import")
	}
	var sourceID uuid.UUID
	switch format {
	case "pdf":
		sourceID, err = w.Store.ImportPDFWithOriginID(ctx, bytes.NewReader(raw), title, authors, "", "", resolver, sourceURL, "Article-level CC BY 4.0 or CC0 signal; exact use requires rights review", sourceURL, j.WorkID)
	case "html", "jats":
		contentType := "text/html"
		if format == "jats" {
			contentType = "application/xml"
		}
		info := core.DocumentImport{Title: title, Author: authors, Repository: resolver, SourceURL: sourceURL, RequestedURL: sourceURL, FinalURL: sourceURL, Transport: "https", ContentType: contentType, RightsStatement: "Article-level CC BY 4.0 or CC0 signal; exact use requires rights review", SourceID: &j.WorkID}
		sourceID, err = w.Store.ImportDocument(ctx, bytes.NewReader(raw), info)
	default:
		return errors.New("validated checkpoint has unsupported format")
	}
	if err != nil {
		return fmt.Errorf("validated article could not be saved: %w", err)
	}
	return w.complete(ctx, j, sourceID)
}

func (w *Worker) complete(ctx context.Context, j job, sourceID uuid.UUID) error {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE paper_acquisition_jobs SET status='review',stage='source_queued',source_id=$3,lease_until=NULL,lease_token=NULL,updated_at=now(),completed_at=now() WHERE id=$1 AND lease_token=$2 AND status='running'`, j.ID, j.LeaseToken, sourceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("acquisition cancelled before completion")
	}
	_, err = tx.Exec(ctx, `UPDATE scholarly_works SET source_id=$2,processing_status='queued',access_status='full_text_candidate',updated_at=now() WHERE id=$1 AND (source_id IS NULL OR source_id=$2)`, j.WorkID, sourceID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Worker) fetchCandidate(ctx context.Context, c Candidate, pmcid string) ([]byte, string, error) {
	step, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := w.Client
	if client == nil {
		client = &scholarly.Client{}
	}
	switch c.Resolver {
	case "europepmc":
		if c.Format != "jats" || pmcid == "" {
			return nil, "", errors.New("invalid Europe PMC candidate")
		}
		data, err := client.EuropeFullText(step, pmcid)
		return data, c.URL, err
	case "pmc_oai":
		if c.Format != "jats" || pmcid == "" {
			return nil, "", errors.New("invalid PMC candidate")
		}
		data, err := client.PMC(step, pmcid)
		return data, c.URL, err
	case "unpaywall":
		if c.Format != "pdf" && c.Format != "html" {
			return nil, "", errors.New("invalid alternate-copy format")
		}
		parsed, err := url.Parse(c.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return nil, "", errors.New("alternate-copy URL is not public HTTPS")
		}
		if w.fetchAlternate != nil {
			return w.fetchAlternate(step, c)
		}
		limit := int64(10 << 20)
		if c.Format == "pdf" {
			limit = 25 << 20
		}
		fetcher := w.Fetcher
		if fetcher == nil {
			fetcher = &safefetch.Fetcher{}
		}
		result, err := fetcher.FetchScoped(step, c.URL, limit, false, func(u *url.URL) error {
			if u.Scheme != "https" {
				return errors.New("alternate-copy redirect is not HTTPS")
			}
			return nil
		})
		if err != nil {
			return nil, "", fmt.Errorf("alternate-copy request failed: %w", err)
		}
		defer result.Body.Close()
		data, err := io.ReadAll(io.LimitReader(result.Body, limit+1))
		if err != nil || int64(len(data)) > limit {
			return nil, "", errors.New("alternate-copy body exceeded its limit")
		}
		return data, result.FinalURL, nil
	default:
		return nil, "", errors.New("unknown acquisition resolver")
	}
}

func validateCandidate(ctx context.Context, raw []byte, format, pmcid, pmid, doi, title string) error {
	switch format {
	case "jats":
		return validateArticle(raw, pmcid, pmid, doi)
	case "pdf":
		return validatePDFIdentity(ctx, raw, doi, title)
	case "html":
		return validateHTMLIdentity(raw, doi, title)
	default:
		return errors.New("unsupported candidate format")
	}
}

func (w *Worker) saveCheckpoint(raw []byte) (string, string, error) {
	return w.saveCheckpointFormat(raw, "jats")
}

func (w *Worker) saveCheckpointFormat(raw []byte, format string) (string, string, error) {
	if format != "jats" && format != "pdf" && format != "html" {
		return "", "", errors.New("unsupported checkpoint format")
	}
	hash := sha256.Sum256(raw)
	sha := hex.EncodeToString(hash[:])
	relative := filepath.Join("data/runtime/paper-jobs", sha+"."+format)
	path := filepath.Join(w.Store.Root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", "", err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(existing, raw) {
			return "", "", errors.New("checkpoint checksum collision")
		}
		return sha, relative, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".paper-*")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(temp.Name())
	if _, err = io.Copy(temp, bytes.NewReader(raw)); err != nil {
		temp.Close()
		return "", "", err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return "", "", err
	}
	if err = temp.Chmod(0400); err != nil {
		temp.Close()
		return "", "", err
	}
	if err = temp.Close(); err != nil {
		return "", "", err
	}
	if err = os.Rename(temp.Name(), path); err != nil {
		return "", "", err
	}
	return sha, relative, nil
}

func validateArticle(raw []byte, pmcid, pmid, doi string) error {
	if _, err := document.ExtractJATS(raw); err != nil {
		return err
	}
	var article struct {
		IDs []struct {
			Type  string `xml:"pub-id-type,attr"`
			Value string `xml:",chardata"`
		} `xml:"front>article-meta>article-id"`
		Licenses []struct {
			Text string `xml:",innerxml"`
		} `xml:"front>article-meta>permissions>license"`
	}
	if err := xml.Unmarshal(raw, &article); err != nil {
		return errors.New("malformed article XML")
	}
	foundPMCID, foundPMID, foundDOI := false, pmid == "", doi == ""
	for _, id := range article.IDs {
		v := strings.TrimSpace(id.Value)
		switch id.Type {
		case "pmc", "pmcid":
			foundPMCID = "PMC"+strings.TrimPrefix(strings.ToUpper(v), "PMC") == pmcid
		case "pmid":
			if v == pmid {
				foundPMID = true
			}
		case "doi":
			if strings.EqualFold(v, doi) {
				foundDOI = true
			}
		}
	}
	if !foundPMCID || !foundPMID || !foundDOI {
		return errors.New("article identifiers differ from saved reference")
	}
	licensed := false
	for _, v := range article.Licenses {
		text := strings.ToLower(v.Text)
		if strings.Contains(text, "creativecommons.org/licenses/by/4.0/") || strings.Contains(text, "creativecommons.org/publicdomain/zero/1.0/") {
			licensed = true
		}
	}
	if !licensed {
		return errors.New("article-level commercial reuse license is unverified; keep reference-only")
	}
	return nil
}
