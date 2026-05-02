import {
  binarySensorEntityId,
  buttonEntityId,
  numberEntityId,
  normalizeBridgeOrigin,
  parseChannelNumber,
  sensorEntityId,
  switchEntityId,
} from "../ha/entity-id";
import {
  areaNameForDevice,
  areaNameForEntity,
  type DeviceRegistryEntry,
  deviceNameForDevice,
  deviceNameForEntity,
  type EntityRegistryEntry,
  rootDeviceName,
  type RegistrySnapshot,
} from "../ha/registry";
import {
  entityBooleanState,
  entityById,
  entityFriendlyName,
  entityNumberState,
  entityState,
} from "../ha/state";
import type { HassEntity, HomeAssistant } from "../types/home-assistant";
import {
  buildCameraArchiveCapabilities,
  normalizeArchiveSearchUrlTemplate,
  type CameraArchiveCapabilities,
} from "./archive";
import {
  buildCameraAuxCapabilities,
  type CameraAuxCapabilities,
} from "./camera-aux";

export type BridgeDeviceKind =
  | "nvr"
  | "nvr_channel"
  | "nvr_disk"
  | "ipc"
  | "vto"
  | "vto_lock"
  | "vto_alarm";

export type VtoCallState = "idle" | "ringing" | "active" | "offline";

export interface BridgeFeatureTarget {
  key: string | null;
  label: string | null;
  url: string | null;
}

export interface BridgeFeatureSummary {
  key: string | null;
  label: string | null;
  group: string | null;
  kind: string | null;
  url: string | null;
  supported: boolean;
  parameterKey: string | null;
  parameterValue: string | null;
  commands: string[];
  actions: string[];
  targets: BridgeFeatureTarget[];
  allowedValues: number[];
  minValue: number | null;
  maxValue: number | null;
  stepValue: number | null;
  currentValue: number | null;
  active: boolean | null;
  currentText: string | null;
}

export interface BridgeCaptureSummary {
  snapshotUrl: string | null;
  recordingActive: boolean;
  startRecordingUrl: string | null;
  stopRecordingUrl: string | null;
  recordingsUrl: string | null;
  activeClipId: string | null;
  activeClipDownloadUrl: string | null;
}

export interface BridgePtzControlSummary {
  url: string | null;
  supported: boolean;
  pan: boolean;
  tilt: boolean;
  zoom: boolean;
  focus: boolean;
  moveRelatively: boolean;
  autoScan: boolean;
  preset: boolean;
  pattern: boolean;
  tour: boolean;
  commands: string[];
}

export interface BridgeAuxControlSummary {
  url: string | null;
  supported: boolean;
  outputs: string[];
  features: string[];
}

export interface BridgeAudioControlSummary {
  supported: boolean;
  mute: boolean;
  volume: boolean;
  volumePermissionDenied: boolean;
  playbackSupported: boolean;
  playbackSiren: boolean;
  playbackQuickReply: boolean;
  playbackFormats: string[];
  playbackFileCount: number | null;
}

export interface BridgeRecordingControlSummary {
  url: string | null;
  supported: boolean;
  active: boolean;
  mode: string | null;
}

export interface BridgeControlsSummary {
  ptz: BridgePtzControlSummary | null;
  aux: BridgeAuxControlSummary | null;
  audio: BridgeAudioControlSummary | null;
  recording: BridgeRecordingControlSummary | null;
  validationNotes: string[];
}

export interface BridgeIntercomSummary {
  callState: string | null;
  lastRingAt: string | null;
  lastCallStartedAt: string | null;
  lastCallEndedAt: string | null;
  lastCallSource: string | null;
  lastCallDurationSeconds: number | null;
  answerUrl: string | null;
  hangupUrl: string | null;
  bridgeSessionResetUrl: string | null;
  lockUrls: string[];
  externalUplinkEnableUrl: string | null;
  externalUplinkDisableUrl: string | null;
  outputVolumeUrl: string | null;
  inputVolumeUrl: string | null;
  muteUrl: string | null;
  recordingUrl: string | null;
  bridgeSessionActive: boolean;
  bridgeSessionCount: number | null;
  externalUplinkEnabled: boolean;
  bridgeUplinkActive: boolean;
  bridgeUplinkCodec: string | null;
  bridgeUplinkPackets: number | null;
  bridgeForwardedPackets: number | null;
  bridgeForwardErrors: number | null;
  supportsHangup: boolean;
  supportsBridgeSessionReset: boolean;
  supportsUnlock: boolean;
  supportsBrowserMicrophone: boolean;
  supportsBridgeAudioUplink: boolean;
  supportsBridgeAudioOutput: boolean;
  supportsExternalAudioExport: boolean;
  configuredExternalUplinkTargetCount: number | null;
  supportsVtoCallAnswer: boolean;
  supportsVtoOutputVolumeControl: boolean;
  supportsVtoInputVolumeControl: boolean;
  supportsVtoMuteControl: boolean;
  supportsVtoRecordingControl: boolean;
  supportsVtoTalkback: boolean;
  supportsFullCallAcceptance: boolean;
  outputVolumeLevel: number | null;
  outputVolumeLevels: number[];
  inputVolumeLevel: number | null;
  inputVolumeLevels: number[];
  muted: boolean;
  autoRecordEnabled: boolean;
  autoRecordTimeSeconds: number | null;
  streamAudioEnabled: boolean;
  validationNotes: string[];
}

export interface BridgeDetectionState {
  motion: boolean;
  human: boolean;
  vehicle: boolean;
  tripwire: boolean;
  intrusion: boolean;
}

export interface BridgeMediaModel {
  streamAvailable: boolean;
  streamSource: string | null;
  snapshotUrl: string | null;
  capture: BridgeCaptureSummary | null;
  previewUrl: string | null;
  localIntercomUrl: string | null;
  onvifStreamUrl: string | null;
  onvifSnapshotUrl: string | null;
  recommendedProfile: string | null;
  recommendedHaIntegration: string | null;
  preferredVideoProfile: string | null;
  preferredVideoSource: string | null;
  resolution: string;
  codec: string;
  frameRate: string;
  bitrate: string;
  profile: string;
  audioCodec: string;
  profiles: Record<string, BridgeStreamProfileModel>;
}

export interface BridgeStreamProfileModel {
  name: string;
  streamUrl: string | null;
  localMjpegUrl: string | null;
  localHlsUrl: string | null;
  localDashUrl: string | null;
  localWebRtcUrl: string | null;
  subtype: number | null;
  rtspTransport: string | null;
  frameRate: number | null;
  sourceWidth: number | null;
  sourceHeight: number | null;
  useWallclockAsTimestamps: boolean;
  recommended: boolean;
}

export interface BridgeDeviceMetadataModel {
  manufacturer: string | null;
  model: string | null;
  serial: string | null;
  firmware: string | null;
  buildDate: string | null;
}

export interface BridgeCatalogHintsModel {
  recommendedProfile: string | null;
  recommendedHaIntegration: string | null;
  recommendedHaReason: string | null;
}

export interface BridgeOnvifModel {
  h264Available: boolean | null;
  profileName: string | null;
  profileToken: string | null;
}

export interface BridgeDeviceDiagnosticsModel {
  channel: number | null;
  channelCount: number | null;
  diskCount: number | null;
  lockCount: number | null;
  totalBytes: number | null;
  usedBytes: number | null;
  freeBytes: number | null;
  usedPercent: number | null;
  mainCodec: string | null;
  mainResolution: string | null;
  subCodec: string | null;
  subResolution: string | null;
  audioCodec: string | null;
  controlAudioAuthority: string | null;
  controlAudioSemantic: string | null;
  nvrConfigWritable: boolean | null;
  nvrConfigReason: string | null;
  directIPCConfigured: boolean | null;
  directIPCConfiguredIP: string | null;
  directIPCIP: string | null;
  directIPCModel: string | null;
  catalog: BridgeCatalogHintsModel;
  onvif: BridgeOnvifModel;
}

export interface CameraAudioPlaybackCapabilities {
  supported: boolean;
  siren: boolean;
  quickReply: boolean;
  formats: string[];
  fileCount: number | null;
}

export interface CameraAudioCapabilities {
  supported: boolean;
  mute: boolean;
  volume: boolean;
  volumePermissionDenied: boolean;
  playback: CameraAudioPlaybackCapabilities;
}

export interface CameraCapabilityModel {
  archive: CameraArchiveCapabilities;
  ptz: BridgePtzControlSummary | null;
  aux: CameraAuxCapabilities | null;
  recording: BridgeRecordingControlSummary | null;
  audio: CameraAudioCapabilities;
  validationNotes: string[];
}

