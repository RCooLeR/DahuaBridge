import { css, html, LitElement, nothing, type TemplateResult } from "lit";
import { z } from "zod";

import { SurveillancePanelActions } from "./surveillance-panel-actions";
import {
  cameraImageSrc,
  defaultOverviewStreamProfileKey,
  defaultSelectedStreamProfileKey,
  renderSelectedCameraViewport,
  renderSelectedVtoViewport,
  resolveOverviewCameraViewportSource,
  resolveStreamViewportSource,
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
  BridgeIntercomSessionController,
  type BridgeIntercomSnapshot,
  resolveIntercomOfferUrl,
} from "../ha/bridge-intercom";
import {
  syncRemoteStreamStyles,
} from "./surveillance-panel-media";
import { surveillancePanelBaseStyles, surveillancePanelOverviewStyles } from "./surveillance-panel-styles";
import { openExternalUrl } from "../utils/browser";
import { logCardInfo, redactUrlForLog } from "../utils/logging";

const vtoSchema = z
  .object({
    device_id: z.string().min(1).optional(),
    label: z.string().min(1).optional(),
    lock_button_entity: z.string().min(1).optional(),
    input_volume_entity: z.string().min(1).optional(),
    output_volume_entity: z.string().min(1).optional(),
    muted_entity: z.string().min(1).optional(),
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
  title: "DahuaBridge Surveillance",
  subtitle: "Compact tile",
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

      .tile-title-banner {
        min-width: 0;
        max-width: calc(100% - 88px);
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
        .tile-title-banner {
          max-width: calc(100% - 72px);
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
  private _remoteStreamSyncTimer: number | null = null;

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
        this._errorMessage = snapshot.error;
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
    void this.stopVtoMicrophone();
  }

  disconnectedCallback(): void {
    if (this._remoteStreamSyncTimer !== null) {
      window.clearTimeout(this._remoteStreamSyncTimer);
      this._remoteStreamSyncTimer = null;
    }

    void this.stopVtoMicrophone();
    super.disconnectedCallback();
  }

  private scheduleRemoteStreamStyleSync(): void {
    if (this._remoteStreamSyncTimer !== null) {
      window.clearTimeout(this._remoteStreamSyncTimer);
    }

    const syncDelays = [0, 50, 150, 400, 1000, 2500, 5000];

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

    runSyncAt(0);
  }

  protected updated(changedProperties: Map<PropertyKey, unknown>): void {
    this.scheduleRemoteStreamStyleSync();
    window.requestAnimationFrame(() => {
      this.syncCameraViewportAudioState(this._cameraAudioMuted);
    });

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
    if (!this._config) {
      return html`<ha-card><div class="tile-shell"><div class="muted">Card configuration missing.</div></div></ha-card>`;
    }
    if (!this.hass) {
      return html`<ha-card><div class="tile-shell"><div class="muted">Home Assistant state unavailable.</div></div></ha-card>`;
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
    const vto = model.vtos.find((item) => item.deviceId === this._config?.device_id) ?? null;

    if (camera) {
      return this.renderCameraTile(camera);
    }
    if (vto) {
      return this.renderVtoTile(vto);
    }

    return html`
      <ha-card>
        <div class="tile-shell">
          <div class="muted">
            Device ${this._config.device_id} was not found in DahuaBridge camera discovery.
          </div>
        </div>
      </ha-card>
    `;
  }

  private renderCameraTile(camera: CameraViewModel): TemplateResult {
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
                {
                  controls: false,
                  preload: "none",
                  fallbackOrder: ["hls", "mjpeg"],
                },
              )}
              <div class="tile-topbar">
                <div class="tile-title-banner">
                  <div class="tile-name">${title}</div>
                </div>
              </div>
              <div class="media-overlay">
                <div class="media-bottom">
                  <div class="tile-overlay-badges">
                    ${camera.bridgeRecordingActive
                      ? html`<span class="badge warning">MP4</span>`
                      : nothing}
                    ${!camera.streamAvailable
                      ? html`<span class="badge critical">Stream Down</span>`
                      : nothing}
                    ${camera.detections.map(
                      (badge) => html`<span class="badge ${badge.tone}">${badge.label}</span>`,
                    )}
                  </div>
                  <div class="tile-controls">
                    ${this.hasSnapshot(camera)
                      ? renderIconButton(
                          "Snapshot",
                          "mdi:camera",
                          () => this.openWindow(this.resolveSnapshotUrl(camera)),
                          this.renderIcon,
                        )
                      : nothing}
                    ${camera.supportsRecording
                      ? renderIconButton(
                          camera.bridgeRecordingActive ? "Stop MP4" : "Start MP4",
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
                          lightActive ? "Return to Smart Light" : "Turn on White Light",
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
                          warningLightActive ? "Turn Warning Light Off" : "Turn Warning Light On",
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
                          sirenActive ? "Turn Siren Off" : "Turn Siren On",
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
                            ? "Enable Stream Audio"
                            : "Disable Stream Audio",
                          this._cameraAudioMuted ? "mdi:volume-high" : "mdi:volume-off",
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

  private renderVtoTile(vto: VtoViewModel): TemplateResult {
    const selectedProfileKey = defaultSelectedStreamProfileKey(vto.stream);
    const selectedSource = resolveStreamViewportSource(
      vto.stream,
      null,
      selectedProfileKey,
      Boolean(vto.cameraEntity),
    );
    const title = this._config?.title ?? vto.label;
    const showCallActions = vto.callState === "ringing" || vto.callState === "active";

    return html`
      <ha-card>
        <div class="tile-shell">
          <article class="tile-card">
            <div class="tile-media">
              ${renderSelectedVtoViewport(
                vto,
                this._vtoStreamPlaying,
                selectedProfileKey,
                selectedSource,
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
                    ${vto.doorbell ? html`<span class="badge warning">Doorbell</span>` : nothing}
                    ${vto.tamper ? html`<span class="badge critical">Tamper</span>` : nothing}
                    ${this._vtoMicrophoneState.enabled
                      ? html`<span class="badge info">${this._vtoMicrophoneState.statusText}</span>`
                      : nothing}
                  </div>
                  <div class="tile-controls">
                    ${this.hasPlayableVtoStream(vto)
                      ? renderIconButton(
                          this._vtoStreamPlaying ? "Stop Stream" : "Play Stream",
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
                          "Snapshot",
                          "mdi:camera",
                          () => this.openWindow(this.resolveVtoSnapshotUrl(vto)),
                          this.renderIcon,
                        )
                      : nothing}
                    ${vto.recordingStartUrl || vto.recordingStopUrl
                      ? renderIconButton(
                          vto.bridgeRecordingActive ? "Stop MP4" : "Start MP4",
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
                          "Unlock",
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
                          "Answer Call",
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
                          "Hang Up",
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
                    ${(vto.hasMutedEntity || Boolean(vto.mutedActionUrl))
                      ? renderIconButton(
                          vto.muted ? "Unmute" : "Mute",
                          vto.muted ? "mdi:volume-off" : "mdi:volume-high",
                          () =>
                            void this._actions.triggerVtoSwitchAction(
                              "vto:mute",
                              vto.mutedEntityId,
                              !vto.muted,
                              vto.mutedActionUrl,
                              "muted",
                            ),
                          this.renderIcon,
                          {
                            disabled: this._actions.isBusy("vto:mute"),
                            tone: vto.muted ? "warning" : undefined,
                            active: vto.muted,
                          },
                        )
                      : nothing}
                    ${vto.capabilities.browserMicrophoneSupported && this.hasAvailableVtoIntercom(vto)
                      ? renderIconButton(
                          this._vtoMicrophoneState.enabled ? "Disable Mic" : "Enable Mic",
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
      audio_supported: camera.audioMuteSupported,
    });

    if (!camera.audioMuteSupported) {
      return;
    }
  }

  private async triggerVtoBridgeRecording(vto: VtoViewModel): Promise<void> {
    const targetUrl = vto.bridgeRecordingActive ? vto.recordingStopUrl : vto.recordingStartUrl;
    if (!targetUrl) {
      this._errorMessage = "Bridge MP4 recording is unavailable for this door station.";
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
        error instanceof Error ? error.message : "Bridge MP4 recording request failed.";
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete("vto:bridge_recording");
      this._busyActions = reducedBusy;
    }
  }

  private async startVtoMicrophone(vto: VtoViewModel): Promise<void> {
    const offerUrl = resolveIntercomOfferUrl(vto.stream);
    if (!offerUrl) {
      this._errorMessage = "Bridge intercom offer URL is unavailable for this door station.";
      return;
    }
    if (!this._vtoStreamPlaying) {
      const previousPlaying = this._vtoStreamPlaying;
      this._vtoStreamPlaying = true;
      this.requestUpdate("_vtoStreamPlaying", previousPlaying);
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
    return (
      vto.streamAvailable &&
      (Boolean(vto.cameraEntity) ||
        vto.stream.profiles.some(
          (profile) =>
            Boolean(profile.localDashUrl) ||
            Boolean(profile.localHlsUrl) ||
            Boolean(profile.localMjpegUrl),
        ))
    );
  }

  private syncCameraViewportAudioState(muted: boolean): void {
    syncViewportAudioState(
      this.renderRoot.querySelector(".tile-media"),
      muted,
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

  private renderIcon(icon: string): TemplateResult {
    return html`<ha-icon .icon=${icon}></ha-icon>`;
  }
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
