import { describe, expect, test } from "bun:test";
import * as v from "valibot";
import { TomoshiJsonSchemaAdapter } from "./adapter.js";
import { num, bool, composite } from "./tools/schema.js";

const adapter = new TomoshiJsonSchemaAdapter();

describe("TomoshiJsonSchemaAdapter", () => {
  test("keeps a flat object flat and promotes string unions", async () => {
    const schema = v.object(
      {
        action: v.optional(
          v.pipe(v.picklist(["search", "crawl"]), v.description("action")),
        ),
        limit: v.optional(num("page limit", 1, 100)),
        render: v.optional(bool("render js")),
        urls: v.optional(composite(v.array(v.string()), "urls")),
      },
      "custom message",
    );
    const json = (await adapter.toJsonSchema(schema)) as any;

    expect(json.type).toBe("object");
    expect(json.oneOf).toBeUndefined();
    expect(json.anyOf).toBeUndefined();
    expect(json.required).toEqual(["action"]);
    expect(json.properties.action.enum).toEqual(["search", "crawl"]);
    expect(json.properties.limit).toMatchObject({
      type: "number",
      minimum: 1,
      maximum: 100,
      description: "page limit",
    });
    expect(json.properties.render).toMatchObject({
      type: "boolean",
      description: "render js",
    });
    expect(json.properties.urls).toMatchObject({
      type: "array",
      description: "urls",
    });
    expect(JSON.stringify(json.properties)).not.toContain("$schema");
  });

  test("keeps sibling keywords (default, description) when promoting", async () => {
    const schema = v.object(
      {
        action: v.optional(v.picklist(["create"])),
        interval: v.optional(
          v.union([
            v.pipe(v.number(), v.minValue(3600), v.description("interval")),
            v.string(),
          ]),
          3600,
        ),
      },
      "message",
    );
    const json = (await adapter.toJsonSchema(schema)) as any;
    expect(json.properties.interval).toMatchObject({
      type: "number",
      minimum: 3600,
      default: 3600,
      description: "interval",
    });
  });

  test("still forces type:object on a union root (legacy safety net)", async () => {
    const legacy = v.union([
      v.object({ a: v.string() }),
      v.object({ b: v.string() }),
    ]);
    const json = (await adapter.toJsonSchema(legacy)) as any;
    expect(json.type).toBe("object");
    expect(json.oneOf ?? json.anyOf).toBeDefined();
  });

  test("leaves ambiguous unions alone", async () => {
    const node: Record<string, unknown> = {
      mixed: {
        anyOf: [{ type: "string" }, { type: "number" }, { type: "boolean" }],
      },
      plain: { type: "string" },
    };
    const json = (await adapter.toJsonSchema(v.object({ x: v.string() }))) as any;
    // direct unit check on the walker
    const { promoteUnionTypes } = await import("./adapter.js");
    promoteUnionTypes(node);
    expect((node.mixed as any).anyOf).toBeDefined();
    expect(node.plain).toEqual({ type: "string" });
    expect(json.type).toBe("object");
  });
});
