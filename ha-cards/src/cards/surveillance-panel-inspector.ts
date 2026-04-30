import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import {
  displayCameraLabel,
  type CameraViewModel,
  type NvrViewModel,
  type PanelModel,
  type VtoLockViewModel,
  type VtoViewModel,
} from "../domain/model";
import { formatDuration } from "../utils/format";
import {
  renderControlButton,
  renderSegmentButton,
} from "./surveillance-panel-primitives";

type DetailTab = "overview" | "events" | "recordings" | "settings";

interface RenderSurveillancePanelInspectorArgs {
  model: PanelModel;
  inspectorOpen: boolean;
  errorMessage: string;
  detailTab: DetailTab;
  eventContent: TemplateResult | typeof nothing;
  archiveContent: TemplateResult | typeof nothing;
  renderIcon: (icon: string) => TemplateResult;
  isBusy: (key: string) => boolean;
  onSelectDetailTab: (tab: DetailTab) => void;
  onVtoRangeChange: (
    event: Event,
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ) => Promise<void>;
  onVtoSwitchAction: (
    key: string,
    entityId: string,
    enabled: boolean,
    fallbackUrl: string | null,
    payloadKey: string,
  ) => Promise<void>;
  onVtoButtonAction: (
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ) => Promise<void>;
}

export function renderSurveillancePanelInspector({
  model,
  inspectorOpen,
  errorMessage,
  detailTab,
  eventContent,
  archiveContent,
  renderIcon,
  isBusy,
  onSelectDetailTab,
  onVtoRangeChange,
  onVtoSwitchAction,
  onVtoButtonAction,
}: RenderSurveillancePanelInspectorArgs): TemplateResult | typeof nothing {
  if (!model.selectedCamera && !model.selectedNvr && !model.selectedVto) {
    return nothing;
  }

  return html`
    <aside class="inspector ${inspectorOpen ? "" : "mobile-hidden"}">
      ${errorMessage ? html`<div class="error-banner">${errorMessage}</div>` : nothing}

      ${model.selectedCamera
        ? renderCameraInspector(
            model.selectedCamera,
            detailTab,
            eventContent,
            archiveContent,
            onSelectDetailTab,
          )
        : model.selectedNvr
          ? renderNvrInspector(model.selectedNvr, renderIcon, detailTab, archiveContent, onSelectDetailTab)
          : model.selectedVto
          ? renderVtoInspector(
              model.selectedVto,
              detailTab,
              eventContent,
              renderIcon,
              isBusy,
              onSelectDetailTab,
              onVtoRangeChange,
              onVtoSwitchAction,
              onVtoButtonAction,
            )
          : nothing}
    </aside>
  `;
}

