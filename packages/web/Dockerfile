# ─── Stage 1: Build ────────────────────────────────────────────────────
FROM oven/bun:1 AS builder
WORKDIR /app

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .
RUN DOCKER_BUILD=1 bun run build

# ─── Stage 2: Runtime (Bun) ────────────────────────────────────────────
FROM oven/bun:1-slim
WORKDIR /app

# Only the built output + production deps
COPY --from=builder /app/build ./build
COPY --from=builder /app/package.json ./package.json
COPY --from=builder /app/node_modules ./node_modules

ENV NODE_ENV=production
ENV PORT=7432
EXPOSE 7432

# SvelteKit adapter-node output is a Node-compatible server — Bun runs it natively
CMD ["bun", "build/index.js"]
