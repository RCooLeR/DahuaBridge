import type { HassEntity, HomeAssistant } from "../types/home-assistant";

export function currentCameraEntity(
  hass: HomeAssistant | undefined,
  entityId: string | undefined,
): HassEntity | undefined {
  if (!entityId?.startsWith("camera.")) {
    return undefined;
  }
  // A registry entry or restored state does not mean HA loaded a camera object.
  const entity = hass?.states[entityId];
  return entity && entity.attributes.restored !== true ? entity : undefined;
}

export function nativeCameraEntityAvailable(
  hass: HomeAssistant | undefined,
  entity: HassEntity | undefined,
): boolean {
  // Recheck current states instead of trusting a view model from before a reload.
  const current = currentCameraEntity(hass, entity?.entity_id);
  return Boolean(current && current.state !== "unavailable" && current.state !== "unknown");
}
