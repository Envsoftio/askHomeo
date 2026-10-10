package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/localllm"
	"homeopath-poc/backend/internal/pdfocr"
)

type Worker struct {
	Store  *core.Store
	Client *http.Client
	Model  *localllm.Client
}

func New(s *core.Store, model ...*localllm.Client) *Worker {
	var m *localllm.Client
	if len(model) > 0 {
		m = model[0]
	}
	return &Worker{Store: s, Model: m, Client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 2 || r.URL.Scheme != "https" || r.URL.Hostname() != "api.wellcomecollection.org" {
			return errors.New("OCR redirect blocked")
		}
		return nil
	}}}
}

func (w *Worker) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// File cleanup must continue while a large book is being OCR-checked.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.Store.CleanupDeletedFile(ctx); err != nil && ctx.Err() == nil {
					log.Printf("source file cleanup: %v", err)
				}
			}
		}
	}()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := w.once(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (w *Worker) once(ctx context.Context) error {
	if err := w.reconcileExpiredJobs(ctx); err != nil {
		return err
	}
	var jobID, sourceID uuid.UUID
	var kind string
	err := w.Store.DB.QueryRow(ctx, `UPDATE jobs SET status='running',attempts=attempts+1,lease_until=now()+interval '90 seconds',updated_at=now()
 WHERE id=(SELECT id FROM jobs WHERE kind IN ('ingest','triage','text_qa','embed') AND attempts<5 AND run_after<=now() AND (status='queued' OR (status='running' AND lease_until<now())) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
 RETURNING id,source_id,kind`).Scan(&jobID, &sourceID, &kind)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil
		}
		return fmt.Errorf("claim job: %w", err)
	}
	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = w.Store.DB.Exec(ctx, `UPDATE jobs SET lease_until=now()+interval '90 seconds',updated_at=now() WHERE id=$1 AND status='running'`, jobID)
			}
		}
	}()
	if kind == "ingest" {
		_, _ = w.Store.DB.Exec(ctx, `UPDATE sources SET status='processing',updated_at=now() WHERE id=$1 AND status IN ('queued','failed','processing')`, sourceID)
		_, _ = w.Store.DB.Exec(ctx, `UPDATE processing_revisions SET status='processing' WHERE id=(SELECT current_revision_id FROM sources WHERE id=$1) AND status IN ('queued','failed','processing')`, sourceID)
		err = w.process(ctx, jobID, sourceID)
	} else if kind == "triage" {
		err = w.triage(ctx, jobID, sourceID)
	} else if kind == "text_qa" {
		err = w.checkText(ctx, jobID, sourceID)
	} else {
		err = w.embed(ctx, jobID, sourceID)
	}
	close(stopHeartbeat)
	if err != nil {
		attempts := 0
		_ = w.Store.DB.QueryRow(ctx, `SELECT attempts FROM jobs WHERE id=$1`, jobID).Scan(&attempts)
		state := "queued"
		sourceState := "processing"
		if attempts >= 5 {
			state = "failed"
			sourceState = "failed"
		}
		_, _ = w.Store.DB.Exec(ctx, `UPDATE jobs SET status=$2,error=$3,run_after=now()+($4::int * interval '1 minute'),lease_until=NULL,updated_at=now() WHERE id=$1`, jobID, state, err.Error(), attempts*attempts)
		if kind == "embed" {
			_, _ = w.Store.DB.Exec(ctx, `UPDATE index_runs SET status=CASE WHEN $2='failed' THEN 'failed' ELSE 'pending' END,error=$3 WHERE id=(SELECT index_run_id FROM embedding_jobs WHERE job_id=$1)`, jobID, state, err.Error())
		}
		if kind == "ingest" {
			_, _ = w.Store.DB.Exec(ctx, `UPDATE sources SET status=$2,updated_at=now() WHERE id=$1`, sourceID, sourceState)
			_, _ = w.Store.DB.Exec(ctx, `UPDATE processing_revisions SET status=$2 WHERE id=(SELECT current_revision_id FROM sources WHERE id=$1)`, sourceID, sourceState)
		}
	}
	return nil
}

