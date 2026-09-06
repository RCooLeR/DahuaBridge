import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DahuaBridgeRemoteStreamElement, type RemoteStreamDescriptor } from "../src/cards/surveillance-remote-stream";

const engines = vi.hoisted(() => ({ callbacks: new Map<string, () => void>(), reset: vi.fn(), destroy: vi.fn(), initialize: vi.fn(), failHls: false }));
vi.mock("dashjs", () => ({ MediaPlayer: () => ({ create: () => ({ updateSettings: vi.fn(), on: vi.fn(), initialize: engines.initialize, reset: engines.reset }) }) }));
vi.mock("hls.js", () => ({ default: class {
  static isSupported = () => true;
  static Events = { MANIFEST_PARSED: "manifest", LEVEL_LOADED: "level", FRAG_LOADED: "fragment", BUFFER_APPENDED: "buffer", ERROR: "error" };
  static ErrorTypes = { MEDIA_ERROR: "media", NETWORK_ERROR: "network" };
  constructor() { if (engines.failHls) throw new Error("engine failed"); }
  on(event: string, callback: () => void) { engines.callbacks.set(event, callback); }
  attachMedia() {}
  loadSource() {}
  destroy = engines.destroy;
} }));

interface PlayerUnderTest {
  descriptor: RemoteStreamDescriptor;
  _activeSourceIndex: number;
  _sourceAbort: AbortController | null;
  _videoRef: { value: HTMLVideoElement | undefined };
  attachVideoSource(video: HTMLVideoElement, source: RemoteStreamDescriptor["sources"][number], key: string): Promise<void>;
  sourceRuntimeKey(source: RemoteStreamDescriptor["sources"][number]): string;
  updated(properties: Map<string, unknown>): void;
  disconnectedCallback(): void;
  connectedCallback(): void;
  requestUpdate(): void;
  syncAudioState(muted: boolean, volume: number): void;
}

describe("remote player lifecycle", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    engines.callbacks.clear(); engines.failHls = false;
    vi.stubGlobal("window", { setTimeout, clearTimeout, requestAnimationFrame: (callback: () => void) => setTimeout(callback, 0), cancelAnimationFrame: clearTimeout });
    vi.spyOn(console, "log").mockImplementation(() => undefined);
    vi.spyOn(console, "warn").mockImplementation(() => undefined);
  });
  afterEach(() => { vi.clearAllMocks(); vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers(); });

  it.each(["http", "network"])("ignores a stale DASH %s failure after switching source", async (failure) => {
    const { player, video } = newPlayer();
    let finish!: (value: Response) => void;
    let reject!: (error: Error) => void;
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() => new Promise((resolve, fail) => { finish = resolve; reject = fail; }));
    const old = attach(player, video, "dash");
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const oldSignal = fetchMock.mock.calls[0]?.[1]?.signal;
    await attach(player, video, "hls");
    expect(oldSignal?.aborted).toBe(true);
    if (failure === "http") finish({ ok: false, status: 503 } as Response);
    else reject(new Error("network unavailable"));
    await old;
    expect(player._activeSourceIndex).toBe(0);
    expect(engines.destroy).not.toHaveBeenCalled();
    player.disconnectedCallback();
  });

  it("aborts DASH loading on disconnect and cannot install a late player", async () => {
    const { player, video } = newPlayer();
    let finish!: (value: Response) => void;
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    const pending = attach(player, video, "dash");
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled());
    player.disconnectedCallback();
    expect(fetchMock.mock.calls[0]?.[1]?.signal?.aborted).toBe(true);
    finish({ ok: true, text: async () => "<MPD></MPD>" } as Response);
    await pending;
    expect(engines.initialize).not.toHaveBeenCalled();
  });

  it("keeps user pause across descriptor updates, buffer appends and audio synchronization", async () => {
    const { player, video } = newPlayer();
    await attach(player, video, "hls");
    video.dispatchEvent(new Event("play"));
    video.pause();
    video.play.mockClear();
    player.updated(new Map([["descriptor", player.descriptor]]));
    engines.callbacks.get("buffer")?.();
    player.syncAudioState(false, 0.5);
    await vi.advanceTimersByTimeAsync(1);
    expect(video.play).not.toHaveBeenCalled();
    expect(video.volume).toBe(0.5);
    video.dispatchEvent(new Event("play"));
    engines.callbacks.get("buffer")?.();
    await vi.advanceTimersByTimeAsync(1);
    expect(video.play).toHaveBeenCalledOnce();
    player.disconnectedCallback();
  });

  it("contains engine initialization rejection and schedules bounded recovery", async () => {
    const { player, video } = newPlayer();
    engines.failHls = true;
    await expect(attach(player, video, "hls")).resolves.toBeUndefined();
    expect(player._sourceAbort).toBeNull();
    expect(vi.getTimerCount()).toBeGreaterThan(0);
    player.disconnectedCallback();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("reattaches an unchanged source when a cached element is reinserted", async () => {
    const { player, video } = newPlayer();
    await attach(player, video, "hls");
    const previous = player._sourceAbort;
    player.disconnectedCallback();
    expect(previous?.signal.aborted).toBe(true);
    // Keep the test focused on Lit's connected -> update lifecycle without a DOM renderer.
    Object.defineProperty(player, "renderRoot", { value: {} });
    Object.defineProperty(player, "enableUpdating", { value: () => undefined });
    vi.spyOn(player, "requestUpdate").mockImplementation(() => player.updated(new Map()));
    player.connectedCallback();
    expect(player._sourceAbort).not.toBe(previous);
    expect(player._sourceAbort?.signal.aborted).toBe(false);
    await Promise.resolve();
    player.disconnectedCallback();
  });
});

function newPlayer() {
  const instance = new DahuaBridgeRemoteStreamElement();
  Object.defineProperty(instance, "isConnected", { value: true });
  const player = instance as unknown as PlayerUnderTest;
  const video = new FakeVideo();
  player._videoRef.value = video as unknown as HTMLVideoElement;
  return { player, video };
}
function attach(player: PlayerUnderTest, video: FakeVideo, kind: "hls" | "dash") {
  player.descriptor = { cacheKey: "camera", alt: "Camera", fallbackImageUrl: null, sources: [{ kind, url: `https://bridge/api/v1/media/${kind}/camera/quality/${kind === "hls" ? "index.m3u8" : "manifest.mpd"}` }] };
  const source = player.descriptor.sources[0]!;
  return player.attachVideoSource(video as unknown as HTMLVideoElement, source, player.sourceRuntimeKey(source));
}
class FakeVideo extends EventTarget {
  readyState = 2; networkState = 1; currentSrc = "blob:video"; src = ""; srcObject = null;
  paused = true; autoplay = true; playsInline = true; muted = true; defaultMuted = true; volume = 1;
  dataset: Record<string, string> = {};
  style = { setProperty: vi.fn() };
  private attributes = new Set<string>();
  canPlayType() { return ""; }
  play = vi.fn(async () => { this.paused = false; this.dispatchEvent(new Event("play")); });
  pause() { this.paused = true; this.dispatchEvent(new Event("pause")); }
  load() {}
  hasAttribute(name: string) { return this.attributes.has(name); }
  toggleAttribute(name: string, enabled: boolean) { if (enabled) this.attributes.add(name); else this.attributes.delete(name); }
  removeAttribute(name: string) { this.attributes.delete(name); }
}