function renderNvrInspector(
  nvr: NvrViewModel,
  renderIcon: (icon: string) => TemplateResult,
  detailTab: DetailTab,
  archiveContent: TemplateResult | typeof nothing,
  onSelectDetailTab: (tab: DetailTab) => void,
): TemplateResult {
  const channels = nvr.rooms.flatMap((room) => room.channels);
  const onlineCount = channels.filter((channel) => channel.online).length;
  const recordingCount = channels.filter((channel) => channel.recordingActive).length;
  const alertCount = channels.filter((channel) => channel.detections.length > 0).length;

  return html`
    <div class="detail-header">
      <div class="detail-title">${nvr.label}</div>
      <div class="muted">${nvr.roomLabel}</div>
    </div>
    <div class="detail-tabs">
      ${renderSegmentButton("overview", "Overview", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("recordings", "Recordings", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
    </div>
    <div class="detail-main">
      ${detailTab === "overview"
        ? html`
            <div class="panel">
              <div class="panel-title">Recorder</div>
              <div class="nvr-summary-chip-grid">
                ${renderNvrSummaryChip(
                  "mdi:lan-connect",
                  "Connection",
                  nvr.online ? "Connected" : "Offline",
                  nvr.online ? "success" : "critical",
                  renderIcon,
                )}
                ${renderNvrSummaryChip(
                  "mdi:record-rec",
                  "Recorder",
                  nvr.recordingActive ? "Recording active" : "Recording idle",
                  nvr.recordingActive ? "critical" : "info",
                  renderIcon,
                )}
                ${renderNvrSummaryChip(
                  "mdi:harddisk",
                  "Storage",
                  nvr.storageText,
                  nvr.healthy ? "success" : "warning",
                  renderIcon,
                )}
                ${nvr.nvrConfigWritable !== null
                  ? renderNvrSummaryChip(
                      "mdi:cog-refresh-outline",
                      "Config Writes",
                      nvr.nvrConfigWritable ? "Writable" : "Blocked",
                      nvr.nvrConfigWritable ? "success" : "warning",
                      renderIcon,
                    )
                  : nothing}
                ${renderNvrSummaryChip(
                  "mdi:cctv",
                  "Channels recording",
                  `${recordingCount} recording`,
                  "info",
                  renderIcon,
                )}
                ${alertCount > 0
                  ? renderNvrSummaryChip(
                      "mdi:alert-outline",
                      "Alerts",
                      `${alertCount} alert${alertCount === 1 ? "" : "s"}`,
                      "warning",
                      renderIcon,
                    )
                  : nothing}
              </div>
            </div>
            ${renderNvrDriveBreakdown(nvr, renderIcon)}
            ${renderNvrRecordingState(nvr, renderIcon)}
          `
        : nothing}
      ${detailTab === "recordings" ? archiveContent : nothing}
    </div>
  `;
}

function renderNvrRecordingState(
  nvr: NvrViewModel,
  renderIcon: (icon: string) => TemplateResult,
): TemplateResult {
  const channels = nvr.rooms.flatMap((room) => room.channels);
  const onlineCount = channels.filter((channel) => channel.online).length;
  const recordingChannels = channels.filter((channel) => channel.recordingActive);
  const offlineChannels = channels.filter((channel) => !channel.online);
  const streamDownChannels = channels.filter((channel) => !channel.streamAvailable);
  const idleChannels = channels.filter((channel) => channel.online && !channel.recordingActive);

  return html`
    <div class="panel">
      <div class="panel-title">Channel State</div>
      <div class="nvr-summary-chip-grid">
        ${renderNvrSummaryChip(
          "mdi:lan-connect",
          "Online",
          `${onlineCount}/${channels.length} online`,
          "info",
          renderIcon,
        )}
        ${renderNvrSummaryChip(
          "mdi:record-rec",
          "Recording",
          `${recordingChannels.length} recording`,
          recordingChannels.length > 0 ? "critical" : "info",
          renderIcon,
        )}
        ${renderNvrSummaryChip(
          "mdi:pause-circle-outline",
          "Idle",
          `${idleChannels.length} idle`,
          "info",
          renderIcon,
        )}
        ${offlineChannels.length > 0
          ? renderNvrSummaryChip(
              "mdi:lan-disconnect",
              "Offline",
              `${offlineChannels.length} offline`,
              "critical",
              renderIcon,
            )
          : nothing}
        ${streamDownChannels.length > 0
          ? renderNvrSummaryChip(
              "mdi:wifi-strength-alert-outline",
              "Stream health",
              `${streamDownChannels.length} stream down`,
              "warning",
              renderIcon,
            )
          : nothing}
      </div>
      <div class="muted">
        Channel grouping and room assignment stay in the left sidebar so the recorder panel can stay focused on current state.
      </div>
    </div>
  `;
}

