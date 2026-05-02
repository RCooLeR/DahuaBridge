import Hls from "hls.js";
import { css, html, LitElement, type PropertyValues, type TemplateResult } from "lit";
import { createRef, ref } from "lit/directives/ref.js";

import type { CameraViewportSource } from "./surveillance-panel-viewport-sources";
import {
  clearCardLogState,
  logCardWarn,
  redactUrlForLog,
  setCardLogState,
} from "../utils/logging";

const STARTUP_TIMEOUT_MS = 35_000;
const STARTUP_GRACE_TIMEOUT_MS = 15_000;
const EXHAUSTED_SOURCE_RETRY_MS = 60_000;
const SOURCE_RETRY_DELAY_MS = 1_500;
const REMOTE_STREAM_LOG_SCOPE = "remote_streams";
const MAX_SOURCE_FAILURE_RETRIES = 3;
const MAX_HLS_NETWORK_RECOVERY_ATTEMPTS = 2;
const HLS_RETRY_CONFIG = {
  manifestLoadingMaxRetry: 6,
  manifestLoadingRetryDelay: 1_000,
  manifestLoadingMaxRetryTimeout: 16_000,
  levelLoadingMaxRetry: 6,
  levelLoadingRetryDelay: 1_000,
  levelLoadingMaxRetryTimeout: 16_000,
  fragLoadingMaxRetry: 8,
  fragLoadingRetryDelay: 1_000,
  fragLoadingMaxRetryTimeout: 16_000,
} as const;

export interface RemoteStreamDescriptor {
  cacheKey: string;
  alt: string;
  fallbackImageUrl: string | null;
  className?: string;
  sources: Array<{
    kind: CameraViewportSource;
    url: string;
  }>;
}

export interface RemoteStreamAudioHost extends HTMLElement {
  syncAudioState(muted: boolean): void;
}

interface DashPlayerLike {
  initialize(view: HTMLVideoElement, source: string, autoPlay: boolean): void;
  updateSettings?(settings: Record<string, unknown>): void;
  on?(event: string, listener: (...args: unknown[]) => void): void;
  off?(event: string, listener: (...args: unknown[]) => void): void;
  reset(): void;
}

const DASH_EVENT_STREAM_INITIALIZED = "streamInitialized";
const DASH_EVENT_CAN_PLAY = "canPlay";
const DASH_EVENT_PLAYBACK_STARTED = "playbackStarted";
const DASH_EVENT_ERROR = "error";
const DASH_EVENT_PLAYBACK_ERROR = "playbackError";

export function renderRemoteStream(
  descriptor: RemoteStreamDescriptor,
  options: {
    muted: boolean;
    controls?: boolean;
    preload?: "none" | "metadata" | "auto";
  },
): TemplateResult {
  return html`
    <dahuabridge-remote-stream
      .descriptor=${descriptor}
      .muted=${options.muted}
      .controls=${options.controls ?? true}
      .preload=${options.preload ?? "auto"}
    ></dahuabridge-remote-stream>
  `;
}

class DahuaBridgeRemoteStreamElement extends LitElement {
  static properties = {
    descriptor: { attribute: false },
    muted: { type: Boolean },
    controls: { type: Boolean },
    preload: { type: String },
    _activeSourceIndex: { state: true },
    _sourceRevision: { state: true },
  } as const;

  static styles = css`
    :host {
      display: block;
      width: 100%;
      height: 100%;
      aspect-ratio: 16 / 9;
      background: #06101a;
    }

    video,
    img,
    .viewport-empty {
      display: block;
      width: 100%;
      height: 100%;
      aspect-ratio: 16 / 9;
    }

    video,
    img {
      object-fit: fill;
    }

    .viewport-empty {
      display: grid;
      place-items: center;
      color: rgba(232, 242, 255, 0.72);
      font-size: 0.82rem;
      background: rgba(5, 13, 21, 0.92);
    }
  `;

  descriptor: RemoteStreamDescriptor | null = null;
  muted = true;
  controls = true;
  preload: "none" | "metadata" | "auto" = "auto";

