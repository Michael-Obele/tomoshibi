import * as v from "valibot";
import type {
  CinderClient,
  ScrapeParams,
  MultiScrapeParams,
} from "../client.js";

/**
 * Resource-oriented multiplexed extraction tool.
 * Consolidates `cinder_scrape` + `cinder_links` + `cinder_batch_scrape`
 * into a single `cinder_extract` resource (5→1 with scrape_multi).
 *
 * Principle: 1 tool per domain resource with `action` enum
 * (arch.md "Why 7 Tools Instead of 17" — FlarelyLegal 17→7).
 * Reduces token overhead, improves LLM tool-selection accuracy,
 * stays well under MCP client limits.
 *
 * Actions:
 * - `scrape`       — single page → markdown (POST /v1/scrape)
 * - `scrape_multi` — sync multi-URL scrape, no Redis (POST /v1/scrape {urls}, max 10, errgroup 5)
 * - `links`        — hyperlinks only (POST /v1/scrape {include_links})
 * - `batch`        — enqueue up to 20 URLs async (POST /v1/batch/scrape, Redis)
 * - `batch_status` — poll batch (GET /v1/batch/:id, Redis)
 */

// ── scrape (single) ──────────────────────────────────────────────────
const ExtractScrapeShape = v.object({
  action: v.pipe(
    v.picklist(["scrape"]),
    v.description("Scrape a single page into markdown"),
  ),
  url: v.pipe(
    v.string(),
    v.description("The URL to scrape"),
    v.url("Must be a valid URL"),
  ),
  mode: v.optional(
    v.pipe(
      v.picklist(["smart", "static", "dynamic"]),
      v.description(
        "Scraping mode: smart (auto), static (colly), dynamic (chromedp)",
      ),
    ),
    "smart",
  ),
  screenshot: v.optional(
    v.pipe(
      v.boolean(),
      v.description("Capture a screenshot (needs dynamic/smart)"),
    ),
    false,
  ),
  screenshot_opts: v.optional(
    v.pipe(
      v.object({
        width: v.optional(v.number()),
        height: v.optional(v.number()),
        full_page: v.optional(v.boolean()),
        format: v.optional(v.picklist(["jpeg", "png"])),
        quality: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(100))),
        wait_selector: v.optional(v.string()),
      }),
      v.description(
        "Screenshot configuration (width, height, full_page, format, quality, wait_selector)",
      ),
    ),
  ),
  images: v.optional(
    v.pipe(v.boolean(), v.description("Extract images from the page")),
    false,
  ),
  image_format: v.optional(
    v.pipe(
      v.picklist(["url", "blob"]),
      v.description(
        "Image transport: 'url' (metadata only) or 'blob' (base64)",
      ),
    ),
    "url",
  ),
  max_images: v.optional(
    v.pipe(
      v.number(),
      v.minValue(1),
      v.maxValue(50),
      v.description("Max images to extract (default 10)"),
    ),
    10,
  ),
  max_image_size_kb: v.optional(
    v.pipe(
      v.number(),
      v.minValue(1),
      v.description("Max individual image size in KB (default 5120)"),
    ),
    5120,
  ),
  image_process: v.optional(
    v.pipe(
      v.object({
        format: v.optional(v.picklist(["jpeg", "png"])),
        max_width: v.optional(v.number()),
        quality: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(100))),
      }),
      v.description(
        "Resize/re-encode image blobs (format, max_width, quality)",
      ),
    ),
  ),
  actions: v.optional(
    v.pipe(
      v.array(
        v.object({
          type: v.picklist([
            "wait_ms",
            "wait_selector",
            "click",
            "scroll_down",
            "scroll_to_bottom",
          ]),
          ms: v.optional(v.number()),
          selector: v.optional(v.string()),
        }),
      ),
      v.description("Page interactions before capture (dynamic mode only)"),
    ),
  ),
  extract_schema: v.optional(
    v.pipe(
      v.record(
        v.string(),
        v.object({
          selector: v.string(),
          attr: v.optional(v.string()),
          multiple: v.optional(v.boolean()),
        }),
      ),
      v.description(
        "Deterministic CSS-selector extraction: {field: {selector, attr?, multiple?}}",
      ),
    ),
  ),
  summary: v.optional(
    v.pipe(v.boolean(), v.description("Return an extractive summary (no LLM)")),
    false,
  ),
  summary_sentences: v.optional(
    v.pipe(
      v.number(),
      v.minValue(1),
      v.maxValue(50),
      v.description("Sentence count for the summary (default 5)"),
    ),
    5,
  ),
  redact_pii: v.optional(
    v.pipe(
      v.boolean(),
      v.description("Mask emails, phones, and card-shaped numbers"),
    ),
    false,
  ),
  block_ads: v.optional(
    v.pipe(
      v.boolean(),
      v.description("Strip ad/tracker containers before conversion"),
    ),
    true,
  ),
  remove_base64_images: v.optional(
    v.pipe(
      v.boolean(),
      v.description("Drop inline data: images before conversion"),
    ),
    true,
  ),
  include_links: v.optional(
    v.pipe(
      v.boolean(),
      v.description(
        'Include extracted links (default true, Firecrawl formats: ["links"] parity — enriched to {url, text, isInternal})',
      ),
    ),
    true,
  ),
  render: v.optional(
    v.pipe(v.boolean(), v.description("Deprecated: behaves like mode=dynamic")),
    false,
  ),
});

