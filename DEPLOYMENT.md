# Deployment Plan — Validator on GCP

> Dockerize the full application (backend API + worker + UI) and deploy to GCP
> using Cloud Run + Compute Engine + TursoDB, with Cloud Build CI/CD.

---

## Decisions

| Decision          | Choice                                  |
| ----------------- | --------------------------------------- |
| Compute platform  | Cloud Run (stateless) + Compute Engine (Restate) |
| Database          | TursoDB (external managed)              |
| Secrets           | Environment variables                   |
| GCP project       | Exists, no custom domain (use `*.run.app`) |
| Scope             | Full pipeline: Dockerize + CI/CD + infra + deploy |

---

## Critical Considerations

### 1. TursoDB requires a DB-layer migration

The current code is hardcoded to PostgreSQL:

- `github.com/lib/pq` driver (`internal/db/db.go:110` → `sql.Open("postgres", ...)`)
- Postgres-specific SQL: `DO $$ ... $$` blocks, `RETURNING`, `pq.Array()`, `$1` placeholders, `sslmode=disable`

Turso has two modes:

- **libSQL** (SQLite-based) → needs `github.com/tursodatabase/libsql-client-go` + SQL syntax rewrites
- **Turso for Postgres** (wire-compatible) → `lib/pq` _might_ work as-is

> **Decision needed:** Which Turso mode? If libSQL, `internal/db/db.go` (654 lines) must be rewritten before deployment is useful.

### 2. Restate runtime is stateful

Cloud Run containers are ephemeral (lose disk on cold start). Restate needs persistent storage for its durable log/bifrost. Options:

- **(A) Recommended:** Restate on Compute Engine VM with persistent SSD, rest on Cloud Run
- **(B)** All on Cloud Run + mount Cloud Storage/NFS volume for Restate (complex, performance caveats)

This plan uses **option A**.

---

## 1. Dockerization (3 images, all from scratch)

```
REPO ROOT
│
├── validator-backend/
│   ├── Dockerfile              ← multi-stage, builds BOTH binaries
│   ├── .dockerignore
│   └── (existing code)
│
├── validator-ui/
│   ├── Dockerfile              ← multi-stage: node build → nginx serve
│   ├── nginx.conf              ← SPA fallback + /api proxy
│   ├── .dockerignore
│   └── (existing code)
│
└── (no root Dockerfile needed — each package builds independently)
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
│  STAGE 2: runtime  (gcr.io/distroless/static)        │
│                                                       │
│  COPY --from=builder /out/api     /usr/local/bin/    │
│  COPY --from=builder /out/worker  /usr/local/bin/    │
│                                                       │
│  CMD via build-arg or env: "api" | "worker"          │
│  (entrypoint script selects binary)                  │
└─────────────────────────────────────────────────────┘
  • No CGO, no OS deps → distroless/static (~15MB)
  • Graceful shutdown: both handle SIGTERM natively
```

### UI Dockerfile (build → serve)

```
┌─────────────────────────────────────────────────────┐
│  STAGE 1: builder  (node:20-alpine)                  │
│                                                       │
│  COPY package*.json → npm ci                         │
│  COPY . .                                            │
│  ARG VITE_API_BASE=""    (empty = relative /api)     │
│  RUN npm run build  → dist/                          │
└───────────────────────┬─────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│  STAGE 2: serve  (nginx:alpine)                      │
│                                                       │
│  COPY --from=builder /app/dist /usr/share/nginx/html │
│  COPY nginx.conf /etc/nginx/conf.d/default.conf      │
│  EXPOSE 8080                                         │
└─────────────────────────────────────────────────────┘

nginx.conf:
  ┌─────────────────────────────────────────────┐
  │  location / {                                │
  │    root /usr/share/nginx/html;               │
  │    try_files $uri /index.html;  ← SPA fallback│
  │  }                                           │
  │  # /api proxied to API Cloud Run URL         │
  │  # (set via envsubst at container start)     │
  └─────────────────────────────────────────────┘
```

---

## 2. GCP Architecture

