import { ValibotJsonSchemaAdapter } from "@tmcp/adapter-valibot";

/**
 * Walk a generated JSON Schema and promote unions shaped like `[typed, string]`
 * (the coercion-friendly fields built by `tools/schema.ts`) back to their typed
 * branch, so the model-facing schema shows `type: "number"` / `"boolean"` /
 * `"array"` instead of an `anyOf` the client may not understand.
 *
 * Ambiguous unions (more than one typed branch, or no typed branch) are left
 * alone and recursed into instead.
 */
export function promoteUnionTypes(node: unknown): void {
  if (node === null || typeof node !== "object") return;
  if (Array.isArray(node)) {
    for (const item of node) promoteUnionTypes(item);
    return;
  }

  const record = node as Record<string, unknown>;
  const union = record.anyOf ?? record.oneOf;
  if (Array.isArray(union)) {
    const typed = union.filter(
      (branch) =>
        branch !== null &&
        typeof branch === "object" &&
        typeof (branch as { type?: unknown }).type === "string" &&
        (branch as { type?: string }).type !== "string",
    );
    if (typed.length === 1 && typed.length === union.length - 1) {
      const branch = { ...(typed[0] as Record<string, unknown>) };
      delete branch.$schema;
      // keep sibling keywords (description, default, examples, …) and let the
      // promoted branch win on overlapping keys (type, minimum, enum, …)
      const siblings: Record<string, unknown> = {};
      for (const [key, value] of Object.entries(record)) {
        if (key !== "anyOf" && key !== "oneOf" && key !== "$schema") {
          siblings[key] = value;
        }
      }
      for (const key of Object.keys(record)) delete record[key];
      Object.assign(record, siblings, branch);
      return;
    }
    for (const branch of union) promoteUnionTypes(branch);
  }

  for (const [key, value] of Object.entries(record)) {
    if (key === "anyOf" || key === "oneOf") continue;
    promoteUnionTypes(value);
  }
}

/**
 * Valibot → JSON Schema adapter for the three resource-oriented tools.
 *
 * - Forces `type: "object"` on union roots: tmcp and most MCP clients expect a
 *   root object, and a bare `oneOf`/`anyOf` at the root is what made clients
 *   hand the model an empty `properties: {}` schema (models then called the
 *   tools with `{}`).
 * - Promotes `[typed, string]` unions (see `promoteUnionTypes`).
 */
export class TomoshiJsonSchemaAdapter extends ValibotJsonSchemaAdapter {
  async toJsonSchema(schema: any): Promise<any> {
    const json: any = await super.toJsonSchema(schema);
    if (!json.type && (json.oneOf || json.anyOf)) json.type = "object";
    promoteUnionTypes(json);
    if (json.type === "object" && json.properties?.action) {
      // `action` stays optional in the runtime schema so a missing action can
      // produce a readable usage error from the handler instead of tmcp's raw
      // validation dump; the model-facing schema still marks it required.
      json.required = Array.from(
        new Set([...(json.required ?? []), "action"]),
      );
    }
    return json;
  }
}
