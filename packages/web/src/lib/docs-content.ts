import {
  Play,
  Zap,
  Container,
  Terminal,
  Globe,
  Layers,
  Shield,
  Search,
  FileText,
  Database,
  Copy,
  Clock,
  BookOpen,
  HeartHandshake,
  Box,
  Cpu,
  Map,
  Package,
  Monitor,
} from "@lucide/svelte";
import type { Component } from "svelte";

export type DocSection =
  | { type: "hero"; title: string; description: string }
  | { type: "text"; content: string }
  | {
      type: "alert";
      title: string;
      description: string;
      icon?: Component;
      variant?: "default" | "destructive";
    }
  | {
      type: "cards";
      items: {
        title: string;
        description: string;
        icon?: Component;
        badge?: string;
      }[];
      columns?: number;
    }
  | {
      type: "steps";
      items: {
        title: string;
        description: string;
        code?: string;
        note?: string;
      }[];
    }
  | { type: "code"; code: string; language?: string; title?: string }
  | {
      type: "heading";
      level: 2 | 3;
      text: string;
      icon?: Component;
      iconColor?: string;
    }
  | { type: "flow"; items: { title: string; description: string }[] }
  | {
      type: "colors";
      items: {
        name: string;
        value: string;
        description: string;
        class?: string;
      }[];
    }
  | {
      type: "features";
      items: {
        title: string;
        description: string;
        icon?: Component;
        features: string[];
      }[];
    }
  | { type: "tree"; content: string; note?: string }
  | { type: "list"; items: string[]; title?: string }
  | {
      type: "key-value";
      items: { key: string; value: string; badge?: string }[];
    };

export type DocPage = {
  title: string;
  description: string;
  sections: DocSection[];
};

export const hero = {
  title: "Tomoshibi Documentation",
  description:
    "Self-hosted scraping API — one binary, $5/mo infra. This hosted playground is rate-limited and ephemeral. For unlimited use, run it locally with Docker.",
};

export const quickStart = [
  {
    title: "Run with Docker",
    description: "One command. All 5 services on 7431–7435. Recommended.",
    href: "/docs/setup",
    icon: Container,
    iconColor: "text-primary",
  },
  {
    title: "Try Playground",
    description: "Test scrape / crawl / search in the browser (limited).",
    href: "/playground",
    icon: Play,
    iconColor: "text-amber-500",
  },
  {
    title: "API Reference",
    description: "POST /v1/scrape, /v1/crawl, /v1/search — with examples.",
    href: "/docs/api",
    icon: FileText,
    iconColor: "text-blue-500",
  },
];

export const techStack = [
  {
    title: "API",
    icon: Cpu,
    items: [
      { name: "Go 1.25+ · Gin", badge: "Runtime", variant: "default" },
      { name: "Chromedp + Colly", badge: "Engines", variant: "secondary" },
      {
        name: "SearXNG + Redis",
        badge: "Search & Queue",
        variant: "secondary",
      },
    ],
  },
  {
    title: "Web Playground",
    icon: Globe,
    items: [
      { name: "SvelteKit 5", badge: "Framework", variant: "default" },
      { name: "shadcn-svelte", badge: "UI", variant: "secondary" },
      { name: "Tailwind v4", badge: "Styling", variant: "secondary" },
    ],
  },
  {
    title: "MCP",
    icon: Terminal,
    items: [
      { name: "tomoshi CLI", badge: "npm", variant: "default" },
      {
        name: "3 tools",
        badge: "scrape · crawl · search",
        variant: "secondary",
      },
      { name: "MCP SDK", badge: "Protocol", variant: "secondary" },
    ],
  },
  {
    title: "Self-host",
    icon: Box,
    items: [
      { name: "Docker Compose", badge: "Recommended", variant: "default" },
      { name: "Fly / Render", badge: "Hobby tier", variant: "secondary" },
      { name: "MIT · No telemetry", badge: "License", variant: "secondary" },
    ],
  },
];

export const principles = [
  {
    title: "Self-hosted by default",
    description:
      "Your data never leaves your VPC. One binary + Redis + SearXNG. No per-token billing.",
    icon: Shield,
    highlight: true,
  },
  {
    title: "Fast by design",
    description:
      "Shared Chromedp allocator, recycled tabs, smart static→dynamic fallback. ~200ms static, parallel pools.",
    highlight: false,
  },
  {
    title: "LLM-ready output",
    description:
      "Readability main-content + ad block → clean markdown, metadata, links, optional screenshot.",
    highlight: false,
  },
  {
    title: "Open & funded by you",
    description:
      "MIT licensed. Stars drive discovery, sponsors fund Chromedp & SearXNG upkeep. See FUNDING.yml.",
    highlight: false,
  },
];

