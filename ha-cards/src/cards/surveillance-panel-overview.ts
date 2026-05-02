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

type OverviewTile = CameraViewModel | VtoViewModel;

interface RenderSurveillancePanelOverviewArgs {
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
  onVtoMute: (vto: VtoViewModel) => void;
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
  onVtoMute,
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
                    onVtoMute,
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
            ${camera.online ? "Online" : "Offline"}
          </span>
          <span class="status-dot ${camera.online ? "" : "critical"}"></span>
        </div>
      </div>
      <div class="tile-media">
        ${renderCameraViewport(camera)}
        ${renderCameraEventCountBadges(camera, "overlay")}
        <div class="media-overlay">
          <div class="media-bottom">
            <div class="tile-overlay-badges">
              ${camera.recordingActive
                ? html`<span
                    class="recording-dot"
                    title="NVR Recording"
                    aria-label="NVR Recording"
                  ></span>`
                : null}
              ${bridgeRecordingActive
                ? html`<span class="badge warning">MP4 Clip</span>`
                : null}
              ${!camera.streamAvailable
                ? html`<span class="badge critical">Stream Down</span>`
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
                    "Snapshot",
                    "mdi:camera",
                    () => onOpenSnapshot(camera),
                    renderIcon,
                  )
                : null}
              ${camera.supportsRecording
                ? renderIconButton(
                    bridgeRecordingActive ? "Stop MP4" : "Start MP4",
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
                    lightActive ? "Return to smart light" : "Turn on white light",
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
                    warningLightActive ? "Turn warning light off" : "Turn warning light on",
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
                    sirenActive ? "Turn siren off" : "Turn siren on",
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
                    "PTZ controls",
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
                    cameraMuted ? "Enable Stream Audio" : "Disable Stream Audio",
                    cameraMuted ? "mdi:volume-high" : "mdi:volume-off",
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
  onVtoMute,
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
}: {
  vto: VtoViewModel;
  selection: PanelSelection;
  onSelectVto: (vto: VtoViewModel) => void;
  onVtoUnlock: (vto: VtoViewModel) => void;
  onVtoAnswer: (vto: VtoViewModel) => void;
  onVtoHangup: (vto: VtoViewModel) => void;
  onVtoMute: (vto: VtoViewModel) => void;
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
                ? html`<span class="recording-dot" title="MP4 Recording" aria-label="MP4 Recording"></span>`
                : null}
              ${vto.doorbell ? html`<span class="badge warning">Doorbell</span>` : null}
              ${vto.tamper ? html`<span class="badge critical">Tamper</span>` : null}
            </div>
            <div class="tile-controls">
              ${hasPlayableVtoStream(vto)
                ? renderIconButton(
                    streamPlaying ? "Stop Stream" : "Play Stream",
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
                    "Snapshot",
                    "mdi:camera",
                    () => onOpenVtoSnapshot(vto),
                    renderIcon,
                  )
                : null}
              ${vto.recordingStartUrl || vto.recordingStopUrl
                ? renderIconButton(
                    bridgeRecordingActive ? "Stop MP4" : "Start MP4",
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
                    "Unlock",
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
                    "Answer call",
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
                    "Hang up",
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
              ${vto.hasMutedEntity || Boolean(vto.mutedActionUrl)
                ? renderIconButton(
                    vto.muted ? "Unmute" : "Mute",
                    vto.muted ? "mdi:volume-off" : "mdi:volume-high",
                    () => onVtoMute(vto),
                    renderIcon,
                    {
                      disabled: isBusy("vto:mute"),
                      tone: vto.muted ? "warning" : undefined,
                      active: vto.muted,
                    },
                  )
                : null}
              ${vto.capabilities.browserMicrophoneSupported && hasAvailableVtoIntercom(vto)
                ? renderIconButton(
                    isVtoMicrophoneActive(vto) ? "Disable Mic" : "Enable Mic",
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