function renderCameraInspector(
  camera: CameraViewModel,
  detailTab: DetailTab,
  eventContent: TemplateResult | typeof nothing,
  archiveContent: TemplateResult | typeof nothing,
  onSelectDetailTab: (tab: DetailTab) => void,
): TemplateResult {
  return html`
    <div class="detail-header">
      <div class="detail-title">${displayCameraLabel(camera)}</div>
      <div class="muted">${camera.roomLabel}</div>
    </div>
    <div class="detail-tabs">
      ${renderSegmentButton("overview", "Overview", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("events", "Events", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("recordings", "Recordings", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("settings", "Settings", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
    </div>
    <div class="detail-main">
      ${detailTab === "overview"
        ? html`
            <div class="panel">
              <div class="panel-title">Camera Status</div>
              <div class="chip-row">
                <span class="badge info">${camera.kindLabel}</span>
                <span class="badge ${camera.online ? "success" : "critical"}">
                  ${camera.online ? "Online" : "Offline"}
                </span>
                <span class="badge ${camera.streamAvailable ? "success" : "warning"}">
                  ${camera.streamAvailable ? "Stream ready" : "Stream unavailable"}
                </span>
                <span class="badge ${camera.recordingActive ? "critical" : "info"}">
                  ${camera.recordingActive ? "NVR Recording" : "NVR Idle"}
                </span>
                ${camera.bridgeRecordingActive
                  ? html`<span class="badge warning">MP4 Clip Active</span>`
                  : nothing}
                ${camera.supportsPtz ? html`<span class="badge">PTZ</span>` : nothing}
              </div>
            </div>
            <div class="panel">
              <div class="panel-title">AI / Detection</div>
              <div class="chip-row">
                ${camera.detections.length === 0
                  ? html`<span class="muted">No active detections.</span>`
                  : repeat(
                      camera.detections,
                      (badge) => badge.key,
                      (badge) => html`<span class="badge ${badge.tone}">${badge.label}</span>`,
                    )}
              </div>
            </div>
            <div class="panel">
              <div class="panel-title">Capabilities</div>
              <div class="chip-row">
                ${camera.supportsPtzPan || camera.supportsPtzTilt
                  ? html`<span class="badge info">Pan/Tilt</span>`
                  : nothing}
                ${camera.supportsPtzZoom ? html`<span class="badge info">Zoom</span>` : nothing}
                ${camera.supportsPtzFocus ? html`<span class="badge info">Focus</span>` : nothing}
                ${camera.aux?.targets.length
                  ? html`
                      <span class="badge warning">
                        ${camera.aux.targets.length} aux target${camera.aux.targets.length === 1 ? "" : "s"}
                      </span>
                    `
                  : camera.supportsAux
                    ? html`<span class="badge warning">Aux available</span>`
                    : nothing}
                ${camera.supportsRecording
                  ? html`
                      <span class="badge ${camera.bridgeRecordingActive ? "warning" : "info"}">
                        Bridge MP4
                      </span>
                    `
                  : nothing}
                ${camera.recording
                  ? html`
                      <span class="badge ${camera.recording.active ? "critical" : "info"}">
                        NVR Recording State
                      </span>
                    `
                  : nothing}
                ${camera.archive?.searchUrl
                  ? html`<span class="badge success">Archive ready</span>`
                  : nothing}
              </div>
            </div>
            ${renderCameraControlAuthority(camera)}
          `
        : nothing}

      ${detailTab === "events" ? eventContent : nothing}

      ${detailTab === "recordings" ? archiveContent : nothing}

      ${detailTab === "settings"
        ? html`
            ${renderCameraControlAuthority(camera)}
            ${renderCameraStreamStatus(camera)}
            ${renderCameraStreamProfiles(camera)}
            ${renderCameraBridgeAdapter(camera)}
          `
        : nothing}
    </div>
  `;
}

