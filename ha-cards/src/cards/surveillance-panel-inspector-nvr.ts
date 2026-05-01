import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import type { NvrViewModel } from "../domain/model";
import type { DetailTab } from "./surveillance-panel-state";
import { renderNvrSummaryChip, type RenderIconFn } from "./surveillance-panel-inspector-shared";
import { renderSegmentButton } from "./surveillance-panel-primitives";

export function renderNvrInspector(
  nvr: NvrViewModel,
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
      ${renderSegmentButton("overview", "Overview", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("recordings", "Recordings", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
    </div>
    <div class="detail-main">
      ${detailTab === "overview"
        ? html`
            <div class="panel">
              <div class="panel-title">Recorder</div>
              <div class="nvr-summary-chip-grid">
                ${renderNvrSummaryChip(
                  "mdi:lan-connect",
                  "Connection",
                  nvr.online ? "Connected" : "Offline",
                  nvr.online ? "success" : "critical",
                  renderIcon,
                )}
                ${renderNvrSummaryChip(
                  "mdi:record-rec",
                  "Recorder",
                  nvr.recordingActive ? "Recording active" : "Recording idle",
                  nvr.recordingActive ? "critical" : "info",
                  renderIcon,
                )}
                ${renderNvrSummaryChip(
                  "mdi:harddisk",
                  "Storage",
                  nvr.storageText,
                  nvr.healthy ? "success" : "warning",
                  renderIcon,
                )}
                ${nvr.nvrConfigWritable !== null
                  ? renderNvrSummaryChip(
                      "mdi:cog-refresh-outline",
                      "Config Writes",
                      nvr.nvrConfigWritable ? "Writable" : "Blocked",
                      nvr.nvrConfigWritable ? "success" : "warning",
                      renderIcon,
                    )
                  : nothing}
                ${renderNvrSummaryChip(
                  "mdi:cctv",
                  "Channels recording",
                  `${recordingCount} recording`,
                  "info",
                  renderIcon,
                )}
                ${alertCount > 0
                  ? renderNvrSummaryChip(
                      "mdi:alert-outline",
                      "Alerts",
                      `${alertCount} alert${alertCount === 1 ? "" : "s"}`,
                      "warning",
                      renderIcon,
                    )
                  : nothing}
              </div>
            </div>
            ${renderNvrDriveBreakdown(nvr, renderIcon)}
          `
        : nothing}
      ${detailTab === "recordings" ? archiveContent : nothing}
    </div>
  `;
}

function renderNvrDriveBreakdown(
  nvr: NvrViewModel,
  renderIcon: RenderIconFn,
): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">Drive Inventory</div>
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
                      ${disk.healthy ? "Healthy" : "Fault"}
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
                      ${disk.online ? "Online" : "Offline"}
                    </span>
                    <span class="badge info">
                      ${disk.usedPercent !== null ? `${Math.round(disk.usedPercent)}% used` : "Usage unknown"}
                    </span>
                    <span class="badge">
                      ${disk.stateText ?? "State unknown"}
                    </span>
                  </div>
                  <div class="storage-drive-meta">
                    <span class="sidebar-secondary">${disk.usedBytesText} / ${disk.totalBytesText}</span>
                    <span class="sidebar-secondary">${disk.stateText ?? "State unknown"}</span>
                  </div>
                </div>
              `,
            )
          : html`<div class="muted">No drive child devices discovered for this recorder.</div>`}
      </div>
    </div>
  `;
}
