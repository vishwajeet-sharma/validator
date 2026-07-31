package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration sourced from environment variables.
type Config struct {
	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string

	// HTTPAddr is the listen address for the public REST API.
	HTTPAddr string

	// RestateDeploymentAddr is the listen address the Restate runtime dials
	// to invoke registered services/workflows (the SDK deployment endpoint).
	RestateDeploymentAddr string

	// RestateIngressURL is the base URL of the Restate runtime ingress that the
	// API server talks to in order to start workflows.
	RestateIngressURL string

	// RestateAuthKey is an optional bearer token used against the Restate ingress.
	RestateAuthKey string

	// RestateIdentityKey is the Restate Cloud request-identity public key
	// (publickeyv1_...). When set, the worker verifies that incoming requests
	// are signed by the matching Restate Cloud environment. Empty = no check
	// (local dev with a self-hosted Restate runtime).
	RestateIdentityKey string

	// YutoriAPIKey authenticates against the Yutori Research/Scouting APIs.
	YutoriAPIKey string
	// YutoriAPIBase is the root URL of the Yutori API.
	YutoriAPIBase string
	// YutoriTimeout caps a single HTTP call to the Yutori API.
	YutoriTimeout time.Duration

	// WebhookPublicURL is the public base URL Yutori calls back with scout
	// updates. The worker appends /api/webhooks/yutori. Empty disables webhook
	// delivery (scouts still run, but signals won't flow back automatically).
	WebhookPublicURL string

	// LLMAPIBase / LLMAPIKey / LLMModel configure the OpenAI-compatible chat
	// client used for the scout prompt-mutation evaluation. This deliberately
	// does NOT use the Yutori Research API (which is paid and reserved for Day 0
	// research + recurring scouts). When LLM_API_KEY is empty the mutation eval
	// is skipped gracefully (no external call, no proposal).
	LLMAPIBase string
	LLMAPIKey  string
	LLMModel   string
	LLMTimeout time.Duration

	// ScoutIntervalSeconds overrides the recurring-scout output_interval so
	// local testing gets fresh data quickly instead of waiting days. 0 = use the
	// idea's scoutingFrequencyDays (production behaviour).
	ScoutIntervalSeconds int
}

// Load reads configuration from the environment, applying safe defaults.
// It first loads variables from a .env file in the working directory if
// present; existing environment variables take precedence over .env values.
func Load() (Config, error) {
	_ = godotenv.Load()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	cfg := Config{
		DatabaseURL:           databaseURL,
		HTTPAddr:              envOrDefault("HTTP_ADDR", ":8080"),
		RestateDeploymentAddr: envOrDefault("RESTATE_DEPLOYMENT_ADDR", ":9080"),
		RestateIngressURL:     envOrDefault("RESTATE_INGRESS_URL", "http://localhost:8080"),
		RestateAuthKey:        strings.TrimSpace(os.Getenv("RESTATE_AUTH_KEY")),
		RestateIdentityKey:    strings.TrimSpace(os.Getenv("RESTATE_IDENTITY_KEY")),
		YutoriAPIKey:          strings.TrimSpace(os.Getenv("YUTORI_API_KEY")),
		YutoriAPIBase:         strings.TrimRight(envOrDefault("YUTORI_API_BASE", "https://api.yutori.com"), "/"),
		YutoriTimeout:         time.Duration(envIntOrDefault("YUTORI_TIMEOUT_SECONDS", 60)) * time.Second,
		WebhookPublicURL:      strings.TrimRight(os.Getenv("WEBHOOK_PUBLIC_URL"), "/"),
		LLMAPIBase:            strings.TrimRight(envOrDefault("LLM_API_BASE", "https://api.openai.com/v1"), "/"),
		LLMAPIKey:             strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		LLMModel:              envOrDefault("LLM_MODEL", "gpt-4o-mini"),
		LLMTimeout:            time.Duration(envIntOrDefault("LLM_TIMEOUT_SECONDS", 30)) * time.Second,
		ScoutIntervalSeconds:  envIntOrDefault("SCOUT_INTERVAL_SECONDS", 0),
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