func (w *Worker) reconcileExpiredJobs(ctx context.Context) error {
	rows, err := w.Store.DB.Query(ctx, `UPDATE jobs SET status='failed',error=coalesce(error,'The preparation worker stopped before this step finished.'),lease_until=NULL,updated_at=now() WHERE status='running' AND attempts>=5 AND lease_until<now() RETURNING source_id,kind,id`)
	if err != nil {
		return fmt.Errorf("reconcile source jobs: %w", err)
	}
	type expired struct {
		source, job uuid.UUID
		kind        string
	}
	items := []expired{}
	for rows.Next() {
		var item expired
		if err = rows.Scan(&item.source, &item.kind, &item.job); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.kind == "ingest" {
			_, err = w.Store.DB.Exec(ctx, `UPDATE sources SET status='failed',updated_at=now() WHERE id=$1 AND status='processing'`, item.source)
			if err == nil {
				_, err = w.Store.DB.Exec(ctx, `UPDATE processing_revisions SET status='failed' WHERE id=(SELECT current_revision_id FROM sources WHERE id=$1) AND status='processing'`, item.source)
			}
		} else if item.kind == "embed" {
			_, err = w.Store.DB.Exec(ctx, `UPDATE index_runs SET status='failed',error='The preparation worker stopped before this step finished.' WHERE id=(SELECT index_run_id FROM embedding_jobs WHERE job_id=$1) AND status='running'`, item.job)
		}
		if err != nil {
			return fmt.Errorf("mark interrupted source job: %w", err)
		}
	}
	return nil
}
func (w *Worker) process(ctx context.Context, jobID, sourceID uuid.UUID) error {
	var sourceKey, format string
	var pdfSHA *string
	if err := w.Store.DB.QueryRow(ctx, `SELECT source_key,pdf_sha256,document_format FROM sources WHERE id=$1`, sourceID).Scan(&sourceKey, &pdfSHA, &format); err != nil {
		return err
	}
	if format != "pdf" {
		return w.processDocument(ctx, jobID, sourceID)
	}
	if err := w.recordRevisionTools(ctx, sourceID); err != nil {
		return err
	}
	if strings.HasPrefix(sourceKey, "upload-") {
		return w.processLocal(ctx, jobID, sourceID)
	}
	seed, err := w.Store.Starter(sourceKey)
	if err != nil {
		return err
	}
	if seed.SourceKey != sourceKey || len(seed.Assets) == 0 {
		return fmt.Errorf("starter manifest does not match source %s", sourceKey)
	}
	pdfMatched := false
	for _, asset := range seed.Assets {
		if asset.Kind == "pdf" && pdfSHA != nil && asset.SHA256 == *pdfSHA {
			pdfMatched = true
		}
	}
	if !pdfMatched {
		return fmt.Errorf("starter PDF checksum does not match source %s", sourceKey)
	}
	mapping, err := w.Store.PageMap(seed)
	if err != nil {
		return err
	}
	if len(mapping.Pages) != seed.PDFPageCount-1 {
		return fmt.Errorf("page map has %d scans; expected %d", len(mapping.Pages), seed.PDFPageCount-1)
	}
	_, err = w.Store.DB.Exec(ctx, `INSERT INTO pages(id,source_id,pdf_page_index,scan_page_index,canvas_id,alto_url,image_url,text_raw,text_sha256,extraction_method,review_status,page_kind)
 VALUES($1,$2,0,-1,'','','','','e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855','generated cover','excluded','book_info') ON CONFLICT(source_id,pdf_page_index) DO NOTHING`, uuid.New(), sourceID)
	if err != nil {
		return err
	}
	for _, p := range mapping.Pages {
		var exists bool
		if err = w.Store.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pages WHERE source_id=$1 AND pdf_page_index=$2)`, sourceID, p.PDFPageIndex).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		raw, err := w.fetch(ctx, p.ALTOURL)
		if err != nil {
			return fmt.Errorf("PDF page %d: %w", p.PDFPageIndex, err)
		}
		if err = w.savePage(ctx, jobID, sourceID, p, raw); err != nil {
			return fmt.Errorf("save PDF page %d: %w", p.PDFPageIndex, err)
		}
	}
	return w.finishIngest(ctx, jobID, sourceID)
}

func (w *Worker) recordRevisionTools(ctx context.Context, sourceID uuid.UUID) error {
	var recorded bool
	if err := w.Store.DB.QueryRow(ctx, `SELECT component_versions_json ? 'ghostscript' FROM processing_revisions WHERE id=(SELECT current_revision_id FROM sources WHERE id=$1)`, sourceID).Scan(&recorded); err != nil {
		return err
	}
	if recorded {
		return nil
	}
	versions := map[string]string{"ingest": "local-v1", "page_qa": "confidence-v2", "ocr_language": "eng"}
	for _, tool := range []struct{ key, binary, arg string }{{"ghostscript", "gs", "--version"}, {"tesseract", "tesseract", "--version"}} {
		step, cancel := context.WithTimeout(ctx, 5*time.Second)
		output, err := exec.CommandContext(step, tool.binary, tool.arg).Output()
		cancel()
		if err != nil {
			return fmt.Errorf("read %s version: %w", tool.binary, err)
		}
		versions[tool.key] = strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
	}
	config := map[string]any{"text_extraction": "gs txtwrite", "ocr": "tesseract eng psm3", "qa_render_dpi": pdfocr.RenderDPI, "blank_render_dpi": 40, "chunking": "page-safe-v1"}
	configBytes, _ := json.Marshal(config)
	configHash := sha256.Sum256(configBytes)
	config["processing_config_sha256"] = hex.EncodeToString(configHash[:])
	versionBytes, _ := json.Marshal(versions)
	configBytes, _ = json.Marshal(config)
	_, err := w.Store.DB.Exec(ctx, `UPDATE processing_revisions SET component_versions_json=$2::jsonb,configuration_json=configuration_json||$3::jsonb WHERE id=(SELECT current_revision_id FROM sources WHERE id=$1) AND status IN ('queued','processing') AND NOT (component_versions_json ? 'ghostscript')`, sourceID, versionBytes, configBytes)
	return err
}
func (w *Worker) finishIngest(ctx context.Context, jobID, sourceID uuid.UUID) error {
	_, err := w.Store.DB.Exec(ctx, `INSERT INTO jobs(id,source_id,kind,status,current_step,total)
 SELECT $1,$2,'triage','queued','Checking scans without text',count(*)::int FROM pages
 WHERE source_id=$2 AND scan_page_index>=0 AND length(trim(text_raw))<10 HAVING count(*)>0
 ON CONFLICT(source_id,kind) DO NOTHING`, uuid.New(), sourceID)
	if err != nil {
		return fmt.Errorf("enqueue scan triage: %w", err)
	}
	_, err = w.Store.DB.Exec(ctx, `INSERT INTO jobs(id,source_id,kind,status,current_step,total)
 SELECT $1,$2,'text_qa','queued','Checking text against scans',count(*)::int FROM pages
 WHERE source_id=$2 AND scan_page_index>=0 AND page_kind='text' HAVING count(*)>0
 ON CONFLICT(source_id,kind) DO NOTHING`, uuid.New(), sourceID)
	if err != nil {
		return fmt.Errorf("enqueue text check: %w", err)
	}
	_, err = w.Store.DB.Exec(ctx, `UPDATE jobs SET status='done',current_step='Review pages',completed=total,lease_until=NULL,error=NULL,updated_at=now() WHERE id=$1`, jobID)
	if err != nil {
		return err
	}
	_, err = w.Store.DB.Exec(ctx, `UPDATE sources SET status='review',updated_at=now() WHERE id=$1`, sourceID)
	if err != nil {
		return err
	}
	_, err = w.Store.DB.Exec(ctx, `UPDATE processing_revisions SET status='review',completed_at=now() WHERE id=(SELECT current_revision_id FROM sources WHERE id=$1)`, sourceID)
	if err == nil {
		w.suggestPDFCategories(ctx, sourceID)
	}
	return err
}
func (w *Worker) processLocal(ctx context.Context, jobID, sourceID uuid.UUID) error {
	var sha string
	var count int
	if err := w.Store.DB.QueryRow(ctx, `SELECT pdf_sha256,page_count-1 FROM sources WHERE id=$1`, sourceID).Scan(&sha, &count); err != nil {
		return err
	}
	path, releasePDF, storageErr := w.Store.AcquirePDF(ctx, sha)
	if storageErr != nil {
		return storageErr
	}
	defer releasePDF()
	for i := 0; i < count; i++ {
		var exists bool
		if err := w.Store.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pages WHERE source_id=$1 AND pdf_page_index=$2)`, sourceID, i).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		raw, method, err := extractLocalPage(ctx, path, i)
		if err != nil {
			return fmt.Errorf("PDF page %d: %w", i+1, err)
		}
		p := core.MappedPage{PDFPageIndex: i, ScanPageIndex: i, CanvasID: fmt.Sprintf("local-%d", i), ImageURL: fmt.Sprintf("/api/v1/sources/%s/pages/%d/image", sourceID, i)}
		p.PrintedLabel = guessPrintedLabel(raw)
		if err := w.savePage(ctx, jobID, sourceID, p, raw); err != nil {
			return err
		}
		_, err = w.Store.DB.Exec(ctx, `UPDATE pages SET extraction_method=$3 WHERE source_id=$1 AND pdf_page_index=$2`, sourceID, i, method)
		if err != nil {
			return err
		}
	}
	return w.finishIngest(ctx, jobID, sourceID)
}

