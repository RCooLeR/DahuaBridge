import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DahuaBridgeSurveillancePanelCard } from "../src/cards/surveillance-panel-card";
import type { CameraViewModel, PanelModel } from "../src/domain/model";
import type { BridgeRecordingClipModel, NvrArchiveRecordingModel, NvrPlaybackSessionModel } from "../src/domain/archive";
import { createPlaybackSession } from "../src/ha/bridge-playback";
import { exportArchiveRecording, waitForArchiveExportCompletion } from "../src/ha/bridge-archive";
import type { HomeAssistant } from "../src/types/home-assistant";

vi.hoisted(() => { vi.stubGlobal("window", { customCards: [] }); });

vi.mock("../src/ha/bridge-playback", () => ({ createPlaybackSession: vi.fn() }));
vi.mock("../src/ha/bridge-archive", () => ({ exportArchiveRecording: vi.fn(), waitForArchiveExportCompletion: vi.fn(), fetchArchiveRecordings: vi.fn(), fetchBridgeRecordings: vi.fn() }));

interface PanelUnderTest {
  _selectedPlayback: { session: NvrPlaybackSessionModel } | null;
  _selectedBridgeRecordingPlayback: { recording: BridgeRecordingClipModel } | null;
  _errorMessage: string;
  _selectedNativePlayback: { seekTime: string } | null;
  hass: HomeAssistant;
  startBridgeArchivePlayback(camera: CameraViewModel, start: Date, end: Date): Promise<boolean>;
  stopSelectedPlayback(): void;
  resetSharedSelectionViewState(): void;
  disconnectedCallback(): void;
  playBridgeRecording(camera: CameraViewModel, recording: BridgeRecordingClipModel): void;
  launchArchiveClipPlayback(model: PanelModel, recording: NvrArchiveRecordingModel): Promise<void>;
  resolveArchiveSource(model: PanelModel): CameraViewModel;
  startNativeArchivePlayback(camera: CameraViewModel, start: Date): Promise<boolean>;
  stopSelectedNativePlayback(): void;
  buildNativeArchivePlaybackSource(camera: CameraViewModel, start: Date): string;
}

