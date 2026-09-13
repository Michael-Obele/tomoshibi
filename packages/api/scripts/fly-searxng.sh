#!/usr/bin/env bash
# fly-searxng.sh — manage the SearXNG sidecar machine inside the SAME Fly app
# as Cinder (Option A). Both machines auto-stop when idle (no public traffic)
# and auto-start on first request. This script also supports manual start/stop
# of BOTH machines together.
#
# Usage:
#   ./scripts/fly-searxng.sh deploy    # build → push → create/update machine → secret
#   ./scripts/fly-searxng.sh status    # show both machines + secret
#   ./scripts/fly-searxng.sh start     # start BOTH Cinder + SearXNG machines
#   ./scripts/fly-searxng.sh stop      # stop BOTH machines (winds everything down)
#   ./scripts/fly-searxng.sh destroy   # remove the searxng machine (keeps secret)
#
# Auto-stop behavior:
#   Both machines have services with auto_stop='stop' and auto_start=true.
#   When no public traffic hits either machine, the Fly proxy stops them.
#   When someone hits the public URL, Cinder auto-starts.
#   First search after idle falls back to Brave (SEARXNG_ENDPOINT secret is
#   set but the sidecar is stopped). Hit the SearXNG public URL or run
#   `start` to wake it.
#
# Env overrides:
#   FLY_APP      (default: cinder9630)
#   FLY_REGION   (default: lhr)
#   FLY_IMAGE    (default: registry.fly.io/cinder9630:searxng)
#   SEARXNG_PORT (default: 8080)
set -euo pipefail

APP="${FLY_APP:-cinder9630}"
REGION="${FLY_REGION:-lhr}"
IMAGE="${FLY_IMAGE:-registry.fly.io/${APP}:searxng}"
PORT="${SEARXNG_PORT:-8080}"
ENDPOINT="http://searxng.internal:${PORT}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

require_fly() { command -v fly >/dev/null || { echo "error: fly CLI not found" >&2; exit 1; }; }
require_docker() { command -v docker >/dev/null || { echo "error: docker not found" >&2; exit 1; }; }

# ── helpers ──────────────────────────────────────────────────────────────────

machine_id() {
  local name="$1"
  fly machine list --app "$APP" --json 2>/dev/null \
    | python3 -c "import json,sys; ids=[m['id'] for m in json.load(sys.stdin) if m.get('name')=='$name']; print(ids[0] if ids else '')"
}

machine_state() {
  local name="$1"
  fly machine list --app "$APP" --json 2>/dev/null \
    | python3 -c "import json,sys; ms=[m for m in json.load(sys.stdin) if m.get('name')=='$name']; print(ms[0]['state'] if ms else 'missing')"
}

fly_docker_auth() {
  local cfg="${DOCKER_FLY_CONFIG:-/tmp/docker-fly-config}"
  rm -rf "$cfg" && cp -r ~/.docker "$cfg"
  python3 - "$cfg" <<'PY'
import json, sys
p = sys.argv[1] + '/config.json'
d = json.load(open(p))
d.pop('credsStore', None)
d['auths'] = {}
json.dump(d, open(p, 'w'))
PY
  DOCKER_CONFIG="$cfg" fly auth docker
}

# ── commands ─────────────────────────────────────────────────────────────────

