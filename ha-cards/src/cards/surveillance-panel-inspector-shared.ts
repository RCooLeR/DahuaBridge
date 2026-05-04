import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import type { VtoLockViewModel, VtoViewModel } from "../domain/model";
import { createLocalizer, pluralUnit, type Localizer } from "../localization";
import { formatDuration } from "../utils/format";
import { renderControlButton } from "./surveillance-panel-primitives";

export type RenderIconFn = (icon: string) => TemplateResult;
export type IsBusyFn = (key: string) => boolean;
export type OnVtoSwitchAction = (
  key: string,
  entityId: string,
  enabled: boolean,
  fallbackUrl: string | null,
  payloadKey: string,
) => Promise<void>;
export type OnVtoButtonAction = (
  key: string,
  entityId: string,
  fallbackUrl: string | null,
) => Promise<void>;

export function streamProfileLabel(
  value: string | null | undefined,
  t: Localizer = createLocalizer("en"),
): string {
  switch (value?.trim().toLowerCase()) {
    case "quality":
    case "default":
      return t("profile.quality");
    case "stable":
    case "substream":
      return t("profile.stable");
    default:
      return value?.trim() || t("profile.unknown");
  }
}

export function streamSourceLabel(
  value: string | null | undefined,
  t: Localizer = createLocalizer("en"),
): string {
  switch (value?.trim().toLowerCase()) {
    case "native":
    case "ha":
    case "homeassistant":
    case "home_assistant":
    case "onvif":
    case "rtsp":
    case "direct_rtsp":
      return t("source.nativeHaRtsp");
    case "dash":
      return t("source.bridgeDash");
    case "hls":
      return t("source.bridgeHls");
    case "webrtc":
      return t("source.bridgeWebRtc");
    case "mjpeg":
      return t("source.bridgeMjpeg");
    case "auto":
      return t("source.auto");
    default:
      return value?.trim() || t("profile.unknown");
  }
}

export function renderNvrSummaryChip(
  icon: string,
  label: string,
  value: string,
  tone: "info" | "success" | "warning" | "critical",
  renderIcon: RenderIconFn,
): TemplateResult {
  return html`
    <div class="header-chip nvr-summary-chip">
      <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
        ${renderIcon(icon)}
      </span>
      <div class="header-chip-copy">
        <div class="header-chip-label">${label}</div>
        <div class="header-chip-value tone-${tone}">${value}</div>
      </div>
    </div>
  `;
}

export function renderVtoLockViews(
  vto: VtoViewModel,
  renderIcon: RenderIconFn,
  isBusy: IsBusyFn,
  onVtoButtonAction: OnVtoButtonAction,
  t: Localizer = createLocalizer("en"),
): TemplateResult | typeof nothing {
  if (vto.locks.length === 0) {
    return nothing;
  }

  if (vto.locks.length === 1) {
    return renderSingleVtoLockView(
      vto.locks[0]!,
      renderIcon,
      isBusy,
      onVtoButtonAction,
      t,
    );
  }

  return html`
    <div class="panel">
      <div class="panel-title">
        <span class="split-row">
          <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
            ${renderIcon("mdi:door-sliding-lock")}
          </span>
          <span>${t("vto.lockTargets")}</span>
        </span>
        <span class="badge info">${vto.locks.length}</span>
      </div>
      <div class="vto-accessory-grid">
        ${repeat(
          vto.locks,
          (lock) => lock.deviceId,
          (lock) =>
            renderVtoLockCard(lock, renderIcon, isBusy, onVtoButtonAction, t),
        )}
      </div>
    </div>
  `;
}

