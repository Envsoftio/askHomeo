package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"homeopath-poc/backend/internal/document"
)

const maxCombinedTextBytes = 25 << 20

// collectionText is a reading/review artifact. Searchable evidence is still
// published and indexed from reviewed, URL-backed document blocks per page.
func (a *API) collectionText(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var snapshotID uuid.UUID
	var title, author, state string
	var reasons []string
	err := a.Store.DB.QueryRow(r.Context(), `SELECT s.id,c.title,c.author,s.state,s.incomplete_reasons
 FROM linked_collections c JOIN collection_snapshots s ON s.collection_id=c.id
 WHERE c.id=$1 ORDER BY s.generation DESC LIMIT 1`, id).Scan(&snapshotID, &title, &author, &state, &reasons)
	if err == pgx.ErrNoRows {
		fail(w, 404, "collection not found")
		return
	}
	if err != nil {
		fail(w, 500, "could not read collection")
		return
	}
	if state != "review" && state != "active" {
		fail(w, 409, "wait for book capture to finish before reading combined text")
		return
	}
	rows, err := a.Store.DB.Query(r.Context(), `SELECT i.final_url,i.content_type,i.format,coalesce(i.sha256,''),coalesce(i.object_locator,'')
 FROM collection_items i WHERE i.snapshot_id=$1 AND i.state='fetched' AND i.role='content_candidate'
 ORDER BY i.discovery_ordinal`, snapshotID)
	if err != nil {
		fail(w, 500, "could not read saved pages")
		return
	}
	defer rows.Close()
	var out bytes.Buffer
	fmt.Fprintf(&out, "%s\nBy %s\n\n", title, author)
	out.WriteString("UNREVIEWED BOOK TEXT — for inspection only. Publish reviewed pages separately for RAG citations.\n")
	if len(reasons) == 0 {
		out.WriteString("Capture coverage: complete within the selected host, path, and limits.\n")
	} else {
		out.WriteString("Capture coverage: incomplete (" + strings.Join(uniqueStrings(reasons), ", ") + ").\n")
	}
	pageCount := 0
	for rows.Next() {
		var pageURL, contentType, format, sha, locator string
		if err = rows.Scan(&pageURL, &contentType, &format, &sha, &locator); err != nil {
			break
		}
		if sha == "" || locator != filepath.Join("data/runtime/assets", sha+"."+format) {
			err = fmt.Errorf("invalid saved asset identity")
			break
		}
		raw, readErr := os.ReadFile(filepath.Join(a.Store.Root, locator))
		if readErr != nil {
			err = readErr
			break
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != sha {
			err = fmt.Errorf("saved asset checksum mismatch")
			break
		}
		pageCount++
		extracted, extractErr := document.Extract(raw, contentType, "")
		fmt.Fprintf(&out, "\n\n===== PAGE %d =====\nSource URL: %s\n", pageCount, pageURL)
		if extractErr != nil {
			fmt.Fprintf(&out, "Extraction unavailable: %s\n", extractErr)
			continue
		}
		if extracted.Title != "" {
			fmt.Fprintf(&out, "Page title: %s\n", extracted.Title)
		}
		out.WriteString("\n")
		lastHeading := ""
		for _, block := range extracted.Blocks {
			if block.ExclusionReason != "" {
				continue
			}
			if block.Heading != "" && block.Heading != lastHeading {
				fmt.Fprintf(&out, "\n[%s]\n", block.Heading)
				lastHeading = block.Heading
			}
			fmt.Fprintf(&out, "%s\n", block.Text)
		}
		if out.Len() > maxCombinedTextBytes {
			err = fmt.Errorf("combined text exceeds the review export limit")
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		fail(w, 500, "could not assemble combined book text: "+err.Error())
		return
	}
	if pageCount == 0 {
		fail(w, 409, "capture has no reviewable text pages")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out.Bytes())
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
