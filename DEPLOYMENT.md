# Deployment Plan — Validator on GCP

> Deploy the full application: UI on Vercel, backend (API + worker) on GCP
> Cloud Run, with Neon (Postgres) + Restate Cloud + GitHub Actions CI/CD.

---

## Decisions

| Decision          | Choice                                              |
| ----------------- | --------------------------------------------------- |
| UI hosting        | **Vercel** (static SPA, global CDN, auto-deploy)    |
| Backend compute   | Cloud Run (API + worker — two Go binaries)          |
| Database          | Neon (managed PostgreSQL, pooled connection)        |
| Durable runtime   | Restate Cloud (managed Restate, free tier)          |
| CI/CD             | **GitHub Actions** (free tier, 2k min/mo private)   |
| Secrets           | GitHub Actions secrets + Cloud Run env vars         |
| GCP project       | Exists, no custom domain (use `*.run.app`)          |
| Scope             | Full pipeline: Dockerize + CI/CD + infra + deploy   |

### Why these choices simplify everything

- **Vercel for UI** → zero-config static hosting with global CDN, automatic
  builds on push, and built-in SPA routing via `vercel.json`. No nginx, no
  Docker image, no Cloud Run cold starts for the frontend.
- **Neon is wire-compatible Postgres** → the existing `lib/pq` driver and all
  SQL in `internal/db/db.go` work unchanged. Only `DATABASE_URL` changes
  (pooled hostname + `sslmode=require`).
- **Restate Cloud removes the self-hosted runtime** → no Compute Engine VM,
  no persistent disk, no `docker run restatedev/restate`. Restate Cloud hosts
  the runtime; the worker registers as a public endpoint.
- **GitHub Actions for CI/CD** → free 2,000 min/mo (private repos, unlimited
  for public). Uses Workload Identity Federation (no JSON keys). One push
  triggers both Vercel (UI) and Actions (backend).
- **Everything is managed** → GCP only runs two stateless Cloud Run services
  (API + worker). Vercel handles the UI.

---

## Code Changes Required

Two small changes are needed before deployment. Neither affects local dev.

### 1. Worker: add Restate Cloud identity verification

Restate Cloud signs every request to the worker with an environment-specific key.
The worker must verify this signature so only your Restate Cloud env can call it.

**File:** `validator-backend/cmd/worker/main.go` (lines 65-75)

```go
// Current:
restateSrv := server.NewRestate().
    Bind(restate.Reflect(&workflow.Day0SetupWorkflow{...})).
    Bind(restate.Reflect(&workflow.ScoutOps{...}))

// After:
restateSrv := server.NewRestate().
    Bind(restate.Reflect(&workflow.Day0SetupWorkflow{...})).
    Bind(restate.Reflect(&workflow.ScoutOps{...}))

if cfg.RestateIdentityKey != "" {
    restateSrv = restateSrv.WithIdentityV1(cfg.RestateIdentityKey)
}
```

The public key (`publickeyv1_...`) is safe to put in env/config — it is not
secret. When `RESTATE_IDENTITY_KEY` is empty (local dev), identity verification
is skipped, preserving the current local workflow.

### 2. Config: add `RestateIdentityKey` field

**File:** `validator-backend/internal/config/config.go`

```go
// Add to Config struct:
RestateIdentityKey string

// Add to Load():
RestateIdentityKey: os.Getenv("RESTATE_IDENTITY_KEY"),
```

### 3. DATABASE_URL: switch to Neon pooled string

The only runtime difference is the connection string format:

```
# Before (local):
postgres://validator:validator@localhost:5432/validator?sslmode=disable

# After (Neon pooled):
postgresql://user:pass@ep-xxx-pooler.<region>.aws.neon.tech/dbname?sslmode=require
```

- The `-pooler` suffix enables PgBouncer (transaction mode) — essential for
  Cloud Run's connection-per-instance model.
- `sslmode=require` (Neon enforces TLS).
- The auto-schema DDL (`CREATE TABLE IF NOT EXISTS`, `DO $$ ... $$`,
  `ALTER TABLE`) works through PgBouncer transaction mode — no session-level
  features are used.