function renderNvrDriveBreakdown(
  nvr: NvrViewModel,
  renderIcon: (icon: string) => TemplateResult,
): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">Drive Inventory</div>
      <div class="storage-drives inspector-storage-drives">
        ${nvr.disks.length > 0
          ? repeat(
              nvr.disks,
              (disk) => disk.deviceId,
              (disk) => html`
                <div class="storage-drive inspector-storage-drive">
                  <div class="storage-drive-head">
                    <span class="split-row">
                      <span class="sidebar-glyph" aria-hidden="true">
                        ${renderIcon("mdi:harddisk")}
                      </span>
                      <span class="sidebar-label">${disk.label}</span>
                    </span>
                    <span class="badge ${disk.healthy ? "success" : "critical"}">
                      ${disk.healthy ? "Healthy" : "Fault"}
                    </span>
                  </div>
                  <div class="progress">
                    <div
                      class="progress-bar"
                      style=${`width:${Math.max(0, Math.min(100, disk.usedPercent ?? 0))}%`}
                    ></div>
                  </div>
                  <div class="storage-drive-meta">
                    <span class="badge ${disk.online ? "success" : "critical"}">
                      ${disk.online ? "Online" : "Offline"}
                    </span>
                    <span class="badge info">
                      ${disk.usedPercent !== null ? `${Math.round(disk.usedPercent)}% used` : "Usage unknown"}
                    </span>
                    <span class="badge">
                      ${disk.stateText ?? "State unknown"}
                    </span>
                  </div>
                  <div class="storage-drive-meta">
                    <span class="sidebar-secondary">${disk.usedBytesText} / ${disk.totalBytesText}</span>
                    <span class="sidebar-secondary">${disk.stateText ?? "State unknown"}</span>
                  </div>
                </div>
              `,
            )
          : html`<div class="muted">No drive child devices discovered for this recorder.</div>`}
      </div>
    </div>
  `;
}

function renderCameraStreamStatus(camera: CameraViewModel): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">Stream Status</div>
      <div class="chip-row">
        <span class="badge ${camera.stream.available ? "success" : "critical"}">
          ${camera.stream.available ? "Live stream ready" : "Live stream unavailable"}
        </span>
        <span class="badge info">${camera.stream.profile}</span>
        <span class="badge">${camera.stream.resolution}</span>
        <span class="badge">${camera.stream.codec}</span>
        <span class="badge">${camera.stream.frameRate}</span>
        <span class="badge">${camera.stream.bitrate}</span>
        <span class="badge">${camera.stream.audioCodec}</span>
        ${camera.stream.recommendedProfile
          ? html`<span class="badge success">Recommended ${camera.stream.recommendedProfile}</span>`
          : nothing}
        ${camera.stream.onvifStreamUrl ? html`<span class="badge info">ONVIF stream ready</span>` : nothing}
        ${camera.stream.onvifSnapshotUrl ? html`<span class="badge info">ONVIF snapshot ready</span>` : nothing}
      </div>
      <div class="detail-inline-meta">
        ${camera.stream.preferredVideoProfile
          ? html`<span class="muted">Preferred profile: ${camera.stream.preferredVideoProfile}</span>`
          : nothing}
        ${camera.stream.preferredVideoSource
          ? html`<span class="muted">Preferred source: ${camera.stream.preferredVideoSource}</span>`
          : nothing}
        <span class="muted">
          ${camera.stream.source
            ? "Primary stream route is exposed."
            : "No primary stream route is exposed."}
        </span>
      </div>
    </div>
  `;
}

