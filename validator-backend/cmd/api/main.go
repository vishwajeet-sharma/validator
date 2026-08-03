// Command api is the Public API Surface of the Validator platform. It runs on
// the edge (HTTP_ADDR, default :8080) and handles all internet-facing traffic:
// idea onboarding, human proposal responses, and Yutori webhook ingress. It
// writes to Postgres and fire-and-forgets heavy work to the worker binary via
// the Restate ingress.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	restateingress "github.com/restatedev/sdk-go/ingress"

	"validator-backend/internal/api"
	"validator-backend/internal/config"
	"validator-backend/internal/db"
	"validator-backend/internal/llm"
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
		"http_addr", cfg.HTTPAddr,
		"restate_ingress_url", cfg.RestateIngressURL,
		"webhook_public_url", cfg.WebhookPublicURL)

	store, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database init failed", "err", err)
		os.Exit(1)
	}
	defer store.Close() //nolint:errcheck

	var ingressOpts []restateingress.ClientOption
	if cfg.RestateAuthKey != "" {
		ingressOpts = append(ingressOpts, restateingress.WithAuthKey(cfg.RestateAuthKey))
	}
	ingressClient := restateingress.NewClient(cfg.RestateIngressURL, ingressOpts...)

	chatLLM := llm.New(cfg.LLMAPIBase, cfg.LLMAPIKey, cfg.LLMChatModel, cfg.LLMTimeout)
	slog.Info("chat llm client", "configured", chatLLM.Configured(),
		"model", cfg.LLMChatModel, "base", cfg.LLMAPIBase)

	apiServer := api.NewServer(store, ingressClient, chatLLM)
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	httpErrCh := make(chan error, 1)
	go func() {
		slog.Info("public API listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErrCh <- err
		}
	}()

	go func() {
		<-ctx.Done()
		slog.Info("shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	select {
	case err := <-httpErrCh:
		slog.Error("API server failed", "err", err)
		os.Exit(1)
	default:
	}

	<-ctx.Done()
	slog.Info("api surface stopped")
}
