import { html, nothing, type TemplateResult } from "lit";

import type { VtoViewModel } from "../domain/model";
import type { Localizer } from "../localization";
import type { DetailTab } from "./surveillance-panel-state";
import {
  type IsBusyFn,
  type OnVtoButtonAction,
  type OnVtoSwitchAction,
  type RenderIconFn,
  renderVtoIntercomViews,
  renderVtoLockViews,
  renderVtoStatusOverview,
} from "./surveillance-panel-inspector-shared";
import { renderControlButton, renderSegmentButton } from "./surveillance-panel-primitives";

export function renderVtoInspector(
  vto: VtoViewModel,
  t: Localizer,
  detailTab: DetailTab,
  eventContent: TemplateResult | typeof nothing,
  renderIcon: RenderIconFn,
  isBusy: IsBusyFn,
  onSelectDetailTab: (tab: DetailTab) => void,
  onVtoSwitchAction: OnVtoSwitchAction,
  onVtoButtonAction: OnVtoButtonAction,
): TemplateResult {
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
      <div class="muted">${t("vto.doorStationSuffix", {room: vto.roomLabel})}</div>
    </div>
    <div class="detail-tabs">
      ${renderSegmentButton("overview", t("tab.overview"), detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("events", t("tab.events"), detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("settings", t("tab.settings"), detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
    </div>
    <div class="detail-main">
      ${detailTab === "overview"
        ? html`
            ${renderVtoStatusOverview(vto, t)}
            ${renderVtoLockViews(
              vto,
              renderIcon,
              isBusy,
              onVtoButtonAction,
              t,
            )}
            ${renderVtoIntercomViews(vto, renderIcon, t)}
          `
        : nothing}
      ${detailTab === "events" ? eventContent : nothing}
      ${detailTab === "settings"
        ? html`
            ${autoRecordAvailable
              ? html`
                  <div class="panel">
                    <div class="panel-title">${t("inspector.recordingControls")}</div>
                    <div class="control-row">
                      ${renderControlButton(
                        vto.autoRecordEnabled
                          ? t("button.autoRecordOn")
                          : t("button.autoRecordOff"),
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
                  </div>
                `
              : nothing}
            <div class="panel">
              <div class="panel-title">${t("inspector.intercomControls")}</div>
              <div class="chip-row">
                <span class="badge ${vto.capabilities.resetSupported ? "success" : "warning"}">
                  ${vto.capabilities.resetSupported ? t("inspector.sessionResetReady") : t("inspector.sessionResetUnavailable")}
                </span>
                <span class="badge ${vto.capabilities.bridgeAudioUplinkSupported ? "success" : "warning"}">
                  ${vto.capabilities.bridgeAudioUplinkSupported ? t("inspector.bridgeUplinkSupported") : t("inspector.bridgeUplinkUnavailable")}
                </span>
                <span class="badge ${vto.capabilities.bridgeAudioOutputSupported ? "info" : "warning"}">
                  ${vto.capabilities.bridgeAudioOutputSupported ? t("inspector.bridgeOutputSupported") : t("inspector.bridgeOutputUnavailable")}
                </span>
              </div>
              ${externalUplinkAvailable || sessionResetAvailable
                ? html`
                    <div class="control-row">
                      ${externalUplinkAvailable
                        ? renderControlButton(
                            vto.intercom.externalUplinkEnabled
                              ? t("button.disableExternalUplink")
                              : t("button.enableExternalUplink"),
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
                            t("button.resetBridgeSession"),
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
