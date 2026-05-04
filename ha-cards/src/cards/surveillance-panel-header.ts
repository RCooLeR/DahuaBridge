import { html, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import { displayCameraLabel, type HeaderMetric, type PanelModel, type VtoViewModel } from "../domain/model";
import type { Localizer } from "../localization";

interface RenderSurveillancePanelHeaderArgs {
  model: PanelModel;
  t: Localizer;
  bridgeLogoUrl: string;
  sidebarOpen: boolean;
  inspectorOpen: boolean;
  inspectorAvailable: boolean;
  onSelectOverview: () => void;
  onToggleSidebar: () => void;
  onToggleInspector: () => void;
  renderIcon: (icon: string) => TemplateResult;
  vtoBadgeTone: (vto: VtoViewModel) => HeaderMetric["tone"];
}

export function renderSurveillancePanelHeader({
  model,
  t,
  bridgeLogoUrl,
  sidebarOpen,
  inspectorOpen,
  inspectorAvailable,
  onSelectOverview,
  onToggleSidebar,
  onToggleInspector,
  renderIcon,
  vtoBadgeTone,
}: RenderSurveillancePanelHeaderArgs): TemplateResult {
  const handleSelectOverview = (event: Event): void => {
    event.preventDefault();
    event.stopPropagation();
    onSelectOverview();
  };
  const handleToggleSidebar = (event: Event): void => {
    event.preventDefault();
    event.stopPropagation();
    onToggleSidebar();
  };
  const handleToggleInspector = (event: Event): void => {
    event.preventDefault();
    event.stopPropagation();
    onToggleInspector();
  };
  return html`
    <header class="header">
      <div class="header-logo">
        <button
          class="header-logo-button"
          type="button"
          title=${t("header.returnOverview")}
          @click=${handleSelectOverview}
          aria-label=${t("header.returnOverview")}
        >
          <img src=${bridgeLogoUrl} alt="" aria-hidden="true" />
        </button>
      </div>
      <div class="header-chip-row">
        ${repeat(
          model.headerMetrics,
          (metric) => metric.label,
          (metric) => renderHeaderChip(metric.label, metric.value, metric.tone, renderIcon),
        )}
        ${model.selectedCamera
          ? renderInteractiveHeaderChip(
              t("metric.focus"),
              displayCameraLabel(model.selectedCamera),
              model.selectedCamera.online ? "success" : "critical",
              renderIcon,
              onSelectOverview,
              t("header.returnOverview"),
            )
          : model.selectedVto
            ? renderInteractiveHeaderChip(
                t("metric.doorStation"),
                model.selectedVto.label,
                vtoBadgeTone(model.selectedVto),
                renderIcon,
                onSelectOverview,
                t("header.returnOverview"),
              )
            : renderInteractiveHeaderChip(
                t("metric.view"),
                t("kind.overview"),
                "info",
                renderIcon,
                onSelectOverview,
                t("header.overviewSelected"),
                true,
              )}
      </div>
      <div class="header-actions">
        <button
          class="icon-button"
          type="button"
          title=${sidebarOpen ? t("header.hideSidebar") : t("header.showSidebar")}
          @click=${handleToggleSidebar}
          aria-label=${sidebarOpen ? t("header.hideSidebar") : t("header.showSidebar")}
          aria-pressed=${String(sidebarOpen)}
        >
          ${renderIcon("mdi:menu")}
        </button>
        <button
          class="icon-button"
          type="button"
          title=${inspectorOpen ? t("header.hideInspector") : t("header.showInspector")}
          @click=${handleToggleInspector}
          aria-label=${inspectorOpen ? t("header.hideInspector") : t("header.showInspector")}
          aria-pressed=${String(inspectorOpen)}
          ?disabled=${!inspectorAvailable}
        >
          ${renderIcon("mdi:tune")}
        </button>
      </div>
    </header>
  `;
}

function renderHeaderChip(
  label: string,
  value: string,
  tone: HeaderMetric["tone"],
  renderIcon: (icon: string) => TemplateResult,
): TemplateResult {
  return html`
    <div class="header-chip">
      <div class="header-chip-icon" aria-hidden="true">
        ${renderIcon(headerChipIcon(label))}
      </div>
      <div class="header-chip-copy">
        <div class="header-chip-label">${label}</div>
        <div class="header-chip-value tone-${tone}">${value}</div>
      </div>
    </div>
  `;
}

function renderInteractiveHeaderChip(
  label: string,
  value: string,
  tone: HeaderMetric["tone"],
  renderIcon: (icon: string) => TemplateResult,
  onClick: () => void,
  title: string,
  disabled = false,
): TemplateResult {
  const handleClick = (event: Event): void => {
    event.preventDefault();
    event.stopPropagation();
    onClick();
  };
  return html`
    <button
      class="header-chip header-chip-button"
      type="button"
      title=${title}
      ?disabled=${disabled}
      @click=${handleClick}
    >
      <div class="header-chip-icon" aria-hidden="true">
        ${renderIcon(headerChipIcon(label))}
      </div>
      <div class="header-chip-copy">
        <div class="header-chip-label">${label}</div>
        <div class="header-chip-value tone-${tone}">${value}</div>
      </div>
    </button>
  `;
}

function headerChipIcon(label: string): string {
  const normalized = label.toLowerCase();
  if (normalized.includes("camera") || normalized.includes("камер")) {
    return "mdi:cctv";
  }
  if (normalized.includes("24") || normalized.includes("под")) {
    return "mdi:calendar-today";
  }
  if (normalized.includes("motion") || normalized.includes("рух")) {
    return "mdi:motion-sensor";
  }
  if (normalized.includes("human") || normalized.includes("людин")) {
    return "mdi:account-alert-outline";
  }
  if (normalized.includes("vehicle") || normalized.includes("авто")) {
    return "mdi:car";
  }
  if (normalized.includes("ivs")) {
    return "mdi:shield-search";
  }
  if (normalized.includes("nvr")) {
    return "mdi:harddisk";
  }
  if (normalized.includes("focus") || normalized.includes("фокус")) {
    return "mdi:crosshairs-gps";
  }
  if (normalized.includes("door") || normalized.includes("виклич")) {
    return "mdi:doorbell-video";
  }
  if (normalized.includes("view") || normalized.includes("вигляд")) {
    return "mdi:view-grid-outline";
  }
  switch (label) {
    case "Cameras Online":
      return "mdi:cctv";
    case "Events 24H":
      return "mdi:calendar-today";
    case "Motion":
      return "mdi:motion-sensor";
    case "Human":
      return "mdi:account-alert-outline";
    case "Vehicle":
      return "mdi:car";
    case "IVS":
      return "mdi:shield-search";
    case "NVR Health":
      return "mdi:harddisk";
    case "Focus":
      return "mdi:crosshairs-gps";
    case "Door Station":
      return "mdi:doorbell-video";
    case "View":
      return "mdi:view-grid-outline";
    default:
      return "mdi:gauge";
  }
}
