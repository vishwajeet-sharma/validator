# Validator

Validator is an evidence-first market-research platform that validates a business
idea by **deploying two long-running background scouts** — one tracking demand
(**PRO**), one tracking threats (**CON**) — and harvesting real, sourced findings
from the web. Instead of handing you an arbitrary AI "success score," Validator
continuously collects verifiable signals (each with a quote and a source URL) and
lets an LLM propose widening a scout's search radius when new angles emerge.
Those expansions are **human-in-the-loop**: a proposal is opened, you review the
proposed prompt, edit it if you like, and approve or reject — the live scout only
mutates on your say-so. The final, unfiltered dataset is yours to feed into any
LLM.

> **How it works internally** — system design, data flows, data model, sequence
> diagrams, failure modes — lives in [`architecture.md`](./architecture.md).
> Engineering conventions for working in the codebase live in [`AGENTS.md`](./AGENTS.md).

---

## Quickstart

```bash
# 0) Prereqs already running: PostgreSQL + a Restate runtime (Docker is fine).
#    Create the DB once:  createdb validator   (schema is auto-applied on startup)

# 1) Backend env — only DATABASE_URL is hard-required to BOOT, but for the full
#    cycle you also need YUTORI_API_KEY, WEBHOOK_PUBLIC_URL, and LLM_API_KEY:
cp validator-backend/.env.example validator-backend/.env
#   set at minimum: DATABASE_URL, YUTORI_API_KEY, WEBHOOK_PUBLIC_URL, LLM_API_KEY

# 2) Start BOTH binaries (each in its own terminal, from validator-backend/):
go run ./cmd/api         # public REST API  (:8000 per .env)
go run ./cmd/worker      # Restate SDK server (:9080)

# 3) Register the worker with the Restate runtime (one-time, and after any
#    handler-shape change):
#    (Restate runs in Docker, so it reaches the host worker via the bridge GW.)
restate deployments register http://172.17.0.1:9080 --yes
curl -s http://localhost:9070/services   # verify Day0SetupWorkflow + ScoutOps appear

# 4) Expose the API publicly so Yutori can deliver scout webhooks back to you.
#    WEBHOOK_PUBLIC_URL must point here. cloudflared is recommended (stable,
#    free, no session limit); ngrok/pinggy also work but free tiers drop often:
cloudflared tunnel --url http://localhost:8000     # copy the https://*.trycloudflare.com URL
#    put that URL into WEBHOOK_PUBLIC_URL and restart the worker so it picks it up.

# 5) Frontend (from validator-ui/):
npm install && npm run dev   # Vite proxies /api -> http://localhost:8000
```

### Create your first idea

1. Open the UI (Vite prints the URL, usually `http://localhost:5173`).
2. Click **New Idea**, describe the product idea (≥ 12 chars), pick a scouting
   frequency, and **Deploy Scouts**.
3. The idea is created with `status = INITIAL_SWEEP`. Behind the scenes the
   `Day0SetupWorkflow` runs: Groq expands your idea into a research brief → a
   single Yutori **Research** task harvests signals and writes the PRO/CON
   prompts → two Yutori **Scouts** are deployed (PRO + CON) → the harvested
   signals populate the board → `status = ACTIVE`.
4. From then on each scout runs on its interval and calls back via the webhook;
   new findings stream into the board, and when an LLM thinks a scout should
   widen its net you get a **proposal** to approve/reject.

> **To stop a scout** (halts its recurring credit usage), open the idea and click
> **Stop scout** in the Pro or Con column — this deletes the scout on Yutori and
> marks it `STOPPED` locally; existing findings are kept.

> The database schema is **auto-applied on startup** (idempotent
> `CREATE TABLE IF NOT EXISTS` + a one-time constraint migration). There are no
> migration files to run.

---

## REST API

The public surface is a small set of REST endpoints on `cmd/api`. The JSON
contract is intentionally duplicated: `internal/api/dto.go` emits camelCase that
exactly matches `validator-ui/src/types/index.ts`.

