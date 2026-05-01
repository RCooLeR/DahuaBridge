import { parseChannelNumber } from "../ha/entity-id";
import type { BridgeEvent } from "../ha/bridge-events";
import type { SurveillancePanelCardConfig } from "../types/card-config";
import type { HassEntity, HomeAssistant } from "../types/home-assistant";
import {
  bridgeEventsToTimeline,
  collectCameraEvents,
  collectVtoEvents,
  mergeTimelineEvents,
  type TimelineEvent,
} from "./events";
import {
  discoverBridgeTopology,
  type CameraDeviceModel,
  type IpcModel,
  type NvrChannelModel,
  type NvrModel as DeviceNvrModel,
  type NvrDriveModel as DeviceNvrDriveModel,
  type VtoCallState,
  type VtoModel as DeviceVtoModel,
} from "./devices";
import type {
  CameraArchiveCapabilities,
} from "./archive";
import {
  buildTodayEventHeaderMetrics,
  type PanelTodayEventSummaryModel,
} from "./event-summary";
import type {
  CameraAuxActionName,
  CameraAuxActionTargetModel,
  CameraAuxCapabilities,
} from "./camera-aux";
import {
  buildBridgeEndpointUrl,
  normalizeBrowserBridgeUrl,
  rewriteBridgeUrl,
} from "../ha/bridge-url";
import type { RegistrySnapshot } from "../ha/registry";
import { formatBytes } from "../utils/format";

export interface HeaderMetric {
  label: string;
  value: string;
  tone: "neutral" | "success" | "warning" | "info" | "critical";
}

export interface DetectionBadge {
  key: string;
  label: string;
  icon: string;
  tone: "warning" | "info" | "critical";
}

export interface CameraViewModel {
  type: "camera";
  deviceKind: "nvr_channel" | "ipc";
  kindLabel: string;
  deviceId: string;
  rootDeviceId: string;
  channelNumber: number | null;
  label: string;
  roomLabel: string;
  cameraEntityId: string;
  cameraEntity?: HassEntity;
  online: boolean;
  streamAvailable: boolean;
  bridgeBaseUrl: string | null;
  eventsUrl: string | null;
  snapshotUrl: string | null;
  captureSnapshotUrl: string | null;
  stream: CameraStreamViewModel;
  detections: DetectionBadge[];
  supportsPtz: boolean;
  supportsPtzPan: boolean;
  supportsPtzTilt: boolean;
  supportsPtzZoom: boolean;
  supportsPtzFocus: boolean;
  supportsAux: boolean;
  supportsRecording: boolean;
  recordingActive: boolean;
  bridgeRecordingActive: boolean;
  ptzUrl: string | null;
  aux: CameraAuxViewModel | null;
  auxUrl: string | null;
  archive: CameraArchiveViewModel | null;
  recording: CameraRecordingViewModel | null;
  recordingUrl: string | null;
  recordingStartUrl: string | null;
  recordingStopUrl: string | null;
  recordingsUrl: string | null;
  resolution: string;
  codec: string;
  frameRate: string;
  bitrate: string;
  profile: string;
  audioCodec: string;
  microphoneAvailable: boolean;
  speakerAvailable: boolean;
  audioMuted: boolean;
  audioMuteSupported: boolean;
  audioMuteActionUrl: string | null;
  validationNotes: string[];
  audioControlAuthority: string | null;
  audioControlSemantic: string | null;
  nvrConfigWritable: boolean | null;
  nvrConfigReason: string | null;
  directIPCConfigured: boolean;
  directIPCConfiguredIP: string | null;
  directIPCIP: string | null;
  directIPCModel: string | null;
}

export interface CameraAuxTargetViewModel {
  key: string;
  label: string;
  url: string | null;
  parameterKey: string;
  parameterValue: string;
  outputKey: string;
  actions: CameraAuxActionName[];
  preferredAction: CameraAuxActionName | null;
  active: boolean | null;
  currentText: string | null;
  toggleSupported: boolean;
}

export interface CameraAuxViewModel {
  supported: boolean;
  url: string | null;
  outputs: string[];
  features: string[];
  targets: CameraAuxTargetViewModel[];
}

export interface CameraRecordingViewModel {
  supported: boolean;
  active: boolean;
  mode: string | null;
  url: string | null;
}

export interface CameraArchiveViewModel {
  supported: boolean;
  searchUrl: string | null;
  playbackUrl: string | null;
  channel: number | null;
  defaultLimit: number;
}

export interface CameraStreamProfileViewModel {
  key: string;
  name: string;
  streamUrl: string | null;
  localMjpegUrl: string | null;
  localHlsUrl: string | null;
  localWebRtcUrl: string | null;
  subtype: number | null;
  rtspTransport: string | null;
  frameRate: number | null;
  resolution: string | null;
  recommended: boolean;
}

export interface CameraStreamViewModel {
  available: boolean;
  source: string | null;
  snapshotUrl: string | null;
  localIntercomUrl: string | null;
  onvifStreamUrl: string | null;
  onvifSnapshotUrl: string | null;
  recommendedProfile: string | null;
  preferredVideoProfile: string | null;
  preferredVideoSource: string | null;
  resolution: string;
  codec: string;
  frameRate: string;
  bitrate: string;
  profile: string;
  audioCodec: string;
  profiles: CameraStreamProfileViewModel[];
}

export interface RoomViewModel {
  label: string;
  channels: CameraViewModel[];
}

export interface NvrDiskViewModel {
  deviceId: string;
  label: string;
  stateText: string | null;
  usedPercent: number | null;
  totalBytesText: string;
  usedBytesText: string;
  healthy: boolean;
  online: boolean;
}

export interface NvrViewModel {
  deviceId: string;
  label: string;
  roomLabel: string;
  online: boolean;
  bridgeBaseUrl: string | null;
  eventsUrl: string | null;
  rooms: RoomViewModel[];
  disks: NvrDiskViewModel[];
  storageUsedPercent: number | null;
  storageText: string;
  recordingActive: boolean;
  healthy: boolean;
  nvrConfigWritable: boolean | null;
  nvrConfigReason: string | null;
}

