package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// BeginAssetWrite serializes asset creation/registration with garbage collection.
// It prevents a concurrent re-import from losing its file before registering it.
func (s *Store) BeginAssetWrite(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(918152)`); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

type permanentPDFDeleter interface {
	DeleteAllVersions(context.Context, string) error
}

// CleanupDeletedFile processes one durable cleanup task. Repeated calls are safe,
// including when file deletion succeeded but the transaction could not commit.
func (s *Store) CleanupDeletedFile(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	tx, err := s.BeginAssetWrite(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	var kind, sha, locator string
	err = tx.QueryRow(ctx, `SELECT id,kind,sha256,locator FROM deleted_source_files WHERE retry_after<=now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &kind, &sha, &locator)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var shared bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_assets WHERE sha256=$1 AND kind=$2) OR EXISTS(SELECT 1 FROM collection_items WHERE sha256=$1 AND format=$2)`, sha, kind).Scan(&shared)
	if err != nil {
		return err
	}
	if !shared {
		err = s.deleteManagedAsset(ctx, kind, strings.TrimSpace(sha), locator)
	}
	if err != nil {
		// Do not expose provider errors (which can contain sensitive object details).
		_, saveErr := tx.Exec(ctx, `UPDATE deleted_source_files SET error='Original file cleanup failed; check storage access and retry.',retry_after=now()+interval '1 minute' WHERE id=$1`, id)
		if saveErr != nil {
			return saveErr
		}
		return tx.Commit(ctx)
	}
	if !shared && kind == "pdf" {
		if _, err = tx.Exec(ctx, `DELETE FROM pdf_object_locations WHERE sha256=$1`, sha); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM deleted_source_files WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) deleteManagedAsset(ctx context.Context, kind, sha, locator string) error {
	if !pdfHash.MatchString(sha) || (kind != "pdf" && kind != "html" && kind != "txt") {
		return errors.New("invalid asset identity")
	}
	if kind == "pdf" && s.PDFObjects != nil {
		d, ok := s.PDFObjects.(permanentPDFDeleter)
		if !ok {
			return errors.New("storage does not support permanent deletion")
		}
		// Refuse to erase from a different configured bucket after storage migration.
		if strings.HasPrefix(locator, "s3://") && locator != s.PDFObjects.Location(sha) {
			return errors.New("original bucket is not configured")
		}
		if err := d.DeleteAllVersions(ctx, sha); err != nil {
			return err
		}
	}
	relative := filepath.Join("data/runtime/assets", sha+"."+kind)
	path := filepath.Join(s.Root, relative)
	// Only unlink the content-addressed managed copy, never arbitrary supplied paths.
	if kind != "pdf" && filepath.Clean(locator) != relative && filepath.Clean(locator) != filepath.Clean(path) {
		return fmt.Errorf("unmanaged asset location")
	}
	if kind == "pdf" && s.PDFObjects == nil && strings.HasPrefix(locator, "s3://") {
		return errors.New("original bucket is not configured")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
