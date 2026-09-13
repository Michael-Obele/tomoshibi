#!/usr/bin/env bun

import { serve } from "srvx";
import { StdioTransport } from "@tmcp/transport-stdio";
import { HttpTransport } from "@tmcp/transport-http";
import { SseTransport } from "@tmcp/transport-sse";
import { createServer } from "./server.js";
import { getConfig } from "./config.js";
import { oauth } from "./auth-provider.js";

// ---------------------------------------------------------------------------
// Initialize
// ---------------------------------------------------------------------------

const config = getConfig();
const server = createServer();

// ---------------------------------------------------------------------------
// HTTP + SSE Transport (for remote / production)
// ---------------------------------------------------------------------------

const http_transport = new HttpTransport(server, {
  oauth: config.OAUTH_ENABLED ? oauth : undefined,
});

const sse_transport = new SseTransport(server, {
  oauth: config.OAUTH_ENABLED ? oauth : undefined,
});

serve({
  async fetch(request) {
    // Bun kills idle connections after 10s by default (idleTimeout). MCP's
    // Streamable HTTP (GET /mcp) and legacy SSE (GET /sse) are long-lived
    // streams that sit idle between server→client notifications for minutes.
    // Without this, Bun aborts the stream and tmcp's controller.enqueue()
    // throws "Controller is already closed" → process crash → Docker restart loop.
    // Mirrors effing-use/src/http.ts fix (idleTimeout: 0 + server.timeout(req,0)).
    try {
      const runtime = (request as unknown as { runtime?: { bun?: { server?: { timeout: (req: Request, secs: number) => void } } } }).runtime;
      const bunServer = runtime?.bun?.server;
      if (bunServer) {
        const { pathname } = new URL(request.url);
        if (pathname === "/mcp" || pathname === "/sse" || pathname === "/message") {
          bunServer.timeout(request, 0);
        }
      }
    } catch {
      // ignore — srvx may not expose runtime in all environments (e.g. tests)
    }

    // Compatibility: opencode's MCP client sends initialize without protocolVersion
    // (causes tmcp validation: Expected "protocolVersion" but received undefined).
    // Inject a default if missing so the handshake succeeds — other clients (VS Code, curl)
    // already send it and are unchanged.
    if (request.method === "POST") {
      try {
        const url = new URL(request.url);
        if (url.pathname === "/mcp") {
          const ct = request.headers.get("content-type") || "";
          if (ct.includes("application/json")) {
            const cloned = request.clone();
            const body: any = await cloned.json();
            const patch = (obj: any) => {
              if (obj && obj.method === "initialize" && obj.params && !obj.params.protocolVersion) {
                obj.params.protocolVersion = "2024-11-05";
                return true;
              }
              return false;
            };
            let patched = false;
            if (Array.isArray(body)) {
              for (const item of body) patched = patch(item) || patched;
            } else {
              patched = patch(body);
            }
            if (patched) {
              request = new Request(request.url, {
                method: request.method,
                headers: request.headers,
                body: JSON.stringify(body),
              });
            }
          }
        }
      } catch {
        // ignore parse errors, fall through to normal handling
      }
    }

    // Try HTTP transport (Streamable HTTP for MCP)
    const http_response = await http_transport.respond(request);
    if (http_response) {
      return http_response;
    }

    // Try SSE transport (legacy fallback)
    const sse_response = await sse_transport.respond(request);
    if (sse_response) {
      return sse_response;
    }

    // Root health check
    const url = new URL(request.url);
    if (url.pathname === "/" || url.pathname === "/health") {
      return new Response(
        JSON.stringify({
          service: "cinder-mcp",
          status: "ok",
          version: config.MCP_SERVER_VERSION,
          endpoints: {
            mcp: "/mcp",
            sse: "/sse",
            health: "/health",
          },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );
    }

    return new Response(null, { status: 404 });
  },
  port: Number(config.PORT),
  // Bun's default idleTimeout (10s) kills *any* idle connection, including
  // long-lived MCP SSE streams (GET /mcp, GET /sse) and slow SearXNG searches.
  // 60s was not enough — SSE sits idle for minutes between notifications.
  // idleTimeout: 0 disables the global idle kill (safe: this is an MCP-only
  // service). Per-request server.timeout(req,0) above is the belt; this is
  // the suspenders. Matches effing-use fix (verified 25s+ stable).
  bun: {
    idleTimeout: 0,
  },
});

// Graceful shutdown for Fly.io (sends SIGTERM on deploy/stop)
process.on("SIGTERM", () => {
  console.log("🛑 SIGTERM received, shutting down...");
  process.exit(0);
});

console.log(`🚀 Cinder MCP server running on port ${config.PORT}`);
console.log(`   Health: http://localhost:${config.PORT}/health`);
console.log(`   MCP:    http://localhost:${config.PORT}/mcp`);
console.log(`   SSE:    http://localhost:${config.PORT}/sse`);

// ---------------------------------------------------------------------------
// STDIO Transport (only when not in Fly.io / production)
// ---------------------------------------------------------------------------

const is_fly_io = process.env.FLY_APP_NAME !== undefined;
if (!is_fly_io) {
  const stdio_transport = new StdioTransport(server);
  stdio_transport.listen();
}