export interface VtoViewModel {
  type: "vto";
  deviceId: string;
  label: string;
  roomLabel: string;
  cameraEntityId: string;
  cameraEntity?: HassEntity;
  online: boolean;
  bridgeBaseUrl: string | null;
  eventsUrl: string | null;
  snapshotUrl: string | null;
  captureSnapshotUrl: string | null;
  streamAvailable: boolean;
  stream: CameraStreamViewModel;
  bridgeRecordingActive: boolean;
  recordingStartUrl: string | null;
  recordingStopUrl: string | null;
  recordingsUrl: string | null;
  doorbell: boolean;
  callActive: boolean;
  accessActive: boolean;
  tamper: boolean;
  callState: VtoCallState;
  callStateText: string;
  lastCallSource: string;
  lastCallStartedAt: string;
  lastCallEndedAt: string;
  lastCallDuration: string;
  outputVolume: number | null;
  inputVolume: number | null;
  muted: boolean;
  autoRecordEnabled: boolean;
  answerButtonEntityId: string;
  answerActionUrl: string | null;
  hasAnswerButtonEntity: boolean;
  hangupButtonEntityId: string;
  hangupActionUrl: string | null;
  hasHangupButtonEntity: boolean;
  unlockButtonEntityId: string;
  unlockActionUrl: string | null;
  hasUnlockButtonEntity: boolean;
  outputVolumeEntityId: string;
  outputVolumeActionUrl: string | null;
  hasOutputVolumeEntity: boolean;
  inputVolumeEntityId: string;
  inputVolumeActionUrl: string | null;
  hasInputVolumeEntity: boolean;
  mutedEntityId: string;
  mutedActionUrl: string | null;
  hasMutedEntity: boolean;
  autoRecordEntityId: string;
  autoRecordActionUrl: string | null;
  hasAutoRecordEntity: boolean;
  lockCount: number;
  alarmCount: number;
  locks: VtoLockViewModel[];
  alarms: VtoAlarmViewModel[];
  intercom: VtoIntercomViewModel;
  capabilities: VtoCapabilityViewModel;
}

export interface VtoLockViewModel {
  deviceId: string;
  label: string;
  roomLabel: string;
  slot: number | null;
  online: boolean;
  healthy: boolean;
  stateText: string | null;
  sensorEnabled: boolean;
  lockMode: string | null;
  unlockHoldInterval: string | null;
  unlockButtonEntityId: string;
  unlockActionUrl: string | null;
  hasUnlockButtonEntity: boolean;
  modelText: string | null;
}

export interface VtoAlarmViewModel {
  deviceId: string;
  label: string;
  roomLabel: string;
  slot: number | null;
  online: boolean;
  active: boolean;
  enabled: boolean;
  senseMethod: string | null;
  modelText: string | null;
}

export interface VtoIntercomViewModel {
  bridgeSessionActive: boolean;
  bridgeSessionCount: number | null;
  externalUplinkEnabled: boolean;
  bridgeUplinkActive: boolean;
  bridgeUplinkCodec: string | null;
  bridgeUplinkPackets: number | null;
  bridgeForwardedPackets: number | null;
  bridgeForwardErrors: number | null;
  configuredExternalUplinkTargetCount: number | null;
}

export interface VtoCapabilityViewModel {
  answerSupported: boolean;
  hangupSupported: boolean;
  unlockSupported: boolean;
  resetSupported: boolean;
  browserMicrophoneSupported: boolean;
  bridgeAudioUplinkSupported: boolean;
  bridgeAudioOutputSupported: boolean;
  externalAudioExportSupported: boolean;
  outputVolumeSupported: boolean;
  inputVolumeSupported: boolean;
  muteSupported: boolean;
  recordingSupported: boolean;
  talkbackSupported: boolean;
  fullCallAcceptanceSupported: boolean;
  resetUrl: string | null;
  enableExternalUplinkUrl: string | null;
  disableExternalUplinkUrl: string | null;
  validationNotes: string[];
}

export type SidebarFilter = "all" | "alerts" | "nvr" | "vto";

export interface SidebarItem {
  id: string;
  label: string;
  secondary: string;
  kind: "camera" | "vto" | "accessory" | "nvr";
  selected: boolean;
  highlighted: boolean;
  badge?: string;
}

export interface PanelSelection {
  kind: "overview" | "camera" | "vto" | "nvr";
  deviceId?: string;
}

export interface PanelModel {
  title: string;
  subtitle: string;
  headerMetrics: HeaderMetric[];
  cameras: CameraViewModel[];
  nvrs: NvrViewModel[];
  vtos: VtoViewModel[];
  vto?: VtoViewModel;
  eventFeed: TimelineEvent[];
  timeline: TimelineEvent[];
  selectedNvr?: NvrViewModel;
  selectedCamera?: CameraViewModel;
  selectedVto?: VtoViewModel;
  selection: PanelSelection;
  sidebarItems: SidebarItem[];
}

