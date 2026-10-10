package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	DB         *pgxpool.Pool
	Root       string
	PDFObjects PDFObjects
}

func (s *Store) PDFPath(sha string) string {
	return filepath.Join(s.Root, "data/runtime/assets", strings.TrimSpace(sha)+".pdf")
}

type Asset struct {
	Kind, LocalPath, SHA256 string
	Bytes                   int64
}
type Manifest struct {
	Sources []Seed `json:"sources"`
}
type Seed struct {
	SourceKey       string `json:"source_key"`
	CanonicalTitle  string `json:"canonical_title"`
	Author          string `json:"author"`
	PublicationYear int    `json:"publication_year"`
	SourceURL       string `json:"source_url"`
	PDFPageCount    int    `json:"pdf_page_count"`
	PageMapPath     string `json:"page_map_path"`
	Rights          struct {
		Mark            string `json:"mark"`
		RightsSourceURL string `json:"rights_source_url"`
		ReviewStatus    string `json:"review_status"`
	} `json:"rights"`
	Assets []AssetJSON `json:"assets"`
}
type AssetJSON struct {
	Kind      string `json:"kind"`
	LocalPath string `json:"local_path"`
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
}
type PageMap struct {
	Pages []MappedPage `json:"pages"`
}
type MappedPage struct {
	ScanPageIndex int     `json:"scan_page_index"`
	PDFPageIndex  int     `json:"pdf_page_index"`
	CanvasID      string  `json:"canvas_id"`
	ALTOURL       string  `json:"alto_url"`
	ImageURL      string  `json:"image_url"`
	PrintedLabel  *string `json:"printed_page_label_verified"`
}

func Open(ctx context.Context, databaseURL, root string) (*Store, error) {
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	ping, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = db.Ping(ping); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{DB: db, Root: root}, nil
}
func (s *Store) Migrate(ctx context.Context) error {
	files, err := filepath.Glob(filepath.Join(s.Root, "apps/backend/migrations/*.sql"))
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(918151)`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	for _, file := range files {
		name := filepath.Base(file)
		var existing string
		err = tx.QueryRow(ctx, `SELECT name FROM schema_migrations WHERE name=$1`, name).Scan(&existing)
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		if _, err = tx.Exec(ctx, string(b)); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Starter(sourceKey string) (Seed, error) {
	b, err := os.ReadFile(filepath.Join(s.Root, "data/starter-corpus/manifest.json"))
	if err != nil {
		return Seed{}, err
	}
	var m Manifest
	if err = json.Unmarshal(b, &m); err != nil {
		return Seed{}, err
	}
	var match Seed
	for _, v := range m.Sources {
		if v.SourceKey == sourceKey {
			return v, nil
		}
		if strings.HasPrefix(v.SourceKey, sourceKey+"-") {
			if match.SourceKey != "" {
				return Seed{}, fmt.Errorf("%s matches multiple starter sources", sourceKey)
			}
			match = v
		}
	}
	if match.SourceKey != "" {
		return match, nil
	}
	return Seed{}, fmt.Errorf("%s missing from starter manifest", sourceKey)
}
func (s *Store) Nash() (Seed, error) { return s.Starter("nash") }
func (s *Store) ImportNash(ctx context.Context) (uuid.UUID, error) {
	return s.ImportStarter(ctx, "nash")
}
func (s *Store) ImportStarter(ctx context.Context, sourceKey string) (uuid.UUID, error) {
	seed, err := s.Starter(sourceKey)
	if err != nil {
		return uuid.Nil, err
	}
	var asset *AssetJSON
	for i := range seed.Assets {
		if seed.Assets[i].Kind == "pdf" {
			asset = &seed.Assets[i]
			break
		}
	}
	if asset == nil {
		return uuid.Nil, fmt.Errorf("%s PDF missing from manifest", sourceKey)
	}
	path := filepath.Join(s.Root, asset.LocalPath)
	f, err := os.Open(path)
	if err != nil {
		return uuid.Nil, fmt.Errorf("open %s PDF: %w", sourceKey, err)
	}
	defer f.Close()
	if err = verifyPDFFile(path, asset.SHA256); err != nil {
		return uuid.Nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return uuid.Nil, err
	}
	n := info.Size()
	if n != asset.Bytes {
		return uuid.Nil, errors.New("PDF size differs from manifest")
	}
	tx, err := s.BeginAssetWrite(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	storedPath, err := s.StorePDF(ctx, path, asset.SHA256)
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	job := uuid.New()
	err = tx.QueryRow(ctx, `INSERT INTO sources(id,source_key,title,author,publication_year,source_url,pdf_path,pdf_sha256,pdf_bytes,page_count,status)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'queued') ON CONFLICT(source_key) DO UPDATE SET source_key=EXCLUDED.source_key WHERE sources.pdf_sha256=EXCLUDED.pdf_sha256 RETURNING id`, id, seed.SourceKey, seed.CanonicalTitle, seed.Author, seed.PublicationYear, seed.SourceURL, storedPath, asset.SHA256, n, seed.PDFPageCount).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("save source: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs(id,source_id,kind,status,current_step,total) VALUES($1,$2,'ingest','queued','Reading scanned pages',$3) ON CONFLICT(source_id,kind) DO NOTHING`, job, id, seed.PDFPageCount-1)
	if err != nil {
		return uuid.Nil, fmt.Errorf("enqueue ingestion: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit import: %w", err)
	}
	return id, nil
}
func (s *Store) PageMap(seed Seed) (PageMap, error) {
	var p PageMap
	b, err := os.ReadFile(filepath.Join(s.Root, seed.PageMapPath))
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(b, &p)
	return p, err
}

