import { html, LitElement, type PropertyValues, type TemplateResult } from "lit";
import { keyed } from "lit/directives/keyed.js";

import type { NvrPlaybackSessionModel } from "../domain/archive";
import type {
  CameraStreamProfileViewModel,
  CameraViewModel,
  VtoViewModel,
} from "../domain/model";
import type { HassEntity, HomeAssistant } from "../types/home-assistant";
import {
  availableCameraViewportSources,
  availablePlaybackViewportSources,
  availableStreamViewportSources,
  defaultOverviewStreamProfileKey,
  defaultSelectedStreamProfileKey,
  preserveCameraViewportSourceSelection,
  preserveCameraViewportSourceSelectionOnProfileChange,
  preservePlaybackViewportSourceSelection,
  resolveInitialPlaybackViewportSource,
  resolveBridgeFirstStreamViewportSource,
  resolveConfiguredOverviewStreamViewportSource,
  resolveOverviewCameraViewportSource,
  resolvePlaybackProfile,
  resolvePlaybackViewportSource,
  resolvePreferredCameraViewportSource,
  resolvePreferredStreamViewportSource,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  resolveSelectedStreamProfile,
  resolveStreamViewportSource,
  type CameraViewportSource,
} from "./surveillance-panel-viewport-sources";
import {
  renderRemoteStream,
  type RemoteStreamAudioHost,
  type RemoteStreamDescriptor,
} from "./surveillance-remote-stream";
import { clampStreamVolume } from "./surveillance-panel-player-audio-model";
import { createLocalizer, type Localizer } from "../localization";

export type { CameraViewportSource } from "./surveillance-panel-viewport-sources";
export {
  availableCameraViewportSources,
  availablePlaybackViewportSources,
  availableStreamViewportSources,
  defaultOverviewStreamProfileKey,
  defaultSelectedStreamProfileKey,
  preserveCameraViewportSourceSelection,
  preserveCameraViewportSourceSelectionOnProfileChange,
  preservePlaybackViewportSourceSelection,
  resolveInitialPlaybackViewportSource,
  resolveBridgeFirstStreamViewportSource,
  resolveConfiguredOverviewStreamViewportSource,
  resolveOverviewCameraViewportSource,
  resolvePlaybackViewportSource,
  resolvePreferredCameraViewportSource,
  resolvePreferredStreamViewportSource,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  resolveStreamViewportSource,
};

const STREAM_STYLE_ELEMENT_ID = "dahuabridge-remote-stream-style";
const streamShadowObservers = new WeakMap<Node, MutationObserver>();
const streamShadowObserverSyncTimers = new WeakMap<ParentNode, number>();
const streamHostShadowRootRetries = new WeakMap<Element, { timer: number; attempts: number }>();
const viewportAudioObservers = new WeakMap<Node, MutationObserver>();
const viewportAudioShadowRootRetries = new WeakMap<Element, { timer: number; attempts: number; container: ParentNode }>();
const viewportAudioState = new WeakMap<ParentNode, ViewportAudioPlaybackState>();
const STREAM_HOST_SELECTOR = "ha-camera-stream, dahuabridge-remote-stream";
const PLAYER_AUDIO_HOST_SELECTOR = [
  "ha-hls-player",
  "ha-web-rtc-player",
  "ha-rtsp-player",
  "dahuabridge-native-fallback-stream",
].join(", ");
const MEDIA_SHADOW_HOST_SELECTOR = [
  "ha-camera-stream",
  "ha-hls-player",
  "ha-camera-mjpeg",
  "ha-web-rtc-player",
  "ha-rtsp-player",
  "hui-image",
  "dahuabridge-remote-stream",
].join(", ");
const SHADOW_ROOT_RETRY_MS = 1000;
const SHADOW_ROOT_RETRY_ATTEMPTS = 400;
const NATIVE_FALLBACK_STARTUP_TIMEOUT_MS = 90_000;
const NATIVE_FALLBACK_SCAN_MS = 1000;
const DEFAULT_BRIDGE_FALLBACK_ORDER = ["hls", "dash", "mjpeg"] as const;
const SUBSTREAM_BRIDGE_FALLBACK_ORDER = ["hls", "dash", "mjpeg"] as const;
const DEFAULT_LOCALIZER = createLocalizer("en");

interface ViewportAudioPlaybackState {
  muted: boolean;
  volume: number;
}

interface PlayerAudioHost extends HTMLElement {
  muted?: boolean;
  volume?: number;
}

export function renderLiveViewport(
  hass: HomeAssistant | undefined,
  entity: HassEntity | undefined,
  muted = true,
  volume = 1,
  t: Localizer = DEFAULT_LOCALIZER,
): TemplateResult {
  if (!entity) {
    return html`<div class="viewport empty">${t("media.streamEntityUnavailable")}</div>`;
  }

  const normalizedVolume = clampStreamVolume(volume);
  return html`${keyed(
    nativeCameraStreamKey(entity),
    html`
      <ha-camera-stream
        .hass=${hass}
        .stateObj=${entity}
        data-audio-muted=${muted ? "true" : "false"}
        data-audio-volume=${String(normalizedVolume)}
        style="display:block;width:100%;height:100%;min-width:0;min-height:0;max-width:100%;max-height:100%;aspect-ratio:16 / 9;overflow:hidden;object-fit:fill;"
      ></ha-camera-stream>
    `,
  )}`;
}

function renderNativeLiveViewportWithFallback(
  hass: HomeAssistant | undefined,
  entity: HassEntity,
  fallbackDescriptor: RemoteStreamDescriptor,
  muted: boolean,
  volume: number,
  controls: boolean,
  preload: "none" | "metadata" | "auto",
  t: Localizer,
): TemplateResult {
  if (fallbackDescriptor.sources.length === 0) {
    return renderLiveViewport(hass, entity, muted, volume, t);
  }

  return html`${keyed(
    `${nativeCameraStreamKey(entity)}:${fallbackDescriptor.cacheKey}`,
    html`
      <dahuabridge-native-fallback-stream
        .hass=${hass}
        .entity=${entity}
        .fallbackDescriptor=${fallbackDescriptor}
        .muted=${muted}
        .volume=${clampStreamVolume(volume)}
        .controls=${controls}
        .preload=${preload}
      ></dahuabridge-native-fallback-stream>
    `,
  )}`;
}