export interface VtoCapabilityModel {
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

export interface BridgeDeviceBase {
  kind: BridgeDeviceKind;
  deviceId: string;
  rootDeviceId: string;
  haDeviceId: string | null;
  haParentDeviceId: string | null;
  label: string;
  roomLabel: string;
  online: boolean;
  bridgeBaseUrl: string | null;
  eventsUrl: string | null;
  metadata: BridgeDeviceMetadataModel;
  diagnostics: BridgeDeviceDiagnosticsModel;
}

export interface BridgeCameraDeviceBase extends BridgeDeviceBase {
  cameraEntityId: string;
  cameraEntity?: HassEntity;
  media: BridgeMediaModel;
  controls: BridgeControlsSummary | null;
  features: BridgeFeatureSummary[];
  capabilities: CameraCapabilityModel;
}

export interface NvrChannelModel extends BridgeCameraDeviceBase {
  kind: "nvr_channel";
  channelNumber: number | null;
  detections: BridgeDetectionState;
}

export interface IpcModel extends BridgeCameraDeviceBase {
  kind: "ipc";
  detections: BridgeDetectionState;
}

export interface NvrDriveModel extends BridgeDeviceBase {
  kind: "nvr_disk";
  slot: number | null;
  stateText: string | null;
  totalBytes: number | null;
  usedBytes: number | null;
  usedPercent: number | null;
  healthy: boolean;
}

export interface NvrModel extends BridgeDeviceBase {
  kind: "nvr";
  channels: NvrChannelModel[];
  childChannels: NvrChannelModel[];
  roomGroups: NvrRoomGroupModel[];
  drives: NvrDriveModel[];
  childDrives: NvrDriveModel[];
  storageUsedPercent: number | null;
  totalBytes: number | null;
  healthy: boolean;
  recordingActive: boolean;
}

export interface VtoLockModel extends BridgeDeviceBase {
  kind: "vto_lock";
  slot: number | null;
  stateText: string | null;
  sensorEnabled: boolean;
  lockMode: string | null;
  unlockHoldInterval: string | null;
  unlockButtonEntityId: string;
  unlockActionUrl: string | null;
  hasUnlockButtonEntity: boolean;
}

export interface VtoAlarmModel extends BridgeDeviceBase {
  kind: "vto_alarm";
  slot: number | null;
  active: boolean;
  enabled: boolean;
  senseMethod: string | null;
}

export type VtoSubDeviceModel = VtoLockModel | VtoAlarmModel;

export interface VtoModel extends BridgeCameraDeviceBase {
  kind: "vto";
  doorbell: boolean;
  callActive: boolean;
  accessActive: boolean;
  tamper: boolean;
  callState: VtoCallState;
  callStateText: string;
  lastCallSource: string;
  lastCallStartedAt: string;
  lastCallEndedAt: string;
  lastCallDurationSeconds: number | null;
  outputVolume: number | null;
  inputVolume: number | null;
  muted: boolean;
  autoRecordEnabled: boolean;
  intercom: BridgeIntercomSummary | null;
  answerButtonEntityId: string;
  hasAnswerButtonEntity: boolean;
  hangupButtonEntityId: string;
  hasHangupButtonEntity: boolean;
  outputVolumeEntityId: string;
  hasOutputVolumeEntity: boolean;
  inputVolumeEntityId: string;
  hasInputVolumeEntity: boolean;
  mutedEntityId: string;
  hasMutedEntity: boolean;
  autoRecordEntityId: string;
  hasAutoRecordEntity: boolean;
  lockCount: number;
  vtoCapabilities: VtoCapabilityModel;
  locks: VtoLockModel[];
  alarms: VtoAlarmModel[];
  subDevices: VtoSubDeviceModel[];
}

export type CameraDeviceModel = NvrChannelModel | IpcModel;

export interface NvrRoomGroupModel {
  id: string;
  label: string;
  nvrDeviceId: string;
  channels: NvrChannelModel[];
  onlineCount: number;
  alertCount: number;
}

export interface SiteRoomGroupModel {
  id: string;
  label: string;
  cameras: CameraDeviceModel[];
  nvrChannels: NvrChannelModel[];
  ipcs: IpcModel[];
  vtos: VtoModel[];
}

export interface BridgeTopologyModel {
  cameras: CameraDeviceModel[];
  nvrs: NvrModel[];
  ipcs: IpcModel[];
  vtos: VtoModel[];
  roomGroups: SiteRoomGroupModel[];
}

interface BridgeDescriptor {
  entity: HassEntity;
  cameraEntityId: string;
  deviceId: string;
  rootDeviceId: string;
  haDeviceId: string | null;
  haParentDeviceId: string | null;
  kind: "nvr_channel" | "ipc" | "vto";
  label: string;
  roomLabel: string;
}

interface BridgeFeatureShape {
  key?: unknown;
  label?: unknown;
  group?: unknown;
  kind?: unknown;
  url?: unknown;
  supported?: unknown;
  parameter_key?: unknown;
  parameter_value?: unknown;
  commands?: unknown;
  actions?: unknown;
  targets?: unknown;
  allowed_values?: unknown;
  min_value?: unknown;
  max_value?: unknown;
  step_value?: unknown;
  current_value?: unknown;
  active?: unknown;
  current_text?: unknown;
}

interface BridgeControlsShape {
  ptz?: {
    url?: unknown;
    supported?: unknown;
    pan?: unknown;
    tilt?: unknown;
    zoom?: unknown;
    focus?: unknown;
    move_relatively?: unknown;
    auto_scan?: unknown;
    preset?: unknown;
    pattern?: unknown;
    tour?: unknown;
    commands?: unknown;
  };
  aux?: {
    url?: unknown;
    supported?: unknown;
    outputs?: unknown;
    features?: unknown;
  };
  audio?: {
    supported?: unknown;
    mute?: unknown;
    volume?: unknown;
    volume_permission_denied?: unknown;
    playback_supported?: unknown;
    playback_siren?: unknown;
    playback_quick_reply?: unknown;
    playback_formats?: unknown;
    playback_file_count?: unknown;
  };
  recording?: {
    url?: unknown;
    supported?: unknown;
    active?: unknown;
    mode?: unknown;
  };
  validation_notes?: unknown;
}

interface BridgeCaptureShape {
  snapshot_url?: unknown;
  active?: unknown;
  start_recording_url?: unknown;
  stop_recording_url?: unknown;
  recordings_url?: unknown;
  active_clip_id?: unknown;
  active_clip_download_url?: unknown;
}

interface BridgeArchiveAttributeSummary {
  searchUrl: string | null;
  playbackUrl: string | null;
}

interface BridgeIntercomShape {
  [key: string]: unknown;
}

interface BridgeProfileShape {
  name?: unknown;
  stream_url?: unknown;
  local_mjpeg_url?: unknown;
  local_hls_url?: unknown;
  local_dash_url?: unknown;
  local_webrtc_url?: unknown;
  subtype?: unknown;
  rtsp_transport?: unknown;
  frame_rate?: unknown;
  source_width?: unknown;
  source_height?: unknown;
  use_wallclock_as_timestamps?: unknown;
  recommended?: unknown;
}

interface InferredBridgeDescriptor {
  deviceId: string;
  rootDeviceId: string;
  kind: "nvr_channel" | "ipc" | "vto";
}

export function discoverBridgeTopology(
  hass: HomeAssistant,
  registrySnapshot?: RegistrySnapshot | null,
): BridgeTopologyModel {
  const descriptors = discoverBridgeDescriptors(hass, registrySnapshot);

  const channels = descriptors
    .filter((descriptor) => descriptor.kind === "nvr_channel")
    .map((descriptor) => buildNvrChannelModel(hass, registrySnapshot, descriptor))
    .sort(compareCameraDevices);

  const ipcs = descriptors
    .filter((descriptor) => descriptor.kind === "ipc")
    .map((descriptor) => buildIpcModel(hass, registrySnapshot, descriptor))
    .sort(compareCameraDevices);

  const vtos = descriptors
    .filter((descriptor) => descriptor.kind === "vto")
    .map((descriptor) => buildVtoModel(hass, registrySnapshot, descriptor))
    .sort((left, right) => left.label.localeCompare(right.label));

  const nvrIds = [...new Set(channels.map((channel) => channel.rootDeviceId))];
  const nvrs = nvrIds
    .map((deviceId) => buildNvrModel(hass, registrySnapshot, deviceId, channels))
    .sort((left, right) => left.label.localeCompare(right.label));
  const roomGroups = buildSiteRoomGroups([...channels, ...ipcs], vtos);

  return {
    cameras: [...channels, ...ipcs],
    nvrs,
    ipcs,
    vtos,
    roomGroups,
  };
}

function discoverBridgeDescriptors(
  hass: HomeAssistant,
  registrySnapshot?: RegistrySnapshot | null,
): BridgeDescriptor[] {
  return Object.values(hass.states)
    .filter((entity) => entity.entity_id.startsWith("camera."))
    .map((entity) => {
      const inferred = inferBridgeDescriptor(hass, entity);
      const deviceId =
        bridgeDeviceIdForEntity(entity) ??
        inferDeviceIdFromCameraEntityId(entity.entity_id) ??
        inferred?.deviceId ??
        null;
      const rootDeviceId =
        bridgeRootDeviceIdForEntity(entity) ??
        inferred?.rootDeviceId ??
        inferRootDeviceId(deviceId);
      const haDeviceId = registrySnapshot?.entitiesByEntityId.get(entity.entity_id)?.device_id ?? null;
      const haParentDeviceId =
        haDeviceId ? (registrySnapshot?.devicesById.get(haDeviceId)?.via_device_id ?? null) : null;
      return {
        entity,
        cameraEntityId: entity.entity_id,
        deviceId,
        rootDeviceId,
        haDeviceId,
        haParentDeviceId,
        kind: inferBridgeDeviceKind(hass, entity, deviceId, inferred),
        label:
          deviceNameForEntity(registrySnapshot, entity.entity_id) ??
          bridgeDeviceNameForEntity(entity) ??
          entityFriendlyName(entity) ??
          deviceId ??
          entity.entity_id,
        roomLabel: areaNameForEntity(registrySnapshot, entity.entity_id) ?? "Unassigned",
      };
    })
    .filter(
      (
        item,
      ): item is BridgeDescriptor =>
        item.deviceId !== null &&
        item.rootDeviceId !== null &&
        item.kind !== null,
    );
}

function buildNvrChannelModel(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  descriptor: BridgeDescriptor,
): NvrChannelModel {
  const base = buildCameraDeviceBase(hass, registrySnapshot, descriptor);
  return {
    ...base,
    kind: "nvr_channel",
    channelNumber: bridgeChannelForEntity(base.cameraEntity) ?? parseChannelNumber(base.deviceId),
    detections: buildDetectionState(hass, registrySnapshot, base.deviceId),
  };
}

function buildIpcModel(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  descriptor: BridgeDescriptor,
): IpcModel {
  const base = buildCameraDeviceBase(hass, registrySnapshot, descriptor);
  return {
    ...base,
    kind: "ipc",
    detections: buildDetectionState(hass, registrySnapshot, base.deviceId),
  };
}

function buildVtoModel(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  descriptor: BridgeDescriptor,
): VtoModel {
  const base = buildCameraDeviceBase(hass, registrySnapshot, descriptor);
  const intercom = bridgeIntercomForEntity(base.cameraEntity);
  const vtoCapabilities = buildVtoCapabilities(intercom);
  const callStateRaw =
    sensorStateForDevice(hass, registrySnapshot, base.deviceId, "call_state")?.toLowerCase() ?? "";
  const doorbell = binaryStateForDevice(hass, registrySnapshot, base.deviceId, "doorbell");
  const callActive = binaryStateForDevice(hass, registrySnapshot, base.deviceId, "call");
  const online = base.online;

  let callState: VtoCallState = "idle";
  if (!online) {
    callState = "offline";
  } else if (callActive || callStateRaw.includes("talk")) {
    callState = "active";
  } else if (doorbell || callStateRaw.includes("ring")) {
    callState = "ringing";
  }

  const answerButtonEntityId =
    resolveEntityId(hass, registrySnapshot, "button", base.deviceId, "answer_call") ??
    buttonEntityId(base.deviceId, "answer_call");
  const hangupButtonEntityId =
    resolveEntityId(hass, registrySnapshot, "button", base.deviceId, "hangup_call") ??
    buttonEntityId(base.deviceId, "hangup_call");
  const outputVolumeEntityId =
    resolveEntityId(hass, registrySnapshot, "number", base.deviceId, "output_volume") ??
    numberEntityId(base.deviceId, "output_volume");
  const inputVolumeEntityId =
    resolveEntityId(hass, registrySnapshot, "number", base.deviceId, "input_volume") ??
    numberEntityId(base.deviceId, "input_volume");
  const mutedEntityId =
    resolveEntityId(hass, registrySnapshot, "switch", base.deviceId, "muted") ??
    switchEntityId(base.deviceId, "muted");
  const autoRecordEntityId =
    resolveEntityId(hass, registrySnapshot, "switch", base.deviceId, "auto_record_enabled") ??
    switchEntityId(base.deviceId, "auto_record_enabled");
  const subDevices = discoverVtoSubDevices(hass, registrySnapshot, descriptor, base, intercom);
  const locks = subDevices.filter((device): device is VtoLockModel => device.kind === "vto_lock");
  const alarms = subDevices.filter((device): device is VtoAlarmModel => device.kind === "vto_alarm");

  return {
    ...base,
    kind: "vto",
    doorbell,
    callActive,
    accessActive: binaryStateForDevice(hass, registrySnapshot, base.deviceId, "access"),
    tamper: binaryStateForDevice(hass, registrySnapshot, base.deviceId, "tamper"),
    callState,
    callStateText:
      callState === "active"
        ? "Active Call"
        : callState === "ringing"
          ? "Ringing"
          : callState === "offline"
            ? "Offline"
            : "Idle",
    lastCallSource:
      sensorStateForDevice(hass, registrySnapshot, base.deviceId, "last_call_source") ??
      "Front Door Station",
    lastCallStartedAt:
      sensorStateForDevice(hass, registrySnapshot, base.deviceId, "last_call_started_at") ?? "",
    lastCallEndedAt:
      sensorStateForDevice(hass, registrySnapshot, base.deviceId, "last_call_ended_at") ?? "",
    lastCallDurationSeconds: sensorNumberStateForDevice(
      hass,
      registrySnapshot,
      base.deviceId,
      "last_call_duration_seconds",
    ),
    outputVolume: entityNumberState(hass, outputVolumeEntityId),
    inputVolume: entityNumberState(hass, inputVolumeEntityId),
    muted: entityBooleanState(hass, mutedEntityId),
    autoRecordEnabled: entityBooleanState(hass, autoRecordEntityId),
    intercom,
    answerButtonEntityId,
    hasAnswerButtonEntity: entityById(hass, answerButtonEntityId) !== undefined,
    hangupButtonEntityId,
    hasHangupButtonEntity: entityById(hass, hangupButtonEntityId) !== undefined,
    outputVolumeEntityId,
    hasOutputVolumeEntity: entityById(hass, outputVolumeEntityId) !== undefined,
    inputVolumeEntityId,
    hasInputVolumeEntity: entityById(hass, inputVolumeEntityId) !== undefined,
    mutedEntityId,
    hasMutedEntity: entityById(hass, mutedEntityId) !== undefined,
    autoRecordEntityId,
    hasAutoRecordEntity: entityById(hass, autoRecordEntityId) !== undefined,
    lockCount: intercom?.lockUrls.length ?? 0,
    vtoCapabilities,
    locks,
    alarms,
    subDevices,
  };
}

function buildNvrModel(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
    deviceId: string,
    channels: NvrChannelModel[],
): NvrModel {
  const nvrChannels = channels.filter((channel) => channel.rootDeviceId === deviceId);
  const haDeviceId = preferredString(nvrChannels.map((channel) => channel.haParentDeviceId));
  const roomLabel = preferredRoomLabel(nvrChannels.map((channel) => channel.roomLabel), "Recorder");
  const label =
    rootDeviceName(registrySnapshot, nvrChannels[0]?.cameraEntityId ?? "") ??
    deviceNameForEntity(registrySnapshot, nvrChannels[0]?.cameraEntityId ?? "") ??
    deviceId;
  const drives = discoverNvrDrives(hass, registrySnapshot, deviceId, haDeviceId, roomLabel);
  const roomGroups = buildNvrRoomGroups(deviceId, nvrChannels);
  const storageUsedPercent =
    sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "used_percent") ??
    average(drives.map((drive) => drive.usedPercent));
  const totalBytes =
    sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "total_bytes") ??
    sum(drives.map((drive) => drive.totalBytes));
  const online =
    binaryStateForDevice(hass, registrySnapshot, deviceId, "online") ||
    nvrChannels.some((channel) => channel.online);

  return {
    kind: "nvr",
    deviceId,
    rootDeviceId: deviceId,
    haDeviceId,
    haParentDeviceId: null,
    label,
    roomLabel,
    online,
    bridgeBaseUrl: nvrChannels[0]?.bridgeBaseUrl ?? null,
    eventsUrl: nvrChannels[0]?.eventsUrl ?? null,
    metadata: buildDeviceMetadata(hass, registrySnapshot, deviceId),
    diagnostics: buildDeviceDiagnostics(hass, registrySnapshot, deviceId),
    channels: nvrChannels,
    childChannels: nvrChannels,
    roomGroups,
    drives,
    childDrives: drives,
    storageUsedPercent,
    totalBytes,
    healthy:
      !binaryStateForDevice(hass, registrySnapshot, deviceId, "disk_fault") &&
      drives.every((drive) => drive.healthy),
    recordingActive: nvrChannels.some(
      (channel) => channel.controls?.recording?.active === true,
    ),
  };
}

