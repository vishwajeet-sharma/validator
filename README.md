# Validator

Validator is a data-driven, evidence-first market research platform designed to eliminate AI hallucination and bias during business validation. Instead of giving users automated, arbitrary AI "success percentages," Validator deploys dual, parallel background web scouts to harvest empirical facts (Pros vs. Cons) from the web. The platform maintains an evolving tracking loop that dynamically modifies its own search parameters over time, serving the final, unfiltered dataset directly to the user to feed into their LLM of choice.

---

## Architectural Paradigm

Validator decouples the long-running tracking schedule and parameter mutations from the web-scraping workload by utilizing **Restate.dev** for stateful, durable orchestration and **Yutori's Research API** as a stateless, deep-diving extraction execution engine.

### Core Architecture Flow

```text
 [User HTTP POST] ──► Save to Postgres (ideas table) ──► Trigger Restate Workflow
                                                                │
                                                                ▼
                                                  ┌───────────────────────────┐
                                                  │   Restate Workflow Loop   │ ◄─────────────────┐
                                                  └─────────────┬─────────────┘                   │
                                                                │                                 │
                                                                ▼                                 │
                                                  ┌───────────────────────────┐                   │
                                                  │ Parallel Yutori Research  │                   │
                                                  │   (Pro-Scout & Con-Scout) │                   │
                                                  └─────────────┬─────────────┘                   │
                                                                │                                 │
                                                                ▼                                 │
                                                  ┌───────────────────────────┐                   │
                                                  │  Poll Tasks to Completion │                   │
                                                  └─────────────┬─────────────┘                   │
                                                                │                                 │
                                                                ▼                                 │
                                                  ┌───────────────────────────┐                   │
                                                  │ Store Raw Signals to DB   │                   │
                                                  └─────────────┬─────────────┘                   │
                                                                │                                 │
                                                                ▼                                 │
                                                  ┌───────────────────────────┐                   │
                                                  │ Parse Text & Mutate K/V   │                   │
                                                  │    (Evolve Watchlist)     │                   │
                                                  └─────────────┬─────────────┘                   │
                                                                │                                 │
                                                                ▼                                 │
                                                  ┌───────────────────────────┐                   │
                                                  │   Restate Durable Sleep   │ ──────────────────┘
                                                  └───────────────────────────┘
```

## The Technology Stack

### Backend (Public API & Worker Surface)
- **Language:** Go 1.25 (module `validator-backend`)
- **Durable Orchestration Engine:** Restate.dev (`restate-server` + `github.com/restatedev/sdk-go`)
- **Web Scraping & Agentic Research:** Yutori AI Research API (Bearer-token auth)
- **Database:** PostgreSQL via `lib/pq` (uses `JSONB` for schema-less `custom_channels`, and `TEXT[]` arrays for keywords/channels)
- **Routing:** Go 1.22+ method-pattern `ServeMux` (no router library); structured logging via `log/slog`

> `cmd/main.go` boots **two servers in one process**: the public REST API (`HTTP_ADDR`, default `:8000`) and the Restate SDK deployment server (`RESTATE_DEPLOYMENT_ADDR`, default `:9080`). Both share a single Postgres connection pool. The DB schema is **auto-applied on startup** via idempotent `CREATE TABLE IF NOT EXISTS` DDL embedded in `internal/db/db.go` — there are no migration files to run.

### Frontend (Interface & Dashboard)
- **Framework:** Vite + React 18 + TypeScript
- **Styling & UI:** Tailwind CSS + `lucide-react` icons (the only UI/icon packages used)
- **Routing:** `react-router-dom`
- **Data client:** `@supabase/supabase-js`

## Project Repository Structure

```text
validator/
├── AGENTS.md                       # Engineering master blueprint
├── README.md
├── validator-ui/                    # Frontend dashboard app (Vite + React + TS)
│   ├── src/
│   ├── index.html
│   ├── package.json
│   ├── tailwind.config.js
│   └── vite.config.ts          # Dev server proxies /api → http://localhost:8000
└── validator-backend/              # Core backend app (Go 1.25)
    ├── cmd/
    │   └── main.go                 # Unified entrypoint: REST API + Restate deployment server
    ├── internal/
    │   ├── api/                    # REST handlers, camelCase DTOs, LLM-markdown builder
    │   ├── config/                 # Env-only configuration
    │   ├── db/                     # Postgres pool, embedded schema, transactional scouts
    │   ├── models/                 # Domain types (snake_case for the DB layer)
    │   ├── workflow/               # Restate MarketValidationWorkflow + watchlist mutation
    │   └── yutori/                 # Yutori Research API client
    ├── .env.example                # Source of truth for env config
    └── go.mod
```

## Getting Started & Local Setup

### Prerequisites
- **Go** 1.25 or higher
- **Node.js** v18+ with npm
- **PostgreSQL** running locally or via Docker
- **Restate Server** (`restate-server` binary/CLI)

### 1. Configure Environment Variables
Copy `validator-backend/.env.example` → `validator-backend/.env` and fill it in. Only `DATABASE_URL` is hard-required at startup; everything else has defaults.

```bash
# --- Required ---
DATABASE_URL=postgres://validator:validator@localhost:5432/validator?sslmode=disable
# Yutori scouting API key (Bearer token).
YUTORI_API_KEY=your_private_yutori_api_token_here

# --- Restate runtime wiring ---
# Address the Restate runtime dials to reach this deployment's services.
RESTATE_DEPLOYMENT_ADDR=:9080
# Base URL of the Restate ingress the API server calls to start workflows.
RESTATE_INGRESS_URL=http://localhost:8080
# Optional bearer token if the Restate ingress requires authentication.
RESTATE_AUTH_KEY=

# --- Public REST API ---
HTTP_ADDR=:8000

# --- Yutori tuning ---
YUTORI_API_BASE=https://api.yutori.com
YUTORI_TIMEOUT_SECONDS=60
```

