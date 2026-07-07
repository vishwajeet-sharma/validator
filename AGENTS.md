# AGENTS.md

Two independent packages, no root workspace file:
- `validator-backend/` — Go 1.25 service (module path `validator-backend`).
- `validator-ui/` — Vite + React + TS frontend.

## Commands

Backend (run from `validator-backend/`):
- `go run ./cmd` — start the service.
- `go test ./...` — unit tests only; **no DB/network needed** (mutation, dto, markdown).
- `go vet ./...` — there is no separate lint step; `vet` is the check.

UI (run from `validator-ui/`):
- `npm run dev` — dev server; proxies `/api` → `http://localhost:8000` (override with `VITE_API_TARGET`).
- `npm run build` / `npm run preview`.
- `npm run lint` (eslint), `npm run typecheck`.
- `npm run typecheck` is **non-default**: `tsc --noEmit -p tsconfig.app.json`. Do not assume `tsc` alone.

## Running the backend

`cmd/main.go` boots **two servers in one process**: the public REST API (`HTTP_ADDR`, default `:8000`) and the Restate SDK deployment server (`RESTATE_DEPLOYMENT_ADDR`, default `:9080`). Both share one Postgres pool.

External prerequisites (must already be running): PostgreSQL and a Restate runtime (`RESTATE_INGRESS_URL`). Only `DATABASE_URL` is hard-required at startup; everything else has defaults — see `validator-backend/.env.example` (the source of truth for env config).

DB schema is **auto-applied on startup** via idempotent `CREATE TABLE IF NOT EXISTS` DDL embedded in `internal/db/db.go`. There are no migration files.

## Architecture notes

- **JSON contract is duplicated by design.** `internal/api/dto.go` emits camelCase JSON that exactly matches `validator-ui/src/types/index.ts`. `internal/models` uses snake_case for the DB layer. When changing one side, update the other.
- `POST /api/ideas` persists the idea, then **fire-and-forgets** the Restate workflow via ingress. Workflow trigger failures are logged but never fail the HTTP request.
- `MarketValidationWorkflow.Run` is an **infinite loop**, keyed by idea id (one workflow per idea). It uses Restate durable sleep + KV state (`watchlist`, `cycle`).
- In the workflow, side effects must stay inside `restate.Run`/`RunAsync`/`RunVoid`. Use `terminalf` (not `fmt.Errorf`) to preserve terminal-error semantics — see its comment in `internal/workflow/workflow.go`.

## Conventions

- Go uses method-pattern `ServeMux` routing (e.g. `mux.HandleFunc("GET /api/ideas", ...)`) and `log/slog`. No router library.
- CORS is intentionally permissive (`Access-Control-Allow-Origin: *`); the UI is served from a different origin.
- UI styling is Tailwind + `lucide-react` icons only — do not add other UI/icon packages (see `validator-ui/.bolt/prompt`).
- Config is env-only; there is no config file loader.
