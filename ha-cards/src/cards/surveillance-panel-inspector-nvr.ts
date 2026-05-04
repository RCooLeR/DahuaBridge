import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import type { NvrViewModel } from "../domain/model";
import { pluralUnit, type Localizer } from "../localization";
import type { DetailTab } from "./surveillance-panel-state";
import { renderNvrSummaryChip, type RenderIconFn } from "./surveillance-panel-inspector-shared";
import { renderSegmentButton } from "./surveillance-panel-primitives";

export function renderNvrInspector(
  nvr: NvrViewModel,
  t: Localizer,
  renderIcon: RenderIconFn,
  detailTab: DetailTab,
  archiveContent: TemplateResult | typeof nothing,
  onSelectDetailTab: (tab: DetailTab) => void,
): TemplateResult {
  const channels = nvr.rooms.flatMap((room) => room.channels);
  const recordingCount = channels.filter((channel) => channel.recordingActive).length;
  const alertCount = channels.filter((channel) => channel.detections.length > 0).length;

  return html`
    <div class="detail-header">
      <div class="detail-title">${nvr.label}</div>
      <div class="muted">${nvr.roomLabel}</div>
    </div>
    <div class="detail-tabs">
      ${renderSegmentButton("overview", t("tab.overview"), detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("recordings", t("tab.recordings"), detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
    </div>
    <div class="detail-main">
      ${detailTab === "overview"
        ? html`
            <div class="panel">
              <div class="panel-title">${t("inspector.recorder")}</div>
              <div class="nvr-summary-chip-grid">
                ${renderNvrSummaryChip(
                  "mdi:lan-connect",
                  t("inspector.connection"),
                  nvr.online ? t("state.connected") : t("state.offline"),
                  nvr.online ? "success" : "critical",
                  renderIcon,
                )}
                ${renderNvrSummaryChip(
                  "mdi:record-rec",
                  t("inspector.recorder"),
                  nvr.recordingActive ? t("state.recordingActive") : t("state.recordingIdle"),
                  nvr.recordingActive ? "critical" : "info",
                  renderIcon,
                )}
                ${renderNvrSummaryChip(
                  "mdi:harddisk",
                  t("inspector.storage"),
                  nvr.storageText,
                  nvr.healthy ? "success" : "warning",
                  renderIcon,
                )}
                ${nvr.nvrConfigWritable !== null
                  ? renderNvrSummaryChip(
                      "mdi:cog-refresh-outline",
                      t("inspector.configWrites"),
                      nvr.nvrConfigWritable ? t("state.writable") : t("state.blocked"),
                      nvr.nvrConfigWritable ? "success" : "warning",
                      renderIcon,
                    )
                  : nothing}
                ${renderNvrSummaryChip(
                  "mdi:cctv",
                  t("inspector.channelsRecording"),
                  `${recordingCount} ${t("state.recording").toLowerCase()}`,
                  "info",
                  renderIcon,
                )}
                ${alertCount > 0
                  ? renderNvrSummaryChip(
                      "mdi:alert-outline",
                      t("inspector.alerts"),
                      `${alertCount} ${pluralUnit(alertCount, "unit.alert", "unit.alerts", t)}`,
                      "warning",
                      renderIcon,
                    )
                  : nothing}
              </div>
            </div>
            ${renderNvrDriveBreakdown(nvr, renderIcon, t)}
          `
        : nothing}
      ${detailTab === "recordings" ? archiveContent : nothing}
    </div>
  `;
}

function renderNvrDriveBreakdown(
  nvr: NvrViewModel,
  renderIcon: RenderIconFn,
  t: Localizer,
): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">${t("inspector.driveInventory")}</div>
      <div class="storage-drives inspector-storage-drives">
        ${nvr.disks.length > 0
          ? repeat(
              nvr.disks,
              (disk) => disk.deviceId,
              (disk) => html`
                <div class="storage-drive inspector-storage-drive">
                  <div class="storage-drive-head">
                    <span class="split-row">
                      <span class="sidebar-glyph" aria-hidden="true">
                        ${renderIcon("mdi:harddisk")}
                      </span>
                      <span class="sidebar-label">${disk.label}</span>
                    </span>
                    <span class="badge ${disk.healthy ? "success" : "critical"}">
                      ${disk.healthy ? t("state.healthy") : t("state.attention")}
                    </span>
                  </div>
                  <div class="progress">
                    <div
                      class="progress-bar"
                      style=${`width:${Math.max(0, Math.min(100, disk.usedPercent ?? 0))}%`}
                    ></div>
                  </div>
                  <div class="storage-drive-meta">
                    <span class="badge ${disk.online ? "success" : "critical"}">
                      ${disk.online ? t("state.online") : t("state.offline")}
                    </span>
                    <span class="badge info">
                      ${disk.usedPercent !== null
                        ? t("storage.percentUsed", { value: Math.round(disk.usedPercent) })
                        : t("storage.usageUnknown")}
                    </span>
                    <span class="badge">
                      ${disk.stateText ?? t("state.unknown")}
                    </span>
                  </div>
                  <div class="storage-drive-meta">
                    <span class="sidebar-secondary">${disk.usedBytesText} / ${disk.totalBytesText}</span>
                    <span class="sidebar-secondary">${disk.stateText ?? t("state.unknown")}</span>
                  </div>
                </div>
              `,
            )
          : html`<div class="muted">${t("inspector.noDrives")}</div>`}
      </div>
    </div>
  `;
}
