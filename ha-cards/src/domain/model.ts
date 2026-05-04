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
  findPanelCameraEventSummary,
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
import {
  createLocalizer,
  pluralUnit,
  resolvePanelLanguage,
  type Localizer,
  type PanelLanguage,
} from "../localization";

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
  audioMuteSupported: boolean;
  validationNotes: string[];
  nvrConfigWritable: boolean | null;
  nvrConfigReason: string | null;
  directIPCConfigured: boolean;
  directIPCConfiguredIP: string | null;
  directIPCIP: string | null;
  directIPCModel: string | null;
  eventCount24h: number;
  humanCount24h: number;
  vehicleCount24h: number;
  ivsCount24h: number;
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
  smdIvsUrl: string | null;
  chunksUrl: string | null;
  channel: number | null;
  defaultLimit: number;
}

export interface CameraStreamProfileViewModel {
  key: string;
  name: string;
  streamUrl: string | null;
  localMjpegUrl: string | null;
  localHlsUrl: string | null;
  localDashUrl: string | null;
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
  recommendedHaIntegration: string | null;
  preferredVideoProfile: string | null;
  preferredVideoSource: string | null;
  fallbacksEnabled: boolean;
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
  language: PanelLanguage;
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
  const language = resolvePanelLanguage(hass);
  const t = createLocalizer(language);
  const activeEventSummary = todayEventSummary ?? null;
  const cameras = topology.cameras
    .map((camera) => buildCameraViewModel(camera, browserBridgeUrl, activeEventSummary, t))
    .sort(compareCameraViewModels);
  const nvrs = topology.nvrs
    .map((nvr) => buildNvrViewModel(nvr, browserBridgeUrl, activeEventSummary, t))
    .sort((left, right) => left.label.localeCompare(right.label));
  const vtos = buildVtoViewModels(hass, topology.vtos, config.vto, browserBridgeUrl, t);
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
      label: t("metric.camerasOnline"),
      value: `${onlineCount}/${cameras.length + vtos.length}`,
      tone: onlineCount > 0 ? "success" : "critical",
    },
  ];
  const eventHeaderMetrics = buildTodayEventHeaderMetrics(activeEventSummary, t);
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
        label: t("metric.motion"),
        value: `${motionCount}`,
        tone: motionCount > 0 ? "warning" : "neutral",
      },
      {
        label: t("metric.human"),
        value: `${humanCount}`,
        tone: humanCount > 0 ? "info" : "neutral",
      },
      {
        label: t("metric.vehicle"),
        value: `${vehicleCount}`,
        tone: vehicleCount > 0 ? "info" : "neutral",
      },
    );
  }

  if (nvrs.length > 0) {
    const healthy = nvrs.every((nvr) => nvr.healthy);
    headerMetrics.push({
      label: t("metric.nvrHealth"),
      value: healthy ? t("state.healthy") : t("state.attention"),
      tone: healthy ? "success" : "warning",
    });
  }

  const vtoHeaderMetric = buildVtoHeaderMetric(vtos, t);
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
        t,
      ),
    ),
    ...vtos.flatMap((entry) =>
      collectVtoEvents(hass, entry.deviceId, entry.label, entry.roomLabel, lookbackMs, t),
    ),
  ].sort((left, right) => right.timestamp - left.timestamp);

  const contextByDeviceId = buildEventContextLookup(cameras, nvrs, vtos);
  const bridgeTimeline = bridgeEvents
    ? bridgeEventsToTimeline(bridgeEvents, contextByDeviceId, lookbackMs, t)
    : [];
  const eventFeed = filterTimelineForSelection(
    mergeTimelineEvents(bridgeTimeline, syntheticTimeline),
    selection,
  );
  const timeline = eventFeed.slice(0, config.max_events ?? 14);

  return {
    language,
    title: config.title ?? t("app.title"),
    subtitle: config.subtitle ?? t("app.subtitle"),
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
    sidebarItems: buildSidebarItems(cameras, nvrs, vtos, selection, t),
  };
}