export function buildPanelModel(
  hass: HomeAssistant,
  config: SurveillancePanelCardConfig,
  selection: PanelSelection,
  bridgeEvents?: BridgeEvent[] | null,
  eventLookbackHoursOverride?: number,
  registrySnapshot?: RegistrySnapshot | null,
  todayEventSummary?: PanelTodayEventSummaryModel | null,
): PanelModel {
  const browserBridgeUrl = normalizeBrowserBridgeUrl(config.browser_bridge_url);
  const topology = discoverBridgeTopology(hass, registrySnapshot);
  const cameras = topology.cameras
    .map((camera) => buildCameraViewModel(camera, browserBridgeUrl))
    .sort(compareCameraViewModels);
  const nvrs = topology.nvrs
    .map((nvr) => buildNvrViewModel(nvr, browserBridgeUrl))
    .sort((left, right) => left.label.localeCompare(right.label));
  const vtos = buildVtoViewModels(hass, topology.vtos, config.vto, browserBridgeUrl);
  const vto = vtos[0];

  const selectedCamera =
    selection.kind === "camera" && selection.deviceId
      ? cameras.find((camera) => camera.deviceId === selection.deviceId)
      : undefined;
  const selectedNvr =
    selection.kind === "nvr" && selection.deviceId
      ? nvrs.find((nvr) => nvr.deviceId === selection.deviceId)
      : undefined;
  const selectedVto =
    selection.kind === "vto" && selection.deviceId
      ? vtos.find((entry) => entry.deviceId === selection.deviceId)
      : undefined;

  const onlineCount =
    cameras.filter((camera) => camera.online).length + vtos.filter((entry) => entry.online).length;
  const headerMetrics: HeaderMetric[] = [
    {
      label: "Cameras Online",
      value: `${onlineCount}/${cameras.length + vtos.length}`,
      tone: onlineCount > 0 ? "success" : "critical",
    },
  ];
  const eventHeaderMetrics = buildTodayEventHeaderMetrics(todayEventSummary ?? null);
  if (eventHeaderMetrics) {
    headerMetrics.push(...eventHeaderMetrics);
  } else {
    const motionCount = cameras.filter((camera) =>
      camera.detections.some((detection) => detection.key === "motion"),
    ).length;
    const humanCount = cameras.filter((camera) =>
      camera.detections.some((detection) => detection.key === "human"),
    ).length;
    const vehicleCount = cameras.filter((camera) =>
      camera.detections.some((detection) => detection.key === "vehicle"),
    ).length;
    headerMetrics.push(
      {
        label: "Motion",
        value: `${motionCount}`,
        tone: motionCount > 0 ? "warning" : "neutral",
      },
      {
        label: "Human",
        value: `${humanCount}`,
        tone: humanCount > 0 ? "info" : "neutral",
      },
      {
        label: "Vehicle",
        value: `${vehicleCount}`,
        tone: vehicleCount > 0 ? "info" : "neutral",
      },
    );
  }

  if (nvrs.length > 0) {
    const healthy = nvrs.every((nvr) => nvr.healthy);
    headerMetrics.push({
      label: "NVR Health",
      value: healthy ? "Healthy" : "Attention",
      tone: healthy ? "success" : "warning",
    });
  }

  const vtoHeaderMetric = buildVtoHeaderMetric(vtos);
  if (vtoHeaderMetric) {
    headerMetrics.push(vtoHeaderMetric);
  }

  const lookbackMs =
    (eventLookbackHoursOverride ?? config.event_lookback_hours ?? 12) * 60 * 60 * 1000;
  const syntheticTimeline = [
    ...cameras.flatMap((camera) =>
      collectCameraEvents(
        hass,
        camera.deviceId,
        camera.label,
        camera.roomLabel,
        camera.deviceKind,
        lookbackMs,
      ),
    ),
    ...vtos.flatMap((entry) =>
      collectVtoEvents(hass, entry.deviceId, entry.label, entry.roomLabel, lookbackMs),
    ),
  ].sort((left, right) => right.timestamp - left.timestamp);

  const contextByDeviceId = buildEventContextLookup(cameras, nvrs, vtos);
  const bridgeTimeline = bridgeEvents
    ? bridgeEventsToTimeline(bridgeEvents, contextByDeviceId, lookbackMs)
    : [];
  const eventFeed = filterTimelineForSelection(
    mergeTimelineEvents(bridgeTimeline, syntheticTimeline),
    selection,
  );
  const timeline = eventFeed.slice(0, config.max_events ?? 14);

  return {
    title: config.title ?? "DahuaBridge Surveillance",
    subtitle: config.subtitle ?? "Full-panel command center",
    headerMetrics,
    cameras,
    nvrs,
    vtos,
    vto,
    eventFeed,
    timeline,
    selectedNvr,
    selectedCamera,
    selectedVto,
    selection,
    sidebarItems: buildSidebarItems(cameras, nvrs, vtos, selection),
  };
}

function buildCameraViewModel(
  camera: CameraDeviceModel,
  browserBridgeUrl: string | null,
): CameraViewModel {
  const aux = buildCameraAuxViewModel(camera.capabilities.aux ?? null, browserBridgeUrl);
  const recording = buildCameraRecordingViewModel(
    camera.capabilities.recording ?? null,
    browserBridgeUrl,
  );
  const muteFeature = bridgeFeatureByKey(camera.features, "mute");
  return {
    type: "camera",
    deviceKind: camera.kind,
    kindLabel: camera.kind === "ipc" ? "IPC Camera" : "NVR Channel",
    deviceId: camera.deviceId,
    rootDeviceId: camera.rootDeviceId,
    channelNumber: camera.kind === "nvr_channel" ? camera.channelNumber : null,
    label: camera.label,
    roomLabel: camera.roomLabel,
    cameraEntityId: camera.cameraEntityId,
    cameraEntity: camera.cameraEntity,
    online: camera.online,
    streamAvailable: camera.media.streamAvailable,
    bridgeBaseUrl: rewriteBridgeUrl(camera.bridgeBaseUrl, browserBridgeUrl),
    eventsUrl: rewriteBridgeUrl(camera.eventsUrl, browserBridgeUrl),
    snapshotUrl: rewriteBridgeUrl(camera.media.snapshotUrl, browserBridgeUrl),
    captureSnapshotUrl: rewriteBridgeUrl(camera.media.capture?.snapshotUrl ?? null, browserBridgeUrl),
    stream: buildCameraStreamViewModel(camera, browserBridgeUrl),
    detections: buildDetectionBadges(camera),
    supportsPtz: camera.capabilities.ptz?.supported === true,
    supportsPtzPan: camera.capabilities.ptz?.pan === true,
    supportsPtzTilt: camera.capabilities.ptz?.tilt === true,
    supportsPtzZoom: camera.capabilities.ptz?.zoom === true,
    supportsPtzFocus: camera.capabilities.ptz?.focus === true,
    supportsAux: aux?.supported === true,
    supportsRecording:
      Boolean(camera.media.capture?.startRecordingUrl) ||
      Boolean(camera.media.capture?.stopRecordingUrl),
    recordingActive: recording?.active === true,
    bridgeRecordingActive: camera.media.capture?.recordingActive === true,
    ptzUrl: rewriteBridgeUrl(camera.capabilities.ptz?.url ?? null, browserBridgeUrl),
    aux,
    auxUrl: aux?.url ?? null,
    archive: buildCameraArchiveViewModel(camera.capabilities.archive, browserBridgeUrl),
    recording,
    recordingUrl: recording?.url ?? null,
    recordingStartUrl: rewriteBridgeUrl(
      camera.media.capture?.startRecordingUrl ?? null,
      browserBridgeUrl,
    ),
    recordingStopUrl: rewriteBridgeUrl(
      camera.media.capture?.stopRecordingUrl ?? null,
      browserBridgeUrl,
    ),
    recordingsUrl: rewriteBridgeUrl(camera.media.capture?.recordingsUrl ?? null, browserBridgeUrl),
    resolution: camera.media.resolution,
    codec: camera.media.codec,
    frameRate: camera.media.frameRate,
    bitrate: camera.media.bitrate,
    profile: camera.media.profile,
    audioCodec: camera.media.audioCodec,
    microphoneAvailable: camera.media.audioCodec.trim().length > 0,
    speakerAvailable: camera.capabilities.audio.playback.supported,
    audioMuted: muteFeature?.active === true,
    audioMuteSupported:
      camera.capabilities.audio.mute === true && !!rewriteBridgeUrl(muteFeature?.url ?? null, browserBridgeUrl),
    audioMuteActionUrl: rewriteBridgeUrl(muteFeature?.url ?? null, browserBridgeUrl),
    validationNotes: [...camera.capabilities.validationNotes],
    audioControlAuthority: camera.diagnostics.controlAudioAuthority,
    audioControlSemantic: camera.diagnostics.controlAudioSemantic,
    nvrConfigWritable: camera.diagnostics.nvrConfigWritable,
    nvrConfigReason: camera.diagnostics.nvrConfigReason,
    directIPCConfigured: camera.diagnostics.directIPCConfigured === true,
    directIPCConfiguredIP: camera.diagnostics.directIPCConfiguredIP,
    directIPCIP: camera.diagnostics.directIPCIP,
    directIPCModel: camera.diagnostics.directIPCModel,
  };
}