export function renderNativePlaybackViewport(
  hass: HomeAssistant | undefined,
  entity: HassEntity | undefined,
  playbackUrl: string,
  label: string,
  muted: boolean,
  volume = 1,
  t: Localizer = DEFAULT_LOCALIZER,
  fallbackPlaybackUrl: string | null = null,
): TemplateResult {
  const normalizedUrl = playbackUrl.trim();
  if (!entity) {
    return html`<div class="viewport empty">${t("media.streamEntityUnavailable")}</div>`;
  }
  if (!normalizedUrl) {
    return html`<div class="viewport empty">${t("media.archiveStreamUnavailable")}</div>`;
  }

  const fallbackUrl = fallbackPlaybackUrl?.trim() ?? "";
  const fallbackDescriptor: RemoteStreamDescriptor = {
    cacheKey: `native-playback-fallback:${fallbackUrl}`,
    alt: label,
    fallbackText: t("media.archiveStreamUnavailable"),
    fallbackImageUrl: null,
    className: "playback-stream archive-timeframe-stream",
    sources: fallbackUrl ? [{ kind: "mjpeg", url: fallbackUrl }] : [],
  };

  return renderNativeLiveViewportWithFallback(
    hass,
    overrideNativeLiveProfile(entity, normalizedUrl, null),
    fallbackDescriptor,
    muted,
    volume,
    true,
    "auto",
    t,
  );
}

export function renderTimeframePlaybackViewport(
  playbackUrl: string,
  label: string,
  muted: boolean,
  volume = 1,
  t: Localizer = DEFAULT_LOCALIZER,
): TemplateResult {
  const normalizedUrl = playbackUrl.trim();
  if (!normalizedUrl) {
    return html`<div class="viewport empty">${t("media.archiveStreamUnavailable")}</div>`;
  }

  return renderRemoteStream(
    {
      cacheKey: `timeframe:${normalizedUrl}`,
      alt: label,
      fallbackText: t("media.archiveStreamUnavailable"),
      fallbackImageUrl: null,
      sources: [{ kind: "mjpeg", url: normalizedUrl }],
    },
    { muted, volume, controls: true, preload: "auto" },
  );
}

export function renderPlaybackViewport(
  hass: HomeAssistant | undefined,
  entity: HassEntity | undefined,
  session: NvrPlaybackSessionModel,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
  muted: boolean,
  volume = 1,
  nativeStreamSource: string | null = null,
  t: Localizer = DEFAULT_LOCALIZER,
  fallbacksEnabled = true,
): TemplateResult {
  const resolvedProfile = resolvePlaybackProfile(session, selectedProfileKey);
  const normalizedNativeSource = nativeStreamSource?.trim() ?? "";
  const nativeSelected =
    selectedSource === "native" && Boolean(entity) && Boolean(normalizedNativeSource);
  const resolvedSource = nativeSelected
    ? "native"
    : resolvePlaybackViewportSource(
        session,
        selectedSource === "native" ? null : selectedSource,
        resolvedProfile?.key ?? null,
      );
  const descriptorPreferredSource = resolvedSource === "native" ? null : resolvedSource;
  const descriptorFallbackOrder =
    !fallbacksEnabled
      ? []
      : selectedSource && selectedSource !== "native"
      ? [selectedSource]
      : DEFAULT_BRIDGE_FALLBACK_ORDER;
  const descriptor = buildRemoteStreamDescriptor(
    `${session.id}:${resolvedProfile?.key ?? "none"}:${resolvedSource ?? "auto"}:${session.seekTime}`,
    session.name,
    session.snapshotUrl ?? null,
    "playback-stream",
    t("media.archiveStreamUnavailable"),
    {
      dash: resolvedProfile?.dashUrl ?? null,
      hls: resolvedProfile?.hlsUrl ?? null,
      mjpeg: resolvedProfile?.mjpegUrl ?? null,
    },
    descriptorPreferredSource,
    descriptorFallbackOrder,
  );

  if (nativeSelected && entity) {
    return renderNativeLiveViewportWithFallback(
      hass,
      overrideNativeLiveProfile(entity, normalizedNativeSource, null),
      descriptor,
      muted,
      volume,
      true,
      "auto",
      t,
    );
  }

  return renderRemoteStream(
    descriptor,
    { muted, volume, controls: true, preload: "auto" },
  );
}

