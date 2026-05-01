import Hls from "hls.js";
import { html, type TemplateResult } from "lit";

import type { CameraStreamViewModel, CameraViewModel, VtoViewModel } from "../domain/model";
import type { NvrPlaybackSessionModel } from "../domain/archive";
import { buildWebRtcOfferUrl } from "../ha/bridge-intercom";
import type { HassEntity, HomeAssistant } from "../types/home-assistant";

export type CameraViewportSource = "hls" | "webrtc" | "mjpeg";

const STREAM_STYLE_ELEMENT_ID = "dahuabridge-remote-stream-style";
const streamShadowObservers = new WeakMap<Node, MutationObserver>();
const hlsAttachments = new Map<HTMLVideoElement, HlsAttachment>();
const webRtcAttachments = new Map<HTMLVideoElement, WebRtcAttachment>();

interface HlsAttachment {
  source: string;
  hls: Hls | null;
}

interface WebRtcAttachment {
  offerUrl: string;
  stream: MediaStream;
  peer: RTCPeerConnection | null;
  reconnectAttempts: number;
  reconnectTimer: number | null;
}

export function renderLiveViewport(
  hass: HomeAssistant | undefined,
  entity: HassEntity | undefined,
): TemplateResult {
  if (!entity) {
    return html`<div class="viewport empty">Stream entity unavailable.</div>`;
  }

  return html`
    <ha-camera-stream .hass=${hass} .stateObj=${entity}></ha-camera-stream>
  `;
}

export function renderSelectedCameraViewport(
  hass: HomeAssistant | undefined,
  camera: CameraViewModel,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
  muted: boolean,
): TemplateResult {
  const resolvedProfile = resolveSelectedStreamProfile(camera.stream, selectedProfileKey);
  const resolvedSource = resolveStreamViewportSource(
    camera.stream,
    selectedSource,
    resolvedProfile?.key ?? null,
  );
  const fallbackPreviewUrl = cameraImageSrc(camera.cameraEntity, camera.snapshotUrl);

  if (!camera.streamAvailable && fallbackPreviewUrl) {
    return html`
      <img
        id="remote-stream"
        class="remote-stream preview-fallback"
        src=${fallbackPreviewUrl}
        alt=${camera.label}
      />
    `;
  }

  if (resolvedSource === "hls" && resolvedProfile?.localHlsUrl) {
    return html`
      <video
        id="remote-stream"
        class="remote-stream"
        data-hls-src=${normalizeHlsPlaybackUrl(resolvedProfile.localHlsUrl)}
        data-audio-muted=${muted ? "true" : "false"}
        autoplay
        playsinline
        controls
        ?muted=${muted}
      ></video>
    `;
  }

  if (resolvedSource === "webrtc") {
    const offerUrl = buildWebRtcOfferUrl(resolvedProfile?.localWebRtcUrl ?? null);
    if (offerUrl) {
      return html`
        <video
          id="remote-stream"
          class="remote-stream"
          data-webrtc-offer-src=${offerUrl}
          data-audio-muted=${muted ? "true" : "false"}
          autoplay
          playsinline
          controls
          ?muted=${muted}
        ></video>
      `;
    }
  }

  if (resolvedSource === "mjpeg" && resolvedProfile?.localMjpegUrl) {
    return html`
      <img
        id="remote-stream"
        class="remote-stream"
        src=${resolvedProfile.localMjpegUrl}
        alt=${camera.label}
      />
    `;
  }

  if (resolvedProfile?.localHlsUrl) {
    return html`
      <video
        id="remote-stream"
        class="remote-stream"
        data-hls-src=${normalizeHlsPlaybackUrl(resolvedProfile.localHlsUrl)}
        data-audio-muted=${muted ? "true" : "false"}
        autoplay
        playsinline
        controls
        ?muted=${muted}
      ></video>
    `;
  }

  {
    const offerUrl = buildWebRtcOfferUrl(resolvedProfile?.localWebRtcUrl ?? null);
    if (offerUrl) {
      return html`
        <video
          id="remote-stream"
          class="remote-stream"
          data-webrtc-offer-src=${offerUrl}
          data-audio-muted=${muted ? "true" : "false"}
          autoplay
          playsinline
          controls
          ?muted=${muted}
        ></video>
      `;
    }
  }

  if (resolvedProfile?.localMjpegUrl) {
    return html`
      <img
        id="remote-stream"
        class="remote-stream"
        src=${resolvedProfile.localMjpegUrl}
        alt=${camera.label}
      />
    `;
  }

  return fallbackPreviewUrl
    ? html`
        <img
          id="remote-stream"
          class="remote-stream preview-fallback"
          src=${fallbackPreviewUrl}
          alt=${camera.label}
        />
      `
    : renderLiveViewport(hass, camera.cameraEntity);
}

