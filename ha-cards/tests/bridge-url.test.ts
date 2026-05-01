import { describe, expect, it } from "vitest";

import {
  buildBridgeEndpointUrl,
  normalizeBrowserBridgeUrl,
  rewriteBridgeUrl,
} from "../src/ha/bridge-url";

describe("bridge URL rewriting", () => {
  it("rewrites absolute bridge URLs to the configured browser bridge URL", () => {
    expect(
      rewriteBridgeUrl(
        "http://127.0.0.1:9205/api/v1/events?limit=25",
        "https://ha.example.com/dahuabridge",
      ),
    ).toBe("https://ha.example.com/dahuabridge/api/v1/events?limit=25");
  });

  it("rewrites root-relative bridge paths to the configured browser bridge URL", () => {
    expect(
      rewriteBridgeUrl("/api/v1/vto/front_vto/call/answer", "https://ha.example.com/bridge"),
    ).toBe("https://ha.example.com/bridge/api/v1/vto/front_vto/call/answer");
  });

  it("normalizes the configured browser bridge URL", () => {
    expect(normalizeBrowserBridgeUrl("https://ha.example.com/bridge/")).toBe(
      "https://ha.example.com/bridge",
    );
  });

  it("builds bridge endpoint URLs without dropping the browser bridge path prefix", () => {
    expect(
      buildBridgeEndpointUrl(
        "https://ha.example.com/bridge/",
        "/api/v1/nvr/west20_nvr/events/summary",
      ),
    ).toBe("https://ha.example.com/bridge/api/v1/nvr/west20_nvr/events/summary");
  });
});
