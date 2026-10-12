package scholarly

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Work struct {
	DOI              string          `json:"doi"`
	PMID             string          `json:"pmid"`
	PMCID            string          `json:"pmcid"`
	Title            string          `json:"title"`
	Authors          string          `json:"authors"`
	Year             int             `json:"year"`
	Type             string          `json:"publication_type"`
	Abstract         string          `json:"abstract"`
	Provider         string          `json:"provider"`
	NativeID         string          `json:"native_id"`
	Raw              json.RawMessage `json:"raw_metadata,omitempty"`
	IdentityConflict bool            `json:"identity_conflict,omitempty"`
}
type Page struct {
	Provider    string    `json:"provider"`
	NativeQuery string    `json:"native_query"`
	Results     []Work    `json:"results"`
	Total       int       `json:"total"`
	Next        string    `json:"next,omitempty"`
	Cap         int       `json:"cap"`
	RetrievedAt time.Time `json:"retrieved_at"`
}
type Client struct {
	HTTP                   *http.Client
	PubMedBase, EuropeBase string
	mu                     sync.Mutex
	last                   map[string]time.Time
}
type ProviderError struct {
	Provider   string
	Status     int
	RetryAfter time.Duration
}

func (e *ProviderError) Error() string { return fmt.Sprintf("%s HTTP %d", e.Provider, e.Status) }

func (c *Client) get(ctx context.Context, provider, rawURL string, limit int64) ([]byte, error) {
	interval := 350 * time.Millisecond
	if provider == "pmc" {
		interval = 400 * time.Millisecond
	}
	if provider == "semantic_scholar" {
		interval = 1100 * time.Millisecond
	}
	c.mu.Lock()
	if c.last == nil {
		c.last = map[string]time.Time{}
	}
	wait := time.Until(c.last[provider].Add(interval))
	if wait < 0 {
		wait = 0
	}
	c.last[provider] = time.Now().Add(wait)
	c.mu.Unlock()
	if wait > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	originalURL, _ := url.Parse(rawURL)
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Hostname(), originalURL.Hostname()) {
			return errors.New("provider redirect left approved HTTPS host or exceeded limit")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, application/xml;q=0.8")
	if provider == "semantic_scholar" {
		req.Header.Set("x-api-key", os.Getenv("SEMANTIC_SCHOLAR_API_KEY"))
	}
	resp, err := copyClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request failed (%T)", provider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var retry time.Duration
		if raw := resp.Header.Get("Retry-After"); raw != "" {
			if seconds, e := strconv.Atoi(raw); e == nil && seconds >= 0 {
				retry = time.Duration(seconds) * time.Second
			} else if date, e := http.ParseTime(raw); e == nil {
				retry = time.Until(date)
			}
		}
		if retry < 0 {
			retry = 0
		}
		if retry > time.Hour {
			retry = time.Hour
		}
		return nil, &ProviderError{Provider: provider, Status: resp.StatusCode, RetryAfter: retry}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("provider response exceeds limit")
	}
	return body, nil
}

func normalizeDOI(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "https://doi.org/")
	v = strings.TrimPrefix(v, "doi:")
	return v
}
func normalizePMCID(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if v != "" && !strings.HasPrefix(v, "PMC") {
		v = "PMC" + v
	}
	return v
}

