import { describe, expect, test } from "bun:test";
import * as v from "valibot";
import { ValibotJsonSchemaAdapter } from "@tmcp/adapter-valibot";
import { TomoshiJsonSchemaAdapter } from "../adapter.js";
import {
  num,
  bool,
  composite,
  missingKeys,
  toolError,
  usageError,
  normalize,
  guardAction,
  flattenShapes,
} from "./schema.js";

const adapter = new ValibotJsonSchemaAdapter();

describe("field helpers", () => {
  test("num converts to JSON Schema and accepts numeric strings", async () => {
    const s = num("page limit", 1, 100);
    const json = (await adapter.toJsonSchema(s)) as any;
    expect(json.anyOf).toBeDefined();
    expect(
      json.anyOf.some(
        (b: { type?: string; minimum?: number; maximum?: number }) =>
          b.type === "number" && b.minimum === 1 && b.maximum === 100,
      ),
    ).toBe(true);
    expect(v.safeParse(s, 8)).toMatchObject({ success: true, output: 8 });
    expect(v.safeParse(s, "8")).toMatchObject({ success: true, output: "8" });
  });

  test("bool converts to JSON Schema and accepts boolean strings", async () => {
    const s = bool("capture screenshot");
    const json = (await adapter.toJsonSchema(s)) as any;
    expect(
      json.anyOf.some((b: { type?: string }) => b.type === "boolean"),
    ).toBe(true);
    expect(v.safeParse(s, true)).toMatchObject({ success: true, output: true });
    expect(v.safeParse(s, "true")).toMatchObject({
      success: true,
      output: "true",
    });
  });

  test("composite accepts arrays and JSON-encoded arrays", async () => {
    const s = composite(v.array(v.pipe(v.string(), v.url())), "URLs to scrape");
    const json = (await adapter.toJsonSchema(s)) as any;
    expect(json.anyOf.some((b: { type?: string }) => b.type === "array")).toBe(
      true,
    );
    expect(v.safeParse(s, ["https://a.com"])).toMatchObject({
      success: true,
      output: ["https://a.com"],
    });
    expect(v.safeParse(s, '["https://a.com"]')).toMatchObject({
      success: true,
      output: '["https://a.com"]',
    });
  });
});

describe("missingKeys", () => {
  test("treats undefined, empty string and empty array as missing", () => {
    expect(
      missingKeys({ a: undefined, b: "", c: [], d: "x", e: 0 }, [
        "a",
        "b",
        "c",
        "d",
        "e",
      ]),
    ).toEqual(["a", "b", "c"]);
  });

  test("returns empty list when everything is present", () => {
    expect(missingKeys({ query: "svelte" }, ["query"])).toEqual([]);
  });
});

describe("errors", () => {
  test("toolError sets isError and keeps the text", () => {
    const err = toolError("boom");
    expect(err.isError).toBe(true);
    expect(err.content[0]).toEqual({ type: "text", text: "boom" });
  });

  test("usageError names the action, the missing keys and an example", () => {
    const err = usageError({
      tool: "tomoshi_discover",
      action: "search",
      missing: ["query"],
      usage: "search → query, map → url, crawl → url, crawl_status → id",
      example: { action: "search", query: "svelte 5 runes" },
    });
    expect(err.isError).toBe(true);
    const text = err.content[0].text;
    expect(text).toContain('action "search"');
    expect(text).toContain("query");
    expect(text).toContain('{"action":"search","query":"svelte 5 runes"}');
  });
});

describe("normalize", () => {
  test("coerces numeric, boolean and JSON strings", () => {
    const { value, error } = normalize(
      { limit: "8", flag: "true", urls: '["https://a.com"]', keep: "x" },
      { numbers: ["limit"], booleans: ["flag"], json: ["urls"] },
    );
    expect(error).toBeUndefined();
    expect(value.limit).toBe(8);
    expect(value.flag).toBe(true);
    expect(value.urls).toEqual(["https://a.com"]);
    expect(value.keep).toBe("x");
  });

  test("leaves already-typed values untouched", () => {
    const { value, error } = normalize(
      { limit: 8, flag: false, urls: ["https://a.com"] },
      { numbers: ["limit"], booleans: ["flag"], json: ["urls"] },
    );
    expect(error).toBeUndefined();
    expect(value).toEqual({ limit: 8, flag: false, urls: ["https://a.com"] });
  });

  test("reports a readable error for non-numeric input", () => {
    const { error } = normalize({ limit: "abc" }, { numbers: ["limit"] });
    expect(error).toContain("limit");
    expect(error).toContain("abc");
  });

  test("reports a readable error for malformed JSON", () => {
    const { error } = normalize({ urls: "not-json" }, { json: ["urls"] });
    expect(error).toContain("urls");
  });

  test("ignores keys that are absent", () => {
    const { value, error } = normalize({}, { numbers: ["limit"] });
    expect(error).toBeUndefined();
    expect(value).toEqual({});
  });
});

