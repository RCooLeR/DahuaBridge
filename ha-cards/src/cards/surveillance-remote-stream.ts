import Hls from "hls.js";
import { css, html, LitElement, type PropertyValues, type TemplateResult } from "lit";
import { createRef, ref } from "lit/directives/ref.js";

import { buildWebRtcOfferUrl } from "../ha/bridge-intercom";
import type { CameraViewportSource } from "./surveillance-panel-media";

const STARTUP_TIMEOUT_MS = 10_000;
const MAX_WEBRTC_RECONNECT_ATTEMPTS = 3;

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
  private _webrtcPeer: RTCPeerConnection | null = null;
  private _webrtcStream: MediaStream | null = null;
  private _webrtcReconnectAttempts = 0;
  private _webrtcReconnectTimer: number | null = null;
  private _startupTimer: number | null = null;

  protected willUpdate(changedProperties: PropertyValues<this>): void {
    const previousDescriptor = changedProperties.get("descriptor") as RemoteStreamDescriptor | null | undefined;
    if (
      changedProperties.has("descriptor") &&
      (previousDescriptor?.cacheKey ?? "") !== (this.descriptor?.cacheKey ?? "")
    ) {
      this._activeSourceIndex = 0;
      this._webrtcReconnectAttempts = 0;
      this._hlsMediaRecoveryAttempts = 0;
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
      video.muted = this.muted;
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

  private sourceRuntimeKey(source: RemoteStreamDescriptor["sources"][number]): string {
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

    switch (source.kind) {
      case "hls":
        await this.attachHls(video, source.url, sourceKey);
        return;
      case "webrtc":
        await this.attachWebRtc(video, source.url, sourceKey);
        return;
      default:
        return;
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
      this.queuePlayback(video);
    };
    const onError = () => {
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      this.advanceToNextSource("video element error");
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

    if (canPlayNativeHls(video) || !Hls.isSupported()) {
      video.src = normalizedSource;
      video.load();
      this.queuePlayback(video);
      return;
    }

    const hls = new Hls({
      enableWorker: true,
    });
    this._hls = hls;
    this._hlsMediaRecoveryAttempts = 0;

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      this.queuePlayback(video);
    });
    hls.on(Hls.Events.ERROR, (_event, data) => {
      if (!this.isCurrentSource(sourceKey) || !data.fatal) {
        return;
      }
      if (
        data.type === Hls.ErrorTypes.MEDIA_ERROR &&
        this._hlsMediaRecoveryAttempts < 1
      ) {
        this._hlsMediaRecoveryAttempts += 1;
        hls.recoverMediaError();
        return;
      }
      this.advanceToNextSource(`hls fatal ${data.type}`);
    });
    hls.attachMedia(video);
    hls.loadSource(normalizedSource);
  }

  private async attachWebRtc(
    video: HTMLVideoElement,
    sourceUrl: string,
    sourceKey: string,
  ): Promise<void> {
    const offerUrl = buildWebRtcOfferUrl(sourceUrl);
    if (!offerUrl) {
      this.advanceToNextSource("empty webrtc offer url");
      return;
    }
    if (typeof RTCPeerConnection !== "function") {
      this.advanceToNextSource("webrtc unsupported");
      return;
    }

    this.prepareVideo(video);
    this._webrtcStream = new MediaStream();
    video.srcObject = this._webrtcStream;
    this.startStartupTimer(sourceKey);
    await this.startWebRtcNegotiation(video, offerUrl, sourceKey);
  }

  private async startWebRtcNegotiation(
    video: HTMLVideoElement,
    offerUrl: string,
    sourceKey: string,
  ): Promise<void> {
    this.clearWebRtcReconnectTimer();
    this.closeWebRtcPeer();

    const peer = new RTCPeerConnection({ iceServers: [] });
    this._webrtcPeer = peer;
    peer.addTransceiver("video", { direction: "recvonly" });
    peer.addTransceiver("audio", { direction: "recvonly" });
    peer.ontrack = (event) => {
      if (!this.isCurrentSource(sourceKey) || !this._webrtcStream) {
        return;
      }
      this._webrtcStream.addTrack(event.track);
      this._webrtcReconnectAttempts = 0;
      this.clearStartupTimer();
      this.queuePlayback(video);
    };
    peer.onconnectionstatechange = () => {
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      switch (peer.connectionState) {
        case "connected":
          this._webrtcReconnectAttempts = 0;
          this.clearStartupTimer();
          this.queuePlayback(video);
          return;
        case "disconnected":
        case "failed":
          this.scheduleWebRtcReconnect(video, offerUrl, sourceKey);
          return;
        case "closed":
          this.scheduleWebRtcReconnect(video, offerUrl, sourceKey);
          return;
        default:
          return;
      }
    };

    try {
      const offer = await peer.createOffer();
      if (!this.isCurrentSource(sourceKey)) {
        peer.close();
        return;
      }
      await peer.setLocalDescription(offer);
      await waitForIceComplete(peer);
      if (!this.isCurrentSource(sourceKey)) {
        peer.close();
        return;
      }

      const response = await fetch(offerUrl, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(peer.localDescription),
      });
      if (!response.ok) {
        throw new Error(`offer request failed with status ${response.status}`);
      }

      const answer = (await response.json()) as RTCSessionDescriptionInit;
      if (!this.isCurrentSource(sourceKey)) {
        peer.close();
        return;
      }
      await peer.setRemoteDescription(answer);
      this.queuePlayback(video);
    } catch (error) {
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      this.scheduleWebRtcReconnect(video, offerUrl, sourceKey, error);
    }
  }

  private scheduleWebRtcReconnect(
    video: HTMLVideoElement,
    offerUrl: string,
    sourceKey: string,
    error?: unknown,
  ): void {
    if (!this.isCurrentSource(sourceKey) || this._webrtcReconnectTimer !== null) {
      return;
    }

    if (this._webrtcReconnectAttempts >= MAX_WEBRTC_RECONNECT_ATTEMPTS) {
      this.advanceToNextSource(
        error instanceof Error ? error.message : "webrtc reconnect limit reached",
      );
      return;
    }

    this._webrtcReconnectAttempts += 1;
    this._webrtcReconnectTimer = window.setTimeout(() => {
      this._webrtcReconnectTimer = null;
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      void this.attachWebRtc(video, offerUrl, sourceKey);
    }, Math.min(1000 * 2 ** this._webrtcReconnectAttempts, 8_000));
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
    video.muted = this.muted;
    video.autoplay = true;
    video.playsInline = true;
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

  private startStartupTimer(sourceKey: string): void {
    this.clearStartupTimer();
    this._startupTimer = window.setTimeout(() => {
      this._startupTimer = null;
      if (!this.isCurrentSource(sourceKey)) {
        return;
      }
      this.advanceToNextSource("startup timeout");
    }, STARTUP_TIMEOUT_MS);
  }

  private clearStartupTimer(): void {
    if (this._startupTimer === null) {
      return;
    }
    window.clearTimeout(this._startupTimer);
    this._startupTimer = null;
  }

  private clearWebRtcReconnectTimer(): void {
    if (this._webrtcReconnectTimer === null) {
      return;
    }
    window.clearTimeout(this._webrtcReconnectTimer);
    this._webrtcReconnectTimer = null;
  }

  private closeWebRtcPeer(): void {
    if (!this._webrtcPeer) {
      return;
    }
    try {
      this._webrtcPeer.ontrack = null;
      this._webrtcPeer.onconnectionstatechange = null;
      this._webrtcPeer.close();
    } catch {
      // Ignore cleanup failures.
    } finally {
      this._webrtcPeer = null;
    }
  }

  private cleanupPlayback(): void {
    this.clearStartupTimer();
    this.clearWebRtcReconnectTimer();
    this._videoListenersCleanup?.();
    this._videoListenersCleanup = null;

    if (this._hls) {
      this._hls.destroy();
      this._hls = null;
    }
    this.closeWebRtcPeer();
    this._webrtcStream = null;
    this._webrtcReconnectAttempts = 0;
    this._hlsMediaRecoveryAttempts = 0;

    if (this._attachedVideo) {
      resetVideoElement(this._attachedVideo);
    }

    this._attachedVideo = null;
    this._attachedSourceKey = "";
  }
}

function canPlayNativeHls(video: HTMLVideoElement): boolean {
  return (
    video.canPlayType("application/vnd.apple.mpegurl") !== "" ||
    video.canPlayType("application/x-mpegURL") !== ""
  );
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
    if (parsed.pathname.includes("/api/v1/media/hls/") && !parsed.pathname.endsWith(".m3u8")) {
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

async function waitForIceComplete(peer: RTCPeerConnection): Promise<void> {
  if (peer.iceGatheringState === "complete") {
    return;
  }
  await new Promise<void>((resolve) => {
    const onChange = () => {
      if (peer.iceGatheringState === "complete") {
        peer.removeEventListener("icegatheringstatechange", onChange);
        resolve();
      }
    };
    peer.addEventListener("icegatheringstatechange", onChange);
  });
}

if (!customElements.get("dahuabridge-remote-stream")) {
  customElements.define("dahuabridge-remote-stream", DahuaBridgeRemoteStreamElement);
}