export function renderVtoIntercomViews(
  vto: VtoViewModel,
  renderIcon: RenderIconFn,
  t: Localizer = createLocalizer("en"),
): TemplateResult {
  const bridgeForwardErrors = vto.intercom.bridgeForwardErrors ?? 0;
  const externalUplinkTargetCount = vto.intercom.configuredExternalUplinkTargetCount ?? 0;
  return html`
    <div class="panel">
      <div class="panel-title">
        <span class="split-row">
          <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
            ${renderIcon("mdi:account-voice")}
          </span>
          <span>${t("vto.intercomBridge")}</span>
        </span>
        <span class="badge ${vto.intercom.bridgeSessionActive ? "success" : "info"}">
          ${vto.intercom.bridgeSessionActive ? t("vto.sessionActive") : t("state.idle")}
        </span>
      </div>
      <div class="chip-row">
        <span class="badge info">${t("vto.sessions", {count: vto.intercom.bridgeSessionCount ?? 0})}</span>
        <span class="badge">${vto.intercom.bridgeUplinkCodec ?? t("vto.codecUnknown")}</span>
        <span class="badge info">
          ${externalUplinkTargetCount} ${pluralUnit(externalUplinkTargetCount, "unit.exportTarget", "unit.exportTargets", t)}
        </span>
        ${bridgeForwardErrors > 0
          ? html`
              <span class="badge warning">
                ${bridgeForwardErrors} ${pluralUnit(bridgeForwardErrors, "unit.forwardError", "unit.forwardErrors", t)}
              </span>
            `
          : nothing}
        <span class="badge ${vto.capabilities.browserMicrophoneSupported ? "success" : "warning"}">
          ${renderIcon("mdi:microphone")}
          ${vto.capabilities.browserMicrophoneSupported ? t("vto.browserMicReady") : t("vto.browserMicUnavailable")}
        </span>
        <span class="badge ${vto.capabilities.externalAudioExportSupported ? "info" : "warning"}">
          ${renderIcon("mdi:export")}
          ${vto.capabilities.externalAudioExportSupported ? t("vto.externalExportSupported") : t("vto.noExternalExport")}
        </span>
        <span class="badge ${vto.intercom.externalUplinkEnabled ? "success" : "info"}">
          ${renderIcon("mdi:upload-network")}
          ${vto.intercom.externalUplinkEnabled ? t("vto.externalUplinkEnabled") : t("vto.externalUplinkDisabled")}
        </span>
        <span class="badge ${vto.intercom.bridgeUplinkActive ? "success" : "info"}">
          ${renderIcon("mdi:waveform")}
          ${vto.intercom.bridgeUplinkActive ? t("vto.bridgeUplinkActive") : t("vto.bridgeUplinkIdle")}
        </span>
      </div>
      ${vto.capabilities.validationNotes.length > 0
        ? html`
            <div class="chip-row">
              ${repeat(
                vto.capabilities.validationNotes,
                (note) => note,
                (note) => html`<span class="badge warning">${note}</span>`,
              )}
            </div>
          `
        : nothing}
    </div>
  `;
}

