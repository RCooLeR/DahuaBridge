import Hls from "hls.js";
import { html, type TemplateResult } from "lit";

import type { CameraStreamViewModel, CameraViewModel, VtoViewModel } from "../domain/model";
import type { NvrPlaybackSessionModel } from "../domain/archive";
import { buildWebRtcOfferUrl } from "../ha/bridge-intercom";
import type { HassEntity, HomeAssistant } from "../types/home-assistant";
import {
  renderRemoteStream,
  type RemoteStreamDescriptor,
} from "./surveillance-remote-stream";

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
  options?: {
    controls?: boolean;
    preload?: "none" | "metadata" | "auto";
    fallbackOrder?: CameraViewportSource[];
    className?: string;
  },
): TemplateResult {
  const controls = options?.controls ?? true;
  const preload = options?.preload ?? "auto";
  const resolvedProfile = resolveSelectedStreamProfile(camera.stream, selectedProfileKey);
  const resolvedSource = resolveStreamViewportSource(
    camera.stream,
    selectedSource,
    resolvedProfile?.key ?? null,
  );
  const fallbackPreviewUrl = cameraImageSrc(camera.cameraEntity, camera.snapshotUrl);

  if (!camera.streamAvailable && fallbackPreviewUrl) {
    return renderRemoteStream(
      {
        cacheKey: `${camera.deviceId}:fallback:${fallbackPreviewUrl}`,
        alt: camera.label,
        fallbackImageUrl: fallbackPreviewUrl,
        className: options?.className,
        sources: [],
      },
      { muted, controls, preload },
    );
  }

  const descriptor = buildRemoteStreamDescriptor(
    `${camera.deviceId}:${resolvedProfile?.key ?? "none"}:${resolvedSource ?? "auto"}`,
    camera.label,
    fallbackPreviewUrl,
    options?.className,
    {
      hls: resolvedProfile?.localHlsUrl ?? null,
      webrtc: buildWebRtcOfferUrl(resolvedProfile?.localWebRtcUrl ?? null),
      mjpeg: resolvedProfile?.localMjpegUrl ?? null,
    },
    resolvedSource,
    options?.fallbackOrder ?? ["hls", "webrtc", "mjpeg"],
  );
  if (descriptor.sources.length > 0 || descriptor.fallbackImageUrl) {
    return renderRemoteStream(descriptor, { muted, controls, preload });
  }

  return renderLiveViewport(hass, camera.cameraEntity);
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
    return renderRemoteStream(
      {
        cacheKey: `${vto.deviceId}:fallback:${fallbackPreviewUrl}`,
        alt: vto.label,
        fallbackImageUrl: fallbackPreviewUrl || null,
        className: "vto-live-stream",
        sources: [],
      },
      { muted: true, controls: false, preload: "none" },
    );
  }

  return renderRemoteStream(
    buildRemoteStreamDescriptor(
      `${vto.deviceId}:${resolvedProfile?.key ?? "none"}:${resolvedSource ?? "auto"}`,
      vto.label,
      fallbackPreviewUrl || null,
      "vto-live-stream",
      {
        hls: resolvedProfile?.localHlsUrl ?? null,
        webrtc: buildWebRtcOfferUrl(resolvedProfile?.localWebRtcUrl ?? null),
        mjpeg: resolvedProfile?.localMjpegUrl ?? null,
      },
      resolvedSource,
      ["hls", "webrtc", "mjpeg"],
    ),
    { muted: true, controls: false, preload: "none" },
  );
}

export function renderPlaybackViewport(
  session: NvrPlaybackSessionModel,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
  muted: boolean,
): TemplateResult {
  const resolvedProfile = resolvePlaybackProfile(session, selectedProfileKey);
  const resolvedSource = resolvePlaybackViewportSource(session, selectedSource, resolvedProfile?.key ?? null);
  return renderRemoteStream(
    buildRemoteStreamDescriptor(
      `${session.id}:${resolvedProfile?.key ?? "none"}:${resolvedSource ?? "auto"}:${session.seekTime}`,
      session.name,
      session.snapshotUrl ?? null,
      "playback-stream",
      {
        hls: resolvedProfile?.hlsUrl ?? null,
        webrtc: resolvedProfile?.webrtcOfferUrl ?? null,
        mjpeg: resolvedProfile?.mjpegUrl ?? null,
      },
      resolvedSource,
      ["hls", "webrtc", "mjpeg"],
    ),
    { muted, controls: true, preload: "auto" },
  );
}