func (c *Client) SearchPubMed(ctx context.Context, query string, offset, count int) (Page, error) {
	if count < 1 || count > 30 || offset < 0 || offset > 9970 {
		return Page{}, errors.New("invalid PubMed page")
	}
	base := c.PubMedBase
	if base == "" {
		base = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"
	}
	native := "(" + query + ") AND (homeopathy[All Fields] OR homoeopathy[All Fields])"
	p := url.Values{"db": {"pubmed"}, "term": {native}, "retmode": {"json"}, "retstart": {strconv.Itoa(offset)}, "retmax": {strconv.Itoa(count)}}
	if key := os.Getenv("NCBI_API_KEY"); key != "" {
		p.Set("api_key", key)
	}
	raw, err := c.get(ctx, "pubmed", base+"/esearch.fcgi?"+p.Encode(), 2<<20)
	if err != nil {
		return Page{}, err
	}
	var sr struct {
		ESearchResult struct {
			Count    string   `json:"count"`
			IDList   []string `json:"idlist"`
			RetStart string   `json:"retstart"`
		} `json:"esearchresult"`
	}
	if err = json.Unmarshal(raw, &sr); err != nil {
		return Page{}, fmt.Errorf("PubMed search JSON: %w", err)
	}
	total, _ := strconv.Atoi(sr.ESearchResult.Count)
	out := Page{Provider: "pubmed", NativeQuery: native, Results: []Work{}, Total: total, Cap: count, RetrievedAt: time.Now().UTC()}
	if len(sr.ESearchResult.IDList) > 0 {
		p = url.Values{"db": {"pubmed"}, "id": {strings.Join(sr.ESearchResult.IDList, ",")}, "retmode": {"xml"}}
		if key := os.Getenv("NCBI_API_KEY"); key != "" {
			p.Set("api_key", key)
		}
		body, e := c.get(ctx, "pubmed", base+"/efetch.fcgi?"+p.Encode(), 5<<20)
		if e != nil {
			return Page{}, e
		}
		var parsed struct {
			Articles []struct {
				Citation struct {
					PMID    string `xml:"PMID"`
					Article struct {
						Title    string `xml:"ArticleTitle"`
						Abstract string `xml:"Abstract>AbstractText"`
						Journal  struct {
							Date struct {
								Year string `xml:"Year"`
							} `xml:"JournalIssue>PubDate"`
						} `xml:"Journal"`
						Authors []struct {
							Last string `xml:"LastName"`
							Fore string `xml:"ForeName"`
						} `xml:"AuthorList>Author"`
						Types []string `xml:"PublicationTypeList>PublicationType"`
					} `xml:"Article"`
				} `xml:"MedlineCitation"`
				Data struct {
					IDs []struct {
						Type  string `xml:"IdType,attr"`
						Value string `xml:",chardata"`
					} `xml:"ArticleIdList>ArticleId"`
				} `xml:"PubmedData"`
			} `xml:"PubmedArticle"`
		}
		if e = xml.Unmarshal(body, &parsed); e != nil {
			return Page{}, fmt.Errorf("PubMed EFetch XML: %w", e)
		}
		for _, a := range parsed.Articles {
			w := Work{PMID: a.Citation.PMID, Title: strings.TrimSpace(a.Citation.Article.Title), Abstract: strings.TrimSpace(a.Citation.Article.Abstract), Provider: "pubmed", NativeID: a.Citation.PMID}
			w.Year, _ = strconv.Atoi(a.Citation.Article.Journal.Date.Year)
			w.Type = strings.Join(a.Citation.Article.Types, ", ")
			names := []string{}
			for _, n := range a.Citation.Article.Authors {
				names = append(names, strings.TrimSpace(n.Fore+" "+n.Last))
			}
			w.Authors = strings.Join(names, ", ")
			for _, id := range a.Data.IDs {
				switch id.Type {
				case "doi":
					w.DOI = normalizeDOI(id.Value)
				case "pmc":
					w.PMCID = normalizePMCID(id.Value)
				}
			}
			w.Raw, _ = json.Marshal(a)
			out.Results = append(out.Results, w)
		}
		if len(out.Results) != len(sr.ESearchResult.IDList) {
			return Page{}, errors.New("PubMed EFetch did not return every selected ID")
		}
	}
	if offset+len(out.Results) < total {
		out.Next = strconv.Itoa(offset + len(out.Results))
	}
	return out, nil
}

func (c *Client) SearchEurope(ctx context.Context, query, cursor string, count int) (Page, error) {
	if count < 1 || count > 30 {
		return Page{}, errors.New("invalid Europe PMC page")
	}
	if cursor == "" {
		cursor = "*"
	}
	base := c.EuropeBase
	if base == "" {
		base = "https://www.ebi.ac.uk/europepmc/webservices/rest"
	}
	native := "(" + query + ") AND (homeopathy OR homoeopathy)"
	p := url.Values{"query": {native}, "format": {"json"}, "resultType": {"core"}, "pageSize": {strconv.Itoa(count)}, "cursorMark": {cursor}}
	raw, err := c.get(ctx, "europepmc", base+"/search?"+p.Encode(), 5<<20)
	if err != nil {
		return Page{}, err
	}
	var sr struct {
		HitCount   int    `json:"hitCount"`
		NextCursor string `json:"nextCursorMark"`
		Results    struct {
			Items []struct {
				ID       string `json:"id"`
				Source   string `json:"source"`
				Title    string `json:"title"`
				Author   string `json:"authorString"`
				Year     string `json:"pubYear"`
				DOI      string `json:"doi"`
				PMID     string `json:"pmid"`
				PMCID    string `json:"pmcid"`
				Type     string `json:"pubType"`
				Abstract string `json:"abstractText"`
			} `json:"result"`
		} `json:"resultList"`
	}
	if err = json.Unmarshal(raw, &sr); err != nil {
		return Page{}, fmt.Errorf("Europe PMC JSON: %w", err)
	}
	out := Page{Provider: "europepmc", NativeQuery: native, Results: []Work{}, Total: sr.HitCount, Cap: count, RetrievedAt: time.Now().UTC()}
	for _, v := range sr.Results.Items {
		year, _ := strconv.Atoi(v.Year)
		w := Work{DOI: normalizeDOI(v.DOI), PMID: v.PMID, PMCID: normalizePMCID(v.PMCID), Title: v.Title, Authors: v.Author, Year: year, Type: v.Type, Abstract: v.Abstract, Provider: "europepmc", NativeID: v.Source + ":" + v.ID}
		w.Raw, _ = json.Marshal(v)
		out.Results = append(out.Results, w)
	}
	if len(out.Results) == count && sr.NextCursor != "" && sr.NextCursor != cursor {
		out.Next = sr.NextCursor
	}
	return out, nil
}

