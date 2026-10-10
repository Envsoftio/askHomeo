package pdfocr

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
)

// RenderDPI is shared by page OCR and bibliographic metadata OCR.
const RenderDPI = 300

var qaWords = regexp.MustCompile(`[a-z]{4,}`)

func Read(ctx context.Context, pdf string, index int) (string, error) {
	text, _, err := Check(ctx, pdf, index)
	return text, err
}
func Check(ctx context.Context, pdf string, pdfPageIndex int) (string, string, error) {
	if pdfPageIndex < 0 {
		return "", "", fmt.Errorf("invalid PDF page %d", pdfPageIndex)
	}
	dir, err := os.MkdirTemp("", "text-check-*")
	if err != nil {
		return "", "", fmt.Errorf("create OCR workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	image := filepath.Join(dir, "scan.png")
	page := strconv.Itoa(pdfPageIndex + 1)
	renderCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(renderCtx, "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=png16m", "-r"+strconv.Itoa(RenderDPI), "-dFirstPage="+page, "-dLastPage="+page, "-sOutputFile="+image, "-f", pdf)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("render PDF page %s: %w: %s", page, err, bytes.TrimSpace(out))
	}
	info, err := os.Stat(image)
	if err != nil {
		return "", "", fmt.Errorf("rendered scan missing: %w", err)
	}
	if info.Size() > 32<<20 {
		return "", "", fmt.Errorf("rendered scan %s is too large", page)
	}
	ocrCtx, stop := context.WithTimeout(ctx, 25*time.Second)
	defer stop()
	wordsFile := filepath.Join(dir, "domain-words.txt")
	if err := os.WriteFile(wordsFile, []byte("Materia\nMedica\n"), 0600); err != nil {
		return "", "", err
	}
	cmd = exec.CommandContext(ocrCtx, "tesseract", image, filepath.Join(dir, "ocr"), "-l", "eng", "--psm", "3", "--user-words", wordsFile, "txt", "tsv")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_, err = cmd.Output()
	out, readErr := os.ReadFile(filepath.Join(dir, "ocr.txt"))
	if err == nil {
		err = readErr
	}
	if err != nil {
		return "", "", fmt.Errorf("OCR PDF page %s: %w: %s", page, err, strings.TrimSpace(stderr.String()))
	}
	if len(out) > 1<<20 {
		return "", "", fmt.Errorf("OCR text for page %s is too large", page)
	}
	croppedUsed := false
	if len(qaWords.FindAllString(strings.ToLower(string(out)), -1)) < 40 {
		cropPath := filepath.Join(dir, "center.png")
		if err := cropCenter(image, cropPath); err == nil {
			cropCmd := exec.CommandContext(ocrCtx, "tesseract", cropPath, "stdout", "-l", "eng", "--psm", "3")
			cropCmd.Stderr = &stderr
			if cropped, cropErr := cropCmd.Output(); cropErr == nil && len(cropped) <= 1<<20 && len(qaWords.FindAllString(strings.ToLower(string(cropped)), -1)) > len(qaWords.FindAllString(strings.ToLower(string(out)), -1)) {
				out = cropped
				croppedUsed = true
			}
		}
	}
	warning := SuspiciousText(string(out))
	tsv, tsvErr := os.ReadFile(filepath.Join(dir, "ocr.tsv"))
	if tsvErr != nil {
		warning = "Word confidence is unavailable; compare this scan with the text."
	} else if lowConfidence(string(tsv)) {
		warning = "OCR found uncertain words. Compare the scan and correct or confirm the text."
	}
	if croppedUsed {
		warning = "OCR required a cropped scan; check all margins for missing text."
	}
	return string(out), warning, nil
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

// SuspiciousText detects broken tokens without treating unfamiliar remedy names as errors.
var brokenToken = regexp.MustCompile(`[[:alpha:]][(\[�][^[:space:]]*[[:alpha:]]|[[:alpha:]][^[:space:]]*[)\]�][[:alpha:]]`)

func SuspiciousText(text string) string {
	if strings.ContainsRune(text, '\ufffd') || brokenToken.MatchString(text) {
		return "Text contains malformed words; retry OCR or correct them against the scan."
	}
	return ""
}
func lowConfidence(tsv string) bool {
	words := 0
	for _, line := range strings.Split(tsv, "\n") {
		fields := strings.SplitN(line, "\t", 12)
		if len(fields) != 12 || fields[0] != "5" || strings.TrimSpace(fields[11]) == "" {
			continue
		}
		confidence, err := strconv.ParseFloat(fields[10], 64)
		if err != nil || confidence < 80 {
			return true
		}
		words++
	}
	return words == 0
}
