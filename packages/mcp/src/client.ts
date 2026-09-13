import { getConfig } from "./config.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Scrape types
// ---------------------------------------------------------------------------

export interface ScreenshotOpts {
  width?: number;
  height?: number;
  full_page?: boolean;
  format?: "jpeg" | "png";
  quality?: number;
  wait_selector?: string;
}

export interface ImageProcessOpts {
  format?: "jpeg" | "png";
  max_width?: number;
  quality?: number;
}

export type ScrapeActionType =
  | "wait_ms"
  | "wait_selector"
  | "click"
  | "scroll_down"
  | "scroll_to_bottom";

export interface ScrapeAction {
  type: ScrapeActionType;
  ms?: number;
  selector?: string;
}

export interface ExtractField {
  selector: string;
  attr?: string;
  multiple?: boolean;
}

export interface ImageData {
  url?: string;
  blob?: string;
  format?: string;
  alt?: string;
  title?: string;
  source?: string;
  width?: number;
  height?: number;
  size_bytes?: number;
}

export interface ScreenshotData {
  blob?: string;
  url?: string;
  format?: string;
  full_page?: boolean;
  width?: number;
  height?: number;
  size_bytes?: number;
  captured_at?: string;
}

export interface ScrapeParams {
  url: string;
  mode?: "smart" | "static" | "dynamic";
  screenshot?: boolean;
  screenshot_opts?: ScreenshotOpts;
  images?: boolean;
  image_format?: "url" | "blob";
  max_images?: number;
  max_image_size_kb?: number;
  image_process?: ImageProcessOpts;
  actions?: ScrapeAction[];
  extract_schema?: Record<string, ExtractField>;
  summary?: boolean;
  summary_sentences?: number;
  redact_pii?: boolean;
  block_ads?: boolean;
  remove_base64_images?: boolean;
  include_links?: boolean;
  /** @deprecated Use `mode: "dynamic"` instead. */
  render?: boolean;
}

export interface ScrapeResult {
  url: string;
  markdown: string;
  html?: string;
  metadata: Record<string, string>;
  images?: ImageData[];
  screenshot?: ScreenshotData;
  summary?: string;
  extracted?: Record<string, unknown>;
  links?: LinkData[];
}

export interface MultiScrapeParams {
  urls: string[];
  mode?: "smart" | "static" | "dynamic";
  screenshot?: boolean;
  screenshot_opts?: ScreenshotOpts;
  images?: boolean;
  image_format?: "url" | "blob";
  max_images?: number;
  max_image_size_kb?: number;
  image_process?: ImageProcessOpts;
  actions?: ScrapeAction[];
  extract_schema?: Record<string, ExtractField>;
  summary?: boolean;
  summary_sentences?: number;
  redact_pii?: boolean;
  block_ads?: boolean;
  remove_base64_images?: boolean;
  include_links?: boolean;
  /** @deprecated Use `mode: "dynamic"` instead. */
  render?: boolean;
}

export interface MultiScrapeItem {
  url: string;
  title?: string;
  word_count?: number;
  markdown?: string;
  html?: string;
  metadata?: Record<string, string>;
  screenshot?: ScreenshotData;
  images?: ImageData[];
  links?: LinkData[];
  extracted?: Record<string, unknown>;
  summary?: string;
  error?: string;
}

export interface MultiScrapeResponse {
  results: MultiScrapeItem[];
}

// ---------------------------------------------------------------------------
// Crawl types
// ---------------------------------------------------------------------------

export interface CrawlParams {
  url: string;
  mode?: "smart" | "static" | "dynamic";
  render?: boolean;
  screenshot?: boolean;
  images?: boolean;
  image_format?: "url" | "blob";
  maxDepth?: number;
  limit?: number;
  include_paths?: string[];
  exclude_paths?: string[];
  webhook_url?: string;
  webhook_secret?: string;
}

export interface CrawlResponse {
  id: string;
  url: string;
  render: boolean;
  screenshot: boolean;
  images: boolean;
  maxDepth: number;
  limit: number;
  image_format?: string;
}

