package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// PDFObjects keeps durable originals separate from disposable processing files.
type PDFObjects interface {
	Put(context.Context, string, *os.File) error
	Get(context.Context, string) (io.ReadCloser, error)
	Check(context.Context) error
	Location(string) string
}

type b2Objects struct {
	client *s3.Client
	bucket string
}

var pdfHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func objectKey(sha string) string               { return "pdf/" + sha + ".pdf" }
func (b *b2Objects) Location(sha string) string { return "s3://" + b.bucket + "/" + objectKey(sha) }
func (b *b2Objects) Put(ctx context.Context, sha string, f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	_, err = b.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(objectKey(sha)), Body: f, ContentLength: aws.Int64(info.Size()), ContentType: aws.String("application/pdf"), Metadata: map[string]string{"sha256": sha}})
	return err
}
func (b *b2Objects) Get(ctx context.Context, sha string) (io.ReadCloser, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(objectKey(sha))})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}
func (b *b2Objects) Check(ctx context.Context) error {
	_, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(b.bucket)})
	return err
}

func (s *Store) CheckPDFStorage(ctx context.Context) error {
	if s.PDFObjects == nil {
		return nil
	}
	return s.PDFObjects.Check(ctx)
}

// ConfigurePDFStorage fails closed on incomplete B2 configuration. Local mode is
// retained only for development and migration; production Compose requires B2.
func (s *Store) ConfigurePDFStorage() error {
	provider := strings.TrimSpace(os.Getenv("PDF_STORAGE_PROVIDER"))
	if provider == "local" {
		s.PDFObjects = nil
		return nil
	}
	if provider != "b2" {
		return errors.New("PDF_STORAGE_PROVIDER must be b2 (or explicitly local for development)")
	}
	region, bucket := os.Getenv("B2_REGION"), os.Getenv("B2_BUCKET")
	key, secret := os.Getenv("B2_KEY_ID"), os.Getenv("B2_APPLICATION_KEY")
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(region) || !regexp.MustCompile(`^[a-zA-Z0-9-]{6,63}$`).MatchString(bucket) || key == "" || secret == "" {
		return errors.New("B2_REGION, B2_BUCKET, B2_KEY_ID and B2_APPLICATION_KEY must be configured")
	}
	endpoint := "https://s3." + region + ".backblazeb2.com"
	client := s3.New(s3.Options{Region: region, BaseEndpoint: &endpoint, UsePathStyle: true, Credentials: credentials.NewStaticCredentialsProvider(key, secret, ""), HTTPClient: &http.Client{Timeout: 4 * time.Minute}, Retryer: retry.NewStandard(func(o *retry.StandardOptions) { o.MaxAttempts = 3 }), RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired})
	s.PDFObjects = &b2Objects{client: client, bucket: bucket}
	return nil
}

func verifyPDFFile(path, sha string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != sha {
		return errors.New("PDF checksum mismatch")
	}
	return nil
}

// StorePDF returns a durable locator only after the stored bytes are verified.
func (s *Store) StorePDF(ctx context.Context, path, sha string) (string, error) {
	if !pdfHash.MatchString(sha) {
		return "", errors.New("invalid PDF checksum")
	}
	if err := verifyPDFFile(path, sha); err != nil {
		return "", err
	}
	if s.PDFObjects == nil {
		dest := s.PDFPath(sha)
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return "", err
		}
		// Copy into the destination filesystem before an atomic link (TMPDIR may differ).
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		tmp, err := os.CreateTemp(filepath.Dir(dest), "incoming-*.pdf")
		if err != nil {
			return "", err
		}
		defer os.Remove(tmp.Name())
		_, err = io.Copy(tmp, f)
		if err == nil {
			err = tmp.Sync()
		}
		if err == nil {
			err = tmp.Chmod(0400)
		}
		closeErr := tmp.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
		if err = os.Link(tmp.Name(), dest); err != nil && !os.IsExist(err) {
			return "", err
		}
		return dest, verifyPDFFile(dest, sha)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err = s.PDFObjects.Put(ctx, sha, f); err != nil {
		return "", fmt.Errorf("store PDF object: %w", err)
	}
	_, cleanup, err := s.AcquirePDF(ctx, sha)
	if err != nil {
		return "", err
	}
	cleanup()
	return s.PDFObjects.Location(sha), nil
}

// AcquirePDF returns a verified processing file and a mandatory cleanup callback.
// B2 failures never silently fall back to an old local copy.
func (s *Store) AcquirePDF(ctx context.Context, sha string) (string, func(), error) {
	noop := func() {}
	if !pdfHash.MatchString(sha) {
		return "", noop, errors.New("invalid PDF checksum")
	}
	if s.PDFObjects == nil {
		path := s.PDFPath(sha)
		return path, noop, verifyPDFFile(path, sha)
	}
	readCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	body, err := s.PDFObjects.Get(readCtx, sha)
	if err != nil {
		return "", noop, fmt.Errorf("read PDF object: %w", err)
	}
	defer body.Close()
	f, err := os.CreateTemp("", "source-*.pdf")
	if err != nil {
		return "", noop, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(body, (1<<30)+1))
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil && (n > 1<<30 || hex.EncodeToString(h.Sum(nil)) != sha) {
		err = errors.New("stored PDF failed size/checksum verification")
	}
	if err != nil {
		cleanup()
		return "", noop, err
	}
	return f.Name(), cleanup, nil
}

// MigratePDFs copies existing local originals without deleting them or changing
// source identities. Re-running is safe after an interrupted migration.
func (s *Store) MigratePDFs(ctx context.Context) error {
	if s.PDFObjects == nil {
		return errors.New("migration requires B2 storage")
	}
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT pdf_sha256 FROM sources WHERE pdf_path NOT LIKE 's3://%' AND pdf_sha256 NOT IN (SELECT sha256 FROM pdf_object_locations WHERE bucket=$1)`, strings.TrimSuffix(s.PDFObjects.Location(""), "/"+objectKey("")))
	if err != nil {
		return err
	}
	var hashes []string
	for rows.Next() {
		var sha string
		if err = rows.Scan(&sha); err != nil {
			rows.Close()
			return err
		}
		hashes = append(hashes, sha)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, sha := range hashes {
		locator, err := s.StorePDF(ctx, s.PDFPath(sha), sha)
		if err != nil {
			return fmt.Errorf("migrate PDF %s: %w", sha, err)
		}
		if _, err = s.DB.Exec(ctx, `INSERT INTO pdf_object_locations(sha256,bucket,locator) VALUES($1,$2,$3) ON CONFLICT(sha256,bucket) DO UPDATE SET locator=EXCLUDED.locator,verified_at=now()`, sha, strings.TrimSuffix(s.PDFObjects.Location(""), "/"+objectKey("")), locator); err != nil {
			return err
		}
	}
	return nil
}