function buildCameraAuxViewModel(
  aux: CameraAuxCapabilities | null,
  browserBridgeUrl: string | null,
): CameraAuxViewModel | null {
  if (!aux) {
    return null;
  }

  return {
    supported: aux.supported,
    url: rewriteBridgeUrl(aux.url, browserBridgeUrl),
    outputs: [...aux.outputs],
    features: [...aux.features],
    targets: aux.targets.map((target) => buildCameraAuxTargetViewModel(target, browserBridgeUrl)),
  };
}

function buildCameraAuxTargetViewModel(
  target: CameraAuxActionTargetModel,
  browserBridgeUrl: string | null,
): CameraAuxTargetViewModel {
  return {
    key: target.key,
    label: target.label,
    url: rewriteBridgeUrl(target.url, browserBridgeUrl),
    parameterKey: target.parameterKey,
    parameterValue: target.parameterValue,
    outputKey: target.outputKey,
    actions: [...target.actions],
    preferredAction: target.preferredAction,
    active: target.active,
    currentText: target.currentText,
    toggleSupported: target.toggleSupported,
  };
}

function buildCameraRecordingViewModel(
  recording: CameraDeviceModel["capabilities"]["recording"] | null,
  browserBridgeUrl: string | null,
): CameraRecordingViewModel | null {
  if (!recording) {
    return null;
  }

  return {
    supported: recording.supported,
    active: recording.active,
    mode: recording.mode,
    url: rewriteBridgeUrl(recording.url, browserBridgeUrl),
  };
}

function bridgeFeatureByKey(
  features: CameraDeviceModel["features"],
  key: string,
): CameraDeviceModel["features"][number] | null {
  for (const feature of features) {
    if (feature.key === key) {
      return feature;
    }
  }
  return null;
}

function buildCameraStreamViewModel(
  camera: { media: CameraDeviceModel["media"] },
  browserBridgeUrl: string | null,
): CameraStreamViewModel {
  return {
    available: camera.media.streamAvailable,
    source: rewriteBridgeUrl(camera.media.streamSource, browserBridgeUrl),
    snapshotUrl: rewriteBridgeUrl(camera.media.snapshotUrl, browserBridgeUrl),
    localIntercomUrl: rewriteBridgeUrl(camera.media.localIntercomUrl, browserBridgeUrl),
    onvifStreamUrl: rewriteBridgeUrl(camera.media.onvifStreamUrl, browserBridgeUrl),
    onvifSnapshotUrl: rewriteBridgeUrl(camera.media.onvifSnapshotUrl, browserBridgeUrl),
    recommendedProfile: camera.media.recommendedProfile,
    preferredVideoProfile: camera.media.preferredVideoProfile,
    preferredVideoSource: camera.media.preferredVideoSource,
    resolution: camera.media.resolution,
    codec: camera.media.codec,
    frameRate: camera.media.frameRate,
    bitrate: camera.media.bitrate,
    profile: camera.media.profile,
    audioCodec: camera.media.audioCodec,
    profiles: Object.entries(camera.media.profiles)
      .map(([key, profile]) => ({
        key,
        name: streamProfileDisplayName(key, profile.name),
        streamUrl: rewriteBridgeUrl(profile.streamUrl, browserBridgeUrl),
        localMjpegUrl: rewriteBridgeUrl(profile.localMjpegUrl, browserBridgeUrl),
        localHlsUrl: rewriteBridgeUrl(profile.localHlsUrl, browserBridgeUrl),
        localWebRtcUrl: rewriteBridgeUrl(profile.localWebRtcUrl, browserBridgeUrl),
        subtype: profile.subtype,
        rtspTransport: profile.rtspTransport,
        frameRate: profile.frameRate,
        resolution:
          profile.sourceWidth !== null && profile.sourceHeight !== null
            ? `${profile.sourceWidth}x${profile.sourceHeight}`
            : null,
        recommended: profile.recommended,
      }))
      .sort(
        (left, right) =>
          streamProfileSortRank(left.key) - streamProfileSortRank(right.key) ||
          Number(right.recommended) - Number(left.recommended) ||
          left.name.localeCompare(right.name),
      ),
  };
}

function streamProfileDisplayName(key: string, fallback: string | null): string {
  switch (key.trim().toLowerCase()) {
    case "quality":
      return "Quality (Main Stream)";
    case "default":
      return "Default (Main Stream)";
    case "stable":
      return "Stable (Substream)";
    case "substream":
      return "Substream (Native)";
    default:
      return fallback?.trim() || key.trim();
  }
}

function streamProfileSortRank(key: string): number {
  switch (key.trim().toLowerCase()) {
    case "quality":
      return 0;
    case "default":
      return 1;
    case "stable":
      return 2;
    case "substream":
      return 3;
    default:
      return 4;
  }
}

