import {describe, expect, it, vi} from "vitest";
import {currentCameraEntity, nativeCameraEntityAvailable} from "../src/ha/native-camera";
import type {HassEntity, HomeAssistant} from "../src/types/home-assistant";

function fixture(): {hass: HomeAssistant; entity: HassEntity} {
  const entity: HassEntity = {
    entity_id: "camera.renamed_gate", state: "idle", attributes: {},
    last_changed: "2026-05-01T00:00:00Z", last_updated: "2026-05-01T00:00:00Z",
  };
  return {entity, hass: {states: {[entity.entity_id]: entity}, callService: vi.fn().mockResolvedValue(undefined)}};
}

describe("native camera lifecycle", () => {
  it("uses actual current HA entity IDs and rejects missing, restored, and unavailable cameras", () => {
    const {hass, entity} = fixture();
    expect(nativeCameraEntityAvailable(hass, entity)).toBe(true);
    expect(currentCameraEntity(hass, "camera.generated_gate")).toBeUndefined();
    delete hass.states[entity.entity_id];
    expect(nativeCameraEntityAvailable(hass, entity)).toBe(false);
    hass.states[entity.entity_id] = {...entity, attributes: {restored: true}};
    expect(currentCameraEntity(hass, entity.entity_id)).toBeUndefined();
    hass.states[entity.entity_id] = {...entity, state: "unavailable"};
    expect(nativeCameraEntityAvailable(hass, entity)).toBe(false);
  });

});
