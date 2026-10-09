// check-b2 verifies access to the configured private B2 bucket without
// uploading, listing or downloading source material.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/aws/smithy-go"
	"homeopath-poc/backend/internal/core"
)

func main() {
	if os.Getenv("PDF_STORAGE_PROVIDER") != "b2" {
		fmt.Fprintln(os.Stderr, "PDF_STORAGE_PROVIDER must be b2")
		os.Exit(1)
	}
	store := &core.Store{}
	if err := store.ConfigurePDFStorage(); err != nil {
		fmt.Fprintln(os.Stderr, "B2 configuration is incomplete")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := store.CheckPDFStorage(ctx); err != nil {
		var apiErr smithy.APIError
		var networkErr net.Error
		switch {
		case errors.As(err, &apiErr):
			fmt.Fprintln(os.Stderr, "B2 bucket check failed:", apiErr.ErrorCode())
		case errors.As(err, &networkErr), errors.Is(err, context.DeadlineExceeded):
			fmt.Fprintln(os.Stderr, "B2 bucket check failed: network unavailable or timed out")
		default:
			fmt.Fprintln(os.Stderr, "B2 bucket check failed; verify endpoint and key permissions")
		}
		os.Exit(1)
	}
	fmt.Println("B2 bucket check passed")
}
