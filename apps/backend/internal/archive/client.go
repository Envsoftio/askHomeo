package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const MaxPDFBytes = 250 << 20

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type Work struct {
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	Creator    string `json:"creator"`
	Year       string `json:"year"`
	RecordURL  string `json:"record_url"`
}

type SearchResult struct {
	Works []Work `json:"works"`
	Total int    `json:"total"`
	Page  int    `json:"page"`
}

type PDF struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Bytes  int64  `json:"bytes"`
	Source string `json:"source"`
}

type Item struct {
	Work
	PublicationInfo string `json:"publication_info"`
	Rights          string `json:"rights"`
	LicenseURL      string `json:"license_url"`
	PDFs            []PDF  `json:"pdfs"`
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://archive.org"
}

func cleanQuery(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func (c *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Homeopath-POC/1.0 (source discovery)")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Internet Archive returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(target); err != nil {
		return fmt.Errorf("invalid Internet Archive response: %w", err)
	}
	return nil
}

func textValue(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	var values []string
	if json.Unmarshal(raw, &values) == nil {
		return strings.Join(values, ", ")
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

func (c *Client) Search(ctx context.Context, raw string, page int) (SearchResult, error) {
	query := cleanQuery(raw)
	if len([]rune(query)) < 3 || len([]rune(query)) > 120 || page < 1 || page > 50 {
		return SearchResult{}, errors.New("enter 3–120 letters for a title or author")
	}
	params := url.Values{}
	params.Set("q", `mediatype:texts AND (title:"`+query+`" OR creator:"`+query+`")`)
	for _, field := range []string{"identifier", "title", "creator", "year"} {
		params.Add("fl[]", field)
	}
	params.Set("rows", "12")
	params.Set("page", strconv.Itoa(page))
	params.Set("output", "json")
	var response struct {
		Response struct {
			Total int                          `json:"numFound"`
			Docs  []map[string]json.RawMessage `json:"docs"`
		} `json:"response"`
	}
	if err := c.getJSON(ctx, c.baseURL()+"/advancedsearch.php?"+params.Encode(), &response); err != nil {
		return SearchResult{}, err
	}
	result := SearchResult{Works: []Work{}, Total: response.Response.Total, Page: page}
	for _, doc := range response.Response.Docs {
		id := textValue(doc["identifier"])
		if !identifierPattern.MatchString(id) {
			continue
		}
		result.Works = append(result.Works, Work{Identifier: id, Title: textValue(doc["title"]), Creator: textValue(doc["creator"]), Year: textValue(doc["year"]), RecordURL: "https://archive.org/details/" + id})
	}
	return result, nil
}

func (c *Client) Item(ctx context.Context, id string) (Item, error) {
	if !identifierPattern.MatchString(id) {
		return Item{}, errors.New("invalid archive item identifier")
	}
	var response struct {
		Metadata map[string]json.RawMessage `json:"metadata"`
		Files    []struct {
			Name    string          `json:"name"`
			Format  string          `json:"format"`
			Source  string          `json:"source"`
			Size    string          `json:"size"`
			Private json.RawMessage `json:"private"`
			Dark    json.RawMessage `json:"is_dark"`
		} `json:"files"`
	}
	if err := c.getJSON(ctx, c.baseURL()+"/metadata/"+id, &response); err != nil {
		return Item{}, err
	}
	if len(response.Metadata) == 0 {
		return Item{}, errors.New("archive item was not found")
	}
	item := Item{Work: Work{Identifier: id, Title: textValue(response.Metadata["title"]), Creator: textValue(response.Metadata["creator"]), Year: textValue(response.Metadata["year"]), RecordURL: "https://archive.org/details/" + id}, PublicationInfo: textValue(response.Metadata["date"]), Rights: textValue(response.Metadata["rights"]), LicenseURL: textValue(response.Metadata["licenseurl"]), PDFs: []PDF{}}
	if item.PublicationInfo == "" {
		item.PublicationInfo = item.Year
	}
	if item.Year == "" {
		item.Year = item.PublicationInfo
	}
	if textValue(response.Metadata["access-restricted-item"]) == "true" {
		return item, nil
	}
	for _, file := range response.Files {
		size, err := strconv.ParseInt(file.Size, 10, 64)
		if err != nil || size <= 0 || size > MaxPDFBytes || flagSet(file.Private) || flagSet(file.Dark) || !strings.HasSuffix(strings.ToLower(file.Name), ".pdf") || strings.Contains(file.Name, "..") || strings.HasPrefix(file.Name, "/") || strings.Contains(file.Name, "\\") {
			continue
		}
		path := "/download/" + id + "/" + file.Name
		fileURL := (&url.URL{Scheme: "https", Host: "archive.org", Path: path}).String()
		item.PDFs = append(item.PDFs, PDF{Name: file.Name, URL: fileURL, Bytes: size, Source: file.Source})
		if len(item.PDFs) >= 12 {
			break
		}
	}
	return item, nil
}

func flagSet(raw json.RawMessage) bool {
	return string(raw) == "true" || strings.EqualFold(textValue(raw), "true") || textValue(raw) == "1"
}