// ── scrape_multi (sync, no Redis) ─────────────────────────────────────
const ExtractMultiScrapeShape = v.object({
  action: v.pipe(
    v.picklist(["scrape_multi"]),
    v.description(
      "Scrape multiple URLs synchronously (max 10, no Redis) — mirrors web_fetch_exa & Firecrawl batch sync",
    ),
  ),
  urls: v.pipe(
    v.array(v.pipe(v.string(), v.url("Must be a valid URL"))),
    v.minLength(1, "At least one URL is required"),
    v.maxLength(10, "Maximum 10 URLs per sync batch"),
    v.description(
      "URLs to scrape synchronously (max 10, no Redis, errgroup limit 5)",
    ),
  ),
  mode: v.optional(
    v.pipe(
      v.picklist(["smart", "static", "dynamic"]),
      v.description(
        "Scraping mode: smart (auto), static (colly), dynamic (chromedp)",
      ),
    ),
    "smart",
  ),
  screenshot: v.optional(
    v.pipe(v.boolean(), v.description("Capture a screenshot")),
    false,
  ),
  screenshot_opts: v.optional(
    v.pipe(
      v.object({
        width: v.optional(v.number()),
        height: v.optional(v.number()),
        full_page: v.optional(v.boolean()),
        format: v.optional(v.picklist(["jpeg", "png"])),
        quality: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(100))),
        wait_selector: v.optional(v.string()),
      }),
      v.description("Screenshot configuration"),
    ),
  ),
  images: v.optional(
    v.pipe(v.boolean(), v.description("Extract images")),
    false,
  ),
  image_format: v.optional(
    v.pipe(
      v.picklist(["url", "blob"]),
      v.description("Image transport: url or blob"),
    ),
    "url",
  ),
  max_images: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(50)), 10),
  max_image_size_kb: v.optional(v.pipe(v.number(), v.minValue(1)), 5120),
  image_process: v.optional(
    v.pipe(
      v.object({
        format: v.optional(v.picklist(["jpeg", "png"])),
        max_width: v.optional(v.number()),
        quality: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(100))),
      }),
      v.description("Resize/re-encode image blobs"),
    ),
  ),
  actions: v.optional(
    v.pipe(
      v.array(
        v.object({
          type: v.picklist([
            "wait_ms",
            "wait_selector",
            "click",
            "scroll_down",
            "scroll_to_bottom",
          ]),
          ms: v.optional(v.number()),
          selector: v.optional(v.string()),
        }),
      ),
      v.description("Page interactions before capture (dynamic mode only)"),
    ),
  ),
  extract_schema: v.optional(
    v.pipe(
      v.record(
        v.string(),
        v.object({
          selector: v.string(),
          attr: v.optional(v.string()),
          multiple: v.optional(v.boolean()),
        }),
      ),
      v.description("Deterministic CSS-selector extraction"),
    ),
  ),
  summary: v.optional(
    v.pipe(v.boolean(), v.description("Return an extractive summary")),
    false,
  ),
  summary_sentences: v.optional(
    v.pipe(v.number(), v.minValue(1), v.maxValue(50)),
    5,
  ),
  redact_pii: v.optional(
    v.pipe(v.boolean(), v.description("Mask emails, phones, card numbers")),
    false,
  ),
  block_ads: v.optional(
    v.pipe(v.boolean(), v.description("Strip ad/tracker containers")),
    true,
  ),
  remove_base64_images: v.optional(
    v.pipe(v.boolean(), v.description("Drop inline data: images")),
    true,
  ),
  include_links: v.optional(
    v.pipe(
      v.boolean(),
      v.description("Include extracted links (default true)"),
    ),
    true,
  ),
  render: v.optional(
    v.pipe(v.boolean(), v.description("Deprecated: behaves like mode=dynamic")),
    false,
  ),
});

