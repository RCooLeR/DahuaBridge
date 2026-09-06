import { z } from "zod";

import type {
  BridgeRecordingClipListModel,
  BridgeRecordingClipModel,
  NvrArchiveExportClipModel,
  NvrArchiveRecordingModel,
  NvrArchiveSearchResultModel,
} from "../domain/archive";
import { normalizeArchiveSearchUrlTemplate } from "../domain/archive";
import { logCardInfo, redactUrlForLog } from "../utils/logging";
import { rewriteBridgeUrl } from "./bridge-url";
import { authenticatedBridgeResourceUrl } from "./bridge-resource";

const optionalIntegerSchema = z.preprocess((value) => {
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value === "string" && !value.trim()) {
    return null;
  }
  return value;
}, z.coerce.number().int().optional().nullable());

const archiveRecordingSchema = z.object({
  id: z.string().optional().nullable(),
  record_kind: z.string().optional().nullable(),
  recordKind: z.string().optional().nullable(),
  source: z.string().optional().nullable(),
  Source: z.string().optional().nullable(),
  channel: optionalIntegerSchema,
  Channel: optionalIntegerSchema,
  start_time: z.string().optional().nullable(),
  StartTime: z.string().optional().nullable(),
  end_time: z.string().optional().nullable(),
  EndTime: z.string().optional().nullable(),
  download_url: z.string().optional().nullable(),
  DownloadURL: z.string().optional().nullable(),
  export_url: z.string().optional().nullable(),
  ExportURL: z.string().optional().nullable(),
  asset_status: z.string().optional().nullable(),
  assetStatus: z.string().optional().nullable(),
  asset_clip_id: z.string().optional().nullable(),
  assetClipId: z.string().optional().nullable(),
  asset_playback_url: z.string().optional().nullable(),
  assetPlaybackURL: z.string().optional().nullable(),
  asset_download_url: z.string().optional().nullable(),
  assetDownloadURL: z.string().optional().nullable(),
  asset_self_url: z.string().optional().nullable(),
  assetSelfUrl: z.string().optional().nullable(),
  assetSelfURL: z.string().optional().nullable(),
  asset_stop_url: z.string().optional().nullable(),
  assetStopUrl: z.string().optional().nullable(),
  assetStopURL: z.string().optional().nullable(),
  asset_error: z.string().optional().nullable(),
  assetError: z.string().optional().nullable(),
  rtsp_main_url: z.string().optional().nullable(),
  rtspMainUrl: z.string().optional().nullable(),
  RTSPMainURL: z.string().optional().nullable(),
  rtsp_sub_url: z.string().optional().nullable(),
  rtspSubUrl: z.string().optional().nullable(),
  RTSPSubURL: z.string().optional().nullable(),
  file_path: z.string().optional().nullable(),
  FilePath: z.string().optional().nullable(),
  type: z.string().optional().nullable(),
  Type: z.string().optional().nullable(),
  video_stream: z.string().optional().nullable(),
  VideoStream: z.string().optional().nullable(),
  disk: optionalIntegerSchema,
  Disk: optionalIntegerSchema,
  partition: optionalIntegerSchema,
  Partition: optionalIntegerSchema,
  cluster: optionalIntegerSchema,
  Cluster: optionalIntegerSchema,
  length_bytes: optionalIntegerSchema,
  Length: optionalIntegerSchema,
  cut_length_bytes: optionalIntegerSchema,
  CutLength: optionalIntegerSchema,
  flags: z.array(z.string()).optional().default([]),
  Flags: z.array(z.union([z.string(), z.number()])).optional().default([]),
});

const archiveRecordingArraySchema = z.preprocess(
  (value) => (Array.isArray(value) ? value : []),
  z.array(archiveRecordingSchema),
);

