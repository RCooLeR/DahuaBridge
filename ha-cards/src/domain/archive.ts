export const DEFAULT_ARCHIVE_SEARCH_LIMIT = 100;

export type ArchiveRequestFieldKey =
  | "channel"
  | "start_time"
  | "end_time"
  | "limit"
  | "seek_time";

export type ArchiveFieldValueType = "integer" | "datetime";

export interface ArchiveFeatureSource {
  key: string | null;
  label: string | null;
  kind: string | null;
  url: string | null;
  supported: boolean;
}

export interface ArchiveRequestFieldModel {
  key: ArchiveRequestFieldKey;
  label: string;
  valueType: ArchiveFieldValueType;
  required: boolean;
  defaultValue: number | string | null;
}

export interface NvrArchiveSearchRequestModel {
  channel: number;
  startTime: string;
  endTime: string;
  limit: number;
}

export interface NvrPlaybackSessionRequestModel {
  channel: number;
  startTime: string;
  endTime: string;
  seekTime: string | null;
  filePath?: string | null;
}

export interface NvrPlaybackSeekRequestModel {
  seekTime: string;
}

export interface NvrArchiveSearchCapabilityModel {
  supported: boolean;
  label: string;
  kind: "query";
  method: "GET";
  url: string | null;
  channel: number | null;
  defaultLimit: number;
  requestFields: ArchiveRequestFieldModel[];
}

export interface NvrPlaybackSessionCapabilityModel {
  supported: boolean;
  label: string;
  kind: "session";
  method: "POST";
  url: string | null;
  channel: number | null;
  requestFields: ArchiveRequestFieldModel[];
  seekRequestFields: ArchiveRequestFieldModel[];
  responseProfiles: Array<"dash" | "hls" | "mjpeg" | "webrtc">;
}

export interface CameraArchiveCapabilities {
  supported: boolean;
  search: NvrArchiveSearchCapabilityModel;
  playback: NvrPlaybackSessionCapabilityModel;
}

export interface NvrArchiveRecordingModel {
  source: string | null;
  channel: number;
  startTime: string;
  endTime: string;
  downloadUrl: string | null;
  exportUrl: string | null;
  filePath: string | null;
  type: string | null;
  videoStream: string | null;
  disk: number | null;
  partition: number | null;
  cluster: number | null;
  lengthBytes: number | null;
  cutLengthBytes: number | null;
  flags: string[];
}

export interface NvrArchiveExportClipModel {
  id: string;
  status: "recording" | "completed" | "failed" | string;
  playbackUrl: string | null;
  downloadUrl: string | null;
  selfUrl: string | null;
  durationMs: number | null;
  error: string | null;
}

export interface NvrArchiveSearchResultModel {
  deviceId: string;
  channel: number;
  startTime: string;
  endTime: string;
  limit: number;
  returnedCount: number;
  items: NvrArchiveRecordingModel[];
}

export interface BridgeRecordingClipModel {
  id: string;
  streamId: string;
  rootDeviceId: string | null;
  sourceDeviceId: string | null;
  deviceKind: string | null;
  name: string | null;
  channel: number | null;
  profile: string | null;
  status: string;
  startedAt: string;
  endedAt: string | null;
  sourceStartTime: string | null;
  sourceEndTime: string | null;
  durationMs: number | null;
  bytes: number | null;
  fileName: string | null;
  playbackUrl: string | null;
  downloadUrl: string | null;
  selfUrl: string | null;
  stopUrl: string | null;
  error: string | null;
}

export interface BridgeRecordingClipListModel {
  returnedCount: number;
  items: BridgeRecordingClipModel[];
}

export interface NvrPlaybackProfileModel {
  name: string;
  dashUrl: string | null;
  hlsUrl: string | null;
  mjpegUrl: string | null;
  webrtcOfferUrl: string | null;
}

export interface NvrPlaybackSessionModel {
  id: string;
  streamId: string;
  deviceId: string;
  sourceStreamId: string | null;
  name: string;
  channel: number;
  startTime: string;
  endTime: string;
  seekTime: string;
  recommendedProfile: string;
  snapshotUrl: string | null;
  createdAt: string;
  expiresAt: string;
  profiles: Record<string, NvrPlaybackProfileModel>;
}