  private _activeSourceIndex = 0;
  private _sourceRevision = 0;
  private readonly _videoRef = createRef<HTMLVideoElement>();
  private _attachedVideo: HTMLVideoElement | null = null;
  private _attachedSourceKey = "";
  private _videoListenersCleanup: (() => void) | null = null;
  private _hls: Hls | null = null;
  private _dash: DashPlayerLike | null = null;
  private _hlsMediaRecoveryAttempts = 0;
  private _hlsNetworkRecoveryAttempts = 0;
  private _startupTimer: number | null = null;
  private _retryTimer: number | null = null;
  private _sourceRetryTimer: number | null = null;
  private readonly _sourceFailureCounts = new Map<string, number>();

  connectedCallback(): void {
    super.connectedCallback();
    this.updateRemoteStreamLogState("connected");
  }

  protected willUpdate(changedProperties: PropertyValues<this>): void {
    const previousDescriptor = changedProperties.get("descriptor") as
      | RemoteStreamDescriptor
      | null
      | undefined;
    if (
      changedProperties.has("descriptor") &&
      (previousDescriptor?.cacheKey ?? "") !== (this.descriptor?.cacheKey ?? "")
    ) {
      this.clearRemoteStreamLogState(previousDescriptor?.cacheKey ?? "");
      this.clearSourceRetryTimer();
      this.clearRetryTimer();
      this._activeSourceIndex = 0;
      this._sourceRevision = 0;
      this._hlsMediaRecoveryAttempts = 0;
      this._hlsNetworkRecoveryAttempts = 0;
      this._sourceFailureCounts.clear();
      this.cleanupPlayback();
    }
  }

  protected updated(changedProperties: PropertyValues<this>): void {
    const currentSource = this.currentSource();
    const video = this._videoRef.value ?? null;

    if (!currentSource || !video || currentSource.kind === "mjpeg") {
      this.cleanupPlayback();
      return;
    }

    if (changedProperties.has("muted")) {
      this.applyAudioState(video, this.muted);
    }

    const sourceKey = this.sourceRuntimeKey(currentSource);
    if (this._attachedVideo === video && this._attachedSourceKey === sourceKey) {
      this.queuePlayback(video);
      return;
    }

    void this.attachVideoSource(video, currentSource, sourceKey);
  }

  disconnectedCallback(): void {
    this.clearRemoteStreamLogState();
    this.clearSourceRetryTimer();
    this.clearRetryTimer();
    this.cleanupPlayback();
    super.disconnectedCallback();
  }

  syncAudioState(muted: boolean): void {
    const previousMuted = this.muted;
    this.muted = muted;
    this.requestUpdate("muted", previousMuted);
    this.applyAudioState(this._videoRef.value ?? this._attachedVideo, muted, true);
  }

  render(): TemplateResult {
    const currentSource = this.currentSource();
    const className = this.streamClassName();

    if (!currentSource) {
      return this.renderFallback();
    }

    if (currentSource.kind === "mjpeg") {
      return html`
        <img
          class=${className}
          src=${this.renderSourceUrl(currentSource)}
          alt=${this.descriptor?.alt ?? "Remote stream"}
          @error=${this.handleImageError}
        />
      `;
    }

    return html`
      <video
        ${ref(this._videoRef)}
        class=${className}
        autoplay
        playsinline
        ?controls=${this.controls}
        preload=${this.preload}
        ?muted=${this.muted}
      ></video>
    `;
  }

  private renderFallback(): TemplateResult {
    const fallbackImageUrl = this.descriptor?.fallbackImageUrl?.trim() ?? "";
    if (fallbackImageUrl) {
      return html`
        <img
          class=${this.streamClassName("preview-fallback")}
          src=${fallbackImageUrl}
          alt=${this.descriptor?.alt ?? "Remote stream"}
        />
      `;
    }

    return html`<div class="viewport-empty">Stream unavailable.</div>`;
  }

  private currentSource():
    | RemoteStreamDescriptor["sources"][number]
    | null {
    if (!this.descriptor) {
      return null;
    }
    return this.descriptor.sources[this._activeSourceIndex] ?? null;
  }

