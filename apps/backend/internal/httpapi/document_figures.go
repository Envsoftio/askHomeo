package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const maxFigureBytes = 5 << 20

func (a *API) documentFigures(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT b.id,f.sha256,f.media_type,f.origin_note FROM document_blocks b JOIN sources s ON s.id=b.source_id LEFT JOIN document_figure_images f ON f.source_asset_id=b.source_asset_id AND f.section_key=b.section_key WHERE b.source_id=$1 AND b.processing_revision_id=s.current_revision_id AND b.kind='figure' AND s.removed_at IS NULL ORDER BY b.block_index`, id)
	if err != nil {
		fail(w, 500, "could not list figures")
		return
	}
	defer rows.Close()
	type figure struct {
		BlockID    uuid.UUID `json:"block_id"`
		HasImage   bool      `json:"has_image"`
		OriginNote string    `json:"origin_note"`
		ImageURL   string    `json:"image_url"`
	}
	items := []figure{}
	for rows.Next() {
		var item figure
		var sha, media, note *string
		if err = rows.Scan(&item.BlockID, &sha, &media, &note); err != nil {
			fail(w, 500, "could not list figures")
			return
		}
		if sha != nil {
			item.HasImage = true
			item.OriginNote = *note
			item.ImageURL = "/api/v1/sources/" + id.String() + "/figures/" + item.BlockID.String() + "/image"
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		fail(w, 500, "could not list figures")
		return
	}
	write(w, 200, items)
}

func (a *API) uploadFigureImage(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	blockID, err := uuid.Parse(r.PathValue("block"))
	if err != nil {
		fail(w, 400, "invalid figure block")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFigureBytes+(1<<20))
	if err = r.ParseMultipartForm(1 << 20); err != nil {
		fail(w, 400, "invalid image upload or image exceeds 5 MiB")
		return
	}
	note := strings.TrimSpace(r.FormValue("origin_note"))
	if note == "" || len(note) > 2000 {
		fail(w, 400, "record where this exact figure image came from")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "choose a PNG or JPEG image")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxFigureBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxFigureBytes {
		fail(w, 400, "image must be nonempty and at most 5 MiB")
		return
	}
	media := http.DetectContentType(raw)
	if media != "image/png" && media != "image/jpeg" {
		fail(w, 400, "only PNG and JPEG figures are supported")
		return
	}
	config, decoded, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 10000 || config.Height > 10000 || int64(config.Width)*int64(config.Height) > 25000000 || (media == "image/png" && decoded != "png") || (media == "image/jpeg" && decoded != "jpeg") {
		fail(w, 400, "image pixels or dimensions are invalid")
		return
	}
	if _, _, err = image.Decode(bytes.NewReader(raw)); err != nil {
		fail(w, 400, "image data is incomplete or invalid")
		return
	}
	var assetID uuid.UUID
	err = a.Store.DB.QueryRow(r.Context(), `SELECT b.source_asset_id FROM document_blocks b JOIN sources s ON s.id=b.source_id WHERE b.id=$1 AND b.source_id=$2 AND b.processing_revision_id=s.current_revision_id AND b.kind='figure' AND s.document_format='jats' AND s.status='review' AND s.removed_at IS NULL`, blockID, id).Scan(&assetID)
	if err != nil {
		fail(w, 409, "figure is not in the current review revision")
		return
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	ext := ".png"
	if media == "image/jpeg" {
		ext = ".jpg"
	}
	rel := filepath.Join("data/runtime/assets/figures", sha+ext)
	path := filepath.Join(a.Store.Root, rel)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		fail(w, 500, "could not prepare figure storage")
		return
	}
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		if err = os.WriteFile(path, raw, 0600); err != nil {
			fail(w, 500, "could not save figure image")
			return
		}
	}
	result, err := a.Store.DB.Exec(r.Context(), `INSERT INTO document_figure_images(id,source_id,source_asset_id,section_key,sha256,media_type,byte_size,origin_note,uploaded_by_principal_id) SELECT $1,b.source_id,b.source_asset_id,b.section_key,$3,$4,$5,$6,$7 FROM document_blocks b JOIN sources s ON s.id=b.source_id WHERE b.id=$2 AND b.source_id=$8 AND b.processing_revision_id=s.current_revision_id AND b.kind='figure' AND s.status='review' ON CONFLICT(source_asset_id,section_key) DO NOTHING`, uuid.New(), blockID, sha, media, len(raw), note, requestPrincipal(r.Context()).ID, id)
	if err != nil {
		fail(w, 500, "could not record figure provenance")
		return
	}
	if result.RowsAffected() == 0 {
		fail(w, 409, "a figure image is already saved for this block")
		return
	}
	write(w, 201, map[string]any{"sha256": sha, "image_url": "/api/v1/sources/" + id.String() + "/figures/" + blockID.String() + "/image"})
}

func (a *API) figureImage(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	blockID, err := uuid.Parse(r.PathValue("block"))
	if err != nil {
		fail(w, 400, "invalid figure block")
		return
	}
	var sha, media string
	err = a.Store.DB.QueryRow(r.Context(), `SELECT f.sha256,f.media_type FROM document_figure_images f JOIN document_blocks b ON b.source_asset_id=f.source_asset_id AND b.section_key=f.section_key JOIN sources s ON s.id=b.source_id WHERE b.id=$1 AND b.source_id=$2 AND b.processing_revision_id=s.current_revision_id AND b.kind='figure' AND s.removed_at IS NULL`, blockID, id).Scan(&sha, &media)
	if err != nil {
		fail(w, 404, "figure image unavailable")
		return
	}
	ext := ".png"
	if media == "image/jpeg" {
		ext = ".jpg"
	}
	raw, err := os.ReadFile(filepath.Join(a.Store.Root, "data/runtime/assets/figures", sha+ext))
	if err != nil {
		fail(w, 503, "saved figure unavailable")
		return
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != sha {
		fail(w, 409, "saved figure checksum differs")
		return
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(raw)
}