function buildCameraDeviceBase(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  descriptor: BridgeDescriptor,
): BridgeCameraDeviceBase {
  const cameraEntity = entityById(hass, descriptor.cameraEntityId);
  const controls = bridgeControlsForEntity(cameraEntity);
  const features = bridgeFeaturesForEntity(cameraEntity);
  const archiveAttributes = bridgeArchiveAttributesForEntity(cameraEntity);
  const capture = bridgeCaptureForEntity(cameraEntity);
  const streamSource =
    stringValue(cameraEntity?.attributes.stream_source) ??
    stringValue(cameraEntity?.attributes.snapshot_url);

  return {
    kind: descriptor.kind,
    deviceId: descriptor.deviceId,
    rootDeviceId: descriptor.rootDeviceId,
    haDeviceId: descriptor.haDeviceId,
    haParentDeviceId: descriptor.haParentDeviceId,
    label: descriptor.label,
    roomLabel: descriptor.roomLabel,
    cameraEntityId: descriptor.cameraEntityId,
    cameraEntity,
    online: binaryStateForDevice(hass, registrySnapshot, descriptor.deviceId, "online"),
    bridgeBaseUrl: bridgeBaseUrlForEntity(cameraEntity) ?? normalizeBridgeOrigin(streamSource),
    eventsUrl: bridgeEventsUrlForEntity(cameraEntity),
    metadata: buildDeviceMetadata(hass, registrySnapshot, descriptor.deviceId),
    diagnostics: buildDeviceDiagnostics(hass, registrySnapshot, descriptor.deviceId),
    media: {
      streamAvailable:
        binaryStateForDevice(hass, registrySnapshot, descriptor.deviceId, "stream_available") ||
        booleanAttribute(cameraEntity?.attributes.stream_available),
      streamSource,
      snapshotUrl: stringValue(cameraEntity?.attributes.snapshot_url),
      capture,
      previewUrl: stringValue(cameraEntity?.attributes.preview_url),
      localIntercomUrl: stringValue(cameraEntity?.attributes.bridge_local_intercom_url),
      onvifStreamUrl: stringValue(cameraEntity?.attributes.bridge_onvif_stream_url),
      onvifSnapshotUrl: stringValue(cameraEntity?.attributes.bridge_onvif_snapshot_url),
      recommendedProfile: stringValue(cameraEntity?.attributes.recommended_profile),
      recommendedHaIntegration:
        sensorStateForDevice(
          hass,
          registrySnapshot,
          descriptor.deviceId,
          "recommended_ha_integration",
        ) ?? null,
      preferredVideoProfile: stringValue(cameraEntity?.attributes.preferred_video_profile),
      preferredVideoSource: stringValue(cameraEntity?.attributes.preferred_video_source),
      resolution:
        sensorStateForDevice(hass, registrySnapshot, descriptor.deviceId, "main_resolution") ?? "-",
      codec: sensorStateForDevice(hass, registrySnapshot, descriptor.deviceId, "main_codec") ?? "-",
      frameRate:
        sensorStateForDevice(hass, registrySnapshot, descriptor.deviceId, "frame_rate") ?? "-",
      bitrate:
        sensorStateForDevice(hass, registrySnapshot, descriptor.deviceId, "bitrate") ?? "-",
      profile:
        sensorStateForDevice(hass, registrySnapshot, descriptor.deviceId, "recommended_profile") ??
        "auto",
      audioCodec:
        sensorStateForDevice(hass, registrySnapshot, descriptor.deviceId, "audio_codec") ?? "none",
      profiles: bridgeProfilesForEntity(cameraEntity),
    },
    controls,
    features,
    capabilities: buildCameraCapabilities(
      parseChannelNumber(descriptor.deviceId),
      controls,
      features,
      archiveAttributes,
    ),
  };
}