export function buildCameraArchiveCapabilities(
  channelNumber: number | null,
  features: readonly ArchiveFeatureSource[],
): CameraArchiveCapabilities {
  const archiveSearch = featureByKey(features, "archive_search");
  const archivePlayback = featureByKey(features, "archive_playback");

  return {
    supported: archiveSearch?.supported === true || archivePlayback?.supported === true,
    search: {
      supported: archiveSearch?.supported === true,
      label: archiveSearch?.label ?? "Recordings",
      kind: "query",
      method: "GET",
      url: archiveSearch?.url ?? null,
      channel: channelNumber,
      defaultLimit: DEFAULT_ARCHIVE_SEARCH_LIMIT,
      requestFields: [
        buildRequestField("channel", true, "integer", channelNumber),
        buildRequestField("start_time", true, "datetime"),
        buildRequestField("end_time", true, "datetime"),
        buildRequestField("limit", false, "integer", DEFAULT_ARCHIVE_SEARCH_LIMIT),
      ],
    },
    playback: {
      supported: archivePlayback?.supported === true,
      label: archivePlayback?.label ?? "Playback",
      kind: "session",
      method: "POST",
      url: archivePlayback?.url ?? null,
      channel: channelNumber,
      requestFields: [
        buildRequestField("channel", true, "integer", channelNumber),
        buildRequestField("start_time", true, "datetime"),
        buildRequestField("end_time", true, "datetime"),
        buildRequestField("seek_time", false, "datetime"),
      ],
      seekRequestFields: [buildRequestField("seek_time", true, "datetime")],
      responseProfiles: ["dash", "hls", "mjpeg", "webrtc"],
    },
  };
}

export function createArchiveSearchRequest(
  channel: number,
  startTime: string,
  endTime: string,
  limit = DEFAULT_ARCHIVE_SEARCH_LIMIT,
): NvrArchiveSearchRequestModel {
  return {
    channel,
    startTime,
    endTime,
    limit,
  };
}

export function createPlaybackSessionRequest(
  channel: number,
  startTime: string,
  endTime: string,
  seekTime?: string | null,
  filePath?: string | null,
): NvrPlaybackSessionRequestModel {
  const request: NvrPlaybackSessionRequestModel = {
    channel,
    startTime,
    endTime,
    seekTime: seekTime ?? null,
  };
  if (filePath) {
    request.filePath = filePath;
  }
  return request;
}

export function createPlaybackSeekRequest(seekTime: string): NvrPlaybackSeekRequestModel {
  return { seekTime };
}

export function normalizeArchiveSearchUrlTemplate(
  value: string | null | undefined,
): string | null {
  const trimmed = value?.trim() ?? "";
  if (!trimmed) {
    return null;
  }

  try {
    const url = new URL(trimmed, "https://dahuabridge.invalid");
    for (const key of ["channel", "start", "end", "limit", "event", "event_only"]) {
      url.searchParams.delete(key);
    }

    for (const [key, candidateValue] of [...url.searchParams.entries()]) {
      if (candidateValue.includes("{") || candidateValue.includes("}")) {
        url.searchParams.delete(key);
      }
    }

    if (/^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed)) {
      return url.toString();
    }

    return `${url.pathname}${url.search}${url.hash}`;
  } catch {
    return trimmed;
  }
}

function buildRequestField(
  key: ArchiveRequestFieldKey,
  required: boolean,
  valueType: ArchiveFieldValueType,
  defaultValue: number | string | null = null,
): ArchiveRequestFieldModel {
  return {
    key,
    label: requestFieldLabel(key),
    valueType,
    required,
    defaultValue,
  };
}

function featureByKey(
  features: readonly ArchiveFeatureSource[],
  key: string,
): ArchiveFeatureSource | null {
  for (const feature of features) {
    if (feature.key === key) {
      return feature;
    }
  }
  return null;
}

function requestFieldLabel(key: ArchiveRequestFieldKey): string {
  switch (key) {
    case "channel":
      return "Channel";
    case "start_time":
      return "Start time";
    case "end_time":
      return "End time";
    case "limit":
      return "Limit";
    case "seek_time":
      return "Seek time";
  }
}
