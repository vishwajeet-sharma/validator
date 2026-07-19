# AGENTS.md

Two independent packages, no root workspace file:
- `validator-backend/` — Go 1.25 service (module path `validator-backend`).
- `validator-ui/` — Vite + React + TS frontend.

## Architecture (split-surface)

The backend is split into **two binaries** that share one Postgres pool:

- `cmd/api/` — **Public API Surface** (HTTP, `HTTP_ADDR` default `:8080`; `.env` uses `:8000` locally because the Restate runtime occupies `:8080`). Internet-facing only: idea onboarding, proposal responses, Yutori webhook ingress. Writes to Postgres and **fire-and-forgets** heavy work to the worker via the Restate ingress client.
- `cmd/worker/` — **Internal Worker Surface** (Restate SDK server, `RESTATE_DEPLOYMENT_ADDR` default `:9080`). Hosts all durable components: the Day 0 setup workflow and the ScoutOps service. Talks to Yutori + Postgres; never exposed to the internet.

Packages:
- `internal/db/` — Postgres pool, schema, transaction-backed queries (ideas, scouts, prompt_proposals, market_signals).
- `internal/scouts/` — stateless Yutori HTTP client (Research API + Scouting API: create/patch/email). Auth = `X-API-Key` header.
- `internal/workflow/` — Restate components: `Day0SetupWorkflow` (workflow) and `ScoutOps` (service).
- `internal/api/` — public REST handlers + DTOs (consumed by `cmd/api`).
- `internal/models/` — domain types; `internal/config/` — env config (loaded via `godotenv`).

## Commands

Backend (run from `validator-backend/`):
- `go run ./cmd/api` — start the public API surface.
- `go run ./cmd/worker` — start the internal worker (Restate SDK server).
- Both must run for the app to work. Run each in its own terminal.
- `go test ./...` — unit tests only; **no DB/network needed**.
- `go vet ./...` — there is no separate lint step; `vet` is the check.

UI (run from `validator-ui/`):
- `npm run dev` — dev server; proxies `/api` → `http://localhost:8000` (override with `VITE_API_TARGET`).
- `npm run build` / `npm run preview`.
- `npm run lint` (eslint), `npm run typecheck`.
- `npm run typecheck` is **non-default**: `tsc --noEmit -p tsconfig.app.json`. Do not assume `tsc` alone.

## Running the backend

External prerequisites (must already be running): PostgreSQL and a Restate runtime (`RESTATE_INGRESS_URL`, default `http://localhost:8080`). Only `DATABASE_URL` is hard-required at startup; everything else has defaults — see `validator-backend/.env.example` (the source of truth for env config).

**Restate deployment registration (one-time, and after any component-shape change):** the worker must be registered with the Restate runtime before invocations can be dispatched. Restate runs in Docker, so it must reach back to the host worker via the Docker bridge gateway:
```
restate deployments register http://172.17.0.1:9080 --yes
```
If the worker's components change shape (renamed/added handlers), re-register with `--force`. Verify with `curl -s http://localhost:9070/services`.

**Webhook delivery:** Yutori scout updates flow back via `POST /api/webhooks/yutori` on the API surface, which forwards the raw payload to the worker's `ScoutOps.ProcessWebhook`. This requires a publicly-reachable URL set via `WEBHOOK_PUBLIC_URL` (use a tunnel like ngrok for local dev). With it empty, scouts still run on Yutori but signals won't auto-flow into the DB.

DB schema is **auto-applied on startup** via idempotent `CREATE TABLE IF NOT EXISTS` DDL embedded in `internal/db/db.go`. A guarded DO block drops the legacy keyword/channel-based schema exactly once. There are no migration files.

## Architecture notes

- **Day 0 flow** (`Day0SetupWorkflow.Run`, keyed by idea id): broad Research API call → LLM block (`ResearchPromptPair`) synthesises distinct PRO/CON tracking prompts → deploys two Yutori **Scouting** scouts (parallel `RunAsync`) with `output_schema`=SignalSchema + webhook → persists scout rows → activates the idea. One workflow per idea, completes once.
- **Webhook / mutation flow** (`ScoutOps.ProcessWebhook`): parse inbound payload → resolve scout by `yutori_scout_id` → record signals (tx) → isolated LLM eval (`ReviewMutation`) → if expansion recommended, open a `prompt_proposals` row and flip **only that scout** to `PENDING_MUTATION`. The other scout is untouched.
- **Human approval** (`POST /api/proposals/{id}/respond`): APPROVE updates `scouts.current_prompt` + resolves proposal (DB, sync) then fire-and-forgets `ScoutOps.ApplyApproval` to PATCH Yutori; REJECT just resolves the proposal. Either way the scout returns to ACTIVE.
- **JSON contract is duplicated by design.** `internal/api/dto.go` emits camelCase JSON that exactly matches `validator-ui/src/types/index.ts`. `internal/models` is the DB-layer shape. When changing one side, update the other.
- In Restate components, side effects must stay inside `restate.Run`/`RunAsync`/`RunVoid`. Use `terminalf` (not `fmt.Errorf`) to preserve terminal-error semantics — see its comment in `internal/workflow/workflow.go`.
- Service handlers take `restate.Context`; the workflow `Run` takes `restate.WorkflowContext`. Both are registered via `restate.Reflect(&struct{})` with pointer-receiver methods.

## Conventions

- Go uses method-pattern `ServeMux` routing (e.g. `mux.HandleFunc("GET /api/ideas", ...)`) and `log/slog`. No router library.
- CORS is intentionally permissive (`Access-Control-Allow-Origin: *`); the UI is served from a different origin.
- UI state is **Zustand** (`src/store/useIdeaStore.ts`) for the dashboard list/onboarding; the detail board fetches its own idea with polling. Styling is Tailwind + `lucide-react` icons only — do not add other UI/icon packages (see `validator-ui/.bolt/prompt`).
- Config is env-only (`.env` loaded via `godotenv`); there is no config file loader.
