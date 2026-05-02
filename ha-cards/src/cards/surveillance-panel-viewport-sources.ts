import type { NvrPlaybackSessionModel } from "../domain/archive";
import type { CameraStreamViewModel, CameraViewModel } from "../domain/model";

export type CameraViewportSource = "native" | "dash" | "hls" | "mjpeg";

export interface PlaybackViewportProfile {
  key: string;
  dashUrl: string | null;
  hlsUrl: string | null;
  mjpegUrl: string | null;
}

export function resolveSelectedCameraStreamProfile(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
) {
  return resolveSelectedStreamProfile(camera.stream, selectedProfileKey);
}

export function defaultSelectedStreamProfileKey(
  stream: CameraStreamViewModel,
): string | null {
  const preferredProfileKey = preferredProfileKeyForStream(stream);
  if (preferredProfileKey) {
    return preferredProfileKey;
  }

  const recommendedProfileKey = recommendedProfileKeyForStream(stream);
  if (recommendedProfileKey) {
    return recommendedProfileKey;
  }

  const qualityProfile =
    stream.profiles.find((profile) =>
      isQualityProfile(profile.key, profile.name),
    ) ?? null;
  if (qualityProfile) {
    return qualityProfile.key;
  }

  return stream.profiles[0]?.key ?? null;
}

export function defaultOverviewStreamProfileKey(
  stream: CameraStreamViewModel,
): string | null {
  const preferredProfileKey = preferredProfileKeyForStream(stream);
  if (preferredProfileKey) {
    return preferredProfileKey;
  }

  const recommendedProfileKey = recommendedProfileKeyForStream(stream);
  if (recommendedProfileKey) {
    return recommendedProfileKey;
  }

  const bandwidthProfile =
    stream.profiles.find((profile) =>
      isBandwidthOptimizedProfile(profile.key, profile.name),
    ) ?? null;
  if (bandwidthProfile) {
    return bandwidthProfile.key;
  }

  const recommendedNonQuality =
    stream.profiles.find(
      (profile) =>
        profile.recommended && !isQualityProfile(profile.key, profile.name),
    ) ?? null;
  if (recommendedNonQuality) {
    return recommendedNonQuality.key;
  }

  return defaultSelectedStreamProfileKey(stream);
}

export function resolveSelectedStreamProfile(
  stream: CameraStreamViewModel,
  selectedProfileKey: string | null,
) {
  if (selectedProfileKey) {
    const explicitProfile =
      stream.profiles.find((profile) => profile.key === selectedProfileKey) ??
      null;
    if (explicitProfile) {
      return explicitProfile;
    }
  }

  const defaultProfileKey = defaultSelectedStreamProfileKey(stream);
  if (defaultProfileKey) {
    return (
      stream.profiles.find((profile) => profile.key === defaultProfileKey) ??
      null
    );
  }

  return stream.profiles[0] ?? null;
}

export function availableCameraViewportSources(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
): CameraViewportSource[] {
  return availableStreamViewportSources(
    camera.stream,
    selectedProfileKey,
    Boolean(camera.cameraEntity),
  );
}

export function availableStreamViewportSources(
  stream: CameraStreamViewModel,
  selectedProfileKey: string | null,
  nativeAvailable = false,
): CameraViewportSource[] {
  const sources: CameraViewportSource[] = [];
  const profile = resolveSelectedStreamProfile(stream, selectedProfileKey);
  if (nativeAvailable) {
    sources.push("native");
  }
  if (profile?.localDashUrl) {
    sources.push("dash");
  }
  if (profile?.localHlsUrl) {
    sources.push("hls");
  }
  if (profile?.localMjpegUrl) {
    sources.push("mjpeg");
  }
  return sources;
}

export function resolveSelectedCameraViewportSource(
  camera: CameraViewModel,
  selectedSource: CameraViewportSource | null,
  selectedProfileKey: string | null,
): CameraViewportSource | null {
  return resolveStreamViewportSource(
    camera.stream,
    selectedSource,
    selectedProfileKey,
    Boolean(camera.cameraEntity),
  );
}

export function resolveOverviewCameraViewportSource(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
): CameraViewportSource | null {
  return resolveStreamViewportSource(
    camera.stream,
    null,
    selectedProfileKey,
    Boolean(camera.cameraEntity),
  );
}