| Method | Path | Status | Description |
|--------|------|--------|-------------|
| `GET`  | `/api/ideas` | 200 | List all tracked ideas with PRO/CON scout statuses. |
| `POST` | `/api/ideas` | **202** | Persist an idea and fire-and-forget the Day 0 workflow. |
| `GET`  | `/api/ideas/{id}` | 200 | One idea with its scouts (and any pending proposals) + recent findings. |
| `POST` | `/api/proposals/{id}/respond` | 200 | Human APPROVE/REJECT of a prompt proposal. `409` if already resolved. |
| `DELETE` | `/api/scouts/{id}` | 200 | Stop a scout: mark `STOPPED` + delete on Yutori (halts credit usage). Idempotent. |
| `POST` | `/api/webhooks/yutori` | 202 | Receive a raw Yutori scout update; forward to the worker. |
| `POST` | `/api/webhooks/yutori/research/{awakeableID}` | 202 | Day 0 research-result callback; resolves the workflow's awakeable. |
| `GET`  | `/healthz` | 200 | Liveness probe (`{status:"ok"}`). |

### Create an idea — `POST /api/ideas`

Persists the idea, then asynchronously triggers the Day 0 workflow via the
Restate ingress. Workflow trigger failures are logged but never fail the HTTP
request — the idea is already saved and can be retried.

**Request body** (`title` optional; everything else inferred server-side):
```json
{
  "description": "A SaaS that auto-reviews pull requests with LLMs, aimed at mid-size eng teams.",
  "scoutingFrequencyDays": 7
}
```

**Response (202):**
```json
{ "id": "<uuid>", "status": "INITIAL_SWEEP", "workflowId": "<idea id>" }
```

### List ideas — `GET /api/ideas`