function renderCameraStreamProfiles(camera: CameraViewModel): TemplateResult | typeof nothing {
  if (camera.stream.profiles.length === 0) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">Stream Profiles</div>
      <div class="compact-list">
        ${repeat(
          camera.stream.profiles,
          (profile) => profile.key,
          (profile) => html`
            <div class="compact-card stream-profile-card">
              <div class="panel-title compact-card-head">
                <span class="sidebar-label">${profile.name}</span>
                ${profile.recommended ? html`<span class="badge success">Recommended</span>` : nothing}
              </div>
              <div class="chip-row">
                ${profile.subtype !== null ? html`<span class="badge">Subtype ${profile.subtype}</span>` : nothing}
                ${profile.resolution ? html`<span class="badge info">${profile.resolution}</span>` : nothing}
                ${profile.frameRate !== null ? html`<span class="badge">${profile.frameRate}</span>` : nothing}
                ${profile.rtspTransport ? html`<span class="badge">${profile.rtspTransport}</span>` : nothing}
              </div>
              <div class="chip-row">
                ${profile.streamUrl ? html`<span class="badge info">RTSP</span>` : nothing}
                ${profile.localHlsUrl ? html`<span class="badge success">HLS</span>` : nothing}
                ${profile.localMjpegUrl ? html`<span class="badge">MJPEG</span>` : nothing}
                ${profile.localWebRtcUrl ? html`<span class="badge warning">WebRTC</span>` : nothing}
              </div>
            </div>
          `,
        )}
      </div>
    </div>
  `;
}

function renderCameraBridgeAdapter(camera: CameraViewModel): TemplateResult | typeof nothing {
  if (camera.bridgeBaseUrl && camera.eventsUrl) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">Bridge Adapter</div>
      <div class="chip-row">
        <span class="badge ${camera.bridgeBaseUrl ? "success" : "warning"}">
          ${camera.bridgeBaseUrl ? "Bridge base URL available" : "Bridge base URL unavailable"}
        </span>
        <span class="badge ${camera.eventsUrl ? "success" : "warning"}">
          ${camera.eventsUrl ? "Event route available" : "Event route unavailable"}
        </span>
      </div>
      <div class="muted">
        Bridge diagnostics are only shown here when a required route is missing.
      </div>
    </div>
  `;
}

