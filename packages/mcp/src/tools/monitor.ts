import * as v from "valibot";
import type { TomoshiClient } from "../client.js";
import {
  flattenShapes,
  guardAction,
  normalize,
  toolError,
  type ActionSpec,
} from "./schema.js";

/**
 * Schema for the tomoshi_monitor tool.
 * A single tool that creates, checks, or deletes a change-tracking monitor,
 * selected by the `action` discriminator (mirrors handlers.MonitorRequest).
 */
const MonitorCreateShape = v.object({
  action: v.literal("create"),
  url: v.pipe(
    v.string(),
    v.description("The URL to monitor for content changes"),
    v.url("Must be a valid URL"),
  ),
  interval_seconds: v.optional(
    v.pipe(
      v.number(),
      v.minValue(3600),
      v.description("Check interval in seconds (minimum 3600 = 1h)"),
    ),
    3600,
  ),
  webhook_url: v.optional(
    v.pipe(
      v.string(),
      v.description("POST a change notification here when content changes"),
    ),
  ),
  webhook_secret: v.optional(
    v.pipe(
      v.string(),
      v.description("HMAC-SHA256 key for the X-Tomoshibi-Signature header"),
    ),
  ),
});

const MonitorStatusShape = v.object({
  action: v.literal("status"),
  id: v.pipe(
    v.string(),
    v.description("The monitor ID to check"),
    v.minLength(1, "Monitor ID is required"),
  ),
});

const MonitorDeleteShape = v.object({
  action: v.literal("delete"),
  id: v.pipe(
    v.string(),
    v.description("The monitor ID to delete"),
    v.minLength(1, "Monitor ID is required"),
  ),
});

/**
 * Flat change-tracking schema: ONE object, with `action` forced into the
 * required list of the emitted JSON Schema (see `discover.ts` for the why).
 * Per-action required fields are enforced by MONITOR_ACTIONS in the handler.
 */
export const MonitorSchema = v.object(
  flattenShapes(
    [MonitorCreateShape, MonitorStatusShape, MonitorDeleteShape],
    {
      action: v.optional(
        v.pipe(
          v.picklist(["create", "status", "delete"]),
          v.description(
            "Change-tracking action — required. create needs `url`; status needs `id`; delete needs `id`",
          ),
        ),
      ),
      // fields that may arrive stringified from untyped MCP clients
      lenient: ["interval_seconds"],
      overrides: {
        // per-action required fields — enforced by MONITOR_ACTIONS below
        url: v.optional(
          v.pipe(
            v.string(),
            v.description("The URL to monitor for content changes (action=create)"),
            v.url("Must be a valid URL"),
          ),
        ),
        id: v.optional(
          v.pipe(
            v.string(),
            v.description("Monitor ID from action=create (action=status|delete)"),
            v.minLength(1, "Monitor ID is required"),
          ),
        ),
      },
    },
  ),
  'tomoshi_monitor requires "action": create | status | delete — e.g. {"action":"create","url":"https://example.com/pricing"}',
);

export type MonitorInput = v.InferOutput<typeof MonitorSchema>;

/** Per-action required parameters, quoted verbatim in handler errors. */
const MONITOR_ACTIONS: Record<string, ActionSpec> = {
  create: {
    required: ["url"],
    example: {
      action: "create",
      url: "https://example.com/pricing",
      interval_seconds: 3600,
    },
  },
  status: {
    required: ["id"],
    example: { action: "status", id: "mon_01H…" },
  },
  delete: {
    required: ["id"],
    example: { action: "delete", id: "mon_01H…" },
  },
};
const MONITOR_USAGE = "create → url, status → id, delete → id";

/** Fields that may arrive as strings from untyped MCP clients. */
const MONITOR_SHAPE = {
  numbers: ["interval_seconds"],
  booleans: [],
  json: [],
};

/**
 * Handler for the tomoshi_monitor tool (formerly cinder_monitor).
 * Dispatches to create / status / delete based on the `action` field.
 */
export function createMonitorHandler(client: TomoshiClient) {
  return async (input: Record<string, unknown>) => {
    const { value: args, error } = normalize(input, MONITOR_SHAPE);
    if (error) return toolError(error);
    const invalid = guardAction(
      "tomoshi_monitor",
      args,
      MONITOR_ACTIONS,
      MONITOR_USAGE,
    );
    if (invalid) return invalid;
    const { action } = args as { action: string };

    try {
      if (action === "create") {
        const result = await client.createMonitor(args as any);
        const lines: string[] = [
          "# Monitor Created",
          "",
          `**Monitor ID:** \`${result.id}\``,
          `**URL:** ${result.url}`,
          `**Interval:** ${result.interval_seconds}s`,
          `**Next Check:** ${result.next_check}`,
          "",
          "---",
          "",
          'Use `tomoshi_monitor` with `action: "status"` to check, or `action: "delete"` to stop monitoring.',
        ];
        return {
          content: [{ type: "text" as const, text: lines.join("\n") }],
        };
      }

      if (action === "status") {
        const result = await client.getMonitor((args as any).id);
        const lines: string[] = [
          "# Monitor Status",
          "",
          `**Monitor ID:** \`${result.id}\``,
          `**URL:** ${result.url}`,
          `**Interval:** ${result.interval_seconds}s`,
          `**Next Check:** ${result.next_check}`,
        ];
        if (result.last_hash) {
          lines.push(`**Last Hash:** ${result.last_hash}`);
        }
        return {
          content: [{ type: "text" as const, text: lines.join("\n") }],
        };
      }

      // action === "delete"
      await client.deleteMonitor((args as any).id);
      return {
        content: [
          {
            type: "text" as const,
            text: `Monitor \`${(args as any).id}\` deleted.`,
          },
        ],
      };
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      return {
        content: [
          {
            type: "text" as const,
            text: `Monitor ${action} failed: ${message}`,
          },
        ],
        isError: true,
      };
    }
  };
}