export interface CrawlPageResult {
  url: string;
  /** May be absent — the backend omits it when the page has no H1 heading. */
  title?: string;
  preview?: string;
}

export interface CrawlResultData {
  status: "completed" | "partial" | "failed" | "cancelled";
  total_pages: number;
  max_depth: number;
  limit: number;
  pages: CrawlPageResult[];
  failed_urls?: { url: string; error: string }[];
}

export interface CrawlStatusResponse {
  id: string;
  queue?: string;
  state: "pending" | "active" | "completed" | "failed" | "retry" | "cancelled";
  crawl?: CrawlResultData;
  failed_urls?: { url: string; error: string }[];
}

// ---------------------------------------------------------------------------
// Search types
// ---------------------------------------------------------------------------

export interface SearchParams {
  query: string;
  offset?: number;
  limit?: number;
  mode?: "fast";
  includeDomains?: string[];
  excludeDomains?: string[];
  requiredText?: string[];
  maxAge?: 1 | 7 | 30;
  category?: "general" | "news" | "code";
  rerank?: boolean;
}

export interface SearchResult {
  title: string;
  url: string;
  description: string;
  domain?: string;
  id?: string;
  relevance?: number;
  highlights?: string[];
}

export interface SearchResponse {
  query: string;
  results: SearchResult[];
  hasMore: boolean;
  nextOffset: number;
  count: number;
}

// ---------------------------------------------------------------------------
// Link types (mirrors domain.LinkData)
// ---------------------------------------------------------------------------

export interface LinkData {
  url: string;
  text?: string;
  isInternal: boolean;
}

// ---------------------------------------------------------------------------
// Map types (POST /v1/map)
// ---------------------------------------------------------------------------

export interface MapParams {
  url: string;
  search?: string;
  limit?: number;
}

export interface MapLink {
  url: string;
  source: string;
}

export interface MapResponse {
  url: string;
  count: number;
  links: MapLink[];
}

// ---------------------------------------------------------------------------
// Batch types (POST /v1/batch/scrape + GET /v1/batch/:id)
// ---------------------------------------------------------------------------

export interface BatchParams {
  urls: string[];
}

export interface BatchTask {
  id: string;
  url: string;
}

export interface BatchResponse {
  batch_id: string;
  tasks: BatchTask[];
}

export interface BatchStatusResponse {
  batch_id: string;
  total: number;
  completed: number;
  failed: number;
  tasks: BatchTask[];
}

// ---------------------------------------------------------------------------
// Monitor types
// ---------------------------------------------------------------------------

export interface MonitorParams {
  url: string;
  interval_seconds?: number;
  webhook_url?: string;
  webhook_secret?: string;
}

export interface MonitorResponse {
  id: string;
  url: string;
  interval_seconds: number;
  next_check: string;
}

export interface MonitorStatusResponse {
  id: string;
  url: string;
  interval_seconds: number;
  next_check: string;
  last_hash?: string;
}

// ---------------------------------------------------------------------------
// SSRF Prevention
// ---------------------------------------------------------------------------

/**
 * Validate a URL to prevent Server-Side Request Forgery attacks.
 * Blocks private IPs, localhost, and non-HTTP(S) protocols.
 */
function validateUrl(url: string): boolean {
  try {
    const parsed = new URL(url);

    // Must be HTTP or HTTPS
    if (!["http:", "https:"].includes(parsed.protocol)) {
      return false;
    }

    // Block private/internal hosts
    const hostname = parsed.hostname.toLowerCase();
    if (
      hostname === "localhost" ||
      hostname === "127.0.0.1" ||
      hostname === "0.0.0.0" ||
      hostname.startsWith("10.") ||
      hostname.startsWith("172.16.") ||
      hostname.startsWith("192.168.") ||
      hostname.endsWith(".local") ||
      hostname.endsWith(".internal")
    ) {
      return false;
    }

    return true;
  } catch {
    return false;
  }
}

// ---------------------------------------------------------------------------
// Cinder API Client
// ---------------------------------------------------------------------------

/**
 * Timeout configuration per endpoint type (milliseconds).
 *
 * These are deliberately generous: the Cinder backend runs on Fly.io with
 * scale-to-zero, so a request may first trigger a ~10-15s cold start before
 * the actual work begins. Tight timeouts turn cold starts into hard failures.
 */
