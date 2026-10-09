package doi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"homeopath-poc/backend/internal/safefetch"
)

type Record struct {
	DOI                 string `json:"doi"`
	Title               string `json:"title"`
	Authors             string `json:"authors"`
	Year                int    `json:"publication_year"`
	Publisher           string `json:"publisher"`
	WorkType            string `json:"work_type"`
	DOIURL              string `json:"doi_url"`
	PDFURL              string `json:"pdf_url"`
	LicenseURL          string `json:"license_url"`
	Provider            string `json:"metadata_provider"`
	RetractionNoticeURL string `json:"retraction_notice_url"`
}

var doiPattern = regexp.MustCompile(`(?i)^10\.[0-9]{4,9}/[^\s?#]{1,250}$`)

func Normalize(input string) (string, error) {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://doi.org/"), "http://doi.org/")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://dx.doi.org/"), "http://dx.doi.org/")
	if strings.HasPrefix(strings.ToLower(s), "doi:") {
		s = s[4:]
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if !doiPattern.MatchString(s) || strings.ContainsAny(s, "<>\\\"'%") || strings.Contains(s, "..") {
		return "", errors.New("enter a valid DOI, such as 10.1371/journal.pone.0134657")
	}
	return s, nil
}

type Client struct {
	HTTP        *http.Client
	BaseURL     string
	DataCiteURL string
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}
func (c *Client) base() string {
	if c != nil && c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://api.crossref.org"
}

func (c *Client) Lookup(ctx context.Context, input string) (Record, error) {
	name, err := Normalize(input)
	if err != nil {
		return Record{}, err
	}
	address := c.base() + "/works/" + url.PathEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return Record{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HomeopathPOC/0.1 (DOI metadata lookup)")
	res, err := c.http().Do(req)
	if err != nil {
		return Record{}, fmt.Errorf("Crossref lookup failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return c.lookupDataCite(ctx, name)
	}
	if res.StatusCode != 200 {
		return Record{}, fmt.Errorf("Crossref returned HTTP %d", res.StatusCode)
	}
	var envelope struct {
		Message struct {
			DOI    string   `json:"DOI"`
			Title  []string `json:"title"`
			Author []struct {
				Given  string `json:"given"`
				Family string `json:"family"`
				Name   string `json:"name"`
			} `json:"author"`
			Publisher string `json:"publisher"`
			Type      string `json:"type"`
			URL       string `json:"URL"`
			Published struct {
				DateParts [][]int `json:"date-parts"`
			} `json:"published"`
			Link []struct {
				URL            string `json:"URL"`
				ContentType    string `json:"content-type"`
				ContentVersion string `json:"content-version"`
			} `json:"link"`
			License []struct {
				URL            string `json:"URL"`
				ContentVersion string `json:"content-version"`
				Start          struct {
					DateParts [][]int `json:"date-parts"`
				} `json:"start"`
			} `json:"license"`
			UpdatedBy []struct {
				DOI  string `json:"DOI"`
				Type string `json:"type"`
			} `json:"updated-by"`
		} `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&envelope); err != nil {
		return Record{}, fmt.Errorf("invalid Crossref response: %w", err)
	}
	m := envelope.Message
	returned, err := Normalize(m.DOI)
	if err != nil || returned != name || len(m.Title) == 0 || strings.TrimSpace(m.Title[0]) == "" {
		return Record{}, errors.New("Crossref returned incomplete or mismatched DOI metadata")
	}
	authors := make([]string, 0, len(m.Author))
	for _, a := range m.Author {
		name := strings.TrimSpace(strings.TrimSpace(a.Given + " " + a.Family))
		if name == "" {
			name = a.Name
		}
		if name != "" {
			authors = append(authors, name)
		}
		if len(authors) == 20 {
			break
		}
	}
	if len(authors) == 0 {
		authors = []string{"Unknown author"}
	}
	r := Record{DOI: name, Title: strings.TrimSpace(m.Title[0]), Authors: strings.Join(authors, ", "), Publisher: m.Publisher, WorkType: m.Type, DOIURL: "https://doi.org/" + name, Provider: "Crossref"}
	for _, update := range m.UpdatedBy {
		if !strings.EqualFold(update.Type, "retraction") {
			continue
		}
		if noticeDOI, err := Normalize(update.DOI); err == nil {
			r.RetractionNoticeURL = "https://doi.org/" + noticeDOI
			break
		}
	}
	if len(m.Published.DateParts) > 0 && len(m.Published.DateParts[0]) > 0 {
		r.Year = m.Published.DateParts[0][0]
	}
	for _, license := range m.License {
		if !eligibleLicense(license.URL) || !licenseStarted(license.Start.DateParts) {
			continue
		}
		r.LicenseURL = license.URL
		for _, link := range m.Link {
			if !strings.EqualFold(link.ContentType, "application/pdf") || validPublicPDFURL(link.URL) != nil {
				continue
			}
			if license.ContentVersion != "" && !strings.EqualFold(license.ContentVersion, link.ContentVersion) {
				continue
			}
			r.PDFURL = link.URL
			break
		}
		if r.PDFURL != "" {
			break
		}
	}
	if r.PDFURL == "" && r.LicenseURL != "" && strings.HasPrefix(name, "10.1371/journal.pone.") {
		for _, license := range m.License {
			if eligibleLicense(license.URL) && licenseStarted(license.Start.DateParts) && (license.ContentVersion == "" || strings.EqualFold(license.ContentVersion, "vor")) {
				r.PDFURL = "https://journals.plos.org/plosone/article/file?" + url.Values{"id": {name}, "type": {"printable"}}.Encode()
				break
			}
		}
	}
	return r, nil
}

func licenseStarted(parts [][]int) bool {
	if len(parts) == 0 || len(parts[0]) == 0 {
		return true
	}
	d := parts[0]
	month, day := 1, 1
	if len(d) > 1 {
		month = d[1]
	}
	if len(d) > 2 {
		day = d[2]
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return false
	}
	return !time.Date(d[0], time.Month(month), day, 0, 0, 0, 0, time.UTC).After(time.Now().UTC())
}

func (c *Client) lookupDataCite(ctx context.Context, name string) (Record, error) {
	base := "https://api.datacite.org"
	if c != nil && c.DataCiteURL != "" {
		base = strings.TrimRight(c.DataCiteURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/dois/"+url.PathEscape(name), nil)
	if err != nil {
		return Record{}, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.http().Do(req)
	if err != nil {
		return Record{}, fmt.Errorf("DataCite lookup failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return Record{}, errors.New("DOI not found in Crossref or DataCite")
	}
	if res.StatusCode != 200 {
		return Record{}, fmt.Errorf("DataCite returned HTTP %d", res.StatusCode)
	}
	var envelope struct {
		Data struct {
			Attributes struct {
				DOI    string `json:"doi"`
				Titles []struct {
					Title string `json:"title"`
				} `json:"titles"`
				Creators []struct {
					Name       string `json:"name"`
					GivenName  string `json:"givenName"`
					FamilyName string `json:"familyName"`
				} `json:"creators"`
				Publisher       string `json:"publisher"`
				PublicationYear int    `json:"publicationYear"`
				Types           struct {
					ResourceTypeGeneral string `json:"resourceTypeGeneral"`
				} `json:"types"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&envelope); err != nil {
		return Record{}, fmt.Errorf("invalid DataCite response: %w", err)
	}
	m := envelope.Data.Attributes
	returned, err := Normalize(m.DOI)
	if err != nil || returned != name || len(m.Titles) == 0 || strings.TrimSpace(m.Titles[0].Title) == "" {
		return Record{}, errors.New("DataCite returned incomplete or mismatched DOI metadata")
	}
	authors := make([]string, 0, len(m.Creators))
	for _, a := range m.Creators {
		n := strings.TrimSpace(a.Name)
		if n == "" {
			n = strings.TrimSpace(a.GivenName + " " + a.FamilyName)
		}
		if n != "" {
			authors = append(authors, n)
		}
		if len(authors) == 20 {
			break
		}
	}
	if len(authors) == 0 {
		authors = []string{"Unknown author"}
	}
	return Record{DOI: name, Title: strings.TrimSpace(m.Titles[0].Title), Authors: strings.Join(authors, ", "), Year: m.PublicationYear, Publisher: m.Publisher, WorkType: m.Types.ResourceTypeGeneral, DOIURL: "https://doi.org/" + name, Provider: "DataCite"}, nil
}

func eligibleLicense(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), "creativecommons.org") {
		return false
	}
	p := strings.ToLower(u.Path)
	return strings.HasPrefix(p, "/licenses/by/") || strings.HasPrefix(p, "/licenses/by-sa/") || strings.HasPrefix(p, "/publicdomain/zero/")
}

func validPublicPDFURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return errors.New("PDF URL must be a public HTTPS address")
	}
	return nil
}

// FetchPDF keeps DOI PDF acquisition HTTPS-only while sharing the public-destination checks.
func FetchPDF(ctx context.Context, raw string) (io.ReadCloser, error) {
	if err := validPublicPDFURL(raw); err != nil {
		return nil, err
	}
	result, err := (&safefetch.Fetcher{}).Fetch(ctx, raw, safefetch.PDFLimit, false)
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}