cmd_deploy() {
  require_docker
  echo "==> Building sidecar image (JSON API enabled)..."
  ( cd "$REPO_ROOT/deploy" && docker build -f searxng-fly/Dockerfile -t "$IMAGE" . )
  echo "==> Pushing to Fly registry..."
  fly_docker_auth
  DOCKER_CONFIG="${DOCKER_FLY_CONFIG:-/tmp/docker-fly-config}" docker push "$IMAGE"

  local mid
  mid=$(machine_id searxng)
  if [ -n "$mid" ]; then
    echo "==> searxng machine exists ($mid) — updating image..."
    fly machine update "$mid" --app "$APP" --image "$IMAGE" -y 2>&1 | grep -v WARN | grep -v 'not valid' | tail -3
  else
    echo "==> Creating searxng machine in '$APP' (lhr, 256MB, auto_stop)..."
    fly machine run "$IMAGE" \
      --app "$APP" \
      --name searxng \
      --memory 256 \
      --region "$REGION" \
      --port "$PORT/tcp" \
      --autostop stop \
      --autostart \
      2>&1 | grep -v WARN | grep -v 'not valid' | tail -5
    mid=$(machine_id searxng)
  fi

  # Set process group via Machines API. No fly_platform_version = detached.
  local tok
  tok=$(fly auth token 2>/dev/null)
  cat > /tmp/searxng-machine.json <<EOF
{
  "config": {
    "init": {},
    "guest": {"cpu_kind": "shared", "cpus": 1, "memory_mb": 256},
    "image": "$IMAGE",
    "restart": {"policy": "on-failure", "max_retries": 10},
    "dns": {},
    "metadata": {"fly_process_group": "searxng"},
    "services": [{"ports": [{"port": $PORT, "handlers": ["tls", "http"]}], "protocol": "tcp", "internal_port": $PORT, "autostop": "stop", "autostart": true}]
  }
}
EOF
  curl -s -X POST -H "Authorization: Bearer $tok" -H "Content-Type: application/json" \
    -d @/tmp/searxng-machine.json \
    "https://api.machines.dev/v1/apps/$APP/machines/$mid" >/dev/null
  echo "==> Process group 'searxng' set (detached from fly deploy)."

  echo "==> Pointing Cinder at the sidecar ($ENDPOINT)..."
  fly secrets set --app "$APP" "SEARXNG_ENDPOINT=$ENDPOINT" 2>&1 | grep -v WARN | grep -v 'not valid' | tail -2
  echo "==> Done. Both machines auto-stop when idle, auto-start on first request."
}

cmd_status() {
  echo "==> Machines in $APP:"
  fly machine list --app "$APP" --json 2>/dev/null \
    | python3 -c '
import json,sys
for m in json.load(sys.stdin):
    cfg=m.get("config",{})
    meta=cfg.get("metadata",{})
    svcs=cfg.get("services",[])
    pg=meta.get("fly_process_group","?")
    plat=meta.get("fly_platform_version") or "detached"
    as_=svcs[0].get("autostop") if svcs else "no-svc"
    mem=cfg.get("guest",{}).get("memory_mb","?")
    n=m["name"]; s=m["state"]; r=m["region"]
    print(f"  {n:<25} state={s:<10} region={r:<5} pg={pg:<10} {mem}MB  autostop={as_}  platform={plat}")
'
  echo "==> SEARXNG_ENDPOINT secret:"
  fly secrets list --app "$APP" 2>/dev/null | grep -E 'NAME|SEARXNG' | sed 's/^/  /' || echo "  (not set)"
}

cmd_start() {
  echo "==> Starting both machines..."
  fly machine list --app "$APP" --json 2>/dev/null \
    | python3 -c '
import json,sys
for m in json.load(sys.stdin):
    print(m["name"], m["state"])
' | while read -r name state; do
    if [ "$state" = "started" ]; then
      echo "  $name: already started"
    else
      echo "  $name: starting..."
      fly machine start "$(machine_id "$name")" --app "$APP" 2>&1 | grep -v WARN | grep -v 'not valid' | tail -1
    fi
  done
  echo "==> Both machines started."
}

cmd_stop() {
  echo "==> Stopping both machines..."
  fly machine list --app "$APP" --json 2>/dev/null \
    | python3 -c '
import json,sys
for m in json.load(sys.stdin):
    print(m["name"], m["state"])
' | while read -r name state; do
    if [ "$state" = "stopped" ]; then
      echo "  $name: already stopped"
    else
      echo "  $name: stopping..."
      fly machine stop "$(machine_id "$name")" --app "$APP" 2>&1 | grep -v WARN | grep -v 'not valid' | tail -1
    fi
  done
  echo "==> Both machines stopped. Cost: \$0 until next request."
}

cmd_destroy() {
  local mid
  mid=$(machine_id searxng)
  if [ -z "$mid" ]; then
    echo "==> No searxng machine to destroy."
    return
  fi
  echo "==> Destroying searxng machine $mid..."
  fly machine destroy "$mid" --app "$APP" --force 2>&1 | grep -v WARN | grep -v 'not valid' | tail -2
}

# ── main ─────────────────────────────────────────────────────────────────────

require_fly
case "${1:-status}" in
  deploy)  cmd_deploy ;;
  status)  cmd_status ;;
  start)   cmd_start ;;
  stop)    cmd_stop ;;
  destroy) cmd_destroy ;;
  *) echo "usage: $0 [deploy|status|start|stop|destroy]" >&2; exit 1 ;;
esac