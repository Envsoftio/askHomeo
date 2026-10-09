package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/document"
)

type DocumentImport struct {
	Title, Author, Edition, PublicationInfo, Repository, SourceURL, RightsStatement string
	RequestedURL, FinalURL, Transport, ContentType, CharsetOverride                 string
	SupersedesSourceID                                                              *uuid.UUID
	LiteratureCategories                                                            []string
	EvidenceCategory                                                                string
	LiteratureCategoryOrigin                                                        string
	CategoryActorID                                                                 *uuid.UUID
}

// ImportDocument validates a single snapshot and queues extraction. The saved
// bytes are content-addressed and never exposed through a raw document route.
func (s *Store) ImportDocument(ctx context.Context, input io.Reader, info DocumentImport) (uuid.UUID, error) {
	raw, err := io.ReadAll(io.LimitReader(input, document.MaxBytes+1))
	if err != nil {
		return uuid.Nil, err
	}
	if len(raw) > document.MaxBytes {
		return uuid.Nil, errors.New("document exceeds 10 MiB limit")
	}
	format, err := document.Detect(raw, info.ContentType)
	if err != nil {
		return uuid.Nil, err
	}
	if format != "html" && format != "txt" {
		return uuid.Nil, errors.New("only HTML and TXT are accepted")
	}
	extracted, err := document.Extract(raw, info.ContentType, info.CharsetOverride)
	if err != nil {
		return uuid.Nil, err
	}
	if len(extracted.Blocks) == 0 {
		return uuid.Nil, errors.New("document has no reviewable text")
	}
	var sample strings.Builder
	for _, block := range extracted.Blocks {
		sample.WriteString(" ")
		sample.WriteString(block.Text)
		if sample.Len() > 600 {
			break
		}
	}
	if sample.Len() < 600 {
		lower := strings.ToLower(sample.String())
		for _, marker := range []string{"sign in to continue", "log in to continue", "access denied", "404 not found", "page not found"} {
			if strings.Contains(lower, marker) {
				return uuid.Nil, errors.New("document appears to be a login or error page")
			}
		}
	}
	if info.Transport != "upload" && info.Transport != "http" && info.Transport != "https" {
		return uuid.Nil, errors.New("invalid acquisition transport")
	}
	if info.SourceURL != "" {
		u, e := url.Parse(info.SourceURL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return uuid.Nil, errors.New("source URL must be an HTTP or HTTPS address")
		}
	}
	title := strings.TrimSpace(info.Title)
	if title == "" {
		for _, block := range extracted.Blocks {
			if block.Kind == "heading" {
				title = block.Text
				break
			}
		}
	}
	if title == "" {
		title = "Untitled document (verify details)"
	}
	author := strings.TrimSpace(info.Author)
	if author == "" {
		author = "Unknown author (verify details)"
	}
	hash := sha256.Sum256(raw)
	sha := hex.EncodeToString(hash[:])
	relative := filepath.Join("data/runtime/assets", sha+"."+format)
	path := filepath.Join(s.Root, relative)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return uuid.Nil, err
	}
	if existing, e := os.ReadFile(path); e == nil {
		if !bytes.Equal(existing, raw) {
			return uuid.Nil, errors.New("stored document checksum collision")
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return uuid.Nil, e
	} else {
		tmp, e := os.CreateTemp(filepath.Dir(path), ".document-*")
		if e != nil {
			return uuid.Nil, e
		}
		defer os.Remove(tmp.Name())
		if _, e = tmp.Write(raw); e != nil {
			tmp.Close()
			return uuid.Nil, e
		}
		if e = tmp.Sync(); e != nil {
			tmp.Close()
			return uuid.Nil, e
		}
		if e = tmp.Chmod(0400); e != nil {
			tmp.Close()
			return uuid.Nil, e
		}
		if e = tmp.Close(); e != nil {
			return uuid.Nil, e
		}
		if e = os.Rename(tmp.Name(), path); e != nil {
			return uuid.Nil, e
		}
	}
	id := uuid.New()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO sources(id,source_key,title,author,source_url,status,edition,publication_info,repository,rights_statement,document_format,document_object_locator,document_sha256,document_bytes,document_content_type,document_charset_override,supersedes_source_id)
 VALUES($1,$2,$3,$4,$5,'queued',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, id, "document-"+id.String(), title, author, strings.TrimSpace(info.SourceURL), strings.TrimSpace(info.Edition), strings.TrimSpace(info.PublicationInfo), strings.TrimSpace(info.Repository), strings.TrimSpace(info.RightsStatement), format, relative, sha, len(raw), info.ContentType, info.CharsetOverride, info.SupersedesSourceID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("save document source: %w", err)
	}
	if len(info.LiteratureCategories) > 0 {
		_, err = tx.Exec(ctx, `UPDATE sources SET literature_categories=$2,evidence_category=$3,literature_category_origin=$4 WHERE id=$1`, id, info.LiteratureCategories, info.EvidenceCategory, info.LiteratureCategoryOrigin)
		if err != nil {
			return uuid.Nil, err
		}
		if info.CategoryActorID != nil {
			_, err = tx.Exec(ctx, `INSERT INTO literature_category_decisions(id,source_id,processing_revision_id,actor_principal_id,previous_categories,chosen_categories,previous_evidence_category,chosen_evidence_category,origin,rationale)
 SELECT $2,id,current_revision_id,$3,ARRAY['unclassified']::text[],$4,'unknown',$5,'manual','Administrator selected literature category at intake' FROM sources WHERE id=$1`, id, uuid.New(), info.CategoryActorID, info.LiteratureCategories, info.EvidenceCategory)
			if err != nil {
				return uuid.Nil, err
			}
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO document_acquisitions(source_id,source_asset_id,requested_url,final_url,transport,content_type)
 SELECT id,primary_asset_id,$2,$3,$4,$5 FROM sources WHERE id=$1`, id, info.RequestedURL, info.FinalURL, info.Transport, info.ContentType)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs(id,source_id,kind,status,current_step,total) VALUES($1,$2,'ingest','queued','Extracting document blocks',1)`, uuid.New(), id)
	if err != nil {
		return uuid.Nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