function discoverNvrDrives(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  nvrDeviceId: string,
  nvrHaDeviceId: string | null,
  roomLabel: string,
): NvrDriveModel[] {
  if (registrySnapshot && nvrHaDeviceId) {
    const drives = childRegistryDeviceIds(registrySnapshot, nvrHaDeviceId)
      .map((haDeviceId) => buildRegistryDriveModel(hass, registrySnapshot, haDeviceId, nvrDeviceId, roomLabel))
      .filter((drive): drive is NvrDriveModel => drive !== null)
      .sort((left, right) => left.label.localeCompare(right.label));
    if (drives.length > 0) {
      return drives;
    }
  }

  const diskIds = discoverChildDeviceIds(
    hass,
    new RegExp(`^[^.]+\\.(${escapeRegExp(nvrDeviceId)}_disk_\\d+)_`),
  );

  return [...diskIds]
    .sort((left, right) => left.localeCompare(right))
    .map((diskId) => {
      const slot = parseSuffixNumber(diskId, /_disk_(\d+)$/);
      return {
        kind: "nvr_disk",
        deviceId: diskId,
        rootDeviceId: nvrDeviceId,
        haDeviceId: null,
        haParentDeviceId: nvrHaDeviceId,
        label: `Drive ${slot ?? "--"}`,
        roomLabel,
        online: binaryStateForDevice(hass, registrySnapshot, diskId, "online"),
        bridgeBaseUrl: null,
        eventsUrl: null,
        metadata: buildDeviceMetadata(hass, registrySnapshot, diskId),
        diagnostics: buildDeviceDiagnostics(hass, registrySnapshot, diskId),
        slot,
        stateText: sensorStateForDevice(hass, registrySnapshot, diskId, "state") ?? null,
        totalBytes: sensorNumberStateForDevice(hass, registrySnapshot, diskId, "total_bytes"),
        usedBytes: sensorNumberStateForDevice(hass, registrySnapshot, diskId, "used_bytes"),
        usedPercent: sensorNumberStateForDevice(hass, registrySnapshot, diskId, "used_percent"),
        healthy: !binaryStateForDevice(hass, registrySnapshot, diskId, "disk_fault"),
      };
    });
}

function discoverVtoSubDevices(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  descriptor: BridgeDescriptor,
  vto: BridgeCameraDeviceBase,
  intercom: BridgeIntercomSummary | null,
): VtoSubDeviceModel[] {
  if (registrySnapshot && descriptor.haDeviceId) {
    const subDevices = childRegistryDeviceIds(registrySnapshot, descriptor.haDeviceId)
      .map((haDeviceId) =>
        buildRegistryVtoSubDeviceModel(
          hass,
          registrySnapshot,
          haDeviceId,
          vto,
          intercom,
        ),
      )
      .filter((device): device is VtoSubDeviceModel => device !== null)
      .sort((left, right) => left.label.localeCompare(right.label));
    if (subDevices.length > 0) {
      return subDevices;
    }
  }

  const lockIds = discoverChildDeviceIds(
    hass,
    new RegExp(`^[^.]+\\.(${escapeRegExp(vto.deviceId)}_lock_\\d+)_`),
  );
  for (const lockUrl of intercom?.lockUrls ?? []) {
    const match = lockUrl.match(/\/locks\/(\d+)\/unlock/);
    if (!match?.[1]) {
      continue;
    }
    const index = Number.parseInt(match[1], 10);
    if (!Number.isFinite(index) || index < 0) {
      continue;
    }
    lockIds.add(`${vto.deviceId}_lock_${String(index).padStart(2, "0")}`);
  }

  const alarmIds = discoverChildDeviceIds(
    hass,
    new RegExp(`^[^.]+\\.(${escapeRegExp(vto.deviceId)}_alarm_\\d+)_`),
  );

  const locks: VtoLockModel[] = [...lockIds]
    .sort((left, right) => left.localeCompare(right))
    .map((lockId) => buildVtoLockModel(hass, registrySnapshot, vto, intercom, lockId));
  const alarms: VtoAlarmModel[] = [...alarmIds]
    .sort((left, right) => left.localeCompare(right))
    .map((alarmId) => buildVtoAlarmModel(hass, registrySnapshot, vto, alarmId));

  return [...locks, ...alarms];
}

function buildVtoLockModel(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  vto: BridgeCameraDeviceBase,
  intercom: BridgeIntercomSummary | null,
  deviceId: string,
): VtoLockModel {
  const slot = parseSuffixNumber(deviceId, /_lock_(\d+)$/);
  const buttonKey = slot === null ? null : `unlock_${slot + 1}`;
  const unlockButtonEntityId = buttonKey
    ? resolveEntityId(hass, registrySnapshot, "button", vto.deviceId, buttonKey) ??
      buttonEntityId(vto.deviceId, buttonKey)
    : "";
  const unlockActionUrl =
    slot !== null && slot < (intercom?.lockUrls.length ?? 0)
      ? intercom?.lockUrls[slot] ?? null
      : null;

  return {
    kind: "vto_lock",
    deviceId,
    rootDeviceId: vto.deviceId,
    haDeviceId: null,
    haParentDeviceId: vto.haDeviceId,
    label: inferChildLabel(hass, deviceId, `Door ${(slot ?? 0) + 1}`),
    roomLabel: vto.roomLabel,
    online: binaryStateForDevice(hass, registrySnapshot, deviceId, "online") || vto.online,
    bridgeBaseUrl: vto.bridgeBaseUrl,
    eventsUrl: vto.eventsUrl,
    metadata: buildDeviceMetadata(hass, registrySnapshot, deviceId),
    diagnostics: buildDeviceDiagnostics(hass, registrySnapshot, deviceId),
    slot,
    stateText: sensorStateForDevice(hass, registrySnapshot, deviceId, "state") ?? null,
    sensorEnabled: binaryStateForDevice(hass, registrySnapshot, deviceId, "sensor_enabled"),
    lockMode: sensorStateForDevice(hass, registrySnapshot, deviceId, "lock_mode") ?? null,
    unlockHoldInterval:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "unlock_hold_interval") ?? null,
    unlockButtonEntityId,
    unlockActionUrl,
    hasUnlockButtonEntity:
      !!unlockButtonEntityId && entityById(hass, unlockButtonEntityId) !== undefined,
  };
}

function buildVtoAlarmModel(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  vto: BridgeCameraDeviceBase,
  deviceId: string,
): VtoAlarmModel {
  const slot = parseSuffixNumber(deviceId, /_alarm_(\d+)$/);
  return {
    kind: "vto_alarm",
    deviceId,
    rootDeviceId: vto.deviceId,
    haDeviceId: null,
    haParentDeviceId: vto.haDeviceId,
    label: inferChildLabel(hass, deviceId, `Alarm ${(slot ?? 0) + 1}`),
    roomLabel: vto.roomLabel,
    online: binaryStateForDevice(hass, registrySnapshot, deviceId, "online") || vto.online,
    bridgeBaseUrl: vto.bridgeBaseUrl,
    eventsUrl: vto.eventsUrl,
    metadata: buildDeviceMetadata(hass, registrySnapshot, deviceId),
    diagnostics: buildDeviceDiagnostics(hass, registrySnapshot, deviceId),
    slot,
    active: binaryStateForDevice(hass, registrySnapshot, deviceId, "active"),
    enabled: binaryStateForDevice(hass, registrySnapshot, deviceId, "enabled"),
    senseMethod: sensorStateForDevice(hass, registrySnapshot, deviceId, "sense_method") ?? null,
  };
}

function buildDetectionState(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  deviceId: string,
): BridgeDetectionState {
  return {
    motion: binaryStateForDevice(hass, registrySnapshot, deviceId, "motion"),
    human: binaryStateForDevice(hass, registrySnapshot, deviceId, "human"),
    vehicle: binaryStateForDevice(hass, registrySnapshot, deviceId, "vehicle"),
    tripwire: binaryStateForDevice(hass, registrySnapshot, deviceId, "tripwire"),
    intrusion: binaryStateForDevice(hass, registrySnapshot, deviceId, "intrusion"),
  };
}

function buildDeviceMetadata(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  deviceId: string,
): BridgeDeviceMetadataModel {
  return {
    manufacturer: sensorStateForDevice(hass, registrySnapshot, deviceId, "manufacturer") ?? null,
    model: sensorStateForDevice(hass, registrySnapshot, deviceId, "model") ?? null,
    serial: sensorStateForDevice(hass, registrySnapshot, deviceId, "serial") ?? null,
    firmware: sensorStateForDevice(hass, registrySnapshot, deviceId, "firmware") ?? null,
    buildDate: sensorStateForDevice(hass, registrySnapshot, deviceId, "build_date") ?? null,
  };
}

function buildDeviceDiagnostics(
  hass: HomeAssistant,
  registrySnapshot: RegistrySnapshot | null | undefined,
  deviceId: string,
): BridgeDeviceDiagnosticsModel {
  return {
    channel: sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "channel"),
    channelCount: sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "channel_count"),
    diskCount: sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "disk_count"),
    lockCount: sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "lock_count"),
    totalBytes: sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "total_bytes"),
    usedBytes: sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "used_bytes"),
    freeBytes: sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "free_bytes"),
    usedPercent: sensorNumberStateForDevice(hass, registrySnapshot, deviceId, "used_percent"),
    mainCodec: sensorStateForDevice(hass, registrySnapshot, deviceId, "main_codec") ?? null,
    mainResolution:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "main_resolution") ?? null,
    subCodec: sensorStateForDevice(hass, registrySnapshot, deviceId, "sub_codec") ?? null,
    subResolution:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "sub_resolution") ?? null,
    audioCodec: sensorStateForDevice(hass, registrySnapshot, deviceId, "audio_codec") ?? null,
    controlAudioAuthority:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "control_audio_authority") ?? null,
    controlAudioSemantic:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "control_audio_semantic") ?? null,
    nvrConfigWritable: optionalBinaryStateForDevice(
      hass,
      registrySnapshot,
      deviceId,
      "nvr_config_writable",
    ),
    nvrConfigReason:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "nvr_config_reason") ?? null,
    directIPCConfigured: optionalBinaryStateForDevice(
      hass,
      registrySnapshot,
      deviceId,
      "direct_ipc_configured",
    ),
    directIPCConfiguredIP:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "direct_ipc_configured_ip") ?? null,
    directIPCIP:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "direct_ipc_ip") ?? null,
    directIPCModel:
      sensorStateForDevice(hass, registrySnapshot, deviceId, "direct_ipc_model") ?? null,
    catalog: {
      recommendedProfile:
        sensorStateForDevice(hass, registrySnapshot, deviceId, "recommended_profile") ?? null,
      recommendedHaIntegration:
        sensorStateForDevice(
          hass,
          registrySnapshot,
          deviceId,
          "recommended_ha_integration",
        ) ?? null,
      recommendedHaReason:
        sensorStateForDevice(hass, registrySnapshot, deviceId, "recommended_ha_reason") ?? null,
    },
    onvif: {
      h264Available: optionalBinaryStateForDevice(
        hass,
        registrySnapshot,
        deviceId,
        "onvif_h264_available",
      ),
      profileName:
        sensorStateForDevice(hass, registrySnapshot, deviceId, "onvif_profile_name") ?? null,
      profileToken:
        sensorStateForDevice(hass, registrySnapshot, deviceId, "onvif_profile_token") ?? null,
    },
  };
}

