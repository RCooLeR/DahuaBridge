import { z } from "zod";

import type {
  NvrArchiveExportClipModel,
  NvrArchiveRecordingModel,
  NvrArchiveSearchResultModel,
} from "../domain/archive";
import { rewriteBridgeUrl } from "./bridge-url";

const archiveRecordingSchema = z.object({
  channel: z.number().int(),
  start_time: z.string().min(1),
  end_time: z.string().min(1),
  download_url: z.string().optional().nullable(),
  export_url: z.string().optional().nullable(),
  file_path: z.string().optional().nullable(),
  type: z.string().optional().nullable(),
  video_stream: z.string().optional().nullable(),
  disk: z.number().int().optional().nullable(),
  partition: z.number().int().optional().nullable(),
  cluster: z.number().int().optional().nullable(),
  length_bytes: z.number().int().optional().nullable(),
  cut_length_bytes: z.number().int().optional().nullable(),
  flags: z.array(z.string()).optional().default([]),
});

const archiveSearchResultSchema = z.object({
  device_id: z.string().min(1),
  channel: z.number().int(),
  start_time: z.string().min(1),
  end_time: z.string().min(1),
  limit: z.number().int(),
  returned_count: z.number().int(),
  items: z.array(archiveRecordingSchema),
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

export interface ArchiveRecordingsQuery {
  channel: number;
  startTime: string;
  endTime: string;
  limit: number;
  eventCode?: string;
}

export async function fetchArchiveRecordings(
  searchUrl: string,
  query: ArchiveRecordingsQuery,
  signal?: AbortSignal,
): Promise<NvrArchiveSearchResultModel> {
  const url = new URL(searchUrl);
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
  return {
    deviceId: payload.device_id,
    channel: payload.channel,
    startTime: payload.start_time,
    endTime: payload.end_time,
    limit: payload.limit,
    returnedCount: payload.returned_count,
    items: payload.items.map(mapArchiveRecording),
  };
}

function mapArchiveRecording(
  item: z.infer<typeof archiveRecordingSchema>,
): NvrArchiveRecordingModel {
  return {
    channel: item.channel,
    startTime: item.start_time,
    endTime: item.end_time,
    downloadUrl: item.download_url ?? null,
    exportUrl: item.export_url ?? null,
    filePath: item.file_path ?? null,
    type: item.type ?? null,
    videoStream: item.video_stream ?? null,
    disk: item.disk ?? null,
    partition: item.partition ?? null,
    cluster: item.cluster ?? null,
    lengthBytes: item.length_bytes ?? null,
    cutLengthBytes: item.cut_length_bytes ?? null,
    flags: item.flags,
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
