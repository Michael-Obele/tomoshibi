# Deploy SearXNG on Fly.io for Tomoshibi Search

> [!TIP]
> Tomoshibi's `/v1/search` uses a self-hosted SearXNG when `SEARXNG_ENDPOINT` is
> set, falling back to the `BRAVE_SEARCH_API_KEY` secret otherwise. SearXNG
> aggregates Google, Bing, DuckDuckGo, Brave, Mojeek and Wikipedia — free,
> no per-query cost, and no single fragile engine to scrape.

SearXNG is _not_ bundled into the Tomoshibi image (the Tomoshibi Dockerfile only
builds `tomoshibi-api` and `tomoshibi-worker`), but there are two ways to run it
on Fly with one app to manage:

- **Option A — same app, separate machine** (sidecar): add a second Fly
  machine running SearXNG _inside_ the same app as Tomoshibi. One app, one
  dashboard entry, private-network access for free.
- **Option B — separate app**: SearXNG as its own Fly app. Cleanest
  isolation, but a second app to manage.

Both reach SearXNG over Fly's private network (`*.internal`), which is free.

## Option A — Same app, separate machine (sidecar)

The repo already ships the pieces: `deploy/searxng-fly/Dockerfile` bakes the
JSON-enabled `deploy/searxng/settings.yml` into the official image.

> [!TIP]
> There is an idempotent helper for all of this — `scripts/fly-searxng.sh`
> (`deploy` / `status` / `start` / `stop` / `destroy`). It builds, pushes,
> creates or updates the machine, sets the secret, and can start/stop BOTH
> machines together. The manual steps below are what it runs.

### Auto-stop behavior

Both machines have services with `auto_stop='stop'` and `auto_start=true`.
When no public traffic hits either machine, the Fly proxy stops them — cost
drops to $0. When someone hits the public URL, Tomoshibi auto-starts. The
first search after idle falls back to Brave (the `SEARXNG_ENDPOINT` secret
is set but the sidecar is still stopped). Hit the SearXNG public URL or
run `scripts/fly-searxng.sh start` to wake it.

To manually wind down everything: `scripts/fly-searxng.sh stop` — both
machines stop, cost goes to $0 until next request.

### 1. Build and push the sidecar image

> [!NOTE]
> Fly's registry only accepts repositories named after an **app**, so the
> sidecar image is tagged under the app's own repo as `:searxng` (not a new
> repo name). `fly auth docker` also needs a scratch `DOCKER_CONFIG` on some
> machines (its `pass` credential store fails uninitialized) — the helper
> script handles both.

```bash
cd deploy
docker build -f searxng-fly/Dockerfile -t registry.fly.io/<your-app>:searxng .
# fly auth docker (use the helper's scratch DOCKER_CONFIG if pass is broken)
docker push registry.fly.io/<your-app>:searxng
```

### 2. Create the SearXNG machine in the SAME app

```bash
fly machine run registry.fly.io/<your-app>:searxng \
  --app <your-app> \
  --name searxng \
  --memory 256 \
  --region lhr \
  --port 8080/tcp \
  --autostop stop \
  --autostart
```

Then set its process group. This flyctl version has no `--process-group` flag
on `machine run`/`update` — the group lives in
`config.metadata["fly_process_group"]`, set via the Machines API (the helper
script does this):

```bash
fly machine update searxng --app <your-app> \
  --machine-config <(echo '{"config":{"metadata":{"fly_process_group":"searxng"}}}') -y
```

Notes:

- **Public port with auto_stop** — SearXNG has a public service on port 8080
  so the Fly proxy can auto-stop it when no traffic hits. The URL is random
  and not easily discoverable. Internal traffic from Tomoshibi (via
  `searxng.internal`) bypasses the proxy, so it won't keep SearXNG alive —
  which is what we want (both machines stop when idle).
- **Process group `searxng`** keeps it out of the app's default group.
  Critically, the machine must **not** carry `fly_platform_version: v2`
  metadata — that would mark it Fly-Launch-managed and `fly deploy` would
  destroy it (its group is not in `fly.toml`). Without that key it is a
  *detached* machine that `fly deploy` leaves alone.
- `--memory 256` is the minimum valid increment on Fly (125MB is not allowed).
  SearXNG uses ~150 MB baseline; 256 MB gives it headroom.
- Both machines auto-stop when idle (cost drops to $0) and auto-start on first
  request. First search after idle falls back to Brave. Use
  `scripts/fly-searxng.sh start` / `stop` to manually control both together.

### 3. Point Tomoshibi at it

```bash
fly secrets set --app <your-app> SEARXNG_ENDPOINT=http://searxng.internal:8080
```

(`searxng.internal` is the machine's private-network name; `8080` is the
port SearXNG listens on in the container.)