export const CINDER_TIMEOUT = {
  scrape: 60_000,
  crawl: 30_000,
  crawlStatus: 15_000,
  search: 30_000,
  monitor: 30_000,
  monitorStatus: 15_000,
  monitorDelete: 15_000,
  map: 30_000,
  batch: 30_000,
  batchStatus: 15_000,
  links: 60_000,
} as const;

/**
 * HTTP client for the Cinder API.
 * Wraps all Cinder endpoints with type-safe methods, error handling, and SSRF prevention.
 */
export class CinderClient {
  private baseUrl: string;
  private apiKey: string;

  constructor() {
    const config = getConfig();
    this.baseUrl = config.CINDER_API_URL.replace(/\/+$/, "");
    this.apiKey = config.CINDER_API_KEY;
  }

  private get headers(): Record<string, string> {
    const h: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (this.apiKey) {
      // Cinder expects the API key as `X-API-Key` (see API docs §8).
      h["X-API-Key"] = this.apiKey;
    }
    return h;
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
    timeout?: number,
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const controller = new AbortController();
    const timer = timeout
      ? setTimeout(() => controller.abort(), timeout)
      : null;

    try {
      const response = await fetch(url, {
        method,
        headers: this.headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });

      // 204 No Content (e.g. DELETE /v1/monitor/:id) has no body to parse.
      if (response.status === 204) {
        return undefined as T;
      }

      if (!response.ok) {
        const errorBody = await response.text().catch(() => "");
        throw new CinderError(
          `Cinder API error: ${response.status} ${response.statusText}`,
          response.status,
          errorBody,
        );
      }

      return (await response.json()) as T;
    } catch (err) {
      if (err instanceof CinderError) throw err;
      if (err instanceof DOMException && err.name === "AbortError") {
        throw new CinderError(`Request timed out after ${timeout}ms`, 408, "");
      }
      throw new CinderError(
        err instanceof Error ? err.message : "Unknown error",
        0,
        "",
      );
    } finally {
      if (timer) clearTimeout(timer);
    }
  }

  /**
   * Scrape a single webpage.
   * POST /v1/scrape
   */
  async scrape(params: ScrapeParams): Promise<ScrapeResult> {
    if (!validateUrl(params.url)) {
      throw new CinderError(
        "Invalid or blocked URL. Only HTTP(S) URLs to public hosts are allowed.",
        400,
        "",
      );
    }
    return this.request<ScrapeResult>(
      "POST",
      "/v1/scrape",
      params,
      CINDER_TIMEOUT.scrape,
    );
  }

  /**
   * Scrape multiple URLs synchronously (no Redis).
   * POST /v1/scrape with { urls: [...] } — mirrors web_fetch_exa and
   * Firecrawl POST /v2/batch/scrape sync. Max 10 URLs, errgroup limit 5.
   */
  async scrapeMulti(params: MultiScrapeParams): Promise<MultiScrapeResponse> {
    for (const u of params.urls) {
      if (!validateUrl(u)) {
        throw new CinderError(
          `Invalid or blocked URL in multi-scrape: ${u}. Only HTTP(S) URLs to public hosts are allowed.`,
          400,
          "",
        );
      }
    }
    return this.request<MultiScrapeResponse>(
      "POST",
      "/v1/scrape",
      params,
      CINDER_TIMEOUT.scrape,
    );
  }

  /**
   * Enqueue an asynchronous crawl job.
   * POST /v1/crawl
   */
  async crawl(params: CrawlParams): Promise<CrawlResponse> {
    if (!validateUrl(params.url)) {
      throw new CinderError(
        "Invalid or blocked URL. Only HTTP(S) URLs to public hosts are allowed.",
        400,
        "",
      );
    }
    return this.request<CrawlResponse>(
      "POST",
      "/v1/crawl",
      params,
      CINDER_TIMEOUT.crawl,
    );
  }

