# ─── Stage 1: Dependencies ───────────────────────────────────────────────
FROM oven/bun:1 AS deps
WORKDIR /app

# Install dependencies first (separate layer for caching)
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --production

# ─── Stage 2: Runtime ────────────────────────────────────────────────────
FROM oven/bun:1-slim
WORKDIR /app

# wget for Docker healthcheck
USER root
RUN apt-get update && apt-get install -y --no-install-recommends wget ca-certificates && rm -rf /var/lib/apt/lists/*
USER bun

# Copy only what's needed for production
COPY --from=deps /app/node_modules ./node_modules
COPY src/ ./src/
COPY package.json tsconfig.json ./

# Run as non-root user (built into oven/bun images)
USER bun

EXPOSE 7433

# Start the MCP server (PORT and TOMOSHIBI_API_URL set via docker-compose env)
ENTRYPOINT ["bun", "run", "src/index.ts"]
