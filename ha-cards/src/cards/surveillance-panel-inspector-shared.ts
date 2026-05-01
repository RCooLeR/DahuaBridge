import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import type { VtoLockViewModel, VtoViewModel } from "../domain/model";
import { formatDuration } from "../utils/format";
import { renderControlButton } from "./surveillance-panel-primitives";

export type RenderIconFn = (icon: string) => TemplateResult;
export type IsBusyFn = (key: string) => boolean;
export type OnVtoRangeChange = (
  event: Event,
  key: string,
  entityId: string,
  fallbackUrl: string | null,
) => Promise<void>;
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

export function audioAuthorityLabel(authority: string | null): string {
  switch (authority?.trim().toLowerCase()) {
    case "direct_ipc":
      return "Direct IPC";
    case "imou_override":
      return "IMOU";
    case "nvr":
      return "NVR";
    case "nvr_read_only":
      return "NVR read-only";
    default:
      return authority?.trim() || "unknown";
  }
}

export function audioAuthorityTone(
  authority: string | null,
): "success" | "warning" | "critical" | "info" {
  switch (authority?.trim().toLowerCase()) {
    case "direct_ipc":
      return "success";
    case "imou_override":
      return "info";
    case "nvr":
      return "warning";
    case "nvr_read_only":
      return "critical";
    default:
      return "info";
  }
}

export function audioSemanticLabel(semantic: string | null): string {
  switch (semantic?.trim().toLowerCase()) {
    case "stream_audio_enable":
      return "Stream audio toggle";
    default:
      return semantic?.trim() || "Audio control";
  }
}

export function streamProfileLabel(value: string | null | undefined): string {
  switch (value?.trim().toLowerCase()) {
    case "quality":
      return "Quality (Main Stream)";
    case "default":
      return "Default (Main Stream)";
    case "stable":
      return "Stable (Substream)";
    case "substream":
      return "Substream (Native)";
    default:
      return value?.trim() || "unknown";
  }
}

export function streamSourceLabel(value: string | null | undefined): string {
  switch (value?.trim().toLowerCase()) {
    case "rtsp":
      return "Direct RTSP";
    case "hls":
      return "Bridge HLS (H.264/AAC)";
    case "webrtc":
      return "Bridge WebRTC";
    case "mjpeg":
      return "Bridge MJPEG";
    case "auto":
      return "Auto (Bridge Recommended)";
    default:
      return value?.trim() || "unknown";
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
    );
  }

  return html`
    <div class="panel">
      <div class="panel-title">
        <span class="split-row">
          <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
            ${renderIcon("mdi:door-sliding-lock")}
          </span>
          <span>Lock Targets</span>
        </span>
        <span class="badge info">${vto.locks.length}</span>
      </div>
      <div class="vto-accessory-grid">
        ${repeat(
          vto.locks,
          (lock) => lock.deviceId,
          (lock) =>
            renderVtoLockCard(lock, renderIcon, isBusy, onVtoButtonAction),
        )}
      </div>
    </div>
  `;
}

export function renderVtoIntercomViews(
  vto: VtoViewModel,
  renderIcon: RenderIconFn,
): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">
        <span class="split-row">
          <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
            ${renderIcon("mdi:account-voice")}
          </span>
          <span>Intercom Bridge</span>
        </span>
        <span class="badge ${vto.intercom.bridgeSessionActive ? "success" : "info"}">
          ${vto.intercom.bridgeSessionActive ? "Session Active" : "Idle"}
        </span>
      </div>
      <div class="chip-row">
        <span class="badge info">Sessions ${vto.intercom.bridgeSessionCount ?? 0}</span>
        <span class="badge">${vto.intercom.bridgeUplinkCodec ?? "Codec unknown"}</span>
        <span class="badge info">
          ${vto.intercom.configuredExternalUplinkTargetCount ?? 0} export target${(vto.intercom.configuredExternalUplinkTargetCount ?? 0) === 1 ? "" : "s"}
        </span>
        ${(vto.intercom.bridgeForwardErrors ?? 0) > 0
          ? html`
              <span class="badge warning">
                ${vto.intercom.bridgeForwardErrors} forward error${vto.intercom.bridgeForwardErrors === 1 ? "" : "s"}
              </span>
            `
          : nothing}
        <span class="badge ${vto.capabilities.browserMicrophoneSupported ? "success" : "warning"}">
          ${renderIcon("mdi:microphone")}
          ${vto.capabilities.browserMicrophoneSupported ? "Browser mic ready" : "Browser mic unavailable"}
        </span>
        <span class="badge ${vto.capabilities.externalAudioExportSupported ? "info" : "warning"}">
          ${renderIcon("mdi:export")}
          ${vto.capabilities.externalAudioExportSupported ? "External export supported" : "No external export"}
        </span>
        <span class="badge ${vto.intercom.externalUplinkEnabled ? "success" : "info"}">
          ${renderIcon("mdi:upload-network")}
          ${vto.intercom.externalUplinkEnabled ? "External uplink enabled" : "External uplink disabled"}
        </span>
        <span class="badge ${vto.intercom.bridgeUplinkActive ? "success" : "info"}">
          ${renderIcon("mdi:waveform")}
          ${vto.intercom.bridgeUplinkActive ? "Bridge uplink active" : "Bridge uplink idle"}
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

