package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestB2PDFRoundTripAndIntegrity(t *testing.T) {
	data := []byte("%PDF-1.4 original fixture")
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	var stored []byte
	corrupt := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/private-bucket/pdf/"+sha+".pdf" {
			t.Errorf("unexpected key: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("unsigned request")
		}
		switch r.Method {
		case "PUT":
			stored, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
		case "GET":
			if corrupt {
				_, _ = w.Write([]byte("corrupt"))
			} else {
				_, _ = w.Write(stored)
			}
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	client := s3.New(s3.Options{Region: "us-west-004", BaseEndpoint: aws.String(server.URL), UsePathStyle: true, Credentials: credentials.NewStaticCredentialsProvider("key", "secret", ""), RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired})
	s := &Store{Root: t.TempDir(), PDFObjects: &b2Objects{client: client, bucket: "private-bucket"}}
	input := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	locator, err := s.StorePDF(context.Background(), input, sha)
	if err != nil {
		t.Fatal(err)
	}
	if locator != "s3://private-bucket/pdf/"+sha+".pdf" {
		t.Fatal(locator)
	}
	if _, err = os.Stat(s.PDFPath(sha)); !os.IsNotExist(err) {
		t.Fatal("durable local PDF created")
	}
	path, cleanup, err := s.AcquirePDF(context.Background(), sha)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(data) {
		t.Fatal("wrong content")
	}
	cleanup()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("temporary PDF not removed")
	}
	corrupt = true
	if _, cleanup, err = s.AcquirePDF(context.Background(), sha); err == nil {
		cleanup()
		t.Fatal("corrupt object accepted")
	}
	if _, err = s.StorePDF(context.Background(), input, strings.Repeat("0", 64)); err == nil {
		t.Fatal("bad upload hash accepted")
	}
	if _, _, err = s.AcquirePDF(context.Background(), "../../escape"); err == nil {
		t.Fatal("invalid key accepted")
	}
}

func TestPDFStorageConfigurationFailsClosed(t *testing.T) {
	for _, key := range []string{"PDF_STORAGE_PROVIDER", "B2_REGION", "B2_BUCKET", "B2_KEY_ID", "B2_APPLICATION_KEY"} {
		t.Setenv(key, "")
	}
	s := &Store{}
	if s.ConfigurePDFStorage() == nil {
		t.Fatal("missing provider accepted")
	}
	t.Setenv("PDF_STORAGE_PROVIDER", "b2")
	if s.ConfigurePDFStorage() == nil {
		t.Fatal("incomplete B2 config accepted")
	}
	t.Setenv("PDF_STORAGE_PROVIDER", "local")
	if err := s.ConfigurePDFStorage(); err != nil {
		t.Fatal(err)
	}
}