const archiveSearchResultSchema = z.object({
  device_id: z.string().optional().nullable(),
  deviceId: z.string().optional().nullable(),
  channel: optionalIntegerSchema,
  Channel: optionalIntegerSchema,
  start_time: z.string().optional().nullable(),
  StartTime: z.string().optional().nullable(),
  end_time: z.string().optional().nullable(),
  EndTime: z.string().optional().nullable(),
  limit: optionalIntegerSchema,
  Limit: optionalIntegerSchema,
  returned_count: optionalIntegerSchema,
  found: optionalIntegerSchema,
  items: archiveRecordingArraySchema,
  event: archiveRecordingArraySchema,
  events: archiveRecordingArraySchema,
  recordings: archiveRecordingArraySchema,
});

const archiveExportClipSchema = z.object({
  id: z.string().min(1),
  status: z.string().min(1),
  playback_url: z.string().optional().nullable(),
  download_url: z.string().optional().nullable(),
  self_url: z.string().optional().nullable(),
  duration_ms: z.number().int().optional().nullable(),
  error: z.string().optional().nullable(),
});

const archiveExportResponseSchema = z.object({
  status: z.string().min(1),
  clip: archiveExportClipSchema,
});

const bridgeErrorResponseSchema = z.object({
  error: z.string().optional().nullable(),
  code: z.string().optional().nullable(),
  message: z.string().optional().nullable(),
});

const bridgeRecordingSchema = z.object({
  id: z.string().min(1),
  stream_id: z.string().min(1),
  root_device_id: z.string().optional().nullable(),
  source_device_id: z.string().optional().nullable(),
  device_kind: z.string().optional().nullable(),
  name: z.string().optional().nullable(),
  channel: z.number().int().optional().nullable(),
  profile: z.string().optional().nullable(),
  status: z.string().min(1),
  started_at: z.string().min(1),
  ended_at: z.string().optional().nullable(),
  start_time: z.string().optional().nullable(),
  end_time: z.string().optional().nullable(),
  duration_ms: z.number().int().optional().nullable(),
  bytes: z.number().int().optional().nullable(),
  file_name: z.string().optional().nullable(),
  playback_url: z.string().optional().nullable(),
  download_url: z.string().optional().nullable(),
  error: z.string().optional().nullable(),
});

const bridgeRecordingArraySchema = z.preprocess(
  (value) => (Array.isArray(value) ? value : []),
  z.array(bridgeRecordingSchema),
);

const bridgeRecordingListSchema = z.object({
  returned_count: optionalIntegerSchema,
  items: bridgeRecordingArraySchema,
});

export interface ArchiveRecordingsQuery {
  channel: number;
  startTime: string;
  endTime: string;
  limit: number;
  eventCode?: string;
  eventOnly?: boolean;
  dbOnly?: boolean;
}

export interface BridgeRecordingsQuery {
  startTime?: string;
  endTime?: string;
  limit?: number;
}

export async function fetchArchiveRecordings(
  searchUrl: string,
  query: ArchiveRecordingsQuery,
  signal?: AbortSignal,
): Promise<NvrArchiveSearchResultModel> {
  const normalizedSearchUrl = normalizeArchiveSearchUrlTemplate(searchUrl) ?? searchUrl;
  const url = new URL(normalizedSearchUrl, window.location.origin);
  url.searchParams.set("channel", String(query.channel));
  url.searchParams.set("start", query.startTime);
  url.searchParams.set("end", query.endTime);
  url.searchParams.set("limit", String(Math.max(1, Math.trunc(query.limit))));
  if (query.eventOnly) {
    url.searchParams.set("event_only", "true");
    url.searchParams.set("event", query.eventCode && query.eventCode !== "__all__" ? query.eventCode : "all");
  } else if (query.eventCode && query.eventCode !== "__all__") {
    url.searchParams.set("event", query.eventCode);
  }
  if (query.dbOnly) {
    url.searchParams.set("db_only", "true");
  }

  const started = performance.now();
  logArchiveRequest("request", "GET", url.toString());
  const response = await fetch(url, {
    method: "GET",
    headers: {
      Accept: "application/json",
    },
    signal,
  });
  logArchiveRequest("response", "GET", url.toString(), response.status, performance.now() - started);

  if (!response.ok) {
    throw new Error(`Bridge archive request failed with status ${response.status}`);
  }

  const payload = archiveSearchResultSchema.parse(await response.json());
  const items = selectArchiveItems(payload);
  const browserBridgeUrl = browserBridgeUrlFromRequestUrl(url.toString());
  const resultChannel = firstNumber(payload.channel, payload.Channel, query.channel);
  const resultStartTime = firstString(payload.start_time, payload.StartTime, query.startTime);
  const resultEndTime = firstString(payload.end_time, payload.EndTime, query.endTime);
  const resultLimit = firstNumber(payload.limit, payload.Limit, query.limit);
  return {
    deviceId: firstString(payload.device_id, payload.deviceId),
    channel: resultChannel,
    startTime: resultStartTime,
    endTime: resultEndTime,
    limit: resultLimit,
    returnedCount: firstNumber(payload.returned_count, payload.found, items.length),
    items: items.map((item) =>
      mapArchiveRecording(
        item,
        {
          channel: resultChannel,
          startTime: resultStartTime,
          endTime: resultEndTime,
        },
        browserBridgeUrl,
        url.toString(),
      ),
    ),
  };
}

