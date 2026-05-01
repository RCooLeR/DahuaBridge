import { z } from "zod";

const bridgeEventSchema = z
  .object({
    device_id: z.string().min(1),
    device_kind: z.string().min(1).optional().default("unknown"),
    child_id: z.string().min(1).optional(),
    code: z.string().min(1),
    action: z.string().min(1),
    index: z.coerce.number().int().optional(),
    channel: z.coerce.number().int().optional(),
    occurred_at: z.string().min(1),
    data: z.record(z.string(), z.unknown()).optional(),
  })
  .passthrough();

const bridgeEventsObjectResponseSchema = z
  .object({
    events: z.array(bridgeEventSchema),
  })
  .passthrough();

const bridgeEventsArrayResponseSchema = z.array(bridgeEventSchema);

export interface BridgeEventQuery {
  limit: number;
  deviceId?: string;
  childId?: string;
  code?: string;
  action?: string;
}

export type BridgeEvent = z.infer<typeof bridgeEventSchema>;

function parseBridgeEventsPayload(payload: unknown): BridgeEvent[] {
  const objectResult = bridgeEventsObjectResponseSchema.safeParse(payload);
  if (objectResult.success) {
    return objectResult.data.events;
  }

  const arrayResult = bridgeEventsArrayResponseSchema.safeParse(payload);
  if (arrayResult.success) {
    return arrayResult.data;
  }

  const message = objectResult.error.issues
    .map((issue) => `${issue.path.join(".") || "payload"}: ${issue.message}`)
    .join("; ");
  throw new Error(`Bridge event response schema mismatch: ${message}`);
}

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

  return parseBridgeEventsPayload(await response.json());
}