function discoverChildDeviceIds(hass: HomeAssistant, pattern: RegExp): Set<string> {
  const deviceIds = new Set<string>();
  for (const entityId of Object.keys(hass.states)) {
    const match = entityId.match(pattern);
    if (match?.[1]) {
      deviceIds.add(match[1]);
    }
  }
  return deviceIds;
}

function childRegistryDeviceIds(
  snapshot: RegistrySnapshot,
  parentHaDeviceId: string,
): string[] {
  const childIds: string[] = [];
  for (const [haDeviceId, device] of snapshot.devicesById.entries()) {
    if (device.via_device_id === parentHaDeviceId) {
      childIds.push(haDeviceId);
    }
  }
  return childIds;
}

function buildRegistryDriveModel(
  hass: HomeAssistant,
  snapshot: RegistrySnapshot,
  haDeviceId: string,
  nvrDeviceId: string,
  fallbackRoomLabel: string,
): NvrDriveModel | null {
  const group = registryDeviceEntityGroup(snapshot, haDeviceId);
  if (!isDriveRegistryGroup(group)) {
    return null;
  }

  const deviceId = inferBridgeDeviceIdFromRegistryGroup(group) ?? haDeviceId;
  const slot = parseSuffixNumber(deviceId, /(?:_disk_|_sd)(\d+)$/);
  return {
    kind: "nvr_disk",
    deviceId,
    rootDeviceId: nvrDeviceId,
    haDeviceId,
    haParentDeviceId: group.deviceEntry?.via_device_id ?? null,
    label:
      deviceNameForDevice(snapshot, haDeviceId) ??
      inferChildLabel(hass, deviceId, `Drive ${slot ?? "--"}`),
    roomLabel: areaNameForDevice(snapshot, haDeviceId) ?? fallbackRoomLabel,
    online: registryGroupBooleanValue(hass, group, "online") ?? false,
    bridgeBaseUrl: null,
    eventsUrl: null,
    metadata: buildDeviceMetadata(hass, snapshot, deviceId),
    diagnostics: buildDeviceDiagnostics(hass, snapshot, deviceId),
    slot,
    stateText: registryGroupStringValue(hass, group, "state"),
    totalBytes: registryGroupNumberValue(hass, group, "total_bytes"),
    usedBytes: registryGroupNumberValue(hass, group, "used_bytes"),
    usedPercent: registryGroupNumberValue(hass, group, "used_percent"),
    healthy: !(registryGroupBooleanValue(hass, group, "disk_fault") ?? false),
  };
}

function buildRegistryVtoSubDeviceModel(
  hass: HomeAssistant,
  snapshot: RegistrySnapshot,
  haDeviceId: string,
  vto: BridgeCameraDeviceBase,
  intercom: BridgeIntercomSummary | null,
): VtoSubDeviceModel | null {
  const group = registryDeviceEntityGroup(snapshot, haDeviceId);
  const deviceId = inferBridgeDeviceIdFromRegistryGroup(group) ?? haDeviceId;
  const roomLabel = areaNameForDevice(snapshot, haDeviceId) ?? vto.roomLabel;
  const label = deviceNameForDevice(snapshot, haDeviceId);

  if (isVtoLockRegistryGroup(group)) {
    const slot = parseSuffixNumber(deviceId, /_lock_(\d+)$/);
    const buttonKey = slot === null ? null : `unlock_${slot + 1}`;
    const unlockButtonEntityId = buttonKey
      ? resolveEntityId(hass, snapshot, "button", vto.deviceId, buttonKey) ??
        buttonEntityId(vto.deviceId, buttonKey)
      : "";
    const unlockActionUrl =
      slot !== null && slot < (intercom?.lockUrls.length ?? 0)
        ? intercom?.lockUrls[slot] ?? null
        : null;

    return {
      kind: "vto_lock",
      deviceId,
      rootDeviceId: vto.deviceId,
      haDeviceId,
      haParentDeviceId: group.deviceEntry?.via_device_id ?? vto.haDeviceId,
      label: label ?? inferChildLabel(hass, deviceId, `Door ${(slot ?? 0) + 1}`),
      roomLabel,
      online: (registryGroupBooleanValue(hass, group, "online") ?? false) || vto.online,
      bridgeBaseUrl: vto.bridgeBaseUrl,
      eventsUrl: vto.eventsUrl,
      metadata: buildDeviceMetadata(hass, snapshot, deviceId),
      diagnostics: buildDeviceDiagnostics(hass, snapshot, deviceId),
      slot,
      stateText: registryGroupStringValue(hass, group, "state"),
      sensorEnabled: registryGroupBooleanValue(hass, group, "sensor_enabled") ?? false,
      lockMode: registryGroupStringValue(hass, group, "lock_mode"),
      unlockHoldInterval: registryGroupStringValue(hass, group, "unlock_hold_interval"),
      unlockButtonEntityId,
      unlockActionUrl,
      hasUnlockButtonEntity:
        !!unlockButtonEntityId && entityById(hass, unlockButtonEntityId) !== undefined,
    };
  }

  if (isVtoAlarmRegistryGroup(group)) {
    const slot = parseSuffixNumber(deviceId, /_alarm_(\d+)$/);
    return {
      kind: "vto_alarm",
      deviceId,
      rootDeviceId: vto.deviceId,
      haDeviceId,
      haParentDeviceId: group.deviceEntry?.via_device_id ?? vto.haDeviceId,
      label: label ?? inferChildLabel(hass, deviceId, `Alarm ${(slot ?? 0) + 1}`),
      roomLabel,
      online: (registryGroupBooleanValue(hass, group, "online") ?? false) || vto.online,
      bridgeBaseUrl: vto.bridgeBaseUrl,
      eventsUrl: vto.eventsUrl,
      metadata: buildDeviceMetadata(hass, snapshot, deviceId),
      diagnostics: buildDeviceDiagnostics(hass, snapshot, deviceId),
      slot,
      active: registryGroupBooleanValue(hass, group, "active") ?? false,
      enabled: registryGroupBooleanValue(hass, group, "enabled") ?? false,
      senseMethod: registryGroupStringValue(hass, group, "sense_method"),
    };
  }

  return null;
}

interface RegistryDeviceEntityGroup {
  haDeviceId: string;
  deviceEntry: DeviceRegistryEntry | undefined;
  entityEntries: EntityRegistryEntry[];
}

function registryDeviceEntityGroup(
  snapshot: RegistrySnapshot,
  haDeviceId: string,
): RegistryDeviceEntityGroup {
  const entityEntries = [...snapshot.entitiesByEntityId.values()].filter(
    (entry) => entry.device_id === haDeviceId,
  );
  return {
    haDeviceId,
    deviceEntry: snapshot.devicesById.get(haDeviceId),
    entityEntries,
  };
}

function inferBridgeDeviceIdFromRegistryGroup(
  group: RegistryDeviceEntityGroup,
): string | null {
  const uniqueIds = group.entityEntries
    .map((entry) => stringValue(entry.unique_id))
    .filter((value): value is string => value !== null);
  if (uniqueIds.length === 0) {
    return null;
  }
  if (uniqueIds.length === 1) {
    return uniqueIds[0]?.replace(/_(camera|online|active|enabled|state|total_bytes|used_bytes|used_percent|disk_fault|sensor_enabled|lock_mode|unlock_hold_interval|sense_method|call_state|doorbell|call|access|tamper|last_call_source|last_call_started_at|last_call_ended_at|last_call_duration_seconds)$/, "") ?? null;
  }
  const prefix = longestCommonPrefix(uniqueIds).replace(/_+$/, "");
  return prefix || null;
}

function longestCommonPrefix(values: string[]): string {
  if (values.length === 0) {
    return "";
  }
  let prefix = values[0] ?? "";
  for (const value of values.slice(1)) {
    while (prefix && !value.startsWith(prefix)) {
      prefix = prefix.slice(0, -1);
    }
    if (!prefix) {
      return "";
    }
  }
  return prefix;
}

function registryGroupBooleanValue(
  hass: HomeAssistant,
  group: RegistryDeviceEntityGroup,
  suffix: string,
): boolean | null {
  const state = registryGroupStateValue(hass, group, suffix);
  if (state === undefined) {
    return null;
  }
  return state === "on" || state === "true";
}

function registryGroupNumberValue(
  hass: HomeAssistant,
  group: RegistryDeviceEntityGroup,
  suffix: string,
): number | null {
  const state = registryGroupStateValue(hass, group, suffix);
  if (!state) {
    return null;
  }
  const parsed = Number.parseFloat(state);
  return Number.isFinite(parsed) ? parsed : null;
}

function registryGroupStringValue(
  hass: HomeAssistant,
  group: RegistryDeviceEntityGroup,
  suffix: string,
): string | null {
  const state = registryGroupStateValue(hass, group, suffix);
  return state?.trim() ? state : null;
}

