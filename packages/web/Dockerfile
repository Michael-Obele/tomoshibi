# ─── Stage 1: Build ────────────────────────────────────────────────────
# Build from the REPO ROOT: the docs pipeline reads docs.sources.json and the
# tracked markdown, which live outside packages/web.
#
#   docker build -f packages/web/Dockerfile .
FROM oven/bun:1 AS builder
WORKDIR /app

# .dockerignore keeps node_modules, .git and .env out of the context.
# packages/web/Dockerfile.dockerignore keeps the docs the build reads.
COPY . .
RUN bun install --frozen-lockfile

# No backend URL is baked in: the app reads PRIVATE_TOMOSHI_* from
# $env/dynamic/private at runtime, so the same image works against any API.
RUN cd packages/web && bun run build

# ─── Stage 2: Runtime (Bun) ────────────────────────────────────────────
FROM oven/bun:1-slim
WORKDIR /app

# Default for a bare `docker run`; compose and -e override it.
ENV PRIVATE_TOMOSHI_BACKEND_URL=http://api:7431
ENV PRIVATE_TOMOSHI_API_KEY=

# curl for healthcheck (wget not in slim)
USER root
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && rm -rf /var/lib/apt/lists/*
USER bun

# Only the built output + production deps. Dependencies resolve upwards from
# packages/web/node_modules to the workspace root.
COPY --from=builder /app/packages/web/build ./build
COPY --from=builder /app/packages/web/package.json ./package.json
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/packages/web/node_modules ./packages/web/node_modules

ENV NODE_ENV=production
ENV PORT=7432
EXPOSE 7432

# SvelteKit adapter-node output is a Node-compatible server — Bun runs it natively
CMD ["bun", "build/index.js"]