export function renderSelectedVtoViewport(
  vto: VtoViewModel,
  playing: boolean,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
): TemplateResult {
  const fallbackPreviewUrl = cameraImageSrc(vto.cameraEntity, vto.snapshotUrl);
  const resolvedProfile = resolveSelectedStreamProfile(vto.stream, selectedProfileKey);
  const resolvedSource = resolveStreamViewportSource(
    vto.stream,
    selectedSource,
    resolvedProfile?.key ?? null,
  );

  if (!playing || !vto.streamAvailable) {
    return fallbackPreviewUrl
      ? html`
          <img
            id="remote-stream"
            class="remote-stream preview-fallback"
            src=${fallbackPreviewUrl}
            alt=${vto.label}
          />
        `
      : html`<div class="viewport empty">Stream unavailable.</div>`;
  }

  if (resolvedSource === "hls" && resolvedProfile?.localHlsUrl) {
    return html`
      <video
        id="remote-stream"
        class="remote-stream vto-live-stream"
        data-hls-src=${normalizeHlsPlaybackUrl(resolvedProfile.localHlsUrl)}
        muted
        playsinline
        preload="none"
      ></video>
    `;
  }

  if (resolvedSource === "webrtc") {
    const offerUrl = buildWebRtcOfferUrl(resolvedProfile?.localWebRtcUrl ?? null);
    if (offerUrl) {
      return html`
        <video
          id="remote-stream"
          class="remote-stream vto-live-stream"
          data-webrtc-offer-src=${offerUrl}
          data-audio-muted="true"
          muted
          playsinline
          preload="none"
        ></video>
      `;
    }
  }

  if (resolvedSource === "mjpeg" && resolvedProfile?.localMjpegUrl) {
    return html`
      <img
        id="remote-stream"
        class="remote-stream"
        src=${resolvedProfile.localMjpegUrl}
        alt=${vto.label}
      />
    `;
  }

  if (resolvedProfile?.localHlsUrl) {
    return html`
      <video
        id="remote-stream"
        class="remote-stream vto-live-stream"
        data-hls-src=${normalizeHlsPlaybackUrl(resolvedProfile.localHlsUrl)}
        muted
        playsinline
        preload="none"
      ></video>
    `;
  }

  {
    const offerUrl = buildWebRtcOfferUrl(resolvedProfile?.localWebRtcUrl ?? null);
    if (offerUrl) {
      return html`
        <video
          id="remote-stream"
          class="remote-stream vto-live-stream"
          data-webrtc-offer-src=${offerUrl}
          data-audio-muted="true"
          muted
          playsinline
          preload="none"
        ></video>
      `;
    }
  }

  if (resolvedProfile?.localMjpegUrl) {
    return html`
      <img
        id="remote-stream"
        class="remote-stream"
        src=${resolvedProfile.localMjpegUrl}
        alt=${vto.label}
      />
    `;
  }

  return fallbackPreviewUrl
    ? html`
        <img
          id="remote-stream"
          class="remote-stream preview-fallback"
          src=${fallbackPreviewUrl}
          alt=${vto.label}
        />
      `
    : html`<div class="viewport empty">Stream unavailable.</div>`;
}

