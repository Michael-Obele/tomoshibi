import * as v from "valibot";

/**
 * Environment configuration schema validated with Valibot.
 * Provides typed access to all configuration values with sensible defaults.
 */
const ConfigSchema = v.object({
  /** Cinder API base URL — must point to your own Cinder instance */
  CINDER_API_URL: v.optional(v.pipe(v.string(), v.url()), ""),

  /** Optional API key if Cinder requires authentication */
  CINDER_API_KEY: v.optional(v.string(), ""),

  /** MCP server identity */
  MCP_SERVER_NAME: v.optional(v.string(), "cinder-mcp"),
  MCP_SERVER_VERSION: v.optional(v.string(), "1.0.0"),

  /** HTTP server port */
  PORT: v.optional(v.pipe(v.string(), v.transform(Number)), "9631"),

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

  if (!config.CINDER_API_URL) {
    console.error(
      "❌ CINDER_API_URL is required. Set it in your .env or via `fly secrets set CINDER_API_URL=https://your-cinder.fly.dev`",
    );
    process.exit(1);
  }

  return config;
}

export type { Config };