function registryGroupStateValue(
  hass: HomeAssistant,
  group: RegistryDeviceEntityGroup,
  suffix: string,
): string | undefined {
  const entry = group.entityEntries.find((candidate) =>
    stringValue(candidate.unique_id)?.endsWith(`_${suffix}`),
  );
  return entry ? hass.states[entry.entity_id]?.state : undefined;
}

function isDriveRegistryGroup(group: RegistryDeviceEntityGroup): boolean {
  return (
    hasRegistryGroupSuffix(group, "total_bytes") ||
    hasRegistryGroupSuffix(group, "used_bytes") ||
    hasRegistryGroupSuffix(group, "used_percent")
  );
}

function isVtoLockRegistryGroup(group: RegistryDeviceEntityGroup): boolean {
  return (
    hasRegistryGroupSuffix(group, "lock_mode") ||
    hasRegistryGroupSuffix(group, "unlock_hold_interval") ||
    hasRegistryGroupSuffix(group, "sensor_enabled")
  );
}

function isVtoAlarmRegistryGroup(group: RegistryDeviceEntityGroup): boolean {
  return (
    hasRegistryGroupSuffix(group, "sense_method") ||
    (hasRegistryGroupSuffix(group, "enabled") && hasRegistryGroupSuffix(group, "active"))
  );
}

function hasRegistryGroupSuffix(
  group: RegistryDeviceEntityGroup,
  suffix: string,
): boolean {
  return group.entityEntries.some((entry) =>
    stringValue(entry.unique_id)?.endsWith(`_${suffix}`) === true,
  );
}

function compareCameraDevices(left: CameraDeviceModel, right: CameraDeviceModel): number {
  return (
    left.rootDeviceId.localeCompare(right.rootDeviceId) ||
    left.roomLabel.localeCompare(right.roomLabel) ||
    (parseChannelNumber(left.deviceId) ?? Number.MAX_SAFE_INTEGER) -
      (parseChannelNumber(right.deviceId) ?? Number.MAX_SAFE_INTEGER) ||
    left.label.localeCompare(right.label)
  );
}

function buildCameraCapabilities(
  channelNumber: number | null,
  controls: BridgeControlsSummary | null,
  features: BridgeFeatureSummary[],
  archiveAttributes: BridgeArchiveAttributeSummary,
): CameraCapabilityModel {
  const archiveFeatures = withArchiveFallbackFeatures(features, archiveAttributes);
  return {
    archive: buildCameraArchiveCapabilities(channelNumber, archiveFeatures),
    ptz: controls?.ptz ?? null,
    aux: buildCameraAuxCapabilities(controls?.aux ?? null, features),
    recording: controls?.recording ?? null,
    audio: {
      supported: controls?.audio?.supported === true,
      mute: controls?.audio?.mute === true,
      volume: controls?.audio?.volume === true,
      volumePermissionDenied: controls?.audio?.volumePermissionDenied === true,
      playback: {
        supported: controls?.audio?.playbackSupported === true,
        siren: controls?.audio?.playbackSiren === true,
        quickReply: controls?.audio?.playbackQuickReply === true,
        formats: controls?.audio?.playbackFormats ?? [],
        fileCount: controls?.audio?.playbackFileCount ?? null,
      },
    },
    validationNotes: controls?.validationNotes ?? [],
  };
}

function buildVtoCapabilities(
  intercom: BridgeIntercomSummary | null,
): VtoCapabilityModel {
  return {
    answerSupported: intercom?.supportsVtoCallAnswer === true && !!intercom.answerUrl,
    hangupSupported: intercom?.supportsHangup === true && !!intercom.hangupUrl,
    unlockSupported: intercom?.supportsUnlock === true && intercom.lockUrls.length > 0,
    resetSupported:
      intercom?.supportsBridgeSessionReset === true && !!intercom.bridgeSessionResetUrl,
    browserMicrophoneSupported: intercom?.supportsBrowserMicrophone === true,
    bridgeAudioUplinkSupported: intercom?.supportsBridgeAudioUplink === true,
    bridgeAudioOutputSupported: intercom?.supportsBridgeAudioOutput === true,
    externalAudioExportSupported: intercom?.supportsExternalAudioExport === true,
    outputVolumeSupported:
      intercom?.supportsVtoOutputVolumeControl === true && !!intercom.outputVolumeUrl,
    inputVolumeSupported:
      intercom?.supportsVtoInputVolumeControl === true && !!intercom.inputVolumeUrl,
    muteSupported: intercom?.supportsVtoMuteControl === true && !!intercom.muteUrl,
    recordingSupported:
      intercom?.supportsVtoRecordingControl === true && !!intercom.recordingUrl,
    talkbackSupported: intercom?.supportsVtoTalkback === true,
    fullCallAcceptanceSupported: intercom?.supportsFullCallAcceptance === true,
    resetUrl: intercom?.bridgeSessionResetUrl ?? null,
    enableExternalUplinkUrl: intercom?.externalUplinkEnableUrl ?? null,
    disableExternalUplinkUrl: intercom?.externalUplinkDisableUrl ?? null,
    validationNotes: intercom?.validationNotes ?? [],
  };
}

function bridgeFeatureByKey(
  features: BridgeFeatureSummary[],
  key: string,
): BridgeFeatureSummary | null {
  for (const feature of features) {
    if (feature.key === key) {
      return feature;
    }
  }
  return null;
}

function buildNvrRoomGroups(
  nvrDeviceId: string,
  channels: NvrChannelModel[],
): NvrRoomGroupModel[] {
  const rooms = new Map<string, NvrChannelModel[]>();
  for (const channel of channels) {
    const existing = rooms.get(channel.roomLabel) ?? [];
    existing.push(channel);
    rooms.set(channel.roomLabel, existing);
  }

  return [...rooms.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([label, roomChannels]) => {
      const sortedChannels = [...roomChannels].sort(compareCameraDevices);
      return {
        id: `${nvrDeviceId}:${label}`,
        label,
        nvrDeviceId,
        channels: sortedChannels,
        onlineCount: sortedChannels.filter((channel) => channel.online).length,
        alertCount: sortedChannels.filter((channel) => hasDetections(channel)).length,
      };
    });
}

function buildSiteRoomGroups(
  cameras: CameraDeviceModel[],
  vtos: VtoModel[],
): SiteRoomGroupModel[] {
  const groups = new Map<
    string,
    {
      cameras: CameraDeviceModel[];
      nvrChannels: NvrChannelModel[];
      ipcs: IpcModel[];
      vtos: VtoModel[];
    }
  >();

  const ensureGroup = (label: string) => {
    const existing = groups.get(label);
    if (existing) {
      return existing;
    }
    const next = {
      cameras: [] as CameraDeviceModel[],
      nvrChannels: [] as NvrChannelModel[],
      ipcs: [] as IpcModel[],
      vtos: [] as VtoModel[],
    };
    groups.set(label, next);
    return next;
  };

  for (const camera of cameras) {
    const group = ensureGroup(camera.roomLabel);
    group.cameras.push(camera);
    if (camera.kind === "nvr_channel") {
      group.nvrChannels.push(camera);
    } else {
      group.ipcs.push(camera);
    }
  }

  for (const vto of vtos) {
    ensureGroup(vto.roomLabel).vtos.push(vto);
  }

  return [...groups.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([label, group]) => ({
      id: label,
      label,
      cameras: [...group.cameras].sort(compareCameraDevices),
      nvrChannels: [...group.nvrChannels].sort(compareCameraDevices),
      ipcs: [...group.ipcs].sort(compareCameraDevices),
      vtos: [...group.vtos].sort((left, right) => left.label.localeCompare(right.label)),
    }));
}

function hasDetections(camera: CameraDeviceModel): boolean {
  return (
    camera.detections.motion ||
    camera.detections.human ||
    camera.detections.vehicle ||
    camera.detections.tripwire ||
    camera.detections.intrusion
  );
}

function inferChildLabel(
  hass: HomeAssistant,
  deviceId: string,
  fallback: string,
): string {
  for (const domain of ["sensor", "binary_sensor", "button"]) {
    for (const entityId of Object.keys(hass.states)) {
      if (!entityId.startsWith(`${domain}.${deviceId}_`)) {
        continue;
      }
      const label = entityFriendlyName(hass.states[entityId]);
      if (label && label !== entityId) {
        return label.replace(/\s+(State|Enabled|Online)$/i, "").trim();
      }
    }
  }
  return fallback;
}

function bridgeFeaturesForEntity(entity: HassEntity | undefined): BridgeFeatureSummary[] {
  const value = entity?.attributes.bridge_features;
  if (!Array.isArray(value)) {
    return [];
  }

  return value
    .filter(isObject)
    .map((feature) => {
      const typed = feature as BridgeFeatureShape;
      return {
        key: stringValue(typed.key),
        label: stringValue(typed.label),
        group: stringValue(typed.group),
        kind: stringValue(typed.kind),
        url: stringValue(typed.url),
        supported: booleanValue(typed.supported),
        parameterKey: stringValue(typed.parameter_key),
        parameterValue: stringValue(typed.parameter_value),
        commands: stringArray(typed.commands),
        actions: stringArray(typed.actions),
        targets: Array.isArray(typed.targets)
          ? typed.targets.filter(isObject).map((target) => ({
              key: stringValue(target.key),
              label: stringValue(target.label),
              url: stringValue(target.url),
            }))
          : [],
        allowedValues: numberArray(typed.allowed_values),
        minValue: numberValue(typed.min_value),
        maxValue: numberValue(typed.max_value),
        stepValue: numberValue(typed.step_value),
        currentValue: numberValue(typed.current_value),
        active: booleanOrNull(typed.active),
        currentText: stringValue(typed.current_text),
      };
    });
}

function bridgeArchiveAttributesForEntity(
  entity: HassEntity | undefined,
): BridgeArchiveAttributeSummary {
  return {
    searchUrl: normalizeArchiveSearchUrlTemplate(
      stringValue(entity?.attributes.bridge_archive_recordings_url_template),
    ),
    playbackUrl: stringValue(entity?.attributes.bridge_playback_sessions_url),
  };
}