export function renderSelectedCameraViewport(
  hass: HomeAssistant | undefined,
  camera: CameraViewModel,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
  muted: boolean,
  volume: number,
  options?: {
    controls?: boolean;
    preload?: "none" | "metadata" | "auto";
    fallbackOrder?: CameraViewportSource[];
    className?: string;
    t?: Localizer;
    manageAudioExternally?: boolean;
    includeSubstreamFallback?: boolean;
    fallbacksEnabled?: boolean;
  },
): TemplateResult {
  const t = options?.t ?? DEFAULT_LOCALIZER;
  const controls = options?.controls ?? true;
  const preload = options?.preload ?? "auto";
  const renderMuted = options?.manageAudioExternally ? true : muted;
  const renderVolume = options?.manageAudioExternally ? 1 : volume;
  const fallbacksEnabled =
    options?.fallbacksEnabled ?? camera.stream.fallbacksEnabled;
  const resolvedProfile = resolveSelectedStreamProfile(
    camera.stream,
    selectedProfileKey,
  );
  const resolvedSource = resolveStreamViewportSource(
    camera.stream,
    selectedSource,
    resolvedProfile?.key ?? null,
    Boolean(camera.cameraEntity),
    fallbacksEnabled,
  );
  const fallbackPreviewUrl = cameraImageSrc(
    camera.cameraEntity,
    camera.snapshotUrl,
  );
  const fallbackOrder = fallbacksEnabled
    ? options?.fallbackOrder ?? DEFAULT_BRIDGE_FALLBACK_ORDER
    : [];
  const descriptorSources = buildLiveRemoteStreamSources(
    camera,
    resolvedProfile,
    resolvedSource,
    fallbackOrder,
    fallbacksEnabled && (options?.includeSubstreamFallback ?? true),
  );
  const descriptor = buildRemoteStreamDescriptorFromSources(
    `${camera.deviceId}:${resolvedProfile?.key ?? "none"}:${resolvedSource ?? "auto"}`,
    camera.label,
    fallbackPreviewUrl,
    options?.className,
    t("media.streamUnavailable"),
    descriptorSources,
  );

  if (resolvedSource === "native" && camera.cameraEntity) {
    const nativeEntity = overrideNativeLiveProfile(
      camera.cameraEntity,
      resolvedProfile?.streamUrl ?? null,
      resolvedProfile?.subtype ?? null,
    );
    return renderNativeLiveViewportWithFallback(
      hass,
      nativeEntity,
      descriptor,
      renderMuted,
      renderVolume,
      controls,
      preload,
      t,
    );
  }

  if (!camera.streamAvailable && fallbackPreviewUrl) {
    return renderRemoteStream(
      {
        cacheKey: `${camera.deviceId}:fallback:${fallbackPreviewUrl}`,
        alt: camera.label,
        fallbackText: t("media.streamUnavailable"),
        fallbackImageUrl: fallbackPreviewUrl,
        className: options?.className,
        sources: [],
      },
      { muted: renderMuted, volume: renderVolume, controls, preload },
    );
  }

  if (descriptor.sources.length > 0 || descriptor.fallbackImageUrl) {
    return renderRemoteStream(descriptor, {
      muted: renderMuted,
      volume: renderVolume,
      controls,
      preload,
    });
  }

  return renderLiveViewport(hass, camera.cameraEntity, renderMuted, renderVolume, t);
}

export function renderSelectedVtoViewport(
  hass: HomeAssistant | undefined,
  vto: VtoViewModel,
  playing: boolean,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
  t: Localizer = DEFAULT_LOCALIZER,
  fallbacksEnabled = vto.stream.fallbacksEnabled,
): TemplateResult {
  const fallbackPreviewUrl = cameraImageSrc(vto.cameraEntity, vto.snapshotUrl);

  if (!playing) {
    return renderRemoteStream(
      {
        cacheKey: `${vto.deviceId}:fallback:${fallbackPreviewUrl}`,
        alt: vto.label,
        fallbackText: t("media.streamUnavailable"),
        fallbackImageUrl: fallbackPreviewUrl || null,
        className: "vto-live-stream",
        sources: [],
      },
      { muted: true, controls: false, preload: "none" },
    );
  }

  const resolvedProfile = resolveSelectedStreamProfile(
    vto.stream,
    selectedProfileKey,
  );
  const resolvedSource = resolveStreamViewportSource(
    vto.stream,
    selectedSource,
    resolvedProfile?.key ?? null,
    Boolean(vto.cameraEntity),
    fallbacksEnabled,
  );
  const fallbackOrder = fallbacksEnabled ? DEFAULT_BRIDGE_FALLBACK_ORDER : [];
  const descriptor = buildRemoteStreamDescriptor(
    `${vto.deviceId}:${resolvedProfile?.key ?? "none"}:${resolvedSource ?? "auto"}`,
    vto.label,
    fallbackPreviewUrl || null,
    "vto-live-stream",
    t("media.streamUnavailable"),
    {
      dash: resolvedProfile?.localDashUrl ?? null,
      hls: resolvedProfile?.localHlsUrl ?? null,
      mjpeg: resolvedProfile?.localMjpegUrl ?? null,
    },
    resolvedSource,
    fallbackOrder,
  );

  if (resolvedSource === "native" && vto.cameraEntity) {
    const nativeEntity = overrideNativeLiveProfile(
      vto.cameraEntity,
      resolvedProfile?.streamUrl ?? null,
      resolvedProfile?.subtype ?? null,
    );
    return renderNativeLiveViewportWithFallback(
      hass,
      nativeEntity,
      descriptor,
      true,
      1,
      false,
      "none",
      t,
    );
  }

  if (!vto.streamAvailable) {
    return renderRemoteStream(
      {
        cacheKey: `${vto.deviceId}:fallback:${fallbackPreviewUrl}`,
        alt: vto.label,
        fallbackText: t("media.streamUnavailable"),
        fallbackImageUrl: fallbackPreviewUrl || null,
        className: "vto-live-stream",
        sources: [],
      },
      { muted: true, controls: false, preload: "none" },
    );
  }

  return renderRemoteStream(descriptor, {
    muted: true,
    controls: false,
    preload: "none",
  });
}

export function renderClipPlaybackViewport(
  playbackUrl: string,
  label: string,
  muted: boolean,
  volume = 1,
): TemplateResult {
  const normalizedVolume = clampStreamVolume(volume);
  return html`
    <video
      class="remote-stream playback-stream local-playback-stream"
      aria-label=${label}
      src=${playbackUrl}
      controls
      preload="auto"
      autoplay
      playsinline
      .muted=${muted}
      .volume=${normalizedVolume}
      data-audio-muted=${muted ? "true" : "false"}
      data-audio-volume=${String(normalizedVolume)}
    ></video>
  `;
}

class DahuaBridgeNativeFallbackStreamElement extends LitElement {
  static properties = {
    hass: { attribute: false },
    entity: { attribute: false },
    fallbackDescriptor: { attribute: false },
    muted: { type: Boolean },
    volume: { type: Number },
    controls: { type: Boolean },
    preload: { type: String },
    _nativeFailed: { state: true },
  } as const;

  hass?: HomeAssistant;
  entity?: HassEntity;
  fallbackDescriptor: RemoteStreamDescriptor | null = null;
  muted = true;
  volume = 1;
  controls = true;
  preload: "none" | "metadata" | "auto" = "auto";

