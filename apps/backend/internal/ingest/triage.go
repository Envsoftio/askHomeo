package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type scanMetrics struct{ dark, edges float64 }

// A blank decision is intentionally narrow. Any meaningful mark, texture,
// handwriting, cover, or diagram stays in the review queue.
func (m scanMetrics) blank() bool { return m.dark <= 0.0002 && m.edges <= 0.0067 }

func measurePGM(data []byte) (scanMetrics, error) {
	pos := 0
	next := func() string {
		for pos < len(data) {
			if data[pos] == '#' {
				for pos < len(data) && data[pos] != '\n' {
					pos++
				}
			} else if data[pos] == ' ' || data[pos] == '\r' || data[pos] == '\n' || data[pos] == '\t' {
				pos++
			} else {
				break
			}
		}
		start := pos
		for pos < len(data) && data[pos] != ' ' && data[pos] != '\r' && data[pos] != '\n' && data[pos] != '\t' {
			pos++
		}
		return string(data[start:pos])
	}
	if next() != "P5" {
		return scanMetrics{}, errors.New("scan render is not PGM")
	}
	w, err := strconv.Atoi(next())
	if err != nil {
		return scanMetrics{}, fmt.Errorf("PGM width: %w", err)
	}
	h, err := strconv.Atoi(next())
	if err != nil {
		return scanMetrics{}, fmt.Errorf("PGM height: %w", err)
	}
	if next() != "255" || w < 1 || h < 1 || w*h > 8<<20 || pos >= len(data) {
		return scanMetrics{}, errors.New("invalid scan render dimensions")
	}
	if data[pos] != '\n' && data[pos] != ' ' && data[pos] != '\r' {
		return scanMetrics{}, errors.New("invalid PGM header")
	}
	pos++
	if len(data)-pos != w*h {
		return scanMetrics{}, errors.New("incomplete scan render")
	}
	pixels := data[pos:]
	var dark, edges int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if pixels[i] < 100 {
				dark++
			}
			if x > 0 && y > 0 && (absDiff(pixels[i], pixels[i-1]) > 25 || absDiff(pixels[i], pixels[i-w]) > 25) {
				edges++
			}
		}
	}
	n := float64(w * h)
	return scanMetrics{float64(dark) / n, float64(edges) / n}, nil
}

func absDiff(a, b byte) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func renderMetrics(ctx context.Context, pdf string, pdfPageIndex int) (scanMetrics, error) {
	if pdfPageIndex < 0 {
		return scanMetrics{}, errors.New("invalid PDF page")
	}
	dir, err := os.MkdirTemp("", "scan-triage-*")
	if err != nil {
		return scanMetrics{}, fmt.Errorf("create scan workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "scan.pgm")
	page := strconv.Itoa(pdfPageIndex + 1)
	jobCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(jobCtx, "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pgmraw", "-r40", "-dFirstPage="+page, "-dLastPage="+page, "-sOutputFile="+output, "-f", pdf)
	if out, err := cmd.CombinedOutput(); err != nil {
		return scanMetrics{}, fmt.Errorf("render PDF page %s: %w: %s", page, err, bytes.TrimSpace(out))
	}
	info, err := os.Stat(output)
	if err != nil {
		return scanMetrics{}, fmt.Errorf("read rendered scan: %w", err)
	}
	if info.Size() > 8<<20 {
		return scanMetrics{}, errors.New("rendered scan too large")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return scanMetrics{}, fmt.Errorf("read rendered scan: %w", err)
	}
	return measurePGM(data)
}

func (w *Worker) triage(ctx context.Context, jobID, sourceID uuid.UUID) error {
	var sha string
	if err := w.Store.DB.QueryRow(ctx, `SELECT pdf_sha256 FROM sources WHERE id=$1 AND status='review'`, sourceID).Scan(&sha); err != nil {
		return fmt.Errorf("find PDF for scan triage: %w", err)
	}
	pdf, releasePDF, storageErr := w.Store.AcquirePDF(ctx, sha)
	if storageErr != nil {
		return storageErr
	}
	defer releasePDF()
	rows, err := w.Store.DB.Query(ctx, `SELECT id,pdf_page_index FROM pages WHERE source_id=$1 AND scan_page_index>=0 AND length(trim(text_raw))<10 AND triaged_at IS NULL ORDER BY scan_page_index`, sourceID)
	if err != nil {
		return fmt.Errorf("list scans for triage: %w", err)
	}
	type page struct {
		id    uuid.UUID
		index int
	}
	var pages []page
	for rows.Next() {
		var p page
		if err = rows.Scan(&p.id, &p.index); err != nil {
			rows.Close()
			return err
		}
		pages = append(pages, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range pages {
		metrics, err := renderMetrics(ctx, pdf, p.index)
		if err != nil {
			return fmt.Errorf("scan %d: %w", p.index, err)
		}
		blank := metrics.blank()
		reason := "Visual content detected; check whether this is a cover, diagram, book information, or missing text."
		if blank {
			reason = "No meaningful marks detected in the rendered scan; automatically omitted from answers."
		}
		_, err = w.Store.DB.Exec(ctx, `UPDATE pages SET triaged_at=now(),triage_reason=$2,
 page_kind=CASE WHEN $3 AND page_kind='blank' THEN 'unclassified' WHEN NOT $3 AND page_kind='unclassified' THEN 'blank' ELSE page_kind END,
 review_status=CASE WHEN $3 AND page_kind='blank' THEN 'needs_review' WHEN NOT $3 AND page_kind='unclassified' THEN 'auto_checked' ELSE review_status END
 WHERE id=$1 AND triaged_at IS NULL`, p.id, reason, !blank)
		if err != nil {
			return fmt.Errorf("save scan %d triage: %w", p.index, err)
		}
		if _, err = w.Store.DB.Exec(ctx, `UPDATE jobs SET completed=(SELECT count(*) FROM pages WHERE source_id=$2 AND triaged_at IS NOT NULL AND scan_page_index>=0 AND length(trim(text_raw))<10),lease_until=now()+interval '90 seconds',updated_at=now() WHERE id=$1`, jobID, sourceID); err != nil {
			return err
		}
	}
	_, err = w.Store.DB.Exec(ctx, `UPDATE jobs SET status='done',completed=total,current_step='Scans checked',error=NULL,lease_until=NULL,updated_at=now() WHERE id=$1`, jobID)
	return err
}
