import { SimpleProvider } from "@tmcp/auth";

/**
 * In-memory storage for OAuth 2.1 client registrations, codes, and tokens.
 * For production, replace with a database-backed store (Redis, Postgres, etc.).
 */

const codes = new Map<string, unknown>();
const clients = new Map<string, unknown>();
const tokens = new Map<string, unknown>();
const refreshTokens = new Map<string, unknown>();

const chars = "abcdefghijkmnpqrstuvwxyz23456789";

function randomString(length = 32): string {
  const randomBytes = new Uint8Array(length);
  crypto.getRandomValues(randomBytes);
  let result = "";
  for (let i = 0; i < randomBytes.byteLength; i++) {
    result += chars[randomBytes[i] >> 3];
  }
  return result;
}

/**
 * OAuth 2.1 SimpleProvider configuration.
 * Uses in-memory stores — swap for persistent stores in production.
 *
 * @see https://tmcp.io/docs/auth/oauth
 */
export const oauth = new SimpleProvider({
  clients: {
    async get(client_id: string) {
      return clients.get(client_id) as any;
    },
    async register(client_info: any) {
      const client_id = randomString(13);
      const newClient: any = {
        ...client_info,
        client_id,
        client_id_issued_at: Date.now(),
        redirect_uris: client_info.redirect_uris ?? [],
      };
      clients.set(client_id, newClient);
      return newClient;
    },
  },
  codes: {
    async get(code: string) {
      return codes.get(code) as any;
    },
    async store(code: string, codeData: unknown) {
      codes.set(code, codeData);
    },
    async delete(code: string) {
      codes.delete(code);
    },
  },
  tokens: {
    async get(token: string) {
      return tokens.get(token) as any;
    },
    async store(token: string, tokenData: unknown) {
      tokens.set(token, tokenData);
    },
    async delete(token: string) {
      tokens.delete(token);
    },
  },
  refreshTokens: {
    async get(token: string) {
      return refreshTokens.get(token) as any;
    },
    async store(token: string, tokenData: unknown) {
      refreshTokens.set(token, tokenData);
    },
    async delete(token: string) {
      refreshTokens.delete(token);
    },
  },
}).build("http://localhost:9631", {
  bearer: {
    paths: {
      POST: ["/mcp"],
      GET: ["/sse"],
    },
  },
  cors: {
    origin: "*",
    credentials: true,
  },
  registration: true,
});
