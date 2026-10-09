package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"homeopath-poc/backend/internal/safefetch"
)

var htmlHidden = regexp.MustCompile(`(?is)<(?:script|style|noscript|svg|template)\b[^>]*>.*?</(?:script|style|noscript|svg|template)\s*>`)
var htmlTags = regexp.MustCompile(`(?s)<[^>]*>`)
var whitespace = regexp.MustCompile(`\s+`)

func (a *API) previewURL(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var input struct {
		URL               string `json:"url"`
		AllowHTTPRedirect bool   `json:"allow_https_to_http_redirect"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
		fail(w, 400, "enter a source URL")
		return
	}
	raw := strings.TrimSpace(input.URL)
	fetcher := a.Fetcher
	if fetcher == nil {
		fetcher = &safefetch.Fetcher{}
	}
	result, err := fetcher.Fetch(r.Context(), raw, safefetch.PreviewLimit, input.AllowHTTPRedirect)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	defer result.Body.Close()
	data, err := io.ReadAll(result.Body)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	detected := "unsupported"
	sample := ""
	readiness := "unsupported type"
	mime := strings.ToLower(strings.TrimSpace(strings.Split(result.ContentType, ";")[0]))
	trimmed := bytes.TrimSpace(data)
	switch {
	case len(trimmed) == 0:
		readiness = "empty response"
	case bytes.HasPrefix(trimmed, []byte("%PDF-")):
		detected = "pdf"
		readiness = "PDF can be imported for review"
		if result.Downgraded {
			readiness = "HTTPS to HTTP redirect was previewed; direct PDF import blocks this downgrade"
		}
	case bytes.HasPrefix(bytes.ToLower(trimmed), []byte("<!doctype html")) || bytes.HasPrefix(bytes.ToLower(trimmed), []byte("<html")) || mime == "text/html":
		detected = "html"
		sample = shortText(htmlTags.ReplaceAllString(htmlHidden.ReplaceAllString(string(data), " "), " "))
		readiness = "HTML structure and text need extraction review in ING-02"
		if sample == "" {
			readiness = "missing text"
		} else if loginPage(sample) {
			readiness = "login or error page"
		}
	case bytes.HasPrefix(trimmed, []byte("<?xml")) || mime == "application/xml" || mime == "text/xml":
		detected = "xml"
		sample = shortText(htmlTags.ReplaceAllString(string(data), " "))
		readiness = "XML structure needs extraction review in ING-02"
		if sample == "" {
			readiness = "missing text"
		}
	case mime == "text/plain" || (utf8.Valid(data) && !bytes.ContainsRune(data, 0) && !bytes.Contains(trimmed, []byte("<"))):
		detected = "txt"
		sample = shortText(string(data))
		readiness = "Text needs extraction review in ING-02"
		if sample == "" {
			readiness = "missing text"
		} else if loginPage(sample) {
			readiness = "login or error page"
		}
	}
	write(w, 200, map[string]any{"requested_url": result.RequestedURL, "final_url": result.FinalURL, "redirected": result.Redirected, "transport_changed": result.TransportChanged, "https_to_http_redirect_allowed": input.AllowHTTPRedirect && result.Downgraded, "unencrypted": result.Unencrypted, "detected_type": detected, "content_type": result.ContentType, "byte_size": len(data), "sample": sample, "readiness": readiness, "can_import_pdf": detected == "pdf" && !result.Downgraded})
}
func shortText(s string) string {
	s = whitespace.ReplaceAllString(strings.TrimSpace(s), " ")
	if len(s) > 400 {
		s = s[:400]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}
func loginPage(s string) bool {
	v := strings.ToLower(s)
	return strings.Contains(v, "sign in") || strings.Contains(v, "log in") || strings.Contains(v, "access denied") || strings.Contains(v, "not found") || strings.Contains(v, "404")
}
