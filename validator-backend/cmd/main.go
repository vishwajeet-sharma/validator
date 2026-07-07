// Command validator-backend boots the Validator platform: it opens the
// PostgreSQL pool, registers the durable MarketValidationWorkflow with the
// Restate runtime, and serves the public REST ingress layer.
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

	restate "github.com/restatedev/sdk-go"
	restateingress "github.com/restatedev/sdk-go/ingress"
	"github.com/restatedev/sdk-go/server"

	"validator-backend/internal/api"
	"validator-backend/internal/config"
	"validator-backend/internal/db"
	"validator-backend/internal/workflow"
	"validator-backend/internal/yutori"
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
		"restate_deployment_addr", cfg.RestateDeploymentAddr,
		"restate_ingress_url", cfg.RestateIngressURL,
		"yutori_base", cfg.YutoriAPIBase)

	// --- Database -----------------------------------------------------------
	store, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database init failed", "err", err)
		os.Exit(1)
	}
	defer store.Close() //nolint:errcheck

	// --- Yutori AI client ---------------------------------------------------
	yutoriClient := yutori.New(cfg.YutoriAPIBase, cfg.YutoriAPIKey, cfg.YutoriTimeout)

	// --- Restate ingress client (used to trigger workflows) ----------------
	var ingressOpts []restateingress.ClientOption
	if cfg.RestateAuthKey != "" {
		ingressOpts = append(ingressOpts, restateingress.WithAuthKey(cfg.RestateAuthKey))
	}
	ingressClient := restateingress.NewClient(cfg.RestateIngressURL, ingressOpts...)

	// --- REST API server (public ingress) ----------------------------------
	apiServer := api.NewServer(store, ingressClient)
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// --- Restate SDK deployment server (the durable execution runtime) -----
	// The Restate server registers the MarketValidationWorkflow component,
	// injecting the DB store and Yutori client the workflow needs.
	restateSrv := server.NewRestate().Bind(
		restate.Reflect(&workflow.MarketValidationWorkflow{
			DB:     store,
			Yutori: yutoriClient,
		}),
	)

	// Start the REST API on a background goroutine.
	httpErrCh := make(chan error, 1)
	go func() {
		slog.Info("REST API listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErrCh <- err
		}
	}()

	// When a shutdown signal arrives, stop the public REST API gracefully.
	go func() {
		<-ctx.Done()
		slog.Info("shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	// Start the Restate SDK deployment server. This blocks until the context is
	// cancelled by a shutdown signal, at which point it stops cleanly.
	slog.Info("Restate deployment listening", "addr", cfg.RestateDeploymentAddr)
	if err := restateSrv.Start(ctx, cfg.RestateDeploymentAddr); err != nil &&
		!errors.Is(err, context.Canceled) {
		slog.Error("restate server exited unexpectedly", "err", err)
		os.Exit(1)
	}

	select {
	case err := <-httpErrCh:
		slog.Error("REST API server failed", "err", err)
		os.Exit(1)
	default:
	}
	slog.Info("validator backend stopped")
}
