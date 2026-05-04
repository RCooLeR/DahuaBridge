import { html, nothing, type TemplateResult } from "lit";

import type { CameraViewModel } from "../domain/model";
import { createLocalizer, type Localizer } from "../localization";

export function renderCameraEventCountBadges(
  camera: CameraViewModel,
  variant: "inline" | "overlay",
  t: Localizer = createLocalizer("en"),
): TemplateResult | typeof nothing {
  const humanCount = positiveEventCount(camera.humanCount24h);
  const vehicleCount = positiveEventCount(camera.vehicleCount24h);
  const ivsCount = positiveEventCount(camera.ivsCount24h);
  const categorizedCount = humanCount + vehicleCount + ivsCount;
  const uncategorizedEventCount =
    categorizedCount <= 0 ? positiveEventCount(camera.eventCount24h) : 0;
  if (
    humanCount <= 0 &&
    vehicleCount <= 0 &&
    ivsCount <= 0 &&
    uncategorizedEventCount <= 0
  ) {
    return nothing;
  }

  return html`
    <div class="tile-event-counts tile-event-counts-${variant}">
      ${humanCount > 0
        ? html`
            <span
              class="tile-event-count info"
              title="${humanCount} ${t("event.human").toLowerCase()} ${t("metric.events24h").toLowerCase()}"
              aria-label="${humanCount} ${t("event.human").toLowerCase()} ${t("metric.events24h").toLowerCase()}"
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
              title="${vehicleCount} ${t("event.vehicle").toLowerCase()} ${t("metric.events24h").toLowerCase()}"
              aria-label="${vehicleCount} ${t("event.vehicle").toLowerCase()} ${t("metric.events24h").toLowerCase()}"
            >
              <ha-icon .icon=${"mdi:car"}></ha-icon>
              <span>${vehicleCount}</span>
            </span>
          `
        : nothing}
      ${ivsCount > 0
        ? html`
            <span
              class="tile-event-count warning"
              title="${ivsCount} ${t("metric.ivs")} ${t("metric.events24h").toLowerCase()}"
              aria-label="${ivsCount} ${t("metric.ivs")} ${t("metric.events24h").toLowerCase()}"
            >
              <ha-icon .icon=${"mdi:vector-square"}></ha-icon>
              <span>${ivsCount}</span>
            </span>
          `
        : nothing}
      ${uncategorizedEventCount > 0
        ? html`
            <span
              class="tile-event-count warning"
              title="${uncategorizedEventCount} SMD/IVS ${t("metric.events24h").toLowerCase()}"
              aria-label="${uncategorizedEventCount} SMD/IVS ${t("metric.events24h").toLowerCase()}"
            >
              <ha-icon .icon=${"mdi:motion-sensor"}></ha-icon>
              <span>${uncategorizedEventCount}</span>
            </span>
          `
        : nothing}
    </div>
  `;
}

function positiveEventCount(value: number | null | undefined): number {
  return typeof value === "number" && Number.isFinite(value)
    ? Math.max(0, Math.trunc(value))
    : 0;
}
