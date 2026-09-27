import { describe, expect, test } from "bun:test";
import * as v from "valibot";
import { TomoshiJsonSchemaAdapter } from "../adapter.js";
import { DiscoverSchema, createDiscoverHandler } from "./discover.js";
import type { TomoshiClient } from "../client.js";

const adapter = new TomoshiJsonSchemaAdapter();

type Result = { content: { type: string; text: string }[]; isError?: boolean };

/** Stub client that records calls so tests can assert no network happened. */
function stubClient() {
  const calls: { method: string; params: unknown }[] = [];
  const client = {
    async search(params: Record<string, unknown>) {
      calls.push({ method: "search", params });
      return {
        query: String(params.query),
        count: 0,
        hasMore: false,
        results: [],
      };
    },
    async map(params: Record<string, unknown>) {
      calls.push({ method: "map", params });
      return { url: String(params.url), count: 0, links: [] };
    },
    async crawl(params: Record<string, unknown>) {
      calls.push({ method: "crawl", params });
      return {
        url: String(params.url),
        id: "crawl_1",
        render: false,
        screenshot: false,
        images: false,
        maxDepth: 2,
        limit: 10,
      };
    },
    async getCrawlStatus(id: string) {
      calls.push({ method: "getCrawlStatus", params: id });
      return {
        id,
        state: "completed",
        crawl: {
          status: "completed",
          total_pages: 1,
          max_depth: 2,
          limit: 10,
          pages: [{ title: "Doc", url: "https://example.com/" }],
          failed_urls: [],
        },
      };
    },
  };
  return { client: client as unknown as TomoshiClient, calls };
}

describe("tomoshi_discover schema", () => {
  test("emits a flat object schema with action required", async () => {
    const json = (await adapter.toJsonSchema(DiscoverSchema)) as any;
    expect(json.type).toBe("object");
    expect(json.oneOf).toBeUndefined();
    expect(json.anyOf).toBeUndefined();
    expect(json.required).toEqual(["action"]);
    expect(json.properties.action.enum).toEqual([
      "search",
      "map",
      "crawl",
      "crawl_status",
    ]);
    expect(json.properties.query).toMatchObject({ type: "string" });
    expect(json.properties.limit).toMatchObject({
      type: "number",
      minimum: 1,
      maximum: 5000,
    });
    expect(json.properties.limit).not.toHaveProperty("default");
    expect(json.properties.rerank).toMatchObject({ type: "boolean" });
    expect(json.properties.action.description).toContain("query");
  });

  test("action stays required in the emitted schema", async () => {
    const json = (await adapter.toJsonSchema(DiscoverSchema)) as any;
    expect(json.required).toEqual(["action"]);
    expect(v.safeParse(DiscoverSchema, {}).success).toBe(true);
  });
});

describe("tomoshi_discover handler", () => {
  test("returns a friendly usage error for empty arguments", async () => {
    const { client, calls } = stubClient();
    const res = (await createDiscoverHandler(client)({})) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("Valid actions");
    expect(res.content[0].text).toContain("crawl_status");
    expect(calls).toHaveLength(0);
  });

  test("returns a usage error when the required param is missing", async () => {
    const { client, calls } = stubClient();
    const res = (await createDiscoverHandler(client)({
      action: "search",
    })) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("query");
    expect(calls).toHaveLength(0);
  });

  test("rejects unknown actions with the valid action list", async () => {
    const { client, calls } = stubClient();
    const res = (await createDiscoverHandler(client)({
      action: "nope",
    })) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("crawl_status");
    expect(calls).toHaveLength(0);
  });

  test("coerces stringified numbers before calling the API", async () => {
    const { client, calls } = stubClient();
    const res = (await createDiscoverHandler(client)({
      action: "search",
      query: "svelte 5 runes",
      limit: "8",
    })) as Result;
    expect(res.isError).toBeUndefined();
    expect(calls).toHaveLength(1);
    expect((calls[0].params as Record<string, unknown>).limit).toBe(8);
  });

  test("reports a readable error for a non-numeric limit", async () => {
    const { client, calls } = stubClient();
    const res = (await createDiscoverHandler(client)({
      action: "search",
      query: "x",
      limit: "abc",
    })) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("limit");
    expect(calls).toHaveLength(0);
  });

  test("parses JSON-encoded path globs", async () => {
    const { client, calls } = stubClient();
    const res = (await createDiscoverHandler(client)({
      action: "crawl",
      url: "https://example.com",
      include_paths: '["/docs/*"]',
    })) as Result;
    expect(res.isError).toBeUndefined();
    expect((calls[0].params as Record<string, unknown>).include_paths).toEqual([
      "/docs/*",
    ]);
  });

  test("rejects a limit above the action's bound with a readable error", async () => {
    const { client, calls } = stubClient();
    const res = (await createDiscoverHandler(client)({
      action: "search",
      query: "x",
      limit: 500,
    })) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("≤ 100");
    expect(res.content[0].text).toContain("500");
    expect(calls).toHaveLength(0);
  });

  test("allows map's larger limit", async () => {
    const { client, calls } = stubClient();
    const res = (await createDiscoverHandler(client)({
      action: "map",
      url: "https://example.com",
      limit: 4000,
    })) as Result;
    expect(res.isError).toBeUndefined();
    expect(calls).toHaveLength(1);
    expect(calls[0].method).toBe("map");
  });
});
