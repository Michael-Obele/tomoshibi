import * as v from "valibot";
import type {
  TomoshiClient,
  SearchParams,
  MapParams,
  CrawlParams,
} from "../client.js";
import {
  flattenShapes,
  guardAction,
  normalize,
  num,
  toolError,
  type ActionSpec,
} from "./schema.js";

/**
 * Resource-oriented multiplexed discovery tool.
 * Consolidates `cinder_search` + `cinder_map` + `cinder_crawl`
 * into a single `tomoshi_discover` resource (3→1). Formerly Cinder.
 *
 * Principle: 1 tool per domain resource with `action` enum
 * (arch.md "Why 7 Tools Instead of 17" — FlarelyLegal 17→7).
 * Reduces token overhead, improves LLM tool-selection accuracy,
 * stays well under MCP client limits.
 *
 * Actions:
 * - `search`      — web search via SearXNG/Brave (POST /v1/search)
 * - `map`         — site URL discovery via sitemap/traversal (POST /v1/map)
 * - `crawl`       — enqueue async BFS crawl (POST /v1/crawl, Redis)
 * - `crawl_status`— poll crawl job (GET /v1/crawl/:id, Redis)
 */

// ── search ─────────────────────────────────────────────────────────────
const DiscoverSearchShape = v.object({
  action: v.pipe(
    v.picklist(["search"]),
    v.description("Web search via the native engine roster (SearXNG, Brave/Serper/Tavily fallbacks)"),
  ),
  query: v.pipe(
    v.string(),
    v.description("The search query"),
    v.minLength(1, "Search query cannot be empty"),
  ),
  offset: v.optional(
    v.pipe(
      v.number(),
      v.minValue(0),
      v.description("Pagination offset for results"),
    ),
    0,
  ),
  limit: v.optional(
    v.pipe(
      v.number(),
      v.minValue(1),
      v.maxValue(100),
      v.description("Number of results to return (1-100)"),
    ),
    10,
  ),
  mode: v.optional(
    v.pipe(
      v.picklist(["fast"]),
      v.description(
        "Search speed: 'fast' restricts to recent results (last day)",
      ),
    ),
  ),
  includeDomains: v.optional(
    v.pipe(
      v.array(v.string()),
      v.description("Restrict results to these domains"),
    ),
  ),
  excludeDomains: v.optional(
    v.pipe(
      v.array(v.string()),
      v.description("Exclude results from these domains"),
    ),
  ),
  requiredText: v.optional(
    v.pipe(
      v.array(v.string()),
      v.description("Only return results containing this text"),
    ),
  ),
  maxAge: v.optional(
    v.pipe(
      v.picklist([1, 7, 30]),
      v.description("Max age in days: 1 (day), 7 (week), 30 (month)"),
    ),
  ),
  category: v.optional(
    v.pipe(
      v.picklist(["general", "news", "code"]),
      v.description(
        "Category filter: general, news, code (maps to SearXNG categories)",
      ),
    ),
  ),
  rerank: v.optional(
    v.pipe(
      v.boolean(),
      v.description(
        "Lightweight TF-IDF re-rank (pure Go, no ONNX) — reorders by query-term score",
      ),
    ),
  ),
});

// ── map ────────────────────────────────────────────────────────────────
const DiscoverMapShape = v.object({
  action: v.pipe(
    v.picklist(["map"]),
    v.description("Discover URLs on a site via sitemap/link traversal"),
  ),
  url: v.pipe(
    v.string(),
    v.description(
      "The site URL to map (discover URLs via sitemap/link traversal)",
    ),
    v.url("Must be a valid URL"),
  ),
  search: v.optional(
    v.pipe(
      v.string(),
      v.description(
        "Only return URLs containing this substring (e.g. '/docs')",
      ),
    ),
  ),
  limit: v.optional(
    v.pipe(
      v.number(),
      v.minValue(1),
      v.maxValue(5000),
      v.description("Maximum URLs to return (1-5000, default 100)"),
    ),
    100,
  ),
});

