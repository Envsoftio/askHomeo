package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"homeopath-poc/backend/internal/core"
	"homeopath-poc/backend/internal/httpapi"
	"homeopath-poc/backend/internal/ingest"
	"homeopath-poc/backend/internal/localllm"
)

func main() {
	mode := flag.String("mode", "api", "api or worker")
	flag.Parse()
	root := os.Getenv("PROJECT_ROOT")
	if root == "" {
		log.Fatal("PROJECT_ROOT required")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	store, err := core.Open(ctx, dbURL, root)
	if err != nil {
		log.Fatal(err)
	}
	defer store.DB.Close()
	migrateCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err = store.Migrate(migrateCtx)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := localllm.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	model := localllm.New(cfg)
	if *mode == "worker" {
		workerCtx, cancelWorker := context.WithCancel(ctx)
		defer cancelWorker()
		go func() {
			if answerErr := httpapi.RunAnswerWorker(workerCtx, store, model); answerErr != nil && !errors.Is(answerErr, context.Canceled) {
				log.Printf("answer worker stopped: %v", answerErr)
				cancelWorker()
			}
		}()
		if err = ingest.New(store, model).Run(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
		return
	}
	if *mode != "api" {
		log.Fatal("mode must be api or worker")
	}
	token := httpapi.Token()
	if len(token) < 24 {
		log.Fatal("API_TOKEN must have at least 24 characters")
	}
	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	api := &httpapi.API{Store: store, Token: token, ReviewerToken: os.Getenv("REVIEWER_TOKEN"), Model: model}
	if raw := os.Getenv("AUTH_PRINCIPALS_JSON"); raw != "" {
		if err = json.Unmarshal([]byte(raw), &api.Principals); err != nil {
			log.Fatalf("AUTH_PRINCIPALS_JSON: %v", err)
		}
	}
	if err = api.SyncPrincipals(ctx); err != nil {
		log.Fatalf("configure principals: %v", err)
	}
	server := &http.Server{Addr: addr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 260 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("API listening on %s", addr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