describe("panel playback lifecycle", () => {
  beforeEach(() => {
    vi.spyOn(console, "log").mockImplementation(() => undefined);
    vi.stubGlobal("window", { location: { origin: "https://ha.test" }, setTimeout, clearTimeout, clearInterval });
  });
  afterEach(() => { vi.clearAllMocks(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  it("keeps the latest seek when session responses arrive out of order", async () => {
    const panel = newPanel();
    const first = deferred<NvrPlaybackSessionModel>();
    const second = deferred<NvrPlaybackSessionModel>();
    vi.mocked(createPlaybackSession).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const old = start(panel, 1);
    const next = start(panel, 2);
    expect(vi.mocked(createPlaybackSession).mock.calls[0]?.[3]?.aborted).toBe(true);
    second.resolve(session("new"));
    expect(await next).toBe(true);
    first.resolve(session("old"));
    expect(await old).toBe(false);
    expect(panel._selectedPlayback?.session.id).toBe("new");
  });

  it.each(["stopSelectedPlayback", "resetSharedSelectionViewState", "disconnectedCallback"] as const)("invalidates a late session on %s", async (method) => {
    const panel = newPanel();
    const result = deferred<NvrPlaybackSessionModel>();
    vi.mocked(createPlaybackSession).mockReturnValueOnce(result.promise);
    const pending = start(panel, 1);
    panel[method]();
    result.resolve(session("obsolete"));
    expect(await pending).toBe(false);
    expect(panel._selectedPlayback).toBeNull();
    expect(panel._errorMessage).toBe("");
  });

  it("aborts an export and ignores its late completion after selecting an indexed clip", async () => {
    const panel = newPanel();
    const result = deferred<Awaited<ReturnType<typeof waitForArchiveExportCompletion>>>();
    vi.mocked(exportArchiveRecording).mockResolvedValueOnce({ id: "old", status: "recording", selfUrl: "https://bridge/api/v1/media/recordings/old", playbackUrl: null, downloadUrl: null, durationMs: null, error: null });
    vi.mocked(waitForArchiveExportCompletion).mockReturnValueOnce(result.promise);
    const pending = panel.launchArchiveClipPlayback({} as PanelModel, { id: "old", exportUrl: "https://bridge/api/v1/export", channel: 1, startTime: "2026-09-06T00:00:00Z", endTime: "2026-09-06T00:10:00Z" } as NvrArchiveRecordingModel);
    await vi.waitFor(() => expect(waitForArchiveExportCompletion).toHaveBeenCalled());
    panel.playBridgeRecording(camera, { id: "new", playbackUrl: "https://bridge/api/v1/media/recordings/new/play" } as BridgeRecordingClipModel);
    expect(vi.mocked(waitForArchiveExportCompletion).mock.calls[0]?.[2]?.aborted).toBe(true);
    result.resolve({ id: "old", status: "completed", selfUrl: null, playbackUrl: "https://bridge/api/v1/old/play", downloadUrl: null, durationMs: null, error: null });
    await pending;
    expect(panel._selectedBridgeRecordingPlayback?.recording.id).toBe("new");
    expect(panel._errorMessage).toBe("");
  });

  it("serializes a canceled legacy HA source change before clearing and applying the next seek", async () => {
    const panel = newPanel();
    const firstCall = deferred<void>();
    const callService = vi.fn().mockReturnValueOnce(firstCall.promise).mockResolvedValue(undefined);
    panel.hass = { states: { "camera.cam": { entity_id: "camera.cam", state: "streaming", attributes: {} } }, callService } as unknown as HomeAssistant;
    panel.buildNativeArchivePlaybackSource = (_camera, time) => `rtsp://nvr/cam/playback?start=${time.toISOString()}`;
    const first = panel.startNativeArchivePlayback(camera, new Date("2026-09-06T01:00:00Z"));
    await vi.waitFor(() => expect(callService).toHaveBeenCalledTimes(1));
    panel.stopSelectedNativePlayback();
    const next = panel.startNativeArchivePlayback(camera, new Date("2026-09-06T02:00:00Z"));
    firstCall.resolve();
    expect(await first).toBe(false);
    expect(await next).toBe(true);
    expect(callService.mock.calls.map((call) => call[1])).toEqual(["set_native_playback_source", "clear_native_playback_source", "set_native_playback_source"]);
    expect(panel._selectedNativePlayback?.seekTime).toBe("2026-09-06T02:00:00.000Z");
  });
});

const camera = { deviceId: "cam", rootDeviceId: "nvr", channelNumber: 1, label: "Camera", cameraEntityId: "camera.cam", cameraEntity: { entity_id: "camera.cam", attributes: { bridge_playback_sessions_url: "https://bridge/api/v1/nvr/nvr/playback/sessions" } }, bridgeBaseUrl: "https://bridge", stream: { profiles: [] } } as unknown as CameraViewModel;
function newPanel(): PanelUnderTest {
  const panel = new DahuaBridgeSurveillancePanelCard() as unknown as PanelUnderTest;
  panel.resolveArchiveSource = () => camera;
  return panel;
}
function start(panel: PanelUnderTest, hour: number): Promise<boolean> {
  return panel.startBridgeArchivePlayback(camera, new Date(`2026-09-06T0${hour}:00:00Z`), new Date(`2026-09-06T0${hour}:10:00Z`));
}
function session(id: string): NvrPlaybackSessionModel {
  return { id, streamId: id, deviceId: "nvr", sourceStreamId: "cam", name: "Camera", channel: 1, startTime: "2026-09-06T00:00:00Z", endTime: "2026-09-06T01:00:00Z", seekTime: "2026-09-06T00:00:00Z", recommendedProfile: "quality", snapshotUrl: null, createdAt: "", expiresAt: "", profiles: { quality: { name: "Quality", hlsUrl: `https://bridge/api/v1/media/hls/${id}/quality/index.m3u8`, dashUrl: null, mjpegUrl: null, webrtcOfferUrl: null } } };
}
function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
