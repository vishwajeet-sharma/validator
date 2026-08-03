#!/usr/bin/env bash
# Validator local-dev orchestrator (zero-dependency bash equivalent of the Makefile).
#
# Brings up the full stack: Postgres + Restate in Docker, then the Go API and
# Worker binaries, registering the worker with Restate. Starts the Vite UI too
# unless it is already running.
#
# Docker is invoked through sudo (the dev user is not in the docker group); sudo
# caches the password for ~15 min so you only type it once per invocation.
#
# Usage:
#   ./dev.sh up        # start everything
#   ./dev.sh status    # show containers, app pids, listening ports
#   ./dev.sh down      # stop containers + app processes (keeps data volumes)
#   ./dev.sh help      # full command list
set -euo pipefail

# --- overridable knobs (e.g. `PG_PORT=5432 ./dev.sh up`) ---
: "${GO:=go}"
: "${SUDO:=sudo}"
: "${DOCKER:=${SUDO} docker}"

: "${PG_NAME:=validator-pg}"
: "${PG_IMAGE:=postgres:16}"
: "${PG_PORT:=5432}"            # 5432 matches the original data container (holds existing ideas)
: "${PG_USER:=validator}"
: "${PG_PASS:=validator}"
: "${PG_DB:=validator}"
: "${PG_VOLUME:=validator-pg-data}"

: "${RESTATE_NAME:=validator-restate}"
: "${RESTATE_IMAGE:=docker.io/restatedev/restate:latest}"
: "${RESTATE_INGRESS:=8080}"    # ingress (API surface calls this)
: "${RESTATE_META:=9070}"       # meta/admin (CLI + /services)
: "${RESTATE_VOLUME:=validator-restate-data}"

: "${API_PORT:=8000}"
: "${WORKER_PORT:=9080}"
: "${WORKER_HOST:=172.17.0.1}"  # docker0 gateway: how restate reaches the host worker
: "${UI_PORT:=5173}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND="$ROOT/validator-backend"
UI="$ROOT/validator-ui"
LOGDIR="$ROOT/.logs"

