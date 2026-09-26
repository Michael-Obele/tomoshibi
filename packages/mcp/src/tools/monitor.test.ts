import { describe, expect, test } from "bun:test";
import { TomoshiJsonSchemaAdapter } from "../adapter.js";
import { MonitorSchema, createMonitorHandler } from "./monitor.js";
import type { TomoshiClient } from "../client.js";

const adapter = new TomoshiJsonSchemaAdapter();

type Result = { content: { type: string; text: string }[]; isError?: boolean };

/** Stub client that records calls so tests can assert no network happened. */
function stubClient() {
  const calls: { method: string; params: unknown }[] = [];
  const client = {
    async createMonitor(params: Record<string, unknown>) {
      calls.push({ method: "createMonitor", params });
      return {
        id: "mon_1",
        url: String(params.url),
        interval_seconds: params.interval_seconds,
        next_check: "2026-09-27T00:00:00Z",
      };
    },
    async getMonitor(id: string) {
      calls.push({ method: "getMonitor", params: id });
      return {
        id,
        url: "https://example.com",
        interval_seconds: 3600,
        next_check: "2026-09-27T00:00:00Z",
      };
    },
    async deleteMonitor(id: string) {
      calls.push({ method: "deleteMonitor", params: id });
      return { success: true };
    },
  };
  return { client: client as unknown as TomoshiClient, calls };
}

describe("tomoshi_monitor schema", () => {
  test("emits a flat object schema with action required", async () => {
    const json = (await adapter.toJsonSchema(MonitorSchema)) as any;
    expect(json.type).toBe("object");
    expect(json.oneOf).toBeUndefined();
    expect(json.anyOf).toBeUndefined();
    expect(json.required).toEqual(["action"]);
    expect(json.properties.action.enum).toEqual(["create", "status", "delete"]);
    expect(json.properties.url).toMatchObject({ type: "string" });
    expect(json.properties.id).toMatchObject({ type: "string" });
    expect(json.properties.interval_seconds).toMatchObject({
      type: "number",
      minimum: 3600,
      default: 3600,
    });
    expect(json.properties.action.description).toContain("url");
  });
});

describe("tomoshi_monitor handler", () => {
  test("returns a friendly usage error for empty arguments", async () => {
    const { client, calls } = stubClient();
    const res = (await createMonitorHandler(client)({})) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("Valid actions");
    expect(res.content[0].text).toContain("create");
    expect(calls).toHaveLength(0);
  });

  test("returns a usage error when status has no id", async () => {
    const { client, calls } = stubClient();
    const res = (await createMonitorHandler(client)({
      action: "status",
    })) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("id");
    expect(res.content[0].text).toContain('"action":"status"');
    expect(calls).toHaveLength(0);
  });

  test("returns a usage error when create has no url", async () => {
    const { client, calls } = stubClient();
    const res = (await createMonitorHandler(client)({
      action: "create",
    })) as Result;
    expect(res.isError).toBe(true);
    expect(res.content[0].text).toContain("url");
    expect(calls).toHaveLength(0);
  });

  test("coerces the stringified interval before calling the API", async () => {
    const { client, calls } = stubClient();
    const res = (await createMonitorHandler(client)({
      action: "create",
      url: "https://example.com/pricing",
      interval_seconds: "7200",
    })) as Result;
    expect(res.isError).toBeUndefined();
    expect(calls).toHaveLength(1);
    expect((calls[0].params as Record<string, unknown>).interval_seconds).toBe(
      7200,
    );
  });
});
