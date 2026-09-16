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
      { name: "SearXNG + Redis", badge: "Search & Queue", variant: "secondary" },
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
      { name: "3 tools", badge: "scrape · crawl · search", variant: "secondary" },
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
    description: "Scrape, crawl, batch, search, map — request & response shapes.",
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
            description: "Starts api (7431) + web (7432) + mcp (7433) + redis (7434) + searxng (7435).",
            code: "docker compose up -d\ncurl http://localhost:7431/health  # → {\"status\":\"ok\"}",
          },
          {
            title: "Scrape",
            description: "Smart mode is default — no config needed.",
            code: "curl -X POST http://localhost:7431/v1/scrape \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"url\": \"https://example.com\"}'",
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
      "All endpoints are POST under /v1. Smart mode is default. No API key needed for self-host (set API_KEYS to enable auth).",
    sections: [
      {
        type: "heading",
        level: 2,
        text: "Scrape — POST /v1/scrape",
        icon: Globe,
        iconColor: "text-amber-500",
      },
      {
        type: "text",
        content:
          "Convert any URL to LLM-ready markdown. Smart mode tries static (Colly) first, falls back to dynamic (Chromedp) for SPAs.",
      },
      {
        type: "code",
        title: "Request",
        code: "curl -X POST http://localhost:7431/v1/scrape \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\n    \"url\": \"https://example.com\",\n    \"mode\": \"smart\",\n    \"onlyMainContent\": true,\n    \"blockAds\": true\n  }'",
      },
      {
        type: "code",
        title: "Response",
        code: "{\n  \"url\": \"https://example.com\",\n  \"markdown\": \"# Example Domain\\n\\n...\",\n  \"html\": \"<h1>...</h1>\",\n  \"metadata\": { \"title\": \"Example Domain\", \"description\": \"...\" },\n  \"links\": [\"https://example.com/more\"]\n}",
      },
      {
        type: "key-value",
        items: [
          { key: "mode", value: "smart | static | dynamic (default: smart)" },
          { key: "onlyMainContent", value: "Readability extraction (default: true)" },
          { key: "blockAds", value: "Remove ads/trackers" },
          { key: "screenshot", value: "Return base64 screenshot" },
          { key: "extract_schema", value: "CSS-selector JSON extraction" },
        ],
      },
      {
        type: "heading",
        level: 2,
        text: "Crawl — POST /v1/crawl",
        icon: Layers,
        iconColor: "text-blue-500",
      },
      {
        type: "text",
        content:
          "Async BFS crawl with Redis. Requires REDIS_URL. Returns job ID — poll GET /v1/crawl/{id}.",
      },
      {
        type: "code",
        code: "curl -X POST http://localhost:7431/v1/crawl \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"url\": \"https://example.com\", \"limit\": 20, \"maxDepth\": 2}'\n\n# poll\ncurl http://localhost:7431/v1/crawl/<id>",
      },
      {
        type: "heading",
        level: 2,
        text: "Search — POST /v1/search",
        icon: Search,
        iconColor: "text-green-500",
      },
      {
        type: "text",
        content: "SearXNG-backed search. No Brave key needed when self-hosting with compose.",
      },
      {
        type: "code",
        code: "curl -X POST http://localhost:7431/v1/search \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"query\": \"self-hosted scraping\", \"limit\": 5}'",
      },
      {
        type: "heading",
        level: 2,
        text: "Other endpoints",
      },
      {
        type: "key-value",
        items: [
          { key: "POST /v1/batch", value: "Batch scrape (async, Redis)" },
          { key: "POST /v1/map", value: "Sitemap discovery" },
          { key: "POST /v1/monitor", value: "Change tracking (webhook)" },
          { key: "GET /health", value: "Health check (no auth)" },
        ],
      },
      {
        type: "alert",
        title: "Need full spec?",
        description:
          "See README.md and docs/guides/API_REFERENCE.md in the repo. Swagger UI is available at /swagger in debug mode.",
        icon: BookOpen,
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
          { key: "REDIS_URL", value: "Required for /v1/crawl, /v1/batch, /v1/monitor" },
          { key: "SEARXNG_URL", value: "Default http://searxng:8080 (compose)" },
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
