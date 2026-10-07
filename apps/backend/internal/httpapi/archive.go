package httpapi

import (
	"net/http"
	"strconv"

	"homeopath-poc/backend/internal/archive"
)

func (a *API) archiveClient() *archive.Client {
	if a.Archive != nil {
		return a.Archive
	}
	return &archive.Client{}
}

func (a *API) searchArchive(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > 50 {
			fail(w, 400, "invalid search page")
			return
		}
	}
	result, err := a.archiveClient().Search(r.Context(), r.URL.Query().Get("q"), page)
	if err != nil {
		fail(w, 502, "Could not search Internet Archive: "+err.Error())
		return
	}
	write(w, 200, result)
}

func (a *API) archiveItem(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	item, err := a.archiveClient().Item(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 502, "Could not inspect Internet Archive item: "+err.Error())
		return
	}
	write(w, 200, item)
}
