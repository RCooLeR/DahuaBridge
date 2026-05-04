import { css, html, LitElement, nothing, type TemplateResult } from "lit";
import { z } from "zod";

import { SurveillancePanelActions } from "./surveillance-panel-actions";
import {
  cameraImageSrc,
  defaultOverviewStreamProfileKey,
  renderSelectedCameraViewport,
  renderSelectedVtoViewport,
  resolveConfiguredOverviewStreamViewportSource,
  resolveOverviewCameraViewportSource,
  syncViewportAudioState,
} from "./surveillance-panel-media";
import { renderIconButton } from "./surveillance-panel-primitives";
import {
  buildPanelModel,
  displayCameraLabel,
  findAuxTarget,
  supportsAuxTarget,
  type CameraViewModel,
  type VtoViewModel,
} from "../domain/model";
import type { SurveillancePanelCardConfig } from "../types/card-config";
import type {
  HomeAssistant,
  LovelaceCard,
  LovelaceCardConfig,
} from "../types/home-assistant";
import { postBridgeRequest } from "../ha/actions";
import {
  buildNvrEventSummaryUrl,
  fetchNvrEventSummary,
} from "../ha/bridge-event-summary";
import {
  BridgeIntercomSessionController,
  type BridgeIntercomSnapshot,
  resolveIntercomOfferUrl,
} from "../ha/bridge-intercom";
import {
  localizeIntercomError,
  localizeIntercomStatus,
} from "./surveillance-panel-intercom-status";
import {
  summarizePanelTodayEvents,
  type NvrEventSummaryModel,
  type PanelTodayEventSummaryModel,
} from "../domain/event-summary";
import {
  syncRemoteStreamStyles,
} from "./surveillance-panel-media";
import { surveillancePanelBaseStyles, surveillancePanelOverviewStyles } from "./surveillance-panel-styles";
import { renderCameraEventCountBadges } from "./surveillance-panel-event-badges";
import { openExternalUrl } from "../utils/browser";
import { logCardInfo, redactUrlForLog } from "../utils/logging";
import { createLocalizer, resolvePanelLanguage, type Localizer } from "../localization";

const vtoSchema = z
  .object({
    device_id: z.string().min(1).optional(),
    label: z.string().min(1).optional(),
    lock_button_entity: z.string().min(1).optional(),
    auto_record_entity: z.string().min(1).optional(),
  })
  .optional();

const configSchema = z.object({
  type: z.literal("custom:dahuabridge-surveillance-tile"),
  device_id: z.string().min(1),
  title: z.string().min(1).optional(),
  browser_bridge_url: z.string().min(1).optional(),
  vto: vtoSchema,
});

type CompactCardConfig = z.infer<typeof configSchema> & LovelaceCardConfig;

const DISCOVERY_CONFIG_BASE: SurveillancePanelCardConfig = {
  type: "custom:dahuabridge-surveillance-panel",
  event_lookback_hours: 12,
  bridge_event_poll_seconds: 15,
  max_events: 14,
};

const INITIAL_VTO_MICROPHONE_STATE: BridgeIntercomSnapshot = {
  enabled: false,
  phase: "idle",
  statusText: "Mic inactive",
  error: "",
};