```
          ┌──────────────────────────────────────────────────────────────┐
          │                         I N T E R N E T                        │
          │                                                                │
          │    Users                  Yutori (webhooks)                   │
          └────────┬─────────────────────────┬────────────────────────────┘
                   │                         │ POST /api/webhooks/yutori
                   │ HTTPS                   │
          ┌────────▼─────────────────────────▼───────────────────────────┐
          │              CLOUD LOAD BALANCER (global HTTPS)               │
          │           Google-managed TLS cert  (no domain yet →           │
          │           use *.run.app direct URLs as interim)               │
          │                                                                │
          │   Serverless NEG → routes to Cloud Run services                │
          └──────┬──────────────────────────────────┬────────────────────┘
                 │                                   │
    ┌────────────▼──────────────┐     ┌──────────────▼──────────────────┐
    │   CLOUD RUN: validator-ui │     │   CLOUD RUN: validator-api       │
    │                           │     │                                  │
    │   nginx serves SPA        │     │   Go binary (cmd/api)            │
    │   • static dist/          │     │   HTTP_ADDR=:8080                │
    │   • SPA fallback          │     │   min-instances: 0               │
    │   • /api → proxy to api   │     │   max-instances: 10              │
    │                           │     │   concurrency: 80                │
    │   min-instances: 0        │     │                                  │
    │   (cold start OK for UI)  │     │   ENV:                           │
    │                           │     │   • DATABASE_URL (Turso)         │
    └───────────────────────────┘     │   • RESTATE_INGRESS_URL ─────────┼──┐
                                      │   • RESTATE_AUTH_KEY             │  │
                                      │   • HTTP_ADDR=:8080              │  │
                                      └──────────┬───────────────────────┘  │
                                                 │                            │
                          reads/writes DB        │ fire-and-forget            │
                      ┌──────────────────────────┘ (ingress client)           │
                      │                            │                          │
                      │                  ┌──────────▼──────────────────────▼──┐
                      │                  │       COMPUTE ENGINE: restate-vm    │
                      │                  │                                     │
                      │                  │  docker.io/restatedev/restate       │
                      │                  │  • ingress :8080  (internal)        │
                      │                  │  • meta/admin :9070  (internal)     │
                      │                  │  • Persistent SSD (bifrost log)     │
                      │                  │  • Internal IP only (no public)     │
                      │                  │  • Always-on (no scaling)           │
                      │                  │                                     │
                      │                  │  One-time: register deployment ─────┼──┐
                      │                  └─────────────────────────────────────┘  │
                      │                                       ▲                  │
                      │                                       │ dials INTO       │
                      │                          ┌────────────┴──────────────────▼─┐
                      │                          │   CLOUD RUN: validator-worker     │
                      │                          │                                   │
                      │                          │   Go binary (cmd/worker)          │
                      │                          │   RESTATE_DEPLOYMENT_ADDR=:8080  │
                      │                          │   min-instances: 1  ◀── ALWAYS ON │
                      │                          │   max-instances: 1  (single)      │
                      │                          │                                   │
                      │                          │   ENV:                            │
                      │                          │   • DATABASE_URL (Turso) ─────────┼──┐
                      │                          │   • YUTORI_API_KEY ───────────────┼──┤
                      │                          │   • YUTORI_API_BASE               │  │
                      │                          │   • LLM_API_KEY ──────────────────┼──┤
                      │                          │   • LLM_MODEL                     │  │
                      │                          │   • WEBHOOK_PUBLIC_URL            │  │
                      │                          └──────┬───────────┬───────────────┘  │ │
                      │                                 │           │                  │ │
                      │                    ┌────────────▼──┐  ┌─────▼──────────┐       │ │
              ┌───────▼──────────┐         │  YUTORI SaaS  │  │   LLM API      │       │ │
              │     TURSO DB     │         │  (api.yutori  │  │ (OpenAI/etc.)  │       │ │
              │   (external,     │         │   .com)       │  │                │       │ │
              │    managed)      │         └───────────────┘  └────────────────┘       │ │
              │                  │              ▲                                     │ │
              │  api-svc ────────┼──────────────┼── (webhooks come back to api)       │ │
              │  worker-svc ─────┼──────────────┘                                      │ │
              └──────────────────┘                  (both read/write)                  │ │
                                                                                    │ │
    ┌────────────────────────────────────────────────────────────────────────────────┘ │
    │  ARTIFACT REGISTRY (<region>-docker.pkg.dev)                                    │
    │                                                                                  │
    │  • validator/ui:latest    ← pushed by Cloud Build                                │
    │  • validator/api:latest   ← pushed by Cloud Build                                │
    │  • validator/worker:latest ← pushed by Cloud Build                               │
    │     (single backend image; CMD "api" or "worker" selects binary)                 │
    └──────────────────────────────────────────────────────────────────────────────────┘

    ┌──────────────────────────────────────────────────────────────────────────────────┐
    │  CLOUD BUILD (CI/CD) — triggered on git push to main                             │
    │                                                                                  │
    │  push → build images → push to Artifact Registry → deploy to Cloud Run services  │
    │         → (re)register Restate deployment if worker changed                      │
    └──────────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Request & Data Flows

```
USER LOADS APP
  (1) Browser ──HTTPS──► Load Balancer ──► validator-ui (Cloud Run)
  (2) ui serves index.html + assets (static)

