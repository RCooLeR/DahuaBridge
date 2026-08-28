import Hls from "hls.js";
import { describe, expect, it } from "vitest";

import {
  isHlsPlaylistStalled,
  resolveHlsPlaybackMode,
  resolveSourceFailureAction,
  resolveSourceFailureTransition,
} from "../src/cards/surveillance-remote-stream";
import {
  clampStreamVolume,
  streamVolumeFromInputValue,
  streamVolumeIcon,
  streamVolumePercent,
} from "../src/cards/surveillance-panel-player-audio-model";

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

  it("recognizes hls.js's bounded unchanged-playlist failure", () => {
    expect(isHlsPlaylistStalled(Hls.ErrorDetails.PLAYLIST_UNCHANGED_ERROR)).toBe(true);
    expect(isHlsPlaylistStalled(Hls.ErrorDetails.LEVEL_LOAD_ERROR)).toBe(false);
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

describe("player audio model", () => {
  it("clamps browser stream volume to the media element range", () => {
    expect(clampStreamVolume(-1)).toBe(0);
    expect(clampStreamVolume(0.42)).toBe(0.42);
    expect(clampStreamVolume(2)).toBe(1);
    expect(clampStreamVolume(Number.NaN)).toBe(1);
  });

  it("maps slider input and icons to browser-local volume state", () => {
    expect(streamVolumeFromInputValue("37")).toBe(0.37);
    expect(streamVolumePercent(0.374)).toBe(37);
    expect(streamVolumeIcon(true, 1)).toBe("mdi:volume-off");
    expect(streamVolumeIcon(false, 0.2)).toBe("mdi:volume-medium");
    expect(streamVolumeIcon(false, 0.8)).toBe("mdi:volume-high");
  });
});