function buildRemoteStreamDescriptor(
  cacheKey: string,
  alt: string,
  fallbackImageUrl: string | null,
  className: string | undefined,
  sources: Partial<Record<CameraViewportSource, string | null>>,
  preferredSource: CameraViewportSource | null,
  fallbackOrder: readonly CameraViewportSource[],
): RemoteStreamDescriptor {
  const availableSources = new Map<CameraViewportSource, string>();
  for (const [kind, url] of Object.entries(sources) as Array<[CameraViewportSource, string | null | undefined]>) {
    const normalizedUrl = url?.trim() ?? "";
    if (normalizedUrl) {
      availableSources.set(kind, normalizedUrl);
    }
  }

  const orderedKinds = uniqueSourceOrder(preferredSource, fallbackOrder);
  return {
    cacheKey,
    alt,
    fallbackImageUrl,
    className,
    sources: orderedKinds.flatMap((kind) => {
      const url = availableSources.get(kind);
      return url ? [{ kind, url }] : [];
    }),
  };
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

export function defaultOverviewStreamProfileKey(stream: CameraStreamViewModel): string | null {
  const bandwidthProfile =
    stream.profiles.find((profile) => isBandwidthOptimizedProfile(profile.key, profile.name)) ??
    null;
  if (bandwidthProfile) {
    return bandwidthProfile.key;
  }

  const recommendedNonQuality =
    stream.profiles.find(
      (profile) => profile.recommended && !isQualityProfile(profile.key, profile.name),
    ) ?? null;
  if (recommendedNonQuality) {
    return recommendedNonQuality.key;
  }

  return defaultSelectedStreamProfileKey(stream);
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

export function resolveOverviewCameraViewportSource(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
): CameraViewportSource | null {
  return selectSourceByPriority(
    availableCameraViewportSources(camera, selectedProfileKey),
    ["hls", "mjpeg", "webrtc"],
  );
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

export function resolveInitialPlaybackViewportSource(
  session: NvrPlaybackSessionModel,
  selectedProfileKey: string | null,
  previousSource: CameraViewportSource | null,
): CameraViewportSource | null {
  const availableSources = availablePlaybackViewportSources(session, selectedProfileKey);
  if (previousSource === "webrtc" && availableSources.includes("webrtc")) {
    return "webrtc";
  }
  if (availableSources.includes("hls")) {
    return "hls";
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

  const offerPayload = buildWebRtcOfferPayload(peer);
  if (!offerPayload) {
    closeWebRtcPeer(attachment);
    throw new Error("WebRTC offer SDP is empty");
  }

  const response = await fetch(attachment.offerUrl, {
    method: "POST",
    headers: {
      "Accept": "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(offerPayload),
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

function buildWebRtcOfferPayload(peer: RTCPeerConnection): RTCSessionDescriptionInit | null {
  const description = peer.localDescription;
  const type = description?.type;
  const rawSdp = description?.sdp ?? "";
  const sdp = rawSdp
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .split("\n")
    .map((line) => line.trimEnd())
    .filter((line) => line.length > 0)
    .join("\r\n");

  if (!type || !sdp.trim()) {
    return null;
  }

  return {
    type,
    sdp: `${sdp}\r\n`,
  };
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

function isBandwidthOptimizedProfile(key: string, name: string): boolean {
  const haystack = `${key} ${name}`.trim().toLowerCase();
  return (
    haystack.includes("stable") ||
    haystack.includes("substream") ||
    haystack.includes("sub stream") ||
    haystack.includes("preview") ||
    haystack.includes("low")
  );
}

function selectSourceByPriority(
  availableSources: readonly CameraViewportSource[],
  priority: readonly CameraViewportSource[],
): CameraViewportSource | null {
  for (const candidate of priority) {
    if (availableSources.includes(candidate)) {
      return candidate;
    }
  }
  return availableSources[0] ?? null;
}