export function renderPlaybackViewport(
  session: NvrPlaybackSessionModel,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
  muted: boolean,
): TemplateResult {
  const resolvedProfile = resolvePlaybackProfile(session, selectedProfileKey);
  const resolvedSource = resolvePlaybackViewportSource(session, selectedSource, resolvedProfile?.key ?? null);

  if (resolvedSource === "hls" && resolvedProfile?.hlsUrl) {
    return html`
      <video
        id="remote-stream"
        class="remote-stream playback-stream"
        data-hls-src=${normalizeHlsPlaybackUrl(resolvedProfile.hlsUrl)}
        data-audio-muted=${muted ? "true" : "false"}
        autoplay
        playsinline
        controls
        preload="auto"
        ?muted=${muted}
      ></video>
    `;
  }

  if (resolvedSource === "webrtc" && resolvedProfile?.webrtcOfferUrl) {
    return html`
      <video
        id="remote-stream"
        class="remote-stream playback-stream"
        data-webrtc-offer-src=${resolvedProfile.webrtcOfferUrl}
        data-audio-muted=${muted ? "true" : "false"}
        autoplay
        playsinline
        controls
        preload="auto"
        ?muted=${muted}
      ></video>
    `;
  }

  if (resolvedSource === "mjpeg" && resolvedProfile?.mjpegUrl) {
    return html`
      <img
        id="remote-stream"
        class="remote-stream"
        src=${resolvedProfile.mjpegUrl}
        alt=${session.name}
      />
    `;
  }

  if (resolvedProfile?.hlsUrl) {
    return html`
      <video
        id="remote-stream"
        class="remote-stream playback-stream"
        data-hls-src=${normalizeHlsPlaybackUrl(resolvedProfile.hlsUrl)}
        data-audio-muted=${muted ? "true" : "false"}
        autoplay
        playsinline
        controls
        preload="auto"
        ?muted=${muted}
      ></video>
    `;
  }

  if (resolvedProfile?.webrtcOfferUrl) {
    return html`
      <video
        id="remote-stream"
        class="remote-stream playback-stream"
        data-webrtc-offer-src=${resolvedProfile.webrtcOfferUrl}
        data-audio-muted=${muted ? "true" : "false"}
        autoplay
        playsinline
        controls
        preload="auto"
        ?muted=${muted}
      ></video>
    `;
  }

  if (resolvedProfile?.mjpegUrl) {
    return html`
      <img
        id="remote-stream"
        class="remote-stream"
        src=${resolvedProfile.mjpegUrl}
        alt=${session.name}
      />
    `;
  }

  return session.snapshotUrl
    ? html`
        <img
          id="remote-stream"
          class="remote-stream preview-fallback"
          src=${session.snapshotUrl}
          alt=${session.name}
        />
      `
    : html`<div class="viewport empty">Playback unavailable.</div>`;
}

export function syncRemoteStreamStyles(renderRoot: ParentNode): void {
  const streamHosts = renderRoot.querySelectorAll("ha-camera-stream");
  for (const streamHost of streamHosts) {
    applyHostStreamStyles(streamHost);
    applyStreamStylesInTree(streamHost);
  }
}

export function syncRemoteStreamPlayback(renderRoot: ParentNode): void {
  const activeVideos = new Set(
    Array.from(
      renderRoot.querySelectorAll<HTMLVideoElement>("video[data-hls-src], video[data-webrtc-offer-src]"),
    ),
  );

  for (const video of activeVideos) {
    const webrtcOfferUrl = normalizeWebRtcOfferUrl(video.dataset.webrtcOfferSrc);
    if (webrtcOfferUrl) {
      destroyHlsAttachment(video);
      void attachWebRtcPlayback(video, webrtcOfferUrl);
      continue;
    }

    const hlsSource = normalizeHlsPlaybackUrl(video.dataset.hlsSrc);
    if (!hlsSource) {
      destroyHlsAttachment(video);
      destroyWebRtcAttachment(video);
      continue;
    }
    destroyWebRtcAttachment(video);
    void attachHlsPlayback(video, hlsSource);
  }

  for (const [video] of hlsAttachments) {
    if (!video.isConnected || !activeVideos.has(video)) {
      destroyHlsAttachment(video);
    }
  }
  for (const [video] of webRtcAttachments) {
    if (!video.isConnected || !activeVideos.has(video)) {
      destroyWebRtcAttachment(video);
    }
  }
}

