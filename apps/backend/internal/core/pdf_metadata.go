package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type pdfMetadata struct {
	Title, Author, Edition, Publication, Repository, SourceURL string
}

var editionPattern = regexp.MustCompile(`(?i)\b(first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth|[1-9][0-9]?(?:st|nd|rd|th))\s+(?:revised\s+)?edition\b`)
var volumePattern = regexp.MustCompile(`(?i)\b(?:vol\.?|volume)\s+([ivxlcdm]+|[1-9][0-9]?)\b`)
var authorLine = regexp.MustCompile(`(?i)\bby\s+(?:the\s+late\s+)?([A-Z][A-Za-z. '\-]+)`)

// detectPDFMetadata uses the PDF's opening text and a small OCR fallback. It is
// deliberately conservative: unknown fields remain empty for human correction.
func detectPDFMetadata(ctx context.Context, pdf string, pages int, useOCR bool) pdfMetadata {
	checkCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "pdf-metadata-*")
	if err != nil {
		return pdfMetadata{}
	}
	defer os.RemoveAll(dir)
	textFile := filepath.Join(dir, "front.txt")
	last := pages
	if last > 6 {
		last = 6
	}
	cmd := exec.CommandContext(checkCtx, "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=txtwrite", "-dFirstPage=1", "-dLastPage="+strconv.Itoa(last), "-sOutputFile="+textFile, "-f", pdf)
	if err := cmd.Run(); err == nil {
		if b, err := os.ReadFile(textFile); err == nil && len(b) <= 128<<10 {
			found := parsePDFMetadataText(string(b))
			if found.Title != "" && found.Author != "" && found.Edition != "" {
				return found
			}
		}
	}
	var found pdfMetadata
	if b, err := os.ReadFile(textFile); err == nil && len(b) <= 128<<10 {
		found = parsePDFMetadataText(string(b))
	}
	if !useOCR {
		return found
	}
	ocrPages := pages
	if ocrPages > 4 {
		ocrPages = 4
	}
	for page := 1; page <= ocrPages && checkCtx.Err() == nil; page++ {
		image := filepath.Join(dir, "page-"+strconv.Itoa(page)+".png")
		cmd = exec.CommandContext(checkCtx, "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pnggray", "-r120", "-dFirstPage="+strconv.Itoa(page), "-dLastPage="+strconv.Itoa(page), "-sOutputFile="+image, "-f", pdf)
		if cmd.Run() != nil {
			continue
		}
		cmd = exec.CommandContext(checkCtx, "tesseract", image, "stdout", "-l", "eng", "--psm", "3")
		out, err := cmd.Output()
		if err != nil || len(out) > 128<<10 {
			continue
		}
		candidate := parsePDFMetadataText(string(out))
		if found.Title == "" {
			found.Title = candidate.Title
		}
		if found.Author == "" {
			found.Author = candidate.Author
		}
		if found.Edition == "" {
			found.Edition = candidate.Edition
		}
		if found.Title != "" && found.Author != "" && found.Edition != "" {
			break
		}
	}
	return found
}

func parsePDFMetadataText(raw string) pdfMetadata {
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		line = strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	var found pdfMetadata
	if journal := parseJournalCover(lines); journal.Title != "" && journal.Author != "" {
		return journal
	}
	for i, line := range lines {
		switch line {
		case "Contributors":
			if i > 0 && i < 8 {
				name := strings.Join(lines[:i], " ")
				if p := strings.Index(name, " / by "); p >= 0 {
					name = name[:p]
				}
				found.Title = cleanDetectedText(name, 300)
			}
			if i+1 < len(lines) {
				found.Author = cleanContributor(lines[i+1])
			}
		case "Publication/Creation":
			if i+1 < len(lines) {
				found.Publication = cleanDetectedText(lines[i+1], 500)
			}
		case "Persistent URL":
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "https://wellcomecollection.org/works/") {
				found.SourceURL = lines[i+1]
				found.Repository = "Wellcome Collection"
			}
		}
	}
	if match := editionPattern.FindString(raw); match != "" {
		found.Edition = cleanDetectedText(match, 100)
	} else if match := volumePattern.FindStringSubmatch(raw); len(match) > 1 {
		found.Edition = "Volume " + strings.ToUpper(match[1])
	}
	if found.Title != "" && found.Author != "" {
		return found
	}
	for i, line := range lines {
		if found.Author == "" {
			if match := authorLine.FindStringSubmatch(line); len(match) > 1 {
				found.Author = cleanDetectedText(match[1], 300)
			} else if strings.EqualFold(line, "by") && i+1 < len(lines) {
				found.Author = cleanDetectedText(lines[i+1], 300)
			}
		}
		if found.Title == "" && i < 10 && len(line) >= 8 && len(line) <= 120 && !strings.Contains(strings.ToLower(line), "http") && !strings.HasPrefix(strings.ToLower(line), "digitized") && !strings.HasPrefix(strings.ToLower(line), "google") && !strings.EqualFold(line, "by") && !strings.HasPrefix(strings.ToLower(line), "by ") && !strings.HasPrefix(strings.ToLower(line), "copyright") && !strings.HasPrefix(strings.ToLower(line), "library") {
			found.Title = cleanDetectedText(line, 300)
		}
	}
	return found
}

func parseJournalCover(lines []string) pdfMetadata {
	var found pdfMetadata
	start, abstract := -1, -1
	for i, line := range lines {
		if i < 12 && strings.Contains(strings.ToLower(line), "research") && strings.Contains(strings.ToLower(line), "open access") {
			start = i
		}
		if start >= 0 && i > start && i < start+15 && strings.EqualFold(line, "Abstract") {
			abstract = i
			break
		}
	}
	if start < 0 || abstract < 0 || abstract-start < 3 {
		return found
	}
	var titleLines []string
	for _, line := range lines[start+1 : abstract] {
		if len(titleLines) >= 6 || regexp.MustCompile(`^[0-9*,\s]+$`).MatchString(line) || strings.Contains(line, "@") {
			continue
		}
		if strings.Contains(line, ",") && strings.Contains(line, " and ") {
			found.Author = cleanDetectedText(regexp.MustCompile(`\s+[0-9]+(?:,[0-9]+)*\*?`).ReplaceAllString(line, ""), 300)
			break
		}
		titleLines = append(titleLines, line)
	}
	found.Title = cleanDetectedText(strings.Join(titleLines, " "), 300)
	for _, line := range lines[:start] {
		if strings.Contains(line, "Systematic Reviews") && strings.Contains(line, "(20") {
			found.Publication = cleanDetectedText(strings.TrimPrefix(line, "et al. "), 500)
			found.Repository = "Systematic Reviews / BMC"
		}
		if strings.HasPrefix(line, "https://doi.org/") {
			found.SourceURL = line
		}
	}
	return found
}

func cleanDetectedText(s string, limit int) string {
	s = strings.Trim(strings.Join(strings.Fields(s), " "), " .;,:")
	if len([]rune(s)) > limit {
		return ""
	}
	return s
}

func cleanContributor(line string) string {
	parts := strings.SplitN(line, ",", 2)
	if len(parts) != 2 {
		return cleanDetectedText(line, 300)
	}
	last := strings.TrimSpace(parts[0])
	first := strings.TrimSpace(parts[1])
	first = regexp.MustCompile(`\b[12][0-9]{3}(?:-[0-9]{0,4})?\.?$`).ReplaceAllString(first, "")
	return cleanDetectedText(first+" "+last, 300)
}