USER INTERACTS (browse/create ideas)
  (3) SPA ──GET/POST /api/*──► validator-api (Cloud Run)
  (4) api ──read/write──► TursoDB
  (5) api ──fire-and-forget──► restate-vm (ingress :8080)
       └─ triggers Day0SetupWorkflow / webhook forward / approval apply

RESTATE DISPATCHES WORK
  (6) restate-vm ──HTTP──► validator-worker (Cloud Run, :8080)
       └─ runtime dials worker's deployment endpoint

WORKER DOES HEAVY LIFTING
  (7) worker ──POST──► Yutori Research/Scouting API (create scouts)
  (8) worker ──POST──► LLM API (mutation eval, prompt synthesis)
  (9) worker ──read/write──► TursoDB (signals, proposals, scout state)

YUTORI SENDS SIGNALS BACK
  (10) Yutori ──POST /api/webhooks/yutori──► validator-api (public URL)
  (11) api ──forward──► restate-vm ──► ScoutOps.ProcessWebhook ──► worker
  (12) worker writes signals to TursoDB, opens proposals if needed
```

---

## 4. CI/CD Pipeline (Cloud Build)

```
  git push origin main
        │
        ▼
┌────────────────────────────────────────────────────────────┐
│  CLOUD BUILD TRIGGER (cloudbuild.yaml in repo root)         │
│                                                              │
│  STEP 1: Build UI                                           │
│    docker build -t validator-ui ./validator-ui              │
│                --build-arg VITE_API_BASE=""                 │
│    (empty = relative /api, proxied by nginx)                │
│                                                              │
│  STEP 2: Build Backend (single image)                       │
│    docker build -t validator-backend ./validator-backend    │
│                                                              │
│  STEP 3: Tag & Push                                          │
│    → <region>-docker.pkg.dev/<proj>/validator/ui:$SHORT_SHA │
│    → <region>-docker.pkg.dev/<proj>/validator/api:$SHORT_SHA│
│    → <region>-docker.pkg.dev/<proj>/validator/worker:...    │
│    → + :latest tags                                          │
│                                                              │
│  STEP 4: Deploy to Cloud Run                                 │
│    gcloud run deploy validator-ui   --image .../ui          │
│    gcloud run deploy validator-api  --image .../api         │
│         --set-env-vars DATABASE_URL=...                     │
│    gcloud run deploy validator-worker --image .../worker    │
│         --min-instances=1 (always-on)                       │
│                                                              │
│  STEP 5: Re-register Restate (only if worker shape changed) │
│    restate deployments register <worker-url> --force        │
└────────────────────────────────────────────────────────────┘
        │
        ▼
  Services live at:
    https://validator-ui-<hash>-<reg>.a.run.app
    https://validator-api-<hash>-<reg>.a.run.app
    https://validator-worker-<hash>-<reg>.a.run.app
```

---

## 5. Environment Variables per Service

| Service             | Env Var                  | Value                                        |
| ------------------- | ------------------------ | -------------------------------------------- |
| **validator-ui**    | _(none)_                 | API URL baked at build or nginx-proxied      |
| **validator-api**   | `DATABASE_URL`           | `<turso connection string>`                  |
|                     | `HTTP_ADDR`              | `:8080`                                      |
|                     | `RESTATE_INGRESS_URL`    | `http://<restate-vm-internal>:8080`          |
|                     | `RESTATE_AUTH_KEY`       | `<optional>`                                 |
| **validator-worker**| `DATABASE_URL`           | `<turso connection string>`                  |
|                     | `RESTATE_DEPLOYMENT_ADDR`| `:8080`                                      |
|                     | `YUTORI_API_KEY`         | `<key>`                                      |
|                     | `YUTORI_API_BASE`        | `https://api.yutori.com`                     |
|                     | `LLM_API_KEY`            | `<key>`                                      |
|                     | `LLM_MODEL`              | `gpt-4o-mini`                                |
|                     | `WEBHOOK_PUBLIC_URL`     | `https://<api-cloud-run-url>`                |
| **restate-vm**      | `RESTATE_LOG_FORMAT`     | `compact`                                    |

---

## 6. Implementation Roadmap

```
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 0 — DB MIGRATION (BLOCKER)                                    │
 │  ☐ Decide Turso mode (libSQL vs Postgres-compat)                    │
 │  ☐ If libSQL: rewrite internal/db/db.go (pq → libsql driver, SQL)   │
 │  ☐ Verify schema DDL works on Turso                                  │
 │  ☐ Update DATABASE_URL format for Turso                             │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 1 — DOCKERIZE (local test first)                              │
 │  ☐ Write validator-backend/Dockerfile (+ .dockerignore)             │
 │  ☐ Write validator-ui/Dockerfile + nginx.conf (+ .dockerignore)     │
 │  ☐ Write docker-compose.yml (full stack for local testing)          │
 │  ☐ Test: docker compose up, verify all services connect             │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 2 — GCP INFRA SETUP                                           │
 │  ☐ Enable APIs: Run, Build, Artifact Registry, Compute              │
 │  ☐ Create Artifact Registry repo (validator)                        │
 │  ☐ Provision restate-vm (Compute Engine + persistent SSD)           │
 │  ☐ Configure VPC firewall: api/worker ↔ restate-vm (internal)       │
 │  ☐ Set up TursoDB instance, get connection string                   │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 3 — DEPLOY SERVICES                                           │
 │  ☐ Push images to Artifact Registry                                  │
 │  ☐ Deploy validator-api → Cloud Run (with env vars)                 │
 │  ☐ Deploy validator-worker → Cloud Run (min-instances=1)            │
 │  ☐ Deploy validator-ui → Cloud Run                                   │
 │  ☐ Register worker URL with Restate on restate-vm                   │
 │  ☐ Set WEBHOOK_PUBLIC_URL = api Cloud Run URL                        │
 │  ☐ Smoke test end-to-end                                            │
 └─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
 ┌─────────────────────────────────────────────────────────────────────┐
 │  PHASE 4 — CI/CD AUTOMATION                                          │
 │  ☐ Write cloudbuild.yaml                                             │
 │  ☐ Create Cloud Build trigger (push to main)                        │
 │  ☐ Connect GitHub repo to Cloud Build                               │
 │  ☐ Test: push → auto build → auto deploy                            │
 └─────────────────────────────────────────────────────────────────────┘
```

---

## Open Questions

1. **Turso mode** — libSQL (needs `internal/db/db.go` rewrite) or Postgres-compatible (may work as-is)?
2. **Restate persistence** — confirm Compute Engine VM approach, or attempt all-Cloud-Run with mounted volume?