  private sourceRuntimeKey(
    source: RemoteStreamDescriptor["sources"][number],
  ): string {
    return `${this.sourceIdentityKey(source)}:r${this._sourceRevision}`;
  }

  private sourceIdentityKey(
    source: RemoteStreamDescriptor["sources"][number],
  ): string {
    return `${this.descriptor?.cacheKey ?? ""}:${this._activeSourceIndex}:${source.kind}:${source.url}`;
  }

  private streamClassName(extraClassName?: string): string {
    const classes = ["remote-stream"];
    if (this.descriptor?.className?.trim()) {
      classes.push(this.descriptor.className.trim());
    }
    if (extraClassName?.trim()) {
      classes.push(extraClassName.trim());
    }
    return classes.join(" ");
  }

  private async attachVideoSource(
    video: HTMLVideoElement,
    source: RemoteStreamDescriptor["sources"][number],
    sourceKey: string,
  ): Promise<void> {
    this.clearRetryTimer();
    this.clearSourceRetryTimer();
    this.cleanupPlayback();
    this._attachedVideo = video;
    this._attachedSourceKey = sourceKey;
    this._videoListenersCleanup = this.attachVideoListeners(video, sourceKey);
    this.updateRemoteStreamLogState("attaching", {
      ...this.streamLogContext(source),
      source_key: sourceKey,
    });
    switch (source.kind) {
      case "dash":
        await this.attachDash(video, source.url, sourceKey);
        return;
      case "hls":
        await this.attachHls(video, source.url, sourceKey);
        return;
      default:
        this.advanceToNextSource(`unsupported source kind: ${source.kind}`);
    }
  }

  private attachVideoListeners(
    video: HTMLVideoElement,
    sourceKey: string,
  ): () => void {
    const onReady = () => {
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      this.clearStartupTimer();
      this.clearCurrentSourceFailureCount();
      this.updateRemoteStreamLogState("ready", {
        ...this.streamLogContext(),
        source_key: sourceKey,
        ready_state: video.readyState,
        network_state: video.networkState,
      });
      this.queuePlayback(video);
    };
    const onError = () => {
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      const details = getVideoElementDiagnostics(video);
      this.updateRemoteStreamLogState("error", {
        ...this.streamLogContext(),
        source_key: sourceKey,
        reason: details.codeName,
      });
      logCardWarn("card remote stream video element error", {
        ...this.streamLogContext(),
        ...details,
      });
      this.advanceToNextSource(`video element error: ${details.codeName}`, {
        retryable: isRetryableVideoElementError(details),
      });
    };
    const onPlay = () => {
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      this.updateRemoteStreamLogState("playing", {
        ...this.streamLogContext(),
        source_key: sourceKey,
        current_src: redactUrlForLog(video.currentSrc),
      });
    };

    video.addEventListener("loadedmetadata", onReady);
    video.addEventListener("canplay", onReady);
    video.addEventListener("playing", onReady);
    video.addEventListener("play", onPlay);
    video.addEventListener("error", onError);
    return () => {
      video.removeEventListener("loadedmetadata", onReady);
      video.removeEventListener("canplay", onReady);
      video.removeEventListener("playing", onReady);
      video.removeEventListener("play", onPlay);
      video.removeEventListener("error", onError);
    };
  }

