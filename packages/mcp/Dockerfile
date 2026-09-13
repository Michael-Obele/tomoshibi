# ─── Stage 1: Dependencies ───────────────────────────────────────────────
FROM oven/bun:1 AS deps
WORKDIR /app

# Install dependencies first (separate layer for caching)
# Copy scripts first so postinstall can patch tmcp transports
COPY scripts/ ./scripts/
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --production

# ─── Stage 2: Runtime ────────────────────────────────────────────────────
FROM oven/bun:1-slim
WORKDIR /app

# Copy only what's needed for production
COPY --from=deps /app/node_modules ./node_modules
COPY src/ ./src/
COPY package.json tsconfig.json ./

# Run as non-root user (built into oven/bun images)
USER bun

# Start the MCP server (PORT and CINDER_API_URL set via docker-compose env)
ENTRYPOINT ["bun", "run", "src/index.ts"]