---

## 1. Dockerization (backend only — UI goes to Vercel)

```
REPO ROOT
│
├── validator-backend/
│   ├── Dockerfile              ← multi-stage, builds BOTH binaries
│   ├── entrypoint.sh           ← selects "api" or "worker" via CMD
│   ├── .dockerignore
│   └── (existing code)
│
├── validator-ui/
│   ├── vercel.json             ← SPA rewrites + build config (for Vercel)
│   └── (existing code — Vercel builds from source, no Dockerfile needed)
│
└── (no root Dockerfile or docker-compose needed)
```

### Backend Dockerfile (single image, two binaries)

```
┌─────────────────────────────────────────────────────┐
│  STAGE 1: builder  (golang:1.25-alpine)             │
│                                                       │
│  COPY go.mod go.sum → go mod download                │
│  COPY . .                                            │
│  CGO_ENABLED=0 go build -o /out/api    ./cmd/api     │
│  CGO_ENABLED=0 go build -o /out/worker ./cmd/worker  │
└───────────────────────┬─────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│  STAGE 2: runtime  (alpine:3.20)                     │
│                                                       │
│  COPY --from=builder /out/api     /usr/local/bin/    │
│  COPY --from=builder /out/worker  /usr/local/bin/    │
│  COPY entrypoint.sh /entrypoint.sh                   │
│                                                       │
│  ENTRYPOINT ["/entrypoint.sh"]                       │
│  CMD ["api"]   ← overridable: "api" or "worker"     │
└─────────────────────────────────────────────────────┘
  • Uses alpine (not distroless) for /bin/sh in entrypoint
  • entrypoint.sh exec's /usr/local/bin/$1
  • Both binaries handle SIGTERM natively (graceful shutdown)
```

### UI deployment: Vercel (no Dockerfile)

Vercel builds and serves the SPA from source. No Docker image needed.

**`validator-ui/vercel.json`:**

```json
{
  "$schema": "https://openapi.vercel.sh/vercel/v1.json",
  "buildCommand": "npm run build",
  "outputDirectory": "dist",
  "rewrites": [
    { "source": "/(.*)", "destination": "/index.html" }
  ]
}
```

**How it works:**

- Vercel auto-detects the Vite project (`validator-ui/` as root directory).
- `rewrites` catches all non-file routes (`/new`, `/idea/:id`) and serves
  `index.html` — this is the SPA fallback for `BrowserRouter`.
- Static assets (`/assets/*`, `/vite.svg`) are served directly from CDN
  (filesystem match takes priority over rewrites).
- `VITE_API_BASE` is set as a **Vercel environment variable** to the API's
  Cloud Run URL at build time. Since CORS is `Access-Control-Allow-Origin: *`,
  the browser calls the backend directly — no proxy needed.

**Vercel project settings:**

| Setting             | Value                                    |
| ------------------- | ---------------------------------------- |
| Framework preset    | Vite                                     |
| Root directory      | `validator-ui`                           |
| Build command       | `npm run build` (from vercel.json)       |
| Output directory    | `dist` (from vercel.json)                |
| Install command     | `npm ci` (auto-detected)                 |
| Env var (build)     | `VITE_API_BASE=https://validator-api-...`|

---

## 2. GCP Architecture

