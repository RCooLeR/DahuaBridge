import type { SurveillancePanelCardConfig } from "../types/card-config";

// Dashboard defaults must be available without importing the optional editor.
export function createSurveillancePanelStubConfig(): SurveillancePanelCardConfig {
  return {
    type: "custom:dahuabridge-surveillance-panel",
    event_lookback_hours: 12,
    bridge_event_poll_seconds: 15,
    max_events: 14,
  };
}