### 4. Verify

```bash
# From inside the Tomoshibi machine, SearXNG must answer on the private network:
fly ssh console --app <your-app> -C "wget -qO- http://searxng.internal:8080/search?q=golang&format=json | head -c 120"

# Through Tomoshibi (replace <your-app>.fly.dev with your actual URL):
curl -s -X POST https://<your-app>.fly.dev/v1/search \
  -H 'Content-Type: application/json' -H 'X-API-Key: <your-api-key>' \
  -d '{"query":"golang concurrency","limit":3}'
```

---

## Option B — Separate app

### Step 1 — Create the SearXNG app

SearXNG's official image is `searxng/searxng`. We bake a `settings.yml` into
it so the JSON API (which Tomoshibi queries) is enabled — the stock image has
JSON disabled.

```bash
mkdir searxng-fly && cd searxng-fly
```

**`Dockerfile`**

```dockerfile
FROM searxng/searxng:latest
COPY settings.yml /etc/searxng/settings.yml
```

**`settings.yml`** — replace the secret key with a random 64-char string
(`openssl rand -hex 32`):

```yaml
use_default_settings: true

server:
  # Generate with: openssl rand -hex 32
  secret_key: "REPLACE_WITH_64_HEX_CHARS"
  limiter: false
  public_instance: false

search:
  formats:
    - html
    - json # <-- required: Tomoshibi queries the JSON API
```

## Step 2 — Scaffold and deploy

```bash
# Scaffold the Fly app (generates fly.toml, no deploy yet)
fly launch --name searxng --no-deploy

# SearXNG listens on 8080 inside the container — make sure fly.toml has:
#   [http_service]
#     internal_port = 8080
# (fly launch usually detects it; verify before deploying)

fly deploy
```

Give the machine a moment to boot, then check the JSON API responds:

```bash
curl -s "https://<your-searxng-app>.fly.dev/search?q=golang&format=json" | head -c 200
```

## Step 3 — Point Tomoshibi at it

Set the endpoint as a **secret** (stays out of the repo — cleaner than the
commented `SEARXNG_ENDPOINT` in `fly.toml`):

```bash
cd ~/Documents/GitHub/tomoshibi

# Private network address — free, not exposed publicly
fly secrets set SEARXNG_ENDPOINT=http://searxng.internal

# ...or, if you want it reachable publicly:
# fly secrets set SEARXNG_ENDPOINT=https://<your-searxng-app>.fly.dev
```

> [!NOTE]
> `fly secrets set` triggers a redeploy of the Tomoshibi app. If you prefer
> config-in-repo, uncomment `SEARXNG_ENDPOINT` in `fly.toml` instead — but
> secrets keep the URL out of version control.

## Step 4 — Verify end to end

```bash
# SearXNG direct (JSON API)
curl -s "https://<your-searxng-app>.fly.dev/search?q=fly.io+deployment&format=json" | \
  python3 -c "import json,sys; print(len(json.load(sys.stdin)['results']), 'results')"

# Through Tomoshibi (replace <your-app>.fly.dev with your actual URL)
curl -s -X POST https://<your-app>.fly.dev/v1/search \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: <your-api-key>' \
  -d '{"query":"fly.io deployment","limit":3}'
```

If search still falls back to Brave, confirm the secret landed:
`fly secrets list | grep SEARXNG`.

## Local equivalent (docker-compose)

The Tomoshibi repo's `docker-compose.yml` already runs SearXNG as a sibling
service (port `8889` host → `8080` container) with the JSON API enabled via
`deploy/searxng/settings.yml`. Local `.env` points `SEARXNG_ENDPOINT` at
`http://localhost:8889`:

```bash
docker compose up -d searxng     # start just the search backend
go run ./cmd/api                 # local Tomoshibi now searches via SearXNG
```

## Notes & troubleshooting

- **Empty results**: SearXNG occasionally returns zero results when all its
  upstream engines hiccup at once (visible in its logs as engine timeouts /
  DDG CAPTCHAs). Tomoshibi does **not** cache empty result sets, so the next
  request re-queries and recovers. If you want a hard fallback for these
  windows, also set `BRAVE_SEARCH_API_KEY` — Tomoshibi tries SearXNG first,
  then Brave.
- **Memory**: SearXNG is light (~150 MB). Fly's default shared-1x-256MB VM
  is enough; bump to 512MB if you enable many engines.
- **Rate limiting**: Tomoshibi deliberately does **not** rate-limit SearXNG
  client-side — SearXNG handles concurrency internally (its own limiter is
  disabled in the settings above, which is fine for a private instance).
- **Private networking**: `http://searxng.internal` only resolves between
  apps in the same Fly organization. It is the recommended address — no
  public exposure, no egress cost.
