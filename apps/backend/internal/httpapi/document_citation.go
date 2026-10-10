package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

func (a *API) citationDocument(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	principal := requestPrincipal(r.Context())
	var chunkID, sourceID, assetID, revisionID, publicationID, blockID, locationID uuid.UUID
	var exact, title, author, sectionKey, original, reviewed, sha, relative, sourceURL, format string
	var start, end int
	var snapshot []byte
	err := a.Store.DB.QueryRow(r.Context(), `SELECT c.id,s.id,sa.id,b.processing_revision_id,pub.id,b.id,loc.id,c.text_exact,s.title,s.author,b.section_key,b.original_text,b.reviewed_text,sa.sha256,sa.object_locator,coalesce(a.final_url,s.source_url,''),s.document_format,c.start_character,c.end_character,coalesce(ans.literature_category_snapshot->c.id::text,'{}'::jsonb)
 FROM answer_citations ac JOIN answers ans ON ans.id=ac.answer_id JOIN chunks c ON c.id=ac.chunk_id JOIN document_blocks b ON b.id=c.document_block_id JOIN sources s ON s.id=b.source_id JOIN source_assets sa ON sa.id=b.source_asset_id JOIN publications pub ON pub.source_id=s.id AND pub.processing_revision_id=b.processing_revision_id JOIN document_locations loc ON loc.processing_revision_id=b.processing_revision_id AND loc.section_key=b.section_key AND loc.kind='text_span' AND loc.start_character=c.start_character AND loc.end_character=c.end_character LEFT JOIN document_acquisitions a ON a.source_id=s.id
	 WHERE ac.id=$1 AND ($2='admin' OR ans.owner_principal_id=$3) AND s.status='published' AND s.rights_status='allowed' AND b.review_status IN ('accepted','corrected')`, id, principal.Role, principal.ID).Scan(&chunkID, &sourceID, &assetID, &revisionID, &publicationID, &blockID, &locationID, &exact, &title, &author, &sectionKey, &original, &reviewed, &sha, &relative, &sourceURL, &format, &start, &end, &snapshot)
	if err != nil {
		fail(w, 404, "citation not available")
		return
	}
	runes := []rune(reviewed)
	if start < 0 || end > len(runes) || end <= start || string(runes[start:end]) != exact {
		fail(w, 409, "saved passage offsets differ from reviewed document")
		return
	}
	if relative != filepath.Join("data/runtime/assets", sha+"."+format) {
		fail(w, 409, "saved document location differs from checksum")
		return
	}
	raw, err := os.ReadFile(filepath.Join(a.Store.Root, relative))
	if err != nil {
		fail(w, 503, "saved document unavailable")
		return
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != sha {
		fail(w, 409, "saved document checksum differs")
		return
	}
	var label struct {
		Categories       []string `json:"categories"`
		EvidenceCategory string   `json:"evidence_category"`
	}
	_ = json.Unmarshal(snapshot, &label)
	write(w, 200, map[string]any{"id": id, "chunk_id": chunkID, "source_id": sourceID, "source_asset_id": assetID, "asset_sha256": sha, "processing_revision_id": revisionID, "publication_id": publicationID, "document_block_id": blockID, "document_location_id": locationID, "format": format, "section_key": sectionKey, "passage": exact, "title": title, "author": author, "literature_categories": label.Categories, "evidence_category": label.EvidenceCategory, "original_text": original, "reader_text": reviewed, "start_character": start, "end_character": end, "original_url": sourceURL, "document_url": "/api/v1/sources/" + sourceID.String() + "/document/" + revisionID.String() + "/raw", "reader_url": "/api/v1/sources/" + sourceID.String() + "/document/" + revisionID.String() + "/reader"})
}

func (a *API) rawDocument(w http.ResponseWriter, r *http.Request) {
	sourceID, ok := parseID(w, r)
	if !ok {
		return
	}
	revisionID, err := uuid.Parse(r.PathValue("revision"))
	if err != nil {
		fail(w, 400, "invalid revision ID")
		return
	}
	var sha, relative, format, status, rights string
	err = a.Store.DB.QueryRow(r.Context(), `SELECT sa.sha256,sa.object_locator,sa.kind,s.status,s.rights_status FROM processing_revisions rev JOIN source_assets sa ON sa.id=rev.source_asset_id JOIN sources s ON s.id=rev.source_id WHERE rev.id=$1 AND rev.source_id=$2 AND sa.kind IN ('html','txt')`, revisionID, sourceID).Scan(&sha, &relative, &format, &status, &rights)
	if err != nil {
		fail(w, 404, "document not found")
		return
	}
	if requestRole(r.Context()) != "admin" && (status != "published" || rights != "allowed") {
		fail(w, 403, "document unavailable")
		return
	}
	if relative != filepath.Join("data/runtime/assets", sha+"."+format) {
		fail(w, 409, "saved document location differs from checksum")
		return
	}
	raw, err := os.ReadFile(filepath.Join(a.Store.Root, relative))
	if err != nil {
		fail(w, 503, "saved document unavailable")
		return
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != sha {
		fail(w, 409, "saved document checksum differs")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=original."+format+".txt")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(raw)
}
