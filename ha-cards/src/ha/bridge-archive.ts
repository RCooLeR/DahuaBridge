import { z } from "zod";

import type {
  NvrArchiveRecordingModel,
  NvrArchiveSearchResultModel,
} from "../domain/archive";

const archiveRecordingSchema = z.object({
  channel: z.number().int(),
  start_time: z.string().min(1),
  end_time: z.string().min(1),
  download_url: z.string().optional().nullable(),
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

export interface ArchiveRecordingsQuery {
  channel: number;
  startTime: string;
  endTime: string;
  limit: number;
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

  const response = await fetch(url, {
    method: "GET",
    headers: {
      Accept: "application/json",
    },
    signal,
  });

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