// ── links ──────────────────────────────────────────────────────────────
const ExtractLinksShape = v.object({
  action: v.pipe(
    v.picklist(["links"]),
    v.description("Extract hyperlinks only (lightweight, no markdown)"),
  ),
  url: v.pipe(
    v.string(),
    v.description("The URL to extract hyperlinks from"),
    v.url("Must be a valid URL"),
  ),
});

// ── batch (enqueue) ────────────────────────────────────────────────────
const ExtractBatchShape = v.object({
  action: v.pipe(
    v.picklist(["batch"]),
    v.description("Enqueue async batch scrape (max 20 URLs, requires Redis)"),
  ),
  urls: v.pipe(
    v.array(v.pipe(v.string(), v.url("Must be a valid URL"))),
    v.minLength(1, "At least one URL is required"),
    v.maxLength(20, "Maximum 20 URLs per batch"),
    v.description("URLs to scrape asynchronously (max 20, requires Redis)"),
  ),
});

// ── batch_status (poll) ────────────────────────────────────────────────
const ExtractBatchStatusShape = v.object({
  action: v.pipe(
    v.picklist(["batch_status"]),
    v.description("Poll async batch status"),
  ),
  batch_id: v.pipe(
    v.string(),
    v.minLength(1, "Batch ID is required"),
    v.description("The batch_id returned by cinder_extract action=batch"),
  ),
});

export const ExtractSchema = v.variant("action", [
  ExtractScrapeShape,
  ExtractMultiScrapeShape,
  ExtractLinksShape,
  ExtractBatchShape,
  ExtractBatchStatusShape,
]);
export type ExtractInput = v.InferOutput<typeof ExtractSchema>;