function buildNvrViewModel(
  nvr: DeviceNvrModel,
  browserBridgeUrl: string | null,
): NvrViewModel {
  const usedBytesTotal = sumNullable(nvr.drives.map((drive) => drive.usedBytes));
  const rooms = nvr.roomGroups.map((roomGroup) => ({
    label: roomGroup.label,
    channels: roomGroup.channels
      .map((channel) => buildCameraViewModel(channel, browserBridgeUrl))
      .sort(compareCameraViewModels),
  }));

  return {
    deviceId: nvr.deviceId,
    label: nvr.label,
    roomLabel: nvr.roomLabel,
    online: nvr.online,
    bridgeBaseUrl: rewriteBridgeUrl(nvr.bridgeBaseUrl, browserBridgeUrl),
    eventsUrl: rewriteBridgeUrl(nvr.eventsUrl, browserBridgeUrl),
    rooms,
    disks: nvr.drives.map(buildNvrDiskViewModel),
    storageUsedPercent: nvr.storageUsedPercent,
    storageText:
      usedBytesTotal !== null && nvr.totalBytes !== null
        ? `${formatBytes(usedBytesTotal)} / ${formatBytes(nvr.totalBytes)} used${
            nvr.storageUsedPercent !== null ? ` (${Math.round(nvr.storageUsedPercent)}%)` : ""
          }`
        : nvr.totalBytes !== null
          ? `${formatBytes(nvr.totalBytes)} total`
          : nvr.storageUsedPercent !== null
            ? `${Math.round(nvr.storageUsedPercent)}% used`
          : nvr.drives.length > 0
            ? `${nvr.drives.length} drives`
            : "Storage unknown",
    recordingActive: nvr.recordingActive,
    healthy: nvr.healthy,
    nvrConfigWritable: nvr.diagnostics.nvrConfigWritable,
    nvrConfigReason: nvr.diagnostics.nvrConfigReason,
  };
}

function buildNvrDiskViewModel(disk: DeviceNvrDriveModel): NvrDiskViewModel {
  const normalizedStorage = normalizeStorageValues(
    disk.totalBytes,
    disk.usedBytes,
    disk.usedPercent,
  );
  return {
    deviceId: disk.deviceId,
    label: disk.label,
    stateText: disk.stateText,
    usedPercent: normalizedStorage.usedPercent,
    totalBytesText: formatBytes(normalizedStorage.totalBytes),
    usedBytesText: formatBytes(normalizedStorage.usedBytes),
    healthy: disk.healthy,
    online: disk.online,
  };
}

function buildCameraArchiveViewModel(
  archive: CameraArchiveCapabilities,
  browserBridgeUrl: string | null,
): CameraArchiveViewModel | null {
  if (!archive.supported && !archive.search.supported && !archive.playback.supported) {
    return null;
  }

  return {
    supported: archive.supported,
    searchUrl: rewriteBridgeUrl(archive.search.url, browserBridgeUrl),
    playbackUrl: rewriteBridgeUrl(archive.playback.url, browserBridgeUrl),
    channel: archive.search.channel ?? archive.playback.channel,
    defaultLimit: archive.search.defaultLimit,
  };
}

function buildVtoViewModel(
  vto: DeviceVtoModel,
  browserBridgeUrl: string | null,
): VtoViewModel {
  const firstLock = vto.locks[0];
  const locks = vto.locks.map((lock) => buildVtoLockViewModel(lock, browserBridgeUrl));
  const alarms = vto.alarms.map(buildVtoAlarmViewModel);
  return {
    type: "vto",
    deviceId: vto.deviceId,
    label: vto.label,
    roomLabel: vto.roomLabel,
    cameraEntityId: vto.cameraEntityId,
    cameraEntity: vto.cameraEntity,
    online: vto.online,
    bridgeBaseUrl: rewriteBridgeUrl(vto.bridgeBaseUrl, browserBridgeUrl),
    eventsUrl: rewriteBridgeUrl(vto.eventsUrl, browserBridgeUrl),
    snapshotUrl: rewriteBridgeUrl(vto.media.snapshotUrl, browserBridgeUrl),
    captureSnapshotUrl: rewriteBridgeUrl(vto.media.capture?.snapshotUrl ?? null, browserBridgeUrl),
    streamAvailable: vto.media.streamAvailable,
    stream: buildCameraStreamViewModel(vto, browserBridgeUrl),
    bridgeRecordingActive: vto.media.capture?.recordingActive === true,
    recordingStartUrl: rewriteBridgeUrl(
      vto.media.capture?.startRecordingUrl ?? null,
      browserBridgeUrl,
    ),
    recordingStopUrl: rewriteBridgeUrl(
      vto.media.capture?.stopRecordingUrl ?? null,
      browserBridgeUrl,
    ),
    recordingsUrl: rewriteBridgeUrl(vto.media.capture?.recordingsUrl ?? null, browserBridgeUrl),
    doorbell: vto.doorbell,
    callActive: vto.callActive,
    accessActive: vto.accessActive,
    tamper: vto.tamper,
    callState: vto.callState,
    callStateText: vto.callStateText,
    lastCallSource: vto.lastCallSource,
    lastCallStartedAt: vto.lastCallStartedAt,
    lastCallEndedAt: vto.lastCallEndedAt,
    lastCallDuration: String(vto.lastCallDurationSeconds ?? 0),
    outputVolume: vto.outputVolume,
    inputVolume: vto.inputVolume,
    muted: vto.muted,
    autoRecordEnabled: vto.autoRecordEnabled,
    answerButtonEntityId: vto.answerButtonEntityId,
    answerActionUrl: rewriteBridgeUrl(vto.intercom?.answerUrl ?? null, browserBridgeUrl),
    hasAnswerButtonEntity: vto.hasAnswerButtonEntity,
    hangupButtonEntityId: vto.hangupButtonEntityId,
    hangupActionUrl: rewriteBridgeUrl(vto.intercom?.hangupUrl ?? null, browserBridgeUrl),
    hasHangupButtonEntity: vto.hasHangupButtonEntity,
    unlockButtonEntityId: firstLock?.unlockButtonEntityId ?? "",
    unlockActionUrl: rewriteBridgeUrl(firstLock?.unlockActionUrl ?? null, browserBridgeUrl),
    hasUnlockButtonEntity: firstLock?.hasUnlockButtonEntity ?? false,
    outputVolumeEntityId: vto.outputVolumeEntityId,
    outputVolumeActionUrl: rewriteBridgeUrl(
      vto.intercom?.outputVolumeUrl ?? null,
      browserBridgeUrl,
    ),
    hasOutputVolumeEntity: vto.hasOutputVolumeEntity,
    inputVolumeEntityId: vto.inputVolumeEntityId,
    inputVolumeActionUrl: rewriteBridgeUrl(
      vto.intercom?.inputVolumeUrl ?? null,
      browserBridgeUrl,
    ),
    hasInputVolumeEntity: vto.hasInputVolumeEntity,
    mutedEntityId: vto.mutedEntityId,
    mutedActionUrl: rewriteBridgeUrl(vto.intercom?.muteUrl ?? null, browserBridgeUrl),
    hasMutedEntity: vto.hasMutedEntity,
    autoRecordEntityId: vto.autoRecordEntityId,
    autoRecordActionUrl: rewriteBridgeUrl(
      vto.intercom?.recordingUrl ?? null,
      browserBridgeUrl,
    ),
    hasAutoRecordEntity: vto.hasAutoRecordEntity,
    lockCount: vto.locks.length,
    alarmCount: vto.alarms.length,
    locks,
    alarms,
    intercom: {
      bridgeSessionActive: vto.intercom?.bridgeSessionActive === true,
      bridgeSessionCount: vto.intercom?.bridgeSessionCount ?? null,
      externalUplinkEnabled: vto.intercom?.externalUplinkEnabled === true,
      bridgeUplinkActive: vto.intercom?.bridgeUplinkActive === true,
      bridgeUplinkCodec: vto.intercom?.bridgeUplinkCodec ?? null,
      bridgeUplinkPackets: vto.intercom?.bridgeUplinkPackets ?? null,
      bridgeForwardedPackets: vto.intercom?.bridgeForwardedPackets ?? null,
      bridgeForwardErrors: vto.intercom?.bridgeForwardErrors ?? null,
      configuredExternalUplinkTargetCount:
        vto.intercom?.configuredExternalUplinkTargetCount ?? null,
    },
    capabilities: {
      answerSupported: vto.vtoCapabilities.answerSupported,
      hangupSupported: vto.vtoCapabilities.hangupSupported,
      unlockSupported: vto.vtoCapabilities.unlockSupported,
      resetSupported: vto.vtoCapabilities.resetSupported,
      browserMicrophoneSupported: vto.vtoCapabilities.browserMicrophoneSupported,
      bridgeAudioUplinkSupported: vto.vtoCapabilities.bridgeAudioUplinkSupported,
      bridgeAudioOutputSupported: vto.vtoCapabilities.bridgeAudioOutputSupported,
      externalAudioExportSupported: vto.vtoCapabilities.externalAudioExportSupported,
      outputVolumeSupported: vto.vtoCapabilities.outputVolumeSupported,
      inputVolumeSupported: vto.vtoCapabilities.inputVolumeSupported,
      muteSupported: vto.vtoCapabilities.muteSupported,
      recordingSupported: vto.vtoCapabilities.recordingSupported,
      talkbackSupported: vto.vtoCapabilities.talkbackSupported,
      fullCallAcceptanceSupported: vto.vtoCapabilities.fullCallAcceptanceSupported,
      resetUrl: rewriteBridgeUrl(vto.vtoCapabilities.resetUrl, browserBridgeUrl),
      enableExternalUplinkUrl: rewriteBridgeUrl(
        vto.vtoCapabilities.enableExternalUplinkUrl,
        browserBridgeUrl,
      ),
      disableExternalUplinkUrl: rewriteBridgeUrl(
        vto.vtoCapabilities.disableExternalUplinkUrl,
        browserBridgeUrl,
      ),
      validationNotes: [...vto.vtoCapabilities.validationNotes],
    },
  };
}