```
       ┌───────────────────────────────────────────────────────────┐
       │                      I N T E R N E T                       │
       │                                                           │
       │    Users (browser)        Yutori (webhooks)               │
       └───────┬──────────────────────────┬────────────────────────┘
               │ HTTPS                    │ POST /api/webhooks/yutori
               │                          │
    ┌──────────▼──────────┐   ┌───────────▼───────────────────────┐
    │       VERCEL         │   │  CLOUD RUN:                        │
    │  validator-ui        │   │  validator-api                     │
    │                      │   │                                    │
    │  Static SPA on CDN   │   │  Go binary (cmd/api)               │
    │  • dist/ served      │   │  HTTP_ADDR=:8080                   │
    │  • SPA rewrites      │   │  PORT=8080                         │
    │    (vercel.json)     │   │  min-instances: 0                  │
    │  • VITE_API_BASE ────┼───┼─► max-instances: 1                │
    │    baked at build    │   │  concurrency: 80                   │
    │    (CORS = *)        │   │                                    │
    │                      │   │  Browser calls API directly:       │
    │  Auto-deploys on     │   │  VITE_API_BASE =                   │
    │  git push to main    │   │    https://validator-api-...       │
    │                      │   │    .a.run.app                      │
    │  Global edge CDN     │   │                                    │
    └──────────────────────┘   │  ┌──────────────────────────────┐  │
                               │  │ fire-and-forget (ingress)    │  │
                               │  │ RESTATE_INGRESS_URL ─────────┼──┼──► Restate Cloud
                               │  │ RESTATE_AUTH_KEY ────────────┤  │
                               │  └──────────────────────────────┘  │
                               │                                    │
                               │  ┌──────────────────────────────┐  │
                               │  │ reads/writes DB              │  │
                               │  │ DATABASE_URL (Neon) ─────────┼──┼──► Neon
                               │  └──────────────────────────────┘  │
                               └────────────────────────────────────┘

          ┌──────────────────────────────────────────────────────┐
          │                  RESTATE CLOUD                        │
          │              (managed Restate runtime)                │
          │                                                      │
          │  Environment: env_xxxxxxxx                           │
          │  Ingress URL:                                        │
          │    https://<env>.env.<region>.restate.cloud:8080     │
          │  Region: us | eu                                     │
          │                                                      │
          │  API key (Bearer): key_...                           │
          │  Public key (identity): publickeyv1_...              │
          │                                                      │
          │  Holds: Day0SetupWorkflow + ScoutOps invocations,    │
          │         durable state, journals, timers              │
          │                                                      │
          │  Dials INTO worker public endpoint ─────────┐        │
          └──────────────────────────────────────────────┼────────┘
                                                         │
                               ┌─────────────────────────▼──────────┐
                               │  CLOUD RUN:                         │
                               │  validator-worker                   │
                               │                                    │
                               │  Go binary (cmd/worker)             │
                               │  RESTATE_DEPLOYMENT_ADDR=:8080     │
                               │  PORT=8080                          │
                                │  .WithIdentityV1(publickeyv1_...)   │
                                │  min-instances: 0  ◀── SCALE TO 0  │
                                │  max-instances: 1  (on-demand)     │
                               │                                    │
                               │  ┌───────────────────────────────┐  │
                               │  │ reads/writes DB               │  │
                               │  │ DATABASE_URL (Neon) ──────────┼──┼──► Neon
                               │  └───────────────────────────────┘  │
                               │                                    │
                               │  ┌──────────┐  ┌────────────────┐  │
                               │  │ YUTORI   │  │  LLM API       │  │
                               │  │ SaaS ────┼──┤  (OpenAI/etc) ─┤  │
                               │  └──────────┘  └────────────────┘  │
                               └────────────────────────────────────┘

    ┌────────────────────────────────┐
    │          NEON                  │   ┌──────────────┐
    │  (managed PostgreSQL 16)       │   │  YUTORI SaaS │
    │                                │   │  (external)  │
    │  Pooled conn string:           │   │              │
    │  ep-xxx-pooler.region.         │   │  Webhooks ───┼──► validator-api
    │    aws.neon.tech               │   └──────────────┘
    │  sslmode=require               │
    │  PgBouncer transaction mode    │   ┌──────────────┐
    │                                │   │  LLM API     │
    │  api ────── (read/write) ──────┤   │  (external)  │
    │  worker ─── (read/write) ──────┤   └──────────────┘
    │                                │
    │  Schema auto-applied on boot   │
    │  (CREATE TABLE IF NOT EXISTS)  │
    └────────────────────────────────┘
```

---

## 3. Request & Data Flows

