import { z } from "zod";

const bridgeEventSchema = z.object({
  device_id: z.string().min(1),
  device_kind: z.string().min(1),
  child_id: z.string().min(1).optional(),
  code: z.string().min(1),
  action: z.string().min(1),
  index: z.number().int().optional(),
  channel: z.number().int().optional(),
  occurred_at: z.string().min(1),
  data: z.record(z.string(), z.string()).optional(),
});

const bridgeEventsResponseSchema = z.object({
  events: z.array(bridgeEventSchema),
});

export interface BridgeEventQuery {
  limit: number;
  deviceId?: string;
  childId?: string;
  code?: string;
  action?: string;
}

export type BridgeEvent = z.infer<typeof bridgeEventSchema>;

export async function fetchBridgeEvents(
  eventsUrl: string,
  query: BridgeEventQuery,
  signal?: AbortSignal,
): Promise<BridgeEvent[]> {
  const url = new URL(eventsUrl);
  url.searchParams.set("limit", String(Math.max(1, Math.trunc(query.limit))));

  if (query.deviceId) {
    url.searchParams.set("device_id", query.deviceId);
  }
  if (query.childId) {
    url.searchParams.set("child_id", query.childId);
  }
  if (query.code) {
    url.searchParams.set("code", query.code);
  }
  if (query.action) {
    url.searchParams.set("action", query.action);
  }

  const response = await fetch(url, {
    method: "GET",
    headers: {
      Accept: "application/json",
    },
    signal,
  });

  if (!response.ok) {
    throw new Error(`Bridge event request failed with status ${response.status}`);
  }

  const payload = bridgeEventsResponseSchema.parse(await response.json());
  return payload.events;
}