function buildVtoLockViewModel(
  lock: DeviceVtoModel["locks"][number],
  browserBridgeUrl: string | null,
): VtoLockViewModel {
  return {
    deviceId: lock.deviceId,
    label: lock.label,
    roomLabel: lock.roomLabel,
    slot: lock.slot,
    online: lock.online,
    healthy: lock.online && lock.sensorEnabled,
    stateText: lock.stateText,
    sensorEnabled: lock.sensorEnabled,
    lockMode: lock.lockMode,
    unlockHoldInterval: lock.unlockHoldInterval,
    unlockButtonEntityId: lock.unlockButtonEntityId,
    unlockActionUrl: rewriteBridgeUrl(lock.unlockActionUrl, browserBridgeUrl),
    hasUnlockButtonEntity: lock.hasUnlockButtonEntity,
    modelText: lock.metadata.model,
  };
}

function buildVtoAlarmViewModel(
  alarm: DeviceVtoModel["alarms"][number],
): VtoAlarmViewModel {
  return {
    deviceId: alarm.deviceId,
    label: alarm.label,
    roomLabel: alarm.roomLabel,
    slot: alarm.slot,
    online: alarm.online,
    active: alarm.active,
    enabled: alarm.enabled,
    senseMethod: alarm.senseMethod,
    modelText: alarm.metadata.model,
  };
}

function buildVtoViewModels(
  hass: HomeAssistant,
  vtos: DeviceVtoModel[],
  overrides?: NonNullable<SurveillancePanelCardConfig["vto"]>,
  browserBridgeUrl?: string | null,
): VtoViewModel[] {
  const preferredVtoId = stringValue(overrides?.device_id);
  const ordered = [...vtos].sort((left, right) => {
    if (preferredVtoId) {
      if (left.deviceId === preferredVtoId && right.deviceId !== preferredVtoId) {
        return -1;
      }
      if (right.deviceId === preferredVtoId && left.deviceId !== preferredVtoId) {
        return 1;
      }
    }
    return left.label.localeCompare(right.label);
  });

  return ordered.map((vto) => {
    const viewModel = buildVtoViewModel(vto, browserBridgeUrl ?? null);
    if (!preferredVtoId || vto.deviceId !== preferredVtoId) {
      return viewModel;
    }

    return {
      ...viewModel,
      label: overrides?.label ?? viewModel.label,
      unlockButtonEntityId: overrides?.lock_button_entity ?? viewModel.unlockButtonEntityId,
      hasUnlockButtonEntity:
        hass.states[overrides?.lock_button_entity ?? viewModel.unlockButtonEntityId] !== undefined,
      outputVolumeEntityId: overrides?.output_volume_entity ?? viewModel.outputVolumeEntityId,
      hasOutputVolumeEntity:
        hass.states[overrides?.output_volume_entity ?? viewModel.outputVolumeEntityId] !== undefined,
      inputVolumeEntityId: overrides?.input_volume_entity ?? viewModel.inputVolumeEntityId,
      hasInputVolumeEntity:
        hass.states[overrides?.input_volume_entity ?? viewModel.inputVolumeEntityId] !== undefined,
      mutedEntityId: overrides?.muted_entity ?? viewModel.mutedEntityId,
      hasMutedEntity:
        hass.states[overrides?.muted_entity ?? viewModel.mutedEntityId] !== undefined,
      autoRecordEntityId: overrides?.auto_record_entity ?? viewModel.autoRecordEntityId,
      hasAutoRecordEntity:
        hass.states[overrides?.auto_record_entity ?? viewModel.autoRecordEntityId] !== undefined,
      locks:
        overrides?.lock_button_entity && viewModel.locks.length > 0
          ? viewModel.locks.map((lock, index) =>
              index === 0
                ? {
                    ...lock,
                    unlockButtonEntityId: overrides.lock_button_entity ?? lock.unlockButtonEntityId,
                    hasUnlockButtonEntity:
                      hass.states[
                        overrides.lock_button_entity ?? lock.unlockButtonEntityId
                      ] !== undefined,
                  }
                : lock,
            )
          : viewModel.locks,
    };
  });
}

