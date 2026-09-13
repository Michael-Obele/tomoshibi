# ─── Stage 1: Build ────────────────────────────────────────────────────
FROM oven/bun:1 AS builder
WORKDIR /app

COPY package.json ./
COPY src ./src
COPY static ./static
COPY svelte.config.js vite.config.ts tsconfig.json components.json ./
RUN bun install

# PRIVATE_CINDER_* required at build time for $env/static/private
ARG PRIVATE_CINDER_BACKEND_URL=http://api:7431
ARG PRIVATE_CINDER_API_KEY=dummy
ENV PRIVATE_CINDER_BACKEND_URL=$PRIVATE_CINDER_BACKEND_URL
ENV PRIVATE_CINDER_API_KEY=$PRIVATE_CINDER_API_KEY
RUN DOCKER_BUILD=1 bun run build

# ─── Stage 2: Runtime (Bun) ────────────────────────────────────────────
FROM oven/bun:1-slim
WORKDIR /app

# curl for healthcheck (wget not in slim)
USER root
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && rm -rf /var/lib/apt/lists/*
USER bun

# Only the built output + production deps
COPY --from=builder /app/build ./build
COPY --from=builder /app/package.json ./package.json
COPY --from=builder /app/node_modules ./node_modules

ENV NODE_ENV=production
ENV PORT=7432
EXPOSE 7432

# SvelteKit adapter-node output is a Node-compatible server — Bun runs it natively
CMD ["bun", "build/index.js"]
