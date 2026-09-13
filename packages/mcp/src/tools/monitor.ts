import * as v from "valibot";
import type { CinderClient } from "../client.js";

/**
 * Schema for the cinder_monitor tool.
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
      v.description("HMAC-SHA256 key for the X-Cinder-Signature header"),
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

export const MonitorSchema = v.variant("action", [
  MonitorCreateShape,
  MonitorStatusShape,
  MonitorDeleteShape,
]);

export type MonitorInput = v.InferOutput<typeof MonitorSchema>;

/**
 * Handler for the cinder_monitor tool.
 * Dispatches to create / status / delete based on the `action` field.
 */
export function createMonitorHandler(client: CinderClient) {
  return async (input: Record<string, unknown>) => {
    const { action } = input as { action: string };

    try {
      if (action === "create") {
        const result = await client.createMonitor(input as any);
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
          'Use `cinder_monitor` with `action: "status"` to check, or `action: "delete"` to stop monitoring.',
        ];
        return {
          content: [{ type: "text" as const, text: lines.join("\n") }],
        };
      }

      if (action === "status") {
        const result = await client.getMonitor((input as any).id);
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
      await client.deleteMonitor((input as any).id);
      return {
        content: [
          {
            type: "text" as const,
            text: `Monitor \`${(input as any).id}\` deleted.`,
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