  private _nativeFailed = false;
  private _nativeSourceKey = "";
  private _startupTimer: number | null = null;
  private _scanTimer: number | null = null;
  private readonly _nativeVideoCleanups = new Map<HTMLVideoElement, () => void>();

  protected willUpdate(changedProperties: PropertyValues<this>): void {
    if (
      changedProperties.has("entity") ||
      changedProperties.has("fallbackDescriptor")
    ) {
      const nextSourceKey = this.nativeSourceKey();
      if (nextSourceKey !== this._nativeSourceKey) {
        this.cleanupNativeMonitoring();
        this._nativeFailed = false;
        this._nativeSourceKey = nextSourceKey;
      }
    }
  }

  protected updated(changedProperties: PropertyValues<this>): void {
    if (this._nativeFailed) {
      return;
    }

    syncRemoteStreamStyles(this.renderRoot);
    syncViewportAudioState(this.renderRoot, this.muted, this.volume);
    this.startNativeStartupTimer();
    this.scheduleNativeScan(0);

    if (changedProperties.has("muted") || changedProperties.has("volume")) {
      syncViewportAudioState(this.renderRoot, this.muted, this.volume);
    }
  }

  disconnectedCallback(): void {
    this.cleanupNativeMonitoring();
    super.disconnectedCallback();
  }

  render(): TemplateResult {
    if (this._nativeFailed || !this.entity) {
      return this.renderFallback();
    }
    return renderLiveViewport(
      this.hass,
      this.entity,
      this.muted,
      this.volume,
    );
  }

  private renderFallback(): TemplateResult {
    const descriptor = this.fallbackDescriptor;
    if (!descriptor) {
      return html`<div class="viewport empty"></div>`;
    }
    return renderRemoteStream(descriptor, {
      muted: this.muted,
      volume: this.volume,
      controls: this.controls,
      preload: this.preload,
    });
  }

  private startNativeStartupTimer(): void {
    if (this._startupTimer !== null || !this.fallbackDescriptor?.sources.length) {
      return;
    }
    this._startupTimer = window.setTimeout(() => {
      this._startupTimer = null;
      if (!this.hasReadyNativeVideo()) {
        this.failNative();
      }
    }, NATIVE_FALLBACK_STARTUP_TIMEOUT_MS);
  }

  private scheduleNativeScan(delayMs = NATIVE_FALLBACK_SCAN_MS): void {
    if (this._scanTimer !== null || this._nativeFailed) {
      return;
    }
    this._scanTimer = window.setTimeout(() => {
      this._scanTimer = null;
      if (!this.isConnected || this._nativeFailed) {
        return;
      }
      syncRemoteStreamStyles(this.renderRoot);
      syncViewportAudioState(this.renderRoot, this.muted, this.volume);
      const foundReadyVideo = this.attachNativeVideoListeners();
      if (foundReadyVideo) {
        this.clearStartupTimer();
        return;
      }
      this.scheduleNativeScan();
    }, delayMs);
  }

  private attachNativeVideoListeners(): boolean {
    let foundReadyVideo = false;
    for (const video of mediaVideosInTree(this.renderRoot)) {
      applyVideoAudioState(video, {
        muted: this.muted,
        volume: this.volume,
      });
      if (isNativeVideoReady(video)) {
        foundReadyVideo = true;
      }
      if (this._nativeVideoCleanups.has(video)) {
        continue;
      }

      const onReady = (): void => {
        this.clearStartupTimer();
      };
      const onError = (): void => {
        if (this.fallbackDescriptor?.sources.length) {
          this.failNative();
        }
      };
      video.addEventListener("loadedmetadata", onReady);
      video.addEventListener("canplay", onReady);
      video.addEventListener("playing", onReady);
      video.addEventListener("error", onError);
      this._nativeVideoCleanups.set(video, () => {
        video.removeEventListener("loadedmetadata", onReady);
        video.removeEventListener("canplay", onReady);
        video.removeEventListener("playing", onReady);
        video.removeEventListener("error", onError);
      });
    }
    return foundReadyVideo;
  }

  private hasReadyNativeVideo(): boolean {
    return mediaVideosInTree(this.renderRoot).some(isNativeVideoReady);
  }

  private failNative(): void {
    if (this._nativeFailed || !this.fallbackDescriptor?.sources.length) {
      return;
    }
    this.cleanupNativeMonitoring();
    this._nativeFailed = true;
    this.requestUpdate("_nativeFailed", false);
  }

  private nativeSourceKey(): string {
    const streamSource = this.entity?.attributes.stream_source;
    return [
      this.entity?.entity_id ?? "",
      typeof streamSource === "string" ? streamSource : "",
      this.fallbackDescriptor?.cacheKey ?? "",
    ].join(":");
  }

  private cleanupNativeMonitoring(): void {
    this.clearStartupTimer();
    if (this._scanTimer !== null) {
      window.clearTimeout(this._scanTimer);
      this._scanTimer = null;
    }
    for (const cleanup of this._nativeVideoCleanups.values()) {
      cleanup();
    }
    this._nativeVideoCleanups.clear();
  }

  private clearStartupTimer(): void {
    if (this._startupTimer === null) {
      return;
    }
    window.clearTimeout(this._startupTimer);
    this._startupTimer = null;
  }
}

export function syncRemoteStreamStyles(renderRoot: ParentNode): void {
  const streamHosts = renderRoot.querySelectorAll(STREAM_HOST_SELECTOR);
  for (const streamHost of streamHosts) {
    applyHostStreamStyles(streamHost);
    applyStreamStylesInTree(streamHost);
  }
}

export function syncViewportAudioState(
  container: ParentNode | null | undefined,
  muted: boolean,
  volume = 1,
): boolean {
  if (!container) {
    return false;
  }

  const state = {
    muted,
    volume: clampStreamVolume(volume),
  };
  viewportAudioState.set(container, state);
  const applied = applyViewportAudioState(container, state);
  observeViewportAudioState(container, container);
  return applied;
}

export function cameraImageSrc(
  entity: HassEntity | undefined,
  fallbackSnapshotUrl?: string | null,
): string {
  const fallback = fallbackSnapshotUrl ?? entity?.attributes.snapshot_url;
  return typeof fallback === "string" && fallback.trim() ? fallback : "";
}