var printedPagePattern = regexp.MustCompile(`(?i)^(?:[0-9]{1,4}|[ivxlcdm]{1,12})$`)

func guessPrintedLabel(raw string) *string {
	lines := strings.Split(raw, "\n")
	if len(lines) < 3 {
		return nil
	}
	for _, line := range []string{lines[0], lines[len(lines)-1]} {
		candidate := strings.TrimSpace(line)
		if printedPagePattern.MatchString(candidate) {
			return &candidate
		}
	}
	return nil
}
func extractLocalPage(ctx context.Context, pdf string, index int) (string, string, error) {
	dir, err := os.MkdirTemp("", "pdf-extract-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "page.txt")
	page := strconv.Itoa(index + 1)
	step, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(step, "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=txtwrite", "-dFirstPage="+page, "-dLastPage="+page, "-sOutputFile="+output, "-f", pdf)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("extract PDF text: %w: %s", err, strings.TrimSpace(string(out)))
	}
	b, err := os.ReadFile(output)
	if err != nil {
		return "", "", err
	}
	if len(b) > 1<<20 {
		return "", "", errors.New("page text is too large")
	}
	raw := strings.TrimSpace(string(b))
	if len([]rune(raw)) >= 80 && pdfocr.SuspiciousText(raw) == "" {
		return raw, "PDF text", nil
	}
	ocr, err := freshOCR(ctx, pdf, index)
	if err != nil {
		return "", "", err
	}
	if len([]rune(strings.TrimSpace(ocr))) > len([]rune(raw)) || (pdfocr.SuspiciousText(raw) != "" && strings.TrimSpace(ocr) != "") {
		return strings.TrimSpace(ocr), "Tesseract OCR", nil
	}
	return raw, "PDF text", nil
}
func (w *Worker) fetch(ctx context.Context, address string) (string, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() != "api.wellcomecollection.org" || !strings.HasPrefix(u.Path, "/text/alto/") {
		return "", errors.New("unapproved OCR URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", err
	}
	res, err := w.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("OCR HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return ParseALTO(b)
}
func ParseALTO(b []byte) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	var out strings.Builder
	line := false
	first := true
	for {
		t, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse ALTO: %w", err)
		}
		switch v := t.(type) {
		case xml.StartElement:
			if v.Name.Local == "TextLine" {
				line = true
				first = true
			}
			if line && v.Name.Local == "String" {
				for _, a := range v.Attr {
					if a.Name.Local == "CONTENT" {
						if !first {
							out.WriteByte(' ')
						}
						out.WriteString(a.Value)
						first = false
						break
					}
				}
			}
		case xml.EndElement:
			if v.Name.Local == "TextLine" {
				out.WriteByte('\n')
				line = false
			}
		}
	}
	return strings.TrimSpace(out.String()), nil
}
func (w *Worker) savePage(ctx context.Context, jobID, sourceID uuid.UUID, p core.MappedPage, raw string) error {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	pageID := uuid.New()
	digest := sha256.Sum256([]byte(raw))
	_, err = tx.Exec(ctx, `INSERT INTO pages(id,source_id,pdf_page_index,scan_page_index,printed_label,canvas_id,alto_url,image_url,text_raw,text_sha256,extraction_method)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'Wellcome ALTO') ON CONFLICT(source_id,pdf_page_index) DO NOTHING`, pageID, sourceID, p.PDFPageIndex, p.ScanPageIndex, p.PrintedLabel, p.CanvasID, p.ALTOURL, p.ImageURL, raw, hex.EncodeToString(digest[:]))
	if err != nil {
		return err
	}
	// Unicode code-point offsets refer directly to the immutable OCR transcription.
	runes := []rune(raw)
	start := 0
	index := 0
	for start < len(runes) {
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
		if start >= len(runes) {
			break
		}
		end := start + 700
		if end >= len(runes) {
			end = len(runes)
		} else {
			for end > start+350 && !unicode.IsSpace(runes[end-1]) {
				end--
			}
		}
		for end > start && unicode.IsSpace(runes[end-1]) {
			end--
		}
		if end <= start {
			break
		}
		exact := string(runes[start:end])
		_, err = tx.Exec(ctx, `INSERT INTO chunks(id,page_id,source_id,chunk_index,text_exact,start_character,end_character) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(page_id,chunk_index) DO NOTHING`, uuid.New(), pageID, sourceID, index, exact, start, end)
		if err != nil {
			return err
		}
		index++
		start = end
	}
	_, err = tx.Exec(ctx, `UPDATE jobs SET completed=(SELECT count(*) FROM pages WHERE source_id=$2 AND scan_page_index>=0)+1,lease_until=now()+interval '90 seconds',updated_at=now() WHERE id=$1`, jobID, sourceID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