export function createExtractHandler(client: CinderClient) {
  return async (input: Record<string, unknown>) => {
    const { action } = input as { action: string };
    try {
      if (action === "scrape") {
        const params = input as unknown as ScrapeParams & { action: string };
        const result = await client.scrape(params);
        const titleMatch = result.markdown.match(/^#\s+(.+)/m);
        const title = titleMatch ? titleMatch[1].trim() : result.url;
        const wordCount = result.markdown.split(/\s+/).length;
        const lines: string[] = [
          `# Scrape Result: ${title}`,
          "",
          `**URL:** ${result.url}`,
          `**Words:** ${wordCount.toLocaleString()}`,
        ];
        if (result.summary) lines.push("", "## Summary", "", result.summary);
        if (result.extracted && Object.keys(result.extracted).length > 0) {
          lines.push("", "## Extracted", "");
          for (const [k, val] of Object.entries(result.extracted))
            lines.push(
              `- **${k}:** ${typeof val === "string" ? val : JSON.stringify(val)}`,
            );
        }
        lines.push("", "---", "", result.markdown);
        if (result.images && result.images.length > 0) {
          lines.push("", "---", "", `## Images (${result.images.length})`, "");
          for (const img of result.images) {
            const label =
              img.alt || img.title || img.source || img.url || "image";
            if (img.url) lines.push(`- [${label}](${img.url})`);
            else if (img.blob)
              lines.push(
                `- ${label} (base64 ${img.format ?? ""}${img.size_bytes ? `, ${img.size_bytes} bytes` : ""})`,
              );
            else lines.push(`- ${label}`);
          }
        }
        if (result.screenshot) {
          lines.push("", "## Screenshot", "");
          if (result.screenshot.url)
            lines.push(`- [screenshot](${result.screenshot.url})`);
          else if (result.screenshot.blob)
            lines.push(
              `- screenshot captured (base64 ${result.screenshot.format ?? ""}${result.screenshot.size_bytes ? `, ${result.screenshot.size_bytes} bytes` : ""})`,
            );
        }
        if (result.metadata && Object.keys(result.metadata).length > 0) {
          lines.push("", "## Metadata", "");
          for (const [k, val] of Object.entries(result.metadata))
            lines.push(`- **${k}:** ${val}`);
        }
        if (result.links && result.links.length > 0) {
          lines.push(
            "",
            "## Links",
            `Found ${result.links.length} link(s):`,
            "",
          );
          const toShow = result.links.slice(0, 50);
          for (const link of toShow) {
            const label = link.text ? ` — "${link.text.slice(0, 80)}"` : "";
            const scope = link.isInternal ? "internal" : "external";
            lines.push(`- ${link.url}${label} _(${scope})_`);
          }
          if (result.links.length > 50)
            lines.push(`- ... and ${result.links.length - 50} more`);
        }
        return { content: [{ type: "text" as const, text: lines.join("\n") }] };
      }

      if (action === "scrape_multi") {
        const params = input as unknown as MultiScrapeParams & {
          action: string;
        };
        const result = await client.scrapeMulti(params);
        const lines: string[] = [
          `# Multi-Scrape Results (${result.results.length} URLs)`,
          "",
        ];
        for (let i = 0; i < result.results.length; i++) {
          const item = result.results[i];
          if (item.error) {
            lines.push(
              `## Result ${i + 1}: ❌ ${item.url}`,
              "",
              `**Error:** ${item.error}`,
              "",
              "---",
              "",
            );
            continue;
          }
          const title = item.title || item.url;
          const wc =
            item.word_count != null
              ? `**Words:** ${item.word_count.toLocaleString()}`
              : "";
          lines.push(
            `## Result ${i + 1}: ${title}`,
            "",
            `**URL:** ${item.url}`,
          );
          if (wc) lines.push(wc);
          if (item.summary) lines.push("", "### Summary", "", item.summary);
          if (item.extracted && Object.keys(item.extracted).length > 0) {
            lines.push("", "### Extracted", "");
            for (const [k, val] of Object.entries(item.extracted))
              lines.push(
                `- **${k}:** ${typeof val === "string" ? val : JSON.stringify(val)}`,
              );
          }
          lines.push("", "---", "", item.markdown ?? "_(no markdown)_");
          if (item.images && item.images.length > 0) {
            lines.push("", `### Images (${item.images.length})`, "");
            for (const img of item.images) {
              const label =
                img.alt || img.title || img.source || img.url || "image";
              if (img.url) lines.push(`- [${label}](${img.url})`);
              else if (img.blob)
                lines.push(
                  `- ${label} (base64 ${img.format ?? ""}${img.size_bytes ? `, ${img.size_bytes} bytes` : ""})`,
                );
              else lines.push(`- ${label}`);
            }
          }
          if (item.screenshot) {
            lines.push("", "### Screenshot", "");
            if (item.screenshot.url)
              lines.push(`- [screenshot](${item.screenshot.url})`);
            else if (item.screenshot.blob)
              lines.push(
                `- screenshot captured (base64 ${item.screenshot.format ?? ""}${item.screenshot.size_bytes ? `, ${item.screenshot.size_bytes} bytes` : ""})`,
              );
          }
          if (item.metadata && Object.keys(item.metadata).length > 0) {
            lines.push("", "### Metadata", "");
            for (const [k, val] of Object.entries(item.metadata))
              lines.push(`- **${k}:** ${val}`);
          }
          if (item.links && item.links.length > 0) {
            lines.push("", `### Links (${item.links.length})`, "");
            for (const link of item.links.slice(0, 20)) {
              const label = link.text ? ` — "${link.text.slice(0, 60)}"` : "";
              lines.push(
                `- ${link.url}${label} _(${link.isInternal ? "internal" : "external"})_`,
              );
            }
            if (item.links.length > 20)
              lines.push(`- ... and ${item.links.length - 20} more`);
          }
          lines.push("", "---", "");
        }
        return { content: [{ type: "text" as const, text: lines.join("\n") }] };
      }

      if (action === "links") {
        const { url } = input as { url: string };
        const result = await client.links(url);
        const links = result.links ?? [];
        const lines: string[] = [
          `# Links: ${url}`,
          "",
          `**Found:** ${links.length} link(s)`,
          `**Source:** ${result.url}`,
          "",
          "---",
          "",
        ];
        if (links.length === 0) lines.push("No links found on this page.");
        else {
          const internal = links.filter((l) => l.isInternal);
          const external = links.filter((l) => !l.isInternal);
          if (internal.length > 0) {
            lines.push(`## Internal (${internal.length})`, "");
            for (const l of internal)
              lines.push(`- ${l.url}${l.text ? ` — "${l.text}"` : ""}`);
            lines.push("");
          }
          if (external.length > 0) {
            lines.push(`## External (${external.length})`, "");
            for (const l of external)
              lines.push(`- ${l.url}${l.text ? ` — "${l.text}"` : ""}`);
            lines.push("");
          }
          if (internal.length === 0 && external.length === 0)
            for (const l of links)
              lines.push(
                `- ${l.url}${l.text ? ` — "${l.text}"` : ""} (isInternal: ${l.isInternal})`,
              );
        }
        return { content: [{ type: "text" as const, text: lines.join("\n") }] };
      }

      if (action === "batch") {
        const { urls } = input as { urls: string[] };
        const result = await client.batchScrape({ urls });
        const lines: string[] = [
          "# Batch Scrape Enqueued",
          "",
          `**Batch ID:** \`${result.batch_id}\``,
          `**Total:** ${result.tasks.length}`,
          "",
          "## Tasks",
          "",
        ];
        for (const t of result.tasks) lines.push(`- \`${t.id}\` — ${t.url}`);
        lines.push(
          "",
          "---",
          "",
          `Use \`cinder_extract\` with \`action: "batch_status"\` and \`batch_id: "${result.batch_id}"\` to poll.`,
          "Requires Redis-backed Cinder instance.",
        );
        return { content: [{ type: "text" as const, text: lines.join("\n") }] };
      }

      // batch_status
      const { batch_id } = input as { batch_id: string };
      const result = await client.getBatchStatus(batch_id);
      const lines: string[] = [
        "# Batch Status",
        "",
        `**Batch ID:** \`${result.batch_id}\``,
        `**Total:** ${result.total} | **Completed:** ${result.completed} | **Failed:** ${result.failed}`,
        "",
      ];
      if (result.tasks.length > 0) {
        lines.push("## Tasks", "");
        for (const t of result.tasks) lines.push(`- \`${t.id}\` — ${t.url}`);
      }
      if (result.completed === result.total && result.failed === 0)
        lines.push("", "✅ All tasks completed.");
      else if (result.failed > 0)
        lines.push("", `⚠️ ${result.failed} task(s) failed.`);
      else lines.push("", "⏳ Batch still in progress — poll again.");
      return { content: [{ type: "text" as const, text: lines.join("\n") }] };
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      return {
        content: [
          {
            type: "text" as const,
            text: `Extract ${action} failed: ${message}`,
          },
        ],
        isError: true,
      };
    }
  };
}