describe("guardAction", () => {
  const actions = {
    search: {
      required: ["query"],
      example: { action: "search", query: "svelte 5 runes" },
    },
    map: {
      required: ["url"],
      example: { action: "map", url: "https://example.com" },
    },
  };
  const usage = "search → query, map → url";

  test("passes when the action and its required params are present", () => {
    expect(
      guardAction("demo", { action: "search", query: "x" }, actions, usage),
    ).toBeUndefined();
  });

  test("lists the valid actions when action is missing", () => {
    const err = guardAction("demo", {}, actions, usage);
    expect(err?.isError).toBe(true);
    expect(err?.content[0].text).toContain("search → query, map → url");
  });

  test("reports missing required params with an example", () => {
    const err = guardAction("demo", { action: "map" }, actions, usage);
    expect(err?.isError).toBe(true);
    expect(err?.content[0].text).toContain("url");
    expect(err?.content[0].text).toContain(
      '{"action":"map","url":"https://example.com"}',
    );
  });

  test("enforces the action's limit bound when one is set", () => {
    const bounded = {
      ...actions,
      search: { ...actions.search, limitMax: 100 },
    };
    const over = guardAction(
      "demo",
      { action: "search", query: "x", limit: 500 },
      bounded,
      usage,
    );
    expect(over?.content[0].text).toContain("≤ 100");
    expect(over?.content[0].text).toContain("500");
    expect(
      guardAction(
        "demo",
        { action: "search", query: "x", limit: 100 },
        bounded,
        usage,
      ),
    ).toBeUndefined();
    expect(
      guardAction(
        "demo",
        { action: "map", url: "https://example.com", limit: 500 },
        bounded,
        usage,
      ),
    ).toBeUndefined();
  });
});

describe("flattenShapes", () => {
  const adapter = new TomoshiJsonSchemaAdapter();
  const searchShape = v.object({
    action: v.picklist(["search"]),
    query: v.pipe(v.string(), v.minLength(1)),
    limit: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(100)), 10),
  });
  const mapShape = v.object({
    action: v.picklist(["map"]),
    url: v.pipe(v.string(), v.url()),
    limit: v.optional(v.pipe(v.number(), v.minValue(1), v.maxValue(5000)), 100),
  });

  test("merges shapes with only action required and no root union", async () => {
    const entries = flattenShapes([searchShape, mapShape], {
      action: v.optional(
        v.pipe(v.picklist(["search", "map"]), v.description("action")),
      ),
      lenient: ["limit"],
      overrides: { limit: v.optional(num("max results", 1, 5000)) },
    });
    const json = (await adapter.toJsonSchema(
      v.object(entries, "message"),
    )) as any;

    expect(json.type).toBe("object");
    expect(json.oneOf).toBeUndefined();
    expect(json.anyOf).toBeUndefined();
    expect(json.required).toEqual(["action"]);
    expect(Object.keys(json.properties).sort()).toEqual([
      "action",
      "limit",
      "query",
      "url",
    ]);
    expect(json.properties.limit).toMatchObject({
      type: "number",
      minimum: 1,
      maximum: 5000,
    });
    expect(json.properties.limit).not.toHaveProperty("default");
  });

  test("keeps per-field defaults through the unwrap/rewrap", () => {
    const entries = flattenShapes([searchShape], {
      action: v.optional(v.picklist(["search"])),
      lenient: ["limit"],
    });
    const parsed = v.safeParse(v.object(entries, "message"), {
      action: "search",
      query: "x",
    });
    expect(parsed.success).toBe(true);
    expect((parsed.output as Record<string, unknown>).limit).toBe(10);
  });

  test("makes branch-required fields optional (per-action rules live in handlers)", () => {
    const entries = flattenShapes([searchShape, mapShape], {
      action: v.optional(v.picklist(["search", "map"])),
    });
    const schema = v.object(entries, "message");
    expect(v.safeParse(schema, { action: "map" }).success).toBe(true);
    expect(v.safeParse(schema, {}).success).toBe(true);
  });
});
