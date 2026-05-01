import type { HomeAssistant } from "../types/home-assistant";
import { logCardInfo, redactUrlForLog } from "../utils/logging";

export interface BridgeRequestOptions {
  method?: "POST" | "GET";
  body?: Record<string, unknown>;
}

export async function pressButton(
  hass: HomeAssistant,
  entityId: string,
): Promise<void> {
  await hass.callService("button", "press", { entity_id: entityId });
}

export async function toggleSwitch(
  hass: HomeAssistant,
  entityId: string,
  enabled: boolean,
): Promise<void> {
  await hass.callService("switch", enabled ? "turn_on" : "turn_off", {
    entity_id: entityId,
  });
}

export async function setNumberValue(
  hass: HomeAssistant,
  entityId: string,
  value: number,
): Promise<void> {
  await hass.callService("number", "set_value", {
    entity_id: entityId,
    value,
  });
}

export async function postBridgeRequest(
  targetUrl: string,
  options: BridgeRequestOptions = {},
): Promise<void> {
  const method = options.method ?? "POST";
  const started = performance.now();
  logBridgeRequest("request", method, targetUrl);
  const response = await fetch(targetUrl, {
    method,
    headers: {
      "Content-Type": "application/json",
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  logBridgeRequest("response", method, targetUrl, response.status, performance.now() - started);

  if (!response.ok) {
    throw new Error(`Bridge request failed with status ${response.status}`);
  }
}

export async function readBridgeJson<T>(targetUrl: string): Promise<T> {
  const started = performance.now();
  logBridgeRequest("request", "GET", targetUrl);
  const response = await fetch(targetUrl, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
    },
  });
  logBridgeRequest("response", "GET", targetUrl, response.status, performance.now() - started);

  if (!response.ok) {
    throw new Error(`Bridge request failed with status ${response.status}`);
  }

  return (await response.json()) as T;
}

function logBridgeRequest(
  phase: "request" | "response",
  method: string,
  targetUrl: string,
  status?: number,
  durationMs?: number,
): void {
  logCardInfo(`card bridge ${phase}`, {
    method,
    url: redactUrlForLog(targetUrl),
    status,
    duration_ms: durationMs === undefined ? undefined : Math.round(durationMs),
  });
}