export async function fetchBridgeRecordings(
  recordingsUrl: string,
  query: BridgeRecordingsQuery = {},
  signal?: AbortSignal,
): Promise<BridgeRecordingClipListModel> {
  const url = new URL(recordingsUrl, window.location.origin);
  if (query.startTime) {
    url.searchParams.set("start", query.startTime);
  }
  if (query.endTime) {
    url.searchParams.set("end", query.endTime);
  }
  if (typeof query.limit === "number" && Number.isFinite(query.limit) && query.limit > 0) {
    url.searchParams.set("limit", String(Math.trunc(query.limit)));
  }

  const started = performance.now();
  logArchiveRequest("request", "GET", url.toString());
  const response = await fetch(url, {
    method: "GET",
    headers: {
      Accept: "application/json",
    },
    signal,
  });
  logArchiveRequest("response", "GET", url.toString(), response.status, performance.now() - started);

  if (!response.ok) {
    throw new Error(`Bridge MP4 request failed with status ${response.status}`);
  }

  const payload = bridgeRecordingListSchema.parse(await response.json());
  const browserBridgeUrl = browserBridgeUrlFromRequestUrl(url.toString());
  return {
    returnedCount: firstNumber(payload.returned_count, payload.items.length),
    items: payload.items.map((item) => mapBridgeRecording(item, browserBridgeUrl, url.toString())),
  };
}

function mapArchiveRecording(
  item: z.infer<typeof archiveRecordingSchema>,
  fallback: {
    channel: number;
    startTime: string;
    endTime: string;
  },
  browserBridgeUrl?: string | null,
  requestUrl = browserBridgeUrl ?? "",
): NvrArchiveRecordingModel {
  const resourceUrl = (target: string | null) => authenticatedBridgeResourceUrl(target, requestUrl, browserBridgeUrl);
  return {
    id: firstNullableString(item.id),
    recordKind: firstNullableString(item.record_kind, item.recordKind),
    source: firstNullableString(item.source, item.Source),
    channel: firstNumber(item.channel, item.Channel, fallback.channel),
    startTime: firstString(item.start_time, item.StartTime, fallback.startTime),
    endTime: firstString(item.end_time, item.EndTime, fallback.endTime),
    downloadUrl: resourceUrl(firstNullableString(item.download_url, item.DownloadURL)),
    exportUrl: resourceUrl(firstNullableString(item.export_url, item.ExportURL)),
    assetStatus: firstNullableString(item.asset_status, item.assetStatus),
    assetClipId: firstNullableString(item.asset_clip_id, item.assetClipId),
    assetPlaybackUrl: resourceUrl(firstNullableString(item.asset_playback_url, item.assetPlaybackURL)),
    assetDownloadUrl: resourceUrl(firstNullableString(item.asset_download_url, item.assetDownloadURL)),
    assetSelfUrl: resourceUrl(firstNullableString(item.asset_self_url, item.assetSelfUrl, item.assetSelfURL)),
    assetStopUrl: resourceUrl(firstNullableString(item.asset_stop_url, item.assetStopUrl, item.assetStopURL)),
    assetError: firstNullableString(item.asset_error, item.assetError),
    rtspMainUrl: firstNullableString(item.rtsp_main_url, item.rtspMainUrl, item.RTSPMainURL),
    rtspSubUrl: firstNullableString(item.rtsp_sub_url, item.rtspSubUrl, item.RTSPSubURL),
    filePath: firstNullableString(item.file_path, item.FilePath),
    type: firstNullableString(item.type, item.Type),
    videoStream: firstNullableString(item.video_stream, item.VideoStream),
    disk: firstNullableNumber(item.disk, item.Disk),
    partition: firstNullableNumber(item.partition, item.Partition),
    cluster: firstNullableNumber(item.cluster, item.Cluster),
    lengthBytes: firstNullableNumber(item.length_bytes, item.Length),
    cutLengthBytes: firstNullableNumber(item.cut_length_bytes, item.CutLength),
    flags: normalizeFlags(item.flags, item.Flags),
  };
}