```
USER LOADS APP
  (1) Browser ──HTTPS──► Vercel (global CDN)
  (2) Vercel serves index.html + assets (static, SPA rewrites for deep links)

USER INTERACTS (browse/create ideas)
  (3) SPA ──GET/POST──► validator-api (Cloud Run, direct via VITE_API_BASE)
  (4) api ──read/write──► Neon (Postgres, pooled)
  (5) api ──fire-and-forget──► Restate Cloud (ingress URL + Bearer token)
       └─ triggers Day0SetupWorkflow / webhook forward / approval apply

RESTATE CLOUD DISPATCHES WORK
  (6) Restate Cloud ──HTTPS (signed)──► validator-worker (Cloud Run, :8080)
       └─ identity verified via WithIdentityV1(publickeyv1_...)

WORKER DOES HEAVY LIFTING
  (7) worker ──POST──► Yutori Research/Scouting API (create/patch scouts)
  (8) worker ──POST──► LLM API (mutation eval, prompt synthesis)
  (9) worker ──read/write──► Neon (signals, proposals, scout state)

YUTORI SENDS SIGNALS BACK
  (10) Yutori ──POST /api/webhooks/yutori──► validator-api (public *.run.app URL)
  (11) api ──forward──► Restate Cloud ──► ScoutOps.ProcessWebhook ──► worker
  (12) worker writes signals to Neon, opens proposals if needed
```

---

## 4. Neon Connection Details

Neon is a fully managed, serverless PostgreSQL. The app connects with a standard
`postgresql://` connection string — `lib/pq` works unchanged.

### Pooled vs Direct connection strings

| Use case                        | Connection type | Hostname suffix      |
| ------------------------------- | --------------- | -------------------- |
| App runtime (API + worker)      | **Pooled**      | `-pooler`            |
| Schema migration / `pg_dump`    | Direct          | _(no suffix)_        |

Since the schema auto-applies via `CREATE TABLE IF NOT EXISTS` (transaction-safe),
the **pooled** string works for everything including the startup DDL.

```
# Pooled (use this for Cloud Run):
postgresql://user:pass@ep-xxx-pooler.<region>.aws.neon.tech/dbname?sslmode=require

# Direct (only for manual DB ops / pg_dump):
postgresql://user:pass@ep-xxx.<region>.aws.neon.tech/dbname?sslmode=require
```

### PgBouncer transaction mode — what works, what doesn't

| Supported                          | NOT supported (session-level)        |
| ---------------------------------- | ------------------------------------ |
| `CREATE TABLE IF NOT EXISTS`       | `SET` / `RESET` (session variables)  |
| `DO $$ ... $$` (anonymous blocks)  | `LISTEN` / `NOTIFY`                  |
| `ALTER TABLE`                      | Temp tables with `PRESERVE ROWS`     |
| `RETURNING`, `$1` parameterized Q  | SQL-level `PREPARE`/`DEALLOCATE`     |
| Protocol-level prepared statements | Session-level advisory locks         |

The current `internal/db/db.go` uses none of the unsupported features.

### Pool sizing (free tier)

| Compute | RAM  | max_connections | pool size (90%) |
| ------- | ---- | --------------- | --------------- |
| 0.25 CU | 1 GB | 104             | ~93             |
| 0.50 CU | 2 GB | 209             | ~188            |

Free tier (0.25 CU) supports 10,000 client connections through the pooler —
more than enough for Cloud Run instances.

---

## 5. Restate Cloud Setup

### What Restate Cloud provides

- A managed Restate **environment** (cluster) with durable storage
- An **ingress URL** for the API to submit invocations
- **Admin API** for deployment registration
- Request **identity signing** so only your env can call the worker

### Setup steps