  private async attachHls(
    video: HTMLVideoElement,
    sourceUrl: string,
    sourceKey: string,
  ): Promise<void> {
    const normalizedSource = normalizeHlsPlaybackUrl(sourceUrl);
    if (!normalizedSource) {
      this.advanceToNextSource("empty hls source");
      return;
    }

    this.prepareVideo(video);
    this.startStartupTimer(sourceKey);

    const playbackMode = resolveHlsPlaybackMode({
      hlsJsSupported: Hls.isSupported(),
      nativeHlsSupported: canPlayNativeHls(video),
    });

    if (playbackMode === "hls.js") {
      const hls = new Hls({
        enableWorker: true,
        ...HLS_RETRY_CONFIG,
      });
      this._hls = hls;
      this._hlsMediaRecoveryAttempts = 0;
      this._hlsNetworkRecoveryAttempts = 0;
      this.updateRemoteStreamLogState("attaching", {
        ...this.streamLogContext({ kind: "hls", url: normalizedSource }),
        source_key: sourceKey,
        playback_mode: "hls.js",
      });

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        if (!this.isCurrentSource(sourceKey)) {
          return;
        }
        this.startStartupTimer(sourceKey, STARTUP_GRACE_TIMEOUT_MS);
        this._hlsNetworkRecoveryAttempts = 0;
        this.queuePlayback(video);
      });
      hls.on(Hls.Events.LEVEL_LOADED, () => {
        if (!this.isCurrentSource(sourceKey)) {
          return;
        }
        this.startStartupTimer(sourceKey, STARTUP_GRACE_TIMEOUT_MS);
      });
      hls.on(Hls.Events.FRAG_LOADED, () => {
        if (!this.isCurrentSource(sourceKey)) {
          return;
        }
        this.startStartupTimer(sourceKey, STARTUP_GRACE_TIMEOUT_MS);
        this._hlsNetworkRecoveryAttempts = 0;
      });
      hls.on(Hls.Events.BUFFER_APPENDED, () => {
        if (!this.isCurrentSource(sourceKey)) {
          return;
        }
        this.clearStartupTimer();
        this.queuePlayback(video);
      });
      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (!this.isCurrentSource(sourceKey)) {
          return;
        }
        logCardWarn("card remote stream hls.js error", {
          ...this.streamLogContext({ kind: "hls", url: normalizedSource }),
          fatal: data.fatal,
          type: data.type,
          details: data.details,
          reason: data.reason,
          response_code: data.response?.code,
        });
        if (!data.fatal) {
          return;
        }
        if (
          data.type === Hls.ErrorTypes.MEDIA_ERROR &&
          this._hlsMediaRecoveryAttempts < 1
        ) {
          this._hlsMediaRecoveryAttempts += 1;
          this.startStartupTimer(sourceKey, STARTUP_GRACE_TIMEOUT_MS);
          hls.recoverMediaError();
          return;
        }
        if (
          data.type === Hls.ErrorTypes.NETWORK_ERROR &&
          this._hlsNetworkRecoveryAttempts < MAX_HLS_NETWORK_RECOVERY_ATTEMPTS
        ) {
          this._hlsNetworkRecoveryAttempts += 1;
          this.startStartupTimer(sourceKey, STARTUP_GRACE_TIMEOUT_MS);
          hls.startLoad(-1);
          return;
        }
        this.advanceToNextSource(`hls fatal ${data.type}`, {
          retryable: true,
        });
      });
      hls.attachMedia(video);
      hls.loadSource(normalizedSource);
      return;
    }

    if (playbackMode === "native") {
      this.updateRemoteStreamLogState("attaching", {
        ...this.streamLogContext({ kind: "hls", url: normalizedSource }),
        source_key: sourceKey,
        playback_mode: "native-hls",
      });
      video.src = normalizedSource;
      video.load();
      this.queuePlayback(video);
      return;
    }

    if (playbackMode === "unsupported") {
      this.advanceToNextSource("hls unsupported");
      return;
    }
  }

  private async attachDash(
    video: HTMLVideoElement,
    sourceUrl: string,
    sourceKey: string,
  ): Promise<void> {
    const normalizedSource = normalizeDashPlaybackUrl(sourceUrl);
    if (!normalizedSource) {
      this.advanceToNextSource("empty dash source");
      return;
    }

    const dashModule = (await import("dashjs")) as unknown as {
      MediaPlayer?: () => {
        create: () => DashPlayerLike;
      };
    };
    const playerFactory = dashModule.MediaPlayer;
    if (!playerFactory) {
      this.advanceToNextSource("dash.js unavailable");
      return;
    }

    this.prepareVideo(video);
    this.startStartupTimer(sourceKey);

    try {
      const manifestResponse = await fetch(normalizedSource, {
        cache: "no-store",
        headers: {
          Accept: "application/dash+xml,application/xml,text/xml;q=0.9,*/*;q=0.1",
        },
      });
      if (!manifestResponse.ok) {
        this.advanceToNextSource(
          `dash manifest request failed: ${manifestResponse.status}`,
          { retryable: manifestResponse.status >= 500 },
        );
        return;
      }
      const manifestText = await manifestResponse.text();
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      if (!manifestText.includes("<MPD")) {
        this.advanceToNextSource("dash manifest invalid", { retryable: true });
        return;
      }
    } catch (error) {
      logCardWarn("card remote stream dash manifest request failed", {
        ...this.streamLogContext({ kind: "dash", url: normalizedSource }),
        source_key: sourceKey,
        error: error instanceof Error ? error.message : String(error),
      });
      this.advanceToNextSource("dash manifest request failed", { retryable: true });
      return;
    }

    try {
      const player = playerFactory().create();
      this._dash = player;
      player.updateSettings?.({
        debug: {
          logLevel: 0,
        },
        streaming: {
          scheduleWhilePaused: false,
          fastSwitchEnabled: false,
          lowLatencyEnabled: false,
        },
      });
      this.updateRemoteStreamLogState("attaching", {
        ...this.streamLogContext({ kind: "dash", url: normalizedSource }),
        source_key: sourceKey,
        playback_mode: "dash.js",
      });
      const onReady = () => {
        if (!this.isCurrentSource(sourceKey)) {
          return;
        }
        this.clearStartupTimer();
        this.clearCurrentSourceFailureCount();
        this.queuePlayback(video);
      };
      const onError = (...args: unknown[]) => {
        if (!this.isCurrentSource(sourceKey)) {
          return;
        }
        logCardWarn("card remote stream dash.js error", {
          ...this.streamLogContext({ kind: "dash", url: normalizedSource }),
          source_key: sourceKey,
          args_count: args.length,
          event_type:
            args.length > 0 &&
            args[0] &&
            typeof args[0] === "object" &&
            "type" in (args[0] as Record<string, unknown>)
              ? (args[0] as Record<string, unknown>).type ?? null
              : null,
        });
        this.advanceToNextSource("dash player error", { retryable: true });
      };
      player.on?.(DASH_EVENT_STREAM_INITIALIZED, onReady);
      player.on?.(DASH_EVENT_CAN_PLAY, onReady);
      player.on?.(DASH_EVENT_PLAYBACK_STARTED, onReady);
      player.on?.(DASH_EVENT_ERROR, onError);
      player.on?.(DASH_EVENT_PLAYBACK_ERROR, onError);
      player.initialize(video, normalizedSource, false);
    } catch (error) {
      logCardWarn("card remote stream dash attach failed", {
        ...this.streamLogContext({ kind: "dash", url: normalizedSource }),
        source_key: sourceKey,
        error: error instanceof Error ? error.message : String(error),
      });
      this.advanceToNextSource("dash attach failed", { retryable: true });
    }
  }

  private advanceToNextSource(
    reason: string,
    options: {
      retryable?: boolean;
    } = {},
  ): void {
    const descriptor = this.descriptor;
    if (!descriptor) {
      return;
    }

    logCardWarn("card remote stream fallback", {
      reason,
      descriptor: descriptor.cacheKey,
      source_index: this._activeSourceIndex,
    });
    this.updateRemoteStreamLogState("fallback", {
      ...this.streamLogContext(),
      reason,
    });

    const currentSource = this.currentSource();
    const currentFailureCount = currentSource
      ? this._sourceFailureCounts.get(this.sourceIdentityKey(currentSource)) ?? 0
      : 0;
    const action = resolveSourceFailureAction({
      sourceIndex: this._activeSourceIndex,
      sourceCount: descriptor.sources.length,
      retryable: options.retryable ?? false,
      attempt: currentFailureCount,
      maxAttempts: MAX_SOURCE_FAILURE_RETRIES,
    });

    this.cleanupPlayback();
    if (action.retryCurrentSource) {
      if (currentSource) {
        this._sourceFailureCounts.set(
          this.sourceIdentityKey(currentSource),
          action.nextAttempt,
        );
      }
      this.scheduleCurrentSourceRetry(reason, action.nextAttempt);
      return;
    }

    this.clearCurrentSourceFailureCount();
    this._activeSourceIndex = action.nextIndex;
    this._sourceRevision = 0;
    if (action.retryExhaustedSources) {
      this.scheduleRetry();
    }
    this.requestUpdate("_activeSourceIndex");
  }

  private handleImageError = (): void => {
    this.advanceToNextSource("mjpeg image error", { retryable: true });
  };

  private prepareVideo(video: HTMLVideoElement): void {
    this.applyAudioState(video, this.muted);
    video.autoplay = true;
    video.playsInline = true;
  }

  private applyAudioState(
    video: HTMLVideoElement | null,
    muted: boolean,
    playImmediately = false,
  ): void {
    if (!video) {
      return;
    }
    video.dataset.audioMuted = muted ? "true" : "false";
    video.muted = muted;
    if (playImmediately) {
      void video.play().catch(() => undefined);
    }
  }

  private queuePlayback(video: HTMLVideoElement): void {
    this.prepareVideo(video);
    window.requestAnimationFrame(() => {
      void video.play().catch(() => undefined);
    });
  }

  private isCurrentSource(sourceKey: string): boolean {
    return this.isConnected && this._attachedSourceKey === sourceKey;
  }

  private startStartupTimer(
    sourceKey: string,
    timeoutMs = STARTUP_TIMEOUT_MS,
  ): void {
    this.clearStartupTimer();
    this._startupTimer = window.setTimeout(() => {
      this._startupTimer = null;
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      if (this.shouldExtendStartupTimer()) {
        this.startStartupTimer(sourceKey, STARTUP_GRACE_TIMEOUT_MS);
        return;
      }
      this.advanceToNextSource("startup timeout", { retryable: true });
    }, timeoutMs);
  }

  private clearStartupTimer(): void {
    if (this._startupTimer === null) {
      return;
    }
    window.clearTimeout(this._startupTimer);
    this._startupTimer = null;
  }

  private scheduleRetry(): void {
    if (this._retryTimer !== null || !this.descriptor || this.descriptor.sources.length === 0) {
      return;
    }
    this.updateRemoteStreamLogState("retry_wait", {
      ...this.streamLogContext(),
      retry_delay_ms: EXHAUSTED_SOURCE_RETRY_MS,
    });
    this._retryTimer = window.setTimeout(() => {
      this._retryTimer = null;
      if (!this.isConnected || !this.descriptor || this.descriptor.sources.length === 0) {
        return;
      }
      this.updateRemoteStreamLogState("retrying", this.streamLogContext());
      this._activeSourceIndex = 0;
      this.requestUpdate("_activeSourceIndex");
    }, EXHAUSTED_SOURCE_RETRY_MS);
  }

  private scheduleCurrentSourceRetry(reason: string, attempt: number): void {
    this.clearSourceRetryTimer();
    this.updateRemoteStreamLogState("retry_source_wait", {
      ...this.streamLogContext(),
      retry_reason: reason,
      retry_attempt: attempt,
      retry_delay_ms: SOURCE_RETRY_DELAY_MS,
    });
    this._sourceRetryTimer = window.setTimeout(() => {
      this._sourceRetryTimer = null;
      if (!this.isConnected || !this.currentSource()) {
        return;
      }
      this._sourceRevision += 1;
      this.updateRemoteStreamLogState("retry_source", {
        ...this.streamLogContext(),
        retry_reason: reason,
        retry_attempt: attempt,
      });
      this.requestUpdate("_sourceRevision");
    }, SOURCE_RETRY_DELAY_MS);
  }

  private clearSourceRetryTimer(): void {
    if (this._sourceRetryTimer === null) {
      return;
    }
    window.clearTimeout(this._sourceRetryTimer);
    this._sourceRetryTimer = null;
  }

  private clearRetryTimer(): void {
    if (this._retryTimer === null) {
      return;
    }
    window.clearTimeout(this._retryTimer);
    this._retryTimer = null;
  }

  private cleanupPlayback(): void {
    this.clearStartupTimer();
    this._videoListenersCleanup?.();
    this._videoListenersCleanup = null;

    if (this._hls) {
      this._hls.destroy();
      this._hls = null;
    }
    if (this._dash) {
      try {
        this._dash.reset();
      } catch {
        // dash.js can throw while tearing down a worker that is still parsing startup state.
      }
      this._dash = null;
    }
    this._hlsMediaRecoveryAttempts = 0;
    this._hlsNetworkRecoveryAttempts = 0;

    if (this._attachedVideo) {
      resetVideoElement(this._attachedVideo);
    }

    this._attachedVideo = null;
    this._attachedSourceKey = "";
  }

  private shouldExtendStartupTimer(): boolean {
    const video = this._attachedVideo;
    if (!video || !this._hls) {
      return false;
    }

    return (
      video.readyState > HTMLMediaElement.HAVE_NOTHING ||
      video.networkState === HTMLMediaElement.NETWORK_LOADING ||
      video.currentSrc.trim().length > 0
    );
  }

  private streamLogContext(
    source?: RemoteStreamDescriptor["sources"][number] | null,
  ): Record<string, unknown> {
    const currentSource = source ?? this.currentSource();
    return {
      descriptor: this.descriptor?.cacheKey ?? "",
      source_index: this._activeSourceIndex,
      source_kind: currentSource?.kind ?? null,
      source_url: currentSource ? redactUrlForLog(this.renderSourceUrl(currentSource)) : null,
      source_retry_attempt:
        currentSource ? this._sourceFailureCounts.get(this.sourceIdentityKey(currentSource)) ?? 0 : 0,
      muted: this.muted,
      controls: this.controls,
    };
  }

  private renderSourceUrl(
    source: RemoteStreamDescriptor["sources"][number],
  ): string {
    if (this._sourceRevision <= 0) {
      return source.url;
    }
    return appendRetryQueryParam(source.url, this._sourceRevision);
  }

  private clearCurrentSourceFailureCount(): void {
    const currentSource = this.currentSource();
    if (!currentSource) {
      return;
    }
    this._sourceFailureCounts.delete(this.sourceIdentityKey(currentSource));
  }

  private updateRemoteStreamLogState(
    state: string,
    details: Record<string, unknown> = {},
  ): void {
    const cacheKey = this.descriptor?.cacheKey?.trim() ?? "";
    if (!cacheKey) {
      return;
    }
    setCardLogState(REMOTE_STREAM_LOG_SCOPE, cacheKey, {
      state,
      source_count: this.descriptor?.sources.length ?? 0,
      retry_scheduled: this._retryTimer !== null,
      source_retry_scheduled: this._sourceRetryTimer !== null,
      ...details,
    });
  }

  private clearRemoteStreamLogState(cacheKey?: string): void {
    const targetKey = (cacheKey ?? this.descriptor?.cacheKey ?? "").trim();
    if (!targetKey) {
      return;
    }
    clearCardLogState(REMOTE_STREAM_LOG_SCOPE, targetKey);
  }
}

