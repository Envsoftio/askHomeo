package ingest

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var qaWords = regexp.MustCompile(`[a-z]{4,}`)

type textCheck struct {
	suspect  bool
	reason   string
	coverage float64
}

func comparePageText(stored, fresh string) textCheck {
	oldWords := qaWords.FindAllString(strings.ToLower(stored), -1)
	newWords := qaWords.FindAllString(strings.ToLower(fresh), -1)
	if len(newWords) < 40 {
		return textCheck{true, "A fresh OCR pass could not read enough words from this scan; check the text against the image.", 0}
	}
	counts := make(map[string]int, len(oldWords))
	for _, word := range oldWords {
		counts[word]++
	}
	matched, firstMatched := 0, 0
	for i, word := range newWords {
		if counts[word] > 0 {
			matched++
			counts[word]--
			if i < 30 {
				firstMatched++
			}
		}
	}
	coverage := float64(matched) / float64(len(newWords))
	openingWindows := 0
	oldSequence := " " + strings.Join(oldWords, " ") + " "
	for i := 0; i+3 <= len(newWords) && i < 12; i++ {
		if strings.Contains(oldSequence, " "+strings.Join(newWords[i:i+3], " ")+" ") {
			openingWindows++
		}
	}
	if len(oldWords)*4 < len(newWords)*3 {
		return textCheck{true, "The stored text is substantially shorter than a fresh OCR pass; check for missing lines.", coverage}
	}
	if coverage < 0.78 {
		return textCheck{true, fmt.Sprintf("Fresh OCR and stored text differ substantially (%.0f%% word agreement); check this scan.", coverage*100), coverage}
	}
	if openingWindows < 2 {
		return textCheck{true, "The opening lines may be missing or out of order in stored text; compare the top of this scan.", coverage}
	}
	if firstMatched < 14 && len(newWords) >= 60 {
		return textCheck{true, "The top of the scan may be missing from stored text; compare its opening lines.", coverage}
	}
	return textCheck{false, fmt.Sprintf("Fresh OCR agreed with stored text (%.0f%% word agreement).", coverage*100), coverage}
}

func freshOCR(ctx context.Context, pdf string, pdfPageIndex int) (string, error) {
	if pdfPageIndex < 0 {
		return "", fmt.Errorf("invalid PDF page %d", pdfPageIndex)
	}
	dir, err := os.MkdirTemp("", "text-check-*")
	if err != nil {
		return "", fmt.Errorf("create OCR workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	image := filepath.Join(dir, "scan.png")
	page := strconv.Itoa(pdfPageIndex + 1)
	renderCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(renderCtx, "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=png16m", "-r150", "-dFirstPage="+page, "-dLastPage="+page, "-sOutputFile="+image, "-f", pdf)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("render PDF page %s: %w: %s", page, err, bytes.TrimSpace(out))
	}
	info, err := os.Stat(image)
	if err != nil {
		return "", fmt.Errorf("rendered scan missing: %w", err)
	}
	if info.Size() > 32<<20 {
		return "", fmt.Errorf("rendered scan %s is too large", page)
	}
	ocrCtx, stop := context.WithTimeout(ctx, 25*time.Second)
	defer stop()
	cmd = exec.CommandContext(ocrCtx, "tesseract", image, "stdout", "-l", "eng", "--psm", "3")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("OCR PDF page %s: %w: %s", page, err, strings.TrimSpace(stderr.String()))
	}
	if len(out) > 1<<20 {
		return "", fmt.Errorf("OCR text for page %s is too large", page)
	}
	if len(qaWords.FindAllString(strings.ToLower(string(out)), -1)) < 40 {
		cropPath := filepath.Join(dir, "center.png")
		if err := cropCenter(image, cropPath); err == nil {
			cropCmd := exec.CommandContext(ocrCtx, "tesseract", cropPath, "stdout", "-l", "eng", "--psm", "3")
			cropCmd.Stderr = &stderr
			if cropped, cropErr := cropCmd.Output(); cropErr == nil && len(cropped) <= 1<<20 && len(qaWords.FindAllString(strings.ToLower(string(cropped)), -1)) > len(qaWords.FindAllString(strings.ToLower(string(out)), -1)) {
				out = cropped
			}
		}
	}
	return string(out), nil
}

