import type { HomeAssistant } from "../types/home-assistant";

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
  const response = await fetch(targetUrl, {
    method: options.method ?? "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });

  if (!response.ok) {
    throw new Error(`Bridge request failed with status ${response.status}`);
  }
}

export async function readBridgeJson<T>(targetUrl: string): Promise<T> {
  const response = await fetch(targetUrl, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
    },
  });

  if (!response.ok) {
    throw new Error(`Bridge request failed with status ${response.status}`);
  }

  return (await response.json()) as T;
}
