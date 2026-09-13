import { McpServer } from "tmcp";
import { ValibotJsonSchemaAdapter } from "@tmcp/adapter-valibot";
import { getConfig } from "./config.js";

class FixedValibotAdapter extends ValibotJsonSchemaAdapter {
  async toJsonSchema(schema: any) {
    const s: any = await super.toJsonSchema(schema);
    if (!s.type && (s.oneOf || s.anyOf)) s.type = "object";
    return s;
  }
}
import { CinderClient } from "./client.js";
import { ExtractSchema, createExtractHandler } from "./tools/extract.js";
import { DiscoverSchema, createDiscoverHandler } from "./tools/discover.js";
import { MonitorSchema, createMonitorHandler } from "./tools/monitor.js";

/**
 * Create and configure the McpServer instance.
 * Registers all Cinder MCP tools with Valibot-validated schemas.
 */
export function createServer(): McpServer {
  const config = getConfig();
  const client = new CinderClient();

  const server = new McpServer(
    {
      name: config.MCP_SERVER_NAME,
      version: config.MCP_SERVER_VERSION,
      description:
        "Cinder MCP — web scraping, crawling, and search powered by Cinder API",
    },
    {
      adapter: new FixedValibotAdapter(),
      capabilities: {
        tools: { listChanged: false },
      },
      instructions: [
        'Cinder MCP — 3 resource-oriented tools (≤7 philosophy, arch.md "Why 7 Tools Instead of 17"): 1 tool per domain resource with `action` enum, not 1 tool per CRUD operation.',
        "",
        "## Tools (3 — resource-oriented multiplexing)",
        "- `cinder_extract` — extraction resource: `scrape` (single page → markdown) | `scrape_multi` (sync multi-URL, max 10, no Redis) | `links` (hyperlinks only) | `batch` (enqueue 20 URLs, Redis) | `batch_status` (poll batch)",
        "- `cinder_discover` — discovery resource: `search` (SearXNG/Brave) | `map` (sitemap/traversal) | `crawl` (enqueue BFS, Redis) | `crawl_status` (poll crawl)",
        "- `cinder_monitor` — change-tracking resource: `create` | `status` | `delete` (all Redis)",
        "",
        "## Tips",
        "- Use `cinder_discover` (search/map) first to find URLs, then `cinder_extract` (scrape/scrape_multi) to fetch them.",
        "- Use `cinder_extract` action=links for lightweight hyperlink extraction (no markdown).",
        "- Use `cinder_extract` action=scrape_multi for 2–10 URLs in one call (no Redis, mirrors web_fetch_exa).",
        "- Async actions (crawl/crawl_status, batch/batch_status, monitor) require Redis-backed Cinder — poll their status actions until done.",
      ].join("\n"),
    },
  );

  // Resource: extraction — 1 tool per domain resource with `action` enum (arch.md "Why 7 Instead of 17")
  // Consolidates former cinder_scrape + cinder_links + cinder_batch_scrape (3→1) + scrape_multi (sync multi-URL)
  server.tool(
    {
      name: "cinder_extract",
      description:
        "Extraction resource (5 actions): `scrape` (single page → markdown, screenshots/images/summary/schema), `scrape_multi` (sync multi-URL max 10, no Redis, mirrors web_fetch_exa), `links` (hyperlinks only, no markdown), `batch` (enqueue 20 URLs async, Redis), `batch_status` (poll batch). Replaces cinder_scrape/cinder_links/cinder_batch_scrape.",
      schema: ExtractSchema,
      annotations: {
        readOnlyHint: false,
        openWorldHint: true,
        idempotentHint: false,
        destructiveHint: false,
      },
    },
    createExtractHandler(client) as any,
  );

  // Resource: discovery — 1 tool per domain resource with `action` enum
  // Consolidates former cinder_search + cinder_map + cinder_crawl (3→1, crawl already merged start/status)
  server.tool(
    {
      name: "cinder_discover",
      description:
        "Discovery resource (4 actions): `search` (SearXNG/Brave, domain filters/pagination), `map` (sitemap/traversal), `crawl` (enqueue BFS async, Redis), `crawl_status` (poll crawl). Replaces cinder_search/cinder_map/cinder_crawl.",
      schema: DiscoverSchema,
      annotations: {
        readOnlyHint: false,
        openWorldHint: true,
        idempotentHint: false,
        destructiveHint: false,
      },
    },
    createDiscoverHandler(client) as any,
  );

  // Resource: change-tracking — already action-multiplexed (create|status|delete)
  server.tool(
    {
      name: "cinder_monitor",
      description:
        "Change-tracking resource (3 actions): `create` (hashes markdown, fires signed webhook on change), `status` (config/last hash/next check), `delete` (stop & remove). Requires Redis.",
      schema: MonitorSchema,
      annotations: {
        readOnlyHint: false,
        openWorldHint: true,
        idempotentHint: false,
        destructiveHint: false,
      },
    },
    createMonitorHandler(client) as any,
  );

  return server;
}
