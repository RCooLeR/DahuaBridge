import { html, LitElement, nothing } from "lit";
import { repeat } from "lit/directives/repeat.js";
import type { TemplateResult } from "lit";

import { createSurveillancePanelStubConfig } from "./surveillance-panel-card-editor";
import { SurveillancePanelActions } from "./surveillance-panel-actions";
import { SurveillancePanelRuntime } from "./surveillance-panel-runtime";
import { surveillancePanelStyles } from "./surveillance-panel-styles";
import {
  renderSurveillancePanelEvents,
  type EventWindowOption,
} from "./surveillance-panel-events";
import { renderSurveillancePanelHeader } from "./surveillance-panel-header";
import { renderSurveillancePanelSidebar } from "./surveillance-panel-sidebar";
import { renderSurveillancePanelOverview } from "./surveillance-panel-overview";
import { renderSurveillancePanelInspector } from "./surveillance-panel-inspector";
import {
  renderArchiveRecordings,
  resolveSelectedNvrArchiveCamera,
} from "./surveillance-panel-archive";
import {
  cameraImageSrc,
  availableCameraViewportSources,
  availablePlaybackViewportSources,
  availableStreamViewportSources,
  defaultSelectedStreamProfileKey,
  renderPlaybackViewport,
  renderSelectedCameraViewport,
  renderSelectedVtoViewport,
  resolvePlaybackViewportSource,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  resolveStreamViewportSource,
  syncRemoteStreamPlayback,
  syncRemoteStreamStyles,
  teardownRemoteStreamPlayback,
  type CameraViewportSource,
} from "./surveillance-panel-media";
import {
  renderControlButton as renderControlPrimitive,
  renderIconButton as renderIconPrimitive,
  renderSegmentButton as renderSegmentPrimitive,
} from "./surveillance-panel-primitives";
import type { BridgeEvent } from "../ha/bridge-events";
import type {
  NvrArchiveRecordingModel,
  NvrArchiveSearchResultModel,
  NvrPlaybackSessionModel,
} from "../domain/archive";
import { fetchArchiveRecordings } from "../ha/bridge-archive";
import {
  createPlaybackSession,
  createPlaybackSeekRequestFromRecording,
  createPlaybackSessionFromRecording,
  seekPlaybackSession,
} from "../ha/bridge-playback";
import { postBridgeRequest } from "../ha/actions";
import {
  BridgeIntercomSessionController,
  type BridgeIntercomSnapshot,
  resolveIntercomOfferUrl,
} from "../ha/bridge-intercom";
import type { RegistrySnapshot } from "../ha/registry";
import {
  buildPanelModel,
  displayCameraLabel,
  findAuxTarget,
  resolveAuxTargetAction,
  supportsAuxTarget,
  type CameraViewModel,
  type NvrViewModel,
  type PanelModel,
  type PanelSelection,
  type SidebarFilter,
  type VtoViewModel,
} from "../domain/model";
import { parseConfig, type SurveillancePanelCardConfig } from "../types/card-config";
import type {
  HomeAssistant,
  LovelaceCard,
  LovelaceCardConfig,
} from "../types/home-assistant";
import {
  type DetailTab,
  buildTimelineEventFilterOptions,
  type EventViewMode,
  boundedEventHistoryPage,
  defaultTimelineEventFilters,
  filterTimelineEvents,
  historyPageCount,
  matchesSidebarFilters,
  overviewLayoutForCount,
  parseHistoryPageInput,
  selectCameraState,
  selectNvrState,
  selectOverviewState,
  selectVtoState,
  type TimelineEventFilters,
  visibleTimelineEvents,
  vtoBadgeClass,
  vtoBadgeTone,
} from "./surveillance-panel-state";
const BRIDGE_LOGO_URL = new URL("../assets/bridge-logo-small.png", import.meta.url).href;

const EVENT_WINDOW_OPTIONS = [
  { hours: 1, label: "1H" },
  { hours: 6, label: "6H" },
  { hours: 24, label: "24H" },
  { hours: 168, label: "7D" },
] as const;

const ACTION_STATE_OVERRIDE_TTL_MS = 10_000;

interface TimedActionStateOverride {
  active: boolean;
  expiresAt: number;
}

interface SelectedPlaybackState {
  sourceDeviceId: string;
  bridgeBaseUrl: string | null;
  recording: NvrArchiveRecordingModel;
  session: NvrPlaybackSessionModel;
}

const INITIAL_VTO_MICROPHONE_STATE: BridgeIntercomSnapshot = {
  enabled: false,
  phase: "idle",
  statusText: "Mic inactive",
  error: "",
};

