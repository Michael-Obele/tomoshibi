import { describe, expect, test } from "bun:test";
import {
  ENV_HEADER,
  buildForwardEnvHeader,
  forwardEnvDisabled,
} from "./client.js";

// Parity pin: the Go side hardcodes this name too (internal/search/envctx).
// If the two drift, forwarding silently stops working.
test("ENV_HEADER matches the backend header", () => {
  expect(ENV_HEADER).toBe("X-Tomoshi-Env");
});

describe("forwardEnvDisabled", () => {
  test("accepts the documented off switches", () => {
    for (const setting of ["false", "0", "off", "OFF", " never "]) {
      expect(forwardEnvDisabled(setting)).toBe(true);
    }
  });

  test("leaves auto and comma lists on", () => {
    for (const setting of ["", "  ", "BRAVE_SEARCH_API_KEY", "a,b"]) {
      expect(forwardEnvDisabled(setting)).toBe(false);
    }
  });
});

describe("buildForwardEnvHeader", () => {
  const env = {
    BRAVE_SEARCH_API_KEY: "BSB-test",
    TAVILY_API_KEY: "",
    UNRELATED_SECRET: "sk-never-sent",
  };

  test("forwards only wanted names that are present", () => {
    const got = buildForwardEnvHeader(
      ["BRAVE_SEARCH_API_KEY", "TAVILY_API_KEY"],
      "",
      env,
    );
    expect(got).toBe(JSON.stringify({ BRAVE_SEARCH_API_KEY: "BSB-test" }));
  });

  test("restricts to the TOMOSHI_FORWARD_ENV comma list", () => {
    const got = buildForwardEnvHeader(
      ["BRAVE_SEARCH_API_KEY"],
      "BRAVE_SEARCH_API_KEY,OTHER",
      env,
    );
    expect(got).toBe(JSON.stringify({ BRAVE_SEARCH_API_KEY: "BSB-test" }));

    expect(
      buildForwardEnvHeader(["BRAVE_SEARCH_API_KEY"], "OTHER_KEY", env),
    ).toBeUndefined();
  });

  test("returns undefined when there is nothing to send", () => {
    expect(buildForwardEnvHeader([], "", env)).toBeUndefined();
    expect(
      buildForwardEnvHeader(["TAVILY_API_KEY"], "", env),
    ).toBeUndefined();
  });

  test("an off switch in the setting sends nothing", () => {
    expect(
      buildForwardEnvHeader(["BRAVE_SEARCH_API_KEY"], "false", env),
    ).toBeUndefined();
  });

  test("drops oversized payloads rather than blow the header cap", () => {
    const big = { BRAVE_SEARCH_API_KEY: "x".repeat(7000) };
    expect(buildForwardEnvHeader(["BRAVE_SEARCH_API_KEY"], "", big)).toBeUndefined();
  });
});