export class DahuaBridgeSurveillanceTileCard
  extends LitElement
  implements LovelaceCard
{
  static properties = {
    hass: { attribute: false },
    _config: { state: true },
    _busyActions: { state: true },
    _errorMessage: { state: true },
    _cameraAudioMuted: { state: true },
    _vtoStreamPlaying: { state: true },
    _vtoMicrophoneState: { state: true },
    _eventSummary: { state: true },
  } as const;

  static styles = [
    surveillancePanelBaseStyles,
    surveillancePanelOverviewStyles,
    css`
      :host {
        height: auto;
      }

      ha-card {
        aspect-ratio: auto;
        min-height: 0;
        max-height: none;
        height: auto;
        background: rgba(6, 16, 26, 0.98);
      }

      .tile-shell {
        display: grid;
        gap: 0;
        padding: 0;
      }

      .tile-card {
        position: relative;
        overflow: hidden;
        border-radius: 8px;
        border: 1px solid var(--db-border);
        background: rgba(5, 13, 21, 0.92);
      }

      .tile-card .tile-media {
        aspect-ratio: 16 / 9;
        border-bottom: 0;
      }

      .tile-topbar {
        position: absolute;
        top: 10px;
        left: 10px;
        right: 10px;
        z-index: 2;
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: 10px;
        pointer-events: none;
      }

      .tile-topbar-copy {
        display: flex;
        align-items: flex-start;
        gap: 8px;
        min-width: 0;
        max-width: 100%;
      }

      .tile-title-banner {
        min-width: 0;
        max-width: 100%;
        flex: 1 1 auto;
        padding: 7px 10px;
        border-radius: 8px;
        border: 1px solid rgba(255, 255, 255, 0.14);
        background: rgba(7, 18, 29, 0.56);
        backdrop-filter: blur(14px);
        -webkit-backdrop-filter: blur(14px);
      }

      .tile-title-banner .tile-name {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }

      .tile-indicators {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 8px 10px;
        border-radius: 999px;
        border: 1px solid rgba(255, 255, 255, 0.14);
        background: rgba(7, 18, 29, 0.56);
        backdrop-filter: blur(14px);
        -webkit-backdrop-filter: blur(14px);
      }

      .tile-indicators .recording-dot {
        width: 8px;
        height: 8px;
      }

      .tile-controls {
        right: 10px;
        bottom: 10px;
        gap: 6px;
      }

      .tile-overlay-badges {
        padding-right: 212px;
      }

      .tile-copy {
        display: grid;
        gap: 8px;
      }

      .tile-copy .badge {
        max-width: 100%;
      }

      @media (max-width: 480px) {
        .tile-topbar-copy {
          flex-wrap: wrap;
        }

        .tile-overlay-badges {
          padding-right: 0;
          padding-bottom: 54px;
        }

        .tile-controls {
          left: 10px;
          right: 10px;
          justify-content: flex-end;
          flex-wrap: wrap;
        }
      }
    `,
  ];

  hass?: HomeAssistant;

  private _config?: CompactCardConfig;
  private _busyActions = new Set<string>();
  private _errorMessage = "";
  private _cameraAudioMuted = true;
  private _vtoStreamPlaying = false;
  private _vtoMicrophoneState = INITIAL_VTO_MICROPHONE_STATE;
  private _eventSummary: PanelTodayEventSummaryModel | null = null;
  private _remoteStreamSyncTimer: number | null = null;
  private _viewportAudioSyncTimer: number | null = null;
  private _eventSummaryAbort?: AbortController;
  private _eventSummaryRequestVersion = 0;
  private _eventSummaryRefreshedAt = 0;
  private _eventSummaryCameraKey = "";
  private _eventSummaryRefreshTimer: number | null = null;

  private readonly _actions = new SurveillancePanelActions({
    getHass: () => this.hass,
    getBusyActions: () => this._busyActions,
    setBusyActions: (next) => {
      this._busyActions = next;
    },
    setError: (message) => {
      this._errorMessage = message;
    },
  });
  private readonly _intercomSession = new BridgeIntercomSessionController({
    onChange: (snapshot) => {
      const previousState = this._vtoMicrophoneState;
      this._vtoMicrophoneState = snapshot;
      if (snapshot.error) {
        this._errorMessage = localizeIntercomError(snapshot.error, this.t());
      } else if (this._errorMessage === previousState.error) {
        this._errorMessage = "";
      }
      this.requestUpdate("_vtoMicrophoneState", previousState);
    },
  });

  setConfig(config: LovelaceCardConfig): void {
    this._config = configSchema.parse(config);
    this._busyActions = new Set();
    this._errorMessage = "";
    this._cameraAudioMuted = true;
    this._vtoStreamPlaying = false;
    this._vtoMicrophoneState = INITIAL_VTO_MICROPHONE_STATE;
    this.setEventSummary(null);
    this._eventSummaryRefreshedAt = 0;
    this._eventSummaryCameraKey = "";
    this.cancelEventSummaryRefresh();
    this.scheduleEventSummaryRefresh(0);
    void this.stopVtoMicrophone();
  }

  connectedCallback(): void {
    super.connectedCallback();
    if (this._config) {
      this.scheduleEventSummaryRefresh(0);
    }
  }

  disconnectedCallback(): void {
    if (this._remoteStreamSyncTimer !== null) {
      window.clearTimeout(this._remoteStreamSyncTimer);
      this._remoteStreamSyncTimer = null;
    }
    if (this._viewportAudioSyncTimer !== null) {
      window.clearTimeout(this._viewportAudioSyncTimer);
      this._viewportAudioSyncTimer = null;
    }

    this.cancelEventSummaryRefresh();
    void this.stopVtoMicrophone();
    super.disconnectedCallback();
  }

  private scheduleRemoteStreamStyleSync(): void {
    if (this._remoteStreamSyncTimer !== null) {
      window.clearTimeout(this._remoteStreamSyncTimer);
    }

    const syncDelays = [0, 50, 150, 400, 1000, 2500, 5000, 10000, 20000, 45000, 90000];

    const runSyncAt = (index: number): void => {
      syncRemoteStreamStyles(this.renderRoot);

      if (index >= syncDelays.length - 1) {
        this._remoteStreamSyncTimer = null;
        return;
      }

      this._remoteStreamSyncTimer = window.setTimeout(
        () => runSyncAt(index + 1),
        syncDelays[index + 1]!,
      );
    };

    this._remoteStreamSyncTimer = window.setTimeout(() => runSyncAt(0), 0);
  }

  private scheduleCameraViewportAudioSync(): void {
    if (this._viewportAudioSyncTimer !== null) {
      window.clearTimeout(this._viewportAudioSyncTimer);
    }

    const syncDelays = [0, 50, 150, 400, 1000, 2500, 5000, 10000, 20000, 45000, 90000];

    const runSyncAt = (index: number): void => {
      this.syncCameraViewportAudioState(this._cameraAudioMuted);

      if (index >= syncDelays.length - 1) {
        this._viewportAudioSyncTimer = null;
        return;
      }

      this._viewportAudioSyncTimer = window.setTimeout(
        () => runSyncAt(index + 1),
        syncDelays[index + 1]!,
      );
    };

    this._viewportAudioSyncTimer = window.setTimeout(() => runSyncAt(0), 0);
  }

  private shouldSyncMediaAfterUpdate(
    changedProperties: Map<PropertyKey, unknown>,
  ): boolean {
    if (changedProperties.has("hass") && changedProperties.get("hass") === undefined) {
      return true;
    }
    return [
      "_config",
      "_cameraAudioMuted",
      "_vtoStreamPlaying",
    ].some((key) => changedProperties.has(key));
  }

  protected updated(changedProperties: Map<PropertyKey, unknown>): void {
    if (this.shouldRefreshEventSummary()) {
      void this.refreshEventSummary();
    }
    if (this.shouldSyncMediaAfterUpdate(changedProperties)) {
      this.scheduleRemoteStreamStyleSync();
      this.scheduleCameraViewportAudioSync();
    }

    if (
      this._vtoStreamPlaying &&
      (changedProperties.has("_vtoStreamPlaying") || changedProperties.has("hass"))
    ) {
      window.requestAnimationFrame(() => {
        const video = this.renderRoot.querySelector<HTMLVideoElement>("video.vto-live-stream");
        if (!video) {
          return;
        }
        void video.play().catch(() => undefined);
      });
    }
  }

  getCardSize(): number {
    return 6;
  }

  render(): TemplateResult {
    const t = this.t();
    if (!this._config) {
      return html`<ha-card><div class="tile-shell"><div class="muted">${t("panel.configMissing")}</div></div></ha-card>`;
    }
    if (!this.hass) {
      return html`<ha-card><div class="tile-shell"><div class="muted">${t("panel.hassMissing")}</div></div></ha-card>`;
    }

    const model = buildPanelModel(
      this.hass,
      {
        ...DISCOVERY_CONFIG_BASE,
        browser_bridge_url: this._config.browser_bridge_url,
        vto: this._config.vto,
      },
      { kind: "overview" },
      undefined,
      undefined,
      undefined,
      this._eventSummary,
    );
    const camera = model.cameras.find((item) => item.deviceId === this._config?.device_id) ?? null;
    const vto = model.vtos.find((item) => item.deviceId === this._config?.device_id) ?? null;

    if (camera) {
      return this.renderCameraTile(camera, createLocalizer(model.language));
    }
    if (vto) {
      return this.renderVtoTile(vto, createLocalizer(model.language));
    }

    return html`
      <ha-card>
        <div class="tile-shell">
          <div class="muted">
            ${t("panel.deviceNotFound", { deviceId: this._config.device_id })}
          </div>
        </div>
      </ha-card>
    `;
  }

  private renderCameraTile(camera: CameraViewModel, t: Localizer): TemplateResult {
    const selectedProfileKey = defaultOverviewStreamProfileKey(camera.stream);
    const selectedSource = resolveOverviewCameraViewportSource(camera, selectedProfileKey);
    const lightAvailable = supportsAuxTarget(camera, "light");
    const warningLightAvailable = supportsAuxTarget(camera, "warning_light");
    const sirenAvailable = supportsAuxTarget(camera, "siren");
    const lightActive = findAuxTarget(camera, "light")?.active === true;
    const warningLightActive = findAuxTarget(camera, "warning_light")?.active === true;
    const sirenActive = findAuxTarget(camera, "siren")?.active === true;
    const showRecording = camera.recordingActive || camera.bridgeRecordingActive;
    const title = this._config?.title ?? displayCameraLabel(camera);

    return html`
      <ha-card>
        <div class="tile-shell">
          <article class="tile-card">
            <div class="tile-media">
              ${renderSelectedCameraViewport(
                this.hass,
                camera,
                selectedProfileKey,
                selectedSource,
                this._cameraAudioMuted,
                1,
                {
                  controls: false,
                  preload: "none",
                  fallbackOrder: ["hls", "dash"],
                  includeSubstreamFallback: false,
                  manageAudioExternally: true,
                  t,
                },
              )}
              <div class="tile-topbar">
                <div class="tile-topbar-copy">
                  <div class="tile-title-banner">
                    <div class="tile-name">${title}</div>
                  </div>
                </div>
              </div>
              <div class="media-overlay">
                <div class="media-bottom">
                  <div class="tile-overlay-badges">
                    ${renderCameraEventCountBadges(camera, "inline", t)}
                    ${camera.bridgeRecordingActive
                      ? html`<span class="badge warning">MP4</span>`
                      : nothing}
                    ${!camera.streamAvailable
                      ? html`<span class="badge critical">${t("badge.streamDown")}</span>`
                      : nothing}
                    ${camera.detections.map(
                      (badge) => html`<span class="badge ${badge.tone}">${badge.label}</span>`,
                    )}
                  </div>
                  <div class="tile-controls">
                    ${this.hasSnapshot(camera)
                      ? renderIconButton(
                          t("button.snapshot"),
                          "mdi:camera",
                          () => this.openWindow(this.resolveSnapshotUrl(camera)),
                          this.renderIcon,
                        )
                      : nothing}
                    ${camera.supportsRecording
                      ? renderIconButton(
                          camera.bridgeRecordingActive ? t("button.stopMp4") : t("button.startMp4"),
                          camera.bridgeRecordingActive ? "mdi:record-rec" : "mdi:record-circle-outline",
                          () =>
                            void this._actions.triggerRecordingAction(
                              camera,
                              camera.bridgeRecordingActive ? "stop" : "start",
                            ),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy(
                              `${camera.deviceId}:recording:${camera.bridgeRecordingActive ? "stop" : "start"}`,
                            ),
                            tone: camera.bridgeRecordingActive ? "danger" : "warning",
                            active: camera.bridgeRecordingActive,
                          },
                        )
                      : nothing}
                    ${lightAvailable
                      ? renderIconButton(
                          lightActive ? t("button.lightSmart") : t("button.lightWhite"),
                          "mdi:lightbulb-on-outline",
                          () => void this._actions.triggerAuxAction(camera, "light", lightActive),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy(`${camera.deviceId}:aux:light`),
                            tone: lightActive ? "primary" : undefined,
                            active: lightActive,
                          },
                        )
                      : nothing}
                    ${warningLightAvailable
                      ? renderIconButton(
                          warningLightActive ? t("button.warningLightOff") : t("button.warningLightOn"),
                          "mdi:alarm-light-outline",
                          () => void this._actions.triggerAuxAction(camera, "warning_light", warningLightActive),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy(`${camera.deviceId}:aux:warning_light`),
                            tone: "warning",
                            active: warningLightActive,
                          },
                        )
                      : nothing}
                    ${sirenAvailable
                      ? renderIconButton(
                          sirenActive ? t("button.sirenOff") : t("button.sirenOn"),
                          "mdi:bullhorn",
                          () => void this._actions.triggerAuxAction(camera, "siren", sirenActive),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy(`${camera.deviceId}:aux:siren`),
                            tone: "warning",
                            active: sirenActive,
                          },
                        )
                      : nothing}
                    ${camera.audioCodec.trim()
                      ? renderIconButton(
                          this._cameraAudioMuted
                            ? t("button.enableStreamAudio")
                            : t("button.disableStreamAudio"),
                          this._cameraAudioMuted ? "mdi:volume-off" : "mdi:volume-high",
                          () => void this.toggleCameraAudio(camera),
                          this.renderIcon,
                          {
                            active: !this._cameraAudioMuted,
                          },
                        )
                      : nothing}
                  </div>
                </div>
              </div>
            </div>
          </article>
          ${this._errorMessage
            ? html`<div class="error-banner">${this._errorMessage}</div>`
            : nothing}
        </div>
      </ha-card>
    `;
  }

  private renderVtoTile(vto: VtoViewModel, t: Localizer): TemplateResult {
    const selectedProfileKey = defaultOverviewStreamProfileKey(vto.stream);
    const selectedSource = resolveConfiguredOverviewStreamViewportSource(
      vto.stream,
      null,
      selectedProfileKey,
      Boolean(vto.cameraEntity),
      vto.stream.fallbacksEnabled,
    );
    const title = this._config?.title ?? vto.label;
    const showCallActions = vto.callState === "ringing" || vto.callState === "active";

    return html`
      <ha-card>
        <div class="tile-shell">
          <article class="tile-card">
            <div class="tile-media">
              ${renderSelectedVtoViewport(
                this.hass,
                vto,
                this._vtoStreamPlaying,
                selectedProfileKey,
                selectedSource,
                t,
                vto.stream.fallbacksEnabled,
              )}
              <div class="tile-topbar">
                <div class="tile-title-banner">
                  <div class="tile-name">${title}</div>
                </div>
              </div>
              <div class="media-overlay">
                <div class="media-bottom">
                  <div class="tile-overlay-badges">
                    <span class="badge ${this.vtoBadgeTone(vto)}">${vto.callStateText}</span>
                    ${vto.doorbell ? html`<span class="badge warning">${t("badge.doorbell")}</span>` : nothing}
                    ${vto.tamper ? html`<span class="badge critical">${t("badge.tamper")}</span>` : nothing}
                    ${this._vtoMicrophoneState.enabled
                      ? html`<span class="badge info">${localizeIntercomStatus(this._vtoMicrophoneState, t)}</span>`
                      : nothing}
                  </div>
                  <div class="tile-controls">
                    ${this.hasPlayableVtoStream(vto)
                      ? renderIconButton(
                          this._vtoStreamPlaying ? t("button.stopStream") : t("button.playStream"),
                          this._vtoStreamPlaying ? "mdi:stop-circle-outline" : "mdi:play-circle-outline",
                          () => {
                            const previousPlaying = this._vtoStreamPlaying;
                            this._vtoStreamPlaying = !this._vtoStreamPlaying;
                            if (!this._vtoStreamPlaying && this._vtoMicrophoneState.enabled) {
                              void this.stopVtoMicrophone();
                            }
                            this.requestUpdate("_vtoStreamPlaying", previousPlaying);
                          },
                          this.renderIcon,
                          {
                            tone: this._vtoStreamPlaying ? "warning" : "primary",
                            active: this._vtoStreamPlaying,
                          },
                        )
                      : nothing}
                    ${this.hasVtoSnapshot(vto)
                      ? renderIconButton(
                          t("button.snapshot"),
                          "mdi:camera",
                          () => this.openWindow(this.resolveVtoSnapshotUrl(vto)),
                          this.renderIcon,
                        )
                      : nothing}
                    ${vto.recordingStartUrl || vto.recordingStopUrl
                      ? renderIconButton(
                          vto.bridgeRecordingActive ? t("button.stopMp4") : t("button.startMp4"),
                          vto.bridgeRecordingActive ? "mdi:record-rec" : "mdi:record-circle-outline",
                          () => void this.triggerVtoBridgeRecording(vto),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy("vto:bridge_recording"),
                            tone: vto.bridgeRecordingActive ? "danger" : "warning",
                            active: vto.bridgeRecordingActive,
                          },
                        )
                      : nothing}
                    ${showCallActions && (vto.hasUnlockButtonEntity || Boolean(vto.unlockActionUrl))
                      ? renderIconButton(
                          t("button.unlock"),
                          "mdi:lock-open-variant",
                          () =>
                            void this._actions.triggerVtoButtonAction(
                              "vto:unlock",
                              vto.unlockButtonEntityId,
                              vto.unlockActionUrl,
                            ),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy("vto:unlock"),
                            tone: "primary",
                          },
                        )
                      : nothing}
                    ${vto.callState === "ringing" &&
                    (vto.hasAnswerButtonEntity || Boolean(vto.answerActionUrl))
                      ? renderIconButton(
                          t("button.answerCall"),
                          "mdi:phone",
                          () =>
                            void this._actions.triggerVtoButtonAction(
                              "vto:answer",
                              vto.answerButtonEntityId,
                              vto.answerActionUrl,
                            ),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy("vto:answer"),
                            tone: "warning",
                          },
                        )
                      : nothing}
                    ${showCallActions &&
                    (vto.hasHangupButtonEntity || Boolean(vto.hangupActionUrl))
                      ? renderIconButton(
                          t("button.hangUp"),
                          "mdi:phone-hangup",
                          () =>
                            void this._actions.triggerVtoButtonAction(
                              "vto:hangup",
                              vto.hangupButtonEntityId,
                              vto.hangupActionUrl,
                            ),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy("vto:hangup"),
                            tone: "danger",
                            active: vto.callState === "active",
                          },
                        )
                      : nothing}
                    ${vto.capabilities.browserMicrophoneSupported && this.hasAvailableVtoIntercom(vto)
                      ? renderIconButton(
                          this._vtoMicrophoneState.enabled ? t("button.disableMic") : t("button.enableMic"),
                          this._vtoMicrophoneState.enabled ? "mdi:microphone-off" : "mdi:microphone",
                          () =>
                            void (this._vtoMicrophoneState.enabled
                              ? this.stopVtoMicrophone()
                              : this.startVtoMicrophone(vto)),
                          this.renderIcon,
                          {
                            tone: this._vtoMicrophoneState.enabled ? "warning" : undefined,
                            active: this._vtoMicrophoneState.enabled,
                          },
                        )
                      : nothing}
                  </div>
                </div>
              </div>
            </div>
          </article>
          ${this._errorMessage
            ? html`<div class="error-banner">${this._errorMessage}</div>`
            : nothing}
        </div>
      </ha-card>
    `;
  }

  private async toggleCameraAudio(camera: CameraViewModel): Promise<void> {
    const nextMuted = !this._cameraAudioMuted;
    const previousMuted = this._cameraAudioMuted;
    this._cameraAudioMuted = nextMuted;
    this.requestUpdate("_cameraAudioMuted", previousMuted);
    this.syncCameraViewportAudioState(nextMuted);
    this.logMedia("card tile camera audio toggled", {
      device_id: camera.deviceId,
      muted: nextMuted,
    });
  }

  private async triggerVtoBridgeRecording(vto: VtoViewModel): Promise<void> {
    const targetUrl = vto.bridgeRecordingActive ? vto.recordingStopUrl : vto.recordingStartUrl;
    if (!targetUrl) {
      this._errorMessage = this.t()("error.bridgeMp4VtoUnavailable");
      return;
    }
    if (this._actions.isBusy("vto:bridge_recording")) {
      return;
    }
    const nextBusy = new Set(this._busyActions);
    nextBusy.add("vto:bridge_recording");
    this._busyActions = nextBusy;
    this._errorMessage = "";
    try {
      this.logMedia("card tile vto bridge recording request", {
        device_id: vto.deviceId,
        active: vto.bridgeRecordingActive,
        url: redactUrlForLog(targetUrl),
      });
      await postBridgeRequest(targetUrl);
      this.logMedia("card tile vto bridge recording completed", {
        device_id: vto.deviceId,
        active: vto.bridgeRecordingActive,
      });
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : this.t()("error.vtoBridgeRecordingFailed");
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete("vto:bridge_recording");
      this._busyActions = reducedBusy;
    }
  }

  private async startVtoMicrophone(vto: VtoViewModel): Promise<void> {
    const offerUrl = resolveIntercomOfferUrl(vto.stream);
    if (!offerUrl) {
      this._errorMessage = this.t()("error.intercomOfferUnavailable");
      return;
    }
    this.logMedia("card tile vto microphone enable", {
      device_id: vto.deviceId,
      offer_url: redactUrlForLog(offerUrl),
    });
    await this._intercomSession.enable(offerUrl);
  }

  private async stopVtoMicrophone(): Promise<void> {
    this.logMedia("card tile vto microphone disable");
    await this._intercomSession.disable();
  }

  private hasPlayableVtoStream(vto: VtoViewModel): boolean {
    if (!vto.streamAvailable) {
      return false;
    }
    return (
      resolveConfiguredOverviewStreamViewportSource(
        vto.stream,
        null,
        defaultOverviewStreamProfileKey(vto.stream),
        Boolean(vto.cameraEntity),
        vto.stream.fallbacksEnabled,
      ) !== null
    );
  }

  private syncCameraViewportAudioState(muted: boolean): void {
    syncViewportAudioState(
      this.renderRoot.querySelector(".tile-media"),
      muted,
      1,
    );
  }

  private hasAvailableVtoIntercom(vto: VtoViewModel): boolean {
    return resolveIntercomOfferUrl(vto.stream) !== null;
  }

  private hasSnapshot(camera: CameraViewModel): boolean {
    return this.resolveSnapshotUrl(camera).length > 0;
  }

  private hasVtoSnapshot(vto: VtoViewModel): boolean {
    return this.resolveVtoSnapshotUrl(vto).length > 0;
  }

  private resolveSnapshotUrl(camera: CameraViewModel): string {
    if (typeof camera.captureSnapshotUrl === "string" && camera.captureSnapshotUrl.trim()) {
      return camera.captureSnapshotUrl;
    }
    if (typeof camera.snapshotUrl === "string" && camera.snapshotUrl.trim()) {
      return camera.snapshotUrl;
    }
    return cameraImageSrc(camera.cameraEntity, camera.snapshotUrl);
  }

  private resolveVtoSnapshotUrl(vto: VtoViewModel): string {
    if (typeof vto.captureSnapshotUrl === "string" && vto.captureSnapshotUrl.trim()) {
      return vto.captureSnapshotUrl;
    }
    if (typeof vto.snapshotUrl === "string" && vto.snapshotUrl.trim()) {
      return vto.snapshotUrl;
    }
    return cameraImageSrc(vto.cameraEntity, vto.snapshotUrl);
  }

  private openWindow(targetUrl: string): void {
    if (!targetUrl.trim()) {
      return;
    }
    this.logMedia("card tile external open", {
      url: redactUrlForLog(targetUrl),
    });
    openExternalUrl(targetUrl);
  }

  private logMedia(message: string, details?: Record<string, unknown>): void {
    logCardInfo(message, {
      card: "surveillance-tile",
      ...details,
    });
  }

  private vtoBadgeTone(vto: VtoViewModel): "success" | "warning" | "critical" | "info" {
    if (vto.callState === "active") {
      return "info";
    }
    if (vto.callState === "ringing") {
      return "warning";
    }
    return vto.online ? "success" : "critical";
  }

  private t(): Localizer {
    return createLocalizer(this.hass ? resolvePanelLanguage(this.hass) : "en");
  }

  private renderIcon(icon: string): TemplateResult {
    return html`<ha-icon .icon=${icon}></ha-icon>`;
  }

  private shouldRefreshEventSummary(): boolean {
    if (!this.hass || !this._config) {
      return false;
    }
    if (this._eventSummaryAbort) {
      return false;
    }

    const camera = this.resolveSummaryCamera();
    if (!camera) {
      return this._eventSummary !== null || this._eventSummaryCameraKey !== "";
    }
    const cameraKey = `${camera.rootDeviceId}:${camera.channelNumber}`;
    if (cameraKey !== this._eventSummaryCameraKey) {
      return true;
    }
    if (!this._eventSummary) {
      return Date.now() - this._eventSummaryRefreshedAt >= 5_000;
    }
    return Date.now() - this._eventSummaryRefreshedAt >= 60_000;
  }

  private async refreshEventSummary(): Promise<void> {
    if (!this.hass || !this._config) {
      this.scheduleEventSummaryRefresh(1_000);
      return;
    }

    const camera = this.resolveSummaryCamera();
    if (!camera) {
      this.cancelEventSummaryRefresh();
      this.logMedia("card tile event summary skipped", {
        device_id: this._config.device_id,
        reason: "nvr_channel_not_found",
      });
      this.setEventSummary(null);
      this._eventSummaryCameraKey = "";
      this._eventSummaryRefreshedAt = Date.now();
      return;
    }

    const summaryUrl = buildNvrEventSummaryUrl(camera.bridgeBaseUrl, camera.rootDeviceId);
    if (!summaryUrl) {
      this.cancelEventSummaryRefresh();
      this.logMedia("card tile event summary skipped", {
        device_id: camera.deviceId,
        root_device_id: camera.rootDeviceId,
        channel: camera.channelNumber,
        reason: "summary_url_unavailable",
        bridge_base_url: camera.bridgeBaseUrl,
      });
      this.setEventSummary(null);
      this._eventSummaryCameraKey = `${camera.rootDeviceId}:${camera.channelNumber}`;
      this._eventSummaryRefreshedAt = Date.now();
      this.scheduleEventSummaryRefresh(15_000);
      return;
    }

    this.cancelEventSummaryRefresh();
    const controller = new AbortController();
    this._eventSummaryAbort = controller;
    const requestVersion = ++this._eventSummaryRequestVersion;
    const endTime = new Date();
    const startTime = new Date(endTime.getTime() - (24 * 60 * 60 * 1000));

    try {
      this.logMedia("card tile event summary request", {
        device_id: camera.deviceId,
        root_device_id: camera.rootDeviceId,
        channel: camera.channelNumber,
        url: redactUrlForLog(summaryUrl),
        start_time: startTime.toISOString(),
        end_time: endTime.toISOString(),
      });
      const summary = await fetchNvrEventSummary(
        summaryUrl,
        {
          startTime: startTime.toISOString(),
          endTime: endTime.toISOString(),
          eventCode: "all",
          channel: camera.channelNumber,
        },
        controller.signal,
      );
      if (
        controller.signal.aborted ||
        this._eventSummaryAbort !== controller ||
        requestVersion !== this._eventSummaryRequestVersion
      ) {
        return;
      }
      const cameraSummary = ensureTileSummaryChannel(summary, camera.channelNumber);
      const panelSummary = summarizePanelTodayEvents(
        summary.startTime,
        summary.endTime,
        [cameraSummary],
      );
      const cameraCounts = panelSummary.cameras.find(
        (item) =>
          item.rootDeviceId === camera.rootDeviceId &&
          item.channel === camera.channelNumber,
      ) ?? null;
      this.setEventSummary(panelSummary);
      this._eventSummaryCameraKey = `${camera.rootDeviceId}:${camera.channelNumber}`;
      this.logMedia("card tile event summary completed", {
        device_id: camera.deviceId,
        root_device_id: camera.rootDeviceId,
        channel: camera.channelNumber,
        total_count: summary.totalCount,
        channel_total_count: cameraCounts?.totalCount ?? 0,
        human_count: cameraCounts?.humanCount ?? 0,
        vehicle_count: cameraCounts?.vehicleCount ?? 0,
        ivs_count: cameraCounts?.ivsCount ?? 0,
      });
    } catch (error) {
      if (
        controller.signal.aborted ||
        this._eventSummaryAbort !== controller ||
        requestVersion !== this._eventSummaryRequestVersion
      ) {
        return;
      }
      this.logMedia("card tile event summary failed", {
        device_id: camera.deviceId,
        root_device_id: camera.rootDeviceId,
        channel: camera.channelNumber,
        url: redactUrlForLog(summaryUrl),
        error: error instanceof Error ? error.message : String(error),
      });
    } finally {
      if (
        this._eventSummaryAbort === controller &&
        requestVersion === this._eventSummaryRequestVersion
      ) {
        this._eventSummaryAbort = undefined;
        this._eventSummaryRefreshedAt = Date.now();
        this.scheduleEventSummaryRefresh(60_000);
      }
    }
  }

  private scheduleEventSummaryRefresh(delayMs: number): void {
    if (this._eventSummaryRefreshTimer !== null) {
      window.clearTimeout(this._eventSummaryRefreshTimer);
    }
    this._eventSummaryRefreshTimer = window.setTimeout(() => {
      this._eventSummaryRefreshTimer = null;
      void this.refreshEventSummary();
    }, delayMs);
  }

  private cancelEventSummaryRefresh(): void {
    this._eventSummaryAbort?.abort();
    this._eventSummaryAbort = undefined;
    if (this._eventSummaryRefreshTimer !== null) {
      window.clearTimeout(this._eventSummaryRefreshTimer);
      this._eventSummaryRefreshTimer = null;
    }
  }

  private resolveSummaryCamera(): CameraViewModel | null {
    if (!this.hass || !this._config) {
      return null;
    }
    const model = buildPanelModel(
      this.hass,
      {
        ...DISCOVERY_CONFIG_BASE,
        browser_bridge_url: this._config.browser_bridge_url,
        vto: this._config.vto,
      },
      { kind: "overview" },
    );
    const camera = model.cameras.find((item) => item.deviceId === this._config?.device_id) ?? null;
    if (!camera || camera.deviceKind !== "nvr_channel" || camera.channelNumber === null) {
      return null;
    }
    return camera;
  }

  private setEventSummary(summary: PanelTodayEventSummaryModel | null): void {
    const previousSummary = this._eventSummary;
    this._eventSummary = summary;
    this.requestUpdate("_eventSummary", previousSummary);
  }
}

function ensureTileSummaryChannel(
  summary: NvrEventSummaryModel,
  channel: number | null,
): NvrEventSummaryModel {
  if (channel === null || channel <= 0) {
    return summary;
  }
  if (summary.channels.some((item) => item.channel === channel)) {
    return summary;
  }
  if (summary.totalCount <= 0) {
    return summary;
  }
  return {
    ...summary,
    channels: [
      {
        channel,
        totalCount: summary.totalCount,
        items: summary.items,
      },
    ],
  };
}

if (!customElements.get("dahuabridge-surveillance-tile")) {
  customElements.define("dahuabridge-surveillance-tile", DahuaBridgeSurveillanceTileCard);
}

window.customCards = window.customCards || [];
window.customCards.push({
  type: "dahuabridge-surveillance-tile",
  name: "DahuaBridge Surveillance Tile",
  description: "Compact single-device DahuaBridge camera or VTO tile.",
  preview: true,
});
