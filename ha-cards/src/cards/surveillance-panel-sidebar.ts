import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import {
  displayCameraLabel,
  type CameraViewModel,
  type NvrViewModel,
  type PanelModel,
  type PanelSelection,
  type RoomViewModel,
  type SidebarFilter,
  type VtoViewModel,
} from "../domain/model";

type SidebarItemKind = "camera" | "vto" | "accessory" | "nvr";
type SidebarTone = "neutral" | "success" | "warning" | "critical";

interface RenderSurveillancePanelSidebarArgs {
  model: PanelModel;
  sidebarOpen: boolean;
  searchText: string;
  sidebarFilter: SidebarFilter;
  selection: PanelSelection;
  onSearchInput: (event: Event) => void;
  onSelectFilter: (filter: SidebarFilter) => void;
  onSelectNvr: (nvr: NvrViewModel) => void;
  onSelectCamera: (camera: CameraViewModel) => void;
  onSelectVto: (vto: VtoViewModel) => void;
  renderIcon: (icon: string) => TemplateResult;
  matchesSidebarFilters: (
    label: string,
    secondary: string,
    kind: SidebarItemKind,
    highlighted: boolean,
  ) => boolean;
}

function renderSidebarSectionLabel(
  label: string,
  icon: string,
  renderIcon: (icon: string) => TemplateResult,
): TemplateResult {
  return html`
    <div class="sidebar-section-head">
      <span class="sidebar-section-icon" aria-hidden="true">${renderIcon(icon)}</span>
      <span class="section-label">${label}</span>
    </div>
  `;
}

export function renderSurveillancePanelSidebar({
  model,
  sidebarOpen,
  searchText,
  sidebarFilter,
  selection,
  onSearchInput,
  onSelectFilter,
  onSelectNvr,
  onSelectCamera,
  onSelectVto,
  renderIcon,
  matchesSidebarFilters,
}: RenderSurveillancePanelSidebarArgs): TemplateResult {
  const vtos = model.vtos;
  const nvrDeviceIds = new Set(model.nvrs.map((nvr) => nvr.deviceId));
  const ipcCameras = model.cameras.filter((camera) => !nvrDeviceIds.has(camera.rootDeviceId));

  return html`
    <aside class="sidebar ${sidebarOpen ? "" : "mobile-hidden"}">
      <div class="toolbar">
        <input
          class="search"
          type="search"
          .value=${searchText}
          placeholder="Search devices"
          @input=${onSearchInput}
        />
        <div class="chip-row">
          ${renderFilterChip("all", "All", sidebarFilter, onSelectFilter)}
          ${renderFilterChip("alerts", "Alerts", sidebarFilter, onSelectFilter)}
          ${renderFilterChip("nvr", "NVR", sidebarFilter, onSelectFilter)}
          ${renderFilterChip("vto", "VTO", sidebarFilter, onSelectFilter)}
        </div>
      </div>

      <div class="sidebar-scroll">
        ${vtos.length > 0
          ? html`
              <section class="sidebar-group">
                ${renderSidebarSectionLabel("Door Stations", "mdi:doorbell-video", renderIcon)}
                ${repeat(
                  vtos,
                  (vto) => vto.deviceId,
                  (vto) =>
                    renderSidebarDeviceButton({
                      label: vto.label,
                      secondary: formatVtoSecondary(vto),
                      selected:
                        selection.kind === "vto" && selection.deviceId === vto.deviceId,
                      highlighted: vto.callState === "ringing" || vto.callState === "active",
                      badgeText:
                        vto.callState === "ringing"
                          ? "Ringing"
                          : vto.callState === "active"
                            ? "Active"
                            : undefined,
                      onClick: () => onSelectVto(vto),
                      tone: vto.online ? "success" : "critical",
                      kind: "vto",
                      renderIcon,
                      matchesSidebarFilters,
                    }),
                )}
              </section>
            `
          : nothing}

        <section class="sidebar-group">
          ${renderSidebarSectionLabel("NVR Systems", "mdi:server-network", renderIcon)}
          ${repeat(
            model.nvrs,
            (nvr) => nvr.deviceId,
            (nvr) => html`
              <section class="sidebar-nvr">
                ${renderSidebarDeviceButton({
                  label: nvr.label,
                  secondary: `${nvr.rooms.length} room${nvr.rooms.length === 1 ? "" : "s"} | ${nvr.disks.length} drive${nvr.disks.length === 1 ? "" : "s"}`,
                  selected: selection.kind === "nvr" && selection.deviceId === nvr.deviceId,
                  highlighted: !nvr.healthy,
                  badgeText: !nvr.healthy ? "Alert" : undefined,
                  onClick: () => onSelectNvr(nvr),
                  tone: !nvr.healthy ? "warning" : "success",
                  kind: "nvr",
                  renderIcon,
                  matchesSidebarFilters,
                })}
                ${repeat(
                  nvr.rooms.filter((room) =>
                    room.channels.some((camera) =>
                      matchesSidebarFilters(
                        displayCameraLabel(camera),
                        room.label,
                        "camera",
                        camera.detections.length > 0,
                      ),
                    ),
                  ),
                  (room) => `${nvr.deviceId}:${room.label}`,
                  (room) =>
                    renderSidebarRoomGroup({
                      nvrDeviceId: nvr.deviceId,
                      room,
                      selection,
                      onSelectCamera,
                      renderIcon,
                      matchesSidebarFilters,
                    }),
                )}
              </section>
            `,
          )}
        </section>

        ${ipcCameras.length > 0
          ? html`
              <section class="sidebar-group">
                ${renderSidebarSectionLabel("IPC Cameras", "mdi:cctv", renderIcon)}
                ${repeat(
                  ipcCameras.filter((camera) =>
                    matchesSidebarFilters(
                      displayCameraLabel(camera),
                      `${camera.roomLabel} | ${camera.kindLabel}`,
                      "camera",
                      camera.detections.length > 0,
                    ),
                  ),
                  (camera) => camera.deviceId,
                  (camera) =>
                    renderSidebarDeviceButton({
                      label: displayCameraLabel(camera),
                      secondary: `${camera.roomLabel} | ${camera.kindLabel}`,
                      selected:
                        selection.kind === "camera" && selection.deviceId === camera.deviceId,
                      highlighted: camera.detections.length > 0,
                      badgeText: camera.detections[0]?.label,
                      onClick: () => onSelectCamera(camera),
                      tone: camera.online
                        ? camera.detections.length > 0
                          ? "warning"
                          : "success"
                        : "critical",
                      kind: "camera",
                      renderIcon,
                      matchesSidebarFilters,
                    }),
                )}
              </section>
            `
          : nothing}
      </div>

      <div class="storage-widget">
        ${renderSidebarSectionLabel("NVR Storage", "mdi:harddisk", renderIcon)}
        <div class="storage-drives">
          ${model.nvrs.length > 0
            ? repeat(
                model.nvrs,
                (nvr) => `storage:${nvr.deviceId}`,
                (nvr) => renderStorageSummaryRow(nvr),
              )
            : html`<div class="muted">No NVR storage state discovered.</div>`}
        </div>
      </div>
    </aside>
  `;
}

