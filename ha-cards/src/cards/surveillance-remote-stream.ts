import Hls from "hls.js";
import { css, html, LitElement, type PropertyValues, type TemplateResult } from "lit";
import { createRef, ref } from "lit/directives/ref.js";

import type { CameraViewportSource } from "./surveillance-panel-viewport-sources";

const STARTUP_TIMEOUT_MS = 35_000;
const STARTUP_GRACE_TIMEOUT_MS = 15_000;
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
  private readonly _videoRef = createRef<HTMLVideoElement>();
  private _attachedVideo: HTMLVideoElement | null = null;
  private _attachedSourceKey = "";
  private _videoListenersCleanup: (() => void) | null = null;
  private _hls: Hls | null = null;
  private _hlsMediaRecoveryAttempts = 0;
  private _hlsNetworkRecoveryAttempts = 0;
  private _startupTimer: number | null = null;

  protected willUpdate(changedProperties: PropertyValues<this>): void {
    const previousDescriptor = changedProperties.get("descriptor") as
      | RemoteStreamDescriptor
      | null
      | undefined;
    if (
      changedProperties.has("descriptor") &&
      (previousDescriptor?.cacheKey ?? "") !== (this.descriptor?.cacheKey ?? "")
    ) {
      this._activeSourceIndex = 0;
      this._hlsMediaRecoveryAttempts = 0;
      this._hlsNetworkRecoveryAttempts = 0;
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
          src=${currentSource.url}
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
    this.cleanupPlayback();
    this._attachedVideo = video;
    this._attachedSourceKey = sourceKey;
    this._videoListenersCleanup = this.attachVideoListeners(video, sourceKey);
    await this.attachHls(video, source.url, sourceKey);
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
      this.queuePlayback(video);
    };
    const onError = () => {
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      const details = getVideoElementDiagnostics(video);
      console.warn("[DahuaBridge] video element error", details);
      this.advanceToNextSource(`video element error: ${details.codeName}`);
    };

    video.addEventListener("loadedmetadata", onReady);
    video.addEventListener("canplay", onReady);
    video.addEventListener("playing", onReady);
    video.addEventListener("error", onError);
    return () => {
      video.removeEventListener("loadedmetadata", onReady);
      video.removeEventListener("canplay", onReady);
      video.removeEventListener("playing", onReady);
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

    if (canPlayNativeHls(video)) {
      video.src = normalizedSource;
      video.load();
      this.queuePlayback(video);
      return;
    }

    if (!Hls.isSupported()) {
      this.advanceToNextSource("hls unsupported");
      return;
    }

    const hls = new Hls({
      enableWorker: true,
      ...HLS_RETRY_CONFIG,
    });
    this._hls = hls;
    this._hlsMediaRecoveryAttempts = 0;
    this._hlsNetworkRecoveryAttempts = 0;

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
      console.warn("[DahuaBridge] hls.js error", {
        descriptor: this.descriptor?.cacheKey ?? "",
        source: normalizedSource,
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
      this.advanceToNextSource(`hls fatal ${data.type}`);
    });
    hls.attachMedia(video);
    hls.loadSource(normalizedSource);
  }

  private advanceToNextSource(reason: string): void {
    const descriptor = this.descriptor;
    if (!descriptor) {
      return;
    }

    console.warn("[DahuaBridge] remote stream fallback", {
      reason,
      descriptor: descriptor.cacheKey,
      source_index: this._activeSourceIndex,
    });

    this.cleanupPlayback();
    if (this._activeSourceIndex < descriptor.sources.length - 1) {
      this._activeSourceIndex += 1;
    } else {
      this._activeSourceIndex = descriptor.sources.length;
    }
    this.requestUpdate("_activeSourceIndex");
  }

  private handleImageError = (): void => {
    this.advanceToNextSource("mjpeg image error");
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
      this.advanceToNextSource("startup timeout");
    }, timeoutMs);
  }

  private clearStartupTimer(): void {
    if (this._startupTimer === null) {
      return;
    }
    window.clearTimeout(this._startupTimer);
    this._startupTimer = null;
  }

  private cleanupPlayback(): void {
    this.clearStartupTimer();
    this._videoListenersCleanup?.();
    this._videoListenersCleanup = null;

    if (this._hls) {
      this._hls.destroy();
      this._hls = null;
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
}

function canPlayNativeHls(video: HTMLVideoElement): boolean {
  return (
    video.canPlayType("application/vnd.apple.mpegurl") !== "" ||
    video.canPlayType("application/x-mpegURL") !== ""
  );
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

if (!customElements.get("dahuabridge-remote-stream")) {
  customElements.define("dahuabridge-remote-stream", DahuaBridgeRemoteStreamElement);
}
