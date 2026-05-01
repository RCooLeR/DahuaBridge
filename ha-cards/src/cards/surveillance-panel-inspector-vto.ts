import { html, nothing, type TemplateResult } from "lit";

import type { VtoViewModel } from "../domain/model";
import type { DetailTab } from "./surveillance-panel-state";
import {
  type IsBusyFn,
  type OnVtoButtonAction,
  type OnVtoRangeChange,
  type OnVtoSwitchAction,
  type RenderIconFn,
  renderVtoIntercomViews,
  renderVtoLockViews,
  renderVtoStatusOverview,
} from "./surveillance-panel-inspector-shared";
import { renderControlButton, renderSegmentButton } from "./surveillance-panel-primitives";

export function renderVtoInspector(
  vto: VtoViewModel,
  detailTab: DetailTab,
  eventContent: TemplateResult | typeof nothing,
  renderIcon: RenderIconFn,
  isBusy: IsBusyFn,
  onSelectDetailTab: (tab: DetailTab) => void,
  onVtoRangeChange: OnVtoRangeChange,
  onVtoSwitchAction: OnVtoSwitchAction,
  onVtoButtonAction: OnVtoButtonAction,
): TemplateResult {
  const outputVolumeAvailable =
    vto.hasOutputVolumeEntity || Boolean(vto.outputVolumeActionUrl);
  const inputVolumeAvailable =
    vto.hasInputVolumeEntity || Boolean(vto.inputVolumeActionUrl);
  const autoRecordAvailable =
    vto.hasAutoRecordEntity || Boolean(vto.autoRecordActionUrl);
  const externalUplinkAvailable = Boolean(
    vto.capabilities.enableExternalUplinkUrl ||
      vto.capabilities.disableExternalUplinkUrl,
  );
  const sessionResetAvailable = Boolean(vto.capabilities.resetUrl);

  return html`
    <div class="detail-header">
      <div class="detail-title">${vto.label}</div>
      <div class="muted">${vto.roomLabel} door station</div>
    </div>
    <div class="detail-tabs">
      ${renderSegmentButton("overview", "Overview", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("events", "Events", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("settings", "Settings", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
    </div>
    <div class="detail-main">
      ${detailTab === "overview"
        ? html`
            ${renderVtoStatusOverview(vto)}
            ${renderVtoLockViews(
              vto,
              renderIcon,
              isBusy,
              onVtoButtonAction,
            )}
            ${renderVtoIntercomViews(vto, renderIcon)}
          `
        : nothing}
      ${detailTab === "events" ? eventContent : nothing}
      ${detailTab === "settings"
        ? html`
            <div class="panel">
              <div class="panel-title">Audio Controls</div>
              ${outputVolumeAvailable
                ? html`
                    <div class="slider-wrap">
                      <div class="split-row">
                        <span class="muted">Speaker Volume</span>
                        <strong>${vto.outputVolume ?? 0}</strong>
                      </div>
                      <input
                        type="range"
                        min="0"
                        max="100"
                        step="1"
                        .value=${String(vto.outputVolume ?? 0)}
                        ?disabled=${isBusy("vto:output-volume")}
                        @change=${(event: Event) =>
                          onVtoRangeChange(
                            event,
                            "vto:output-volume",
                            vto.outputVolumeEntityId,
                            vto.outputVolumeActionUrl,
                          )}
                      />
                    </div>
                  `
                : nothing}
              ${inputVolumeAvailable
                ? html`
                    <div class="slider-wrap">
                      <div class="split-row">
                        <span class="muted">Microphone Volume</span>
                        <strong>${vto.inputVolume ?? 0}</strong>
                      </div>
                      <input
                        type="range"
                        min="0"
                        max="100"
                        step="1"
                        .value=${String(vto.inputVolume ?? 0)}
                        ?disabled=${isBusy("vto:input-volume")}
                        @change=${(event: Event) =>
                          onVtoRangeChange(
                            event,
                            "vto:input-volume",
                            vto.inputVolumeEntityId,
                            vto.inputVolumeActionUrl,
                          )}
                      />
                    </div>
                  `
                : nothing}
              <div class="chip-row">
                <span class="badge ${vto.capabilities.outputVolumeSupported ? "success" : "warning"}">
                  Speaker control ${vto.capabilities.outputVolumeSupported ? "ready" : "unavailable"}
                </span>
                <span class="badge ${vto.capabilities.inputVolumeSupported ? "success" : "warning"}">
                  Microphone control ${vto.capabilities.inputVolumeSupported ? "ready" : "unavailable"}
                </span>
                <span class="badge ${vto.capabilities.muteSupported ? "info" : "warning"}">
                  ${vto.capabilities.muteSupported ? "Mute on video controls" : "Mute unavailable"}
                </span>
              </div>
              ${autoRecordAvailable
                ? html`
                    <div class="control-row">
                      ${renderControlButton(
                        vto.autoRecordEnabled
                          ? "Auto Record On"
                          : "Auto Record Off",
                        "mdi:record-rec",
                        () =>
                          void onVtoSwitchAction(
                            "vto:auto-record",
                            vto.autoRecordEntityId,
                            !vto.autoRecordEnabled,
                            vto.autoRecordActionUrl,
                            "auto_record_enabled",
                          ),
                        renderIcon,
                        {
                          tone: vto.autoRecordEnabled
                            ? "warning"
                            : "neutral",
                          disabled: isBusy("vto:auto-record"),
                        },
                      )}
                    </div>
                  `
                : nothing}
            </div>
            <div class="panel">
              <div class="panel-title">Intercom Controls</div>
              <div class="chip-row">
                <span class="badge ${vto.capabilities.resetSupported ? "success" : "warning"}">
                  ${vto.capabilities.resetSupported ? "Session reset ready" : "Session reset unavailable"}
                </span>
                <span class="badge ${vto.capabilities.bridgeAudioUplinkSupported ? "success" : "warning"}">
                  ${vto.capabilities.bridgeAudioUplinkSupported ? "Bridge uplink supported" : "Bridge uplink unavailable"}
                </span>
                <span class="badge ${vto.capabilities.bridgeAudioOutputSupported ? "info" : "warning"}">
                  ${vto.capabilities.bridgeAudioOutputSupported ? "Bridge output supported" : "Bridge output unavailable"}
                </span>
              </div>
              ${externalUplinkAvailable || sessionResetAvailable
                ? html`
                    <div class="control-row">
                      ${externalUplinkAvailable
                        ? renderControlButton(
                            vto.intercom.externalUplinkEnabled
                              ? "Disable External Uplink"
                              : "Enable External Uplink",
                            vto.intercom.externalUplinkEnabled
                              ? "mdi:upload-off-outline"
                              : "mdi:upload-network-outline",
                            () =>
                              void onVtoButtonAction(
                                "vto:external-uplink",
                                "",
                                vto.intercom.externalUplinkEnabled
                                  ? vto.capabilities.disableExternalUplinkUrl
                                  : vto.capabilities.enableExternalUplinkUrl,
                              ),
                            renderIcon,
                            {
                              tone: vto.intercom.externalUplinkEnabled
                                ? "warning"
                                : "primary",
                              disabled: isBusy("vto:external-uplink"),
                              active: vto.intercom.externalUplinkEnabled,
                            },
                          )
                        : nothing}
                      ${sessionResetAvailable
                        ? renderControlButton(
                            "Reset Bridge Session",
                            "mdi:restart",
                            () =>
                              void onVtoButtonAction(
                                "vto:session-reset",
                                "",
                                vto.capabilities.resetUrl,
                              ),
                            renderIcon,
                            {
                              tone: "warning",
                              disabled: isBusy("vto:session-reset"),
                            },
                          )
                        : nothing}
                    </div>
                  `
                : nothing}
            </div>
          `
        : nothing}
    </div>
  `;
}