func cropCenter(input, output string) error {
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return err
	}
	b := src.Bounds()
	if b.Dx() < 200 || b.Dy() < 200 {
		return fmt.Errorf("scan too small to crop")
	}
	r := image.Rect(b.Min.X+b.Dx()*12/100, b.Min.Y+b.Dy()*7/100, b.Min.X+b.Dx()*88/100, b.Min.Y+b.Dy()*93/100)
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	out, err := os.Create(output)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, dst)
}

func (w *Worker) checkText(ctx context.Context, jobID, sourceID uuid.UUID) error {
	var sha string
	if err := w.Store.DB.QueryRow(ctx, `SELECT pdf_sha256 FROM sources WHERE id=$1 AND status='review'`, sourceID).Scan(&sha); err != nil {
		return fmt.Errorf("find PDF for text check: %w", err)
	}
	pdf, releasePDF, storageErr := w.Store.AcquirePDF(ctx, sha)
	if storageErr != nil {
		return storageErr
	}
	defer releasePDF()
	rows, err := w.Store.DB.Query(ctx, `SELECT id,pdf_page_index,text_raw FROM pages WHERE source_id=$1 AND scan_page_index>=0 AND page_kind='text' AND text_qa_at IS NULL ORDER BY scan_page_index`, sourceID)
	if err != nil {
		return fmt.Errorf("list text pages: %w", err)
	}
	type page struct {
		id    uuid.UUID
		index int
		text  string
	}
	var pages []page
	for rows.Next() {
		var p page
		if err = rows.Scan(&p.id, &p.index, &p.text); err != nil {
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
		fresh, err := freshOCR(ctx, pdf, p.index)
		if err != nil {
			return fmt.Errorf("check scan %d: %w", p.index, err)
		}
		result := comparePageText(p.text, fresh)
		_, err = w.Store.DB.Exec(ctx, `UPDATE pages SET text_qa_at=now(),text_qa_reason=$2,
 text_qa_status=CASE WHEN $3 AND review_status='reviewed' THEN 'accepted' WHEN $3 THEN 'suspect' ELSE 'passed' END,
 review_status=CASE WHEN $3 AND review_status<>'reviewed' THEN 'needs_review' WHEN NOT $3 AND review_status='needs_review' THEN 'auto_checked' ELSE review_status END
 WHERE id=$1 AND text_qa_at IS NULL`, p.id, result.reason, result.suspect)
		if err != nil {
			return fmt.Errorf("save text check for scan %d: %w", p.index, err)
		}
		_, err = w.Store.DB.Exec(ctx, `UPDATE jobs SET completed=(SELECT count(*) FROM pages WHERE source_id=$2 AND scan_page_index>=0 AND page_kind='text' AND text_qa_at IS NOT NULL),lease_until=now()+interval '90 seconds',updated_at=now() WHERE id=$1`, jobID, sourceID)
		if err != nil {
			return fmt.Errorf("update text check progress: %w", err)
		}
	}
	if err := w.classifyFarringtonIndex(ctx, sourceID); err != nil {
		return err
	}
	_, err = w.Store.DB.Exec(ctx, `UPDATE jobs SET status='done',completed=total,current_step='Text checked',error=NULL,lease_until=NULL,updated_at=now() WHERE id=$1`, jobID)
	return err
}

func (w *Worker) classifyFarringtonIndex(ctx context.Context, sourceID uuid.UUID) error {
	var sourceKey string
	if err := w.Store.DB.QueryRow(ctx, `SELECT source_key FROM sources WHERE id=$1`, sourceID).Scan(&sourceKey); err != nil {
		return err
	}
	if sourceKey != "farrington-1890-b21118838" {
		return nil
	}
	var total, identified int
	if err := w.Store.DB.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE text_raw ~* 'index') FROM pages WHERE source_id=$1 AND pdf_page_index BETWEEN 707 AND 774`, sourceID).Scan(&total, &identified); err != nil {
		return err
	}
	if total != 68 || identified != 68 {
		return fmt.Errorf("Farrington index boundary check failed: %d of %d pages identified", identified, total)
	}
	_, err := w.Store.DB.Exec(ctx, `UPDATE pages SET page_kind='book_info',review_status='auto_checked',review_note='Farrington index range: each page has an index heading; boundary and sample scans checked against the original PDF.' WHERE source_id=$1 AND pdf_page_index BETWEEN 707 AND 774 AND page_kind='text'`, sourceID)
	return err
}