```
  ┌─────────────────────────────────────────────────────────────────┐
  │  1. SIGN UP at cloud.restate.dev (free tier, no credit card)     │
  │     → Create an account → Create an environment                 │
  │     → Choose region: us | eu                                    │
  │                                                                  │
  │  2. COLLECT CREDENTIALS from Developers tab:                     │
  │     • Environment ID:        env_xxxxxxxx                        │
  │     • Ingress URL:           https://<env>.env.<reg>.r.cloud:8080│
  │     • API key (Bearer):      key_...                             │
  │     • Signing public key:    publickeyv1_...                     │
  │                                                                  │
  │  3. CONNECT CLI:                                                 │
  │     restate cloud login                                          │
  │     restate cloud env configure                                  │
  │     restate config use-env <name>                                │
  │                                                                  │
  │  4. DEPLOY WORKER TO CLOUD RUN FIRST (needs public URL)          │
  │     → After deploy, note the worker's *.run.app URL              │
  │                                                                  │
  │  5. REGISTER THE WORKER DEPLOYMENT:                              │
  │     restate deployments register \                               │
  │       https://validator-worker-<hash>-<reg>.a.run.app            │
  │                                                                  │
  │  6. VERIFY:                                                      │
  │     restate deployments list  (should show worker + services)    │
  │     restate services list    (Day0SetupWorkflow, ScoutOps)       │
  └─────────────────────────────────────────────────────────────────┘
```

### Wire-up mapping

| Restate Cloud artifact             | Where it goes (env var)              |
| ---------------------------------- | ------------------------------------ |
| Ingress URL                        | `RESTATE_INGRESS_URL` (on api-svc)   |
| API key (`key_...`)                | `RESTATE_AUTH_KEY` (on api-svc)      |
| Signing public key (`publickeyv1_`) | `RESTATE_IDENTITY_KEY` (on worker)  |
| Worker Cloud Run URL               | Registered via `restate deployments` |

---

## 6. CI/CD Pipeline (GitHub Actions + Vercel)

The UI and backend deploy independently from the same `git push`. No Cloud Build
involved — GitHub Actions has 2,000 free min/mo for private repos (unlimited for
public repos).

```
  git push origin main
        │
        ├──► VERCEL (auto-triggered via GitHub integration)
        │    │
        │    ▼
        │    npm ci → npm run build → deploy to global CDN
        │    (VITE_API_BASE baked at build time)
        │    Live at: https://validator-ui.vercel.app
        │
        └──► GITHUB ACTIONS (.github/workflows/deploy.yml)
             │
             ▼
  ┌──────────────────────────────────────────────────────────────┐
  │  1. AUTH TO GCP (Workload Identity Federation — no JSON key)  │
  │     google-github-actions/auth@v2                            │
  │     → obtains short-lived OAuth token for the SA              │
  │                                                                │
  │  2. BUILD BACKEND (single image, both binaries)              │
  │     docker build -t $AR/validator/backend:$SHA ./validator-backend│
  │                                                                │
  │  3. PUSH to Artifact Registry                                 │
  │     docker push $AR/validator/backend:$SHA                    │
  │                                                                │
  │  4. DEPLOY API → Cloud Run                                     │
  │     gcloud run deploy validator-api \                         │
  │       --image $AR/validator/backend:$SHA                      │
  │       --min-instances 0 --max-instances 1                     │
  │       --args "api"  (entrypoint.sh selects binary)           │
  │       --set-secrets DATABASE_URL=...  (or env vars)          │
  │       --set-env-vars RESTATE_INGRESS_URL=...                  │
  │                                                                │
  │  5. DEPLOY WORKER → Cloud Run                                  │
  │     gcloud run deploy validator-worker \                      │
  │       --image $AR/validator/backend:$SHA                      │
  │       --min-instances 0 --max-instances 1   ← SCALE TO ZERO  │
  │       --args "worker"                                          │
  │       --set-env-vars RESTATE_IDENTITY_KEY=...                 │
  │                                                                │
  │  6. RE-REGISTER RESTATE (if handler shape changed)            │
  │     restate deployments register <worker-url> --force         │
  └──────────────────────────────────────────────────────────────┘
             │
             ▼
  Services live at:
    UI:     https://validator-ui.vercel.app       (Vercel)
    API:    https://validator-api-<hash>-<reg>.a.run.app
    Worker: https://validator-worker-<hash>-<reg>.a.run.app
```

### Why GitHub Actions instead of Cloud Build