function renderSidebarRoomGroup({
  nvrDeviceId,
  room,
  selection,
  onSelectCamera,
  renderIcon,
  matchesSidebarFilters,
}: {
  nvrDeviceId: string;
  room: RoomViewModel;
  selection: PanelSelection;
  onSelectCamera: (camera: CameraViewModel) => void;
  renderIcon: (icon: string) => TemplateResult;
  matchesSidebarFilters: (
    label: string,
    secondary: string,
    kind: SidebarItemKind,
    highlighted: boolean,
  ) => boolean;
}): TemplateResult {
  const cameras = room.channels.filter((camera) =>
    matchesSidebarFilters(
      displayCameraLabel(camera),
      room.label,
      "camera",
      camera.detections.length > 0,
    ),
  );
  const selected = cameras.some(
    (camera) =>
      selection.kind === "camera" && selection.deviceId === camera.deviceId,
  );
  const alertCount = cameras.filter((camera) => camera.detections.length > 0).length;
  const nvrSelected = selection.kind === "nvr" && selection.deviceId === nvrDeviceId;

  return html`
    <details class="sidebar-room-group" ?open=${selected || alertCount > 0 || nvrSelected}>
      <summary>
        <div class="sidebar-room-summary">
          <div class="sidebar-room-meta">
            <div class="sidebar-label">${room.label}</div>
            <div class="sidebar-secondary">${cameras.length} channels</div>
          </div>
          <div class="split-row">
            ${alertCount > 0
              ? html`<span class="badge warning">${alertCount} alerts</span>`
              : nothing}
            <span class="sidebar-room-toggle" aria-hidden="true">
              ${renderIcon("mdi:chevron-down")}
            </span>
          </div>
        </div>
      </summary>
      <div class="sidebar-room-cameras">
        ${repeat(
          cameras,
          (camera) => `${nvrDeviceId}:${camera.deviceId}`,
          (camera) =>
            renderSidebarDeviceButton({
              label: displayCameraLabel(camera),
              secondary: room.label,
              selected:
                selection.kind === "camera" && selection.deviceId === camera.deviceId,
              highlighted: camera.detections.length > 0,
              badgeText: camera.detections[0]?.label,
              onClick: () => onSelectCamera(camera),
              tone: camera.online
                ? camera.detections.length > 0
                  ? "warning"
                  : "success"
                : "critical",
              kind: "camera",
              renderIcon,
              matchesSidebarFilters,
            }),
        )}
      </div>
    </details>
  `;
}

