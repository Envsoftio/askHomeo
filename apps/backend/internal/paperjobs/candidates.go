package paperjobs

import (
	"strings"

	"homeopath-poc/backend/internal/scholarly"
)

type Candidate struct {
	Resolver string `json:"resolver"`
	URL      string `json:"url"`
	Format   string `json:"format"`
	Version  string `json:"version,omitempty"`
	License  string `json:"license,omitempty"`
}

// Unpaywall records are resolution hints, not rights decisions. Keep only
// plausible commercial-use versions; downloaded bytes face stricter checks.
func CandidateFromLocation(loc scholarly.Location) (Candidate, bool) {
	license := strings.ToLower(strings.TrimSpace(loc.License))
	if license != "cc-by" && license != "cc-by-4.0" && license != "cc0" && license != "cc0-1.0" {
		return Candidate{}, false
	}
	if loc.Version != "publishedVersion" && loc.Version != "acceptedVersion" {
		return Candidate{}, false
	}
	format := loc.Format
	if format == "landing" {
		format = "html"
	}
	if format != "pdf" && format != "html" {
		return Candidate{}, false
	}
	return Candidate{Resolver: "unpaywall", URL: loc.URL, Format: format, Version: loc.Version, License: license}, true
}
