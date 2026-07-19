# Validator — Architecture & Data Flow

This document describes Validator's system design and the **complete data flow**
from idea creation through the full validation cycle. For setup/how-to, see
[`README.md`](./README.md). For engineering conventions, see [`AGENTS.md`](./AGENTS.md).

---

## 1. Design philosophy

Validator validates a business idea with **evidence, not scores**. Three ideas
shape the architecture:

1. **Split surface.** A thin internet-facing API handles only synchronous
   requests (onboarding, approvals, webhook ingress). All heavy/long-lived work
   lives in a private worker behind [Restate](https://restate.dev) durable
   execution. The two share a Postgres pool but nothing else.
2. **Fire-and-forget at the edge.** The API persists state synchronously, then
   hands work to the worker via the Restate ingress **without waiting**. HTTP
   requests stay fast; trigger failures never lose an already-saved idea.
3. **Human-in-the-loop mutations.** Scouts run autonomously, but any change to a
   scout's tracking prompt requires explicit human approval. The AI proposes; a
   human disposes.

The external world is [Yutori](https://yutori.com) (web research/scouting) and an
OpenAI-compatible LLM (Groq by default) for cheap evals that must never burn
Yutori credits.

---

## 2. System topology

```text
                            ┌────────────────────────┐
                            │   Browser (React UI)   │
                            │  Vite dev proxy /api/*  │
                            └───────────┬────────────┘
                                        │ HTTP (relative in dev)
                                        ▼
╔════════════════════════════════════════════════════════════════════════════╗
║                              cmd/api  (Public API)                          ║
║                          HTTP_ADDR :8000 (default)                          ║
║                                                                            ║
║   • POST   /api/ideas                              create idea + trigger   ║
║   • GET    /api/ideas · /api/ideas/{id}            list / detail            ║
║   • POST   /api/proposals/{id}/respond            APPROVE / REJECT          ║
║   • DELETE /api/scouts/{id}                       stop scout                ║
║   • POST   /api/webhooks/yutori                   scout-update ingress      ║
║   • POST   /api/webhooks/yutori/research/{awId}   Day 0 result ingress      ║
║   • GET    /healthz                               liveness                 ║
║                                                                            ║
║   Reads/writes Postgres directly (sync). Forwards heavy work to the worker. ║
╚═══════════════════╤═════════════════════════════════╤═══════════════════════╝
                    │                                 │ restate ingress .Send()
                    │ shared PG pool                  │ (fire-and-forget)
                    ▼                                 ▼
╔═════════════════════════════════╗   ┌───────────────────────────────────────────╗
║          PostgreSQL             │   │        cmd/worker  (Restate SDK)           ║
║   RESTATE_DEPLOYMENT_ADDR :9080 │◄──┤                                           ║
║                                │   │  ┌───────────────────────────────────────┐ ║
║  ideas                         │   │  │ Day0SetupWorkflow  (workflow/key=idea) │ ║
║  scouts                        │◄──┤  │   Run(ctx, Day0Input)                  │ ║
║  prompt_proposals              │   │  └───────────────────────────────────────┘ ║
║  market_signals                │   │  ┌───────────────────────────────────────┐ ║
║                                │   │  │ ScoutOps  (service)                    │ ║
║  (auto-applied schema +        │   │  │   ProcessWebhook(raw)                  │ ║
║   one-time constraint          │   │  │   ResolveResearch({awId, payload})     │ ║
║   migration on startup)        │   │  │   ApplyApproval({scout_id, prompt})    │ ║
╚═════════════════════════════════╝   │  │   DeleteScout({scout_id})              │ ║
                                      │  └───────────────────────────────────────┘ ║
                                      └────────┬──────────────────┬───────────────┘
                                               │                  │
                                  X-API-Key    │                  │ OpenAI-compat
                                       ┌───────▼────────┐  ┌──────▼──────────────┐
                                       │    Yutori      │  │  Groq / any OAI LLM │
                                       │  api.yutori.com│  │  api.groq.com/...    │
                                       │                │  │                      │
                                       │ Research (1-   │  │ • research brief     │
                                       │   shot, by     │  │ • ReviewMutation     │
                                       │   webhook)     │  │   (eval, NO Yutori)  │
                                       │ Scouting       │  └──────────────────────┘
                                       │  (recurring +  │
                                       │   webhook)     │
                                       └───────┬────────┘
                                               │ recurring scout + research-result
                                               │ webhooks  (webhook_format: "scout")
                                       ┌───────▼────────┐
                                       │ Public tunnel  │  cloudflared (recommended)
                                       │ cloudflared /  │  ngrok / pinggy also work
                                       │ ngrok / pinggy │
                                       └───────┬────────┘
                                               │ routes back to cmd/api :8000
                                               ▼
                                      (webhook ingress, see above)
```

### Why two binaries

| Concern | cmd/api | cmd/worker |
|---|---|---|
| Exposure | Internet-facing | Private network only |
| Latency budget | per-request (ms) | per-side-effect (seconds–minutes) |
| Talks to | Postgres, Restate ingress | Postgres, Yutori, LLM |
| Failure model | return HTTP error | durable retry under `boundedRunOpts` |
| Restate role | ingress *client* (sends work) | ingress *server* (runs handlers) |

---

## 3. Restate — the durability backbone

Every external side effect inside the worker is wrapped in a journaled
`restate.Run` / `RunAsync` / `RunVoid`. If the worker crashes mid-flow, Restate
**replays the exact journal entry** on recovery rather than restarting the whole
operation. Three primitives matter here:

- **`Run` / `RunAsync` / `RunVoid`** — a journaled closure. On first execution
  the result is stored; on replay the stored result is returned **without
  re-issuing the external call** (so a POST to Yutori is never duplicated).
- **Awakeables** — a resolvable placeholder the workflow can suspend on. The Day
  0 flow creates an awakeable, embeds its id in the Yutori task's webhook URL,
  then suspends until `ResolveResearch` resolves it with the inbound result.
- **`WaitFirst`** — a durable race. Day 0 races the awakeable against a 15-minute
  timer; whoever wins first wins.

**Retry discipline.** Transient failures retry under `boundedRunOpts`
(`WithMaxRetryAttempts(5)`, `WithMaxRetryDuration(2m)`). Definitive failures
(parse errors, business-rule violations, 4xx, timeouts) are converted to a
`restate.TerminalError` by `wrapClosureErr` so they **terminate** instead of
replaying forever — this is what prevents a transient Yutori hiccup from turning
into a credit-burning POST loop.

---

## 4. The complete idea lifecycle

Four phases. Each is durable on the worker side.

### Phase 1 — Idea creation & Day 0 launch

```text
 UI: NewIdeaForm                cmd/api                        cmd/worker
 ───────────────                ───────                        ──────────
 POST /api/ideas ─────────────► handlePostIdea
                                • validate (desc ≥ 1 char)
                                • derive title if missing
                                • INSERT ideas (status=
                                  INITIAL_SWEEP), RETURN id
                                • triggerDay0() fire-and-─►   Day0SetupWorkflow.Run
                                  forget via Restate           (keyed by idea id)
 ◄─────────── 202 {id,status,─┘                                • IntervalDays default 7
   workflowId}                                                   • (SCOUT_INTERVAL_SECONDS
                                                                   overrides for testing)

 → UI navigates to /idea/:id, polls GET /api/ideas/{id} every 15s
```

### Phase 2 — Day 0 research (webhook-driven, single call)

The workflow makes **exactly one** Yutori Research call. No polling, no loops.

```text
 Day0SetupWorkflow.Run(ctx, input)
 │
 0. brief = IdeaTitle + IdeaDescription
    if LLM configured:  brief = GenerateResearchBrief(Groq)      ── expands the raw
                                                                      idea into a grounded
                                                                      research brief
    (durable Run; falls back to raw text on failure)
 │
 1. awaitResearch():                          ┌──── 15-min durable timer ────┐
    • aw = Awakeable[json.RawMessage](ctx)    │                              │
    • webhookURL = WEBHOOK_PUBLIC_URL         │   WaitFirst(aw, timer)       │
        + "/api/webhooks/yutori/research/"    │                              │
        + url.PathEscape(aw.Id())   ◄── path, │   ◄─ Yutori calls back:      │
        NOT query (Yutori strips query)       │       POST /api/webhooks/    │
    • Run: CreateDay0TaskWithWebhook(         │            yutori/research/  │
        brief, webhookURL)  ── POST ───────►  │            {awakeableID}     │
        Yutori /v1/research/tasks             │            ↓ cmd/api          │
        (webhook_format: "scout",             │       resolveResearch() ─►   │
         output_schema = Day0Schema)          │       ScoutOps.ResolveResearch│
    • SUSPEND until aw resolves or timer      │       → ResolveAwakeable(aw, │
      fires                                  │          raw structured_result)│
                                              └──────────────────────────────┘
 │
 2. day0 = DecodeDay0Result(aw.Result())
      → { pro_prompt, con_prompt,
          pro_signals[], con_signals[] }
 │
 3. deploy TWO scouts in parallel:
       proFut = RunAsync( deployScout(PRO, day0.ProPrompt) )
       conFut = RunAsync( deployScout(CON, day0.ConPrompt) )
       Wait(proFut, conFut)
     each deployScout():
       • CreateScout(Yutori /v1/scouting/tasks)
            query          = the prompt
            output_schema  = SignalSchema
            output_interval= IntervalDays*86400 (or SCOUT_INTERVAL_SECONDS)
            webhook_url    = WEBHOOK_PUBLIC_URL/api/webhooks/yutori
            skip_email     = true
       • INSERT scouts (status=ACTIVE, yutori_scout_id)
 │
 4. RecordSignals under each scout
      (market_signals rows + idea.total_pros/total_cons bump)
 │
 5. ActivateIdea → status = ACTIVE
 │
 ▼  workflow completes (one instance per idea, never re-runs for the same key)
```

**Why one call:** the Day-0 directive (`day0Query`) instructs Yutori to *both*
harvest sourced signals *and* write the two monitoring prompts grounded in what
it found — no lossy digest round-trip. The pro/cons prompts are **not** passed
through Groq for "solidifying"; Groq only expands the raw idea into the brief
beforehand.

### Phase 3 — Recurring scout → mutation proposal (continuous)

Once a scout is `ACTIVE`, Yutori runs it on its interval and delivers each output
as a nested "scout" webhook. **Correlation is by `scout.id` (stable), never by
`update.id` (transient per run).**

```text
        Yutori (scout interval fires)
                   │
                   │ POST /api/webhooks/yutori      (webhook_format: "scout")
                   │   { event_type:"scout_update",
                   │     scout:{ id:<stable>, ... },
                   │     update:{ id:<per-run>,
                   │               structured_result:{ signals:[…] } },
                   │     delivery:{…} }
                   ▼
              cmd/api ──► forwardWebhook(raw) ──► ScoutOps.ProcessWebhook
                                                          │ (journaled)
   ┌──────────────────────────────────────────────────────┘
   ▼
 1. looksLikeResearchResult?  ──► drop (defense-in-depth; research uses
    its own route)
 2. parseWebhookPayload:
       scout_id  = payload.scout.id            (stable scout id)
       signals   = payload.update.structured_result.signals
       (tolerates array / flat legacy shapes too)
 3. Run: GetScoutByYutoriID + RecordSignals (single tx)
       • insert market_signals rows
       • bump idea.total_pros / total_cons
       • capture scout context (ScoutID, IdeaID, type, current prompt, status)
 4. if scout.status == PENDING_MUTATION or STOPPED ──► stop
       (no second proposal; no work on a stopped scout)
 5. if !LLMConfigured ──► stop (signals already recorded)
 6. Run: ReviewMutation(Groq LLM, NOT Yutori)
       → { should_expand:bool, proposed_prompt:string, reasoning }
 7. if should_expand && proposed_prompt != "":
       CreateProposal (single tx):
         • INSERT prompt_proposals (status=PENDING)
         • UPDATE scouts SET status=PENDING_MUTATION   (ONLY this scout)
       ── the sibling scout is never touched
```

Bad payloads, unknown scouts, empty-signal webhooks, and a missing LLM key are
acked (202) and dropped — they don't block ingestion or retry forever.

### Phase 4a — Human approval

```text
 UI: PromptApprovalDrawer              cmd/api                         cmd/worker
 ──────────────────────              ───────                         ──────────
 (side-by-side: active prompt
  vs AI-proposed, editable)
 POST /api/proposals/{id}/respond ─► handleRespondProposal
   { action:"APPROVE",                   • load proposal; 409 if !PENDING
     edited_text? }                      • finalPrompt = edited_text
                                          ?? proposedPrompt
                                        • UpdateScoutPrompt(DB) ──►
                                            scouts.current_prompt=newPrompt
                                            status=ACTIVE
                                        • ResolveProposal(APPROVED)
                                        • applyApproval() ───────► ScoutOps.ApplyApproval
 ◄─────────────── 200 {status:        (fire-and-forget)              • Run: PatchScout(Yutori
   "APPROVED"}                                                       /v1/scouting/tasks/{id},
                                                                    { query:newPrompt })

 REJECT branch: ResolveProposal(REJECTED); scout restored to ACTIVE,
 original prompt retained → 200 {status:"REJECTED"}
```

### Phase 4b — Stop a scout (halt credit usage)

```text
 UI: "Stop scout" button              cmd/api                         cmd/worker
 ──────────────────                 ───────                         ──────────
 DELETE /api/scouts/{id} ─────────► handleDeleteScout
                                       • load scout
                                       • if already STOPPED → 200 (idempotent)
                                       • SetScoutStatus(STOPPED) in DB
                                       • stopScout() ──────────► ScoutOps.DeleteScout
 ◄────────────────── 200                (fire-and-forget)              • Run: DeleteScout(Yutori
   {status:"STOPPED"}                                                  /v1/scouting/tasks/{id},
                                                                       DELETE — 404 treated
                                                                       as success)
```

A stopped scout no longer runs on Yutori (credits stop), is excluded from
mutation eval, and the UI hides its review banner + stop button.

---

## 5. State machines

```text
 idea.status                      scout.status
 ───────────                      ───────────
 ┌──────────────┐   Day0 done    ┌────────┐  proposal opened   ┌──────────────────┐
 │INITIAL_SWEEP│──────────────► │ ACTIVE │──────────────────► │ PENDING_MUTATION │
 └──────────────┘                └────┬───┘                     └────────┬─────────┘
     │                                │ approve/reject                   │
     │ (Day 0 terminal failure        │ ──► ACTIVE                       │ DELETE /scouts/{id}
     │  leaves it in INITIAL_SWEEP)   │                                  ▼
     ▼                                │                            ┌────────┐
   (stuck)                            │ STOPPED ◄──── DELETE ◄─────│        │
                                      └────────┘                   └────────┘

 prompt_proposals.status
 ───────────────────────
 PENDING ──APPROVE──► APPROVED     (scout prompt updated + Yutori PATCHed)
 PENDING ──REJECT ──► REJECTED     (scout unchanged, restored to ACTIVE)
```

---

## 6. Data model (ER)

```text
 ┌─────────────────────┐         ┌──────────────────────────┐
 │       ideas         │ 1     N │          scouts          │
 ├─────────────────────┤─────────┤──────────────────────────┤
 │ id          UUID PK │         │ id            UUID PK     │
 │ title       TEXT    │ ◄────── │ idea_id       UUID FK ──┐ │ ON DELETE CASCADE
 │ description TEXT    │         │ yutori_scout_id UNIQUE  │ │
 │ frequency_days INT  │         │ scout_type    PRO|CON   │ │
 │ status      TEXT    │         │ current_prompt TEXT     │ │
 │ total_pros  Int     │         │ status        ACTIVE|   │ │
 │ total_cons  Int     │         │               PENDING_  │ │
 │ created_at/updated  │         │               MUTATION| │ │
 └─────────────────────┘         │               STOPPED   │ │
        ▲                        └────────────┬─────────────┘ │
        │ 1                                 N │               │
        │                                    ▼               │
        │                     ┌──────────────────────────┐    │
        │                     │    market_signals        │    │
        │                     ├──────────────────────────┤    │
        ├─────────────────────│ idea_id  UUID FK ────────┼────┘
        │                  N  │ scout_id  UUID FK ───────┼──────► scouts
        │                     │ polarity PRO|CON         │
        │                     │ platform, quote, reason, │
        │                     │ source_url, source_title │
        │                     │ created_at               │
        │                     └──────────────────────────┘
        │                                  ▲
        │                                  │ 1
        │                     ┌──────────────────────────┐
        │                  N  │   prompt_proposals       │
        └─────────────────────┤ (per scout, one PENDING  │
                              │  at a time)              │
                              ├──────────────────────────┤
                              │ id            UUID PK     │
                              │ scout_id      UUID FK ────┼────► scouts
                              │ proposed_prompt TEXT      │
                              │ status PENDING|APPROVED|  │
                              │         REJECTED          │
                              │ created_at, resolved_at   │
                              └──────────────────────────┘
```

- `market_signals` is the **evidence layer** — each row is one sourced finding.
- `ideas.total_pros / total_cons` are **denormalized rollup counters** for fast
  dashboard reads (bumped in the same tx as signal inserts).
- All FKs are `ON DELETE CASCADE`: deleting an idea removes its scouts, signals,
  and proposals.

### `ideas` — a market hypothesis being validated
| column | type | notes |
|---|---|---|
| `id` | UUID PK | generated by the API |
| `title` | TEXT | derived from description if not supplied |
| `description` | TEXT | required |
| `frequency_days` | INTEGER | default `7` |
| `status` | TEXT | `INITIAL_SWEEP` → `ACTIVE` |
| `total_pros` / `total_cons` | INTEGER | denormalized signal counts, bumped per batch |
| `created_at` / `updated_at` | TIMESTAMPTZ | |

### `scouts` — one Yutori scouting task (PRO or CON) attached to an idea
| column | type | notes |
|---|---|---|
| `id` | UUID PK | |
| `idea_id` | UUID FK → ideas `ON DELETE CASCADE` | |
| `yutori_scout_id` | VARCHAR(255) UNIQUE | the stable live Yutori task id (used for webhook correlation) |
| `scout_type` | VARCHAR(4) | `PRO` or `CON` |
| `current_prompt` | TEXT | the active tracking query (updated on approval) |
| `status` | VARCHAR(20) | `ACTIVE`, `PENDING_MUTATION`, or `STOPPED` |
| `created_at` / `updated_at` | TIMESTAMPTZ | |

Indexes: `idx_scouts_idea(idea_id)`, `idx_scouts_yutori(yutori_scout_id)`.

### `prompt_proposals` — an AI-proposed search-radius expansion awaiting review
| column | type | notes |
|---|---|---|
| `id` | UUID PK | |
| `scout_id` | UUID FK → scouts `ON DELETE CASCADE` | |
| `proposed_prompt` | TEXT | full revised prompt |
| `status` | VARCHAR(20) | `PENDING` → `APPROVED` / `REJECTED` |
| `created_at` | TIMESTAMPTZ | |
| `resolved_at` | TIMESTAMPTZ | nullable until resolved |

Indexes: `idx_proposals_scout(scout_id)`,
`idx_proposals_pending(scout_id) WHERE status = 'PENDING'`.

### `market_signals` — a single harvested pro/con finding
| column | type | notes |
|---|---|---|
| `id` | UUID PK | |
| `idea_id` | UUID FK → ideas `ON DELETE CASCADE` | |
| `scout_id` | UUID FK → scouts `ON DELETE CASCADE` | |
| `polarity` | VARCHAR(4) | `PRO` or `CON` (matches the scout's type) |
| `platform` | TEXT | e.g. `reddit`, `youtube`, `news`, `social`, `web` |
| `quote` | TEXT | the verbatim finding |
| `reason` | TEXT | why it's a pro/con |
| `source_url` | TEXT | required |
| `source_title` | TEXT | |
| `created_at` | TIMESTAMPTZ | |

Indexes: `idx_signals_idea(idea_id, created_at DESC)`, `idx_signals_scout(scout_id)`.

**Signal sanitization:** Yutori-returned signals missing a `quote` or
`source_url` are dropped; blank `platform` becomes `web`; blank `reason` gets a
default string. Schema is auto-applied on startup (`CREATE TABLE IF NOT EXISTS`
+ a one-time `scouts_status_check` constraint migration that adds `STOPPED`).

---

## 7. Components & contracts

### Restate components (`internal/workflow`)

Both are registered via `restate.Reflect(&struct{})` with pointer-receiver methods
in `cmd/worker/main.go`. Service-name constants live in
`internal/workflow/workflow.go`.

#### `Day0SetupWorkflow` (a Restate **workflow**)
- **Service name:** `Day0SetupWorkflow`. Keyed by idea id → one instance per idea.
- **Entry point:** `Run(ctx restate.WorkflowContext, input Day0Input) (Void, error)`.
- **`Day0Input`:** `idea_id`, `idea_title`, `idea_description`, `interval_days`.
- Steps: brief gen → webhook-driven research → decode → deploy two scouts in
  parallel (`RunAsync` + `restate.Wait`) → record signals → `ActivateIdea`.
  Completes once.

#### `ScoutOps` (a Restate **service**)
- **Service name:** `ScoutOps`.
- **`ProcessWebhook(ctx, raw json.RawMessage) (Void, error)`** — records signals,
  runs the mutation eval, opens a proposal if warranted.
- **`ResolveResearch(ctx, input ResolveResearchInput) (Void, error)`** — resolves
  (or rejects) the Day 0 workflow's waiting awakeable with the inbound Yutori
  research-task result. The awakeable id travelled in the webhook URL path.
- **`ApplyApproval(ctx, input ApprovalInput) (Void, error)`** — PATCHes a Yutori
  scout with an approved prompt (DB is already updated by the API).
- **`DeleteScout(ctx, input DeleteScoutInput) (Void, error)`** — DELETEs a Yutori
  scout to halt recurring credit usage (DB already marked STOPPED by the API).

**Side-effect discipline:** every DB/Yutori/LLM call lives inside `restate.Run`,
`RunAsync`, or `RunVoid`. Permanent failures use `terminalf` (defined in
`internal/workflow/workflow.go`) to preserve terminal-error semantics across
suspension/resume — never `fmt.Errorf`.

### Yutori integration (`internal/scouts`)

Stateless HTTP client. **Auth is the `X-API-Key` header** (`YUTORI_API_KEY`).

- **Research API** (`/v1/research/tasks`) — one-shot. Day 0 creates ONE task
  whose combined directive both harvests signals and writes the PRO/CON prompts.
  Result delivered **by webhook** into a Restate awakeable (no polling). A
  polling path (`Research()`) exists for ad-hoc inline use but is not used by
  Day 0.
- **Scouting API** (`/v1/scouting/tasks`) — recurring. `POST` creates a scout
  (`output_interval ≥ 1800`, `webhook_url`, `output_schema=SignalSchema`,
  `skip_email=true`); `PATCH` updates the query (on approval); `DELETE`
  stops/deletes it (on Stop scout). Each run arrives as the nested "scout"
  webhook shown in phase 3.

**JSON Schemas** (defined in `client.go`):

| Schema | Used by | Shape (required fields) |
|---|---|---|
| `SignalSchema` | research sweep + every scout's `output_schema` | `{signals:[{quote, source_url, platform, reason, source_title}]}` |
| `Day0Schema` | the Day 0 research task | `{pro_prompt, con_prompt, pro_signals[], con_signals[]}` (signals use SignalSchema) |

**Mutation eval (`ReviewMutation`) deliberately does NOT use Yutori** — it runs on
the OpenAI-compatible LLM (`LLM_*`, e.g. Groq), so reviewing signals never burns
Yutori credits. With `LLM_API_KEY` empty, the eval is skipped (signals are still
recorded).

### Conventions

- **Duplicated JSON contract, by design.** `internal/api/dto.go` emits camelCase
  that exactly matches `validator-ui/src/types/index.ts`; `internal/models` is
  the snake_case DB shape. When changing one side, update the other.
- **CORS is intentionally permissive** (`Access-Control-Allow-Origin: *`) because
  the UI is served from a different origin; preflight `OPTIONS` is short-circuited
  with `204`.
- **Fire-and-forget at the edge.** The API persists state synchronously, then
  sends work to the worker via the Restate ingress without waiting. Trigger
  failures are logged but never fail an already-persisted request.
- **One workflow per idea** — long-lived monitoring lives on Yutori's side (the
  recurring scout), not in a durable loop.
- **Isolated mutation** — a proposal only ever affects the single scout whose
  signals triggered it; the sibling scout is untouched.
- **Config is env-only** (`godotenv`); there is no config file loader.
- **Go routing** uses method-pattern `http.ServeMux` and `log/slog`; no router
  library, no separate lint step (`go vet` is the check).

---

## 8. End-to-end sequence (happy path)

```text
 UI            cmd/api          Restate           cmd/worker         Yutori          Groq
 ──            ───────          ───────           ──────────         ──────          ────
 POST /ideas ─► persist idea ──► trigger Day0 ──► workflow.Run
 (202)                                        ├─►                              brief gen ─►
                                              ├─► create research task ──────► (1 task)
                                              └─► SUSPEND on awakeable
                                                                                  research runs …
                                                              ◄────────────────  result webhook
                                              ├─► ResolveResearch ◄─ POST /webhooks/yutori/research/{aw}
                                              ├─► decode {pro_signals, con_signals, pro_prompt, con_prompt}
                                              ├─► create 2 scouts ──────────► (PRO + CON deployed)
                                              ├─► record signals (DB)
                                              └─► ActivateIdea, DONE
 (poll) GET /ideas/{id} ─► ACTIVE, 2 scouts, signals populated

                                  … scout interval fires (per scout) …
                                                              ◄────────────────  scout-update webhook
                                              ├─► ProcessWebhook ◄─ POST /webhooks/yutori
                                              ├─► RecordSignals (DB)
                                              ├─► ReviewMutation ───────────────────────────► (eval)
                                              └─► CreateProposal (scout → PENDING_MUTATION)
 (poll) GET /ideas/{id} ─► shows pendingProposal

 review + approve
 POST /proposals/{id}/respond ─► update DB ─► ApplyApproval ─► PatchScout ────► (live scout adopts new prompt)
 (200 APPROVED)
```

---

## 9. Key design decisions

- **One workflow per idea.** `Day0SetupWorkflow` is keyed by the idea id, runs
  once, and completes. Long-lived monitoring lives on Yutori's side (the
  recurring scout), **not** in a durable loop — this is what prevents the
  credit-burning infinite-loop bug an earlier design had.
- **Awakeable correlation for research.** The Day-0 result is delivered by
  webhook into a Restate awakeable (id in the URL **path**, since Yutori strips
  query params). This avoids any in-closure polling and lets the workflow suspend
  cheaply for minutes.
- **Stable-id correlation for scouts.** Recurring scout webhooks are matched by
  `scout.id` (stable), not `update.id` (transient per run). Mixing these up
  silently drops every recurring signal.
- **Isolated mutation.** A proposal only ever affects the single scout whose
  signals triggered it; the sibling scout is untouched, and a scout already
  `PENDING_MUTATION` (or `STOPPED`) won't accumulate a second proposal.
- **Eval ≠ Yutori.** `ReviewMutation` runs on a cheap OpenAI-compatible LLM, so
  reviewing incoming signals never spends Yutori credits. With no LLM key, the
  eval is skipped (signals are still recorded).
- **Duplicated JSON contract, by design.** `internal/api/dto.go` emits
  camelCase that exactly matches `validator-ui/src/types/index.ts`;
  `internal/models` is the snake_case DB shape. Change both sides together.

---

## 10. Failure modes & semantics

| Failure | Behaviour |
|---|---|
| Worker crash mid-workflow | Restate replays the exact journaled step on recovery (no duplicate external calls). |
| Transient Yutori/LLM error (5xx, network) | Retried under `boundedRunOpts` (≤5 attempts / ≤2 min). |
| Definitive error (4xx, parse, business rule, timeout) | `wrapClosureErr` → `restate.TerminalError` → terminates, not retried. |
| Yutori research webhook never arrives | 15-min durable timer fires → terminal failure (idea stays `INITIAL_SWEEP`). Fix `WEBHOOK_PUBLIC_URL` and retry with a new idea. |
| Tunnel (cloudflared/ngrok/pinggy) drops | Yutori retries the webhook delivery up to 3× (~30s). If all fail, that update is lost (not re-queued) but the scout keeps running. |
| `WEBHOOK_PUBLIC_URL` empty at worker startup | Day 0 fails fast with a clear terminal config error (research cannot return). |
| `SCOUT_INTERVAL_SECONDS < 1800` | Yutori rejects scout creation with `400 "Output interval must be >= 1800"`. |
| Unknown scout in a webhook | Acked & dropped (could be an orphan from a prior test; not retried). |
| Stop scout on already-deleted Yutori scout | Yutori returns 404 → treated as success; DB marked `STOPPED`. |

---

## 11. Local port map

| Port | Owner | Notes |
|---|---|---|
| `8000` | cmd/api | `HTTP_ADDR` (default `:8080`; `.env` uses `:8000` to avoid clashing with Restate). |
| `8080` | Restate runtime | Ingress (API calls here) + admin (`:9070`). |
| `9080` | cmd/worker | `RESTATE_DEPLOYMENT_ADDR` — Restate dials this. |
| `5432` | PostgreSQL | Shared by both binaries. |
| `5173` | Vite dev server | Proxies `/api` → `:8000`. |