export function resolveSourceFailureTransition(
  sourceIndex: number,
  sourceCount: number,
): {
  nextIndex: number;
  retry: boolean;
} {
  if (sourceCount <= 0) {
    return {
      nextIndex: 0,
      retry: false,
    };
  }
  if (sourceIndex < sourceCount - 1) {
    return {
      nextIndex: sourceIndex + 1,
      retry: false,
    };
  }
  return {
    nextIndex: sourceCount,
    retry: true,
  };
}

export function resolveSourceFailureAction(input: {
  sourceIndex: number;
  sourceCount: number;
  retryable: boolean;
  attempt: number;
  maxAttempts: number;
}): {
  nextIndex: number;
  nextAttempt: number;
  retryCurrentSource: boolean;
  retryExhaustedSources: boolean;
} {
  if (
    input.retryable &&
    input.sourceCount > 0 &&
    input.sourceIndex >= 0 &&
    input.sourceIndex < input.sourceCount &&
    input.attempt < input.maxAttempts
  ) {
    return {
      nextIndex: input.sourceIndex,
      nextAttempt: input.attempt + 1,
      retryCurrentSource: true,
      retryExhaustedSources: false,
    };
  }

  const transition = resolveSourceFailureTransition(input.sourceIndex, input.sourceCount);
  return {
    nextIndex: transition.nextIndex,
    nextAttempt: input.attempt,
    retryCurrentSource: false,
    retryExhaustedSources: transition.retry,
  };
}