# --- helpers ---
log()   { printf '%s\n' "$*"; }
port_open() { (: < "/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

wait_port() { # <port> <label>
  local port="$1" what="$2" i
  for i in $(seq 1 60); do
    if port_open "$port"; then
      log "  [ready] $what on :$port"
      return 0
    fi
    sleep 0.5
  done
  log "  [timeout] $what on :$port not listening after 30s"
  return 1
}

cmd_build() {
  mkdir -p "$BACKEND/bin"
  ( cd "$BACKEND" && "$GO" build -o bin/api ./cmd/api && "$GO" build -o bin/worker ./cmd/worker )
  log "built validator-backend/bin/{api,worker}"
}

cmd_env_sync() {
  ( cd "$BACKEND" && sed -i -E "s|^DATABASE_URL=.*|DATABASE_URL=postgres://${PG_USER}:${PG_PASS}@localhost:${PG_PORT}/${PG_DB}?sslmode=disable|" .env )
  log "synced validator-backend/.env  ->  DATABASE_URL=postgres://${PG_USER}:***@localhost:${PG_PORT}/${PG_DB}"
}

start_app() { # <name> <bin> <port>
  local name="$1" bin="$2" port="$3"
  mkdir -p "$LOGDIR"
  # Kill any existing instance first so a port clash can never strand an orphan
  # that `down` then can't reach (its pidfile would be overwritten). Match by comm
  # name — safe because this script's own comm is "bash", not "api"/"worker".
  if pgrep -x "$bin" >/dev/null 2>&1; then
    log "$name: stopping existing instance..."
    pkill -x "$bin" 2>/dev/null; sleep 1
  fi
  # `exec` so the subshell process IS the binary -> $! is its real pid (not a
  # wrapper's), making pidfile-based stop reliable.
  ( cd "$BACKEND" && exec "./bin/$bin" ) > "$LOGDIR/$name.log" 2>&1 &
  echo $! > "$LOGDIR/$name.pid"
  log "$name: started (pid $!, log $LOGDIR/$name.log)"
  wait_port "$port" "$name"
}

# --- docker targets ---
cmd_db() {
  if $DOCKER inspect "$PG_NAME" >/dev/null 2>&1; then
    log "postgres: starting existing container $PG_NAME"; $DOCKER start "$PG_NAME"
  else
    log "postgres: creating container $PG_NAME on :$PG_PORT"
    $DOCKER run -d --name "$PG_NAME" \
      -e POSTGRES_USER="$PG_USER" -e POSTGRES_PASSWORD="$PG_PASS" -e POSTGRES_DB="$PG_DB" \
      -p "$PG_PORT:5432" "$PG_IMAGE"
  fi
  wait_pg "$PG_PORT"
}

# Real Postgres readiness: TCP-open fires before first-boot init finishes, so we
# wait until pg_isready reports "accepting connections".
wait_pg() { # <port>
  local port="$1" i
  log "  waiting for postgres to accept connections..."
  for i in $(seq 1 60); do
    if pg_isready -h 127.0.0.1 -p "$port" -t 1 2>/dev/null | grep -q "accepting connections"; then
      log "  [ready] postgres on :$port"; return 0
    fi
    sleep 0.5
  done
  log "  [timeout] postgres not ready on :$port"; return 1
}

cmd_restate() {
  if $DOCKER inspect "$RESTATE_NAME" >/dev/null 2>&1; then
    log "restate: starting existing container $RESTATE_NAME"; $DOCKER start "$RESTATE_NAME"
  else
    log "restate: creating container $RESTATE_NAME on :$RESTATE_INGRESS/:$RESTATE_META"
    $DOCKER run -d --name "$RESTATE_NAME" \
      -p "$RESTATE_INGRESS:8080" -p "$RESTATE_META:9070" \
      -e RESTATE_LOG_FORMAT=compact -e RESTATE_NODE=validator -v "$RESTATE_VOLUME:/restate-data" "$RESTATE_IMAGE"
  fi
  wait_port "$RESTATE_META" restate-meta
}

cmd_infra() { cmd_env_sync; cmd_db; cmd_restate; }

cmd_register() {
  wait_port "$RESTATE_META" restate-meta
  wait_port "$WORKER_PORT" worker
  restate config use-environment local 2>/dev/null || true
  log "restate: registering http://$WORKER_HOST:$WORKER_PORT ..."
  if ! restate deployments register "http://$WORKER_HOST:$WORKER_PORT" --yes; then
    log "restate: register failed. If components changed shape, re-run:"
    log "  restate deployments register http://$WORKER_HOST:$WORKER_PORT --force --yes"
    return 1
  fi
  log "restate: registered. Services:"
  curl -s "http://localhost:$RESTATE_META/services" || true
  echo
}

cmd_up() {
  cmd_infra
  cmd_build
  start_app api api "$API_PORT"
  start_app worker worker "$WORKER_PORT"
  cmd_register
  if port_open "$UI_PORT"; then
    log "ui already running on :$UI_PORT"
  else
    mkdir -p "$LOGDIR"
    ( cd "$UI" && nohup npm run dev > "$LOGDIR/ui.log" 2>&1 & echo $! > "$LOGDIR/ui.pid" )
    log "ui: started (pid $(cat "$LOGDIR/ui.pid"), log $LOGDIR/ui.log)"
    wait_port "$UI_PORT" ui
  fi
  log ""
  log "=========================================================="
  log " All services up"
  log "   UI:        http://localhost:$UI_PORT"
  log "   API:       http://localhost:$API_PORT"
  log "   Restate:   meta http://localhost:$RESTATE_META  ingress :$RESTATE_INGRESS"
  log "   Postgres:  localhost:$PG_PORT  (db=$PG_DB user=$PG_USER)"
  log " logs:       ./dev.sh logs-api | logs-worker | logs-pg | logs-restate | logs-ui"
  log " status:     ./dev.sh status      stop: ./dev.sh down"
  log "=========================================================="
}

cmd_stop_apps() {
  for f in api worker ui; do
    if [ -f "$LOGDIR/$f.pid" ]; then
      pid="$(cat "$LOGDIR/$f.pid" 2>/dev/null || true)"
      if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        kill "$pid" && log "stopped $f (pid $pid)"
      fi
      rm -f "$LOGDIR/$f.pid"
    fi
  done
  # Catch orphans whose pidfile was lost/overwritten (comm-based; safe).
  for f in api worker; do pkill -x "$f" 2>/dev/null && log "killed stray $f" || true; done
}

cmd_down() {
  cmd_stop_apps
  $DOCKER stop "$PG_NAME" "$RESTATE_NAME" >/dev/null 2>&1 || true
  log "containers + apps stopped (use './dev.sh clean' to remove containers + wipe data)"
}

cmd_clean() {
  cmd_down
  $DOCKER volume rm "$PG_VOLUME" "$RESTATE_VOLUME" >/dev/null 2>&1 || true
  rm -rf "$LOGDIR" "$BACKEND/bin"
  log "cleaned: containers, volumes, logs, binaries"
}

cmd_services() {
  if curl -s "http://localhost:$RESTATE_META/services"; then echo; else log "restate not reachable on :$RESTATE_META"; fi
}

cmd_status() {
  log "=== docker containers ==="
  $DOCKER ps --filter "name=$PG_NAME" --filter "name=$RESTATE_NAME" \
    --format '  {{.Names}}\t{{.Status}}\t{{.Ports}}' 2>/dev/null || log "  (docker unavailable)"
  log "=== app processes ==="
  for f in api worker ui; do
    if [ -f "$LOGDIR/$f.pid" ]; then
      pid="$(cat "$LOGDIR/$f.pid")"
      if kill -0 "$pid" 2>/dev/null; then log "  $f: running (pid $pid)"; else log "  $f: stale pidfile ($pid)"; fi
    else log "  $f: not started"; fi
  done
  log "=== ports ==="
  for p in "$PG_PORT" "$RESTATE_META" "$RESTATE_INGRESS" "$API_PORT" "$WORKER_PORT" "$UI_PORT"; do
    if port_open "$p"; then log "  :$p LISTEN"; else log "  :$p free"; fi
  done
}

cmd_logs() { # <name>
  local f="$1"
  case "$f" in
    pg)      $DOCKER logs --tail 50 -f "$PG_NAME" ;;
    restate) $DOCKER logs --tail 50 -f "$RESTATE_NAME" ;;
    *)       tail -n 50 -f "$LOGDIR/$f.log" ;;
  esac
}

