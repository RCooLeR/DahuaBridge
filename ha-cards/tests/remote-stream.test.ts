import { describe, expect, it } from "vitest";

import {
  resolveHlsPlaybackMode,
  resolveSourceFailureAction,
  resolveSourceFailureTransition,
} from "../src/cards/surveillance-remote-stream";

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

  it("advances to the next source before scheduling a retry", () => {
    expect(resolveSourceFailureTransition(0, 2)).toEqual({
      nextIndex: 1,
      retry: false,
    });
  });

  it("schedules a retry after the last source fails", () => {
    expect(resolveSourceFailureTransition(1, 2)).toEqual({
      nextIndex: 2,
      retry: true,
    });
  });

  it("retries the same source before falling back when the error is retryable", () => {
    expect(
      resolveSourceFailureAction({
        sourceIndex: 0,
        sourceCount: 2,
        retryable: true,
        attempt: 1,
        maxAttempts: 3,
      }),
    ).toEqual({
      nextIndex: 0,
      nextAttempt: 2,
      retryCurrentSource: true,
      retryExhaustedSources: false,
    });
  });

  it("falls through to the next source after retry attempts are exhausted", () => {
    expect(
      resolveSourceFailureAction({
        sourceIndex: 0,
        sourceCount: 2,
        retryable: true,
        attempt: 3,
        maxAttempts: 3,
      }),
    ).toEqual({
      nextIndex: 1,
      nextAttempt: 3,
      retryCurrentSource: false,
      retryExhaustedSources: false,
    });
  });

  it("schedules the exhausted-sources retry after the last source still fails", () => {
    expect(
      resolveSourceFailureAction({
        sourceIndex: 1,
        sourceCount: 2,
        retryable: false,
        attempt: 0,
        maxAttempts: 3,
      }),
    ).toEqual({
      nextIndex: 2,
      nextAttempt: 0,
      retryCurrentSource: false,
      retryExhaustedSources: true,
    });
  });
});
