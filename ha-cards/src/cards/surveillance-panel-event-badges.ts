import { html, nothing, type TemplateResult } from "lit";

import type { CameraViewModel } from "../domain/model";

export function renderCameraEventCountBadges(
  camera: CameraViewModel,
  variant: "inline" | "overlay",
): TemplateResult | typeof nothing {
  const humanCount = Math.max(0, Math.trunc(camera.humanCount24h));
  const vehicleCount = Math.max(0, Math.trunc(camera.vehicleCount24h));
  if (humanCount <= 0 && vehicleCount <= 0) {
    return nothing;
  }

  return html`
    <div class="tile-event-counts tile-event-counts-${variant}">
      ${humanCount > 0
        ? html`
            <span
              class="tile-event-count info"
              title="${humanCount} person events in the last 24 hours"
              aria-label="${humanCount} person events in the last 24 hours"
            >
              <ha-icon .icon=${"mdi:account"}></ha-icon>
              <span>${humanCount}</span>
            </span>
          `
        : nothing}
      ${vehicleCount > 0
        ? html`
            <span
              class="tile-event-count warning"
              title="${vehicleCount} vehicle events in the last 24 hours"
              aria-label="${vehicleCount} vehicle events in the last 24 hours"
            >
              <ha-icon .icon=${"mdi:car"}></ha-icon>
              <span>${vehicleCount}</span>
            </span>
          `
        : nothing}
    </div>
  `;
}