|                     | GitHub Actions            | Cloud Build                 |
| ------------------- | ------------------------- | --------------------------- |
| Free tier           | 2,000 min/mo (private)    | 120 builds/day (~3,600/mo)  |
| Public repos        | **Unlimited**             | Charged after 120/day       |
| Setup               | Just a `.yml` file        | Enable API + trigger config |
| Secrets             | GitHub Secrets UI         | GCP Secret Manager / env    |
| GCP auth            | Workload Identity Fed     | Native (same project)       |
| GitHub-native       | Yes (repo = source)       | Needs GitHub connection     |

### Workload Identity Federation setup (one-time)

Replaces the old "download a JSON service account key" approach with short-lived
token exchange — nothing to rotate or leak.

```
  ┌──────────────────────────────────────────────────────────────┐
  │  GCP SIDE (one-time, via gcloud or console):                   │
  │                                                                │
  │  1. Create service account (e.g. github-ci@<proj>.iam.gserviceaccount.com)│
  │     Roles: roles/run.admin, roles/artifactregistry.writer     │
  │                                                                │
  │  2. Create Workload Identity Pool:                             │
  │     gcloud iam workload-identity-pools create github \         │
  │       --location=global                                        │
  │                                                                │
  │  3. Create provider (GitHub OIDC):                             │
  │     gcloud iam workload-identity-pools providers create github \│
  │       --location=global \                                      │
  │       --workload-identity-pool=github \                        │
  │       --attribute-condition="attribute.repository_owner/vishwajeet-sharma/validator" \│
  │       --issuer-uri=https://token.actions.githubusercontent.com │
  │                                                                │
  │  4. Allow the SA to be impersonated by the pool:               │
  │     (bind the SA's workloadIdentityUser role to the provider)  │
  │                                                                │
  │  GITHUB SIDE:                                                  │
  │  Add these as GitHub Actions secrets:                          │
  │  • GCP_PROJECT_ID        = your GCP project ID                 │
  │  • GCP_WIF_PROVIDER      = projects/<N>/locations/global/      │
  │                            workloadIdentityPools/github/       │
  │                            providers/github                    │
  │  • GCP_SERVICE_ACCOUNT   = github-ci@<proj>.iam.gserviceaccount.com│
  │  • DATABASE_URL          = postgresql://...-pooler... (Neon)   │
  │  • RESTATE_INGRESS_URL   = https://<env>.env.<reg>.restate.cloud:8080│
  │  • RESTATE_AUTH_KEY      = key_...                             │
  │  • RESTATE_IDENTITY_KEY  = publickeyv1_...                     │
  │  • YUTORI_API_KEY        = ...                                 │
  │  • LLM_API_KEY           = ...                                 │
  └──────────────────────────────────────────────────────────────┘
```

---

## 7. Environment Variables per Service

| Service              | Env Var                   | Value                                                     |
| -------------------- | ------------------------- | --------------------------------------------------------- |
| **validator-ui** (Vercel) | `VITE_API_BASE`      | `https://validator-api-<hash>-<reg>.a.run.app` (build-time) |
| **validator-api** (Cloud Run) | `DATABASE_URL`     | `postgresql://...-pooler...?sslmode=require` (Neon)       |
|                      | `HTTP_ADDR`               | `:8080`                                                   |
|                      | `RESTATE_INGRESS_URL`     | `https://<env>.env.<region>.restate.cloud:8080`           |
|                      | `RESTATE_AUTH_KEY`        | `key_...` (Restate Cloud API key)                         |
| **validator-worker** | `DATABASE_URL`            | `postgresql://...-pooler...?sslmode=require` (Neon)       |
|                      | `RESTATE_DEPLOYMENT_ADDR` | `:8080`                                                   |
|                      | `RESTATE_IDENTITY_KEY`    | `publickeyv1_...` (Restate Cloud public key)              |
|                      | `YUTORI_API_KEY`          | `<key>`                                                   |
|                      | `YUTORI_API_BASE`         | `https://api.yutori.com`                                  |
|                      | `LLM_API_KEY`             | `<key>`                                                   |
|                      | `LLM_MODEL`               | `gpt-4o-mini`                                             |
|                      | `WEBHOOK_PUBLIC_URL`      | `https://validator-api-<hash>-<reg>.a.run.app`            |

