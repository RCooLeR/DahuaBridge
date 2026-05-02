import { html, type TemplateResult } from "lit";

import type { NvrPlaybackSessionModel } from "../domain/archive";
import type {
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
  resolveOverviewCameraViewportSource,
  resolvePlaybackProfile,
  resolvePlaybackViewportSource,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  resolveSelectedStreamProfile,
  resolveStreamViewportSource,
  type CameraViewportSource,
} from "./surveillance-panel-viewport-sources";
import {
  renderRemoteStream,
  type RemoteStreamDescriptor, RemoteStreamAudioHost,
} from "./surveillance-remote-stream";

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
  resolveOverviewCameraViewportSource,
  resolvePlaybackViewportSource,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  resolveStreamViewportSource,
};

const STREAM_STYLE_ELEMENT_ID = "dahuabridge-remote-stream-style";
const streamShadowObservers = new WeakMap<Node, MutationObserver>();
const viewportAudioObservers = new WeakMap<Node, MutationObserver>();
const viewportAudioState = new WeakMap<ParentNode, boolean>();
const STREAM_HOST_SELECTOR = "ha-camera-stream, dahuabridge-remote-stream";

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
  const resolvedProfile = resolveSelectedStreamProfile(
    camera.stream,
    selectedProfileKey,
  );
  const resolvedSource = resolveStreamViewportSource(
    camera.stream,
    selectedSource,
    resolvedProfile?.key ?? null,
    Boolean(camera.cameraEntity),
  );
  const fallbackPreviewUrl = cameraImageSrc(
    camera.cameraEntity,
    camera.snapshotUrl,
  );

  if (resolvedSource === "native" && camera.cameraEntity) {
    return renderLiveViewport(hass, camera.cameraEntity);
  }

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
      dash: resolvedProfile?.localDashUrl ?? null,
      hls: resolvedProfile?.localHlsUrl ?? null,
      mjpeg: resolvedProfile?.localMjpegUrl ?? null,
    },
    resolvedSource,
    selectedSource
      ? [selectedSource]
      : (options?.fallbackOrder ?? ["hls", "dash", "mjpeg"]),
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
  const resolvedProfile = resolveSelectedStreamProfile(
    vto.stream,
    selectedProfileKey,
  );
  const resolvedSource = resolveStreamViewportSource(
    vto.stream,
    selectedSource,
    resolvedProfile?.key ?? null,
    Boolean(vto.cameraEntity),
  );

  if (!playing) {
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

  if (resolvedSource === "native" && vto.cameraEntity) {
    return renderLiveViewport(undefined, vto.cameraEntity);
  }

  if (!vto.streamAvailable) {
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
        dash: resolvedProfile?.localDashUrl ?? null,
        hls: resolvedProfile?.localHlsUrl ?? null,
        mjpeg: resolvedProfile?.localMjpegUrl ?? null,
      },
      resolvedSource,
      selectedSource ? [selectedSource] : ["hls", "dash", "mjpeg"],
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
  const resolvedSource = resolvePlaybackViewportSource(
    session,
    selectedSource,
    resolvedProfile?.key ?? null,
  );

  return renderRemoteStream(
    buildRemoteStreamDescriptor(
      `${session.id}:${resolvedProfile?.key ?? "none"}:${resolvedSource ?? "auto"}:${session.seekTime}`,
      session.name,
      session.snapshotUrl ?? null,
      "playback-stream",
      {
        dash: resolvedProfile?.dashUrl ?? null,
        hls: resolvedProfile?.hlsUrl ?? null,
        mjpeg: resolvedProfile?.mjpegUrl ?? null,
      },
      resolvedSource,
      selectedSource ? [selectedSource] : ["hls", "dash", "mjpeg"],
    ),
    { muted, controls: true, preload: "auto" },
  );
}

export function renderClipPlaybackViewport(
  playbackUrl: string,
  label: string,
  muted: boolean,
): TemplateResult {
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
      data-audio-muted=${muted ? "true" : "false"}
    ></video>
  `;
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
): boolean {
  if (!container) {
    return false;
  }

  viewportAudioState.set(container, muted);
  const applied = applyViewportAudioState(container, muted);
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
  for (const [kind, url] of Object.entries(sources) as Array<
    [CameraViewportSource, string | null | undefined]
  >) {
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

function applyViewportAudioState(
  container: ParentNode,
  muted: boolean,
): boolean {
  const remoteStreams = new Set<RemoteStreamAudioHost>();
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

    for (const remoteStream of current.querySelectorAll("dahuabridge-remote-stream")) {
      if (remoteStream instanceof HTMLElement) {
        remoteStreams.add(remoteStream as RemoteStreamAudioHost);
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
    remoteStream.syncAudioState(muted);
  }
  for (const video of videos) {
    applyVideoAudioState(video, muted);
  }

  return remoteStreams.size > 0 || videos.size > 0;
}

function observeViewportAudioState(
  root: ParentNode,
  container: ParentNode,
): void {
  if (viewportAudioObservers.has(root)) {
    return;
  }

  const observer = new MutationObserver(() => {
    const muted = viewportAudioState.get(container);
    if (muted === undefined) {
      return;
    }
    applyViewportAudioState(container, muted);
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
    }

    for (const element of current.querySelectorAll("*")) {
      if (element.shadowRoot) {
        observeViewportAudioState(element.shadowRoot, container);
        pending.push(element.shadowRoot);
      }
    }
  }
}

function applyVideoAudioState(
  video: HTMLVideoElement,
  muted: boolean,
): void {
  video.dataset.audioMuted = muted ? "true" : "false";
  video.defaultMuted = muted;
  video.muted = muted;
  video.toggleAttribute("muted", muted);
  void video.play().catch(() => undefined);
}

function applyHostStreamStyles(streamHost: Element): void {
  if (!(streamHost instanceof HTMLElement)) {
    return;
  }
  const hostStyle = streamHost.style;
  hostStyle.setProperty("display", "block", "important");
  hostStyle.setProperty("width", "100%", "important");
  hostStyle.setProperty("height", "100%", "important");
  hostStyle.setProperty("min-width", "0", "important");
  hostStyle.setProperty("min-height", "0", "important");
  hostStyle.setProperty("max-width", "100%", "important");
  hostStyle.setProperty("max-height", "100%", "important");
  hostStyle.setProperty("aspect-ratio", "16 / 9", "important");
  hostStyle.setProperty("overflow", "hidden", "important");
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
    streamStyle.setProperty("display", "block", "important");
    streamStyle.setProperty("width", "100%", "important");
    streamStyle.setProperty("height", "100%", "important");
    streamStyle.setProperty("min-width", "0", "important");
    streamStyle.setProperty("min-height", "0", "important");
    streamStyle.setProperty("max-width", "100%", "important");
    streamStyle.setProperty("max-height", "100%", "important");
    streamStyle.setProperty("object-fit", "fill", "important");
    streamStyle.setProperty("object-position", "center center", "important");
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

    if (current instanceof Element && current.shadowRoot) {
      pending.push(current.shadowRoot);
    }

    applyMediaElementStyles(current);

    for (const element of current.querySelectorAll("*")) {
      if (element.shadowRoot) {
        pending.push(element.shadowRoot);
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