func (c *Client) PMC(ctx context.Context, pmcid string) ([]byte, error) {
	pmcid = normalizePMCID(pmcid)
	if !strings.HasPrefix(pmcid, "PMC") {
		return nil, errors.New("PMCID required")
	}
	for _, r := range pmcid[3:] {
		if r < '0' || r > '9' {
			return nil, errors.New("invalid PMCID")
		}
	}
	p := url.Values{"verb": {"GetRecord"}, "identifier": {"oai:pubmedcentral.nih.gov:" + pmcid[3:]}, "metadataPrefix": {"pmc"}}
	raw, err := c.get(ctx, "pmc", "https://pmc.ncbi.nlm.nih.gov/api/oai/v1/mh/?"+p.Encode(), 10<<20)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(string(raw), "<article") {
		return nil, errors.New("PMC OAI returned metadata-only or an error record")
	}
	return raw, nil
}

func (c *Client) EuropeFullText(ctx context.Context, pmcid string) ([]byte, error) {
	pmcid = normalizePMCID(pmcid)
	if pmcid == "" {
		return nil, errors.New("PMCID required")
	}
	base := c.EuropeBase
	if base == "" {
		base = "https://www.ebi.ac.uk/europepmc/webservices/rest"
	}
	return c.get(ctx, "europepmc", base+"/"+url.PathEscape(pmcid)+"/fullTextXML", 10<<20)
}

// Semantic Scholar search uses plain text relevance, never PubMed Boolean
// syntax. Production calls require credentials and an explicit permission flag.
func (c *Client) SearchSemantic(ctx context.Context, query string, offset, count int) (Page, error) {
	if os.Getenv("SEMANTIC_SCHOLAR_ENABLED") != "1" || os.Getenv("SEMANTIC_SCHOLAR_COMMERCIAL_PERMISSION_CONFIRMED") != "1" || os.Getenv("SEMANTIC_SCHOLAR_API_KEY") == "" {
		return Page{}, errors.New("Semantic Scholar disabled: API key and intended commercial-use permission are required")
	}
	if offset < 0 || offset > 980 || count < 1 || count > 20 {
		return Page{}, errors.New("invalid Semantic Scholar page")
	}
	values := url.Values{"query": {query}, "offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(count)}, "fields": {"paperId,title,authors,year,publicationTypes,externalIds,abstract,openAccessPdf"}}
	raw, err := c.get(ctx, "semantic_scholar", "https://api.semanticscholar.org/graph/v1/paper/search?"+values.Encode(), 3<<20)
	if err != nil {
		return Page{}, err
	}
	var sr struct {
		Total  int `json:"total"`
		Offset int `json:"offset"`
		Next   int `json:"next"`
		Data   []struct {
			ID      string `json:"paperId"`
			Title   string `json:"title"`
			Year    int    `json:"year"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
			Types    []string          `json:"publicationTypes"`
			External map[string]string `json:"externalIds"`
			Abstract string            `json:"abstract"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &sr); err != nil {
		return Page{}, err
	}
	out := Page{Provider: "semantic_scholar", NativeQuery: query, Results: []Work{}, Total: sr.Total, Cap: count, RetrievedAt: time.Now().UTC()}
	for _, v := range sr.Data {
		names := []string{}
		for _, a := range v.Authors {
			names = append(names, a.Name)
		}
		w := Work{DOI: normalizeDOI(v.External["DOI"]), PMID: v.External["PubMed"], PMCID: normalizePMCID(v.External["PubMedCentral"]), Title: v.Title, Authors: strings.Join(names, ", "), Year: v.Year, Type: strings.Join(v.Types, ", "), Abstract: v.Abstract, Provider: "semantic_scholar", NativeID: v.ID}
		w.Raw, _ = json.Marshal(v)
		out.Results = append(out.Results, w)
	}
	if sr.Next > offset {
		out.Next = strconv.Itoa(sr.Next)
	}
	return out, nil
}
