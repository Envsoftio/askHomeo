package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"homeopath-poc/backend/internal/document"
)

func (w *Worker) processDocument(ctx context.Context, jobID, sourceID uuid.UUID) error {
	var relative, sha, contentType, override, format string
	var assetID, revisionID uuid.UUID
	err := w.Store.DB.QueryRow(ctx, `SELECT document_object_locator,document_sha256,document_content_type,document_charset_override,document_format,primary_asset_id,current_revision_id FROM sources WHERE id=$1`, sourceID).Scan(&relative, &sha, &contentType, &override, &format, &assetID, &revisionID)
	if err != nil {
		return err
	}
	if relative != filepath.Join("data/runtime/assets", sha+"."+format) {
		return errors.New("document object location differs from immutable checksum")
	}
	raw, err := os.ReadFile(filepath.Join(w.Store.Root, relative))
	if err != nil {
		return err
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != sha {
		return errors.New("saved document checksum differs from source asset")
	}
	result, err := document.Extract(raw, contentType, override)
	if err != nil {
		return fmt.Errorf("extract saved document: %w", err)
	}
	if result.Format != format {
		return errors.New("saved document format changed during extraction")
	}
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, block := range result.Blocks {
		_, err = tx.Exec(ctx, `INSERT INTO document_blocks(id,source_id,source_asset_id,processing_revision_id,block_index,section_key,kind,heading,original_text,reviewed_text,start_byte,end_byte,warnings)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$10,$11,$12) ON CONFLICT(processing_revision_id,block_index) DO NOTHING`, uuid.New(), sourceID, assetID, revisionID, i, block.Key, block.Kind, block.Heading, block.Text, block.StartByte, block.EndByte, result.Warnings)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO document_locations(id,source_id,source_asset_id,processing_revision_id,kind,section_key)
 VALUES($1,$2,$3,$4,'section',$5) ON CONFLICT DO NOTHING`, uuid.New(), sourceID, assetID, revisionID, block.Key)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE jobs SET status='done',current_step='Review document blocks',completed=total,lease_until=NULL,error=NULL,updated_at=now() WHERE id=$1`, jobID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE sources SET status='review',updated_at=now() WHERE id=$1`, sourceID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE processing_revisions SET status='review',completed_at=now() WHERE id=$1`, revisionID)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	var text string
	for _, block := range result.Blocks {
		text += "\n" + block.Heading + "\n" + block.Text
	}
	w.suggestCategories(ctx, sourceID, revisionID, assetID, text)
	return nil
}