export function buildRtspPlaybackUrl({
  streamUrl,
  channel,
  subtype,
  seekTime,
  endTime,
}: {
  streamUrl: string | null | undefined;
  channel?: number | null;
  subtype?: number | null;
  seekTime: string;
  endTime?: string | null;
}): string | null {
  const normalizedStreamUrl = streamUrl?.trim() ?? "";
  if (!normalizedStreamUrl) {
    return null;
  }

  try {
    const url = new URL(normalizedStreamUrl);
    if (url.protocol.toLowerCase() !== "rtsp:") {
      return null;
    }

    const start = new Date(seekTime);
    if (Number.isNaN(start.getTime())) {
      return null;
    }
    const end = endTime?.trim() ? new Date(endTime) : null;

    const resolvedChannel =
      normalizeRtspInteger(channel) ?? normalizeRtspInteger(url.searchParams.get("channel"));
    const resolvedSubtype =
      normalizeRtspInteger(subtype) ?? normalizeRtspInteger(url.searchParams.get("subtype"));
    if (resolvedChannel === null) {
      return null;
    }

    url.pathname = "/cam/playback";
    url.search = buildOrderedRtspPlaybackSearch(
      resolvedChannel,
      resolvedSubtype,
      start,
      end && !Number.isNaN(end.getTime()) && end > start ? end : null,
    );
    return url.toString();
  } catch {
    return null;
  }
}

function buildRemoteStreamDescriptor(
  cacheKey: string,
  alt: string,
  fallbackImageUrl: string | null,
  className: string | undefined,
  fallbackText: string,
  sources: Partial<Record<CameraViewportSource, string | null>>,
  preferredSource: CameraViewportSource | null,
  fallbackOrder: readonly CameraViewportSource[],
): RemoteStreamDescriptor {
  const availableSources = new Map<CameraViewportSource, string>();
  for (const [kind, url] of Object.entries(sources) as Array<
    [CameraViewportSource, string | null | undefined]
  >) {
    const normalizedUrl = url?.trim() ?? "";
    if (normalizedUrl) {
      availableSources.set(kind, normalizedUrl);
    }
  }

  const orderedKinds = uniqueSourceOrder(preferredSource, fallbackOrder);
  return buildRemoteStreamDescriptorFromSources(
    cacheKey,
    alt,
    fallbackImageUrl,
    className,
    fallbackText,
    orderedKinds.flatMap((kind) => {
      const url = availableSources.get(kind);
      return url ? [{ kind, url }] : [];
    }),
  );
}

function buildRemoteStreamDescriptorFromSources(
  cacheKey: string,
  alt: string,
  fallbackImageUrl: string | null,
  className: string | undefined,
  fallbackText: string,
  sources: RemoteStreamDescriptor["sources"],
): RemoteStreamDescriptor {
  return {
    cacheKey,
    alt,
    fallbackText,
    fallbackImageUrl,
    className,
    sources,
  };
}

function buildLiveRemoteStreamSources(
  camera: CameraViewModel,
  selectedProfile: CameraStreamProfileViewModel | null,
  preferredSource: CameraViewportSource | null,
  fallbackOrder: readonly CameraViewportSource[],
  includeSubstreamFallback: boolean,
): RemoteStreamDescriptor["sources"] {
  const sources: RemoteStreamDescriptor["sources"] = [];
  appendProfileRemoteSources(
    sources,
    selectedProfile,
    uniqueSourceOrder(preferredSource, fallbackOrder),
  );

  if (includeSubstreamFallback) {
    for (const profile of substreamFallbackProfiles(camera, selectedProfile?.key ?? null)) {
      appendProfileRemoteSources(sources, profile, SUBSTREAM_BRIDGE_FALLBACK_ORDER);
    }
  }

  return dedupeRemoteSources(sources);
}

function appendProfileRemoteSources(
  sources: RemoteStreamDescriptor["sources"],
  profile: CameraStreamProfileViewModel | null,
  order: readonly CameraViewportSource[],
): void {
  if (!profile) {
    return;
  }
  for (const kind of order) {
    switch (kind) {
      case "hls":
        appendRemoteSource(sources, "hls", profile.localHlsUrl);
        break;
      case "dash":
        appendRemoteSource(sources, "dash", profile.localDashUrl);
        break;
      case "mjpeg":
        appendRemoteSource(sources, "mjpeg", profile.localMjpegUrl);
        break;
      case "native":
        break;
    }
  }
}

function appendRemoteSource(
  sources: RemoteStreamDescriptor["sources"],
  kind: CameraViewportSource,
  rawUrl: string | null | undefined,
): void {
  const url = rawUrl?.trim() ?? "";
  if (url) {
    sources.push({ kind, url });
  }
}

