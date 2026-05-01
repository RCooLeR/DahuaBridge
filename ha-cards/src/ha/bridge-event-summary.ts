import { z } from "zod";

import type {
  NvrEventChannelSummaryModel,
  NvrEventSummaryCountModel,
  NvrEventSummaryModel,
} from "../domain/event-summary";
import { buildBridgeEndpointUrl } from "./bridge-url";

const summaryCountSchema = z.object({
  code: z.string(),
  label: z.string().optional().nullable(),
  count: z.coerce.number().int(),
});

const summaryChannelSchema = z.object({
  channel: z.coerce.number().int(),
  total_count: z.coerce.number().int(),
  items: z.array(summaryCountSchema).default([]),
});

const summarySchema = z.object({
  device_id: z.string(),
  start_time: z.string(),
  end_time: z.string(),
  total_count: z.coerce.number().int(),
  items: z.array(summaryCountSchema).default([]),
  channels: z.array(summaryChannelSchema).default([]),
});

export interface NvrEventSummaryQuery {
  startTime: string;
  endTime: string;
  eventCode?: string;
}

export async function fetchNvrEventSummary(
  summaryUrl: string,
  query: NvrEventSummaryQuery,
  signal?: AbortSignal,
): Promise<NvrEventSummaryModel> {
  const url = new URL(summaryUrl);
  url.searchParams.set("start", query.startTime);
  url.searchParams.set("end", query.endTime);
  if (query.eventCode?.trim()) {
    url.searchParams.set("event", query.eventCode.trim());
  }

  const response = await fetch(url, {
    method: "GET",
    headers: {
      Accept: "application/json",
    },
    signal,
  });
  if (!response.ok) {
    throw new Error(`Bridge event summary request failed with status ${response.status}`);
  }

  return mapNvrEventSummary(summarySchema.parse(await response.json()));
}

export function buildNvrEventSummaryUrl(
  bridgeBaseUrl: string | null,
  deviceId: string,
): string | null {
  if (!bridgeBaseUrl?.trim() || !deviceId.trim()) {
    return null;
  }
  return buildBridgeEndpointUrl(
    bridgeBaseUrl,
    `/api/v1/nvr/${encodeURIComponent(deviceId)}/events/summary`,
  );
}

function mapNvrEventSummary(
  payload: z.infer<typeof summarySchema>,
): NvrEventSummaryModel {
  return {
    deviceId: payload.device_id,
    startTime: payload.start_time,
    endTime: payload.end_time,
    totalCount: payload.total_count,
    items: payload.items.map(mapSummaryCount),
    channels: payload.channels.map(mapSummaryChannel),
  };
}

function mapSummaryChannel(
  channel: z.infer<typeof summaryChannelSchema>,
): NvrEventChannelSummaryModel {
  return {
    channel: channel.channel,
    totalCount: channel.total_count,
    items: channel.items.map(mapSummaryCount),
  };
}

function mapSummaryCount(
  item: z.infer<typeof summaryCountSchema>,
): NvrEventSummaryCountModel {
  return {
    code: item.code,
    label: item.label?.trim() || item.code,
    count: item.count,
  };
}