function withArchiveFallbackFeatures(
  features: BridgeFeatureSummary[],
  archiveAttributes: BridgeArchiveAttributeSummary,
): BridgeFeatureSummary[] {
  const merged = [...features];

  if (
    archiveAttributes.searchUrl &&
    !merged.some((feature) => feature.key === "archive_search" && feature.url)
  ) {
    merged.push({
      key: "archive_search",
      label: "Recordings",
      group: "archive",
      kind: "query",
      url: archiveAttributes.searchUrl,
      supported: true,
      parameterKey: null,
      parameterValue: null,
      commands: [],
      actions: [],
      targets: [],
      allowedValues: [],
      minValue: null,
      maxValue: null,
      stepValue: null,
      currentValue: null,
      active: null,
      currentText: null,
    });
  }

  if (
    archiveAttributes.playbackUrl &&
    !merged.some((feature) => feature.key === "archive_playback" && feature.url)
  ) {
    merged.push({
      key: "archive_playback",
      label: "Playback",
      group: "archive",
      kind: "session",
      url: archiveAttributes.playbackUrl,
      supported: true,
      parameterKey: null,
      parameterValue: null,
      commands: [],
      actions: [],
      targets: [],
      allowedValues: [],
      minValue: null,
      maxValue: null,
      stepValue: null,
      currentValue: null,
      active: null,
      currentText: null,
    });
  }

  return merged;
}

function bridgeProfilesForEntity(
  entity: HassEntity | undefined,
): Record<string, BridgeStreamProfileModel> {
  const value = entity?.attributes.bridge_profiles;
  if (!isObject(value)) {
    return {};
  }

  const profiles: Record<string, BridgeStreamProfileModel> = {};
  for (const [profileKey, rawProfile] of Object.entries(value)) {
    if (!isObject(rawProfile)) {
      continue;
    }

    const typed = rawProfile as BridgeProfileShape;
    const normalizedKey = profileKey.trim();
    if (!normalizedKey) {
      continue;
    }

    profiles[normalizedKey] = {
      name: stringValue(typed.name) ?? normalizedKey,
      streamUrl: stringValue(typed.stream_url),
      localMjpegUrl: stringValue(typed.local_mjpeg_url),
      localHlsUrl: stringValue(typed.local_hls_url),
      localDashUrl: stringValue(typed.local_dash_url),
      localWebRtcUrl: stringValue(typed.local_webrtc_url),
      subtype: numberValue(typed.subtype),
      rtspTransport: stringValue(typed.rtsp_transport),
      frameRate: numberValue(typed.frame_rate),
      sourceWidth: numberValue(typed.source_width),
      sourceHeight: numberValue(typed.source_height),
      useWallclockAsTimestamps: booleanValue(typed.use_wallclock_as_timestamps),
      recommended: booleanValue(typed.recommended),
    };
  }

  return profiles;
}

function bridgeCaptureForEntity(
  entity: HassEntity | undefined,
): BridgeCaptureSummary | null {
  const captureValue = entity?.attributes.bridge_capture;
  const typed = isObject(captureValue) ? (captureValue as BridgeCaptureShape) : null;

  const snapshotUrl =
    stringValue(typed?.snapshot_url) ??
    stringValue(entity?.attributes.bridge_capture_snapshot_url) ??
    null;
  const startRecordingUrl =
    stringValue(entity?.attributes.bridge_start_recording_url) ??
    stringValue(typed?.start_recording_url) ??
    null;
  const stopRecordingUrl =
    stringValue(entity?.attributes.bridge_stop_recording_url) ??
    stringValue(typed?.stop_recording_url) ??
    null;
  const recordingsUrl =
    stringValue(entity?.attributes.bridge_recordings_url) ??
    stringValue(typed?.recordings_url) ??
    null;
  const recordingActive =
    booleanAttribute(entity?.attributes.bridge_recording_active) ||
    typed?.active === true;
  const activeClipId = stringValue(typed?.active_clip_id);
  const activeClipDownloadUrl = stringValue(typed?.active_clip_download_url);

  if (
    !snapshotUrl &&
    !startRecordingUrl &&
    !stopRecordingUrl &&
    !recordingsUrl &&
    !recordingActive &&
    !activeClipId &&
    !activeClipDownloadUrl
  ) {
    return null;
  }

  return {
    snapshotUrl,
    recordingActive,
    startRecordingUrl,
    stopRecordingUrl,
    recordingsUrl,
    activeClipId,
    activeClipDownloadUrl,
  };
}

function bridgeControlsForEntity(
  entity: HassEntity | undefined,
): BridgeControlsSummary | null {
  const value = entity?.attributes.bridge_controls;
  if (!isObject(value)) {
    return null;
  }

  const typed = value as BridgeControlsShape;
  return {
    ptz: isObject(typed.ptz)
      ? {
          url: stringValue(typed.ptz.url),
          supported: booleanValue(typed.ptz.supported),
          pan: booleanValue(typed.ptz.pan),
          tilt: booleanValue(typed.ptz.tilt),
          zoom: booleanValue(typed.ptz.zoom),
          focus: booleanValue(typed.ptz.focus),
          moveRelatively: booleanValue(typed.ptz.move_relatively),
          autoScan: booleanValue(typed.ptz.auto_scan),
          preset: booleanValue(typed.ptz.preset),
          pattern: booleanValue(typed.ptz.pattern),
          tour: booleanValue(typed.ptz.tour),
          commands: stringArray(typed.ptz.commands),
        }
      : null,
    aux: isObject(typed.aux)
      ? {
          url: stringValue(typed.aux.url),
          supported: booleanValue(typed.aux.supported),
          outputs: stringArray(typed.aux.outputs),
          features: stringArray(typed.aux.features),
        }
      : null,
    audio: isObject(typed.audio)
      ? {
          supported: booleanValue(typed.audio.supported),
          mute: booleanValue(typed.audio.mute),
          volume: booleanValue(typed.audio.volume),
          volumePermissionDenied: booleanValue(typed.audio.volume_permission_denied),
          playbackSupported: booleanValue(typed.audio.playback_supported),
          playbackSiren: booleanValue(typed.audio.playback_siren),
          playbackQuickReply: booleanValue(typed.audio.playback_quick_reply),
          playbackFormats: stringArray(typed.audio.playback_formats),
          playbackFileCount: numberValue(typed.audio.playback_file_count),
        }
      : null,
    recording: isObject(typed.recording)
      ? {
          url: stringValue(typed.recording.url),
          supported: booleanValue(typed.recording.supported),
          active: booleanValue(typed.recording.active),
          mode: stringValue(typed.recording.mode),
        }
      : null,
    validationNotes: stringArray(typed.validation_notes),
  };
}

function bridgeIntercomForEntity(
  entity: HassEntity | undefined,
): BridgeIntercomSummary | null {
  const value = entity?.attributes.bridge_intercom;
  if (!isObject(value)) {
    return null;
  }

  const typed = value as BridgeIntercomShape;
  return {
    callState: stringValue(typed.call_state),
    lastRingAt: stringValue(typed.last_ring_at),
    lastCallStartedAt: stringValue(typed.last_call_started_at),
    lastCallEndedAt: stringValue(typed.last_call_ended_at),
    lastCallSource: stringValue(typed.last_call_source),
    lastCallDurationSeconds: numberValue(typed.last_call_duration_seconds),
    answerUrl: stringValue(typed.answer_url),
    hangupUrl: stringValue(typed.hangup_url),
    bridgeSessionResetUrl: stringValue(typed.bridge_session_reset_url),
    lockUrls: stringArray(typed.lock_urls),
    externalUplinkEnableUrl: stringValue(typed.external_uplink_enable_url),
    externalUplinkDisableUrl: stringValue(typed.external_uplink_disable_url),
    outputVolumeUrl: stringValue(typed.output_volume_url),
    inputVolumeUrl: stringValue(typed.input_volume_url),
    muteUrl: stringValue(typed.mute_url),
    recordingUrl: stringValue(typed.recording_url),
    bridgeSessionActive: booleanValue(typed.bridge_session_active),
    bridgeSessionCount: numberValue(typed.bridge_session_count),
    externalUplinkEnabled: booleanValue(typed.external_uplink_enabled),
    bridgeUplinkActive: booleanValue(typed.bridge_uplink_active),
    bridgeUplinkCodec: stringValue(typed.bridge_uplink_codec),
    bridgeUplinkPackets: numberValue(typed.bridge_uplink_packets),
    bridgeForwardedPackets: numberValue(typed.bridge_forwarded_packets),
    bridgeForwardErrors: numberValue(typed.bridge_forward_errors),
    supportsHangup: booleanValue(typed.supports_hangup),
    supportsBridgeSessionReset: booleanValue(typed.supports_bridge_session_reset),
    supportsUnlock: booleanValue(typed.supports_unlock),
    supportsBrowserMicrophone: booleanValue(typed.supports_browser_microphone),
    supportsBridgeAudioUplink: booleanValue(typed.supports_bridge_audio_uplink),
    supportsBridgeAudioOutput: booleanValue(typed.supports_bridge_audio_output),
    supportsExternalAudioExport: booleanValue(typed.supports_external_audio_export),
    configuredExternalUplinkTargetCount: numberValue(
      typed.configured_external_uplink_target_count,
    ),
    supportsVtoCallAnswer: booleanValue(typed.supports_vto_call_answer),
    supportsVtoOutputVolumeControl: booleanValue(typed.supports_vto_output_volume_control),
    supportsVtoInputVolumeControl: booleanValue(typed.supports_vto_input_volume_control),
    supportsVtoMuteControl: booleanValue(typed.supports_vto_mute_control),
    supportsVtoRecordingControl: booleanValue(typed.supports_vto_recording_control),
    supportsVtoTalkback: booleanValue(typed.supports_vto_talkback),
    supportsFullCallAcceptance: booleanValue(typed.supports_full_call_acceptance),
    outputVolumeLevel: numberValue(typed.output_volume_level),
    outputVolumeLevels: numberArray(typed.output_volume_levels),
    inputVolumeLevel: numberValue(typed.input_volume_level),
    inputVolumeLevels: numberArray(typed.input_volume_levels),
    muted: booleanValue(typed.muted),
    autoRecordEnabled: booleanValue(typed.auto_record_enabled),
    autoRecordTimeSeconds: numberValue(typed.auto_record_time_seconds),
    streamAudioEnabled: booleanValue(typed.stream_audio_enabled),
    validationNotes: stringArray(typed.validation_notes),
  };
}