export function teardownRemoteStreamPlayback(): void {
  for (const [video] of hlsAttachments) {
    destroyHlsAttachment(video);
  }
  for (const [video] of webRtcAttachments) {
    destroyWebRtcAttachment(video);
  }
}

function applyHostStreamStyles(streamHost: Element): void {
  const hostStyle = (streamHost as HTMLElement).style;
  hostStyle.setProperty("display", "block", "important");
  hostStyle.setProperty("width", "100%", "important");
  hostStyle.setProperty("height", "100%", "important");
  hostStyle.setProperty("aspect-ratio", "16 / 9", "important");
}

function ensureShadowStreamStyles(shadowRoot: ShadowRoot): void {
  if (shadowRoot.getElementById(STREAM_STYLE_ELEMENT_ID)) {
    return;
  }

  const style = document.createElement("style");
  style.id = STREAM_STYLE_ELEMENT_ID;
  style.textContent = `
    :host {
      display: block !important;
      width: 100% !important;
      height: 100% !important;
      aspect-ratio: 16 / 9 !important;
    }

    video#remote-stream,
    video.remote-stream,
    video,
    img#remote-stream,
    img.remote-stream,
    img {
      display: block !important;
      width: 100% !important;
      height: 100% !important;
      object-fit: fill !important;
      aspect-ratio: 16 / 9 !important;
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
    streamStyle.setProperty("display", "block", "important");
    streamStyle.setProperty("width", "100%", "important");
    streamStyle.setProperty("height", "100%", "important");
    streamStyle.setProperty("object-fit", "fill", "important");
    streamStyle.setProperty("aspect-ratio", "16 / 9", "important");
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

    applyMediaElementStyles(current);

    const elements =
      current instanceof ShadowRoot
        ? current.querySelectorAll("*")
        : current.querySelectorAll("*");
    for (const element of elements) {
      const shadowRoot = element.shadowRoot;
      if (shadowRoot) {
        pending.push(shadowRoot);
      }
    }
  }
}

function observeShadowRoot(shadowRoot: ShadowRoot): void {
  if (streamShadowObservers.has(shadowRoot)) {
    return;
  }

  const observer = new MutationObserver(() => {
    applyStreamStylesInTree(shadowRoot);
  });
  observer.observe(shadowRoot, {
    childList: true,
    subtree: true,
  });
  streamShadowObservers.set(shadowRoot, observer);
}

async function attachHlsPlayback(video: HTMLVideoElement, source: string): Promise<void> {
  const existing = hlsAttachments.get(video);
  if (existing?.source === source) {
    queueVideoPlayback(video);
    return;
  }

  destroyHlsAttachment(video);
  destroyWebRtcAttachment(video);
  if (canPlayNativeHls(video)) {
    if (video.src !== source) {
      video.src = source;
    }
    video.load();
    hlsAttachments.set(video, { source, hls: null });
    queueVideoPlayback(video);
    return;
  }

  if (!video.isConnected) {
    return;
  }
  if (!Hls.isSupported()) {
    if (video.src !== source) {
      video.src = source;
    }
    video.load();
    hlsAttachments.set(video, { source, hls: null });
    queueVideoPlayback(video);
    return;
  }

  const hls = new Hls({
    enableWorker: true,
  });
  prepareVideoPlayback(video);
  queueVideoPlayback(video);
  hls.on(Hls.Events.MANIFEST_PARSED, () => {
    queueVideoPlayback(video);
  });
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (!data.fatal) {
      return;
    }
    if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
      hls.recoverMediaError();
      return;
    }
    destroyHlsAttachment(video);
  });
  video.addEventListener("loadedmetadata", () => {
    queueVideoPlayback(video);
  }, { once: true });
  video.addEventListener("canplay", () => {
    queueVideoPlayback(video);
  }, { once: true });
  hls.loadSource(source);
  hls.attachMedia(video);
  hlsAttachments.set(video, { source, hls });
}

function destroyHlsAttachment(video: HTMLVideoElement): void {
  const attachment = hlsAttachments.get(video);
  if (attachment) {
    attachment.hls?.destroy();
    hlsAttachments.delete(video);
  }
  resetVideoElement(video);
}

function canPlayNativeHls(video: HTMLVideoElement): boolean {
  return (
    video.canPlayType("application/vnd.apple.mpegurl") !== "" ||
    video.canPlayType("application/x-mpegURL") !== ""
  );
}

export function cameraImageSrc(
  entity: HassEntity | undefined,
  fallbackSnapshotUrl?: string | null,
): string {
  const fallback = fallbackSnapshotUrl ?? entity?.attributes.snapshot_url;
  return typeof fallback === "string" && fallback.trim() ? fallback : "";
}

export function resolveSelectedCameraStreamProfile(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
) {
  return resolveSelectedStreamProfile(camera.stream, selectedProfileKey);
}

export function defaultSelectedStreamProfileKey(stream: CameraStreamViewModel): string | null {
  const qualityProfile =
    stream.profiles.find((profile) => isQualityProfile(profile.key, profile.name)) ?? null;
  if (qualityProfile) {
    return qualityProfile.key;
  }
  if (stream.preferredVideoProfile) {
    const preferredProfile =
      stream.profiles.find((profile) => profile.key === stream.preferredVideoProfile) ?? null;
    if (preferredProfile) {
      return preferredProfile.key;
    }
  }
  return (stream.profiles.find((profile) => profile.recommended) ?? stream.profiles[0] ?? null)?.key ?? null;
}

export function availableCameraViewportSources(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
): CameraViewportSource[] {
  return availableStreamViewportSources(camera.stream, selectedProfileKey);
}

export function resolveSelectedCameraViewportSource(
  camera: CameraViewModel,
  selectedSource: CameraViewportSource | null,
  selectedProfileKey: string | null,
): CameraViewportSource | null {
  return resolveStreamViewportSource(camera.stream, selectedSource, selectedProfileKey);
}

export function resolveSelectedStreamProfile(
  stream: CameraStreamViewModel,
  selectedProfileKey: string | null,
) {
  if (selectedProfileKey) {
    const explicitProfile =
      stream.profiles.find((profile) => profile.key === selectedProfileKey) ?? null;
    if (explicitProfile) {
      return explicitProfile;
    }
  }

  const defaultProfileKey = defaultSelectedStreamProfileKey(stream);
  if (defaultProfileKey) {
    return stream.profiles.find((profile) => profile.key === defaultProfileKey) ?? null;
  }

  return stream.profiles[0] ?? null;
}

export function availableStreamViewportSources(
  stream: CameraStreamViewModel,
  selectedProfileKey: string | null,
): CameraViewportSource[] {
  const sources: CameraViewportSource[] = [];
  const profile = resolveSelectedStreamProfile(stream, selectedProfileKey);
  if (profile?.localHlsUrl) {
    sources.push("hls");
  }
  if (profile?.localWebRtcUrl) {
    sources.push("webrtc");
  }
  if (profile?.localMjpegUrl) {
    sources.push("mjpeg");
  }
  return sources;
}

export function resolveStreamViewportSource(
  stream: CameraStreamViewModel,
  selectedSource: CameraViewportSource | null,
  selectedProfileKey: string | null,
): CameraViewportSource | null {
  const availableSources = availableStreamViewportSources(stream, selectedProfileKey);
  if (selectedSource && availableSources.includes(selectedSource)) {
    return selectedSource;
  }
  const preferredSource = normalizeViewportSource(stream.preferredVideoSource);
  if (preferredSource && availableSources.includes(preferredSource)) {
    return preferredSource;
  }
  return availableSources[0] ?? null;
}

export function availablePlaybackViewportSources(
  session: NvrPlaybackSessionModel,
  selectedProfileKey: string | null,
): CameraViewportSource[] {
  const sources: CameraViewportSource[] = [];
  const profile = resolvePlaybackProfile(session, selectedProfileKey);
  if (profile?.hlsUrl) {
    sources.push("hls");
  }
  if (profile?.webrtcOfferUrl) {
    sources.push("webrtc");
  }
  if (profile?.mjpegUrl) {
    sources.push("mjpeg");
  }
  return sources;
}

export function resolvePlaybackViewportSource(
  session: NvrPlaybackSessionModel,
  selectedSource: CameraViewportSource | null,
  selectedProfileKey: string | null,
): CameraViewportSource | null {
  const availableSources = availablePlaybackViewportSources(session, selectedProfileKey);
  if (selectedSource && availableSources.includes(selectedSource)) {
    return selectedSource;
  }
  return availableSources[0] ?? null;
}

export function resolvePlaybackProfile(
  session: NvrPlaybackSessionModel,
  selectedProfileKey: string | null,
): { key: string; hlsUrl: string | null; mjpegUrl: string | null; webrtcOfferUrl: string | null } | null {
  const profileKey = selectedProfileKey?.trim() || session.recommendedProfile;
  if (profileKey && session.profiles[profileKey]) {
    const profile = session.profiles[profileKey];
    return {
      key: profileKey,
      hlsUrl: profile.hlsUrl,
      mjpegUrl: profile.mjpegUrl,
      webrtcOfferUrl: profile.webrtcOfferUrl,
    };
  }

  const firstEntry = Object.entries(session.profiles)[0];
  if (!firstEntry) {
    return null;
  }
  return {
    key: firstEntry[0],
    hlsUrl: firstEntry[1].hlsUrl,
    mjpegUrl: firstEntry[1].mjpegUrl,
    webrtcOfferUrl: firstEntry[1].webrtcOfferUrl,
  };
}

function prepareVideoPlayback(video: HTMLVideoElement): void {
  video.muted = requestedMuteState(video);
  video.autoplay = true;
  video.playsInline = true;
}

function queueVideoPlayback(video: HTMLVideoElement): void {
  prepareVideoPlayback(video);
  window.requestAnimationFrame(() => {
    void video.play().catch(() => undefined);
  });
}

function requestedMuteState(video: HTMLVideoElement): boolean {
  return video.dataset.audioMuted !== "false";
}

async function attachWebRtcPlayback(video: HTMLVideoElement, offerUrl: string): Promise<void> {
  const existing = webRtcAttachments.get(video);
  if (existing?.offerUrl === offerUrl) {
    queueVideoPlayback(video);
    return;
  }

  destroyWebRtcAttachment(video);
  destroyHlsAttachment(video);
  if (typeof RTCPeerConnection !== "function") {
    return;
  }

  const attachment: WebRtcAttachment = {
    offerUrl,
    stream: new MediaStream(),
    peer: null,
    reconnectAttempts: 0,
    reconnectTimer: null,
  };
  prepareVideoPlayback(video);
  video.srcObject = attachment.stream;
  webRtcAttachments.set(video, attachment);
  try {
    await startWebRtcPlayback(video, attachment);
  } catch {
    scheduleWebRtcReconnect(video, attachment);
  }
}

async function startWebRtcPlayback(video: HTMLVideoElement, attachment: WebRtcAttachment): Promise<void> {
  if (!isCurrentWebRtcAttachment(video, attachment)) {
    return;
  }

  closeWebRtcPeer(attachment);
  attachment.stream = new MediaStream();
  video.srcObject = attachment.stream;
  prepareVideoPlayback(video);

  const peer = new RTCPeerConnection({ iceServers: [] });
  attachment.peer = peer;
  peer.addTransceiver("video", { direction: "recvonly" });
  peer.addTransceiver("audio", { direction: "recvonly" });
  peer.ontrack = (event) => {
    if (!isCurrentWebRtcAttachment(video, attachment)) {
      return;
    }
    attachment.stream.addTrack(event.track);
    attachment.reconnectAttempts = 0;
    queueVideoPlayback(video);
  };
  peer.onconnectionstatechange = () => {
    if (!isCurrentWebRtcAttachment(video, attachment)) {
      return;
    }
    switch (peer.connectionState) {
      case "connected":
        attachment.reconnectAttempts = 0;
        queueVideoPlayback(video);
        return;
      case "disconnected":
      case "failed":
      case "closed":
        scheduleWebRtcReconnect(video, attachment);
        return;
      default:
        return;
    }
  };

  const offer = await peer.createOffer();
  if (!isCurrentWebRtcAttachment(video, attachment)) {
    peer.close();
    return;
  }
  await peer.setLocalDescription(offer);
  await waitForIceComplete(peer);
  if (!isCurrentWebRtcAttachment(video, attachment)) {
    peer.close();
    return;
  }

  const response = await fetch(attachment.offerUrl, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(peer.localDescription),
  });
  if (!response.ok) {
    closeWebRtcPeer(attachment);
    throw new Error(await response.text());
  }

  const answer = (await response.json()) as RTCSessionDescriptionInit;
  if (!isCurrentWebRtcAttachment(video, attachment)) {
    peer.close();
    return;
  }
  await peer.setRemoteDescription(answer);
  queueVideoPlayback(video);
}

function destroyWebRtcAttachment(video: HTMLVideoElement): void {
  const attachment = webRtcAttachments.get(video);
  if (attachment) {
    clearWebRtcReconnectTimer(attachment);
    closeWebRtcPeer(attachment);
    webRtcAttachments.delete(video);
  }
  resetVideoElement(video);
}

function scheduleWebRtcReconnect(video: HTMLVideoElement, attachment: WebRtcAttachment): void {
  if (!isCurrentWebRtcAttachment(video, attachment) || attachment.reconnectTimer !== null) {
    return;
  }

  attachment.reconnectAttempts += 1;
  const delayMs = Math.min(1000 * 2 ** Math.min(attachment.reconnectAttempts, 4), 10_000);
  attachment.reconnectTimer = window.setTimeout(() => {
    attachment.reconnectTimer = null;
    if (!isCurrentWebRtcAttachment(video, attachment)) {
      return;
    }
    void startWebRtcPlayback(video, attachment).catch(() => {
      scheduleWebRtcReconnect(video, attachment);
    });
  }, delayMs);
}

function isCurrentWebRtcAttachment(video: HTMLVideoElement, attachment: WebRtcAttachment): boolean {
  return video.isConnected && webRtcAttachments.get(video) === attachment;
}

function clearWebRtcReconnectTimer(attachment: WebRtcAttachment): void {
  if (attachment.reconnectTimer === null) {
    return;
  }
  window.clearTimeout(attachment.reconnectTimer);
  attachment.reconnectTimer = null;
}

function closeWebRtcPeer(attachment: WebRtcAttachment): void {
  const peer = attachment.peer;
  attachment.peer = null;
  if (!peer) {
    return;
  }
  try {
    peer.ontrack = null;
    peer.onconnectionstatechange = null;
    peer.close();
  } catch {
    return;
  }
}

function resetVideoElement(video: HTMLVideoElement): void {
  try {
    video.pause();
  } catch {
    return;
  } finally {
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
}

function normalizeViewportSource(value: string | null | undefined): CameraViewportSource | null {
  switch (value?.trim().toLowerCase()) {
    case "hls":
      return "hls";
    case "webrtc":
      return "webrtc";
    case "mjpeg":
      return "mjpeg";
    default:
      return null;
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

function normalizeWebRtcOfferUrl(value: string | null | undefined): string {
  return buildWebRtcOfferUrl(value?.trim() ?? null) ?? "";
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

function isQualityProfile(key: string, name: string): boolean {
  const haystack = `${key} ${name}`.trim().toLowerCase();
  return haystack === "quality quality" || haystack.includes("quality");
}
