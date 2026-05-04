import { html, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import {
  displayCameraLabel,
  supportsAuxTarget,
  type CameraViewModel,
  type PanelSelection,
  type VtoViewModel,
} from "../domain/model";
import type { HassEntity } from "../types/home-assistant";
import type { SurveillanceOverviewLayout } from "./surveillance-panel-state";
import { renderIconButton } from "./surveillance-panel-primitives";
import { renderCameraEventCountBadges } from "./surveillance-panel-event-badges";
import type { Localizer } from "../localization";

type OverviewTile = CameraViewModel | VtoViewModel;

interface RenderSurveillancePanelOverviewArgs {
  t: Localizer;
  overviewTiles: OverviewTile[];
  layout: SurveillanceOverviewLayout;
  selection: PanelSelection;
  ptzAdjusting: boolean;
  onSelectCamera: (camera: CameraViewModel) => void;
  onSelectVto: (vto: VtoViewModel) => void;
  onOpenSnapshot: (camera: CameraViewModel) => void;
  onTriggerRecording: (camera: CameraViewModel, action: "start" | "stop") => void;
  onTriggerAux: (camera: CameraViewModel, output: string) => void;
  onEnablePtz: (camera: CameraViewModel) => void;
  onToggleCameraAudio: (camera: CameraViewModel) => void;
  onVtoUnlock: (vto: VtoViewModel) => void;
  onVtoAnswer: (vto: VtoViewModel) => void;
  onVtoHangup: (vto: VtoViewModel) => void;
  onOpenVtoSnapshot: (vto: VtoViewModel) => void;
  onToggleVtoRecording: (vto: VtoViewModel) => void;
  onToggleVtoStream: (vto: VtoViewModel) => void;
  onToggleVtoMicrophone: (vto: VtoViewModel) => void;
  renderIcon: (icon: string) => TemplateResult;
  renderCameraViewport: (camera: CameraViewModel) => TemplateResult;
  isCameraMuted: (camera: CameraViewModel) => boolean;
  cameraImageSrc: (cameraEntity: HassEntity | undefined, snapshotUrl?: string | null) => string;
  renderVtoViewport: (vto: VtoViewModel, playing: boolean) => TemplateResult;
  canOpenSnapshot: (camera: CameraViewModel) => boolean;
  canOpenVtoSnapshot: (vto: VtoViewModel) => boolean;
  isBridgeRecordingActive: (camera: CameraViewModel) => boolean;
  isVtoBridgeRecordingActive: (vto: VtoViewModel) => boolean;
  isAuxActive: (camera: CameraViewModel, output: string) => boolean;
  isVtoStreamPlaying: (vto: VtoViewModel) => boolean;
  isVtoMicrophoneActive: (vto: VtoViewModel) => boolean;
  hasPlayableVtoStream: (vto: VtoViewModel) => boolean;
  hasAvailableVtoIntercom: (vto: VtoViewModel) => boolean;
  isBusy: (key: string) => boolean;
  vtoBadgeClass: (vto: VtoViewModel) => string;
}

export function renderSurveillancePanelOverview({
  t,
  overviewTiles,
  layout,
  selection,
  ptzAdjusting,
  onSelectCamera,
  onSelectVto,
  onOpenSnapshot,
  onTriggerRecording,
  onTriggerAux,
  onEnablePtz,
  onToggleCameraAudio,
  onVtoUnlock,
  onVtoAnswer,
  onVtoHangup,
  onOpenVtoSnapshot,
  onToggleVtoRecording,
  onToggleVtoStream,
  onToggleVtoMicrophone,
  renderIcon,
  renderCameraViewport,
  isCameraMuted,
  cameraImageSrc,
  renderVtoViewport,
  canOpenSnapshot,
  canOpenVtoSnapshot,
  isBridgeRecordingActive,
  isVtoBridgeRecordingActive,
  isAuxActive,
  isVtoStreamPlaying,
  isVtoMicrophoneActive,
  hasPlayableVtoStream,
  hasAvailableVtoIntercom,
  isBusy,
  vtoBadgeClass,
}: RenderSurveillancePanelOverviewArgs): TemplateResult {
  return html`
    <section class="main">
      <div class="overview-shell">
        <div class="overview-grid layout-${layout.name}">
          ${repeat(
            overviewTiles,
            (item) => item.deviceId,
            (item) =>
              item.type === "vto"
                ? renderVtoTile({
                    vto: item,
                    selection,
                    onSelectVto,
                    onVtoUnlock,
                    onVtoAnswer,
                    onVtoHangup,
                    onOpenVtoSnapshot,
                    onToggleVtoRecording,
                    onToggleVtoStream,
                    onToggleVtoMicrophone,
                    renderIcon,
                    cameraImageSrc,
                    renderVtoViewport,
                    canOpenVtoSnapshot,
                    isVtoBridgeRecordingActive,
                    isVtoStreamPlaying,
                    isVtoMicrophoneActive,
                    hasPlayableVtoStream,
                    hasAvailableVtoIntercom,
                    vtoBadgeClass,
                    isBusy,
                    t,
                  })
                : renderCameraTile({
                    camera: item,
                    selection,
                    onSelectCamera,
                    onOpenSnapshot,
                    onTriggerRecording,
                    onTriggerAux,
                    onEnablePtz,
                    onToggleCameraAudio,
                    renderIcon,
                    renderCameraViewport,
                    isCameraMuted,
                    canOpenSnapshot,
                    isBridgeRecordingActive,
                    isAuxActive,
                    isBusy,
                    ptzAdjusting,
                    t,
                  }),
          )}
        </div>
      </div>
    </section>
  `;
}

function renderCameraTile({
  camera,
  selection,
  onSelectCamera,
  onOpenSnapshot,
  onTriggerRecording,
  onTriggerAux,
  onEnablePtz,
  onToggleCameraAudio,
  renderIcon,
  renderCameraViewport,
  isCameraMuted,
  canOpenSnapshot,
  isBridgeRecordingActive,
  isAuxActive,
  isBusy,
  ptzAdjusting,
  t,
}: {
  camera: CameraViewModel;
  selection: PanelSelection;
  onSelectCamera: (camera: CameraViewModel) => void;
  onOpenSnapshot: (camera: CameraViewModel) => void;
  onTriggerRecording: (camera: CameraViewModel, action: "start" | "stop") => void;
  onTriggerAux: (camera: CameraViewModel, output: string) => void;
  onEnablePtz: (camera: CameraViewModel) => void;
  onToggleCameraAudio: (camera: CameraViewModel) => void;
  renderIcon: (icon: string) => TemplateResult;
  renderCameraViewport: (camera: CameraViewModel) => TemplateResult;
  isCameraMuted: (camera: CameraViewModel) => boolean;
  canOpenSnapshot: (camera: CameraViewModel) => boolean;
  isBridgeRecordingActive: (camera: CameraViewModel) => boolean;
  isAuxActive: (camera: CameraViewModel, output: string) => boolean;
  isBusy: (key: string) => boolean;
  ptzAdjusting: boolean;
  t: Localizer;
}): TemplateResult {
  const selected = selection.kind === "camera" && selection.deviceId === camera.deviceId;
  const lightAvailable = supportsAuxTarget(camera, "light");
  const warningLightAvailable = supportsAuxTarget(camera, "warning_light");
  const sirenAvailable = supportsAuxTarget(camera, "siren");
  const bridgeRecordingActive = isBridgeRecordingActive(camera);
  const lightActive = isAuxActive(camera, "light");
  const warningLightActive = isAuxActive(camera, "warning_light");
  const sirenActive = isAuxActive(camera, "siren");
  const cameraMuted = isCameraMuted(camera);

  return html`
    <article
      class="camera-tile ${selected ? "selected" : ""}"
      data-device-id=${camera.deviceId}
      @click=${() => onSelectCamera(camera)}
    >
      <div class="tile-header">
          <div class="tile-title-text">
          <div class="tile-name">${displayCameraLabel(camera)}</div>
          <div class="tile-subtitle">${camera.roomLabel} | ${camera.kindLabel}</div>
        </div>
        <div class="tile-status">
          <span class="badge ${camera.online ? "success" : "critical"}">
            ${camera.online ? t("state.online") : t("state.offline")}
          </span>
          <span class="status-dot ${camera.online ? "" : "critical"}"></span>
        </div>
      </div>
      <div class="tile-media">
        ${renderCameraViewport(camera)}
        <div class="media-overlay">
          <div class="media-bottom">
            <div class="tile-overlay-badges">
              ${renderCameraEventCountBadges(camera, "inline", t)}
              ${camera.recordingActive
                ? html`<span
                    class="recording-dot"
                    title=${t("badge.nvrRecording")}
                    aria-label=${t("badge.nvrRecording")}
                  ></span>`
                : null}
              ${bridgeRecordingActive
                ? html`<span class="badge warning">${t("badge.mp4Clip")}</span>`
                : null}
              ${!camera.streamAvailable
                ? html`<span class="badge critical">${t("badge.streamDown")}</span>`
                : null}
              ${repeat(
                camera.detections,
                (badge) => badge.key,
                (badge) => html`<span class="badge ${badge.tone}">${badge.label}</span>`,
              )}
            </div>
            <div class="tile-controls">
              ${canOpenSnapshot(camera)
                ? renderIconButton(
                    t("button.snapshot"),
                    "mdi:camera",
                    () => onOpenSnapshot(camera),
                    renderIcon,
                  )
                : null}
              ${camera.supportsRecording
                ? renderIconButton(
                    bridgeRecordingActive ? t("button.stopMp4") : t("button.startMp4"),
                    bridgeRecordingActive ? "mdi:record-rec" : "mdi:record-circle-outline",
                    () =>
                      onTriggerRecording(
                        camera,
                        bridgeRecordingActive ? "stop" : "start",
                      ),
                    renderIcon,
                    {
                      disabled: isBusy(
                        `${camera.deviceId}:recording:${bridgeRecordingActive ? "stop" : "start"}`,
                      ),
                      tone: bridgeRecordingActive ? "danger" : "warning",
                      active: bridgeRecordingActive,
                    },
                  )
                : null}
              ${camera.supportsAux
              && lightAvailable
                ? renderIconButton(
                    lightActive ? t("button.lightSmart") : t("button.lightWhite"),
                    "mdi:lightbulb-on-outline",
                    () => onTriggerAux(camera, "light"),
                    renderIcon,
                    {
                      disabled: isBusy(`${camera.deviceId}:aux:light`),
                      tone: lightActive ? "primary" : undefined,
                      active: lightActive,
                    },
                  )
                : null}
              ${camera.supportsAux
              && warningLightAvailable
                ? renderIconButton(
                    warningLightActive ? t("button.warningLightOff") : t("button.warningLightOn"),
                    "mdi:alarm-light-outline",
                    () => onTriggerAux(camera, "warning_light"),
                    renderIcon,
                    {
                      disabled: isBusy(`${camera.deviceId}:aux:warning_light`),
                      tone: warningLightActive ? "warning" : undefined,
                      active: warningLightActive,
                    },
                  )
                : null}
              ${camera.supportsAux
              && sirenAvailable
                ? renderIconButton(
                    sirenActive ? t("button.sirenOff") : t("button.sirenOn"),
                    "mdi:bullhorn",
                    () => onTriggerAux(camera, "siren"),
                    renderIcon,
                    {
                      disabled: isBusy(`${camera.deviceId}:aux:siren`),
                      tone: "warning",
                      active: sirenActive,
                    },
                  )
                : null}
              ${camera.supportsPtz
                ? renderIconButton(
                    t("button.ptzControls"),
                    "mdi:axis-arrow",
                    () => onEnablePtz(camera),
                    renderIcon,
                    {
                      active: selected && ptzAdjusting,
                      tone: selected && ptzAdjusting ? "primary" : undefined,
                    },
                  )
                : null}
              ${camera.audioCodec.trim()
                ? renderIconButton(
                    cameraMuted ? t("button.enableStreamAudio") : t("button.disableStreamAudio"),
                    cameraMuted ? "mdi:volume-off" : "mdi:volume-high",
                    () => onToggleCameraAudio(camera),
                    renderIcon,
                    {
                      active: !cameraMuted,
                    },
                  )
                : null}
            </div>
          </div>
        </div>
      </div>
    </article>
  `;
}

function renderVtoTile({
  vto,
  selection,
  onSelectVto,
  onVtoUnlock,
  onVtoAnswer,
  onVtoHangup,
  onOpenVtoSnapshot,
  onToggleVtoRecording,
  onToggleVtoStream,
  onToggleVtoMicrophone,
  renderIcon,
  cameraImageSrc,
  renderVtoViewport,
  canOpenVtoSnapshot,
  isVtoBridgeRecordingActive,
  isVtoStreamPlaying,
  isVtoMicrophoneActive,
  hasPlayableVtoStream,
  hasAvailableVtoIntercom,
  vtoBadgeClass,
  isBusy,
  t,
}: {
  vto: VtoViewModel;
  selection: PanelSelection;
  onSelectVto: (vto: VtoViewModel) => void;
  onVtoUnlock: (vto: VtoViewModel) => void;
  onVtoAnswer: (vto: VtoViewModel) => void;
  onVtoHangup: (vto: VtoViewModel) => void;
  onOpenVtoSnapshot: (vto: VtoViewModel) => void;
  onToggleVtoRecording: (vto: VtoViewModel) => void;
  onToggleVtoStream: (vto: VtoViewModel) => void;
  onToggleVtoMicrophone: (vto: VtoViewModel) => void;
  renderIcon: (icon: string) => TemplateResult;
  cameraImageSrc: (cameraEntity: HassEntity | undefined, snapshotUrl?: string | null) => string;
  renderVtoViewport: (vto: VtoViewModel, playing: boolean) => TemplateResult;
  canOpenVtoSnapshot: (vto: VtoViewModel) => boolean;
  isVtoBridgeRecordingActive: (vto: VtoViewModel) => boolean;
  isVtoStreamPlaying: (vto: VtoViewModel) => boolean;
  isVtoMicrophoneActive: (vto: VtoViewModel) => boolean;
  hasPlayableVtoStream: (vto: VtoViewModel) => boolean;
  hasAvailableVtoIntercom: (vto: VtoViewModel) => boolean;
  vtoBadgeClass: (vto: VtoViewModel) => string;
  isBusy: (key: string) => boolean;
  t: Localizer;
}): TemplateResult {
  const streamPlaying = isVtoStreamPlaying(vto);
  const bridgeRecordingActive = isVtoBridgeRecordingActive(vto);
  const callActionVisible = vto.callState === "ringing" || vto.callState === "active";
  return html`
    <article
      class="camera-tile ${selection.kind === "vto" && selection.deviceId === vto.deviceId
        ? "selected"
        : ""}"
      @click=${() => onSelectVto(vto)}
    >
      <div class="tile-header">
        <div class="tile-title-text">
          <div class="tile-name">${vto.label}</div>
          <div class="tile-subtitle">${vto.roomLabel}</div>
        </div>
        <div class="tile-status">
          <span class="badge ${vtoBadgeClass(vto)}">${vto.callStateText}</span>
          <span class="status-dot ${vto.online ? "" : "critical"}"></span>
        </div>
      </div>
      <div class="tile-media">
        ${streamPlaying
          ? renderVtoViewport(vto, true)
          : html`
              <img
                class="tile-image"
                src=${cameraImageSrc(vto.cameraEntity, vto.snapshotUrl)}
                alt=${vto.label}
                loading="lazy"
              />
            `}
        <div class="media-overlay">
          <div class="media-bottom">
            <div class="tile-overlay-badges">
              ${bridgeRecordingActive
                ? html`<span class="recording-dot" title=${t("badge.mp4Clip")} aria-label=${t("badge.mp4Clip")}></span>`
                : null}
              ${vto.doorbell ? html`<span class="badge warning">${t("badge.doorbell")}</span>` : null}
              ${vto.tamper ? html`<span class="badge critical">${t("badge.tamper")}</span>` : null}
            </div>
            <div class="tile-controls">
              ${hasPlayableVtoStream(vto)
                ? renderIconButton(
                    streamPlaying ? t("button.stopStream") : t("button.playStream"),
                    streamPlaying ? "mdi:stop-circle-outline" : "mdi:play-circle-outline",
                    () => onToggleVtoStream(vto),
                    renderIcon,
                    {
                      tone: streamPlaying ? "warning" : "primary",
                      active: streamPlaying,
                    },
                  )
                : null}
              ${canOpenVtoSnapshot(vto)
                ? renderIconButton(
                    t("button.snapshot"),
                    "mdi:camera",
                    () => onOpenVtoSnapshot(vto),
                    renderIcon,
                  )
                : null}
              ${vto.recordingStartUrl || vto.recordingStopUrl
                ? renderIconButton(
                    bridgeRecordingActive ? t("button.stopMp4") : t("button.startMp4"),
                    bridgeRecordingActive ? "mdi:record-rec" : "mdi:record-circle-outline",
                    () => onToggleVtoRecording(vto),
                    renderIcon,
                    {
                      disabled: isBusy("vto:bridge_recording"),
                      tone: bridgeRecordingActive ? "danger" : "warning",
                      active: bridgeRecordingActive,
                    },
                  )
                : null}
              ${callActionVisible && (vto.hasUnlockButtonEntity || Boolean(vto.unlockActionUrl))
                ? renderIconButton(
                    t("button.unlock"),
                    "mdi:lock-open-variant",
                    () => onVtoUnlock(vto),
                    renderIcon,
                    {
                      disabled: isBusy("vto:unlock"),
                      tone: "primary",
                    },
                  )
                : null}
              ${vto.callState === "ringing" &&
              (vto.hasAnswerButtonEntity || Boolean(vto.answerActionUrl))
                ? renderIconButton(
                    t("button.answerCall"),
                    "mdi:phone",
                    () => onVtoAnswer(vto),
                    renderIcon,
                    {
                      disabled: isBusy("vto:answer"),
                      tone: "warning",
                    },
                  )
                : null}
              ${callActionVisible &&
              (vto.hasHangupButtonEntity || Boolean(vto.hangupActionUrl))
                ? renderIconButton(
                    t("button.hangUp"),
                    "mdi:phone-hangup",
                    () => onVtoHangup(vto),
                    renderIcon,
                    {
                      disabled: isBusy("vto:hangup"),
                      tone: "danger",
                      active: vto.callState === "active",
                    },
                  )
                : null}
              ${vto.capabilities.browserMicrophoneSupported && hasAvailableVtoIntercom(vto)
                ? renderIconButton(
                    isVtoMicrophoneActive(vto) ? t("button.disableMic") : t("button.enableMic"),
                    isVtoMicrophoneActive(vto) ? "mdi:microphone-off" : "mdi:microphone",
                    () => onToggleVtoMicrophone(vto),
                    renderIcon,
                    {
                      tone: isVtoMicrophoneActive(vto) ? "warning" : undefined,
                      active: isVtoMicrophoneActive(vto),
                    },
                  )
                : null}
            </div>
          </div>
        </div>
      </div>
    </article>
  `;
}
