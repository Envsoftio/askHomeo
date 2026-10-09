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

	"github.com/google/uuid"

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
	model.RecordCall = func(callCtx context.Context, call localllm.Call) {
		owner := localllm.OwnerFromContext(callCtx)
		var ownerID any
		if owner.ID != "" {
			id, parseErr := uuid.Parse(owner.ID)
			if parseErr == nil {
				ownerID = id
			} else {
				owner.Kind = ""
			}
		}
		costOrigin := "unknown"
		if call.EstimatedCostUSD != nil {
			costOrigin = "provider_reported"
		}
		writeCtx, cancelWrite := context.WithTimeout(context.WithoutCancel(callCtx), 3*time.Second)
		defer cancelWrite()
		_, writeErr := store.DB.Exec(writeCtx, `INSERT INTO model_calls(owner_kind,owner_id,kind,provider,requested_model,returned_model,provider_request_id,outcome,prompt_tokens,completion_tokens,total_tokens,reasoning_tokens,estimated_cost_usd,cost_origin,duration_ms) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, owner.Kind, ownerID, call.Kind, call.Provider, call.RequestedModel, call.ReturnedModel, call.RequestID, call.Outcome, call.PromptTokens, call.CompletionTokens, call.TotalTokens, call.ReasoningTokens, call.EstimatedCostUSD, costOrigin, call.Duration.Milliseconds())
		if writeErr != nil {
			log.Printf("save model call metadata: %v", writeErr)
		}
	}
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
	if err = api.ConfigureAdminLogin(os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD")); err != nil {
		log.Fatalf("configure administrator login: %v", err)
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
