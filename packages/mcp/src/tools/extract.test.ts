import { describe, expect, test } from "bun:test";
import { TomoshiJsonSchemaAdapter } from "../adapter.js";
import { ExtractSchema, createExtractHandler } from "./extract.js";
import type { TomoshiClient } from "../client.js";

const adapter = new TomoshiJsonSchemaAdapter();

type Result = { content: { type: string; text: string }[]; isError?: boolean };

/** Stub client that records calls so tests can assert no network happened. */
function stubClient() {
  const calls: { method: string; params: unknown }[] = [];
  const client = {
    async scrape(params: Record<string, unknown>) {
      calls.push({ method: "scrape", params });
      return {
        url: String(params.url),
        markdown: "# Hello",
        images: [],
        links: [],
      };
    },
    async scrapeMulti(params: { urls: string[] }) {
      calls.push({ method: "scrapeMulti", params });
      return {
        results: params.urls.map((url) => ({ url, markdown: `# ${url}` })),
      };
    },
    async links(url: string) {
      calls.push({ method: "links", params: url });
      return { url, links: [] };
    },
    async batchScrape(params: { urls: string[] }) {
      calls.push({ method: "batchScrape", params });
      return {
        batch_id: "batch_1",
        tasks: params.urls.map((url, i) => ({ id: `t${i}`, url })),
      };
    },
    async getBatchStatus(batchId: string) {
      calls.push({ method: "getBatchStatus", params: batchId });
      return {
        batch_id: batchId,
        total: 0,
        completed: 0,
        failed: 0,
        tasks: [],
      };
    },
  };
  return { client: client as unknown as TomoshiClient, calls };
}

describe("tomoshi_extract schema", () => {
  test("emits a flat object schema with action required", async () => {
    const json = (await adapter.toJsonSchema(ExtractSchema)) as any;
    expect(json.type).toBe("object");
    expect(json.oneOf).toBeUndefined();
    expect(json.anyOf).toBeUndefined();
    expect(json.required).toEqual(["action"]);
    expect(json.properties.action.enum).toEqual([
      "scrape",
      "scrape_multi",
      "links",
      "batch",
      "batch_status",
    ]);
    expect(json.properties.url).toMatchObject({ type: "string" });
    expect(json.properties.urls).toMatchObject({ type: "array" });
    expect(json.properties.max_images).toMatchObject({ type: "number" });
    expect(json.properties.summary).toMatchObject({ type: "boolean" });
    expect(json.properties.batch_id).toMatchObject({ type: "string" });
    expect(json.properties.action.description).toContain("url");
  });

  test("does not require per-action params at the schema level", async () => {
    const json = (await adapter.toJsonSchema(ExtractSchema)) as any;
    expect(json.required).toEqual(["action"]);
  });
});

describe("tomoshi_extract handler", () => {
  test("returns a friendly usage error for empty arguments", async () => {
    const { client, calls } = stubClient();
    const res = (await createExtractHandler(client)({})) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("Valid actions");
    expect(res.content[0].text).toContain("batch_status");
    expect(calls).toHaveLength(0);
  });

  test("returns a usage error when the action's url is missing", async () => {
    const { client, calls } = stubClient();
    const res = (await createExtractHandler(client)({
      action: "scrape",
    })) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("url");
    expect(res.content[0].text).toContain('"action":"scrape"');
    expect(calls).toHaveLength(0);
  });

  test("returns a usage error when batch has no urls", async () => {
    const { client, calls } = stubClient();
    const res = (await createExtractHandler(client)({
      action: "batch",
    })) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("urls");
    expect(calls).toHaveLength(0);
  });

  test("coerces stringified numbers before calling the API", async () => {
    const { client, calls } = stubClient();
    const res = (await createExtractHandler(client)({
      action: "scrape",
      url: "https://example.com",
      max_images: "5",
      summary: "true",
    })) as Result;
    expect(res.isError).toBeUndefined();
    expect(calls).toHaveLength(1);
    const params = calls[0].params as Record<string, unknown>;
    expect(params.max_images).toBe(5);
    expect(params.summary).toBe(true);
  });

  test("parses JSON-encoded urls for the async batch action", async () => {
    const { client, calls } = stubClient();
    const res = (await createExtractHandler(client)({
      action: "batch",
      urls: '["https://a.com", "https://b.com"]',
    })) as Result;
    expect(res.isError).toBeUndefined();
    expect(calls[0].method).toBe("batchScrape");
    expect((calls[0].params as { urls: string[] }).urls).toEqual([
      "https://a.com",
      "https://b.com",
    ]);
  });
});