export function resolveStreamViewportSource(
  stream: CameraStreamViewModel,
  selectedSource: CameraViewportSource | null,
  selectedProfileKey: string | null,
  nativeAvailable = false,
): CameraViewportSource | null {
  const availableSources = availableStreamViewportSources(
    stream,
    selectedProfileKey,
    nativeAvailable,
  );
  if (selectedSource && availableSources.includes(selectedSource)) {
    return selectedSource;
  }

  const preferredSource = normalizeViewportSource(stream.preferredVideoSource);
  if (preferredSource && availableSources.includes(preferredSource)) {
    return preferredSource;
  }

  if (
    prefersNativeIntegration(stream.recommendedHaIntegration) &&
    availableSources.includes("native")
  ) {
    return "native";
  }

  return selectSourceByPriority(
    availableSources,
    ["hls", "dash", "mjpeg", "native"],
  );
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
  if (profile?.dashUrl) {
    sources.push("dash");
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
  const availableSources = availablePlaybackViewportSources(
    session,
    selectedProfileKey,
  );
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
  const availableSources = availablePlaybackViewportSources(
    session,
    selectedProfileKey,
  );
  if (availableSources.includes("hls")) {
    return "hls";
  }
  if (availableSources.includes("dash")) {
    return "dash";
  }
  if (previousSource && availableSources.includes(previousSource)) {
    return previousSource;
  }
  return availableSources[0] ?? null;
}

export function preserveCameraViewportSourceSelection(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
): CameraViewportSource | null {
  return preserveViewportSourceSelection(
    availableCameraViewportSources(camera, selectedProfileKey),
    selectedSource,
  );
}

export function preserveCameraViewportSourceSelectionOnProfileChange(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
): CameraViewportSource | null {
  const preserved = preserveCameraViewportSourceSelection(
    camera,
    selectedProfileKey,
    selectedSource,
  );
  if (preserved !== "native" || selectedSource !== "native") {
    return preserved;
  }

  return (
    selectSourceByPriority(
      availableCameraViewportSources(camera, selectedProfileKey).filter(
        (source) => source !== "native",
      ),
      ["hls", "dash", "mjpeg"],
    ) ?? preserved
  );
}

export function preservePlaybackViewportSourceSelection(
  session: NvrPlaybackSessionModel,
  selectedProfileKey: string | null,
  selectedSource: CameraViewportSource | null,
): CameraViewportSource | null {
  return preserveViewportSourceSelection(
    availablePlaybackViewportSources(session, selectedProfileKey),
    selectedSource,
  );
}

export function resolvePlaybackProfile(
  session: NvrPlaybackSessionModel,
  selectedProfileKey: string | null,
): PlaybackViewportProfile | null {
  const profileKey = selectedProfileKey?.trim() || session.recommendedProfile;
  if (profileKey && session.profiles[profileKey]) {
    const profile = session.profiles[profileKey];
    return {
      key: profileKey,
      dashUrl: profile.dashUrl,
      hlsUrl: profile.hlsUrl,
      mjpegUrl: profile.mjpegUrl,
    };
  }

  const firstEntry = Object.entries(session.profiles)[0];
  if (!firstEntry) {
    return null;
  }

  return {
    key: firstEntry[0],
    dashUrl: firstEntry[1].dashUrl,
    hlsUrl: firstEntry[1].hlsUrl,
    mjpegUrl: firstEntry[1].mjpegUrl,
  };
}

function normalizeViewportSource(
  value: string | null | undefined,
): CameraViewportSource | null {
  switch (value?.trim().toLowerCase()) {
    case "native":
    case "ha":
    case "homeassistant":
    case "home_assistant":
    case "onvif":
    case "rtsp":
    case "direct_rtsp":
      return "native";
    case "dash":
      return "dash";
    case "hls":
      return "hls";
    case "mjpeg":
      return "mjpeg";
    default:
      return null;
  }
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

function preserveViewportSourceSelection(
  availableSources: readonly CameraViewportSource[],
  selectedSource: CameraViewportSource | null,
): CameraViewportSource | null {
  if (!selectedSource) {
    return null;
  }
  return availableSources.includes(selectedSource) ? selectedSource : null;
}

function preferredProfileKeyForStream(
  stream: CameraStreamViewModel,
): string | null {
  const preferredProfileKey = stream.preferredVideoProfile?.trim() ?? "";
  if (!preferredProfileKey) {
    return null;
  }
  return (
    stream.profiles.find((profile) => profile.key === preferredProfileKey)?.key ??
    null
  );
}

function recommendedProfileKeyForStream(
  stream: CameraStreamViewModel,
): string | null {
  const explicitRecommendedKey = stream.recommendedProfile?.trim() ?? "";
  if (explicitRecommendedKey) {
    const explicitRecommended =
      stream.profiles.find((profile) => profile.key === explicitRecommendedKey) ??
      null;
    if (explicitRecommended) {
      return explicitRecommended.key;
    }
  }

  return (
    stream.profiles.find((profile) => profile.recommended)?.key ??
    null
  );
}

function prefersNativeIntegration(
  value: string | null | undefined,
): boolean {
  switch (value?.trim().toLowerCase()) {
    case "native":
    case "onvif":
    case "home_assistant":
    case "homeassistant":
    case "ha":
      return true;
    default:
      return false;
  }
}
