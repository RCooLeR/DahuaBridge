import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { exportArchiveRecording, fetchArchiveRecordings, fetchBridgeRecordings, waitForArchiveExportCompletion } from "../src/ha/bridge-archive";
import { authenticatedBridgeResourceUrl } from "../src/ha/bridge-resource";

describe("authenticated archive resources", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal("window", { location: { origin: "https://ha.test" }, setTimeout, clearTimeout });
    vi.spyOn(console, "log").mockImplementation(() => undefined);
  });
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers(); });

  it("carries request authentication through search, export, polling and clip media", async () => {
    const request = "https://bridge.test/api/v1/nvr/nvr/recordings?auth_token=bridge-token";
    const media = "/api/v1/media/recordings/clip";
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(response({ items: [{ channel: 1, export_url: "/api/v1/nvr/nvr/recordings/export?channel=1", asset_playback_url: `${media}/play`, asset_download_url: `${media}/download` }] }))
      .mockResolvedValueOnce(response({ status: "ok", clip: { id: "clip", status: "recording", self_url: media } }))
      .mockResolvedValueOnce(response({ id: "clip", status: "completed", self_url: media, playback_url: `${media}/play`, download_url: `${media}/download` }));
    const recordings = await fetchArchiveRecordings(request, { channel: 1, startTime: "2026-09-06T00:00:00Z", endTime: "2026-09-07T00:00:00Z", limit: 10 });
    const row = recordings.items[0]!;
    expect(new URL(row.exportUrl!).searchParams.get("auth_token")).toBe("bridge-token");
    expect(row.assetPlaybackUrl).toContain("auth_token=bridge-token");
    expect(row.assetDownloadUrl).toContain("auth_token=bridge-token");
    const clip = await exportArchiveRecording(row.exportUrl!, "https://browser.test/bridge");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("https://browser.test/bridge/api/v1/nvr/nvr/recordings/export?channel=1&auth_token=bridge-token");
    const pending = waitForArchiveExportCompletion(clip, "https://browser.test/bridge");
    await vi.advanceTimersByTimeAsync(1500);
    const completed = await pending;
    expect(fetchMock.mock.calls[2]?.[0]).toBe("https://browser.test/bridge/api/v1/media/recordings/clip?auth_token=bridge-token");
    expect(completed.playbackUrl).toBe("https://browser.test/bridge/api/v1/media/recordings/clip/play?auth_token=bridge-token");
    expect(completed.downloadUrl).toContain("auth_token=bridge-token");
  });

  it("authenticates MP4 links returned by a protected list", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(response({ items: [{ id: "clip", stream_id: "cam", status: "completed", started_at: "2026-09-06T00:00:00Z", playback_url: "/api/v1/media/recordings/clip/play", download_url: "/api/v1/media/recordings/clip/download" }] }));
    const clips = await fetchBridgeRecordings("https://bridge.test/api/v1/media/recordings?token=bridge-token");
    expect(clips.items[0]?.playbackUrl).toContain("token=bridge-token");
    expect(clips.items[0]?.downloadUrl).toContain("token=bridge-token");
  });

  it("does not rewrite or authenticate unrelated origins or RTSP responses", () => {
    const request = "https://bridge.test/api/v1/list?auth_token=bridge-token";
    expect(authenticatedBridgeResourceUrl("https://external.test/api/v1/media/play", request, "https://browser.test/bridge")).toBe("https://external.test/api/v1/media/play");
    expect(authenticatedBridgeResourceUrl("rtsp://camera/live", request)).toBe("rtsp://camera/live");
    expect(authenticatedBridgeResourceUrl("/api/v1/media/play?token=own", request)).toBe("https://bridge.test/api/v1/media/play?token=own");
  });

  it("stops export polling promptly when the operation is canceled", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch");
    const controller = new AbortController();
    const pending = waitForArchiveExportCompletion({ id: "clip", status: "recording", selfUrl: "https://bridge.test/api/v1/media/recordings/clip", playbackUrl: null, downloadUrl: null, durationMs: null, error: null }, null, controller.signal);
    const assertion = expect(pending).rejects.toMatchObject({ name: "AbortError" });
    controller.abort();
    await assertion;
    await vi.advanceTimersByTimeAsync(5000);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

function response(payload: unknown): Response {
  return { ok: true, status: 200, json: async () => payload } as Response;
}