function buildSidebarItems(
  cameras: CameraViewModel[],
  nvrs: NvrViewModel[],
  vtos: VtoViewModel[],
  selection: PanelSelection,
): SidebarItem[] {
  const items: SidebarItem[] = [];
  const nvrDeviceIds = new Set(nvrs.map((nvr) => nvr.deviceId));

  for (const vto of vtos) {
    items.push({
      id: vto.deviceId,
      label: vto.label,
      secondary: "Door Station",
      kind: "vto",
      selected: selection.kind === "vto" && selection.deviceId === vto.deviceId,
      highlighted: vto.callState === "ringing" || vto.callState === "active",
      badge: vto.callState === "ringing" ? "Ringing" : undefined,
    });
    if (vto.lockCount > 0) {
      items.push({
        id: `${vto.deviceId}:lock`,
        label: `${vto.label} Lock`,
        secondary: "Accessory",
        kind: "accessory",
        selected: false,
        highlighted: false,
      });
    }
    if (vto.alarmCount > 0 || vto.tamper) {
      items.push({
        id: `${vto.deviceId}:alarm`,
        label: `${vto.label} Alarm`,
        secondary: "Accessory",
        kind: "accessory",
        selected: false,
        highlighted: vto.tamper,
        badge: vto.tamper ? "Tamper" : undefined,
      });
    }
  }

  for (const nvr of nvrs) {
    items.push({
      id: nvr.deviceId,
      label: nvr.label,
      secondary: "NVR",
      kind: "nvr",
      selected: selection.kind === "nvr" && selection.deviceId === nvr.deviceId,
      highlighted: !nvr.healthy,
      badge: !nvr.healthy ? "Alert" : undefined,
    });
    for (const room of nvr.rooms) {
      for (const channel of room.channels) {
        items.push(buildCameraSidebarItem(channel, selection));
      }
    }
  }

  for (const camera of cameras.filter((camera) => !nvrDeviceIds.has(camera.rootDeviceId))) {
    items.push(buildCameraSidebarItem(camera, selection));
  }

  return items;
}

function buildCameraSidebarItem(
  camera: CameraViewModel,
  selection: PanelSelection,
): SidebarItem {
  return {
    id: camera.deviceId,
    label: camera.label,
    secondary: camera.roomLabel,
    kind: "camera",
    selected: selection.kind === "camera" && selection.deviceId === camera.deviceId,
    highlighted: camera.detections.length > 0,
    badge: camera.detections[0]?.label,
  };
}

export function buildPtzUrl(camera: CameraViewModel): string | null {
  if (camera.ptzUrl) {
    return camera.ptzUrl;
  }

  const channel = parseChannelNumber(camera.deviceId);
  if (!camera.bridgeBaseUrl || !channel) {
    return null;
  }

  return buildBridgeEndpointUrl(
    camera.bridgeBaseUrl,
    `/api/v1/nvr/${camera.rootDeviceId}/channels/${channel}/ptz`,
  );
}

export function buildAuxUrl(camera: CameraViewModel): string | null {
  if (camera.auxUrl) {
    return camera.auxUrl;
  }

  const channel = parseChannelNumber(camera.deviceId);
  if (!camera.bridgeBaseUrl || !channel) {
    return null;
  }

  return buildBridgeEndpointUrl(
    camera.bridgeBaseUrl,
    `/api/v1/nvr/${camera.rootDeviceId}/channels/${channel}/aux`,
  );
}

export function findAuxTarget(
  camera: CameraViewModel,
  outputKey: string,
): CameraAuxTargetViewModel | null {
  if (!camera.aux) {
    return null;
  }

  return (
    camera.aux.targets.find(
      (target) => target.outputKey === outputKey || target.key === outputKey,
    ) ?? null
  );
}

export function supportsAuxTarget(
  camera: CameraViewModel,
  outputKey: string,
): boolean {
  return findAuxTarget(camera, outputKey) !== null;
}

export function buildRecordingUrl(camera: CameraViewModel): string | null {
  if (camera.recordingUrl) {
    return camera.recordingUrl;
  }

  const channel = parseChannelNumber(camera.deviceId);
  if (!camera.bridgeBaseUrl || !channel) {
    return null;
  }

  return buildBridgeEndpointUrl(
    camera.bridgeBaseUrl,
    `/api/v1/nvr/${camera.rootDeviceId}/channels/${channel}/recording`,
  );
}

export function resolveCameraRecordingActionUrl(
  camera: CameraViewModel,
  action: "start" | "stop",
): string | null {
  const directUrl = action === "start" ? camera.recordingStartUrl : camera.recordingStopUrl;
  if (directUrl) {
    return directUrl;
  }
  return null;
}

export function displayCameraLabel(
  camera: Pick<CameraViewModel, "label" | "deviceKind" | "channelNumber">,
): string {
  return camera.deviceKind === "nvr_channel" && camera.channelNumber !== null
    ? `CH ${camera.channelNumber} - ${camera.label}`
    : camera.label;
}

export function resolveAuxTargetAction(
  target: CameraAuxTargetViewModel | null,
  active: boolean,
): CameraAuxActionName | null {
  if (!target) {
    return "pulse";
  }
  if (isDeterrenceAuxTarget(target)) {
    return active ? "stop" : "start";
  }
  if (target.toggleSupported) {
    return active ? "stop" : "start";
  }
  if (!active && target.actions.includes("start")) {
    return "start";
  }
  if (active && target.actions.includes("stop")) {
    return "stop";
  }
  if (target.actions.includes("pulse")) {
    return "pulse";
  }
  return target.preferredAction;
}

