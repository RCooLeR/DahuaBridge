import type { HassEntity, HomeAssistant } from "../types/home-assistant";

export function entityById(
  hass: HomeAssistant,
  entityId: string | undefined,
): HassEntity | undefined {
  if (!entityId) {
    return undefined;
  }

  return hass.states[entityId];
}

export function entityState(
  hass: HomeAssistant,
  entityId: string | undefined,
): string | undefined {
  return entityById(hass, entityId)?.state;
}

export function entityBooleanState(
  hass: HomeAssistant,
  entityId: string | undefined,
): boolean {
  const state = entityState(hass, entityId);
  return state === "on" || state === "true";
}

export function entityNumberState(
  hass: HomeAssistant,
  entityId: string | undefined,
): number | null {
  const state = entityState(hass, entityId);
  if (!state) {
    return null;
  }

  const parsed = Number.parseFloat(state);
  return Number.isFinite(parsed) ? parsed : null;
}

export function entityFriendlyName(entity: HassEntity | undefined): string {
  const name = entity?.attributes.friendly_name;
  return typeof name === "string" && name.trim() ? name : entity?.entity_id ?? "";
}