function buildCameraViewModel(
  camera: CameraDeviceModel,
  browserBridgeUrl: string | null,
  todayEventSummary: PanelTodayEventSummaryModel | null,
  t: Localizer,
): CameraViewModel {
  const aux = buildCameraAuxViewModel(camera.capabilities.aux ?? null, browserBridgeUrl);
  const recording = buildCameraRecordingViewModel(
    camera.capabilities.recording ?? null,
    browserBridgeUrl,
  );
  const eventSummary = findPanelCameraEventSummary(
    todayEventSummary,
    camera.rootDeviceId,
    camera.kind === "nvr_channel" ? camera.channelNumber : null,
  );
  return {
    type: "camera",
    deviceKind: camera.kind,
    kindLabel: camera.kind === "ipc" ? t("kind.ipcCamera") : t("kind.nvrChannel"),
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
    stream: buildCameraStreamViewModel(camera, browserBridgeUrl, t),
    detections: buildDetectionBadges(camera, t),
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
    recordingsUrl: normalizeCaptureRecordingsUrl(
      camera.media.capture?.recordingsUrl ?? null,
      camera.media.capture?.startRecordingUrl ?? null,
      camera.deviceId,
      browserBridgeUrl,
    ),
    resolution: camera.media.resolution,
    codec: camera.media.codec,
    frameRate: camera.media.frameRate,
    bitrate: camera.media.bitrate,
    profile: camera.media.profile,
    audioCodec: camera.media.audioCodec,
    microphoneAvailable: camera.media.audioCodec.trim().length > 0,
    speakerAvailable: camera.capabilities.audio.playback.supported,
    audioMuteSupported: camera.media.audioCodec.trim().length > 0,
    validationNotes: [...camera.capabilities.validationNotes],
    nvrConfigWritable: camera.diagnostics.nvrConfigWritable,
    nvrConfigReason: camera.diagnostics.nvrConfigReason,
    directIPCConfigured: camera.diagnostics.directIPCConfigured === true,
    directIPCConfiguredIP: camera.diagnostics.directIPCConfiguredIP,
    directIPCIP: camera.diagnostics.directIPCIP,
    directIPCModel: camera.diagnostics.directIPCModel,
    eventCount24h: eventSummary?.totalCount ?? 0,
    humanCount24h: eventSummary?.humanCount ?? 0,
    vehicleCount24h: eventSummary?.vehicleCount ?? 0,
    ivsCount24h: eventSummary?.ivsCount ?? 0,
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

function normalizeCaptureRecordingsUrl(
  rawUrl: string | null,
  startRecordingUrl: string | null,
  streamID: string,
  browserBridgeUrl: string | null,
): string | null {
  const sourceUrl = rawUrl?.trim() ? rawUrl : startRecordingUrl;
  if (!sourceUrl?.trim()) {
    return null;
  }
  const normalizedRawUrl = rewriteBridgeUrl(sourceUrl, browserBridgeUrl);
  if (normalizedRawUrl) {
    try {
      const url = new URL(normalizedRawUrl, "https://dahuabridge.invalid");
      const streamMatch = url.pathname.match(/^(.*\/api\/v1\/media\/)streams\/([^/]+)\/recordings\/?$/);
      if (!streamMatch) {
        return normalizedRawUrl;
      }
      const resolvedStreamID = decodeURIComponent(streamMatch[2] ?? streamID);
      url.pathname = `${streamMatch[1]}recordings`;
      url.search = `?stream_id=${encodeURIComponent(resolvedStreamID)}`;
      if (/^[a-z][a-z0-9+.-]*:\/\//i.test(normalizedRawUrl)) {
        return url.toString();
      }
      return `${url.pathname}${url.search}${url.hash}`;
    } catch {
      return normalizedRawUrl;
    }
  }
  return null;
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

function buildCameraStreamViewModel(
  camera: { media: CameraDeviceModel["media"] },
  browserBridgeUrl: string | null,
  t: Localizer = createLocalizer("en"),
): CameraStreamViewModel {
  return {
    available: camera.media.streamAvailable,
    source: rewriteBridgeUrl(camera.media.streamSource, browserBridgeUrl),
    snapshotUrl: rewriteBridgeUrl(camera.media.snapshotUrl, browserBridgeUrl),
    localIntercomUrl: rewriteBridgeUrl(camera.media.localIntercomUrl, browserBridgeUrl),
    onvifStreamUrl: rewriteBridgeUrl(camera.media.onvifStreamUrl, browserBridgeUrl),
    onvifSnapshotUrl: rewriteBridgeUrl(camera.media.onvifSnapshotUrl, browserBridgeUrl),
    recommendedProfile: camera.media.recommendedProfile,
    recommendedHaIntegration: camera.media.recommendedHaIntegration,
    preferredVideoProfile: camera.media.preferredVideoProfile,
    preferredVideoSource: camera.media.preferredVideoSource,
    fallbacksEnabled: camera.media.videoFallbacksEnabled,
    resolution: camera.media.resolution,
    codec: camera.media.codec,
    frameRate: camera.media.frameRate,
    bitrate: camera.media.bitrate,
    profile: camera.media.profile,
    audioCodec: camera.media.audioCodec,
    profiles: Object.entries(camera.media.profiles)
      .map(([key, profile]) => ({
        key,
        name: streamProfileDisplayName(key, profile.name, t),
        streamUrl: rewriteBridgeUrl(profile.streamUrl, browserBridgeUrl),
        localMjpegUrl: rewriteBridgeUrl(profile.localMjpegUrl, browserBridgeUrl),
        localHlsUrl: rewriteBridgeUrl(profile.localHlsUrl, browserBridgeUrl),
        localDashUrl: rewriteBridgeUrl(profile.localDashUrl, browserBridgeUrl),
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

function streamProfileDisplayName(
  key: string,
  fallback: string | null,
  t: Localizer = createLocalizer("en"),
): string {
  switch (key.trim().toLowerCase()) {
    case "quality":
    case "default":
      return t("profile.quality");
    case "stable":
    case "substream":
      return t("profile.stable");
    default:
      return fallback?.trim() || key.trim();
  }
}

function streamProfileSortRank(key: string): number {
  switch (key.trim().toLowerCase()) {
    case "quality":
    case "default":
      return 0;
    case "stable":
    case "substream":
      return 1;
    default:
      return 2;
  }
}

function buildNvrViewModel(
  nvr: DeviceNvrModel,
  browserBridgeUrl: string | null,
  todayEventSummary: PanelTodayEventSummaryModel | null,
  t: Localizer,
): NvrViewModel {
  const usedBytesTotal = sumNullable(nvr.drives.map((drive) => drive.usedBytes));
  const rooms = nvr.roomGroups.map((roomGroup) => ({
    label: roomGroup.label,
      channels: roomGroup.channels
      .map((channel) => buildCameraViewModel(channel, browserBridgeUrl, todayEventSummary, t))
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
        ? nvr.storageUsedPercent !== null
          ? t("storage.summaryUsedPercent", {
              used: formatBytes(usedBytesTotal),
              total: formatBytes(nvr.totalBytes),
              percent: Math.round(nvr.storageUsedPercent),
            })
          : t("storage.summaryUsed", {
              used: formatBytes(usedBytesTotal),
              total: formatBytes(nvr.totalBytes),
            })
        : nvr.totalBytes !== null
          ? t("storage.total", { value: formatBytes(nvr.totalBytes) })
          : nvr.storageUsedPercent !== null
            ? t("storage.percentUsed", { value: Math.round(nvr.storageUsedPercent) })
          : nvr.drives.length > 0
            ? t("storage.driveCount", {
                count: nvr.drives.length,
                unit: pluralUnit(nvr.drives.length, "unit.drive", "unit.drives", t),
              })
            : t("storage.unknown"),
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
  if (!archive.supported && !archive.smdIvsSearch.supported && !archive.chunkSearch.supported) {
    return null;
  }

  return {
    supported: archive.supported,
    smdIvsUrl: rewriteBridgeUrl(archive.smdIvsSearch.url, browserBridgeUrl),
    chunksUrl: rewriteBridgeUrl(archive.chunkSearch.url, browserBridgeUrl),
    channel: archive.smdIvsSearch.channel ?? archive.chunkSearch.channel,
    defaultLimit: archive.smdIvsSearch.defaultLimit || archive.chunkSearch.defaultLimit,
  };
}

function buildVtoViewModel(
  vto: DeviceVtoModel,
  browserBridgeUrl: string | null,
  t: Localizer,
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
    stream: buildCameraStreamViewModel(vto, browserBridgeUrl, t),
    bridgeRecordingActive: vto.media.capture?.recordingActive === true,
    recordingStartUrl: rewriteBridgeUrl(
      vto.media.capture?.startRecordingUrl ?? null,
      browserBridgeUrl,
    ),
    recordingStopUrl: rewriteBridgeUrl(
      vto.media.capture?.stopRecordingUrl ?? null,
      browserBridgeUrl,
    ),
    recordingsUrl: normalizeCaptureRecordingsUrl(
      vto.media.capture?.recordingsUrl ?? null,
      vto.media.capture?.startRecordingUrl ?? null,
      vto.deviceId,
      browserBridgeUrl,
    ),
    doorbell: vto.doorbell,
    callActive: vto.callActive,
    accessActive: vto.accessActive,
    tamper: vto.tamper,
    callState: vto.callState,
    callStateText: callStateLabel(vto.callState, t),
    lastCallSource: vto.lastCallSource,
    lastCallStartedAt: vto.lastCallStartedAt,
    lastCallEndedAt: vto.lastCallEndedAt,
    lastCallDuration: String(vto.lastCallDurationSeconds ?? 0),
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

function callStateLabel(callState: VtoCallState, t: Localizer): string {
  switch (callState) {
    case "active":
      return t("state.activeCall");
    case "ringing":
      return t("state.ringing");
    case "offline":
      return t("state.offline");
    case "idle":
      return t("state.idle");
  }
}

function buildVtoViewModels(
  hass: HomeAssistant,
  vtos: DeviceVtoModel[],
  overrides?: NonNullable<SurveillancePanelCardConfig["vto"]>,
  browserBridgeUrl?: string | null,
  t: Localizer = createLocalizer("en"),
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
    const viewModel = buildVtoViewModel(vto, browserBridgeUrl ?? null, t);
    if (!preferredVtoId || vto.deviceId !== preferredVtoId) {
      return viewModel;
    }

    return {
      ...viewModel,
      label: overrides?.label ?? viewModel.label,
      unlockButtonEntityId: overrides?.lock_button_entity ?? viewModel.unlockButtonEntityId,
      hasUnlockButtonEntity:
        hass.states[overrides?.lock_button_entity ?? viewModel.unlockButtonEntityId] !== undefined,
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
  t: Localizer = createLocalizer("en"),
): SidebarItem[] {
  const items: SidebarItem[] = [];
  const nvrDeviceIds = new Set(nvrs.map((nvr) => nvr.deviceId));

  for (const vto of vtos) {
    items.push({
      id: vto.deviceId,
      label: vto.label,
      secondary: t("kind.doorStation"),
      kind: "vto",
      selected: selection.kind === "vto" && selection.deviceId === vto.deviceId,
      highlighted: vto.callState === "ringing" || vto.callState === "active",
      badge: vto.callState === "ringing" ? t("state.ringing") : undefined,
    });
    if (vto.lockCount > 0) {
      items.push({
        id: `${vto.deviceId}:lock`,
        label: `${vto.label} ${t("unit.lock")}`,
        secondary: t("kind.doorStation"),
        kind: "accessory",
        selected: false,
        highlighted: false,
      });
    }
    if (vto.alarmCount > 0 || vto.tamper) {
      items.push({
        id: `${vto.deviceId}:alarm`,
        label: `${vto.label} ${t("unit.alarm")}`,
        secondary: t("kind.doorStation"),
        kind: "accessory",
        selected: false,
        highlighted: vto.tamper,
        badge: vto.tamper ? t("event.tamper") : undefined,
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
      badge: !nvr.healthy ? t("unit.alert") : undefined,
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

function buildVtoHeaderMetric(
  vtos: VtoViewModel[],
  t: Localizer = createLocalizer("en"),
): HeaderMetric | null {
  if (vtos.length === 0) {
    return null;
  }

  const ringingCount = vtos.filter((vto) => vto.callState === "ringing").length;
  const activeCount = vtos.filter((vto) => vto.callState === "active").length;
  const onlineCount = vtos.filter((vto) => vto.online).length;

  if (vtos.length === 1) {
    const vto = vtos[0]!;
    return {
      label: t("metric.doorStation"),
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
    label: t("sidebar.doorStations"),
    value:
      ringingCount > 0
        ? `${ringingCount} ${t("state.ringing").toLowerCase()}`
        : activeCount > 0
          ? `${activeCount} ${t("state.active").toLowerCase()}`
          : `${onlineCount}/${vtos.length} ${t("state.online").toLowerCase()}`,
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

function buildDetectionBadges(
  camera: NvrChannelModel | IpcModel,
  t: Localizer = createLocalizer("en"),
): DetectionBadge[] {
  const badges: DetectionBadge[] = [];
  if (camera.detections.motion) {
    badges.push({ key: "motion", label: t("event.motion"), icon: "mdi:motion-sensor", tone: "warning" });
  }
  if (camera.detections.human) {
    badges.push({ key: "human", label: t("event.human"), icon: "mdi:account", tone: "info" });
  }
  if (camera.detections.vehicle) {
    badges.push({ key: "vehicle", label: t("event.vehicle"), icon: "mdi:car", tone: "info" });
  }
  if (camera.detections.tripwire) {
    badges.push({ key: "tripwire", label: t("event.tripwire"), icon: "mdi:vector-line", tone: "warning" });
  }
  if (camera.detections.intrusion) {
    badges.push({
      key: "intrusion",
      label: t("event.intrusion"),
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