function bridgeBaseUrlForEntity(entity: HassEntity | undefined): string | null {
  return stringValue(entity?.attributes.bridge_base_url);
}

function bridgeEventsUrlForEntity(entity: HassEntity | undefined): string | null {
  return stringValue(entity?.attributes.bridge_events_url);
}

function bridgeDeviceIdForEntity(entity: HassEntity | undefined): string | null {
  return stringValue(entity?.attributes.bridge_device_id);
}

function bridgeRootDeviceIdForEntity(entity: HassEntity | undefined): string | null {
  return stringValue(entity?.attributes.bridge_root_device_id) ?? bridgeDeviceIdForEntity(entity);
}

function bridgeDeviceKindForEntity(entity: HassEntity | undefined): string | null {
  return stringValue(entity?.attributes.bridge_device_kind);
}

function bridgeDeviceNameForEntity(entity: HassEntity | undefined): string | null {
  return stringValue(entity?.attributes.bridge_device_name);
}

function bridgeChannelForEntity(entity: HassEntity | undefined): number | null {
  const value = entity?.attributes.bridge_channel;
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : null;
}

function inferDeviceIdFromCameraEntityId(entityId: string): string | null {
  const match = entityId.match(/^camera\.(.+)_camera$/);
  return match?.[1] ?? null;
}

function inferRootDeviceId(deviceId: string | null): string | null {
  if (!deviceId) {
    return null;
  }

  const channelMatch = deviceId.match(/^(.*)_channel_\d+$/);
  if (channelMatch?.[1]) {
    return channelMatch[1];
  }

  return deviceId;
}

function inferBridgeDeviceKind(
  hass: HomeAssistant,
  entity: HassEntity,
  deviceId: string | null,
  inferred?: InferredBridgeDescriptor | null,
): "nvr_channel" | "ipc" | "vto" | null {
  const explicit = bridgeDeviceKindForEntity(entity);
  if (explicit === "nvr_channel" || explicit === "ipc" || explicit === "vto") {
    return explicit;
  }
  if (inferred) {
    return inferred.kind;
  }
  if (!deviceId) {
    return null;
  }

  if (parseChannelNumber(deviceId) !== null) {
    return "nvr_channel";
  }

  if (
    entityById(hass, sensorEntityId(deviceId, "call_state")) ||
    entityById(hass, binarySensorEntityId(deviceId, "doorbell")) ||
    bridgeIntercomForEntity(entity)
  ) {
    return "vto";
  }

  return "ipc";
}

function inferBridgeDescriptor(
  hass: HomeAssistant,
  entity: HassEntity,
): InferredBridgeDescriptor | null {
  const bridgePath = firstBridgeResourcePath(
    stringValue(entity.attributes.snapshot_url),
    stringValue(entity.attributes.stream_source),
  );
  if (!bridgePath) {
    return null;
  }

  const nvrMatch = bridgePath.match(/\/api\/v1\/nvr\/([^/]+)\/channels\/(\d+)\//);
  if (nvrMatch) {
    const rootDeviceId = nvrMatch[1];
    const channel = Number.parseInt(nvrMatch[2], 10);
    if (!Number.isFinite(channel) || channel <= 0) {
      return null;
    }

    const deviceId =
      findChannelDeviceId(hass, rootDeviceId, channel) ??
      formatChannelDeviceId(rootDeviceId, bridgeChannelForEntity(entity) ?? channel);
    return {
      deviceId,
      rootDeviceId,
      kind: "nvr_channel",
    };
  }

  const ipcMatch = bridgePath.match(/\/api\/v1\/ipc\/([^/]+)\//);
  if (ipcMatch) {
    const deviceId = ipcMatch[1];
    return {
      deviceId,
      rootDeviceId: deviceId,
      kind: "ipc",
    };
  }

  const vtoMatch = bridgePath.match(/\/api\/v1\/vto\/([^/]+)\//);
  if (vtoMatch) {
    const deviceId = vtoMatch[1];
    return {
      deviceId,
      rootDeviceId: deviceId,
      kind: "vto",
    };
  }

  return null;
}

function firstBridgeResourcePath(...values: Array<string | null>): string | null {
  for (const value of values) {
    if (!value) {
      continue;
    }

    try {
      return new URL(value).pathname;
    } catch {
      if (value.startsWith("/")) {
        return value;
      }
    }
  }

  return null;
}

function findChannelDeviceId(
  hass: HomeAssistant,
  rootDeviceId: string,
  channel: number,
): string | null {
  const needle = `${rootDeviceId}_channel_`;
  for (const entityId of Object.keys(hass.states)) {
    const candidate = inferDeviceIdFromEntityId(entityId);
    if (!candidate || !candidate.startsWith(needle)) {
      continue;
    }
    if (parseChannelNumber(candidate) === channel) {
      return candidate;
    }
  }

  return null;
}

function inferDeviceIdFromEntityId(entityId: string): string | null {
  const match = entityId.match(/^[^.]+\.(.+?)(?:_[a-z0-9]+)+$/);
  return match?.[1] ?? null;
}

function formatChannelDeviceId(rootDeviceId: string, channel: number): string {
  return `${rootDeviceId}_channel_${String(channel).padStart(2, "0")}`;
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function parseSuffixNumber(deviceId: string, pattern: RegExp): number | null {
  const match = deviceId.match(pattern);
  if (!match?.[1]) {
    return null;
  }
  const parsed = Number.parseInt(match[1], 10);
  return Number.isFinite(parsed) ? parsed : null;
}

function sum(values: Array<number | null>): number | null {
  const filtered = values.filter((value): value is number => value !== null);
  if (filtered.length === 0) {
    return null;
  }
  return filtered.reduce((total, value) => total + value, 0);
}

function average(values: Array<number | null>): number | null {
  const filtered = values.filter((value): value is number => value !== null);
  if (filtered.length === 0) {
    return null;
  }
  return filtered.reduce((total, value) => total + value, 0) / filtered.length;
}

function preferredString(values: Array<string | null | undefined>): string | null {
  for (const value of values) {
    if (!value) {
      continue;
    }
    const text = value.trim();
    if (!text) {
      continue;
    }
    return text;
  }
  return null;
}

function preferredRoomLabel(
  values: Array<string | null | undefined>,
  fallback: string,
): string {
  for (const value of values) {
    if (!value) {
      continue;
    }
    const text = value.trim();
    if (!text || text === "Unassigned") {
      continue;
    }
    return text;
  }
  return fallback;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function stringValue(value: unknown): string | null {
  return typeof value === "string" && value.trim() ? value : null;
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string" && item.trim().length > 0);
}

function numberArray(value: unknown): number[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is number => typeof item === "number" && Number.isFinite(item));
}

function numberValue(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function booleanValue(value: unknown): boolean {
  return value === true;
}

function booleanOrNull(value: unknown): boolean | null {
  return typeof value === "boolean" ? value : null;
}

function booleanAttribute(value: unknown): boolean {
  return value === true || value === "true" || value === "on";
}

function optionalEntityBooleanState(
  hass: HomeAssistant,
  entityId: string,
): boolean | null {
  if (entityById(hass, entityId) === undefined) {
    return null;
  }
  return entityBooleanState(hass, entityId);
}

type EntityDomain = "binary_sensor" | "sensor" | "button" | "number" | "switch";

function resolveEntityId(
  hass: HomeAssistant,
  snapshot: RegistrySnapshot | null | undefined,
  domain: EntityDomain,
  deviceId: string,
  key: string,
): string | null {
  const generated = generatedEntityIdForDomain(domain, deviceId, key);
  if (entityById(hass, generated) !== undefined) {
    return generated;
  }

  const uniqueId = `${deviceId}_${key}`;
  const fromUniqueId = snapshot?.entitiesByUniqueId.get(uniqueId)?.entity_id ?? null;
  if (fromUniqueId && entityById(hass, fromUniqueId) !== undefined) {
    return fromUniqueId;
  }

  return null;
}

function generatedEntityIdForDomain(
  domain: EntityDomain,
  deviceId: string,
  key: string,
): string {
  switch (domain) {
    case "binary_sensor":
      return binarySensorEntityId(deviceId, key);
    case "sensor":
      return sensorEntityId(deviceId, key);
    case "button":
      return buttonEntityId(deviceId, key);
    case "number":
      return numberEntityId(deviceId, key);
    case "switch":
      return switchEntityId(deviceId, key);
  }
}

function sensorStateForDevice(
  hass: HomeAssistant,
  snapshot: RegistrySnapshot | null | undefined,
  deviceId: string,
  key: string,
): string | null {
  const entityId = resolveEntityId(hass, snapshot, "sensor", deviceId, key);
  return entityId ? entityState(hass, entityId) ?? null : null;
}

function sensorNumberStateForDevice(
  hass: HomeAssistant,
  snapshot: RegistrySnapshot | null | undefined,
  deviceId: string,
  key: string,
): number | null {
  const entityId = resolveEntityId(hass, snapshot, "sensor", deviceId, key);
  return entityId ? entityNumberState(hass, entityId) : null;
}

function binaryStateForDevice(
  hass: HomeAssistant,
  snapshot: RegistrySnapshot | null | undefined,
  deviceId: string,
  key: string,
): boolean {
  const entityId = resolveEntityId(hass, snapshot, "binary_sensor", deviceId, key);
  return entityId ? entityBooleanState(hass, entityId) : false;
}

function optionalBinaryStateForDevice(
  hass: HomeAssistant,
  snapshot: RegistrySnapshot | null | undefined,
  deviceId: string,
  key: string,
): boolean | null {
  const entityId = resolveEntityId(hass, snapshot, "binary_sensor", deviceId, key);
  if (!entityId) {
    return null;
  }
  return optionalEntityBooleanState(hass, entityId);
}