cmd_tunnel() {
  # Runs an HTTPS reverse tunnel (pinggy, free + no login) in the foreground and
  # auto-rewrites WEBHOOK_PUBLIC_URL in validator-backend/.env with the printed
  # URL. Free pinggy sessions rotate every ~60 min; just rerun on rotation.
  local port="${API_PORT:-8000}" captured=0 host url
  [ -f "$BACKEND/.env" ] || { log "missing $BACKEND/.env (run './dev.sh up' first)"; return 1; }
  log "starting pinggy HTTPS tunnel -> localhost:$port  (Ctrl-C to stop)"
  log "free tier rotates ~hourly; rerun this command to get a fresh URL."
  log "the API must be listening on :$port for callbacks to actually land."
  log ""
  ssh -T -p 443 -o StrictHostKeyChecking=accept-new -o ServerAliveInterval=30 \
      -R "0:localhost:$port" a.pinggy.io 2>&1 \
    | while IFS= read -r line; do
        printf '%s\n' "$line"
        # pinggy prints each tunnel URL on its own line starting with https://.
        # Skip the dashboard URL embedded mid-banner.
        if [ "$captured" = "0" ] && [[ $line =~ ^https://([a-zA-Z0-9._-]+) ]]; then
          host="${BASH_REMATCH[1]}"
          [ "$host" = "dashboard.pinggy.io" ] && continue
          url="${BASH_REMATCH[0]}"
          captured=1
          sed -i -E "s|^WEBHOOK_PUBLIC_URL=.*|WEBHOOK_PUBLIC_URL=$url|" "$BACKEND/.env"
          printf '\n  >> WEBHOOK_PUBLIC_URL=%s\n  >> written to validator-backend/.env\n  >> restart the worker for new scouts to use it: ./dev.sh restart\n\n' "$url"
        fi
      done || true
  log "tunnel closed. rerun './dev.sh tunnel' for a fresh URL (it updates .env again)."
}

cmd_help() {
  cat <<'EOF'
Validator local dev.

Usage: ./dev.sh <command>

Stack:
  up              Start Postgres + Restate (docker), API, Worker, register, and UI
  down            Stop containers + app processes (keeps data volumes)
  restart         down && up
  clean           down + wipe data volumes, build artifacts, logs
  status          Show containers, app pids, listening ports
  infra           Ensure Postgres + Restate containers are running
  db              Start the Postgres container
  restate         Start the Restate runtime container
  register        Register the worker deployment with Restate
  services        List services registered in the Restate runtime
  env-sync        Rewrite DATABASE_URL in validator-backend/.env -> containerized Postgres
  tunnel          Start a free HTTPS tunnel (pinggy) and auto-sync WEBHOOK_PUBLIC_URL
  build           Build cmd/api and cmd/worker into validator-backend/bin
  api             Run the API in the foreground
  worker          Run the Worker in the foreground
  ui              Run the Vite dev server in the foreground
  logs-api        Tail the API log
  logs-worker     Tail the Worker log
  logs-ui         Tail the UI log
  logs-pg         Tail the Postgres container log
  logs-restate    Tail the Restate container log

Overridable env vars: GO SUDO DOCKER PG_PORT PG_USER PG_PASS PG_DB PG_IMAGE
  RESTATE_IMAGE RESTATE_INGRESS RESTATE_META API_PORT WORKER_PORT WORKER_HOST UI_PORT
EOF
}

case "${1:-help}" in
  up|down|restart|clean|status|infra|db|restate|register|services|env-sync|build|tunnel|help) "cmd_${1/-/_}" ;;
  api|worker|ui)
    case "$1" in api) cmd_build; ( cd "$BACKEND" && exec ./bin/api ) ;;
      worker) cmd_build; ( cd "$BACKEND" && exec ./bin/worker ) ;;
      ui) ( cd "$UI" && exec npm run dev ) ;; esac ;;
  start-api)   cmd_build; start_app api api "$API_PORT" ;;
  start-worker) cmd_build; start_app worker worker "$WORKER_PORT" ;;
  start-ui)    mkdir -p "$LOGDIR"; ( cd "$UI" && nohup npm run dev > "$LOGDIR/ui.log" 2>&1 & echo $! > "$LOGDIR/ui.pid" ); wait_port "$UI_PORT" ui ;;
  logs-api|logs-worker|logs-ui|logs-pg|logs-restate) cmd_logs "${1#logs-}" ;;
  stop-apps) cmd_stop_apps ;;
  *) log "unknown command: $1"; log "run './dev.sh help' for usage"; exit 1 ;;
esac
