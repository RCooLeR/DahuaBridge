import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createPlaybackSession,
  createPlaybackSessionFromRecording,
  resolvePlaybackLaunchUrl,
} from "../src/ha/bridge-playback";

describe("bridge playback", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("creates a playback session and rewrites returned profile URLs for the browser bridge URL", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        id: "nvrpb_test",
        stream_id: "nvrpb_test",
        device_id: "west20_nvr",
        source_stream_id: "west20_nvr_channel_02",
        name: "Lobby",
        channel: 2,
        start_time: "2026-04-28T00:00:00Z",
        end_time: "2026-04-28T01:00:00Z",
        seek_time: "2026-04-28T00:20:00Z",
        recommended_profile: "quality",
        snapshot_url: "/api/v1/nvr/playback/sessions/nvrpb_test/snapshot",
        created_at: "2026-04-28T00:00:00Z",
        expires_at: "2026-04-28T01:00:00Z",
        profiles: {
          quality: {
            name: "quality",
            hls_url: "/api/v1/media/hls/nvrpb_test/quality/index.m3u8",
            mjpeg_url: "/api/v1/media/mjpeg/nvrpb_test?profile=quality",
            webrtc_offer_url: "/api/v1/media/webrtc/nvrpb_test/quality/offer",
          },
        },
      }),
    } as Response);

    const session = await createPlaybackSession(
      "https://ha.example.com/bridge/api/v1/nvr/west20_nvr/playback/sessions",
      {
        channel: 2,
        startTime: "2026-04-28T00:00:00Z",
        endTime: "2026-04-28T01:00:00Z",
        seekTime: "2026-04-28T00:20:00Z",
      },
      "https://ha.example.com/bridge",
    );

    expect(session.deviceId).toBe("west20_nvr");
    expect(session.recommendedProfile).toBe("quality");
    expect(session.profiles.quality?.hlsUrl).toBe(
      "https://ha.example.com/bridge/api/v1/media/hls/nvrpb_test/quality/index.m3u8",
    );
    expect(resolvePlaybackLaunchUrl(session)).toBe(
      "https://ha.example.com/bridge/api/v1/media/hls/nvrpb_test/quality/index.m3u8",
    );
  });

  it("creates a session request from an archive recording at the recording start time", () => {
    const request = createPlaybackSessionFromRecording({
      channel: 4,
      startTime: "2026-04-28T03:00:00Z",
      endTime: "2026-04-28T03:10:00Z",
      downloadUrl: null,
      filePath: null,
      type: "Recording",
      videoStream: "main",
      disk: null,
      partition: null,
      cluster: null,
      lengthBytes: null,
      cutLengthBytes: null,
      flags: [],
    });

    expect(request).toEqual({
      channel: 4,
      startTime: "2026-04-28T03:00:00Z",
      endTime: "2026-04-28T03:10:00Z",
      seekTime: "2026-04-28T03:00:00Z",
    });
  });
});