function isDeterrenceAuxTarget(target: CameraAuxTargetViewModel): boolean {
  const keys = [target.key, target.outputKey, target.parameterValue]
    .map((value) => value.trim().toLowerCase())
    .filter((value, index, all) => value.length > 0 && all.indexOf(value) === index);
  return keys.some((value) =>
    value === "warning_light" || value === "light" || value === "siren" || value === "aux",
  );
}

function buildEventContextLookup(
  cameras: CameraViewModel[],
  nvrs: NvrViewModel[],
  vtos: VtoViewModel[],
): Map<string, { label: string; roomLabel: string | null; deviceKind: string | null }> {
  const contextByDeviceId = new Map<
    string,
    { label: string; roomLabel: string | null; deviceKind: string | null }
  >();

  for (const nvr of nvrs) {
    contextByDeviceId.set(nvr.deviceId, {
      label: nvr.label,
      roomLabel: nvr.roomLabel,
      deviceKind: "nvr",
    });
  }
  for (const camera of cameras) {
    contextByDeviceId.set(camera.deviceId, {
      label: camera.label,
      roomLabel: camera.roomLabel,
      deviceKind: camera.deviceKind,
    });
  }
  for (const vto of vtos) {
    contextByDeviceId.set(vto.deviceId, {
      label: vto.label,
      roomLabel: vto.roomLabel,
      deviceKind: "vto",
    });
  }

  return contextByDeviceId;
}

function buildVtoHeaderMetric(vtos: VtoViewModel[]): HeaderMetric | null {
  if (vtos.length === 0) {
    return null;
  }

  const ringingCount = vtos.filter((vto) => vto.callState === "ringing").length;
  const activeCount = vtos.filter((vto) => vto.callState === "active").length;
  const onlineCount = vtos.filter((vto) => vto.online).length;

  if (vtos.length === 1) {
    const vto = vtos[0]!;
    return {
      label: "VTO State",
      value: vto.callStateText,
      tone:
        vto.callState === "active"
          ? "info"
          : vto.callState === "ringing"
            ? "warning"
            : vto.online
              ? "success"
              : "critical",
    };
  }

  return {
    label: "Door Stations",
    value:
      ringingCount > 0
        ? `${ringingCount} ringing`
        : activeCount > 0
          ? `${activeCount} active`
          : `${onlineCount}/${vtos.length} online`,
    tone:
      ringingCount > 0
        ? "warning"
        : activeCount > 0
          ? "info"
          : onlineCount === vtos.length
            ? "success"
            : onlineCount > 0
              ? "warning"
              : "critical",
  };
}

function filterTimelineForSelection(
  timeline: TimelineEvent[],
  selection: PanelSelection,
): TimelineEvent[] {
  if (selection.kind === "overview" || !selection.deviceId) {
    return timeline;
  }

  if (selection.kind === "camera") {
    return timeline.filter((event) => event.deviceId === selection.deviceId);
  }

  return timeline.filter(
    (event) =>
      event.deviceId === selection.deviceId || event.rootDeviceId === selection.deviceId,
  );
}

function buildDetectionBadges(camera: NvrChannelModel | IpcModel): DetectionBadge[] {
  const badges: DetectionBadge[] = [];
  if (camera.detections.motion) {
    badges.push({ key: "motion", label: "Motion", icon: "mdi:motion-sensor", tone: "warning" });
  }
  if (camera.detections.human) {
    badges.push({ key: "human", label: "Human", icon: "mdi:account", tone: "info" });
  }
  if (camera.detections.vehicle) {
    badges.push({ key: "vehicle", label: "Vehicle", icon: "mdi:car", tone: "info" });
  }
  if (camera.detections.tripwire) {
    badges.push({ key: "tripwire", label: "Tripwire", icon: "mdi:vector-line", tone: "warning" });
  }
  if (camera.detections.intrusion) {
    badges.push({
      key: "intrusion",
      label: "Intrusion",
      icon: "mdi:shield-alert",
      tone: "critical",
    });
  }
  return badges;
}

function compareCameraViewModels(left: CameraViewModel, right: CameraViewModel): number {
  return (
    left.rootDeviceId.localeCompare(right.rootDeviceId) ||
    left.roomLabel.localeCompare(right.roomLabel) ||
    (parseChannelNumber(left.deviceId) ?? Number.MAX_SAFE_INTEGER) -
      (parseChannelNumber(right.deviceId) ?? Number.MAX_SAFE_INTEGER) ||
    left.label.localeCompare(right.label)
  );
}

function stringValue(value: unknown): string | null {
  return typeof value === "string" && value.trim() ? value : null;
}

function sumNullable(values: Array<number | null | undefined>): number | null {
  let sum = 0;
  let found = false;
  for (const value of values) {
    if (typeof value !== "number" || !Number.isFinite(value)) {
      continue;
    }
    sum += value;
    found = true;
  }
  return found ? sum : null;
}

function normalizeStorageValues(
  totalBytes: number | null,
  usedBytes: number | null,
  usedPercent: number | null,
): {
  totalBytes: number | null;
  usedBytes: number | null;
  usedPercent: number | null;
} {
  const normalizedPercent =
    typeof usedPercent === "number" && Number.isFinite(usedPercent) ? usedPercent : null;
  const normalizedTotal =
    typeof totalBytes === "number" && Number.isFinite(totalBytes) ? totalBytes : null;
  const normalizedUsed =
    typeof usedBytes === "number" && Number.isFinite(usedBytes) ? usedBytes : null;

  if (normalizedTotal !== null && normalizedUsed !== null) {
    return {
      totalBytes: normalizedTotal,
      usedBytes: normalizedUsed,
      usedPercent:
        normalizedPercent ??
        (normalizedTotal > 0 ? (normalizedUsed / normalizedTotal) * 100 : null),
    };
  }

  if (normalizedTotal !== null && normalizedPercent !== null) {
    return {
      totalBytes: normalizedTotal,
      usedBytes: normalizedTotal * (normalizedPercent / 100),
      usedPercent: normalizedPercent,
    };
  }

  if (normalizedUsed !== null && normalizedPercent !== null && normalizedPercent > 0) {
    return {
      totalBytes: normalizedUsed / (normalizedPercent / 100),
      usedBytes: normalizedUsed,
      usedPercent: normalizedPercent,
    };
  }

  return {
    totalBytes: normalizedTotal,
    usedBytes: normalizedUsed,
    usedPercent: normalizedPercent,
  };
}