function canPlayNativeHls(video: HTMLVideoElement): boolean {
  return (
    video.canPlayType("application/vnd.apple.mpegurl") !== "" ||
    video.canPlayType("application/x-mpegURL") !== ""
  );
}

export function resolveHlsPlaybackMode(capabilities: {
  hlsJsSupported: boolean;
  nativeHlsSupported: boolean;
}): "hls.js" | "native" | "unsupported" {
  if (capabilities.hlsJsSupported) {
    return "hls.js";
  }
  if (capabilities.nativeHlsSupported) {
    return "native";
  }
  return "unsupported";
}

function getVideoElementDiagnostics(video: HTMLVideoElement): {
  code: number | null;
  codeName: string;
  message: string;
  currentSrc: string;
  networkState: number;
  readyState: number;
} {
  const error = video.error;
  const code = error?.code ?? null;
  const messageValue = error && "message" in error ? error.message : "";
  return {
    code,
    codeName: describeMediaErrorCode(code),
    message: typeof messageValue === "string" ? messageValue.trim() : "",
    currentSrc: video.currentSrc,
    networkState: video.networkState,
    readyState: video.readyState,
  };
}

function describeMediaErrorCode(code: number | null): string {
  switch (code) {
    case 1:
      return "MEDIA_ERR_ABORTED";
    case 2:
      return "MEDIA_ERR_NETWORK";
    case 3:
      return "MEDIA_ERR_DECODE";
    case 4:
      return "MEDIA_ERR_SRC_NOT_SUPPORTED";
    case null:
      return "MEDIA_ERR_UNKNOWN";
    default:
      return `MEDIA_ERR_${code}`;
  }
}

