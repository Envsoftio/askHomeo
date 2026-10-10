package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/safefetch"
)

// importPDFURL downloads a public HTTPS PDF and queues the existing page workflow.
func (a *API) importPDFURL(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var b struct {
		LiteratureCategories []string `json:"literature_categories"`
		PDFURL               string   `json:"pdf_url"`
		Title                string   `json:"title"`
		Author               string   `json:"author"`
		Edition              string   `json:"edition"`
		PublicationInfo      string   `json:"publication_info"`
		Repository           string   `json:"repository"`
		SourceURL            string   `json:"source_url"`
		RightsStatement      string   `json:"rights_statement"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b); err != nil {
		fail(w, 400, "enter a PDF link and source details")
		return
	}
	if len(b.LiteratureCategories) > 0 && !validLiteratureCategories(b.LiteratureCategories) {
		fail(w, 400, "choose valid literature categories")
		return
	}
	b.PDFURL = strings.TrimSpace(b.PDFURL)
	if b.PDFURL == "" {
		fail(w, 400, "PDF link is required")
		return
	}
	fetcher := a.Fetcher
	if fetcher == nil {
		fetcher = &safefetch.Fetcher{}
	}
	result, err := fetcher.Fetch(r.Context(), b.PDFURL, safefetch.PDFLimit, false)
	if err != nil {
		fail(w, 502, "PDF could not be downloaded: "+err.Error())
		return
	}
	defer result.Body.Close()
	id, err := a.Store.ImportPDFWithOrigin(r.Context(), result.Body, b.Title, b.Author, b.Edition, b.PublicationInfo, b.Repository, b.SourceURL, b.RightsStatement, result.FinalURL, core.IntakeCategory{Categories: b.LiteratureCategories, ActorID: requestPrincipal(r.Context()).ID})
	if err != nil {
		fail(w, 400, "Downloaded file could not be imported: "+err.Error())
		return
	}
	a.writeImportedSource(w, r, id)
}