---

## 8. Implementation Roadmap

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 1 — CODE CHANGES (small, preserves local dev)                 │
 │  ☐ Add RestateIdentityKey to config.go (new env: RESTATE_IDENTITY_KEY)│
 │  ☐ Add conditional .WithIdentityV1() in cmd/worker/main.go           │
 │  ☐ Verify: go vet ./... && go test ./...                             │
 │  ☐ Verify: local dev still works (empty identity key = no check)    │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 2 — DOCKERIZE (backend only) + VERCEL CONFIG                  │
 │  ☐ Write validator-backend/Dockerfile + entrypoint.sh (+ .dockerignore)│
 │  ☐ Write validator-ui/vercel.json (SPA rewrites)                     │
 │  ☐ Test: docker build ./validator-backend (verify image builds)      │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 3 — EXTERNAL SERVICES SETUP                                   │
 │  ☐ Neon: create project → copy pooled connection string              │
 │  ☐ Restate Cloud: sign up → create env → collect credentials:        │
 │       env ID, ingress URL, API key (key_...), public key (pubkey1_..)│
 │  ☐ Restate CLI: restate cloud login + env configure                  │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 4 — GCP INFRA                                                 │
 │  ☐ Enable APIs: Cloud Run, Artifact Registry                         │
 │  ☐ Create Artifact Registry repo (validator)                         │
 │  ☐ Create service account + Workload Identity Federation for GitHub  │
 │  ☐ Build + push backend image to Artifact Registry                   │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 5 — DEPLOY SERVICES                                           │
 │  ☐ Deploy validator-worker → Cloud Run (min=0, max=3, env vars)      │
 │  ☐ Deploy validator-api   → Cloud Run (min=0, max=10, env vars)      │
 │  ☐ Connect validator-ui repo to Vercel:                              │
 │       • Set root directory = validator-ui                            │
 │       • Set VITE_API_BASE env var = api's *.run.app URL             │
 │       • Deploy                                                        │
 │  ☐ Register worker URL with Restate Cloud:                           │
 │       restate deployments register <worker-url>                      │
 │  ☐ Set WEBHOOK_PUBLIC_URL on api = api's *.run.app URL              │
 │  ☐ Smoke test: create idea → Day0 workflow → scout signals flow     │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 6 — CI/CD AUTOMATION (GitHub Actions)                         │
 │  ☐ Write .github/workflows/deploy.yml (backend: api + worker)        │
 │  ☐ Add GitHub Actions secrets (GCP creds + app secrets)             │
 │  ☐ Connect GitHub repo to Vercel (auto-deploys UI)                   │
 │  ☐ Test: push → Vercel deploys UI + Actions deploys backend          │
 └─────────────────────────────────────────────────────────────────────┘
```

---

## Summary: What changed from the previous plan

| Before                          | After                              | Why                           |
| ------------------------------- | ---------------------------------- | ----------------------------- |
| TursoDB (libSQL migration risk) | Neon (Postgres wire-compat)        | Zero code changes to DB layer |
| Restate on Compute Engine VM    | Restate Cloud (managed)            | No VM, no disk, no ops        |
| UI on Cloud Run (nginx)         | UI on Vercel (CDN)                 | Zero-config SPA, no cold start|
| Worker always-on (min=1)        | Worker scale-to-zero (min=0)       | On-demand dispatch, save cost |
| Cloud Build CI/CD               | GitHub Actions CI/CD               | Free tier, GitHub-native      |
| Phase 0 blocker (DB rewrite)    | Phase 1 (tiny code change)         | No more 654-line rewrite      |
| Compute Engine in GCP           | Not needed                         | API + worker only on Cloud Run|

**External managed services:** Vercel (UI), Neon (DB), Restate Cloud (runtime), Yutori (SaaS), LLM API.
**GCP resources:** 2 Cloud Run services + 1 Artifact Registry repo.
**CI/CD:** GitHub Actions (backend) + Vercel (UI), both triggered from `git push main`.
**Vercel resources:** 1 project (auto-deploys from GitHub).
