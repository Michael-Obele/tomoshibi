import * as v from "valibot";

/**
 * Environment configuration schema validated with Valibot.
 * Provides typed access to all configuration values with sensible defaults.
 */
// Accepts a URL or empty string (VS Code sends "" when env is unset).
// v.optional only skips validation for undefined — "" would fail v.url().
const OptionalUrl = v.optional(
  v.union([v.literal(""), v.pipe(v.string(), v.url())]),
  "",
);

const ConfigSchema = v.object({
  /** Tomoshibi API base URL — must point to your own Tomoshibi instance */
  TOMOSHI_API_URL: OptionalUrl,
  /** Legacy compat — use TOMOSHI_API_URL */
  CINDER_API_URL: OptionalUrl,

  /** Optional API key if Tomoshibi requires authentication */
  TOMOSHI_API_KEY: v.optional(v.string(), ""),
  /** Legacy compat — use TOMOSHI_API_KEY */
  CINDER_API_KEY: v.optional(v.string(), ""),

  /** MCP server identity */
  MCP_SERVER_NAME: v.optional(v.string(), "tomoshi-mcp"),
  MCP_SERVER_VERSION: v.optional(v.string(), "1.0.0"),

  /** HTTP server port */
  PORT: v.optional(v.pipe(v.string(), v.transform(Number)), "7433"),

  /** OAuth 2.1 configuration */
  OAUTH_ENABLED: v.optional(
    v.pipe(
      v.string(),
      v.transform((s) => s === "true"),
    ),
    "false",
  ),
  OAUTH_ISSUER: v.optional(v.string(), ""),
  OAUTH_AUDIENCE: v.optional(v.string(), ""),
  OAUTH_SCOPES: v.optional(v.string(), "scrape,crawl,search,monitor"),

  /** Session management */
  REDIS_URL: v.optional(v.string(), ""),
  MCP_SESSION_MANAGER: v.optional(v.picklist(["memory", "redis"]), "memory"),

  /** Rate limiting */
  RATE_LIMIT_WINDOW_MS: v.optional(
    v.pipe(v.string(), v.transform(Number)),
    "60000",
  ),
  RATE_LIMIT_MAX: v.optional(v.pipe(v.string(), v.transform(Number)), "60"),

  /** Logging */
  LOG_LEVEL: v.optional(v.picklist(["debug", "info", "warn", "error"]), "info"),
});

type Config = v.InferOutput<typeof ConfigSchema>;

let config: Config | null = null;

/**
 * Get the application configuration.
 * Parses environment variables once and caches the result.
 */
export function getConfig(): Config {
  if (config) return config;

  const parsed = v.safeParse(ConfigSchema, process.env);
  if (!parsed.success) {
    console.error("Invalid configuration:", v.flatten(parsed.issues));
    process.exit(1);
  }

  config = parsed.output;

  // TOMOSHI_API_URL is the primary name, CINDER_API_URL is legacy compat
  const apiUrl =
    (config as any).TOMOSHI_API_URL || (config as any).CINDER_API_URL;
  if (!apiUrl) {
    console.error(
      "❌ TOMOSHI_API_URL (or legacy CINDER_API_URL) is required. Set it in your .env or via `fly secrets set TOMOSHI_API_URL=https://your-tomoshibi.fly.dev`",
    );
    process.exit(1);
  }
  // Normalize so downstream code can use either
  (config as any).CINDER_API_URL = apiUrl;
  (config as any).TOMOSHI_API_URL = apiUrl;

  return config;
}

export type { Config };