function isRetryableVideoElementError(details: {
  code: number | null;
  networkState: number;
  readyState: number;
}): boolean {
  if (details.networkState === HTMLMediaElement.NETWORK_NO_SOURCE) {
    return false;
  }

  switch (details.code) {
    case MediaError.MEDIA_ERR_NETWORK:
    case MediaError.MEDIA_ERR_DECODE:
      return true;
    case MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED:
      return details.readyState > HTMLMediaElement.HAVE_NOTHING;
    default:
      return false;
  }
}

function appendRetryQueryParam(sourceUrl: string, revision: number): string {
  if (revision <= 0) {
    return sourceUrl;
  }

  try {
    const parsed = new URL(sourceUrl, globalThis.location?.href);
    parsed.searchParams.set("_retry", String(revision));
    return parsed.toString();
  } catch {
    const separator = sourceUrl.includes("?") ? "&" : "?";
    return `${sourceUrl}${separator}_retry=${encodeURIComponent(String(revision))}`;
  }
}

function resetVideoElement(video: HTMLVideoElement): void {
  try {
    video.pause();
  } catch {
    // Ignore.
  }

  try {
    video.srcObject = null;
  } catch {
    // Ignore.
  }
  try {
    video.removeAttribute("src");
    video.src = "";
  } catch {
    // Ignore.
  }
  try {
    video.load();
  } catch {
    // Ignore.
  }
}

function normalizeHlsPlaybackUrl(value: string | null | undefined): string {
  const source = value?.trim() ?? "";
  if (!source) {
    return "";
  }
  try {
    const parsed = new URL(source, globalThis.location?.href);
    if (
      parsed.pathname.includes("/api/v1/media/hls/") &&
      !parsed.pathname.endsWith(".m3u8")
    ) {
      parsed.pathname = `${parsed.pathname.replace(/\/+$/, "")}/index.m3u8`;
    }
    return parsed.toString();
  } catch {
    if (source.includes("/api/v1/media/hls/") && !source.includes(".m3u8")) {
      return `${source.replace(/\/+$/, "")}/index.m3u8`;
    }
    return source;
  }
}

function normalizeDashPlaybackUrl(value: string | null | undefined): string {
  return value?.trim() ?? "";
}

if (!customElements.get("dahuabridge-remote-stream")) {
  customElements.define("dahuabridge-remote-stream", DahuaBridgeRemoteStreamElement);
}