export const documentationSections = [
  {
    title: "Quick Start (Docker)",
    description: "Clone and docker compose up -d — 30 seconds to markdown.",
    href: "/docs/setup",
    icon: Container,
  },
  {
    title: "API Reference",
    description:
      "Scrape, crawl, batch, search, map — request & response shapes.",
    href: "/docs/api",
    icon: FileText,
  },
  {
    title: "Self-hosting & Deploy",
    description: "Fly, Render, Docker Hub / GHCR, env vars, and health checks.",
    href: "/docs/deployment",
    icon: Box,
  },
];

export const pages: Record<string, DocPage> = {
  setup: {
    title: "Quick Start — Docker",
    description:
      "Run Tomoshibi locally in 30 seconds. This is the recommended way — the hosted playground is rate-limited and ephemeral.",
    sections: [
      {
        type: "alert",
        title: "Hosted playground is a preview",
        description:
          "Rate-limited, shared, no persistence. For crawl/batch/monitor and unlimited use, self-host with Docker.",
        icon: Zap,
      },
      {
        type: "heading",
        level: 2,
        text: "Full stack — Docker Compose (recommended)",
      },
      {
        type: "steps",
        items: [
          {
            title: "Clone",
            description: "Get the source and compose file.",
            code: "git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi",
          },
          {
            title: "Up",
            description:
              "Starts api (7431) + web (7432) + mcp (7433) + redis (7434) + searxng (7435).",
            code: 'docker compose up -d\ncurl http://localhost:7431/health  # → {"status":"ok"}',
          },
          {
            title: "Scrape",
            description: "Smart mode is default — no config needed.",
            code: 'curl -X POST http://localhost:7431/v1/scrape \\\n  -H "Content-Type: application/json" \\\n  -d \'{"url": "https://example.com"}\'',
          },
        ],
      },
      {
        type: "heading",
        level: 2,
        text: "Single image",
      },
      {
        type: "code",
        title: "Docker Hub / GHCR",
        code: "docker pull michaelobele/tomoshibi-api:latest\ndocker run --rm -p 7431:7431 michaelobele/tomoshibi-api\n# or GHCR\ndocker pull ghcr.io/michael-obele/tomoshibi-api:latest",
      },
      {
        type: "heading",
        level: 2,
        text: "From source (Go 1.25+)",
      },
      {
        type: "code",
        code: "go run ./packages/api/cmd/api\n# with async (crawl/batch/monitor)\nREDIS_URL=redis://localhost:7434 go run ./packages/api/cmd/api",
      },
      {
        type: "heading",
        level: 2,
        text: "Next steps",
      },
      {
        type: "cards",
        columns: 2,
        items: [
          {
            title: "API Reference",
            description: "All endpoints with curl examples.",
            icon: FileText,
          },
          {
            title: "Try Playground",
            description: "Test without Docker (limited).",
            icon: Play,
          },
        ],
      },
    ],
  },
  api: {
    title: "API Reference",
    description:
      "Full reference for every endpoint. Base URL is http://localhost:7431 (compose) or your deploy URL. Smart mode is default. No API key needed for self-host unless API_KEYS is set. Swagger UI at /swagger in debug mode.",
    sections: [
      {
        type: "alert",
        title: "Base URL & auth",
        description:
          "All endpoints are under /v1. Set API_KEYS (comma-separated) to require X-API-Key on /v1/* (401 otherwise). RATE_LIMIT_RPM enables per-client limiting (429 with retry_after). /health and /swagger are unauthenticated.",
        icon: Shield,
      },
      {
        type: "heading",
        level: 2,
        text: "1. Scrape — POST /v1/scrape  ·  GET /v1/scrape",
        icon: Globe,
        iconColor: "text-amber-500",
      },
      {
        type: "text",
        content:
          "Scrape a URL to markdown + metadata. Smart tries static (Colly) first, falls back to dynamic (Chromedp) for SPAs. Supports single URL (url) or sync multi-URL (urls: 1–10, no Redis, parallel 5, ordered results). GET mirrors POST via query params.",
      },
      {
        type: "code",
        title: "Request — single URL (POST)",
        code: 'curl -X POST http://localhost:7431/v1/scrape \\\n  -H "Content-Type: application/json" \\\n  -d \'{\n    "url": "https://example.com",\n    "mode": "smart",\n    "onlyMainContent": true,\n    "blockAds": true,\n    "include_links": true,\n    "screenshot": true,\n    "screenshot_opts": { "full_page": true, "format": "jpeg", "quality": 80 },\n    "images": true,\n    "image_format": "blob",\n    "max_images": 5,\n    "summary": true,\n    "summary_sentences": 5,\n    "extract_schema": { "title": { "selector": "h1" }, "links": { "selector": "a", "attr": "href", "multiple": true } },\n    "actions": [{ "type": "scroll_to_bottom" }]\n  }\'',
      },
      {
        type: "code",
        title: "Request — GET",
        code: 'curl "http://localhost:7431/v1/scrape?url=https://example.com&mode=smart&include_links=true"',
      },
      {
        type: "code",
        title: "Request — sync multi-URL (urls: [])",
        code: 'curl -X POST http://localhost:7431/v1/scrape \\\n  -H "Content-Type: application/json" \\\n  -d \'{"urls": ["https://example.com","https://example.org"], "mode": "smart", "images": true}\'',
      },
      {
        type: "key-value",
        items: [
          {
            key: "url",
            value: "string — required unless urls is set. Exclusive with urls.",
          },
          {
            key: "urls",
            value:
              "string[] — 1–10 URLs, exclusive with url. Returns { results: [...] }.",
          },
          { key: "mode", value: "smart | static | dynamic (default: smart)" },
          {
            key: "onlyMainContent",
            value: "boolean — readability extraction (default: true)",
          },
          {
            key: "blockAds",
            value: "boolean — strip ad/tracker containers (default: true)",
          },
          {
            key: "remove_base64_images",
            value: "boolean — drop data: images (default: true)",
          },
          {
            key: "include_links",
            value:
              "boolean — include links: [{url,text,isInternal}] (default: true)",
          },
          {
            key: "screenshot",
            value: "boolean — base64 JPEG/PNG (needs smart/dynamic)",
          },
          {
            key: "screenshot_opts",
            value:
              "object — {width,height,full_page,format,quality,wait_selector}",
          },
          { key: "images", value: "boolean — extract images as blobs" },
          { key: "image_format", value: "url | blob (default: url)" },
          {
            key: "max_images",
            value: "int — max images, quality-ranked (default: 10)",
          },
          {
            key: "max_image_size_kb",
            value: "int — max image file size KB (default: 5120)",
          },
          {
            key: "image_process",
            value: "object — {format,max_width,quality} resize/re-encode",
          },
          {
            key: "actions",
            value:
              "array — dynamic only: wait_ms, wait_selector, click, scroll_down, scroll_to_bottom",
          },
          {
            key: "extract_schema",
            value:
              "object — {field: {selector,attr?,multiple?}} deterministic extraction",
          },
          { key: "summary", value: "boolean — extractive summary (no LLM)" },
          {
            key: "summary_sentences",
            value: "int — sentence count (default: 5)",
          },
          {
            key: "redact_pii",
            value: "boolean — mask emails/phones/cards in markdown/summary",
          },
          {
            key: "render",
            value: "boolean — deprecated, alias for mode=dynamic",
          },
        ],
      },
      {
        type: "code",
        title: "Response — single URL",
        code: '{\n  "url": "https://example.com",\n  "markdown": "# Example Domain\\n\\n...",\n  "html": "<!doctype html>...",\n  "metadata": { "title": "Example Domain", "description": "..." },\n  "links": [{ "url": "https://www.iana.org/domains/example", "text": "More information...", "isInternal": false }],\n  "screenshot": { "format": "jpeg", "width": 1280, "height": 800, "full_page": true, "size_bytes": 12345, "truncated": false },\n  "images": [{ "url": "https://...", "alt": "..." }],\n  "extracted": { "title": "Example Domain" },\n  "summary": "This domain is for use in..."\n}',
      },
      {
        type: "code",
        title: "Response — multi-URL (results[])",
        code: '{\n  "results": [\n    { "url": "https://example.com", "markdown": "# ...", "metadata": { "title": "..." }, "html": "..." },\n    { "url": "https://example.org", "error": "scrape failed: ..." }\n  ]\n}\n// On partial failure an entry carries "error" and no markdown; order matches input.',
      },
      {
        type: "heading",
        level: 2,
        text: "2. Search — POST /v1/search  ·  GET /v1/search",
        icon: Search,
        iconColor: "text-green-500",
      },
      {
        type: "text",
        content:
          "Hybrid search: SearXNG (primary, free) → Brave API (fallback, 1 QPS) → Stealth (chromedp, reuses shared allocator, gofakeit UA). SEARXNG_ENDPOINT, BRAVE_SEARCH_API_KEY, STEALTH_ENABLED control backends.",
      },
      {
        type: "code",
        title: "Request",
        code: 'curl -X POST http://localhost:7431/v1/search \\\n  -H "Content-Type: application/json" \\\n  -d \'{"query": "self-hosted scraping", "limit": 5, "offset": 0, "category": "general", "rerank": true}\'\n\n# GET\ncurl "http://localhost:7431/v1/search?q=self-hosted%20scraping&limit=5&category=news"',
      },
      {
        type: "key-value",
        items: [
          { key: "query | q", value: "string — required" },
          { key: "limit", value: "int — max 100 (default: 10)" },
          { key: "offset", value: "int — pagination offset (default: 0)" },
          {
            key: "category",
            value: "general | news | code (validated, 400 on invalid)",
          },
          {
            key: "rerank",
            value: "boolean — TF-IDF re-rank (no ONNX, pure Go)",
          },
          {
            key: "mode",
            value:
              "string — fast = recent only; legacy news|code maps to category",
          },
          { key: "includeDomains", value: "string[] — restrict to domains" },
          { key: "excludeDomains", value: "string[] — exclude domains" },
          { key: "requiredText", value: "string[] — must contain text" },
          { key: "maxAge", value: "int — 1 (day), 7 (week), 30 (month)" },
        ],
      },
      {
        type: "code",
        title: "Response",
        code: '{\n  "query": "self-hosted scraping",\n  "results": [{ "title": "...", "url": "https://...", "description": "...", "highlights": ["…120-char window…"], "relevance": 0.85 }],\n  "hasMore": true,\n  "nextOffset": 5,\n  "count": 1\n}',
      },
      {
        type: "heading",
        level: 2,
        text: "3. Crawl (async) — POST /v1/crawl",
        icon: Layers,
        iconColor: "text-blue-500",
      },
      {
        type: "text",
        content:
          "Async BFS crawl via Asynq/Redis. Requires REDIS_URL. Returns 202 with job id. Poll GET /v1/crawl/:id. Domain-locked, deduped, skips non-HTML, bounded queue (drops excess, never deadlocks).",
      },
      {
        type: "code",
        title: "Request",
        code: 'curl -X POST http://localhost:7431/v1/crawl \\\n  -H "Content-Type: application/json" \\\n  -d \'{\n    "url": "https://docs.example.com",\n    "maxDepth": 3,\n    "limit": 20,\n    "mode": "smart",\n    "exclude_paths": ["/admin/*", "/login"],\n    "include_paths": ["/docs/**"],\n    "webhook_url": "https://myapp.example.com/hooks/tomoshibi",\n    "webhook_secret": "s3cret"\n  }\'',
      },
      {
        type: "key-value",
        items: [
          { key: "url", value: "string — required, seed URL" },
          { key: "mode", value: "smart | static | dynamic (default: smart)" },
          {
            key: "maxDepth",
            value: "int — max link depth, capped at 10 (default: 2)",
          },
          {
            key: "limit",
            value: "int — max pages, capped at 100 (default: 10)",
          },
          {
            key: "render",
            value: "boolean — alias for dynamic (default: false)",
          },
          { key: "screenshot", value: "boolean — per-page screenshot" },
          { key: "images", value: "boolean — per-page image extraction" },
          {
            key: "include_paths",
            value:
              "string[] — gobwas/glob, * within segment, ** crosses segments",
          },
          {
            key: "exclude_paths",
            value:
              "string[] — exclusion wins; seed URL always bypasses filters",
          },
          { key: "webhook_url", value: "string — POST result on completion" },
          {
            key: "webhook_secret",
            value: "string — HMAC-SHA256 for X-Tomoshibi-Signature",
          },
        ],
      },
      {
        type: "code",
        title: "Response — 202 Accepted",
        code: '{\n  "id": "e8a932c0-82af-4a11-bd4a-6f17e29b1111",\n  "url": "https://docs.example.com",\n  "maxDepth": 3,\n  "limit": 20\n}',
      },
      {
        type: "text",
        content:
          "Tuning via env: CRAWL_CONCURRENCY (default 4, max 10), CRAWL_DOMAIN_DELAY (1s), CRAWL_MAX_RETRIES (2, never retry 4xx), CRAWL_TIMEOUT (30m → status timeout), CRAWL_SCRAPE_TIMEOUT (30s per page).",
      },
      {
        type: "heading",
        level: 2,
        text: "4. Crawl Status — GET /v1/crawl/:id",
        icon: Clock,
        iconColor: "text-amber-500",
      },
      {
        type: "code",
        title: "Request",
        code: "curl http://localhost:7431/v1/crawl/e8a932c0-82af-4a11-bd4a-6f17e29b1111",
      },
      {
        type: "code",
        title: "Response — in progress",
        code: '{ "id": "...", "queue": "default", "state": "active" }',
      },
      {
        type: "code",
        title: "Response — completed",
        code: '{\n  "id": "...",\n  "queue": "default",\n  "state": "completed",\n  "crawl": {\n    "status": "completed | partial | failed | cancelled | timeout",\n    "total_pages": 5,\n    "max_depth": 3,\n    "limit": 20,\n    "pages": [{ "url": "https://...", "title": "...", "preview": "first 300 chars..." }]\n  },\n  "failed_urls": [{ "url": "https://.../404", "error": "scraping failed: ..." }]\n}',
      },
      {
        type: "text",
        content:
          "Frontend poll pattern: GET /v1/crawl/:id every 5s until state is completed or failed. Render pages[] with title + preview.",
      },
      {
        type: "heading",
        level: 2,
        text: "5. Map — POST /v1/map",
        icon: Map,
        iconColor: "text-emerald-500",
      },
      {
        type: "text",
        content:
          "URL discovery without scraping. Reads robots.txt → sitemap.xml (recursive sitemap-index, up to 5,000 URLs), falls back to one-level link discovery.",
      },
      {
        type: "code",
        title: "Request",
        code: 'curl -X POST http://localhost:7431/v1/map \\\n  -H "Content-Type: application/json" \\\n  -d \'{"url": "https://docs.example.com", "search": "/docs", "limit": 200}\'',
      },
      {
        type: "key-value",
        items: [
          { key: "url", value: "string — required" },
          {
            key: "search",
            value: "string — only return URLs containing substring",
          },
          { key: "limit", value: "int — max URLs, max 5000 (default: 100)" },
        ],
      },
      {
        type: "code",
        title: "Response",
        code: '{ "url": "https://docs.example.com", "count": 2, "links": [{ "url": "https://.../intro", "source": "sitemap" }, { "url": "https://.../api", "source": "link" }] }',
      },
      {
        type: "heading",
        level: 2,
        text: "6. Batch Scrape — POST /v1/batch/scrape  ·  GET /v1/batch/:id",
        icon: Package,
        iconColor: "text-violet-500",
      },
      {
        type: "text",
        content:
          "Async batch scrape (Redis). Enqueue up to 20 URLs in one call.",
      },
      {
        type: "code",
        title: "Request — enqueue",
        code: 'curl -X POST http://localhost:7431/v1/batch/scrape \\\n  -H "Content-Type: application/json" \\\n  -d \'{"urls": ["https://a.example.com", "https://b.example.com"]}\'',
      },
      {
        type: "code",
        title: "Response — 202",
        code: '{ "batch_id": "3f2a...", "tasks": [{ "id": "task-id-1", "url": "https://a.example.com" }, { "id": "task-id-2", "url": "https://b.example.com" }] }',
      },
      {
        type: "code",
        title: "Status — GET /v1/batch/:id",
        code: '{ "batch_id": "3f2a...", "total": 2, "completed": 1, "failed": 0, "tasks": [ ... ] }',
      },
      {
        type: "heading",
        level: 2,
        text: "7. Monitor — POST /v1/monitor  ·  GET /v1/monitor/:id  ·  DELETE /v1/monitor/:id",
        icon: Monitor,
        iconColor: "text-rose-500",
      },
      {
        type: "text",
        content:
          "Change tracking (Redis). Scrapes on a schedule, hashes markdown (SHA-256), fires signed webhook on change. First check records baseline without notifying. Minimum interval 3600s (1h).",
      },
      {
        type: "code",
        title: "Create",
        code: 'curl -X POST http://localhost:7431/v1/monitor \\\n  -H "Content-Type: application/json" \\\n  -d \'{\n    "url": "https://pricing.example.com",\n    "interval_seconds": 3600,\n    "webhook_url": "https://myapp.example.com/hooks/price-changed",\n    "webhook_secret": "s3cret"\n  }\'',
      },
      {
        type: "code",
        title: "Status & delete",
        code: "curl http://localhost:7431/v1/monitor/<id>\n# → { config, last_hash, next_check }\n\ncurl -X DELETE http://localhost:7431/v1/monitor/<id>",
      },
      {
        type: "code",
        title: "Webhook payload",
        code: '{\n  "monitor_id": "abc123",\n  "url": "https://pricing.example.com",\n  "changed": true,\n  "hash_old": "aaa...",\n  "hash_new": "bbb...",\n  "changed_at": "2026-08-01T12:00:00Z"\n}\n// Header: X-Tomoshibi-Signature: sha256=<hmac-hex> (HMAC-SHA256 of body with webhook_secret)',
      },
      {
        type: "heading",
        level: 2,
        text: "8. Health & Swagger",
      },
      {
        type: "key-value",
        items: [
          {
            key: "GET /health",
            value: "Health check — no auth, no rate limit",
          },
          {
            key: "GET /swagger/index.html",
            value: "Swagger UI — only in debug mode (SERVER_MODE=debug)",
          },
        ],
      },
      {
        type: "alert",
        title: "Auth & rate limiting",
        description:
          'Set API_KEYS to require X-API-Key on /v1/* (401 otherwise). RATE_LIMIT_RPM sets per-client limit (429 with retry_after). Redis-backed when REDIS_URL is set, else in-memory per instance. Example: curl -H "X-API-Key: sk-a" http://localhost:7431/v1/scrape -d \'{"url":"https://example.com"}\'',
        icon: Shield,
      },
    ],
  },
  deployment: {
    title: "Self-hosting & Deployment",
    description:
      "Deploy to Fly, Render, or any Docker host. Hobby-tier friendly (512MB–1GB).",
    sections: [
      {
        type: "heading",
        level: 2,
        text: "Environment",
      },
      {
        type: "key-value",
        items: [
          { key: "PORT", value: "Default 7431 (api), 7432 (web)" },
          {
            key: "REDIS_URL",
            value: "Required for /v1/crawl, /v1/batch, /v1/monitor",
          },
          {
            key: "SEARXNG_URL",
            value: "Default http://searxng:8080 (compose)",
          },
          { key: "API_KEYS", value: "Comma-separated keys to enable auth" },
          { key: "RATE_LIMIT_RPM", value: "Requests per minute (optional)" },
        ],
      },
      {
        type: "heading",
        level: 2,
        text: "Fly.io",
      },
      {
        type: "code",
        code: "fly launch --dockerfile packages/api/Dockerfile\nfly secrets set REDIS_URL=redis://...\nfly deploy",
      },
      {
        type: "heading",
        level: 2,
        text: "Docker Hub / GHCR",
      },
      {
        type: "code",
        code: "docker pull michaelobele/tomoshibi:latest\ndocker pull ghcr.io/michael-obele/tomoshibi:latest",
      },
      {
        type: "heading",
        level: 2,
        text: "Support the project",
      },
      {
        type: "alert",
        title: "Sponsor & Star",
        description:
          "Tomoshibi is MIT and free. Sponsors fund browser upkeep and SearXNG tuning. Stars drive discovery. Both keep the free tier alive.",
        icon: HeartHandshake,
      },
      {
        type: "list",
        title: "How to help",
        items: [
          "Star on GitHub — https://github.com/Michael-Obele/tomoshibi",
          "Sponsor — https://github.com/sponsors/Michael-Obele (see FUNDING.yml)",
          "Share your self-hosting setup in Discussions",
        ],
      },
    ],
  },
};

export const documentationGroups = [
  {
    title: "Getting Started",
    items: [
      { title: "Introduction", url: "/docs", icon: BookOpen },
      { title: "Quick Start", url: "/docs/setup", icon: Zap },
    ],
  },
  {
    title: "Reference",
    items: [
      { title: "API Reference", url: "/docs/api", icon: FileText },
      { title: "Self-hosting", url: "/docs/deployment", icon: Box },
    ],
  },
];
