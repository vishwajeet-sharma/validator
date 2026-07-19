// Command worker is the Internal Worker Surface of the Validator platform. It
// runs in a private network (RESTATE_DEPLOYMENT_ADDR, default :9080) bound via
// the Restate SDK server, and performs all heavy computation: the Day 0 setup
// workflow (research -> prompt synthesis -> scout deployment) and the ScoutOps
// service (webhook ingestion + isolated prompt-mutation handling + Yutori
// PATCH on approval).
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	restate "github.com/restatedev/sdk-go"
	"github.com/restatedev/sdk-go/server"

	"validator-backend/internal/config"
	"validator-backend/internal/db"
	"validator-backend/internal/llm"
	"validator-backend/internal/scouts"
	"validator-backend/internal/workflow"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelInfo)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}
	slog.Info("configuration loaded",
		"restate_deployment_addr", cfg.RestateDeploymentAddr,
		"yutori_base", cfg.YutoriAPIBase,
		"webhook_public_url", cfg.WebhookPublicURL)

	store, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database init failed", "err", err)
		os.Exit(1)
	}
	defer store.Close() //nolint:errcheck

	scoutsClient := scouts.New(cfg.YutoriAPIBase, cfg.YutoriAPIKey, cfg.YutoriTimeout,
		// Mutation eval runs on a cheap OpenAI-compatible LLM, never on the
		// Yutori Research API (which is paid). When LLM_API_KEY is empty, the
		// eval is skipped (see ScoutOps.ProcessWebhook).
		scouts.WithLLM(llm.New(cfg.LLMAPIBase, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMTimeout)),
	)

	slog.Info("llm eval client", "configured", scoutsClient.LLMConfigured(),
		"base", cfg.LLMAPIBase, "model", cfg.LLMModel)

	webhookURL := ""
	if cfg.WebhookPublicURL != "" {
		webhookURL = cfg.WebhookPublicURL + "/api/webhooks/yutori"
	}

	restateSrv := server.NewRestate().
		Bind(restate.Reflect(&workflow.Day0SetupWorkflow{
			DB:                   store,
			Scouts:               scoutsClient,
			WebhookURL:           webhookURL,
			ScoutIntervalSeconds: cfg.ScoutIntervalSeconds,
		})).
		Bind(restate.Reflect(&workflow.ScoutOps{
			DB:     store,
			Scouts: scoutsClient,
		}))

	slog.Info("restate deployment listening", "addr", cfg.RestateDeploymentAddr)
	if err := restateSrv.Start(ctx, cfg.RestateDeploymentAddr); err != nil &&
		!errors.Is(err, context.Canceled) {
		slog.Error("restate server exited unexpectedly", "err", err)
		os.Exit(1)
	}

	slog.Info("worker surface stopped")
}