export async function exportArchiveRecording(
  exportUrl: string,
  browserBridgeUrl?: string | null,
  signal?: AbortSignal,
): Promise<NvrArchiveExportClipModel> {
  const started = performance.now();
  const requestUrl = rewriteBridgeUrl(exportUrl, browserBridgeUrl) ?? exportUrl;
  logArchiveRequest("request", "POST", requestUrl);
  const response = await fetch(requestUrl, {
    method: "POST",
    headers: {
      Accept: "application/json",
    },
    signal,
  });
  logArchiveRequest("response", "POST", exportUrl, response.status, performance.now() - started);

  if (!response.ok) {
    throw new Error(
      (await readBridgeErrorMessage(response)) ??
        `Bridge archive export failed with status ${response.status}`,
    );
  }

  const payload = archiveExportResponseSchema.parse(await response.json());
  return mapArchiveExportClip(payload.clip, browserBridgeUrl, exportUrl);
}

export async function waitForArchiveExportCompletion(
  clip: NvrArchiveExportClipModel,
  browserBridgeUrl?: string | null,
  signal?: AbortSignal,
): Promise<NvrArchiveExportClipModel> {
  let current = clip;
  const maxWaitMs = Math.min(
    Math.max((clip.durationMs ?? 0) + 15000, 30000),
    30 * 60 * 1000,
  );
  const deadline = Date.now() + maxWaitMs;

  while (current.status === "recording" && current.selfUrl && Date.now() < deadline) {
    await delay(1500, signal);
    const response = await fetch(current.selfUrl, {
      method: "GET",
      headers: {
        Accept: "application/json",
      },
      signal,
    });
    if (!response.ok) {
      throw new Error(
        (await readBridgeErrorMessage(response)) ??
          `Bridge archive export status failed with status ${response.status}`,
      );
    }
    current = mapArchiveExportClip(archiveExportClipSchema.parse(await response.json()), browserBridgeUrl, current.selfUrl);
  }

  if (current.status === "failed") {
    throw new Error(current.error || "Bridge archive export failed.");
  }
  if (current.status !== "completed") {
    throw new Error("Bridge archive export is still recording.");
  }
  return current;
}

function mapArchiveExportClip(
  clip: z.infer<typeof archiveExportClipSchema>,
  browserBridgeUrl?: string | null,
  requestUrl = browserBridgeUrl ?? "",
): NvrArchiveExportClipModel {
  return {
    id: clip.id,
    status: clip.status,
    playbackUrl: authenticatedBridgeResourceUrl(clip.playback_url, requestUrl, browserBridgeUrl),
    downloadUrl: authenticatedBridgeResourceUrl(clip.download_url, requestUrl, browserBridgeUrl),
    selfUrl: authenticatedBridgeResourceUrl(clip.self_url, requestUrl, browserBridgeUrl),
    durationMs: clip.duration_ms ?? null,
    error: clip.error ?? null,
  };
}