> **No schema bootstrap step.** Create the `validator` database in Postgres; the backend applies its own tables on boot.

### 2. Start the Backend
From `validator-backend/`:

```bash
go mod tidy
go run ./cmd
```

This starts the public REST API on `:8000` **and** the Restate deployment server on `:9080` in one process.

### 3. Connect the Restate Runtime
In a separate terminal, start the Restate server and register this deployment so it can discover the workflow/service handlers:

```bash
# Start the durable runtime
restate-server

# In another terminal, register the deployment
restate deployments register http://localhost:9080
```

### 4. Launch the Frontend UI
From `validator-ui/`:

```bash
npm install
npm run dev
```

The Vite dev server proxies `/api` → `http://localhost:8000` (override with `VITE_API_TARGET`).

## Core API Contracts

The JSON contract uses camelCase and is intentionally duplicated between `internal/api/dto.go` and `validator-ui/src/types/index.ts` — when changing one side, update the other.

| Method | Path | Status | Description |
|--------|------|--------|-------------|
| `GET`  | `/api/ideas` | 200 | List all tracked ideas with embedded scout cycles. |
| `POST`| `/api/ideas` | **201** | Persist an idea and fire-and-forget the Restate workflow. |
| `GET`  | `/api/ideas/{id}` | 200 | Fetch a single idea with all scout cycles. |
| `GET`  | `/api/ideas/{id}/payload` | 200 | Compiled LLM-markdown payload for the latest cycle. |
| `GET`  | `/healthz` | 200 | Liveness probe. |

### Create Tracking Loop — `POST /api/ideas`
Persists the idea, then asynchronously triggers the Restate workflow keyed by the idea id. Workflow trigger failures are logged but never fail the HTTP request.

**Request body:**
```json
{
  "title": "Automated B2B Cold Outreach Engine",
  "description": "A system that parses LinkedIn job descriptions to draft custom email pitches.",
  "scoutingFrequencyDays": 3,
  "keywords": ["Cold email SaaS", "LinkedIn automation tool", "B2B sales lead gen"],
  "channels": ["reddit", "youtube", "news"],
  "customChannels": []
}
```
- `scoutingFrequencyDays` defaults to **7** when omitted or `<= 0`.
- `channels` defaults to `["reddit", "youtube", "news"]` when omitted. Valid platforms: `reddit`, `youtube`, `social`, `news`, `custom`.

**Response (201):**
```json
{
  "idea": { "id": "…", "title": "…", "status": "pending", "cycles": [] },
  "workflow_id": "<idea id>",
  "invocation_id": "<restate invocation id>"
}
```

### Fetch Compiled LLM Payload — `GET /api/ideas/{id}/payload`
Returns a system-prompt-style Markdown document compiling the latest scout cycle's Pros vs. Cons from raw scraped signals, ready to paste into Claude, GPT, etc.

**Response (200):**
```json
{
  "payload": "# IDEA: Automated B2B Cold Outreach Engine\n\n## DESCRIPTION\n…\n## SEARCH RADIUS METADATA\n**Keywords:** …\n## CYCLE: Latest Scan (Day 0)\n### RAW PROS FOUND:\n…\n### RAW CONS FOUND:\n…\n"
}
```

## Key Design Features

- **Zero Resource Waste:** During the interval between tracking cycles, Restate natively serializes the loop state and unloads the Go workflow from memory via `restate.Sleep` (durable sleep) — no goroutine is held alive while waiting.
- **The Evolving Watchlist:** The workflow parses the text blobs returned by Yutori to surface new phrases, competitors, and concepts. It calls `restate.Set` to append them to the `watchlist` KV state, automatically widening the search radius for the next cycle without human intervention. Growth is capped per cycle and generic stopwords are filtered out to prevent runaway pollution.
- **Failsafe Execution:** Every side effect (DB reads/writes, scout dispatch) runs inside `restate.Run` / `RunAsync` / `RunVoid`. If the network drops or Yutori times out mid-cycle, Restate's journal replays the exact step on recovery — it never restarts the multi-day timer from zero. Terminal errors are wrapped with `terminalf` to preserve their semantics across suspension/resume.
- **One Workflow Per Idea:** `MarketValidationWorkflow.Run` is an infinite loop keyed by the idea id, so each idea is tracked by exactly one durable workflow instance across its entire lifetime.

## Development Commands

**Backend** (run from `validator-backend/`):
```bash
go run ./cmd        # start the service
go test ./...       # unit tests only (no DB/network needed)
go vet ./...        # the only static check; no separate lint step
```

**Frontend** (run from `validator-ui/`):
```bash
npm run dev         # dev server
npm run build       # production build
npm run preview     # preview the build
npm run lint        # eslint
npm run typecheck   # tsc --noEmit -p tsconfig.app.json (non-default config)
```

## Architecture Notes

- **JSON contract is duplicated by design.** `internal/api/dto.go` emits camelCase JSON that exactly matches `validator-ui/src/types/index.ts`. `internal/models` uses snake_case for the DB layer.
- **CORS is intentionally permissive** (`Access-Control-Allow-Origin: *`) because the UI is served from a different origin.
- **Config is env-only** — there is no config file loader. `validator-backend/.env.example` is the source of truth.