export class DahuaBridgeSurveillancePanelCard
  extends LitElement
  implements LovelaceCard
{
  private _remoteStreamSyncTimer: number | null = null;
  private _archiveAbort?: AbortController;
  private _archiveRequestVersion = 0;

  static async getConfigElement(): Promise<HTMLElement> {
    return document.createElement("dahuabridge-surveillance-panel-editor");
  }

  static getStubConfig(): SurveillancePanelCardConfig {
    return createSurveillancePanelStubConfig();
  }

  static properties = {
    hass: { attribute: false },
    _config: { state: true },
    _bridgeEvents: { state: true },
    _registrySnapshot: { state: true },
    _eventsLoading: { state: true },
    _eventError: { state: true },
    _eventHistoryPage: { state: true },
    _eventViewMode: { state: true },
    _eventWindowHours: { state: true },
    _eventRoomFilter: { state: true },
    _eventDeviceKindFilter: { state: true },
    _eventSeverityFilter: { state: true },
    _eventCodeFilter: { state: true },
    _selection: { state: true },
    _sidebarFilter: { state: true },
    _searchText: { state: true },
    _detailTab: { state: true },
    _ptzAdjusting: { state: true },
    _busyActions: { state: true },
    _errorMessage: { state: true },
    _sidebarOpen: { state: true },
    _inspectorOpen: { state: true },
    _nvrArchiveChannelNumber: { state: true },
    _archiveRecordings: { state: true },
    _archiveLoading: { state: true },
    _archiveError: { state: true },
    _selectedCameraStreamProfile: { state: true },
    _selectedCameraStreamSource: { state: true },
    _selectedCameraAudioMuted: { state: true },
    _selectedVtoStreamProfile: { state: true },
    _selectedVtoStreamSource: { state: true },
    _selectedVtoStreamPlaying: { state: true },
    _selectedVtoMicrophoneState: { state: true },
    _selectedPlayback: { state: true },
    _auxStateOverrides: { state: true },
    _recordingStateOverrides: { state: true },
  } as const;

  static styles = surveillancePanelStyles;

  hass?: HomeAssistant;

  private _config?: SurveillancePanelCardConfig;
  private _bridgeEvents: BridgeEvent[] | null = null;
  private _registrySnapshot: RegistrySnapshot | null = null;
  private _eventsLoading = false;
  private _eventError = "";
  private _eventHistoryPage = 0;
  private _eventViewMode: EventViewMode = "recent";
  private _eventWindowHours = 12;
  private _eventRoomFilter = defaultTimelineEventFilters().roomLabel;
  private _eventDeviceKindFilter = defaultTimelineEventFilters().deviceKind;
  private _eventSeverityFilter = defaultTimelineEventFilters().severity;
  private _eventCodeFilter = defaultTimelineEventFilters().eventCode;
  private _selection: PanelSelection = { kind: "overview" };
  private _sidebarFilter: SidebarFilter = "all";
  private _searchText = "";
  private _detailTab: DetailTab = "overview";
  private _ptzAdjusting = false;
  private _busyActions = new Set<string>();
  private _errorMessage = "";
  private _sidebarOpen = true;
  private _inspectorOpen = true;
  private _nvrArchiveChannelNumber: number | null = null;
  private _archiveRecordings: NvrArchiveSearchResultModel | null = null;
  private _archiveLoading = false;
  private _archiveError = "";
  private _selectedCameraStreamProfile: string | null = null;
  private _selectedCameraStreamSource: CameraViewportSource | null = null;
  private _selectedCameraAudioMuted = true;
  private _selectedVtoStreamProfile: string | null = null;
  private _selectedVtoStreamSource: CameraViewportSource | null = null;
  private _selectedVtoStreamPlaying = false;
  private _selectedVtoMicrophoneState = INITIAL_VTO_MICROPHONE_STATE;
  private _selectedPlayback: SelectedPlaybackState | null = null;
  private _auxStateOverrides = new Map<string, TimedActionStateOverride>();
  private _recordingStateOverrides = new Map<string, TimedActionStateOverride>();
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
      const previousState = this._selectedVtoMicrophoneState;
      this._selectedVtoMicrophoneState = snapshot;
      if (snapshot.error) {
        this._errorMessage = snapshot.error;
      } else if (this._errorMessage === previousState.error) {
        this._errorMessage = "";
      }
      this.requestUpdate("_selectedVtoMicrophoneState", previousState);
    },
  });
  private readonly _runtime = new SurveillancePanelRuntime({
    isConnected: () => this.isConnected,
    getHass: () => this.hass,
    getConfig: () => this._config,
    getSelection: () => this._selection,
    getEventWindowHours: () => this._eventWindowHours,
    getRegistrySnapshot: () => this._registrySnapshot,
    setRegistrySnapshot: (snapshot) => {
      this._registrySnapshot = snapshot;
    },
    setBridgeEvents: (events) => {
      this._bridgeEvents = events;
    },
    setEventsLoading: (loading) => {
      this._eventsLoading = loading;
    },
    setEventError: (message) => {
      this._eventError = message;
    },
    requestUpdate: () => {
      this.requestUpdate();
    },
  });

  setConfig(config: LovelaceCardConfig): void {
    this._config = parseConfig(config);
    this._bridgeEvents = null;
    this._eventsLoading = false;
    this._eventError = "";
    this._eventHistoryPage = 0;
    this._eventViewMode = "recent";
    this._eventWindowHours = this._config.event_lookback_hours ?? 12;
    this.resetEventFilters();
    this._selection = { kind: "overview" };
    this._detailTab = "overview";
    this._ptzAdjusting = false;
    this._errorMessage = "";
    this._nvrArchiveChannelNumber = null;
    this._archiveRecordings = null;
    this._archiveLoading = false;
    this._archiveError = "";
    this._selectedCameraStreamProfile = null;
    this._selectedCameraStreamSource = null;
    this._selectedCameraAudioMuted = true;
    this._selectedVtoStreamProfile = null;
    this._selectedVtoStreamSource = null;
    this._selectedVtoStreamPlaying = false;
    this._selectedVtoMicrophoneState = INITIAL_VTO_MICROPHONE_STATE;
    this._selectedPlayback = null;
    void this.stopSelectedVtoMicrophone();
    this._auxStateOverrides = new Map();
    this._recordingStateOverrides = new Map();
  }

  connectedCallback(): void {
    super.connectedCallback();
    this._runtime.connected();
  }

  disconnectedCallback(): void {
    if (this._remoteStreamSyncTimer !== null) {
      window.clearTimeout(this._remoteStreamSyncTimer);
      this._remoteStreamSyncTimer = null;
    }
    this.cancelArchiveRefresh();
    teardownRemoteStreamPlayback();
    void this.stopSelectedVtoMicrophone();
    this._runtime.disconnected();
    super.disconnectedCallback();
  }

  protected updated(changedProperties: Map<PropertyKey, unknown>): void {
    if (changedProperties.has("hass")) {
      this._runtime.handleHassChanged();
    }
    if (
      changedProperties.has("_config") ||
      changedProperties.has("_eventWindowHours") ||
      changedProperties.has("_selection") ||
      (changedProperties.has("hass") && this._runtime.needsPollingRestartAfterHassChange())
    ) {
      this._runtime.handleEventContextChanged();
    }
    if (
      changedProperties.has("_selection") ||
      changedProperties.has("_config") ||
      changedProperties.has("_eventWindowHours") ||
      changedProperties.has("_nvrArchiveChannelNumber")
    ) {
      void this.refreshArchiveRecordings();
    }
    if (this.shouldClampEventHistoryPage(changedProperties)) {
      this.clampEventHistoryPage();
    }

    this.scheduleRemoteStreamStyleSync();
    this.maybeStartSelectedVtoVideo(changedProperties);
  }

  getCardSize(): number {
    return 20;
  }

  render(): TemplateResult {
    if (!this._config) {
      return html`<ha-card><div class="empty-state">Card configuration missing.</div></ha-card>`;
    }

    if (!this.hass) {
      return html`<ha-card><div class="empty-state">Home Assistant state unavailable.</div></ha-card>`;
    }

    const model = buildPanelModel(
      this.hass,
      this._config,
      this._selection,
      this._bridgeEvents,
      this._eventWindowHours,
      this._registrySnapshot,
    );
    this._runtime.logInitializedModels(model);
    const showInspector =
      this._inspectorOpen &&
      (model.selectedCamera !== undefined ||
        model.selectedNvr !== undefined ||
        model.selectedVto !== undefined);

    return html`
      <ha-card>
        <div
          class="shell ${this._sidebarOpen ? "" : "sidebar-collapsed"} ${showInspector
            ? ""
            : "inspector-collapsed"}"
        >
          ${this.renderSidebar(model)}
          ${this.renderHeader(model)}
          ${this.renderMain(model)}
          ${this.renderInspector(model)}
        </div>
      </ha-card>
    `;
  }

  private renderSidebar(model: PanelModel): TemplateResult {
    return renderSurveillancePanelSidebar({
      model,
      sidebarOpen: this._sidebarOpen,
      searchText: this._searchText,
      sidebarFilter: this._sidebarFilter,
      selection: this._selection,
      onSearchInput: this.handleSearchInput,
      onSelectFilter: (filter) => {
        const previousFilter = this._sidebarFilter;
        this._sidebarFilter = filter;
        this.requestUpdate("_sidebarFilter", previousFilter);
      },
      onSelectNvr: (nvr) => this.selectNvr(nvr),
      onSelectCamera: (camera) => this.selectCamera(camera),
      onSelectVto: (vto) => this.selectVto(vto),
      renderIcon: (icon) => this.renderIcon(icon),
      matchesSidebarFilters: (label, secondary, kind, highlighted) =>
        matchesSidebarFilters(
          this._searchText,
          this._sidebarFilter,
          label,
          secondary,
          kind,
          highlighted,
        ),
    });
  }

  private renderHeader(model: PanelModel): TemplateResult {
    const inspectorAvailable =
      model.selectedCamera !== null || model.selectedNvr !== null || model.selectedVto !== null;
    return renderSurveillancePanelHeader({
      model,
      bridgeLogoUrl: BRIDGE_LOGO_URL,
      sidebarOpen: this._sidebarOpen,
      inspectorOpen: this._inspectorOpen,
      inspectorAvailable,
      onSelectOverview: () => this.selectOverview(),
      onToggleSidebar: () => {
        const previousSidebarOpen = this._sidebarOpen;
        this._sidebarOpen = !this._sidebarOpen;
        this.requestUpdate("_sidebarOpen", previousSidebarOpen);
      },
      onToggleInspector: () => {
        if (!inspectorAvailable) {
          return;
        }
        const previousInspectorOpen = this._inspectorOpen;
        this._inspectorOpen = !this._inspectorOpen;
        this.requestUpdate("_inspectorOpen", previousInspectorOpen);
      },
      renderIcon: (icon) => this.renderIcon(icon),
      vtoBadgeTone,
    });
  }

  private renderMain(model: PanelModel): TemplateResult {
    if (model.selectedCamera) {
      const camera = model.selectedCamera;
      const lightAvailable = supportsAuxTarget(camera, "light");
      const warningLightAvailable = supportsAuxTarget(camera, "warning_light");
      const sirenAvailable = supportsAuxTarget(camera, "siren");
      const lightActive = this.isAuxTargetActive(camera, "light");
      const warningLightActive = this.isAuxTargetActive(camera, "warning_light");
      const sirenActive = this.isAuxTargetActive(camera, "siren");
      const bridgeRecordingActive = this.isBridgeRecordingActive(camera);
      const selectedPlayback = this.selectedPlaybackForCamera(camera);
      const selectedStreamProfile =
        selectedPlayback === null
          ? resolveSelectedCameraStreamProfile(camera, this._selectedCameraStreamProfile)
          : null;
      const selectedStreamSource =
        selectedPlayback === null
          ? resolveSelectedCameraViewportSource(
              camera,
              this._selectedCameraStreamSource,
              selectedStreamProfile?.key ?? null,
            )
          : resolvePlaybackViewportSource(
              selectedPlayback.session,
              this._selectedCameraStreamSource,
              this._selectedCameraStreamProfile,
            );
      const availableStreamSources =
        selectedPlayback === null
          ? availableCameraViewportSources(camera, selectedStreamProfile?.key ?? null)
          : availablePlaybackViewportSources(
              selectedPlayback.session,
              this._selectedCameraStreamProfile,
            );
      const cameraAudioAvailable = this.canPlaySelectedCameraAudio(
        camera,
        selectedStreamProfile?.key ?? null,
        selectedStreamSource,
      );
      const audioCodecLabel = camera.audioCodec.trim();

      return html`
        <section class="main">
          <div class="detail-shell">
            <div class="detail-header">
              <div class="detail-title">
                ${displayCameraLabel(camera)} - ${camera.roomLabel} - ${selectedPlayback ? "Playback" : "Live"}
              </div>
              <div class="split-row">
                <span class="badge ${camera.online ? "success" : "critical"}">
                  ${camera.online ? "Online" : "Offline"}
                </span>
                ${camera.recordingActive
                  ? html`<span class="badge critical">NVR Recording</span>`
                  : nothing}
                ${bridgeRecordingActive
                  ? html`<span class="badge warning">MP4 Clip</span>`
                  : nothing}
                ${repeat(
                  camera.detections,
                  (badge) => badge.key,
                  (badge) =>
                    html`<span class="badge ${badge.tone}">${badge.label}</span>`,
                )}
                ${camera.microphoneAvailable && audioCodecLabel
                  ? html`<span class="badge info">Mic ${audioCodecLabel}</span>`
                  : nothing}
                ${camera.speakerAvailable
                  ? html`<span class="badge neutral">Speaker</span>`
                  : nothing}
              </div>
            </div>
            <div class="video-panel">
              ${selectedPlayback === null &&
              (camera.stream.profiles.length > 1 || availableStreamSources.length > 1)
                ? html`
                    <div class="detail-media-toolbar">
                      ${camera.stream.profiles.length > 1
                        ? html`
                            <div class="detail-media-group chip-row">
                              ${camera.stream.profiles.map((profile) =>
                                renderSegmentPrimitive(
                                  profile.key,
                                  profile.name,
                                  selectedStreamProfile?.key ?? "",
                                  (key) => {
                                    const previousProfile = this._selectedCameraStreamProfile;
                                    this._selectedCameraStreamProfile = key;
                                    this._selectedCameraStreamSource =
                                      resolveSelectedCameraViewportSource(
                                        camera,
                                        this._selectedCameraStreamSource,
                                        key,
                                      );
                                    this.requestUpdate(
                                      "_selectedCameraStreamProfile",
                                      previousProfile,
                                    );
                                  },
                                ),
                              )}
                            </div>
                          `
                        : nothing}
                      ${camera.stream.profiles.length > 1 && availableStreamSources.length > 1
                        ? html`<div class="detail-media-separator" aria-hidden="true"></div>`
                        : nothing}
                      ${availableStreamSources.length > 1
                        ? html`
                            <div class="detail-media-group chip-row">
                              ${availableStreamSources.map((source) =>
                                renderSegmentPrimitive(
                                  source,
                                  this.streamSourceLabel(source),
                                  selectedStreamSource ?? "",
                                  (key) => {
                                    const previousSource = this._selectedCameraStreamSource;
                                    this._selectedCameraStreamSource =
                                      key as CameraViewportSource;
                                    this.requestUpdate(
                                      "_selectedCameraStreamSource",
                                      previousSource,
                                    );
                                  },
                                ),
                              )}
                            </div>
                          `
                        : nothing}
                    </div>
                  `
                : nothing}
              <div class="viewport">
                ${selectedPlayback
                  ? renderPlaybackViewport(
                      selectedPlayback.session,
                      this._selectedCameraStreamProfile,
                      selectedStreamSource,
                    )
                  : renderSelectedCameraViewport(
                      this.hass,
                      camera,
                      selectedStreamProfile?.key ?? null,
                      selectedStreamSource,
                      this._selectedCameraAudioMuted,
                    )}
                ${selectedPlayback === null && this._ptzAdjusting && camera.supportsPtz
                  ? this.renderPtzOverlay(camera)
                  : nothing}
                <div class="viewport-controls">
                  ${selectedPlayback
                    ? html`
                        ${this.renderControlButton(
                          "Return Live",
                          "mdi:cctv",
                          () => this.stopSelectedPlayback(),
                          {
                            compact: true,
                            tone: "primary",
                          },
                        )}
                        ${selectedPlayback.recording.downloadUrl
                          ? this.renderControlButton(
                              "Download",
                              "mdi:download",
                              () => this.downloadArchiveRecording(selectedPlayback.recording),
                              {
                                compact: true,
                              },
                            )
                          : nothing}
                      `
                    : html`
                        ${this.renderControlButton(
                          "Snapshot",
                          "mdi:camera",
                          () => this.openSnapshot(camera),
                          {
                            disabled: !this.hasSnapshot(camera),
                            compact: true,
                          },
                        )}
                        ${this.renderControlButton(
                          bridgeRecordingActive ? "Stop MP4" : "MP4 Recording",
                          "mdi:record-rec",
                          () =>
                            this.triggerRecordingAction(
                              camera,
                              bridgeRecordingActive ? "stop" : "start",
                            ),
                          {
                            tone: bridgeRecordingActive ? "danger" : "warning",
                            disabled:
                              !camera.supportsRecording ||
                              this.isBusy(
                                `${camera.deviceId}:recording:${bridgeRecordingActive ? "stop" : "start"}`,
                              ),
                            compact: true,
                            active: bridgeRecordingActive,
                          },
                        )}
                        ${this.renderControlButton(
                          lightActive ? "Smart Light" : "White Light",
                          "mdi:lightbulb-on-outline",
                          () => this.triggerAuxAction(camera, "light"),
                          {
                            disabled:
                              !camera.supportsAux ||
                              !lightAvailable ||
                              this.isBusy(`${camera.deviceId}:aux:light`),
                            compact: true,
                            tone: lightActive ? "primary" : undefined,
                            active: lightActive,
                          },
                        )}
                        ${this.renderControlButton(
                          warningLightActive ? "Warning Off" : "Warning On",
                          "mdi:alarm-light-outline",
                          () => this.triggerAuxAction(camera, "warning_light"),
                          {
                            disabled:
                              !camera.supportsAux ||
                              !warningLightAvailable ||
                              this.isBusy(`${camera.deviceId}:aux:warning_light`),
                            compact: true,
                            tone: warningLightActive ? "warning" : undefined,
                            active: warningLightActive,
                          },
                        )}
                        ${this.renderControlButton(
                          sirenActive ? "Siren Off" : "Siren On",
                          "mdi:bullhorn",
                          () => this.triggerAuxAction(camera, "siren"),
                          {
                            tone: "warning",
                            disabled:
                              !camera.supportsAux ||
                              !sirenAvailable ||
                              this.isBusy(`${camera.deviceId}:aux:siren`),
                            compact: true,
                            active: sirenActive,
                          },
                        )}
                        ${this.renderControlButton(
                          this._selectedCameraAudioMuted
                            ? "Enable Stream Audio"
                            : "Disable Stream Audio",
                          this._selectedCameraAudioMuted ? "mdi:volume-high" : "mdi:volume-off",
                          () => void this.toggleSelectedCameraAudio(camera),
                          {
                            tone: this._selectedCameraAudioMuted ? "neutral" : "primary",
                            disabled:
                              !cameraAudioAvailable ||
                              (camera.audioMuteSupported &&
                                this.isBusy(`${camera.deviceId}:audio:mute`)),
                            compact: true,
                            active: !this._selectedCameraAudioMuted,
                          },
                        )}
                        ${this.renderControlButton(
                          this._ptzAdjusting ? "Close PTZ" : "PTZ Controls",
                          "mdi:axis-arrow",
                          () => {
                            const previousPtzAdjusting = this._ptzAdjusting;
                            this._ptzAdjusting = !this._ptzAdjusting;
                            this.requestUpdate("_ptzAdjusting", previousPtzAdjusting);
                          },
                          {
                            tone: this._ptzAdjusting ? "primary" : "neutral",
                            disabled: !camera.supportsPtz,
                            compact: true,
                            active: this._ptzAdjusting,
                          },
                        )}
                      `}
                </div>
              </div>
              ${selectedPlayback ? this.renderPlaybackSeekPanel(selectedPlayback) : nothing}
            </div>
          </div>
        </section>
      `;
    }

    if (model.selectedVto) {
      const vto = model.selectedVto;
      const availableVtoStreamSources = availableStreamViewportSources(
        vto.stream,
        this._selectedVtoStreamProfile,
      );

      return html`
        <section class="main">
          <div class="detail-shell">
            <div class="detail-header">
              <div class="detail-title">
                ${vto.label} - ${vto.roomLabel} - ${vto.online ? "Online" : "Offline"} - ${vto.callStateText}
              </div>
              <div class="split-row">
                <span class="badge ${vtoBadgeClass(vto)}">
                  ${vto.callStateText}
                </span>
                <span class="badge ${this.selectedVtoMicrophoneBadgeTone()}">
                  ${this._selectedVtoMicrophoneState.statusText}
                </span>
                ${vto.doorbell
                  ? html`<span class="badge warning">Doorbell</span>`
                  : nothing}
                ${vto.tamper
                  ? html`<span class="badge critical">Tamper</span>`
                  : nothing}
              </div>
            </div>
            <div class="video-panel">
              ${vto.stream.profiles.length > 0 || availableVtoStreamSources.length > 0
                ? html`
                    <div class="detail-media-toolbar">
                      ${vto.stream.profiles.length > 0
                        ? html`
                            <div class="detail-media-group chip-row">
                              ${vto.stream.profiles.map((profile) =>
                                renderSegmentPrimitive(
                                  profile.key,
                                  profile.name,
                                  this._selectedVtoStreamProfile ?? "",
                                  (key) => {
                                    const previousProfile = this._selectedVtoStreamProfile;
                                    this._selectedVtoStreamProfile = key;
                                    this._selectedVtoStreamSource = resolveStreamViewportSource(
                                      vto.stream,
                                      this._selectedVtoStreamSource,
                                      key,
                                    );
                                    this.requestUpdate(
                                      "_selectedVtoStreamProfile",
                                      previousProfile,
                                    );
                                  },
                                ),
                              )}
                            </div>
                          `
                        : nothing}
                      ${vto.stream.profiles.length > 0 && availableVtoStreamSources.length > 0
                        ? html`<div class="detail-media-separator" aria-hidden="true"></div>`
                        : nothing}
                      ${availableVtoStreamSources.length > 0
                        ? html`
                            <div class="detail-media-group chip-row">
                              ${availableVtoStreamSources.map((source) =>
                                renderSegmentPrimitive(
                                  source,
                                  this.streamSourceLabel(source),
                                  this._selectedVtoStreamSource ?? "",
                                  (key) => {
                                    const previousSource = this._selectedVtoStreamSource;
                                    this._selectedVtoStreamSource =
                                      key as CameraViewportSource;
                                    this.requestUpdate(
                                      "_selectedVtoStreamSource",
                                      previousSource,
                                    );
                                  },
                                ),
                              )}
                            </div>
                          `
                        : nothing}
                    </div>
                  `
                : nothing}
              <div class="viewport">
                ${renderSelectedVtoViewport(
                  vto,
                  this._selectedVtoStreamPlaying,
                  this._selectedVtoStreamProfile,
                  this._selectedVtoStreamSource,
                )}
                <div class="viewport-controls">
                  ${this.renderControlButton(
                    "Snapshot",
                    "mdi:camera",
                    () => this.openVtoSnapshot(vto),
                    {
                      disabled: !this.hasVtoSnapshot(vto),
                      compact: true,
                    },
                  )}
                  ${this.renderControlButton(
                    vto.bridgeRecordingActive ? "Stop MP4" : "MP4 Recording",
                    "mdi:record-rec",
                    () => void this.triggerVtoBridgeRecording(vto),
                    {
                      tone: vto.bridgeRecordingActive ? "danger" : "warning",
                      disabled:
                        this.isBusy("vto:bridge_recording") ||
                        (!vto.recordingStartUrl && !vto.recordingStopUrl),
                      compact: true,
                      active: vto.bridgeRecordingActive,
                    },
                  )}
                  ${this.renderControlButton(
                    this._selectedVtoStreamPlaying ? "Stop Stream" : "Play Stream",
                    this._selectedVtoStreamPlaying
                      ? "mdi:stop-circle-outline"
                      : "mdi:play-circle-outline",
                    () => {
                      const previousPlaying = this._selectedVtoStreamPlaying;
                      this._selectedVtoStreamPlaying = !this._selectedVtoStreamPlaying;
                      this.requestUpdate("_selectedVtoStreamPlaying", previousPlaying);
                    },
                    {
                      tone: this._selectedVtoStreamPlaying ? "warning" : "primary",
                      disabled: !this.hasPlayableVtoStream(vto),
                      compact: true,
                      active: this._selectedVtoStreamPlaying,
                    },
                  )}
                  ${this.renderControlButton(
                    this._selectedVtoMicrophoneState.enabled ? "Disable Mic" : "Enable Mic",
                    this._selectedVtoMicrophoneState.enabled ? "mdi:microphone-off" : "mdi:microphone",
                    () => {
                      void (this._selectedVtoMicrophoneState.enabled
                        ? this.stopSelectedVtoMicrophone()
                        : this.startSelectedVtoMicrophone(vto));
                    },
                    {
                      tone: this._selectedVtoMicrophoneState.enabled ? "warning" : "neutral",
                      disabled:
                        !vto.capabilities.browserMicrophoneSupported ||
                        !this.hasAvailableVtoIntercom(vto),
                      compact: true,
                      active: this._selectedVtoMicrophoneState.enabled,
                    },
                  )}
                  ${vto.locks.length > 0
                    ? repeat(
                        vto.locks,
                        (lock) => lock.deviceId,
                        (lock) =>
                          this.renderControlButton(
                            vto.locks.length === 1 ? "Unlock" : `Unlock ${lock.label}`,
                            "mdi:lock-open-variant",
                            () =>
                              this.triggerVtoButtonAction(
                                `vto-lock:${lock.deviceId}:unlock`,
                                lock.unlockButtonEntityId,
                                lock.unlockActionUrl,
                              ),
                            {
                              tone: "primary",
                              disabled:
                                this.isBusy(`vto-lock:${lock.deviceId}:unlock`) ||
                                (!lock.hasUnlockButtonEntity && !lock.unlockActionUrl),
                              compact: true,
                            },
                          ),
                      )
                    : this.renderControlButton(
                        "Unlock",
                        "mdi:lock-open-variant",
                        () =>
                          this.triggerVtoButtonAction(
                            "vto:unlock",
                            vto.unlockButtonEntityId,
                            vto.unlockActionUrl,
                          ),
                        {
                          tone: "primary",
                          disabled:
                            this.isBusy("vto:unlock") ||
                            (!vto.hasUnlockButtonEntity && !vto.unlockActionUrl),
                          compact: true,
                        },
                      )}
                  ${vto.callState === "ringing"
                    ? this.renderControlButton(
                        "Answer Call",
                        "mdi:phone",
                        () =>
                          this.triggerVtoButtonAction(
                            "vto:answer",
                            vto.answerButtonEntityId,
                            vto.answerActionUrl,
                          ),
                        {
                          tone: "warning",
                          disabled:
                            this.isBusy("vto:answer") ||
                            (!vto.hasAnswerButtonEntity && !vto.answerActionUrl),
                          compact: true,
                        },
                      )
                    : nothing}
                  ${vto.callState === "ringing" || vto.callState === "active"
                    ? this.renderControlButton(
                        "Hang Up",
                        "mdi:phone-hangup",
                        () =>
                          this.triggerVtoButtonAction(
                            "vto:hangup",
                            vto.hangupButtonEntityId,
                            vto.hangupActionUrl,
                          ),
                        {
                          tone: "danger",
                          disabled:
                            this.isBusy("vto:hangup") ||
                            (!vto.hasHangupButtonEntity && !vto.hangupActionUrl),
                          compact: true,
                        },
                      )
                    : nothing}
                  ${this.renderControlButton(
                    vto.muted ? "Unmute" : "Mute",
                    vto.muted ? "mdi:volume-off" : "mdi:volume-high",
                    () =>
                      this.triggerVtoSwitchAction(
                        "vto:mute",
                        vto.mutedEntityId,
                        !vto.muted,
                        vto.mutedActionUrl,
                        "muted",
                      ),
                    {
                      tone: vto.muted ? "warning" : "neutral",
                      disabled:
                        this.isBusy("vto:mute") ||
                        (!vto.hasMutedEntity && !vto.mutedActionUrl),
                      compact: true,
                      active: vto.muted,
                    },
                  )}
                </div>
              </div>
            </div>
          </div>
        </section>
      `;
    }

    const overviewTiles = [...model.vtos, ...model.cameras];
    if (overviewTiles.length === 0) {
      return html`
        <section class="main">
          <div class="detail-main">
            <div class="panel">
              <div class="panel-title">No devices discovered</div>
              <div class="muted">
                The card did not find any DahuaBridge camera entities in Home Assistant.
              </div>
              <div class="muted">
                Check that the integration entities exist and reload the DahuaBridge config entry so the latest bridge metadata is exposed to the frontend.
              </div>
            </div>
          </div>
        </section>
      `;
    }

    const layout = overviewLayoutForCount(overviewTiles.length);

    return renderSurveillancePanelOverview({
      overviewTiles,
      layout,
      selection: this._selection,
      ptzAdjusting: this._ptzAdjusting,
      onSelectCamera: (camera) => this.selectCamera(camera),
      onSelectVto: (vto) => this.selectVto(vto),
      onOpenSnapshot: (camera) => this.openSnapshot(camera),
      onTriggerRecording: (camera, action) => this.triggerRecordingAction(camera, action),
      onTriggerAux: (camera, output) => this.triggerAuxAction(camera, output),
      onEnablePtz: (camera) => {
        this.selectCamera(camera);
        this._ptzAdjusting = true;
      },
      onVtoUnlock: (vto) =>
        this.triggerVtoButtonAction("vto:unlock", vto.unlockButtonEntityId, vto.unlockActionUrl),
      onVtoAnswer: (vto) =>
        this.triggerVtoButtonAction("vto:answer", vto.answerButtonEntityId, vto.answerActionUrl),
      onVtoHangup: (vto) =>
        this.triggerVtoButtonAction("vto:hangup", vto.hangupButtonEntityId, vto.hangupActionUrl),
      onVtoMute: (vto) =>
        this.triggerVtoSwitchAction(
          "vto:mute",
          vto.mutedEntityId,
          !vto.muted,
          vto.mutedActionUrl,
          "muted",
        ),
      onOpenVtoSnapshot: (vto) => this.openVtoSnapshot(vto),
      onToggleVtoRecording: (vto) => {
        void this.triggerVtoBridgeRecording(vto);
      },
      onToggleVtoStream: (vto) => this.toggleOverviewVtoStream(vto),
      onToggleVtoMicrophone: (vto) => {
        void (this._selectedVtoMicrophoneState.enabled
          ? this.stopSelectedVtoMicrophone()
          : this.startSelectedVtoMicrophone(vto));
      },
      renderIcon: (icon) => this.renderIcon(icon),
      cameraImageSrc: (cameraEntity, snapshotUrl) =>
        cameraImageSrc(cameraEntity, snapshotUrl),
      renderVtoViewport: (vto, playing) =>
        renderSelectedVtoViewport(vto, playing, this._selectedVtoStreamProfile, this._selectedVtoStreamSource),
      canOpenSnapshot: (camera) => this.hasSnapshot(camera),
      canOpenVtoSnapshot: (vto) => this.hasVtoSnapshot(vto),
      isBridgeRecordingActive: (camera) => this.isBridgeRecordingActive(camera),
      isVtoBridgeRecordingActive: (vto) => vto.bridgeRecordingActive,
      isAuxActive: (camera, output) => this.isAuxTargetActive(camera, output),
      isVtoStreamPlaying: (vto) =>
        this._selectedVtoStreamPlaying &&
        this._selection.kind === "vto" &&
        this._selection.deviceId === vto.deviceId,
      isVtoMicrophoneActive: () => this._selectedVtoMicrophoneState.enabled,
      hasPlayableVtoStream: (vto) => this.hasPlayableVtoStream(vto),
      hasAvailableVtoIntercom: (vto) => this.hasAvailableVtoIntercom(vto),
      isBusy: (key) => this.isBusy(key),
      vtoBadgeClass,
    });
  }

  private renderInspector(model: PanelModel): TemplateResult | typeof nothing {
    if (!model.selectedCamera && !model.selectedNvr && !model.selectedVto) {
      return nothing;
    }

    return renderSurveillancePanelInspector({
      model,
      inspectorOpen: this._inspectorOpen,
      errorMessage: this._errorMessage,
      detailTab: this._detailTab,
      eventContent:
        model.selectedCamera || model.selectedVto ? this.renderEvents(model) : nothing,
      archiveContent:
        model.selectedCamera || model.selectedNvr
          ? renderArchiveRecordings({
              model,
              archiveRecordings: this._archiveRecordings,
              archiveLoading: this._archiveLoading,
              archiveError: this._archiveError,
                eventWindowHours: this._eventWindowHours,
                playbackSupported: Boolean(this.resolveArchiveSource(model)?.archive?.playbackUrl),
                isLaunchingPlayback: (recording) => this.isBusy(this.playbackBusyKey(recording)),
                isPlaybackActive: (recording) => this.isPlaybackActive(recording),
                onLaunchPlayback: (recording) => {
                  void this.launchPlaybackSession(model, recording);
                },
                onDownloadRecording: (recording) => this.downloadArchiveRecording(recording),
                renderIcon: (icon) => this.renderIcon(icon),
              })
          : nothing,
      renderIcon: (icon) => this.renderIcon(icon),
      isBusy: (key) => this.isBusy(key),
      onSelectDetailTab: (tab) => {
        const previousDetailTab = this._detailTab;
        this._detailTab = tab;
        this.requestUpdate("_detailTab", previousDetailTab);
      },
      onVtoRangeChange: (event, key, entityId, fallbackUrl) =>
        this.handleVtoRangeChange(event, key, entityId, fallbackUrl),
      onVtoSwitchAction: (key, entityId, enabled, fallbackUrl, payloadKey) =>
        this.triggerVtoSwitchAction(key, entityId, enabled, fallbackUrl, payloadKey),
      onVtoButtonAction: (key, entityId, fallbackUrl) =>
        this.triggerVtoButtonAction(key, entityId, fallbackUrl),
    });
  }

  private renderEvents(model: PanelModel): TemplateResult {
    const pageSize = this._config?.max_events ?? 14;
    const filteredEventFeed = filterTimelineEvents(
      model.eventFeed,
      this.currentEventFilters(),
    );
    const filterOptions = buildTimelineEventFilterOptions(model.eventFeed);
    const historyPageTotal = historyPageCount(filteredEventFeed.length, pageSize);
    const clampedHistoryPage = boundedEventHistoryPage(
      this._eventHistoryPage,
      historyPageTotal,
    );
    const visibleEvents = visibleTimelineEvents(
      filteredEventFeed,
      this._eventViewMode,
      clampedHistoryPage,
      pageSize,
    );
    return renderSurveillancePanelEvents({
      eventViewMode: this._eventViewMode,
      eventsLoading: this._eventsLoading,
      eventError: this._eventError,
      visibleEvents,
      filteredEventCount: filteredEventFeed.length,
      totalEventCount: model.eventFeed.length,
      historyPageCount: historyPageTotal,
      clampedHistoryPage,
      eventWindowHours: this._eventWindowHours,
      eventWindowOptions: EVENT_WINDOW_OPTIONS,
      selectedFilters: this.currentEventFilters(),
      filterOptions,
      onSelectEventMode: (mode) => {
        const previousMode = this._eventViewMode;
        this._eventViewMode = mode;
        this.requestUpdate("_eventViewMode", previousMode);
      },
      onSelectEventWindow: (hours) => {
        const previousWindow = this._eventWindowHours;
        this._eventWindowHours = hours;
        this.requestUpdate("_eventWindowHours", previousWindow);
      },
      onSelectFilter: (key, value) => {
        switch (key) {
          case "roomLabel":
            this.updateEventFilter("_eventRoomFilter", this._eventRoomFilter, value);
            break;
          case "deviceKind":
            this.updateEventFilter("_eventDeviceKindFilter", this._eventDeviceKindFilter, value);
            break;
          case "severity":
            this.updateEventFilter("_eventSeverityFilter", this._eventSeverityFilter, value);
            break;
          case "eventCode":
            this.updateEventFilter("_eventCodeFilter", this._eventCodeFilter, value);
            break;
        }
      },
      onResetFilters: () => {
        const previousFilters = this.currentEventFilters();
        this.resetEventFilters();
        this.requestUpdate("_eventRoomFilter", previousFilters.roomLabel);
      },
      onHistoryPageInput: this.handleHistoryPageInput,
      renderIcon: (icon) => this.renderIcon(icon),
    });
  }

  private currentEventFilters(): TimelineEventFilters {
    return {
      roomLabel: this._eventRoomFilter,
      deviceKind: this._eventDeviceKindFilter,
      severity: this._eventSeverityFilter,
      eventCode: this._eventCodeFilter,
    };
  }

  private resetEventFilters(): void {
    const defaults = defaultTimelineEventFilters();
    this._eventRoomFilter = defaults.roomLabel;
    this._eventDeviceKindFilter = defaults.deviceKind;
    this._eventSeverityFilter = defaults.severity;
    this._eventCodeFilter = defaults.eventCode;
  }

  private updateEventFilter(
    key:
      | "_eventRoomFilter"
      | "_eventDeviceKindFilter"
      | "_eventSeverityFilter"
      | "_eventCodeFilter",
    previousValue: string,
    nextValue: string,
  ): void {
    if (previousValue === nextValue) {
      return;
    }
    switch (key) {
      case "_eventRoomFilter":
        this._eventRoomFilter = nextValue;
        break;
      case "_eventDeviceKindFilter":
        this._eventDeviceKindFilter = nextValue;
        break;
      case "_eventSeverityFilter":
        this._eventSeverityFilter = nextValue;
        break;
      case "_eventCodeFilter":
        this._eventCodeFilter = nextValue;
        break;
    }
    this.requestUpdate(key, previousValue);
  }

  private shouldClampEventHistoryPage(
    changedProperties: Map<PropertyKey, unknown>,
  ): boolean {
    return (
      changedProperties.has("hass") ||
      changedProperties.has("_config") ||
      changedProperties.has("_bridgeEvents") ||
      changedProperties.has("_registrySnapshot") ||
      changedProperties.has("_selection") ||
      changedProperties.has("_eventWindowHours") ||
      changedProperties.has("_eventRoomFilter") ||
      changedProperties.has("_eventDeviceKindFilter") ||
      changedProperties.has("_eventSeverityFilter") ||
      changedProperties.has("_eventCodeFilter")
    );
  }

  private clampEventHistoryPage(): void {
    if (!this.hass || !this._config) {
      return;
    }

    const model = buildPanelModel(
      this.hass,
      this._config,
      this._selection,
      this._bridgeEvents,
      this._eventWindowHours,
      this._registrySnapshot,
    );
    const filteredEventFeed = filterTimelineEvents(
      model.eventFeed,
      this.currentEventFilters(),
    );
    const pageCount = historyPageCount(
      filteredEventFeed.length,
      this._config.max_events ?? 14,
    );
    const nextPage = boundedEventHistoryPage(this._eventHistoryPage, pageCount);
    if (nextPage === this._eventHistoryPage) {
      return;
    }

    const previousPage = this._eventHistoryPage;
    this._eventHistoryPage = nextPage;
    this.requestUpdate("_eventHistoryPage", previousPage);
  }

  private renderPtzOverlay(camera: CameraViewModel): TemplateResult {
    return html`
      <div class="ptz-overlay">
        <div class="ptz-card">
          <div class="panel-title">
            <span>PTZ Controls</span>
            <span class="badge info">Adjusting</span>
          </div>
          <div class="ptz-grid">
            <span></span>
            ${this.renderPtzButton(camera, "up", "mdi:chevron-up", !camera.supportsPtzTilt)}
            <span></span>
            ${this.renderPtzButton(camera, "left", "mdi:chevron-left", !camera.supportsPtzPan)}
            ${this.renderPtzButton(camera, "stop", "mdi:stop-circle-outline", false)}
            ${this.renderPtzButton(camera, "right", "mdi:chevron-right", !camera.supportsPtzPan)}
            <span></span>
            ${this.renderPtzButton(camera, "down", "mdi:chevron-down", !camera.supportsPtzTilt)}
            <span></span>
          </div>
          <div class="control-row">
            ${this.renderControlButton(
              "Zoom +",
              "mdi:magnify-plus-outline",
              () => this.triggerPtzAction(camera, "zoom_in"),
              { disabled: !camera.supportsPtzZoom },
            )}
            ${this.renderControlButton(
              "Zoom -",
              "mdi:magnify-minus-outline",
              () => this.triggerPtzAction(camera, "zoom_out"),
              { disabled: !camera.supportsPtzZoom },
            )}
            ${this.renderControlButton(
              "Focus +",
              "mdi:crosshairs-plus",
              () => this.triggerPtzAction(camera, "focus_far"),
              { disabled: !camera.supportsPtzFocus },
            )}
            ${this.renderControlButton(
              "Focus -",
              "mdi:crosshairs",
              () => this.triggerPtzAction(camera, "focus_near"),
              { disabled: !camera.supportsPtzFocus },
            )}
          </div>
        </div>
      </div>
    `;
  }

  private renderPtzButton(
    camera: CameraViewModel,
    command: string,
    icon: string,
    disabled: boolean,
  ): TemplateResult {
    return renderIconPrimitive(
      command,
      icon,
      () => {
        void this.triggerPtzAction(camera, command);
      },
      (nextIcon) => this.renderIcon(nextIcon),
      { disabled },
    );
  }

  private renderControlButton(
    label: string,
    icon: string,
    onClick: () => void,
    options: {
      disabled?: boolean;
      tone?: "neutral" | "primary" | "warning" | "danger";
      compact?: boolean;
      active?: boolean;
    } = {},
  ): TemplateResult {
    return renderControlPrimitive(
      label,
      icon,
      onClick,
      (nextIcon) => this.renderIcon(nextIcon),
      options,
    );
  }

  private scheduleRemoteStreamStyleSync(): void {
    if (this._remoteStreamSyncTimer !== null) {
      window.clearTimeout(this._remoteStreamSyncTimer);
    }
    const syncDelays = [0, 100, 300, 800, 1500];
    const runSyncAt = (index: number): void => {
      syncRemoteStreamStyles(this.renderRoot);
      syncRemoteStreamPlayback(this.renderRoot);
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

  private openSnapshot(camera: CameraViewModel): void {
    const imageUrl = this.resolveSnapshotUrl(camera);
    if (imageUrl) {
      window.open(imageUrl, "_blank", "noopener,noreferrer");
    }
  }

  private openVtoSnapshot(vto: VtoViewModel): void {
    const imageUrl = this.resolveVtoSnapshotUrl(vto);
    if (imageUrl) {
      window.open(imageUrl, "_blank", "noopener,noreferrer");
    }
  }

  private hasSnapshot(camera: CameraViewModel): boolean {
    return this.resolveSnapshotUrl(camera).length > 0;
  }

  private hasVtoSnapshot(vto: VtoViewModel): boolean {
    return this.resolveVtoSnapshotUrl(vto).length > 0;
  }

  private resolveSnapshotUrl(camera: CameraViewModel): string {
    const captureSnapshotUrl =
      typeof camera.captureSnapshotUrl === "string" && camera.captureSnapshotUrl.trim()
        ? camera.captureSnapshotUrl
        : "";
    if (captureSnapshotUrl) {
      return captureSnapshotUrl;
    }

    const directSnapshotUrl =
      typeof camera.snapshotUrl === "string" && camera.snapshotUrl.trim()
        ? camera.snapshotUrl
        : "";
    if (directSnapshotUrl) {
      return directSnapshotUrl;
    }

    const entitySnapshot = cameraImageSrc(camera.cameraEntity, camera.snapshotUrl);
    if (entitySnapshot) {
      return entitySnapshot;
    }

    return camera.stream.onvifSnapshotUrl ?? "";
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

  private async triggerVtoBridgeRecording(vto: VtoViewModel): Promise<void> {
    const busyKey = "vto:bridge_recording";
    if (this.isBusy(busyKey)) {
      return;
    }

    const targetUrl = vto.bridgeRecordingActive ? vto.recordingStopUrl : vto.recordingStartUrl;
    if (!targetUrl) {
      this._errorMessage = "Bridge MP4 recording is unavailable for this VTO.";
      return;
    }

    const nextBusy = new Set(this._busyActions);
    nextBusy.add(busyKey);
    this._busyActions = nextBusy;
    this._errorMessage = "";

    try {
      await postBridgeRequest(targetUrl);
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : "VTO bridge recording request failed.";
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete(busyKey);
      this._busyActions = reducedBusy;
    }
  }

  private selectOverview(): void {
    const previousSelection = this._selection;
    const nextState = selectOverviewState();
    this._selection = nextState.selection;
    this._detailTab = nextState.detailTab;
    this._ptzAdjusting = nextState.ptzAdjusting;
    this._eventHistoryPage = nextState.eventHistoryPage;
    this.resetEventFilters();
    this._nvrArchiveChannelNumber = null;
    this._selectedCameraStreamProfile = null;
    this._selectedCameraStreamSource = null;
    this._selectedCameraAudioMuted = true;
    this._selectedVtoStreamProfile = null;
    this._selectedVtoStreamSource = null;
    this._selectedVtoStreamPlaying = false;
    this._selectedPlayback = null;
    void this.stopSelectedVtoMicrophone();
    this.requestUpdate("_selection", previousSelection);
  }

  private selectCamera(camera: CameraViewModel): void {
    const previousSelection = this._selection;
    const nextState = selectCameraState(camera);
    this._selection = nextState.selection;
    this._detailTab = nextState.detailTab;
    this._ptzAdjusting = nextState.ptzAdjusting;
    this._eventHistoryPage = nextState.eventHistoryPage;
    this.resetEventFilters();
    this._inspectorOpen = true;
    this._nvrArchiveChannelNumber = null;
    this._selectedCameraStreamProfile =
      defaultSelectedStreamProfileKey(camera.stream) ??
      resolveSelectedCameraStreamProfile(camera, null)?.key ??
      null;
    this._selectedCameraStreamSource = resolveSelectedCameraViewportSource(
      camera,
      null,
      this._selectedCameraStreamProfile,
    );
    this._selectedCameraAudioMuted = true;
    this._selectedVtoStreamProfile = null;
    this._selectedVtoStreamSource = null;
    this._selectedVtoStreamPlaying = false;
    this._selectedPlayback = null;
    void this.stopSelectedVtoMicrophone();
    this.requestUpdate("_selection", previousSelection);
  }

  private selectNvr(nvr: NvrViewModel): void {
    const previousSelection = this._selection;
    const nextState = selectNvrState(nvr);
    this._selection = nextState.selection;
    this._detailTab = nextState.detailTab;
    this._ptzAdjusting = nextState.ptzAdjusting;
    this._eventHistoryPage = nextState.eventHistoryPage;
    this.resetEventFilters();
    this._inspectorOpen = true;
    this._nvrArchiveChannelNumber = nvr.rooms
      .flatMap((room) => room.channels)
      .find((channel) => channel.archive?.searchUrl && channel.archive.channel !== null)
      ?.archive?.channel ?? null;
    this._selectedCameraStreamProfile = null;
    this._selectedCameraStreamSource = null;
    this._selectedCameraAudioMuted = true;
    this._selectedVtoStreamProfile = null;
    this._selectedVtoStreamSource = null;
    this._selectedVtoStreamPlaying = false;
    this._selectedPlayback = null;
    void this.stopSelectedVtoMicrophone();
    this.requestUpdate("_selection", previousSelection);
  }

  private selectVto(vto: VtoViewModel): void {
    const previousSelection = this._selection;
    const nextState = selectVtoState(vto);
    this._selection = nextState.selection;
    this._detailTab = nextState.detailTab;
    this._ptzAdjusting = nextState.ptzAdjusting;
    this._eventHistoryPage = nextState.eventHistoryPage;
    this.resetEventFilters();
    this._inspectorOpen = true;
    this._nvrArchiveChannelNumber = null;
    this._selectedCameraStreamProfile = null;
    this._selectedCameraStreamSource = null;
    this._selectedCameraAudioMuted = true;
    this._selectedVtoStreamProfile = defaultSelectedStreamProfileKey(vto.stream);
    this._selectedVtoStreamSource = resolveStreamViewportSource(
      vto.stream,
      null,
      this._selectedVtoStreamProfile,
    );
    this._selectedVtoStreamPlaying = false;
    this._selectedPlayback = null;
    void this.stopSelectedVtoMicrophone();
    this.requestUpdate("_selection", previousSelection);
  }

  private handleSearchInput = (event: Event): void => {
    const target = event.currentTarget as HTMLInputElement;
    const previousSearchText = this._searchText;
    this._searchText = target.value;
    this.requestUpdate("_searchText", previousSearchText);
  };

  private handleHistoryPageInput = (event: Event): void => {
    const nextPage = parseHistoryPageInput(event);
    if (nextPage === null) {
      return;
    }
    const previousPage = this._eventHistoryPage;
    this._eventHistoryPage = nextPage;
    this.requestUpdate("_eventHistoryPage", previousPage);
  };

  private async refreshArchiveRecordings(): Promise<void> {
    if (
      !this.hass ||
      !this._config ||
      (this._selection.kind !== "camera" && this._selection.kind !== "nvr")
    ) {
      this.cancelArchiveRefresh();
      this._archiveRecordings = null;
      this._archiveLoading = false;
      this._archiveError = "";
      return;
    }

    const model = buildPanelModel(
      this.hass,
      this._config,
      this._selection,
      this._bridgeEvents,
      this._eventWindowHours,
      this._registrySnapshot,
    );
    const archiveSource =
      this._selection.kind === "camera"
        ? model.selectedCamera ?? null
        : (() => {
            const resolvedArchiveCamera = resolveSelectedNvrArchiveCamera(
              model,
              this._nvrArchiveChannelNumber,
            );
            this._nvrArchiveChannelNumber = resolvedArchiveCamera.nextChannelNumber;
            return resolvedArchiveCamera.camera;
          })();
    if (!archiveSource?.archive?.searchUrl || archiveSource.archive.channel === null) {
      this.cancelArchiveRefresh();
      this._archiveRecordings = null;
      this._archiveLoading = false;
      this._archiveError = "";
      return;
    }

    this.cancelArchiveRefresh();
    const controller = new AbortController();
    this._archiveAbort = controller;
    const requestVersion = ++this._archiveRequestVersion;
    this._archiveLoading = true;
    this._archiveError = "";
    const end = new Date();
    const start = new Date(end.getTime() - this._eventWindowHours * 60 * 60 * 1000);

    try {
      const recordings = await fetchArchiveRecordings(archiveSource.archive.searchUrl, {
        channel: archiveSource.archive.channel,
        startTime: start.toISOString(),
        endTime: end.toISOString(),
        limit: archiveSource.archive.defaultLimit,
      }, controller.signal);
      if (controller.signal.aborted || this._archiveAbort !== controller || requestVersion !== this._archiveRequestVersion) {
        return;
      }

      this._archiveRecordings = recordings;
      this._archiveError = "";
    } catch (error) {
      if (controller.signal.aborted || this._archiveAbort !== controller || requestVersion !== this._archiveRequestVersion) {
        return;
      }
      this._archiveRecordings = null;
      this._archiveError =
        error instanceof Error ? error.message : "Archive recording request failed.";
    } finally {
      if (this._archiveAbort === controller && requestVersion === this._archiveRequestVersion) {
        this._archiveAbort = undefined;
        this._archiveLoading = false;
      }
    }
  }

  private cancelArchiveRefresh(): void {
    this._archiveAbort?.abort();
    this._archiveAbort = undefined;
  }

  private selectedPlaybackForCamera(camera: CameraViewModel): SelectedPlaybackState | null {
    if (!this._selectedPlayback) {
      return null;
    }
    return this._selectedPlayback.sourceDeviceId === camera.deviceId ? this._selectedPlayback : null;
  }

  private resolveArchiveSource(model: PanelModel): CameraViewModel | null {
    if (this._selection.kind === "camera") {
      return model.selectedCamera ?? null;
    }
    if (this._selection.kind !== "nvr") {
      return null;
    }
    return resolveSelectedNvrArchiveCamera(model, this._nvrArchiveChannelNumber).camera;
  }

  private playbackBusyKey(recording: { channel: number; startTime: string; endTime: string }): string {
    return `playback:${recording.channel}:${recording.startTime}:${recording.endTime}`;
  }

  private async launchPlaybackSession(
    model: PanelModel,
    recording: NvrArchiveRecordingModel,
  ): Promise<void> {
    const archiveSource = this.resolveArchiveSource(model);
    const playbackUrl = archiveSource?.archive?.playbackUrl ?? null;
    const browserBridgeUrl = archiveSource?.bridgeBaseUrl ?? null;
    if (!archiveSource || !playbackUrl) {
      this._errorMessage = "Playback is unavailable for the selected archive source.";
      return;
    }

    const busyKey = this.playbackBusyKey(recording);
    const nextBusy = new Set(this._busyActions);
    nextBusy.add(busyKey);
    this._busyActions = nextBusy;
    this._errorMessage = "";

    try {
      const session = await createPlaybackSession(
        playbackUrl,
        createPlaybackSessionFromRecording(recording),
        browserBridgeUrl,
      );
      const previousSelection = this._selection;
      const nextState = selectCameraState(archiveSource);
      this._selection = nextState.selection;
      this._detailTab = nextState.detailTab;
      this._ptzAdjusting = nextState.ptzAdjusting;
      this._eventHistoryPage = nextState.eventHistoryPage;
      this._inspectorOpen = true;
      this._selectedCameraAudioMuted = true;
      this._selectedVtoStreamProfile = null;
      this._selectedVtoStreamSource = null;
      this._selectedVtoStreamPlaying = false;
      void this.stopSelectedVtoMicrophone();
      this._selectedCameraStreamProfile = session.recommendedProfile;
      this._selectedCameraStreamSource = resolvePlaybackViewportSource(
        session,
        "hls",
        session.recommendedProfile,
      );
      this._selectedPlayback = {
        sourceDeviceId: archiveSource.deviceId,
        bridgeBaseUrl: browserBridgeUrl,
        recording,
        session,
      };
      this.requestUpdate("_selection", previousSelection);
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : "Playback session request failed.";
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete(busyKey);
      this._busyActions = reducedBusy;
    }
  }

  private stopSelectedPlayback(): void {
    const previousPlayback = this._selectedPlayback;
    this._selectedPlayback = null;
    this.requestUpdate("_selectedPlayback", previousPlayback);
  }

  private isPlaybackActive(recording: NvrArchiveRecordingModel): boolean {
    if (!this._selectedPlayback) {
      return false;
    }
    const active = this._selectedPlayback.recording;
    return (
      active.channel === recording.channel &&
      active.startTime === recording.startTime &&
      active.endTime === recording.endTime
    );
  }

  private downloadArchiveRecording(recording: NvrArchiveRecordingModel): void {
    if (!recording.downloadUrl) {
      return;
    }
    window.open(recording.downloadUrl, "_blank", "noopener,noreferrer");
  }

  private renderPlaybackSeekPanel(playback: SelectedPlaybackState): TemplateResult {
    const start = new Date(playback.session.startTime);
    const end = new Date(playback.session.endTime);
    const seek = new Date(playback.session.seekTime);
    const durationMs = Math.max(1, end.getTime() - start.getTime());
    const offsetMs = Math.min(durationMs, Math.max(0, seek.getTime() - start.getTime()));
    const percent = Math.round((offsetMs / durationMs) * 1000);

    return html`
      <div class="slider-wrap">
        <div class="split-row">
          <span class="badge info">Playback seek</span>
          <span class="muted">${playback.session.seekTime}</span>
        </div>
        <input
          type="range"
          min="0"
          max="1000"
          step="1"
          .value=${String(percent)}
          @change=${(event: Event) => {
            const target = event.currentTarget as HTMLInputElement;
            void this.seekSelectedPlayback(Number(target.value));
          }}
        />
        <div class="split-row muted">
          <span>${playback.session.startTime}</span>
          <span>${playback.session.endTime}</span>
        </div>
      </div>
    `;
  }

  private async seekSelectedPlayback(percentValue: number): Promise<void> {
    const playback = this._selectedPlayback;
    if (!playback) {
      return;
    }

    const start = new Date(playback.session.startTime);
    const end = new Date(playback.session.endTime);
    const durationMs = Math.max(1, end.getTime() - start.getTime());
    const bounded = Math.min(1000, Math.max(0, Math.round(percentValue)));
    const seekTime = new Date(start.getTime() + Math.round((durationMs * bounded) / 1000));

    try {
      const session = await seekPlaybackSession(
        playback.session.id,
        this.resolvePlaybackSeekUrl(playback.bridgeBaseUrl),
        createPlaybackSeekRequestFromRecording(seekTime.toISOString()),
        playback.bridgeBaseUrl,
      );
      this._selectedPlayback = {
        ...playback,
        session,
      };
      this._selectedCameraStreamProfile = session.recommendedProfile;
      this._selectedCameraStreamSource = resolvePlaybackViewportSource(
        session,
        this._selectedCameraStreamSource,
        session.recommendedProfile,
      );
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : "Playback seek request failed.";
    }
  }

  private resolvePlaybackSeekUrl(bridgeBaseUrl: string | null): string {
    if (bridgeBaseUrl?.trim()) {
      return new URL("/api/v1/nvr/playback/sessions/{session_id}/seek", bridgeBaseUrl).toString();
    }
    return "/api/v1/nvr/playback/sessions/{session_id}/seek";
  }

  private async triggerVtoButtonAction(
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ): Promise<void> {
    await this._actions.triggerVtoButtonAction(key, entityId, fallbackUrl);
  }

  private async triggerVtoSwitchAction(
    key: string,
    entityId: string,
    enabled: boolean,
    fallbackUrl: string | null,
    payloadKey: string,
  ): Promise<void> {
    await this._actions.triggerVtoSwitchAction(
      key,
      entityId,
      enabled,
      fallbackUrl,
      payloadKey,
    );
  }

  private async handleVtoRangeChange(
    event: Event,
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ): Promise<void> {
    await this._actions.handleVtoRangeChange(event, key, entityId, fallbackUrl);
  }

  private async triggerPtzAction(
    camera: CameraViewModel,
    command: string,
  ): Promise<void> {
    await this._actions.triggerPtzAction(camera, command);
  }

  private async triggerAuxAction(
    camera: CameraViewModel,
    output: string,
  ): Promise<void> {
    const target = findAuxTarget(camera, output);
    const active = this.isAuxTargetActive(camera, output);
    const nextAction = resolveAuxTargetAction(target, active);
    const succeeded = await this._actions.triggerAuxAction(camera, output, active);
    if (!succeeded || !nextAction || nextAction === "pulse") {
      return;
    }
    this.setAuxTargetOverride(camera, output, nextAction === "start");
  }

  private async triggerRecordingAction(
    camera: CameraViewModel,
    action: "start" | "stop",
  ): Promise<void> {
    const succeeded = await this._actions.triggerRecordingAction(camera, action);
    if (!succeeded) {
      return;
    }
    this.setRecordingStateOverride(camera.deviceId, action === "start");
  }

  private async toggleSelectedCameraAudio(camera: CameraViewModel): Promise<void> {
    const nextMuted = !this._selectedCameraAudioMuted;
    if (camera.audioMuteSupported) {
      const succeeded = await this._actions.triggerCameraMuteAction(camera, nextMuted);
      if (!succeeded) {
        return;
      }
    }
    const previousMuted = this._selectedCameraAudioMuted;
    this._selectedCameraAudioMuted = nextMuted;
    this.requestUpdate("_selectedCameraAudioMuted", previousMuted);
  }

  private isBusy(key: string): boolean {
    return this._actions.isBusy(key);
  }

  private renderIcon(icon: string): TemplateResult {
    return html`<ha-icon .icon=${icon}></ha-icon>`;
  }

  private streamSourceLabel(source: CameraViewportSource): string {
    switch (source) {
      case "hls":
        return "HLS";
      case "mjpeg":
        return "MJPEG";
    }
  }

  private canPlaySelectedCameraAudio(
    camera: CameraViewModel,
    selectedProfileKey: string | null,
    selectedSource: CameraViewportSource | null,
  ): boolean {
    if (selectedSource !== "hls") {
      return false;
    }
    if (!camera.audioCodec.trim()) {
      return false;
    }
    return Boolean(
      resolveSelectedCameraStreamProfile(camera, selectedProfileKey)?.localHlsUrl,
    );
  }

  private hasPlayableVtoStream(vto: VtoViewModel): boolean {
    return (
      vto.streamAvailable &&
      vto.stream.profiles.some(
        (profile) => Boolean(profile.localHlsUrl) || Boolean(profile.localMjpegUrl),
      )
    );
  }

  private hasAvailableVtoIntercom(vto: VtoViewModel): boolean {
    return resolveIntercomOfferUrl(vto.stream) !== null;
  }

  private selectedVtoMicrophoneBadgeTone(): string {
    if (this._selectedVtoMicrophoneState.phase === "error") {
      return "critical";
    }
    if (this._selectedVtoMicrophoneState.enabled) {
      return this._selectedVtoMicrophoneState.phase === "connected" ? "success" : "warning";
    }
    return "info";
  }

  private async startSelectedVtoMicrophone(vto: VtoViewModel): Promise<void> {
    const offerUrl = resolveIntercomOfferUrl(vto.stream);
    if (!offerUrl) {
      this._errorMessage = "Bridge intercom offer URL is unavailable for the selected VTO.";
      return;
    }

    await this._intercomSession.enable(offerUrl);
  }

  private async stopSelectedVtoMicrophone(): Promise<void> {
    await this._intercomSession.disable();
  }

  private toggleOverviewVtoStream(vto: VtoViewModel): void {
    const isCurrentTile =
      this._selection.kind === "vto" && this._selection.deviceId === vto.deviceId;
    const shouldPlay = !(isCurrentTile && this._selectedVtoStreamPlaying);

    this.selectVto(vto);
    this._selectedVtoStreamPlaying = shouldPlay;
    if (this._selectedVtoStreamProfile === null) {
      this._selectedVtoStreamProfile = defaultSelectedStreamProfileKey(vto.stream);
    }
    if (this._selectedVtoStreamSource === null) {
      this._selectedVtoStreamSource = resolveStreamViewportSource(
        vto.stream,
        null,
        this._selectedVtoStreamProfile,
      );
    }
  }

  private maybeStartSelectedVtoVideo(
    changedProperties: Map<PropertyKey, unknown>,
  ): void {
    if (
      !this._selectedVtoStreamPlaying ||
      this._selection.kind !== "vto" ||
      (!changedProperties.has("_selectedVtoStreamPlaying") &&
        !changedProperties.has("_selection"))
    ) {
      return;
    }

    window.requestAnimationFrame(() => {
      const video = this.renderRoot.querySelector<HTMLVideoElement>("video.vto-live-stream");
      if (!video) {
        return;
      }
      void video.play().catch(() => undefined);
    });
  }

  private auxTargetStateKey(camera: CameraViewModel, output: string): string {
    return `${camera.deviceId}:${output}`;
  }

  private resolveTimedOverride(
    current: Map<string, TimedActionStateOverride>,
    stateKey: string,
    propertyKey: "_auxStateOverrides" | "_recordingStateOverrides",
  ): boolean | null {
    const entry = current.get(stateKey);
    if (!entry) {
      return null;
    }
    if (entry.expiresAt > Date.now()) {
      return entry.active;
    }

    const next = new Map(current);
    next.delete(stateKey);
    if (propertyKey === "_auxStateOverrides") {
      this._auxStateOverrides = next;
    } else {
      this._recordingStateOverrides = next;
    }
    this.requestUpdate(propertyKey, current);
    return null;
  }

  private setTimedOverride(
    current: Map<string, TimedActionStateOverride>,
    stateKey: string,
    active: boolean,
    propertyKey: "_auxStateOverrides" | "_recordingStateOverrides",
  ): void {
    const next = new Map(current);
    next.set(stateKey, {
      active,
      expiresAt: Date.now() + ACTION_STATE_OVERRIDE_TTL_MS,
    });
    if (propertyKey === "_auxStateOverrides") {
      this._auxStateOverrides = next;
    } else {
      this._recordingStateOverrides = next;
    }
    this.requestUpdate(propertyKey, current);
  }

  private isAuxTargetActive(camera: CameraViewModel, output: string): boolean {
    const target = findAuxTarget(camera, output);
    const override = this.resolveTimedOverride(
      this._auxStateOverrides,
      this.auxTargetStateKey(camera, output),
      "_auxStateOverrides",
    );
    if (override !== null) {
      return override;
    }
    return target?.active === true;
  }

  private setAuxTargetOverride(
    camera: CameraViewModel,
    output: string,
    active: boolean,
  ): void {
    this.setTimedOverride(
      this._auxStateOverrides,
      this.auxTargetStateKey(camera, output),
      active,
      "_auxStateOverrides",
    );
  }

  private isBridgeRecordingActive(camera: CameraViewModel): boolean {
    const override = this.resolveTimedOverride(
      this._recordingStateOverrides,
      camera.deviceId,
      "_recordingStateOverrides",
    );
    if (override !== null) {
      return override;
    }
    return camera.bridgeRecordingActive;
  }

  private setRecordingStateOverride(deviceId: string, active: boolean): void {
    this.setTimedOverride(
      this._recordingStateOverrides,
      deviceId,
      active,
      "_recordingStateOverrides",
    );
  }
}

if (!customElements.get("dahuabridge-surveillance-panel")) {
  customElements.define(
    "dahuabridge-surveillance-panel",
    DahuaBridgeSurveillancePanelCard,
  );
}

window.customCards = window.customCards || [];
window.customCards.push({
  type: "dahuabridge-surveillance-panel",
  name: "DahuaBridge Surveillance Panel",
  description: "Full-panel surveillance dashboard for DahuaBridge devices.",
  preview: true,
});