// ── crawl (enqueue) ────────────────────────────────────────────────────
const DiscoverCrawlShape = v.object({
  action: v.pipe(
    v.picklist(["crawl"]),
    v.description("Enqueue async BFS crawl (requires Redis, returns task ID)"),
  ),
  url: v.pipe(
    v.string(),
    v.description("The root URL to start crawling from"),
    v.url("Must be a valid URL"),
  ),
  mode: v.optional(
    v.pipe(
      v.picklist(["smart", "static", "dynamic"]),
      v.description("Scraping mode per page: smart, static, or dynamic"),
    ),
    "smart",
  ),
  render: v.optional(
    v.pipe(
      v.boolean(),
      v.description("Render JavaScript for each page (headless browser)"),
    ),
    false,
  ),
  screenshot: v.optional(v.boolean(), false),
  images: v.optional(v.boolean(), false),
  image_format: v.optional(
    v.pipe(
      v.picklist(["url", "blob"]),
      v.description(
        "Image transport: 'url' (metadata only) or 'blob' (base64)",
      ),
    ),
    "url",
  ),
  maxDepth: v.optional(
    v.pipe(
      v.number(),
      v.minValue(1),
      v.maxValue(10),
      v.description("Maximum crawl depth (1-10)"),
    ),
    2,
  ),
  limit: v.optional(
    v.pipe(
      v.number(),
      v.minValue(1),
      v.maxValue(100),
      v.description("Maximum number of pages to crawl (1-100)"),
    ),
    10,
  ),
  include_paths: v.optional(
    v.pipe(
      v.array(v.string()),
      v.description(
        "Only follow links whose path matches these globs (e.g. ['/blog/*'])",
      ),
    ),
  ),
  exclude_paths: v.optional(
    v.pipe(
      v.array(v.string()),
      v.description(
        "Never follow links whose path matches these globs (exclusion wins)",
      ),
    ),
  ),
  webhook_url: v.optional(
    v.pipe(
      v.string(),
      v.description("POST the crawl result here on completion"),
    ),
  ),
  webhook_secret: v.optional(
    v.pipe(
      v.string(),
      v.description("HMAC-SHA256 key for the X-Tomoshibi-Signature header"),
    ),
  ),
});

// ── crawl_status (poll) ────────────────────────────────────────────────
const DiscoverCrawlStatusShape = v.object({
  action: v.pipe(
    v.picklist(["crawl_status"]),
    v.description("Poll async crawl job status"),
  ),
  id: v.pipe(
    v.string(),
    v.description("The task ID returned by tomoshi_discover action=crawl"),
    v.minLength(1, "Task ID is required"),
  ),
});

/**
 * Flat discovery schema: ONE object, with `action` forced into the required
 * list of the emitted JSON Schema. A root-level `v.variant` made
 * @valibot/to-json-schema emit `oneOf` with no top-level `properties`, which
 * MCP clients handed to models as an empty schema — so models called the tool
 * with `{}`.
 *
 * `action` is optional at runtime on purpose: tmcp validates arguments before
 * the handler runs and dumps raw Valibot issues, so `{}` is routed to
 * guardAction instead, which returns a readable usage error.
 */
