import { z } from "zod";

import type {
  BridgeRecordingClipListModel,
  BridgeRecordingClipModel,
  NvrArchiveExportClipModel,
  NvrArchiveRecordingModel,
  NvrArchiveSearchResultModel,
} from "../domain/archive";
import { rewriteBridgeUrl } from "./bridge-url";

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
  items: z.array(archiveRecordingSchema).optional().default([]),
  event: z.array(archiveRecordingSchema).optional().default([]),
  events: z.array(archiveRecordingSchema).optional().default([]),
  recordings: z.array(archiveRecordingSchema).optional().default([]),
});

const archiveExportClipSchema = z.object({
  id: z.string().min(1),
  status: z.string().min(1),
  download_url: z.string().optional().nullable(),
  self_url: z.string().optional().nullable(),
  duration_ms: z.number().int().optional().nullable(),
  error: z.string().optional().nullable(),
});

const archiveExportResponseSchema = z.object({
  status: z.string().min(1),
  clip: archiveExportClipSchema,
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
  duration_ms: z.number().int().optional().nullable(),
  bytes: z.number().int().optional().nullable(),
  file_name: z.string().optional().nullable(),
  download_url: z.string().optional().nullable(),
  self_url: z.string().optional().nullable(),
  stop_url: z.string().optional().nullable(),
  error: z.string().optional().nullable(),
});

const bridgeRecordingListSchema = z.object({
  returned_count: z.number().int(),
  items: z.array(bridgeRecordingSchema),
});

export interface ArchiveRecordingsQuery {
  channel: number;
  startTime: string;
  endTime: string;
  limit: number;
  eventCode?: string;
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
  const url = new URL(searchUrl, window.location.origin);
  url.searchParams.set("channel", String(query.channel));
  url.searchParams.set("start", query.startTime);
  url.searchParams.set("end", query.endTime);
  url.searchParams.set("limit", String(Math.max(1, Math.trunc(query.limit))));
  if (query.eventCode && query.eventCode !== "__all__") {
    url.searchParams.set("event", query.eventCode);
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
      mapArchiveRecording(item, {
        channel: resultChannel,
        startTime: resultStartTime,
        endTime: resultEndTime,
      }),
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
  return {
    returnedCount: payload.returned_count,
    items: payload.items.map(mapBridgeRecording),
  };
}

function mapArchiveRecording(
  item: z.infer<typeof archiveRecordingSchema>,
  fallback: {
    channel: number;
    startTime: string;
    endTime: string;
  },
): NvrArchiveRecordingModel {
  return {
    channel: firstNumber(item.channel, item.Channel, fallback.channel),
    startTime: firstString(item.start_time, item.StartTime, fallback.startTime),
    endTime: firstString(item.end_time, item.EndTime, fallback.endTime),
    downloadUrl: firstNullableString(item.download_url, item.DownloadURL),
    exportUrl: firstNullableString(item.export_url, item.ExportURL),
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
  logArchiveRequest("request", "POST", exportUrl);
  const response = await fetch(exportUrl, {
    method: "POST",
    headers: {
      Accept: "application/json",
    },
    signal,
  });
  logArchiveRequest("response", "POST", exportUrl, response.status, performance.now() - started);

  if (!response.ok) {
    throw new Error(`Bridge archive export failed with status ${response.status}`);
  }

  const payload = archiveExportResponseSchema.parse(await response.json());
  return mapArchiveExportClip(payload.clip, browserBridgeUrl);
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
      throw new Error(`Bridge archive export status failed with status ${response.status}`);
    }
    current = mapArchiveExportClip(archiveExportClipSchema.parse(await response.json()), browserBridgeUrl);
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
): NvrArchiveExportClipModel {
  return {
    id: clip.id,
    status: clip.status,
    downloadUrl: rewriteBridgeUrl(clip.download_url ?? null, browserBridgeUrl),
    selfUrl: rewriteBridgeUrl(clip.self_url ?? null, browserBridgeUrl),
    durationMs: clip.duration_ms ?? null,
    error: clip.error ?? null,
  };
}

function mapBridgeRecording(
  item: z.infer<typeof bridgeRecordingSchema>,
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
    durationMs: item.duration_ms ?? null,
    bytes: item.bytes ?? null,
    fileName: item.file_name ?? null,
    downloadUrl: item.download_url ?? null,
    selfUrl: item.self_url ?? null,
    stopUrl: item.stop_url ?? null,
    error: item.error ?? null,
  };
}

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) {
    return Promise.reject(new DOMException("Aborted", "AbortError"));
  }
  return new Promise((resolve, reject) => {
    const handle = window.setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        window.clearTimeout(handle);
        reject(new DOMException("Aborted", "AbortError"));
      },
      { once: true },
    );
  });
}

function logArchiveRequest(
  phase: "request" | "response",
  method: "GET" | "POST",
  targetUrl: string,
  status?: number,
  durationMs?: number,
): void {
  if (typeof console === "undefined" || typeof console.debug !== "function") {
    return;
  }
  console.debug("[DahuaBridge]", `card archive ${phase}`, {
    method,
    url: targetUrl,
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