function mapBridgeRecording(
  item: z.infer<typeof bridgeRecordingSchema>,
  browserBridgeUrl?: string | null,
  requestUrl = browserBridgeUrl ?? "",
): BridgeRecordingClipModel {
  return {
    id: item.id,
    streamId: item.stream_id,
    rootDeviceId: item.root_device_id ?? null,
    sourceDeviceId: item.source_device_id ?? null,
    deviceKind: item.device_kind ?? null,
    name: item.name ?? null,
    channel: item.channel ?? null,
    profile: item.profile ?? null,
    status: item.status,
    startedAt: item.started_at,
    endedAt: item.ended_at ?? null,
    sourceStartTime: item.start_time ?? null,
    sourceEndTime: item.end_time ?? null,
    durationMs: item.duration_ms ?? null,
    bytes: item.bytes ?? null,
    fileName: item.file_name ?? null,
    playbackUrl: authenticatedBridgeResourceUrl(item.playback_url, requestUrl, browserBridgeUrl),
    downloadUrl: authenticatedBridgeResourceUrl(item.download_url, requestUrl, browserBridgeUrl),
    error: item.error ?? null,
  };
}

function browserBridgeUrlFromRequestUrl(value: string): string | null {
  try {
    const url = new URL(value, window.location.origin);
    const apiPathIndex = url.pathname.indexOf("/api/");
    const bridgePath = apiPathIndex > 0 ? url.pathname.slice(0, apiPathIndex) : "";
    return `${url.protocol}//${url.host}${bridgePath.replace(/\/+$/, "")}`;
  } catch {
    return null;
  }
}

async function readBridgeErrorMessage(response: Response): Promise<string | null> {
  const contentType = response.headers.get("content-type")?.toLowerCase() ?? "";
  try {
    if (contentType.includes("application/json")) {
      const payload = bridgeErrorResponseSchema.safeParse(await response.json());
      if (payload.success) {
        return firstNullableString(payload.data.error, payload.data.message);
      }
      return null;
    }
    const text = (await response.text()).trim();
    return text || null;
  } catch {
    return null;
  }
}

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) {
    return Promise.reject(new DOMException("Aborted", "AbortError"));
  }
  return new Promise((resolve, reject) => {
    const onAbort = () => {
      window.clearTimeout(handle);
      reject(new DOMException("Aborted", "AbortError"));
    };
    const handle = window.setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

function logArchiveRequest(
  phase: "request" | "response",
  method: "GET" | "POST",
  targetUrl: string,
  status?: number,
  durationMs?: number,
): void {
  logCardInfo(`card archive ${phase}`, {
    method,
    url: redactUrlForLog(targetUrl),
    status,
    duration_ms: durationMs === undefined ? undefined : Math.round(durationMs),
  });
}

function selectArchiveItems(
  payload: z.infer<typeof archiveSearchResultSchema>,
): z.infer<typeof archiveRecordingSchema>[] {
  for (const candidate of [
    payload.items,
    payload.event,
    payload.events,
    payload.recordings,
  ]) {
    if (Array.isArray(candidate) && candidate.length > 0) {
      return candidate;
    }
  }
  return payload.items;
}

function firstString(...values: Array<string | null | undefined>): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) {
      return value;
    }
  }
  return "";
}

function firstNullableString(...values: Array<string | null | undefined>): string | null {
  const resolved = firstString(...values);
  return resolved || null;
}

function firstNumber(...values: Array<number | null | undefined>): number {
  for (const value of values) {
    if (typeof value === "number" && Number.isFinite(value)) {
      return Math.trunc(value);
    }
  }
  return 0;
}

function firstNullableNumber(...values: Array<number | null | undefined>): number | null {
  for (const value of values) {
    if (typeof value === "number" && Number.isFinite(value)) {
      return Math.trunc(value);
    }
  }
  return null;
}

function normalizeFlags(
  lowercaseFlags: readonly string[],
  originalFlags: ReadonlyArray<string | number>,
): string[] {
  const normalized: string[] = [];
  for (const value of [...lowercaseFlags, ...originalFlags.map((flag) => String(flag))]) {
    const trimmed = value.trim();
    if (!trimmed || normalized.includes(trimmed)) {
      continue;
    }
    normalized.push(trimmed);
  }
  return normalized;
}