Returns `IdeaSummaryDTO[]`. Each carries the idea fields plus `proScoutStatus` /
`conScoutStatus`, which are `ACTIVE`, `PENDING_MUTATION`, `STOPPED`, or
`UNDEPLOYED` (Day 0 hasn't created that scout yet):
```json
[{
  "id": "…", "title": "…", "description": "…",
  "scoutingFrequencyDays": 7, "status": "ACTIVE",
  "totalPros": 14, "totalCons": 6,
  "proScoutStatus": "ACTIVE", "conScoutStatus": "PENDING_MUTATION",
  "createdAt": "2026-07-09T…", "lastUpdated": "2026-07-09T…"
}]
```

### One idea — `GET /api/ideas/{id}`

Returns `IdeaDetailDTO` = the summary **plus** `scouts[]` (each with an optional
`pendingProposal`) and `recentPros[]` / `recentCons[]` (capped at **20** each,
newest first):
```json
{
  "id": "…", "title": "…", "description": "…", "status": "ACTIVE",
  "scoutingFrequencyDays": 7, "totalPros": 14, "totalCons": 6,
  "proScoutStatus": "ACTIVE", "conScoutStatus": "PENDING_MUTATION",
  "createdAt": "…", "lastUpdated": "…",
  "scouts": [{
    "id": "…", "scoutType": "CON", "status": "PENDING_MUTATION",
    "currentPrompt": "…",
    "pendingProposal": { "id": "…", "proposedPrompt": "…", "status": "PENDING", "createdAt": "…" }
  }],
  "recentPros": [{ "id": "…", "polarity": "PRO", "platform": "reddit",
    "quote": "…", "reason": "…", "sourceUrl": "…", "sourceTitle": "…", "createdAt": "…" }],
  "recentCons": []
}
```

### Respond to a proposal — `POST /api/proposals/{id}/respond`

**Request body** (`edited_text` only used on `APPROVE`):
```json
{ "action": "APPROVE", "edited_text": "optional hand-edited prompt text" }
```
**Response:** `{ "status": "APPROVED" }` or `{ "status": "REJECTED" }`.
Returns **409** if the proposal isn't `PENDING`.

---

## Configuration

Config is **env-only** (loaded via `godotenv`; existing env vars beat `.env`).
Only `DATABASE_URL` is hard-required at startup; everything else has a default.
`validator-backend/.env.example` is the source of truth.

| Var | Used by | Default | Purpose |
|-----|---------|---------|---------|
| `DATABASE_URL` | both | *(required)* | Postgres connection string (lib/pq format). |
| `YUTORI_API_KEY` | worker | — | `X-API-Key` for Research + Scouting APIs. |
| `HTTP_ADDR` | api | `:8080` | Public API listen addr (`.env` → `:8000`). |
| `RESTATE_DEPLOYMENT_ADDR` | worker | `:9080` | Address Restate dials to reach the worker. |
| `RESTATE_INGRESS_URL` | api | `http://localhost:8080` | Restate ingress the API calls to trigger work. |
| `RESTATE_AUTH_KEY` | api | *(optional)* | Bearer token if the ingress needs auth. |
| `WEBHOOK_PUBLIC_URL` | worker | *(required for full cycle)* | Public base URL; worker appends `/api/webhooks/yutori`. Empty → Day 0 fails fast (research results can't return). |
| `YUTORI_API_BASE` | worker | `https://api.yutori.com` | Yutori API root. |
| `YUTORI_TIMEOUT_SECONDS` | worker | `60` | Per Yutori HTTP call timeout. |
| `LLM_API_BASE` | worker | `https://api.openai.com/v1` | OpenAI-compatible base (e.g. `https://api.groq.com/openai/v1`). |
| `LLM_API_KEY` | worker | *(optional)* | If empty, brief generation falls back to raw idea text and mutation eval is skipped (no Yutori credits used for eval). |
| `LLM_MODEL` | worker | `gpt-4o-mini` | Chat model id. |
| `LLM_TIMEOUT_SECONDS` | worker | `30` | Per LLM call timeout. |
| `SCOUT_INTERVAL_SECONDS` | worker | `0` | Overrides `scoutingFrequencyDays` for local testing. **Must be `>= 1800`** (Yutori minimum). `0` = use days. |

**Restate deployment registration** (one-time, and after any handler-shape
change): the worker must be registered before invocations can be dispatched.
Restate runs in Docker, so it reaches the host worker via the Docker bridge
gateway:
```bash
restate deployments register http://172.17.0.1:9080 --yes   # add --force if handlers changed shape
curl -s http://localhost:9070/services                       # verify Day0SetupWorkflow + ScoutOps
```

**Webhook delivery** requires a publicly-reachable `WEBHOOK_PUBLIC_URL`.
**`cloudflared` is recommended** for local dev (free, stable, no session/time
limit); ngrok and pinggy also work but their free tiers drop connections often
(which surfaces as a Day-0 *"research task did not report back within 15m0s"*
terminal failure). `WEBHOOK_PUBLIC_URL` is read at worker startup, so restart
the worker after changing it.
```bash
cloudflared tunnel --url http://localhost:8000    # copy the trycloudflare.com URL
```

---

## Frontend & UI

Vite + React 18 + TypeScript. Three routes (`src/App.tsx`), all wrapped in a
`ThemeProvider` + `Layout`:

| Route | Component | Role |
|-------|-----------|------|
| `/` | `Dashboard` | List of tracked ideas + summary stats. |
| `/new` | `NewIdeaForm` | Describe an idea, pick frequency, deploy scouts. |
| `/idea/:id` | `IdeaDetailDashboard` | The split PRO/CON board for one idea. |

- **State:** `ThemeContext` (dark/light, persisted), and **Zustand**
  (`src/store/useIdeaStore.ts`) for the dashboard list/onboarding. The detail
  board fetches its own idea (`api.getIdea`) and **polls every 15s**.
- **Pages:** *Dashboard* shows stat cards + idea grid; *NewIdeaForm* takes the
  description (≥ 12 chars) + frequency and navigates to `/idea/:id` on success;
  *IdeaDetailDashboard* is a two-column **split board** that shows a *Running
  Day 0 Research* placeholder while `status === INITIAL_SWEEP`, then lists
  `FindingCard`s per side, a banner that opens the `PromptApprovalDrawer` when a
  scout is `PENDING_MUTATION`, and a **Stop scout** button per column.
- **Components:** `FindingCard` (per-signal card with platform icon, quote,
  reason, source link); `PromptApprovalDrawer` (side-by-side *Active* vs
  *AI-Proposed* prompt, editable, *Approve/Reject*); `Layout` + `Icons.tsx`
  (re-exports `lucide-react`).
- **Styling:** Tailwind CSS + `lucide-react` icons only (see
  `validator-ui/.bolt/prompt`); custom SVGs only for the logo.

---

## Project structure

```text
validator/
├── README.md                              # You are here — setup & usage
├── architecture.md                        # System design, data flows, diagrams
├── AGENTS.md                              # Engineering conventions
├── validator-backend/                     # Go 1.25 service (module validator-backend)
│   ├── cmd/
│   │   ├── api/main.go                    # Public API surface (HTTP)
│   │   └── worker/main.go                 # Internal worker (Restate SDK server)
│   ├── internal/
│   │   ├── api/                           # REST handlers + camelCase DTOs + CORS/logging
│   │   │   ├── server.go                  #   routing, ingress/forward helpers
│   │   │   ├── handlers.go                #   endpoint handlers
│   │   │   └── dto.go                     #   JSON contract (mirrors UI types)
│   │   ├── config/config.go              # Env-only config (.env via godotenv)
│   │   ├── db/db.go                       # Postgres pool, embedded schema, tx queries
│   │   ├── llm/llm.go                    # OpenAI-compatible chat client (Groq etc.)
│   │   ├── models/models.go              # Domain types (snake_case, DB layer)
│   │   ├── scouts/                        # Stateless Yutori HTTP client
│   │   │   ├── client.go                  #   HTTP core + JSON schemas + decode
│   │   │   ├── research.go               #   Research API (create/webhook/decode)
│   │   │   └── scouting.go               #   Scouting API (create/patch/delete)
│   │   └── workflow/                      # Restate durable components
│   │       ├── workflow.go               #   Day0SetupWorkflow + terminalf + awaitResearch
│   │       └── scoutops.go               #   ScoutOps (ProcessWebhook / ResolveResearch / ApplyApproval / DeleteScout)
│   ├── .env.example                       # Source of truth for env config
│   └── go.mod
└── validator-ui/                          # Vite + React + TS frontend
    ├── src/
    │   ├── pages/                         # Dashboard, NewIdeaForm, IdeaDetailDashboard
    │   ├── components/                    # FindingCard, PromptApprovalDrawer, Layout, Icons
    │   ├── store/useIdeaStore.ts         # Zustand store (list + onboarding)
    │   ├── context/ThemeContext.tsx      # Dark/light theme provider
    │   ├── lib/api.ts                     # fetch client (relative /api)
    │   ├── types/index.ts                 # camelCase types mirroring dto.go
    │   ├── App.tsx · main.tsx · index.css
    ├── vite.config.ts                     # dev proxy /api -> http://localhost:8000
    └── package.json
```

---

## Development commands

**Backend** (from `validator-backend/`):
```bash
go run ./cmd/api         # start the public API surface (terminal 1)
go run ./cmd/worker      # start the internal worker (terminal 2) — both must run
go test ./...            # unit tests only (no DB/network needed)
go vet ./...             # the only static check; no separate lint step
```

**Frontend** (from `validator-ui/`):
```bash
npm run dev              # dev server; proxies /api -> http://localhost:8000 (VITE_API_TARGET)
npm run build            # production build
npm run preview          # preview the build
npm run lint             # eslint
npm run typecheck        # tsc --noEmit -p tsconfig.app.json  (non-default config)
```

---

## Technology stack

**Backend** (`validator-backend/`, Go 1.25) — durable orchestration via **Restate**
(`github.com/restatedev/sdk-go`); web research/scouting via **Yutori**
(`X-API-Key`); **PostgreSQL** via `github.com/lib/pq`; cheap evals on an
OpenAI-compatible LLM (**Groq**); Go 1.22+ method-pattern `http.ServeMux` (no
router library); `log/slog`. No separate lint step — `go vet` is the check.

**Frontend** (`validator-ui/`, Vite + React 18 + TypeScript) — **Zustand** for
dashboard/onboarding state; small **fetch** wrapper using relative URLs in dev
(proxied by Vite); `react-router-dom`; **Tailwind CSS** + **`lucide-react`**
icons only.

---

## Further reading

- [`architecture.md`](./architecture.md) — system design, the complete
  idea→cycle data flow, ASCII topology & sequence diagrams, data model, state
  machines, Restate/Yutori component reference, failure modes.
- [`AGENTS.md`](./AGENTS.md) — engineering conventions and commands for working
  in this codebase.
