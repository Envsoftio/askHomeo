package scholarly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Location struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Version  string `json:"version"`
	License  string `json:"license"`
	HostType string `json:"host_type"`
	Source   string `json:"source"`
}
type Locations struct {
	DOI        string     `json:"doi"`
	Available  bool       `json:"available"`
	Candidates []Location `json:"candidates"`
	Reason     string     `json:"reason"`
}

func (c *Client) Unpaywall(ctx context.Context, doi string) (Locations, error) {
	email := strings.TrimSpace(os.Getenv("UNPAYWALL_CONTACT_EMAIL"))
	if email == "" {
		return Locations{DOI: doi, Reason: "Unpaywall disabled: configure UNPAYWALL_CONTACT_EMAIL"}, nil
	}
	doi = normalizeDOI(doi)
	if !strings.HasPrefix(doi, "10.") || strings.ContainsAny(doi, " \t\r\n") {
		return Locations{}, errors.New("valid DOI required")
	}
	endpoint := "https://api.unpaywall.org/v2/" + url.PathEscape(doi) + "?email=" + url.QueryEscape(email)
	raw, err := c.get(ctx, "unpaywall", endpoint, 2<<20)
	if err != nil {
		return Locations{}, fmt.Errorf("Unpaywall request failed; check DOI and resolver availability: %T", err)
	}
	var value struct {
		DOI  string `json:"doi"`
		IsOA bool   `json:"is_oa"`
		Best *struct {
			URLPDF     string `json:"url_for_pdf"`
			URLLanding string `json:"url_for_landing_page"`
			Version    string `json:"version"`
			License    string `json:"license"`
			HostType   string `json:"host_type"`
		} `json:"best_oa_location"`
		All []struct {
			URLPDF     string `json:"url_for_pdf"`
			URLLanding string `json:"url_for_landing_page"`
			Version    string `json:"version"`
			License    string `json:"license"`
			HostType   string `json:"host_type"`
		} `json:"oa_locations"`
	}
	if err = json.Unmarshal(raw, &value); err != nil {
		return Locations{}, errors.New("Unpaywall returned invalid JSON")
	}
	if normalizeDOI(value.DOI) != doi {
		return Locations{}, errors.New("Unpaywall DOI differs from selected reference")
	}
	out := Locations{DOI: doi, Available: value.IsOA, Candidates: []Location{}}
	seen := map[string]bool{}
	add := func(pdf, landing, version, license, host string) {
		for _, item := range []struct{ address, format string }{{pdf, "pdf"}, {landing, "landing"}} {
			if item.address == "" || seen[item.address] {
				continue
			}
			u, e := url.Parse(item.address)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
				continue
			}
			seen[item.address] = true
			out.Candidates = append(out.Candidates, Location{URL: item.address, Format: item.format, Version: version, License: license, HostType: host, Source: "unpaywall"})
		}
	}
	if value.Best != nil {
		add(value.Best.URLPDF, value.Best.URLLanding, value.Best.Version, value.Best.License, value.Best.HostType)
	}
	for _, v := range value.All {
		if len(out.Candidates) >= 10 {
			break
		}
		add(v.URLPDF, v.URLLanding, v.Version, v.License, v.HostType)
	}
	if len(out.Candidates) == 0 {
		out.Reason = "No verified downloadable location; keep reference-only"
	}
	return out, nil
}