  /**
   * Get crawl job status.
   * GET /v1/crawl/:id
   */
  async getCrawlStatus(id: string): Promise<CrawlStatusResponse> {
    return this.request<CrawlStatusResponse>(
      "GET",
      `/v1/crawl/${encodeURIComponent(id)}`,
      undefined,
      CINDER_TIMEOUT.crawlStatus,
    );
  }

  /**
   * Search the web via SearXNG (primary) or Brave Search (fallback).
   * POST /v1/search
   */
  async search(params: SearchParams): Promise<SearchResponse> {
    return this.request<SearchResponse>(
      "POST",
      "/v1/search",
      params,
      CINDER_TIMEOUT.search,
    );
  }

  /**
   * Discover URLs on a site via sitemap/link traversal.
   * POST /v1/map
   */
  async map(params: MapParams): Promise<MapResponse> {
    if (!validateUrl(params.url)) {
      throw new CinderError(
        "Invalid or blocked URL. Only HTTP(S) URLs to public hosts are allowed.",
        400,
        "",
      );
    }
    return this.request<MapResponse>(
      "POST",
      "/v1/map",
      params,
      CINDER_TIMEOUT.map,
    );
  }

  /**
   * Enqueue a batch scrape job (requires Redis).
   * POST /v1/batch/scrape
   */
  async batchScrape(params: BatchParams): Promise<BatchResponse> {
    for (const u of params.urls) {
      if (!validateUrl(u)) {
        throw new CinderError(
          `Invalid or blocked URL in batch: ${u}. Only HTTP(S) URLs to public hosts are allowed.`,
          400,
          "",
        );
      }
    }
    return this.request<BatchResponse>(
      "POST",
      "/v1/batch/scrape",
      params,
      CINDER_TIMEOUT.batch,
    );
  }

  /**
   * Get batch scrape status (requires Redis).
   * GET /v1/batch/:id
   */
  async getBatchStatus(batchId: string): Promise<BatchStatusResponse> {
    return this.request<BatchStatusResponse>(
      "GET",
      `/v1/batch/${encodeURIComponent(batchId)}`,
      undefined,
      CINDER_TIMEOUT.batchStatus,
    );
  }

  /**
   * Extract links from a page (via scrape with include_links).
   * Reuses POST /v1/scrape — same SSRF/timeouts as scrape.
   */
  async links(url: string): Promise<ScrapeResult> {
    if (!validateUrl(url)) {
      throw new CinderError(
        "Invalid or blocked URL. Only HTTP(S) URLs to public hosts are allowed.",
        400,
        "",
      );
    }
    return this.request<ScrapeResult>(
      "POST",
      "/v1/scrape",
      { url, include_links: true },
      CINDER_TIMEOUT.links,
    );
  }

  /**
   * Create a change-tracking monitor (requires Redis).
   * POST /v1/monitor
   */
  async createMonitor(params: MonitorParams): Promise<MonitorResponse> {
    if (!validateUrl(params.url)) {
      throw new CinderError(
        "Invalid or blocked URL. Only HTTP(S) URLs to public hosts are allowed.",
        400,
        "",
      );
    }
    return this.request<MonitorResponse>(
      "POST",
      "/v1/monitor",
      params,
      CINDER_TIMEOUT.monitor,
    );
  }

  /**
   * Get monitor status (config + last hash + next check).
   * GET /v1/monitor/:id
   */
  async getMonitor(id: string): Promise<MonitorStatusResponse> {
    return this.request<MonitorStatusResponse>(
      "GET",
      `/v1/monitor/${encodeURIComponent(id)}`,
      undefined,
      CINDER_TIMEOUT.monitorStatus,
    );
  }

  /**
   * Stop monitoring and remove the monitor record.
   * DELETE /v1/monitor/:id
   */
  async deleteMonitor(id: string): Promise<void> {
    await this.request<void>(
      "DELETE",
      `/v1/monitor/${encodeURIComponent(id)}`,
      undefined,
      CINDER_TIMEOUT.monitorDelete,
    );
  }
}

// ---------------------------------------------------------------------------
// Custom Error
// ---------------------------------------------------------------------------

export class CinderError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
    public readonly body: string,
  ) {
    super(message);
    this.name = "CinderError";
  }
}
