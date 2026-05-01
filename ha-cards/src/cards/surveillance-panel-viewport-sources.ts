import type { NvrPlaybackSessionModel } from "../domain/archive";
import type { CameraStreamViewModel, CameraViewModel } from "../domain/model";

export type CameraViewportSource = "hls" | "mjpeg";

export interface PlaybackViewportProfile {
  key: string;
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
  const qualityProfile =
    stream.profiles.find((profile) =>
      isQualityProfile(profile.key, profile.name),
    ) ?? null;
  if (qualityProfile) {
    return qualityProfile.key;
  }

  if (stream.preferredVideoProfile) {
    const preferredProfile =
      stream.profiles.find(
        (profile) => profile.key === stream.preferredVideoProfile,
      ) ?? null;
    if (preferredProfile) {
      return preferredProfile.key;
    }
  }

  return (
    (stream.profiles.find((profile) => profile.recommended) ??
      stream.profiles[0] ??
      null)?.key ?? null
  );
}

export function defaultOverviewStreamProfileKey(
  stream: CameraStreamViewModel,
): string | null {
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
  return availableStreamViewportSources(camera.stream, selectedProfileKey);
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
  );
}

export function resolveOverviewCameraViewportSource(
  camera: CameraViewModel,
  selectedProfileKey: string | null,
): CameraViewportSource | null {
  return selectSourceByPriority(
    availableCameraViewportSources(camera, selectedProfileKey),
    ["hls", "mjpeg"],
  );
}

export function resolveStreamViewportSource(
  stream: CameraStreamViewModel,
  selectedSource: CameraViewportSource | null,
  selectedProfileKey: string | null,
): CameraViewportSource | null {
  const availableSources = availableStreamViewportSources(
    stream,
    selectedProfileKey,
  );
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
  if (previousSource && availableSources.includes(previousSource)) {
    return previousSource;
  }
  return availableSources[0] ?? null;
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
    hlsUrl: firstEntry[1].hlsUrl,
    mjpegUrl: firstEntry[1].mjpegUrl,
  };
}

function normalizeViewportSource(
  value: string | null | undefined,
): CameraViewportSource | null {
  switch (value?.trim().toLowerCase()) {
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