function renderSidebarDeviceButton({
  label,
  secondary,
  selected,
  highlighted,
  badgeText,
  onClick,
  tone,
  kind,
  renderIcon,
  matchesSidebarFilters,
}: {
  label: string;
  secondary: string;
  selected: boolean;
  highlighted: boolean;
  badgeText: string | undefined;
  onClick: () => void;
  tone: SidebarTone;
  kind: SidebarItemKind;
  renderIcon: (icon: string) => TemplateResult;
  matchesSidebarFilters: (
    label: string,
    secondary: string,
    kind: SidebarItemKind,
    highlighted: boolean,
  ) => boolean;
}): TemplateResult | typeof nothing {
  if (!matchesSidebarFilters(label, secondary, kind, highlighted)) {
    return nothing;
  }

  const handleClick = (event: Event): void => {
    event.preventDefault();
    event.stopPropagation();
    onClick();
  };

  return html`
    <button
      class="sidebar-item ${selected ? "selected" : ""} ${highlighted ? "highlighted" : ""}"
      type="button"
      title=${label}
      @click=${handleClick}
    >
      <div class="sidebar-item-title">
        <span class="sidebar-glyph" aria-hidden="true">
          ${renderIcon(sidebarItemIcon(label, secondary))}
        </span>
        <span
          class="status-dot ${tone === "critical" ? "critical" : tone === "warning" ? "warning" : ""}"
        ></span>
        <div class="sidebar-copy">
          <div class="sidebar-label">${label}</div>
          <div class="sidebar-secondary">${secondary}</div>
        </div>
      </div>
      ${badgeText
        ? html`<span
            class="badge ${tone === "success"
              ? "success"
              : tone === "warning"
                ? "warning"
                : tone === "critical"
                  ? "critical"
                  : ""}"
            >${badgeText}</span
          >`
        : nothing}
    </button>
  `;
}

function sidebarItemIcon(label: string, secondary: string): string {
  const haystack = `${label} ${secondary}`.toLowerCase();
  if (haystack.includes("door")) {
    return "mdi:doorbell-video";
  }
  if (haystack.includes("recorder") || haystack.includes("nvr")) {
    return "mdi:harddisk";
  }
  return "mdi:cctv";
}

function renderFilterChip(
  filter: SidebarFilter,
  label: string,
  activeFilter: SidebarFilter,
  onSelectFilter: (filter: SidebarFilter) => void,
): TemplateResult {
  const handleClick = (event: Event): void => {
    event.preventDefault();
    event.stopPropagation();
    onSelectFilter(filter);
  };
  return html`
    <button
      class="chip ${activeFilter === filter ? "active" : ""}"
      type="button"
      title=${label}
      @click=${handleClick}
    >
      ${label}
    </button>
  `;
}

function renderStorageSummaryRow(nvr: NvrViewModel): TemplateResult {
  return html`
    <div class="storage-drive">
      <div class="storage-drive-head">
        <div class="sidebar-label">${nvr.label}</div>
        <div class="split-row">
          <span class="badge ${nvr.healthy ? "success" : "warning"}">
            ${nvr.healthy ? "Healthy" : "Attention"}
          </span>
          <span class="badge ${nvr.recordingActive ? "critical" : "info"}">
            ${nvr.recordingActive ? "Recording" : "Standby"}
          </span>
        </div>
      </div>
      <div class="progress">
        <div
          class="progress-bar"
          style=${`width:${Math.max(0, Math.min(100, nvr.storageUsedPercent ?? 0))}%`}
        ></div>
      </div>
      <div class="storage-drive-meta">
        <span class="muted">
          ${nvr.storageUsedPercent !== null ? `${Math.round(nvr.storageUsedPercent)}% used` : "Usage unknown"}
        </span>
        <span class="muted">${nvr.storageText}</span>
      </div>
      <div class="muted">${nvr.disks.length} drive${nvr.disks.length === 1 ? "" : "s"}</div>
    </div>
  `;
}

function formatVtoSecondary(vto: VtoViewModel): string {
  const parts = [
    vto.roomLabel && vto.roomLabel !== "Unassigned" ? vto.roomLabel : "Door Station",
  ];
  if (vto.lockCount > 0) {
    parts.push(`${vto.lockCount} lock${vto.lockCount === 1 ? "" : "s"}`);
  }
  if (vto.alarmCount > 0) {
    parts.push(`${vto.alarmCount} alarm${vto.alarmCount === 1 ? "" : "s"}`);
  }
  return parts.join(" | ");
}