export const DiscoverSchema = v.object(
  flattenShapes(
    [
      DiscoverSearchShape,
      DiscoverMapShape,
      DiscoverCrawlShape,
      DiscoverCrawlStatusShape,
    ],
    {
      action: v.optional(
        v.pipe(
          v.picklist(["search", "map", "crawl", "crawl_status"]),
          v.description(
            "Discovery action — required. search needs `query`; map needs `url`; crawl needs `url`; crawl_status needs `id` (returned by crawl)",
          ),
        ),
      ),
      // fields that may arrive stringified from untyped MCP clients
      lenient: [
        "limit",
        "offset",
        "maxDepth",
        "maxAge",
        "rerank",
        "render",
        "screenshot",
        "images",
        "include_paths",
        "exclude_paths",
        "includeDomains",
        "excludeDomains",
        "requiredText",
      ],
      // Conflicting per-branch bounds/defaults are resolved here. The Go API
      // applies its own defaults (search 10, map 100, crawl 10) and caps, so
      // the schema carries no default for these.
      overrides: {
        limit: v.optional(
          num(
            "Max results — search/crawl 1-100 (default 10), map 1-5000 (default 100)",
            1,
            5000,
          ),
        ),
        mode: v.optional(
          v.pipe(
            v.picklist(["fast", "smart", "static", "dynamic"]),
            v.description(
              "search: 'fast' restricts to recent results; crawl: smart|static|dynamic (API default smart)",
            ),
          ),
        ),
        // Per-action required fields — enforced by DISCOVER_ACTIONS below.
        query: v.optional(
          v.pipe(
            v.string(),
            v.description("Search query (action=search)"),
            v.minLength(1, "Search query cannot be empty"),
          ),
        ),
        url: v.optional(
          v.pipe(
            v.string(),
            v.description("Site URL to map, or crawl root (action=map|crawl)"),
            v.url("Must be a valid URL"),
          ),
        ),
        id: v.optional(
          v.pipe(
            v.string(),
            v.description("Task ID returned by action=crawl (action=crawl_status)"),
            v.minLength(1, "Task ID is required"),
          ),
        ),
      },
    },
  ),
  'tomoshi_discover requires "action": search | map | crawl | crawl_status — e.g. {"action":"search","query":"svelte 5 runes"}',
);
export type DiscoverInput = v.InferOutput<typeof DiscoverSchema>;

/** Per-action required parameters, quoted verbatim in handler errors. */
const DISCOVER_ACTIONS: Record<string, ActionSpec> = {
  search: {
    required: ["query"],
    example: { action: "search", query: "svelte 5 runes" },
    limitMax: 100,
  },
  map: {
    required: ["url"],
    example: { action: "map", url: "https://example.com" },
    limitMax: 5000,
  },
  crawl: {
    required: ["url"],
    example: {
      action: "crawl",
      url: "https://example.com/docs",
      maxDepth: 2,
      limit: 10,
    },
    limitMax: 100,
  },
  crawl_status: {
    required: ["id"],
    example: { action: "crawl_status", id: "crawl_01H…" },
  },
};
const DISCOVER_USAGE =
  "search → query, map → url, crawl → url, crawl_status → id";

/** Fields that may arrive as strings from untyped MCP clients. */
const DISCOVER_SHAPE = {
  numbers: ["limit", "offset", "maxDepth", "maxAge"],
  booleans: ["rerank", "render", "screenshot", "images"],
  json: [
    "include_paths",
    "exclude_paths",
    "includeDomains",
    "excludeDomains",
    "requiredText",
  ],
};

function pageTitle(page: { title?: string; url: string }): string {
  const title = page.title?.trim();
  if (title) return title;
  try {
    const path = new URL(page.url).pathname;
    const last = path.split("/").filter(Boolean).pop();
    if (last) return decodeURIComponent(last);
    return new URL(page.url).hostname;
  } catch {
    return page.url;
  }
}

