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

	// YutoriAPIKey authenticates against the Yutori Research API.
	YutoriAPIKey string
	// YutoriAPIBase is the root URL of the Yutori API.
	YutoriAPIBase string
	// YutoriTimeout caps a single HTTP call to the Yutori API.
	YutoriTimeout time.Duration
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
		HTTPAddr:              envOrDefault("HTTP_ADDR", ":8000"),
		RestateDeploymentAddr: envOrDefault("RESTATE_DEPLOYMENT_ADDR", ":9080"),
		RestateIngressURL:     envOrDefault("RESTATE_INGRESS_URL", "http://localhost:8080"),
		RestateAuthKey:        os.Getenv("RESTATE_AUTH_KEY"),
		YutoriAPIKey:          os.Getenv("YUTORI_API_KEY"),
		YutoriAPIBase:         strings.TrimRight(envOrDefault("YUTORI_API_BASE", "https://api.yutori.com"), "/"),
		YutoriTimeout:         time.Duration(envIntOrDefault("YUTORI_TIMEOUT_SECONDS", 60)) * time.Second,
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