export function renderVtoStatusOverview(vto: VtoViewModel): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">Door Station Status</div>
      <div class="chip-row">
        <span class="badge ${vto.online ? "success" : "critical"}">
          ${vto.online ? "Online" : "Offline"}
        </span>
        <span class="badge ${vto.callState === "ringing" ? "warning" : vto.callState === "active" ? "success" : "info"}">
          ${vto.callStateText}
        </span>
        <span class="badge info">${vto.lockCount} lock${vto.lockCount === 1 ? "" : "s"}</span>
        <span class="badge info">${vto.alarmCount} alarm${vto.alarmCount === 1 ? "" : "s"}</span>
        ${vto.doorbell ? html`<span class="badge warning">Doorbell</span>` : nothing}
        ${vto.accessActive ? html`<span class="badge success">Access</span>` : nothing}
        ${vto.tamper ? html`<span class="badge critical">Tamper</span>` : nothing}
      </div>
      <div class="chip-row">
        ${vto.lastCallSource
          ? html`<span class="badge">Source: ${vto.lastCallSource}</span>`
          : nothing}
        ${vto.lastCallStartedAt
          ? html`<span class="badge">Started: ${vto.lastCallStartedAt}</span>`
          : nothing}
        <span class="badge">Duration: ${formatDuration(vto.lastCallDuration)}</span>
      </div>
    </div>
  `;
}

function renderSingleVtoLockView(
  lock: VtoLockViewModel,
  renderIcon: RenderIconFn,
  isBusy: IsBusyFn,
  onVtoButtonAction: OnVtoButtonAction,
): TemplateResult {
  const actionKey = `vto-lock:${lock.deviceId}:unlock`;
  return html`
    <div class="panel vto-lock-surface">
      <div class="panel-title">
        <span class="split-row">
          <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
            ${renderIcon("mdi:lock")}
          </span>
          <span>Lock Target</span>
        </span>
        ${renderControlButton(
          "Unlock",
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
          ${lock.online ? "Online" : "Offline"}
        </span>
        <span class="badge ${lock.online ? (lock.sensorEnabled ? "success" : "warning") : "critical"}">
          ${!lock.online ? "Unavailable" : lock.sensorEnabled ? "Armed" : "Sensor Off"}
        </span>
        <span class="badge">${lock.stateText ?? "Unknown"}</span>
        ${lock.lockMode ? html`<span class="badge info">${lock.lockMode}</span>` : nothing}
      </div>
      <div class="vto-lock-meta">
        <span class="muted">${lock.roomLabel}</span>
        ${lock.modelText ? html`<span class="muted">Model: ${lock.modelText}</span>` : nothing}
        ${lock.unlockHoldInterval
          ? html`<span class="muted">Hold: ${lock.unlockHoldInterval}</span>`
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
          ${!lock.online ? "Offline" : lock.sensorEnabled ? "Armed" : "Sensor Off"}
        </span>
      </div>
      <div class="chip-row">
        <span class="badge">${lock.stateText ?? "Unknown"}</span>
        ${lock.lockMode ? html`<span class="badge info">${lock.lockMode}</span>` : nothing}
        ${lock.unlockHoldInterval ? html`<span class="badge">Hold ${lock.unlockHoldInterval}</span>` : nothing}
      </div>
      <div class="control-row">
        ${renderControlButton(
          "Unlock",
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