export function renderVtoStatusOverview(
  vto: VtoViewModel,
  t: Localizer = createLocalizer("en"),
): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">${t("vto.status")}</div>
      <div class="chip-row">
        <span class="badge ${vto.online ? "success" : "critical"}">
          ${vto.online ? t("state.online") : t("state.offline")}
        </span>
        <span class="badge ${vto.callState === "ringing" ? "warning" : vto.callState === "active" ? "success" : "info"}">
          ${vto.callStateText}
        </span>
        <span class="badge info">${vto.lockCount} ${pluralUnit(vto.lockCount, "unit.lock", "unit.locks", t)}</span>
        <span class="badge info">${vto.alarmCount} ${pluralUnit(vto.alarmCount, "unit.alarm", "unit.alarms", t)}</span>
        ${vto.doorbell ? html`<span class="badge warning">${t("badge.doorbell")}</span>` : nothing}
        ${vto.accessActive ? html`<span class="badge success">${t("badge.access")}</span>` : nothing}
        ${vto.tamper ? html`<span class="badge critical">${t("badge.tamper")}</span>` : nothing}
      </div>
      <div class="chip-row">
        ${vto.lastCallSource
          ? html`<span class="badge">${t("vto.source", {value: vto.lastCallSource})}</span>`
          : nothing}
        ${vto.lastCallStartedAt
          ? html`<span class="badge">${t("vto.started", {value: vto.lastCallStartedAt})}</span>`
          : nothing}
        <span class="badge">${t("vto.duration", {value: formatDuration(vto.lastCallDuration)})}</span>
      </div>
    </div>
  `;
}

function renderSingleVtoLockView(
  lock: VtoLockViewModel,
  renderIcon: RenderIconFn,
  isBusy: IsBusyFn,
  onVtoButtonAction: OnVtoButtonAction,
  t: Localizer,
): TemplateResult {
  const actionKey = `vto-lock:${lock.deviceId}:unlock`;
  return html`
    <div class="panel vto-lock-surface">
      <div class="panel-title">
        <span class="split-row">
          <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
            ${renderIcon("mdi:lock")}
          </span>
          <span>${t("vto.lockTarget")}</span>
        </span>
        ${renderControlButton(
          t("button.unlock"),
          "mdi:lock-open-variant",
          () =>
            void onVtoButtonAction(
              actionKey,
              lock.unlockButtonEntityId,
              lock.unlockActionUrl,
            ),
          renderIcon,
          {
            tone: "primary",
            disabled:
              isBusy(actionKey) ||
              (!lock.hasUnlockButtonEntity && !lock.unlockActionUrl),
          },
        )}
      </div>
      <div class="sidebar-label">${lock.label}</div>
      <div class="chip-row">
        <span class="badge ${lock.online ? "success" : "critical"}">
          ${lock.online ? t("state.online") : t("state.offline")}
        </span>
        <span class="badge ${lock.online ? (lock.sensorEnabled ? "success" : "warning") : "critical"}">
          ${!lock.online ? t("state.unavailable") : lock.sensorEnabled ? t("state.armed") : t("state.sensorOff")}
        </span>
        <span class="badge">${lock.stateText ?? t("state.unknown")}</span>
        ${lock.lockMode ? html`<span class="badge info">${lock.lockMode}</span>` : nothing}
      </div>
      <div class="vto-lock-meta">
        <span class="muted">${lock.roomLabel}</span>
        ${lock.modelText ? html`<span class="muted">${t("vto.model", {value: lock.modelText})}</span>` : nothing}
        ${lock.unlockHoldInterval
          ? html`<span class="muted">${t("vto.hold", {value: lock.unlockHoldInterval})}</span>`
          : nothing}
      </div>
    </div>
  `;
}

function renderVtoLockCard(
  lock: VtoLockViewModel,
  renderIcon: RenderIconFn,
  isBusy: IsBusyFn,
  onVtoButtonAction: OnVtoButtonAction,
  t: Localizer,
): TemplateResult {
  const actionKey = `vto-lock:${lock.deviceId}:unlock`;
  return html`
    <div class="compact-card vto-accessory-card">
      <div class="panel-title compact-card-head">
        <span class="split-row">
          <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
            ${renderIcon("mdi:lock")}
          </span>
          <span class="sidebar-label">${lock.label}</span>
        </span>
        <span class="badge ${lock.online ? (lock.sensorEnabled ? "success" : "warning") : "critical"}">
          ${!lock.online ? t("state.offline") : lock.sensorEnabled ? t("state.armed") : t("state.sensorOff")}
        </span>
      </div>
      <div class="chip-row">
        <span class="badge">${lock.stateText ?? t("state.unknown")}</span>
        ${lock.lockMode ? html`<span class="badge info">${lock.lockMode}</span>` : nothing}
        ${lock.unlockHoldInterval ? html`<span class="badge">${t("vto.holdCompact", {value: lock.unlockHoldInterval})}</span>` : nothing}
      </div>
      <div class="control-row">
        ${renderControlButton(
          t("button.unlock"),
          "mdi:lock-open-variant",
          () =>
            void onVtoButtonAction(
              actionKey,
              lock.unlockButtonEntityId,
              lock.unlockActionUrl,
            ),
          renderIcon,
          {
            tone: "primary",
            disabled:
              isBusy(actionKey) ||
              (!lock.hasUnlockButtonEntity && !lock.unlockActionUrl),
          },
        )}
      </div>
    </div>
  `;
}
