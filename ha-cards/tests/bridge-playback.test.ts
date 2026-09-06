import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createPlaybackSession,
  createPlaybackSessionFromRecording,
  resolvePlaybackLaunchUrl,
  seekPlaybackSession,
} from "../src/ha/bridge-playback";

describe("bridge playback", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("rebases playback requests and carries bridge authentication to returned media URLs", async () => {
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
            dash_url: "/api/v1/media/dash/nvrpb_test/quality/manifest.mpd",
            hls_url: "/api/v1/media/hls/nvrpb_test/quality/index.m3u8",
            mjpeg_url: "/api/v1/media/mjpeg/nvrpb_test?profile=quality",
            webrtc_offer_url: "/api/v1/media/webrtc/nvrpb_test/quality/offer",
          },
        },
      }),
    } as Response);

    const session = await createPlaybackSession(
      "http://internal.local:9020/api/v1/nvr/west20_nvr/playback/sessions?auth_token=bridge-token",
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
    expect(fetch).toHaveBeenCalledWith("https://ha.example.com/bridge/api/v1/nvr/west20_nvr/playback/sessions?auth_token=bridge-token", expect.objectContaining({method: "POST"}));
    expect(session.profiles.quality?.hlsUrl).toBe(
      "https://ha.example.com/bridge/api/v1/media/hls/nvrpb_test/quality/index.m3u8?auth_token=bridge-token",
    );
    expect(resolvePlaybackLaunchUrl(session)).toBe(
      "https://ha.example.com/bridge/api/v1/media/hls/nvrpb_test/quality/index.m3u8?auth_token=bridge-token",
    );
    expect(session.profiles.quality?.mjpegUrl).toBe("https://ha.example.com/bridge/api/v1/media/mjpeg/nvrpb_test?profile=quality&auth_token=bridge-token");
    expect(session.snapshotUrl).toContain("?auth_token=bridge-token");
    expect(session.profiles.quality?.dashUrl).toContain("?auth_token=bridge-token");
    expect(session.profiles.quality?.webrtcOfferUrl).toContain("?auth_token=bridge-token");
  });

  it("never forwards the bridge token to another host and preserves endpoint-specific tokens", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({ok: true, json: async () => ({
      id: "pb", stream_id: "pb", device_id: "nvr", name: "Archive", channel: 1,
      start_time: "2026-05-01T00:00:00Z", end_time: "2026-05-01T01:00:00Z", seek_time: "2026-05-01T00:00:00Z",
      recommended_profile: "quality", profiles: {quality: {
        name: "quality", hls_url: "https://other.example/api/media/index.m3u8",
        mjpeg_url: "https://bridge.local/api/v1/media/mjpeg/pb?token=endpoint-token",
      }},
    })} as Response);
    const session = await createPlaybackSession("https://bridge.local/api/v1/nvr/nvr/playback/sessions?auth_token=bridge-token", {
      channel: 1, startTime: "2026-05-01T00:00:00Z", endTime: "2026-05-01T01:00:00Z", seekTime: null,
    });
    expect(session.profiles.quality?.hlsUrl).toBe("https://other.example/api/media/index.m3u8");
    expect(session.profiles.quality?.mjpegUrl).toBe("https://bridge.local/api/v1/media/mjpeg/pb?token=endpoint-token");
  });

  it("creates a session request from an archive recording at the recording start time", () => {
    const request = createPlaybackSessionFromRecording({
      source: "nvr_event",
      channel: 4,
      startTime: "2026-04-28T03:00:00Z",
      endTime: "2026-04-28T03:10:00Z",
      downloadUrl: null,
      exportUrl: null,
      filePath: "/mnt/dahua/recording.dav",
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
      filePath: "/mnt/dahua/recording.dav",
      source: "nvr_event",
      type: "Recording",
      videoStream: "main",
    });
  });

  it("seeks playback sessions when the session placeholder is URL-encoded", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        id: "nvrpb_next",
        stream_id: "nvrpb_next",
        device_id: "west20_nvr",
        source_stream_id: "west20_nvr_channel_02",
        name: "Lobby",
        channel: 2,
        start_time: "2026-04-28T00:00:00Z",
        end_time: "2026-04-28T01:00:00Z",
        seek_time: "2026-04-28T00:30:00Z",
        recommended_profile: "stable",
        profiles: {
          stable: {
            name: "stable",
            hls_url: "/api/v1/media/hls/nvrpb_next/stable/index.m3u8",
          },
        },
      }),
    } as Response);

    const session = await seekPlaybackSession(
      "nvrpb_test",
      "http://internal.local:9020/api/v1/nvr/playback/sessions/%7Bsession_id%7D/seek?auth_token=bridge-token",
      { seekTime: "2026-04-28T00:30:00Z" },
      "https://ha.example.com/bridge",
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "https://ha.example.com/bridge/api/v1/nvr/playback/sessions/nvrpb_test/seek?auth_token=bridge-token",
      expect.objectContaining({ method: "POST" }),
    );
    expect(session.profiles.stable?.hlsUrl).toBe("https://ha.example.com/bridge/api/v1/media/hls/nvrpb_next/stable/index.m3u8?auth_token=bridge-token");
  });
});
