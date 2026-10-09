package httpapi

import (
	"encoding/json"
	"net/http"

	"homeopath-poc/backend/internal/collection"
	"homeopath-poc/backend/internal/safefetch"
)

// previewCollection fetches within an explicit admin scope and returns a
// reviewable navigation manifest. It creates no source, asset, or index.
func (a *API) previewCollection(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var scope collection.Scope
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&scope); err != nil {
		fail(w, 400, "invalid collection preview scope")
		return
	}
	if scope.RequestDelayMillis < 200 {
		scope.RequestDelayMillis = 200
	}
	if _, err := scope.Validate(); err != nil {
		fail(w, 400, err.Error())
		return
	}
	fetcher := a.Fetcher
	if fetcher == nil {
		fetcher = &safefetch.Fetcher{}
	}
	manifest, err := collection.Preview(r.Context(), scope, fetcher)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	write(w, 200, map[string]any{"scope": scope, "manifest": manifest, "preview_only": true})
}