function renderCameraControlAuthority(
  camera: CameraViewModel,
): TemplateResult | typeof nothing {
  const showAudioAuthority = Boolean(camera.audioControlAuthority);
  const showAudioSemantic = Boolean(camera.audioControlSemantic);
  const showNvrWriteState = camera.nvrConfigWritable !== null;
  const showDirectIPC =
    camera.directIPCConfigured ||
    Boolean(camera.directIPCConfiguredIP) ||
    Boolean(camera.directIPCIP) ||
    Boolean(camera.directIPCModel);
  const showValidationNotes = camera.deviceKind === "nvr_channel" && camera.validationNotes.length > 0;

  if (
    !showAudioAuthority &&
    !showAudioSemantic &&
    !showNvrWriteState &&
    !showDirectIPC &&
    !showValidationNotes
  ) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">Control Authority</div>
      <div class="chip-row">
        ${showAudioAuthority
          ? html`
              <span class="badge ${audioAuthorityTone(camera.audioControlAuthority)}">
                Audio via ${audioAuthorityLabel(camera.audioControlAuthority)}
              </span>
            `
          : nothing}
        ${showAudioSemantic
          ? html`<span class="badge info">${audioSemanticLabel(camera.audioControlSemantic)}</span>`
          : nothing}
        ${showNvrWriteState
          ? html`
              <span class="badge ${camera.nvrConfigWritable ? "success" : "warning"}">
                NVR config writes ${camera.nvrConfigWritable ? "ready" : "blocked"}
              </span>
            `
          : nothing}
        ${showDirectIPC
          ? html`
              <span class="badge ${camera.directIPCConfigured ? "success" : "info"}">
                ${camera.directIPCConfigured ? "Direct IPC mapped" : "Direct IPC seen in inventory"}
              </span>
            `
          : nothing}
      </div>
      <div class="detail-inline-meta">
        ${camera.directIPCIP || camera.directIPCConfiguredIP
          ? html`
              <span class="muted">
                Direct IPC: ${camera.directIPCConfiguredIP ?? camera.directIPCIP}
                ${camera.directIPCModel ? ` (${camera.directIPCModel})` : ""}
              </span>
            `
          : nothing}
        ${camera.nvrConfigReason
          ? html`<span class="muted">NVR write probe: ${camera.nvrConfigReason}</span>`
          : nothing}
      </div>
      ${showValidationNotes
        ? html`
            <div class="chip-row">
              ${repeat(
                camera.validationNotes,
                (note) => note,
                (note) => html`<span class="badge warning">${note}</span>`,
              )}
            </div>
          `
        : nothing}
    </div>
  `;
}

function renderVtoInspector(
  vto: VtoViewModel,
  detailTab: DetailTab,
  eventContent: TemplateResult | typeof nothing,
  renderIcon: (icon: string) => TemplateResult,
  isBusy: (key: string) => boolean,
  onSelectDetailTab: (tab: DetailTab) => void,
  onVtoRangeChange: (
    event: Event,
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ) => Promise<void>,
  onVtoSwitchAction: (
    key: string,
    entityId: string,
    enabled: boolean,
    fallbackUrl: string | null,
    payloadKey: string,
  ) => Promise<void>,
  onVtoButtonAction: (
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ) => Promise<void>,
): TemplateResult {
  const outputVolumeAvailable = vto.hasOutputVolumeEntity || Boolean(vto.outputVolumeActionUrl);
  const inputVolumeAvailable = vto.hasInputVolumeEntity || Boolean(vto.inputVolumeActionUrl);
  const autoRecordAvailable = vto.hasAutoRecordEntity || Boolean(vto.autoRecordActionUrl);
  const externalUplinkAvailable = Boolean(
    vto.capabilities.enableExternalUplinkUrl || vto.capabilities.disableExternalUplinkUrl,
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
            ${renderVtoLockViews(vto, renderIcon, isBusy, onVtoButtonAction)}
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
                        vto.autoRecordEnabled ? "Auto Record On" : "Auto Record Off",
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
                          tone: vto.autoRecordEnabled ? "warning" : "neutral",
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
                            vto.intercom.externalUplinkEnabled ? "Disable External Uplink" : "Enable External Uplink",
                            vto.intercom.externalUplinkEnabled ? "mdi:upload-off-outline" : "mdi:upload-network-outline",
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
                              tone: vto.intercom.externalUplinkEnabled ? "warning" : "primary",
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

function audioAuthorityLabel(authority: string | null): string {
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

function audioAuthorityTone(
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

function audioSemanticLabel(semantic: string | null): string {
  switch (semantic?.trim().toLowerCase()) {
    case "stream_audio_enable":
      return "Stream audio toggle";
    default:
      return semantic?.trim() || "Audio control";
  }
}

function renderNvrSummaryChip(
  icon: string,
  label: string,
  value: string,
  tone: "info" | "success" | "warning" | "critical",
  renderIcon: (icon: string) => TemplateResult,
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

function renderVtoLockViews(
  vto: VtoViewModel,
  renderIcon: (icon: string) => TemplateResult,
  isBusy: (key: string) => boolean,
  onVtoButtonAction: (
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ) => Promise<void>,
): TemplateResult | typeof nothing {
  if (vto.locks.length === 0) {
    return nothing;
  }

  if (vto.locks.length === 1) {
    return renderSingleVtoLockView(vto.locks[0]!, renderIcon, isBusy, onVtoButtonAction);
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
          (lock) => renderVtoLockCard(lock, renderIcon, isBusy, onVtoButtonAction),
        )}
      </div>
    </div>
  `;
}

function renderSingleVtoLockView(
  lock: VtoLockViewModel,
  renderIcon: (icon: string) => TemplateResult,
  isBusy: (key: string) => boolean,
  onVtoButtonAction: (
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ) => Promise<void>,
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
              isBusy(actionKey) || (!lock.hasUnlockButtonEntity && !lock.unlockActionUrl),
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
  renderIcon: (icon: string) => TemplateResult,
  isBusy: (key: string) => boolean,
  onVtoButtonAction: (
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ) => Promise<void>,
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
              isBusy(actionKey) || (!lock.hasUnlockButtonEntity && !lock.unlockActionUrl),
          },
        )}
      </div>
    </div>
  `;
}

function renderVtoIntercomViews(
  vto: VtoViewModel,
  renderIcon: (icon: string) => TemplateResult,
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
