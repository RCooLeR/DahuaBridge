import { describe, expect, it } from "vitest";

import { resolveHlsPlaybackMode } from "../src/cards/surveillance-remote-stream";

describe("remote stream hls playback mode", () => {
  it("prefers hls.js when both hls.js and native hls are reported", () => {
    expect(
      resolveHlsPlaybackMode({
        hlsJsSupported: true,
        nativeHlsSupported: true,
      }),
    ).toBe("hls.js");
  });

  it("falls back to native hls when hls.js is unavailable", () => {
    expect(
      resolveHlsPlaybackMode({
        hlsJsSupported: false,
        nativeHlsSupported: true,
      }),
    ).toBe("native");
  });

  it("marks hls unsupported when neither path is available", () => {
    expect(
      resolveHlsPlaybackMode({
        hlsJsSupported: false,
        nativeHlsSupported: false,
      }),
    ).toBe("unsupported");
  });
});
