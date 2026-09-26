import * as v from "valibot";

/**
 * Shared schema helpers for the resource-oriented MCP tools.
 *
 * Design notes (docs/plans/2026-09-26-mcp-flat-schema-design.md):
 * - `@valibot/to-json-schema` cannot convert `transform` or `rawCheck` actions,
 *   so runtime coercion and per-action rules live in the handlers instead.
 * - Numeric/boolean/composite fields are declared as `union([typed, string])`:
 *   untyped MCP clients (and models that echo strings) keep working, while the
 *   adapter in server.ts promotes the typed branch back for the model-facing
 *   JSON Schema.
 */

/** Number field that also accepts numeric strings from untyped clients. */
export function num(description: string, min?: number, max?: number) {
  const bounds =
    min !== undefined && max !== undefined
      ? v.pipe(v.number(), v.minValue(min), v.maxValue(max))
      : min !== undefined
        ? v.pipe(v.number(), v.minValue(min))
        : max !== undefined
          ? v.pipe(v.number(), v.maxValue(max))
          : v.number();
  return v.pipe(v.union([v.string(), bounds]), v.description(description));
}

/** Boolean field that also accepts "true"/"false"/"1"/"0" strings. */
export function bool(description: string) {
  return v.pipe(v.union([v.boolean(), v.string()]), v.description(description));
}

/** Array/object field that also accepts a JSON-encoded string. */
export function composite<T extends v.GenericSchema>(
  schema: T,
  description: string,
) {
  return v.pipe(v.union([schema, v.string()]), v.description(description));
}

/** Keys whose value is missing/empty for the per-action required check. */
export function missingKeys(
  input: Record<string, unknown>,
  keys: string[],
): string[] {
  return keys.filter((key) => {
    const value = input[key];
    return (
      value === undefined ||
      value === null ||
      value === "" ||
      (Array.isArray(value) && value.length === 0)
    );
  });
}

export interface ToolResult {
  content: [{ type: "text"; text: string }];
  isError: true;
}

/** Plain error result returned by a tool handler. */
export function toolError(text: string): ToolResult {
  return { content: [{ type: "text", text }], isError: true };
}

/** Readable error for a missing per-action parameter. */
export function usageError(opts: {
  tool: string;
  action: string;
  missing: string[];
  usage: string;
  example?: Record<string, unknown>;
}): ToolResult {
  return toolError(
    [
      `Missing required parameter(s) for action "${opts.action}": ${opts.missing.join(", ")}.`,
      `Usage — ${opts.tool}: ${opts.usage}`,
      opts.example ? `Example: ${JSON.stringify(opts.example)}` : undefined,
    ]
      .filter((line): line is string => line !== undefined)
      .join("\n"),
  );
}

export interface NormalizeShape {
  numbers?: string[];
  booleans?: string[];
  json?: string[];
}

/**
 * Coerce stringified scalars that arrive from untyped MCP clients.
 * Returns `error` (readable text) when a value cannot be coerced.
 */
export function normalize(
  input: Record<string, unknown>,
  shape: NormalizeShape,
): { value: Record<string, unknown>; error?: string } {
  const value: Record<string, unknown> = { ...input };

  for (const key of shape.numbers ?? []) {
    const raw = value[key];
    if (typeof raw === "string") {
      const parsed = Number(raw.trim());
      if (!Number.isFinite(parsed)) {
        return {
          value,
          error: `"${key}" must be a number (received "${raw}")`,
        };
      }
      value[key] = parsed;
    }
  }

  for (const key of shape.booleans ?? []) {
    const raw = value[key];
    if (typeof raw === "string") {
      const normalized = raw.trim().toLowerCase();
      if (["true", "1", "yes"].includes(normalized)) value[key] = true;
      else if (["false", "0", "no"].includes(normalized)) value[key] = false;
      else {
        return {
          value,
          error: `"${key}" must be a boolean (received "${raw}")`,
        };
      }
    }
  }

  for (const key of shape.json ?? []) {
    const raw = value[key];
    if (typeof raw === "string") {
      let parsed: unknown;
      try {
        parsed = JSON.parse(raw);
      } catch {
        parsed = undefined;
      }
      if (
        Array.isArray(parsed) ||
        (parsed !== null && typeof parsed === "object")
      ) {
        value[key] = parsed;
      } else {
        return {
          value,
          error: `"${key}" must be a JSON array or object (received "${raw}")`,
        };
      }
    }
  }

  return { value };
}

export interface ActionSpec {
  /** Parameter names that must be present for this action. */
  required?: string[];
  /** Example payload quoted in the error message. */
  example?: Record<string, unknown>;
}

/**
 * Per-action guard used at the top of every tool handler:
 * - unknown/missing `action` → lists the valid actions (with an example)
 * - missing required params → readable usage error, before any network call
 *
 * Returns undefined when the input is acceptable.
 */
export function guardAction(
  tool: string,
  input: Record<string, unknown>,
  actions: Record<string, ActionSpec>,
  usage: string,
): ToolResult | undefined {
  const raw = input.action;
  const action = typeof raw === "string" ? raw : "";
  const spec = actions[action];
  if (!spec) {
    const example = Object.values(actions)[0]?.example;
    return toolError(
      [
        `Missing or unknown "action" for ${tool}. Valid actions: ${usage}`,
        example ? `Example: ${JSON.stringify(example)}` : undefined,
      ]
        .filter((line): line is string => line !== undefined)
        .join("\n"),
    );
  }

  const missing = missingKeys(input, spec.required ?? []);
  if (missing.length > 0) {
    return usageError({ tool, action, missing, usage, example: spec.example });
  }
  return undefined;
}

export interface FlattenOptions {
  /** Discriminator field — the only one that stays required. */
  action: v.GenericSchema;
  /** Field names that must also accept stringified values from untyped clients. */
  lenient?: string[];
  /** Post-merge replacements (conflicting bounds/defaults, action-specific docs). */
  overrides?: v.ObjectEntries;
}

/**
 * Merge per-action `v.object` shapes into the entry list of one flat object.
 *
 * - every field except `action` becomes optional: required-ness is per action
 *   and is enforced by `guardAction` in the handler (tmcp validates arguments
 *   before the handler runs, and the JSON-Schema converter cannot express
 *   conditional requirements)
 * - fields listed in `lenient` accept strings too; the adapter promotes the
 *   typed branch back so the model-facing schema keeps `type: "number"` etc.
 * - per-field defaults survive the unwrap/rewrap; the Go API applies its own
 *   defaults and caps for anything left unset
 */
export function flattenShapes(
  shapes: v.GenericSchema[],
  options: FlattenOptions,
): v.ObjectEntries {
  const merged: Record<string, v.GenericSchema> = {};

  for (const shape of shapes) {
    const entries = (shape as { entries?: v.ObjectEntries }).entries;
    if (!entries) continue;

    for (const [key, schema] of Object.entries(entries)) {
      if (key === "action") continue;
      const isOptional = (schema as { type?: string }).type === "optional";
      const inner = isOptional
        ? (schema as { wrapped: v.GenericSchema }).wrapped
        : schema;
      const defaultValue = isOptional
        ? (schema as { default?: unknown }).default
        : undefined;
      const body = options.lenient?.includes(key)
        ? v.union([inner, v.string()])
        : inner;
      merged[key] =
        defaultValue === undefined
          ? v.optional(body)
          : (v.optional(body, defaultValue as never) as v.GenericSchema);
    }
  }

  return { ...merged, ...options.overrides, action: options.action };
}