function dedupeRemoteSources(
  sources: RemoteStreamDescriptor["sources"],
): RemoteStreamDescriptor["sources"] {
  const seen = new Set<string>();
  return sources.filter((source) => {
    const key = `${source.kind}:${source.url}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

function substreamFallbackProfiles(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
): CameraStreamProfileViewModel[] {
  return camera.stream.profiles.filter(
    (profile) =>
      profile.key !== selectedProfileKey &&
      isSubstreamProfile(profile) &&
      (Boolean(profile.localHlsUrl) || Boolean(profile.localDashUrl)),
  );
}

function isSubstreamProfile(profile: CameraStreamProfileViewModel): boolean {
  if (typeof profile.subtype === "number" && profile.subtype > 0) {
    return true;
  }
  const haystack = `${profile.key} ${profile.name}`.trim().toLowerCase();
  return (
    haystack.includes("stable") ||
    haystack.includes("substream") ||
    haystack.includes("sub stream") ||
    haystack.includes("preview") ||
    haystack.includes("low")
  );
}

function overrideNativeLiveProfile(
  entity: HassEntity,
  streamSource: string | null,
  subtype: number | null,
): HassEntity {
  const nextAttributes: Record<string, unknown> = {
    ...entity.attributes,
  };
  const normalizedSubtype = normalizeRtspInteger(subtype);

  if (streamSource?.trim()) {
    nextAttributes.stream_source = streamSource.trim();
  } else if (normalizedSubtype !== null) {
    const existingStreamSource = stringAttribute(entity, "stream_source");
    if (existingStreamSource) {
      nextAttributes.stream_source = overrideUrlQueryParam(existingStreamSource, "subtype", String(normalizedSubtype));
    }
  }

  if (normalizedSubtype !== null) {
    for (const key of ["entity_picture", "picture", "snapshot_url"] as const) {
      const currentValue = stringAttribute(entity, key);
      if (!currentValue) {
        continue;
      }
      nextAttributes[key] = overrideUrlQueryParam(currentValue, "subtype", String(normalizedSubtype));
    }
  }

  return {
    ...entity,
    attributes: nextAttributes,
  };
}

function nativeCameraStreamKey(entity: HassEntity): string {
  const streamSource = stringAttribute(entity, "stream_source") ?? "";
  const picture = stringAttribute(entity, "entity_picture") ?? stringAttribute(entity, "picture") ?? "";
  const snapshot = stringAttribute(entity, "snapshot_url") ?? "";
  return [
    entity.entity_id,
    streamSource,
    picture,
    snapshot,
  ].join(":");
}

function stringAttribute(entity: HassEntity, key: string): string | null {
  const value = entity.attributes[key];
  return typeof value === "string" && value.trim() ? value : null;
}

function normalizeRtspInteger(value: number | string | null | undefined): number | null {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.trunc(value);
  }
  if (typeof value === "string" && value.trim()) {
    const parsed = Number.parseInt(value, 10);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function formatRtspPlaybackTimestamp(value: Date): string {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  const hours = String(value.getHours()).padStart(2, "0");
  const minutes = String(value.getMinutes()).padStart(2, "0");
  const seconds = String(value.getSeconds()).padStart(2, "0");
  return `${year}_${month}_${day}_${hours}_${minutes}_${seconds}`;
}

function buildOrderedRtspPlaybackSearch(
  channel: number,
  subtype: number | null,
  startTime: Date,
  endTime: Date | null,
): string {
  const parts = [`channel=${encodeURIComponent(String(channel))}`];
  if (subtype !== null) {
    parts.push(`subtype=${encodeURIComponent(String(subtype))}`);
  }
  parts.push(`starttime=${encodeURIComponent(formatRtspPlaybackTimestamp(startTime))}`);
  if (endTime) {
    parts.push(`endtime=${encodeURIComponent(formatRtspPlaybackTimestamp(endTime))}`);
  }
  return `?${parts.join("&")}`;
}

function overrideUrlQueryParam(rawUrl: string, key: string, value: string): string {
  try {
    const parsed = new URL(rawUrl, window.location.origin);
    parsed.searchParams.set(key, value);
    if (/^[a-z][a-z0-9+.-]*:\/\//i.test(rawUrl)) {
      return parsed.toString();
    }
    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return rawUrl;
  }
}

function uniqueSourceOrder(
  preferredSource: CameraViewportSource | null,
  fallbackOrder: readonly CameraViewportSource[],
): CameraViewportSource[] {
  const seen = new Set<CameraViewportSource>();
  const ordered: CameraViewportSource[] = [];
  for (const candidate of [preferredSource, ...fallbackOrder]) {
    if (!candidate || seen.has(candidate)) {
      continue;
    }
    seen.add(candidate);
    ordered.push(candidate);
  }
  return ordered;
}

function applyViewportAudioState(
  container: ParentNode,
  state: ViewportAudioPlaybackState,
): boolean {
  const remoteStreams = new Set<RemoteStreamAudioHost>();
  const playerHosts = new Set<PlayerAudioHost>();
  const videos = new Set<HTMLVideoElement>();
  const pending: ParentNode[] = [container];
  const visited = new Set<ParentNode>();

  while (pending.length > 0) {
    const current = pending.pop();
    if (!current || visited.has(current)) {
      continue;
    }
    visited.add(current);

    if (current instanceof HTMLVideoElement) {
      videos.add(current);
    }
    if (
      current instanceof HTMLElement &&
      current.tagName.toLowerCase() === "dahuabridge-remote-stream"
    ) {
      remoteStreams.add(current as RemoteStreamAudioHost);
    }
    if (current instanceof HTMLElement && current.matches(PLAYER_AUDIO_HOST_SELECTOR)) {
      playerHosts.add(current as PlayerAudioHost);
    }

    for (const remoteStream of current.querySelectorAll("dahuabridge-remote-stream")) {
      if (remoteStream instanceof HTMLElement) {
        remoteStreams.add(remoteStream as RemoteStreamAudioHost);
      }
    }
    for (const playerHost of current.querySelectorAll(PLAYER_AUDIO_HOST_SELECTOR)) {
      if (playerHost instanceof HTMLElement) {
        playerHosts.add(playerHost as PlayerAudioHost);
      }
    }
    for (const video of current.querySelectorAll("video")) {
      if (video instanceof HTMLVideoElement) {
        videos.add(video);
      }
    }
    for (const element of current.querySelectorAll("*")) {
      if (element.shadowRoot) {
        pending.push(element.shadowRoot);
      }
    }
  }

  for (const remoteStream of remoteStreams) {
    remoteStream.syncAudioState(state.muted, state.volume);
  }
  for (const playerHost of playerHosts) {
    applyPlayerHostAudioState(playerHost, state);
  }
  for (const video of videos) {
    applyVideoAudioState(video, state);
  }

  return remoteStreams.size > 0 || playerHosts.size > 0 || videos.size > 0;
}

function mediaVideosInTree(root: ParentNode): HTMLVideoElement[] {
  const videos: HTMLVideoElement[] = [];
  const pending: ParentNode[] = [root];
  const visited = new Set<ParentNode>();

  while (pending.length > 0) {
    const current = pending.pop();
    if (!current || visited.has(current)) {
      continue;
    }
    visited.add(current);

    if (current instanceof HTMLVideoElement) {
      videos.push(current);
    }
    for (const video of current.querySelectorAll("video")) {
      if (video instanceof HTMLVideoElement) {
        videos.push(video);
      }
    }
    for (const element of current.querySelectorAll("*")) {
      if (element.shadowRoot) {
        pending.push(element.shadowRoot);
      }
    }
  }

  return videos;
}

function isNativeVideoReady(video: HTMLVideoElement): boolean {
  return (
    video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA ||
    (!video.paused && !video.ended)
  );
}

function observeViewportAudioState(
  root: ParentNode,
  container: ParentNode,
): void {
  if (viewportAudioObservers.has(root)) {
    return;
  }

  const observer = new MutationObserver(() => {
    const state = viewportAudioState.get(container);
    if (!state) {
      return;
    }
    applyViewportAudioState(container, state);
    observeViewportAudioShadowRoots(container, container);
  });
  observer.observe(root, {
    childList: true,
    subtree: true,
  });
  viewportAudioObservers.set(root, observer);
  observeViewportAudioShadowRoots(root, container);
}

function observeViewportAudioShadowRoots(
  root: ParentNode,
  container: ParentNode,
): void {
  const pending: ParentNode[] = [root];
  const visited = new Set<ParentNode>();

  while (pending.length > 0) {
    const current = pending.pop();
    if (!current || visited.has(current)) {
      continue;
    }
    visited.add(current);

    if (current instanceof Element && current.shadowRoot) {
      observeViewportAudioState(current.shadowRoot, container);
      pending.push(current.shadowRoot);
    } else if (current instanceof Element && shouldObservePendingMediaShadowRoot(current)) {
      observePendingViewportAudioShadowRoot(current, container);
    }

    for (const element of current.querySelectorAll("*")) {
      if (element.shadowRoot) {
        observeViewportAudioState(element.shadowRoot, container);
        pending.push(element.shadowRoot);
      } else if (shouldObservePendingMediaShadowRoot(element)) {
        observePendingViewportAudioShadowRoot(element, container);
      }
    }
  }
}

function applyVideoAudioState(
  video: HTMLVideoElement,
  state: ViewportAudioPlaybackState,
): void {
  const volume = clampStreamVolume(state.volume);
  const muted = state.muted;
  const dataMuted = muted ? "true" : "false";
  const dataVolume = String(volume);

  if (video.dataset.audioMuted !== dataMuted) {
    video.dataset.audioMuted = dataMuted;
  }
  if (video.dataset.audioVolume !== dataVolume) {
    video.dataset.audioVolume = dataVolume;
  }
  if (video.defaultMuted !== muted) {
    video.defaultMuted = muted;
  }
  if (video.muted !== muted) {
    video.muted = muted;
  }
  if (video.volume !== volume) {
    video.volume = volume;
  }
  if (video.hasAttribute("muted") !== muted) {
    video.toggleAttribute("muted", muted);
  }
  if (video.paused && (video.currentSrc.trim() || video.src.trim())) {
    void video.play().catch(() => undefined);
  }
}

function applyPlayerHostAudioState(
  playerHost: PlayerAudioHost,
  state: ViewportAudioPlaybackState,
): void {
  const volume = clampStreamVolume(state.volume);
  const dataMuted = state.muted ? "true" : "false";
  const dataVolume = String(volume);

  if (playerHost.dataset.audioMuted !== dataMuted) {
    playerHost.dataset.audioMuted = dataMuted;
  }
  if (playerHost.dataset.audioVolume !== dataVolume) {
    playerHost.dataset.audioVolume = dataVolume;
  }
  if (playerHost.muted !== state.muted) {
    playerHost.muted = state.muted;
  }
  if (playerHost.volume !== volume) {
    playerHost.volume = volume;
  }
  if (playerHost.hasAttribute("muted") !== state.muted) {
    playerHost.toggleAttribute("muted", state.muted);
  }
}

function applyHostStreamStyles(streamHost: Element): void {
  if (!(streamHost instanceof HTMLElement)) {
    return;
  }
  const hostStyle = streamHost.style;
  setImportantStyle(hostStyle, "display", "block");
  setImportantStyle(hostStyle, "width", "100%");
  setImportantStyle(hostStyle, "height", "100%");
  setImportantStyle(hostStyle, "min-width", "0");
  setImportantStyle(hostStyle, "min-height", "0");
  setImportantStyle(hostStyle, "max-width", "100%");
  setImportantStyle(hostStyle, "max-height", "100%");
  setImportantStyle(hostStyle, "aspect-ratio", "16 / 9");
  setImportantStyle(hostStyle, "overflow", "hidden");
}

function ensureShadowStreamStyles(shadowRoot: ShadowRoot): void {
  const existingStyle = shadowRoot.getElementById(STREAM_STYLE_ELEMENT_ID);
  if (existingStyle) {
    if (shadowRoot.lastElementChild !== existingStyle) {
      shadowRoot.append(existingStyle);
    }
    return;
  }

  const style = document.createElement("style");
  style.id = STREAM_STYLE_ELEMENT_ID;
  style.textContent = `
    :host {
      display: block !important;
      width: 100% !important;
      height: 100% !important;
      min-width: 0 !important;
      min-height: 0 !important;
      max-width: 100% !important;
      max-height: 100% !important;
      aspect-ratio: 16 / 9 !important;
      overflow: hidden !important;
    }

    video#remote-stream,
    video.remote-stream,
    video,
    img#remote-stream,
    img.remote-stream,
    img,
    .viewport-empty {
      display: block !important;
      width: 100% !important;
      height: 100% !important;
      min-width: 0 !important;
      min-height: 0 !important;
      max-width: 100% !important;
      max-height: 100% !important;
      aspect-ratio: 16 / 9 !important;
    }

    video,
    img {
      object-fit: fill !important;
      object-position: center center !important;
    }

    img[src*="logo"],
    img[src*="Logo"],
    img[alt*="logo"],
    img[alt*="Logo"] {
      width: 50% !important;
      height: 50% !important;
      object-fit: contain !important;
      margin: auto !important;
      transform: translateY(50%) !important;
    }
  `;

  shadowRoot.append(style);
}

function applyMediaElementStyles(root: ParentNode): void {
  const remoteStreams = root.querySelectorAll(
    "video#remote-stream, video.remote-stream, video, img#remote-stream, img.remote-stream, img",
  );
  for (const remoteStream of remoteStreams) {
    const streamStyle = (remoteStream as HTMLElement).style;
    setImportantStyle(streamStyle, "display", "block");
    setImportantStyle(streamStyle, "width", "100%");
    setImportantStyle(streamStyle, "height", "100%");
    setImportantStyle(streamStyle, "min-width", "0");
    setImportantStyle(streamStyle, "min-height", "0");
    setImportantStyle(streamStyle, "max-width", "100%");
    setImportantStyle(streamStyle, "max-height", "100%");
    setImportantStyle(streamStyle, "object-fit", "fill");
    setImportantStyle(streamStyle, "object-position", "center center");
    setImportantStyle(streamStyle, "aspect-ratio", "16 / 9");
  }
}

function applyStreamStylesInTree(root: ParentNode): void {
  const pending: ParentNode[] = [root];
  const visited = new Set<ParentNode>();

  while (pending.length > 0) {
    const current = pending.pop();
    if (!current || visited.has(current)) {
      continue;
    }
    visited.add(current);

    if (current instanceof ShadowRoot) {
      ensureShadowStreamStyles(current);
      observeShadowRoot(current);
    }

    if (current instanceof Element && current.shadowRoot) {
      pending.push(current.shadowRoot);
    } else if (current instanceof Element && shouldObservePendingMediaShadowRoot(current)) {
      observePendingStreamHostShadowRoot(current);
    }

    applyMediaElementStyles(current);

    for (const element of current.querySelectorAll("*")) {
      if (element.shadowRoot) {
        pending.push(element.shadowRoot);
      } else if (shouldObservePendingMediaShadowRoot(element)) {
        observePendingStreamHostShadowRoot(element);
      }
    }
  }
}

function observePendingStreamHostShadowRoot(streamHost: Element): void {
  if (streamHost.shadowRoot) {
    applyHostStreamStyles(streamHost);
    applyStreamStylesInTree(streamHost.shadowRoot);
    return;
  }
  if (streamHostShadowRootRetries.has(streamHost)) {
    return;
  }

  const scheduleRetry = (attempts: number): void => {
    const timer = window.setTimeout(() => {
      if (!streamHost.isConnected || attempts >= SHADOW_ROOT_RETRY_ATTEMPTS) {
        streamHostShadowRootRetries.delete(streamHost);
        return;
      }

      applyHostStreamStyles(streamHost);
      if (streamHost.shadowRoot) {
        applyStreamStylesInTree(streamHost.shadowRoot);
        streamHostShadowRootRetries.delete(streamHost);
        return;
      }

      scheduleRetry(attempts + 1);
    }, SHADOW_ROOT_RETRY_MS);
    streamHostShadowRootRetries.set(streamHost, { timer, attempts });
  };

  scheduleRetry(0);
}

function observePendingViewportAudioShadowRoot(
  streamHost: Element,
  container: ParentNode,
): void {
  if (streamHost.shadowRoot) {
    observeViewportAudioState(streamHost.shadowRoot, container);
    const state = viewportAudioState.get(container);
    if (state) {
      applyViewportAudioState(container, state);
    }
    return;
  }
  if (viewportAudioShadowRootRetries.has(streamHost)) {
    return;
  }

  const scheduleRetry = (attempts: number): void => {
    const timer = window.setTimeout(() => {
      if (!streamHost.isConnected || attempts >= SHADOW_ROOT_RETRY_ATTEMPTS) {
        viewportAudioShadowRootRetries.delete(streamHost);
        return;
      }

      if (streamHost.shadowRoot) {
        observeViewportAudioState(streamHost.shadowRoot, container);
        const state = viewportAudioState.get(container);
        if (state) {
          applyViewportAudioState(container, state);
        }
        viewportAudioShadowRootRetries.delete(streamHost);
        return;
      }

      scheduleRetry(attempts + 1);
    }, SHADOW_ROOT_RETRY_MS);
    viewportAudioShadowRootRetries.set(streamHost, { timer, attempts, container });
  };

  scheduleRetry(0);
}

function shouldObservePendingMediaShadowRoot(element: Element): boolean {
  if (element.matches(MEDIA_SHADOW_HOST_SELECTOR)) {
    return true;
  }
  const tagName = element.localName.toLowerCase();
  return (
    tagName.includes("-") &&
    /(?:camera|stream|player|video|media|hls|mjpeg|webrtc|rtc)/u.test(tagName)
  );
}

function setImportantStyle(
  style: CSSStyleDeclaration,
  property: string,
  value: string,
): void {
  if (
    style.getPropertyValue(property) === value &&
    style.getPropertyPriority(property) === "important"
  ) {
    return;
  }
  style.setProperty(property, value, "important");
}

function observeShadowRoot(shadowRoot: ShadowRoot): void {
  if (streamShadowObservers.has(shadowRoot)) {
    return;
  }

  const observer = new MutationObserver(() => {
    scheduleObservedStreamStyleSync(shadowRoot);
  });
  observer.observe(shadowRoot, {
    childList: true,
    subtree: true,
  });
  streamShadowObservers.set(shadowRoot, observer);
}

function scheduleObservedStreamStyleSync(root: ParentNode): void {
  if (streamShadowObserverSyncTimers.has(root)) {
    return;
  }

  const timer = window.requestAnimationFrame(() => {
    streamShadowObserverSyncTimers.delete(root);
    applyStreamStylesInTree(root);
  });
  streamShadowObserverSyncTimers.set(root, timer);
}

if (!customElements.get("dahuabridge-native-fallback-stream")) {
  customElements.define(
    "dahuabridge-native-fallback-stream",
    DahuaBridgeNativeFallbackStreamElement,
  );
}