// ImportPDF stores an uploaded PDF by checksum before atomically creating its source and job.
func (s *Store) ImportPDF(ctx context.Context, input io.Reader, title, author, edition, publication, repository, sourceURL, rightsStatement string) (uuid.UUID, error) {
	return s.ImportPDFWithOrigin(ctx, input, title, author, edition, publication, repository, sourceURL, rightsStatement, "")
}

// ImportPDFWithOrigin also records the exact remote PDF URL used for acquisition.
func (s *Store) ImportPDFWithOrigin(ctx context.Context, input io.Reader, title, author, edition, publication, repository, sourceURL, rightsStatement, pdfOriginURL string) (uuid.UUID, error) {
	if sourceURL != "" {
		u, err := url.Parse(sourceURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return uuid.Nil, errors.New("source URL must be an HTTP or HTTPS address")
		}
	}
	tmp, err := os.CreateTemp("", "upload-*.pdf")
	if err != nil {
		return uuid.Nil, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(input, (250<<20)+1))
	if err != nil {
		tmp.Close()
		return uuid.Nil, err
	}
	if n < 16 || n > 250<<20 {
		tmp.Close()
		return uuid.Nil, errors.New("PDF must be between 16 bytes and 250 MB")
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return uuid.Nil, err
	}
	if err = tmp.Chmod(0400); err != nil {
		tmp.Close()
		return uuid.Nil, err
	}
	if err = tmp.Close(); err != nil {
		return uuid.Nil, err
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		return uuid.Nil, err
	}
	header := make([]byte, 5)
	_, err = io.ReadFull(f, header)
	f.Close()
	if err != nil || string(header) != "%PDF-" {
		return uuid.Nil, errors.New("file is not a PDF")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, "gs", "-q", "-dNODISPLAY", "--permit-file-read="+tmp.Name(), "-sPDF="+tmp.Name(), "-c", "PDF (r) file runpdfbegin pdfpagecount = quit")
	out, err := cmd.Output()
	if err != nil {
		return uuid.Nil, fmt.Errorf("PDF could not be opened: %w", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || count < 1 || count > 2000 {
		return uuid.Nil, errors.New("PDF must have 1 to 2000 pages")
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(author) == "" || strings.TrimSpace(edition) == "" || strings.TrimSpace(publication) == "" || strings.TrimSpace(repository) == "" || strings.TrimSpace(sourceURL) == "" {
		found := detectPDFMetadata(ctx, tmp.Name(), count, strings.TrimSpace(title) == "" || strings.TrimSpace(author) == "")
		if strings.TrimSpace(title) == "" {
			title = found.Title
		}
		if strings.TrimSpace(author) == "" {
			author = found.Author
		}
		if strings.TrimSpace(edition) == "" {
			edition = found.Edition
		}
		if strings.TrimSpace(publication) == "" {
			publication = found.Publication
		}
		if strings.TrimSpace(repository) == "" {
			repository = found.Repository
		}
		if strings.TrimSpace(sourceURL) == "" {
			sourceURL = found.SourceURL
		}
	}
	if strings.TrimSpace(title) == "" {
		title = "Untitled PDF (verify details)"
	}
	if strings.TrimSpace(author) == "" {
		author = "Unknown author (verify details)"
	}
	if strings.TrimSpace(sourceURL) == "" {
		sourceURL = strings.TrimSpace(pdfOriginURL)
	}
	tx, err := s.BeginAssetWrite(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	sha := hex.EncodeToString(h.Sum(nil))
	stored, err := s.StorePDF(ctx, tmp.Name(), sha)
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO sources(id,source_key,title,author,source_url,pdf_path,pdf_sha256,pdf_bytes,page_count,status,edition,publication_info,repository,rights_statement,pdf_origin_url)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'queued',$10,$11,$12,$13,$14)`, id, "upload-"+id.String(), strings.TrimSpace(title), strings.TrimSpace(author), strings.TrimSpace(sourceURL), stored, sha, n, count+1, strings.TrimSpace(edition), strings.TrimSpace(publication), strings.TrimSpace(repository), strings.TrimSpace(rightsStatement), strings.TrimSpace(pdfOriginURL))
	if err != nil {
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs(id,source_id,kind,status,current_step,total) VALUES($1,$2,'ingest','queued','Reading PDF pages',$3)`, uuid.New(), id, count)
	if err != nil {
		return uuid.Nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