export function createDiscoverHandler(client: TomoshiClient) {
  return async (input: Record<string, unknown>) => {
    const { value: args, error } = normalize(input, DISCOVER_SHAPE);
    if (error) return toolError(error);
    const invalid = guardAction(
      "tomoshi_discover",
      args,
      DISCOVER_ACTIONS,
      DISCOVER_USAGE,
    );
    if (invalid) return invalid;
    const { action } = args as { action: string };
    try {
      if (action === "search") {
        const params = args as unknown as SearchParams & { action: string };
        const result = await client.search(params);
        const lines: string[] = [
          `# Search Results: "${result.query}"`,
          "",
          `Found ${result.count} results${result.hasMore ? " (more available)" : ""}`,
          "",
        ];
        for (const item of result.results) {
          lines.push(`### ${item.title}`, "", `${item.description}`, "");
          if (item.highlights && item.highlights.length > 0) {
            lines.push(`> ${item.highlights[0]}`, "");
          }
          lines.push(`🔗 ${item.url}`);
          const meta: string[] = [];
          if (item.domain) meta.push(item.domain);
          if (item.id) meta.push(`id: ${item.id}`);
          if (typeof item.relevance === "number")
            meta.push(`relevance: ${item.relevance.toFixed(3)}`);
          if (meta.length > 0) lines.push(`_${meta.join(" · ")}_`);
          lines.push("", "---", "");
        }
        if (result.hasMore)
          lines.push(
            "",
            `Use \`tomoshi_discover\` with \`action: "search"\` and \`offset: ${result.nextOffset}\` to get the next page.`,
          );
        return { content: [{ type: "text" as const, text: lines.join("\n") }] };
      }

      if (action === "map") {
        const params = args as unknown as MapParams & { action: string };
        const result = await client.map(params);
        const lines: string[] = [
          `# Map Result: ${result.url}`,
          "",
          `**Count:** ${result.count}`,
          `**Source URL:** ${result.url}`,
        ];
        if ((params as any).search)
          lines.push(`**Filter:** \`${(params as any).search}\``);
        lines.push("", "---", "");
        if (result.links.length === 0)
          lines.push(
            "No links discovered (site may have no sitemap and no crawlable links).",
          );
        else {
          lines.push(`## Discovered URLs (${result.links.length})`, "");
          for (const link of result.links)
            lines.push(`- ${link.url} _(source: ${link.source})_`);
        }
        return { content: [{ type: "text" as const, text: lines.join("\n") }] };
      }

      if (action === "crawl") {
        const params = args as unknown as CrawlParams & { action: string };
        const result = await client.crawl(params);
        const lines: string[] = [
          "# Crawl Job Enqueued",
          "",
          `**URL:** ${result.url}`,
          `**Task ID:** \`${result.id}\``,
          `**Render:** ${result.render}`,
          `**Screenshot:** ${result.screenshot}`,
          `**Images:** ${result.images}`,
          `**Max Depth:** ${result.maxDepth}`,
          `**Limit:** ${result.limit}`,
        ];
        if (result.image_format)
          lines.push(`**Image Format:** ${result.image_format}`);
        lines.push(
          "",
          "---",
          "",
          `Use \`tomoshi_discover\` with \`action: "crawl_status"\` and \`id: "${result.id}"\` to poll.`,
          "The crawl runs asynchronously — poll until state is `completed` or `failed`.",
          "Requires Redis-backed Tomoshibi instance.",
        );
        return { content: [{ type: "text" as const, text: lines.join("\n") }] };
      }

      // crawl_status
      const { id } = args as { id: string };
      const result = await client.getCrawlStatus(id);
      const lines: string[] = [
        "# Crawl Status",
        "",
        `**Task ID:** \`${result.id}\``,
        `**State:** ${result.state}`,
      ];
      if (result.queue) lines.push(`**Queue:** ${result.queue}`);
      if (result.state === "completed" && result.crawl) {
        const c = result.crawl;
        lines.push(
          "",
          "---",
          `## Results (${c.status}, ${c.total_pages} pages, max depth: ${c.max_depth}, limit: ${c.limit})`,
          "",
        );
        for (const page of c.pages) {
          lines.push(`### ${pageTitle(page)}`);
          lines.push(`🔗 ${page.url}`);
          if (page.preview) lines.push("", `> ${page.preview}`, "");
          lines.push("");
        }
        if (c.failed_urls && c.failed_urls.length > 0) {
          lines.push("### Failed URLs", "");
          for (const f of c.failed_urls) lines.push(`- ${f.url}: ${f.error}`);
        }
      } else if (
        result.state === "failed" &&
        result.failed_urls &&
        result.failed_urls.length > 0
      ) {
        lines.push("", "---", "### Failed URLs", "");
        for (const f of result.failed_urls)
          lines.push(`- ${f.url}: ${f.error}`);
      }
      if (result.state === "failed")
        lines.push(
          "",
          "---",
          "⚠️ The crawl job failed. Try again with a different URL or fewer pages.",
        );
      if (result.state === "pending" || result.state === "active")
        lines.push(
          "",
          "---",
          "⏳ The crawl is still in progress. Check back later.",
        );
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      return {
        content: [
          {
            type: "text" as const,
            text: `Discover ${action} failed: ${message}`,
          },
        ],
        isError: true,
      };
    }
  };
}
