import { html, LitElement, nothing } from "lit";
import { repeat } from "lit/directives/repeat.js";
import type { TemplateResult } from "lit";

import { createSurveillancePanelStubConfig } from "./surveillance-panel-card-editor";
import { SurveillancePanelActions } from "./surveillance-panel-actions";
import { SurveillancePanelRuntime } from "./surveillance-panel-runtime";
import { surveillancePanelStyles } from "./surveillance-panel-styles";
import { renderSurveillancePanelEvents } from "./surveillance-panel-events";
import { renderSurveillancePanelHeader } from "./surveillance-panel-header";
import { renderSurveillancePanelSidebar } from "./surveillance-panel-sidebar";
import { renderSurveillancePanelOverview } from "./surveillance-panel-overview";
import { renderSurveillancePanelInspector } from "./surveillance-panel-inspector";
import {
  renderArchiveRecordings,
  renderBridgeRecordings,
  resolveSelectedNvrArchiveCamera,
} from "./surveillance-panel-archive";
import {
  cameraImageSrc,
  availableCameraViewportSources,
  availablePlaybackViewportSources,
  availableStreamViewportSources,
  defaultOverviewStreamProfileKey,
  defaultSelectedStreamProfileKey,
  preserveCameraViewportSourceSelection,
  preserveCameraViewportSourceSelectionOnProfileChange,
  preservePlaybackViewportSourceSelection,
  renderClipPlaybackViewport,
  renderPlaybackViewport,
  renderSelectedCameraViewport,
  renderSelectedVtoViewport,
  resolveInitialPlaybackViewportSource,
  resolveOverviewCameraViewportSource,
  resolvePlaybackViewportSource,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  resolveStreamViewportSource,
  syncRemoteStreamStyles,
  syncViewportAudioState,
  type CameraViewportSource,
} from "./surveillance-panel-media";
import {
  renderControlButton as renderControlPrimitive,
  renderIconButton as renderIconPrimitive,
  renderSegmentButton as renderSegmentPrimitive,
} from "./surveillance-panel-primitives";
import type { BridgeEvent } from "../ha/bridge-events";
import type {
  BridgeRecordingClipListModel,
  BridgeRecordingClipModel,
  NvrArchiveRecordingModel,
  NvrArchiveSearchResultModel,
  NvrPlaybackSessionModel,
} from "../domain/archive";
import {
  exportArchiveRecording,
  fetchArchiveRecordings,
  fetchBridgeRecordings,
  waitForArchiveExportCompletion,
} from "../ha/bridge-archive";
import {
  buildNvrEventSummaryUrl,
  fetchNvrEventSummary,
} from "../ha/bridge-event-summary";
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
import {
  summarizePanelTodayEvents,
  type PanelTodayEventSummaryModel,
} from "../domain/event-summary";
import { openExternalUrl } from "../utils/browser";
import { logCardInfo, redactUrlForLog } from "../utils/logging";
import { buildBridgeEndpointUrl } from "../ha/bridge-url";
import { parseConfig, type SurveillancePanelCardConfig } from "../types/card-config";
import type {
  HomeAssistant,
  LovelaceCard,
  LovelaceCardConfig,
} from "../types/home-assistant";
import {
  EVENT_FILTER_ALL,
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
const BRIDGE_LOGO_URL = new URL("../assets/logo-white.png", import.meta.url).href;

const EVENT_WINDOW_OPTIONS = [
  { hours: 1, label: "1H" },
  { hours: 6, label: "6H" },
  { hours: 24, label: "24H" },
  { hours: 168, label: "7D" },
] as const;

const ARCHIVE_PAGE_SIZE = 20;
const MP4_PAGE_SIZE = 20;

const ARCHIVE_EVENT_TYPE_OPTIONS = [
  { value: EVENT_FILTER_ALL, label: "All event types" },
  { value: "smdTypeHuman", label: "Human" },
  { value: "smdTypeVehicle", label: "Vehicle" },
  { value: "smdTypeAnimal", label: "Animal" },
  { value: "CrossLineDetection", label: "Cross Line" },
  { value: "CrossRegionDetection", label: "Cross Region" },
  { value: "LeftDetection", label: "Left Detection" },
  { value: "MoveDetection", label: "Motion Detection" },
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

interface SelectedBridgeRecordingPlaybackState {
  sourceDeviceId: string;
  recording: BridgeRecordingClipModel;
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
  private _mp4Abort?: AbortController;
  private _mp4RequestVersion = 0;
  private _todayEventSummaryAbort?: AbortController;
  private _todayEventSummaryRequestVersion = 0;
  private _todayEventSummaryRefreshedAt = 0;
  private _todayEventSummaryDate = "";

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
    _eventCodeFilter: { state: true },
    _archiveEventCodeFilter: { state: true },
    _archiveDate: { state: true },
    _archivePage: { state: true },
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
    _bridgeRecordings: { state: true },
    _bridgeRecordingsLoading: { state: true },
    _bridgeRecordingsError: { state: true },
    _mp4Page: { state: true },
    _todayEventSummary: { state: true },
    _selectedCameraStreamProfile: { state: true },
    _selectedCameraStreamSource: { state: true },
    _selectedPlaybackStreamProfile: { state: true },
    _selectedPlaybackStreamSource: { state: true },
    _selectedCameraAudioMuted: { state: true },
    _selectedVtoStreamProfile: { state: true },
    _selectedVtoStreamSource: { state: true },
    _selectedVtoStreamPlaying: { state: true },
    _selectedVtoMicrophoneState: { state: true },
    _selectedPlayback: { state: true },
    _selectedBridgeRecordingPlayback: { state: true },
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
  private _eventCodeFilter = defaultTimelineEventFilters().eventCode;
  private _archiveEventCodeFilter = defaultTimelineEventFilters().eventCode;
  private _archiveDate = todayDateInputValue();
  private _archivePage = 0;
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
  private _bridgeRecordings: BridgeRecordingClipListModel | null = null;
  private _bridgeRecordingsLoading = false;
  private _bridgeRecordingsError = "";
  private _mp4Page = 0;
  private _todayEventSummary: PanelTodayEventSummaryModel | null = null;
  private _selectedCameraStreamProfile: string | null = null;
  private _selectedCameraStreamSource: CameraViewportSource | null = null;
  private _selectedPlaybackStreamProfile: string | null = null;
  private _selectedPlaybackStreamSource: CameraViewportSource | null = null;
  private _selectedCameraAudioMuted = true;
  private _selectedVtoStreamProfile: string | null = null;
  private _selectedVtoStreamSource: CameraViewportSource | null = null;
  private _selectedVtoStreamPlaying = false;
  private _selectedVtoMicrophoneState = INITIAL_VTO_MICROPHONE_STATE;
  private _selectedPlayback: SelectedPlaybackState | null = null;
  private _selectedBridgeRecordingPlayback: SelectedBridgeRecordingPlaybackState | null = null;
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
    this.cancelArchiveRefresh();
    this.cancelMp4Refresh();
    this.cancelTodayEventSummaryRefresh();
    this._config = parseConfig(config);
    this._bridgeEvents = null;
    this._eventsLoading = false;
    this._eventError = "";
    this._eventHistoryPage = 0;
    this._eventViewMode = "recent";
    this._eventWindowHours = this._config.event_lookback_hours ?? 12;
    this.resetEventFilters();
    this._archiveEventCodeFilter = defaultTimelineEventFilters().eventCode;
    this._archiveDate = todayDateInputValue();
    this._archivePage = 0;
    this._selection = { kind: "overview" };
    this._detailTab = "overview";
    this._ptzAdjusting = false;
    this._errorMessage = "";
    this._nvrArchiveChannelNumber = null;
    this._archiveRecordings = null;
    this._archiveLoading = false;
    this._archiveError = "";
    this._bridgeRecordings = null;
    this._bridgeRecordingsLoading = false;
    this._bridgeRecordingsError = "";
    this._mp4Page = 0;
    this._todayEventSummary = null;
    this._todayEventSummaryRefreshedAt = 0;
    this._todayEventSummaryDate = "";
    this._selectedCameraStreamProfile = null;
    this._selectedCameraStreamSource = null;
    this._selectedPlaybackStreamProfile = null;
    this._selectedPlaybackStreamSource = null;
    this._selectedCameraAudioMuted = true;
    this._selectedVtoStreamProfile = null;
    this._selectedVtoStreamSource = null;
    this._selectedVtoStreamPlaying = false;
    this._selectedVtoMicrophoneState = INITIAL_VTO_MICROPHONE_STATE;
    this._selectedPlayback = null;
    this._selectedBridgeRecordingPlayback = null;
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
    this.cancelMp4Refresh();
    this.cancelTodayEventSummaryRefresh();
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
      changedProperties.has("_detailTab") ||
      changedProperties.has("_archiveDate") ||
      changedProperties.has("_archiveEventCodeFilter") ||
      changedProperties.has("_nvrArchiveChannelNumber")
    ) {
      void this.refreshArchiveRecordings();
    }
    if (
      changedProperties.has("_selection") ||
      changedProperties.has("_config") ||
      changedProperties.has("_detailTab") ||
      changedProperties.has("_archiveDate")
    ) {
      void this.refreshBridgeRecordings();
    }
    if (this.shouldRefreshTodayEventSummary(changedProperties)) {
      void this.refreshTodayEventSummary();
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
      this._todayEventSummary,
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
      const selectedBridgeRecordingPlayback =
        this.selectedBridgeRecordingPlaybackForCamera(camera);
      const playbackActive =
        selectedPlayback !== null || selectedBridgeRecordingPlayback !== null;
      const selectedLiveProfile =
        !playbackActive
          ? resolveSelectedCameraStreamProfile(camera, this._selectedCameraStreamProfile)
          : null;
      const selectedPlaybackProfileKey =
        selectedPlayback !== null
          ? this._selectedPlaybackStreamProfile ?? selectedPlayback.session.recommendedProfile
          : null;
      const selectedStreamProfileKey =
        !playbackActive
          ? selectedLiveProfile?.key ?? null
          : selectedPlayback !== null
            ? selectedPlaybackProfileKey
            : null;
      const selectedStreamSource =
        !playbackActive
          ? resolveSelectedCameraViewportSource(
              camera,
              this._selectedCameraStreamSource,
              selectedStreamProfileKey,
            )
          : selectedPlayback !== null
            ? resolvePlaybackViewportSource(
                selectedPlayback.session,
                this._selectedPlaybackStreamSource,
                selectedStreamProfileKey,
              )
            : null;
      const availableStreamSources =
        !playbackActive
          ? availableCameraViewportSources(camera, selectedStreamProfileKey)
          : selectedPlayback !== null
            ? availablePlaybackViewportSources(
                selectedPlayback.session,
                selectedStreamProfileKey,
              )
            : [];
      const playbackProfileKeys =
        selectedPlayback !== null ? Object.keys(selectedPlayback.session.profiles) : [];
      const cameraAudioAvailable = this.canPlaySelectedCameraAudio(
        camera,
        selectedPlayback,
        selectedBridgeRecordingPlayback,
        selectedStreamProfileKey,
        selectedStreamSource,
      );
      const audioCodecLabel = camera.audioCodec.trim();

      return html`
        <section class="main">
          <div class="detail-shell">
            <div class="detail-header">
              <div class="detail-title">
                ${displayCameraLabel(camera)} - ${camera.roomLabel} - ${playbackActive ? "Playback" : "Live"}
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
              ${((!playbackActive && camera.stream.profiles.length > 1) ||
              (selectedPlayback !== null && playbackProfileKeys.length > 1) ||
              availableStreamSources.length > 1)
                ? html`
                    <div class="detail-media-toolbar">
                      ${!playbackActive && camera.stream.profiles.length > 1
                        ? html`
                            <div class="detail-media-group chip-row">
                              ${camera.stream.profiles.map((profile) =>
                                renderSegmentPrimitive(
                                  profile.key,
                                  profile.name,
                                  selectedStreamProfileKey ?? "",
                                  (key) => {
                                    const previousProfile = this._selectedCameraStreamProfile;
                                    this._selectedCameraStreamProfile = key;
                                    this._selectedCameraStreamSource =
                                      preserveCameraViewportSourceSelectionOnProfileChange(
                                        camera,
                                        key,
                                        this._selectedCameraStreamSource,
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
                        : selectedPlayback !== null && playbackProfileKeys.length > 1
                          ? html`
                              <div class="detail-media-group chip-row">
                                ${playbackProfileKeys.map((key) =>
                                  renderSegmentPrimitive(
                                    key,
                                    key,
                                    selectedStreamProfileKey ?? "",
                                    (nextKey) => {
                                      const previousProfile = this._selectedPlaybackStreamProfile;
                                      this._selectedPlaybackStreamProfile = nextKey;
                                      this._selectedPlaybackStreamSource =
                                        preservePlaybackViewportSourceSelection(
                                          selectedPlayback.session,
                                          nextKey,
                                          this._selectedPlaybackStreamSource,
                                        );
                                      this.requestUpdate(
                                        "_selectedPlaybackStreamProfile",
                                        previousProfile,
                                      );
                                    },
                                  ),
                                )}
                              </div>
                            `
                        : nothing}
                      ${((!playbackActive && camera.stream.profiles.length > 1) ||
                      (selectedPlayback !== null && playbackProfileKeys.length > 1)) &&
                      availableStreamSources.length > 1
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
                                    if (selectedPlayback) {
                                      const previousSource = this._selectedPlaybackStreamSource;
                                      this._selectedPlaybackStreamSource =
                                        key as CameraViewportSource;
                                      this.requestUpdate(
                                        "_selectedPlaybackStreamSource",
                                        previousSource,
                                      );
                                      return;
                                    }
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
                      selectedStreamProfileKey,
                      this._selectedPlaybackStreamSource,
                      this._selectedCameraAudioMuted,
                    )
                  : selectedBridgeRecordingPlayback?.recording.playbackUrl
                    ? renderClipPlaybackViewport(
                        selectedBridgeRecordingPlayback.recording.playbackUrl,
                        displayCameraLabel(camera),
                        this._selectedCameraAudioMuted,
                      )
                  : renderSelectedCameraViewport(
                      this.hass,
                      camera,
                      selectedStreamProfileKey,
                      this._selectedCameraStreamSource,
                      this._selectedCameraAudioMuted,
                    )}
                ${!playbackActive && this._ptzAdjusting && camera.supportsPtz
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
                        ${selectedPlayback.recording.downloadUrl ||
                        selectedPlayback.recording.exportUrl
                          ? this.renderControlButton(
                              selectedPlayback.recording.downloadUrl ? "Download" : "Export MP4",
                              "mdi:download",
                              () => void this.downloadArchiveRecording(selectedPlayback.recording),
                              {
                                compact: true,
                                disabled: this.isBusy(
                                  this.archiveDownloadBusyKey(selectedPlayback.recording),
                                ),
                              },
                            )
                          : nothing}
                        ${cameraAudioAvailable
                          ? this.renderControlButton(
                              this._selectedCameraAudioMuted
                                ? "Enable Stream Audio"
                                : "Disable Stream Audio",
                              this._selectedCameraAudioMuted ? "mdi:volume-high" : "mdi:volume-off",
                              () => void this.toggleSelectedCameraAudio(camera),
                              {
                                tone: this._selectedCameraAudioMuted ? "neutral" : "primary",
                                compact: true,
                                active: !this._selectedCameraAudioMuted,
                              },
                            )
                          : nothing}
                      `
                    : selectedBridgeRecordingPlayback
                      ? html`
                          ${this.renderControlButton(
                            "Return Live",
                            "mdi:cctv",
                            () => this.stopSelectedBridgeRecordingPlayback(),
                            {
                              compact: true,
                              tone: "primary",
                            },
                          )}
                          ${selectedBridgeRecordingPlayback.recording.downloadUrl
                            ? this.renderControlButton(
                                "Download",
                                "mdi:download",
                                () =>
                                  this.downloadBridgeRecording(
                                    selectedBridgeRecordingPlayback.recording,
                                  ),
                                {
                                  compact: true,
                                  disabled: this.isBusy(
                                    this.bridgeRecordingDownloadBusyKey(
                                      selectedBridgeRecordingPlayback.recording,
                                    ),
                                  ),
                                },
                              )
                            : nothing}
                          ${cameraAudioAvailable
                            ? this.renderControlButton(
                                this._selectedCameraAudioMuted
                                  ? "Enable Stream Audio"
                                  : "Disable Stream Audio",
                                this._selectedCameraAudioMuted
                                  ? "mdi:volume-high"
                                  : "mdi:volume-off",
                                () => void this.toggleSelectedCameraAudio(camera),
                                {
                                  tone: this._selectedCameraAudioMuted ? "neutral" : "primary",
                                  compact: true,
                                  active: !this._selectedCameraAudioMuted,
                                },
                              )
                            : nothing}
                        `
                    : html`
                        ${this.hasSnapshot(camera)
                          ? this.renderControlButton(
                              "Snapshot",
                              "mdi:camera",
                              () => this.openSnapshot(camera),
                              {
                                compact: true,
                              },
                            )
                          : nothing}
                        ${camera.supportsRecording
                          ? this.renderControlButton(
                              bridgeRecordingActive ? "Stop MP4" : "MP4 Recording",
                              "mdi:record-rec",
                              () =>
                                this.triggerRecordingAction(
                                  camera,
                                  bridgeRecordingActive ? "stop" : "start",
                                ),
                              {
                                tone: bridgeRecordingActive ? "danger" : "warning",
                                disabled: this.isBusy(
                                  `${camera.deviceId}:recording:${bridgeRecordingActive ? "stop" : "start"}`,
                                ),
                                compact: true,
                                active: bridgeRecordingActive,
                              },
                            )
                          : nothing}
                        ${camera.supportsAux && lightAvailable
                          ? this.renderControlButton(
                              lightActive ? "Smart Light" : "White Light",
                              "mdi:lightbulb-on-outline",
                              () => this.triggerAuxAction(camera, "light"),
                              {
                                disabled: this.isBusy(`${camera.deviceId}:aux:light`),
                                compact: true,
                                tone: lightActive ? "primary" : undefined,
                                active: lightActive,
                              },
                            )
                          : nothing}
                        ${camera.supportsAux && warningLightAvailable
                          ? this.renderControlButton(
                              warningLightActive ? "Warning Off" : "Warning On",
                              "mdi:alarm-light-outline",
                              () => this.triggerAuxAction(camera, "warning_light"),
                              {
                                disabled: this.isBusy(`${camera.deviceId}:aux:warning_light`),
                                compact: true,
                                tone: warningLightActive ? "warning" : undefined,
                                active: warningLightActive,
                              },
                            )
                          : nothing}
                        ${camera.supportsAux && sirenAvailable
                          ? this.renderControlButton(
                              sirenActive ? "Siren Off" : "Siren On",
                              "mdi:bullhorn",
                              () => this.triggerAuxAction(camera, "siren"),
                              {
                                tone: "warning",
                                disabled: this.isBusy(`${camera.deviceId}:aux:siren`),
                                compact: true,
                                active: sirenActive,
                              },
                            )
                          : nothing}
                        ${cameraAudioAvailable
                          ? this.renderControlButton(
                              this._selectedCameraAudioMuted
                                ? "Enable Stream Audio"
                                : "Disable Stream Audio",
                              this._selectedCameraAudioMuted ? "mdi:volume-high" : "mdi:volume-off",
                              () => void this.toggleSelectedCameraAudio(camera),
                              {
                                tone: this._selectedCameraAudioMuted ? "neutral" : "primary",
                                compact: true,
                                active: !this._selectedCameraAudioMuted,
                              },
                            )
                          : nothing}
                        ${camera.supportsPtz
                          ? this.renderControlButton(
                              this._ptzAdjusting ? "Close PTZ" : "PTZ Controls",
                              "mdi:axis-arrow",
                              () => {
                                const previousPtzAdjusting = this._ptzAdjusting;
                                this._ptzAdjusting = !this._ptzAdjusting;
                                this.requestUpdate("_ptzAdjusting", previousPtzAdjusting);
                              },
                              {
                                tone: this._ptzAdjusting ? "primary" : "neutral",
                                compact: true,
                                active: this._ptzAdjusting,
                              },
                            )
                          : nothing}
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
        Boolean(vto.cameraEntity),
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
                                      Boolean(vto.cameraEntity),
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
                  ${this.hasVtoSnapshot(vto)
                    ? this.renderControlButton(
                        "Snapshot",
                        "mdi:camera",
                        () => this.openVtoSnapshot(vto),
                        {
                          compact: true,
                        },
                      )
                    : nothing}
                  ${vto.recordingStartUrl || vto.recordingStopUrl
                    ? this.renderControlButton(
                        vto.bridgeRecordingActive ? "Stop MP4" : "MP4 Recording",
                        "mdi:record-rec",
                        () => void this.triggerVtoBridgeRecording(vto),
                        {
                          tone: vto.bridgeRecordingActive ? "danger" : "warning",
                          disabled: this.isBusy("vto:bridge_recording"),
                          compact: true,
                          active: vto.bridgeRecordingActive,
                        },
                      )
                    : nothing}
                  ${this.hasPlayableVtoStream(vto)
                    ? this.renderControlButton(
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
                          compact: true,
                          active: this._selectedVtoStreamPlaying,
                        },
                      )
                    : nothing}
                  ${vto.capabilities.browserMicrophoneSupported && this.hasAvailableVtoIntercom(vto)
                    ? this.renderControlButton(
                        this._selectedVtoMicrophoneState.enabled ? "Disable Mic" : "Enable Mic",
                        this._selectedVtoMicrophoneState.enabled ? "mdi:microphone-off" : "mdi:microphone",
                        () => {
                          void (this._selectedVtoMicrophoneState.enabled
                            ? this.stopSelectedVtoMicrophone()
                            : this.startSelectedVtoMicrophone(vto));
                        },
                        {
                          tone: this._selectedVtoMicrophoneState.enabled ? "warning" : "neutral",
                          compact: true,
                          active: this._selectedVtoMicrophoneState.enabled,
                        },
                      )
                    : nothing}
                  ${vto.locks.filter((lock) => lock.hasUnlockButtonEntity || Boolean(lock.unlockActionUrl)).length > 0
                    ? repeat(
                        vto.locks.filter(
                          (lock) => lock.hasUnlockButtonEntity || Boolean(lock.unlockActionUrl),
                        ),
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
                              disabled: this.isBusy(`vto-lock:${lock.deviceId}:unlock`),
                              compact: true,
                            },
                          ),
                      )
                    : vto.hasUnlockButtonEntity || vto.unlockActionUrl
                      ? this.renderControlButton(
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
                          disabled: this.isBusy("vto:unlock"),
                          compact: true,
                        },
                      )
                      : nothing}
                  ${vto.callState === "ringing" && (vto.hasAnswerButtonEntity || vto.answerActionUrl)
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
                          disabled: this.isBusy("vto:answer"),
                          compact: true,
                        },
                      )
                    : nothing}
                  ${(vto.callState === "ringing" || vto.callState === "active") &&
                  (vto.hasHangupButtonEntity || vto.hangupActionUrl)
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
                          disabled: this.isBusy("vto:hangup"),
                          compact: true,
                        },
                      )
                    : nothing}
                  ${vto.hasMutedEntity || vto.mutedActionUrl
                    ? this.renderControlButton(
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
                          disabled: this.isBusy("vto:mute"),
                          compact: true,
                          active: vto.muted,
                        },
                      )
                    : nothing}
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
      renderCameraViewport: (camera) => {
        const profileKey = defaultOverviewStreamProfileKey(camera.stream);
        const source = resolveOverviewCameraViewportSource(camera, profileKey);
        return renderSelectedCameraViewport(this.hass, camera, profileKey, source, true, {
          controls: false,
          preload: "none",
          fallbackOrder: ["hls", "mjpeg"],
        });
      },
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
      eventContent: model.selectedCamera
        ? this.renderCameraArchiveEvents(model)
        : model.selectedVto
          ? this.renderEvents(model)
          : nothing,
      archiveContent:
        model.selectedCamera || model.selectedNvr ? this.renderArchiveTab(model) : nothing,
      mp4Content: model.selectedCamera
        ? this.renderBridgeMp4Tab(model.selectedCamera)
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

  private renderCameraArchiveEvents(model: PanelModel): TemplateResult | typeof nothing {
    if (!model.selectedCamera) {
      return nothing;
    }

    const archiveItems = this._archiveRecordings?.items ?? [];
    const pageCount = pageCountForItems(archiveItems.length, ARCHIVE_PAGE_SIZE);
    const page = boundedPageIndex(this._archivePage, pageCount);

    return renderArchiveRecordings({
      title: "Events",
      archiveRecordings: this._archiveRecordings,
      archiveLoading: this._archiveLoading,
      archiveError: this._archiveError,
      archiveDate: this._archiveDate,
      archiveEventCode: this._archiveEventCodeFilter,
      archiveEventTypeOptions: ARCHIVE_EVENT_TYPE_OPTIONS,
      playbackSupported: Boolean(model.selectedCamera.archive?.playbackUrl),
      showEventFilter: true,
      page,
      pageCount,
      visibleItems: slicePage(archiveItems, page, ARCHIVE_PAGE_SIZE),
      isLaunchingPlayback: (recording) => this.isBusy(this.playbackBusyKey(recording)),
      isPlaybackActive: (recording) => this.isPlaybackActive(recording),
      isDownloadingRecording: (recording) =>
        this.isBusy(this.archiveDownloadBusyKey(recording)),
      onSelectArchiveDate: (value) => this.selectArchiveDate(value),
      onSelectArchiveEventType: (eventCode) => this.selectArchiveEventType(eventCode),
      onSelectArchivePage: (nextPage) => this.selectArchivePage(nextPage),
      onLaunchPlayback: (recording) => {
        void this.launchPlaybackSession(model, recording);
      },
      onDownloadRecording: (recording) => {
        void this.downloadArchiveRecording(recording);
      },
      renderIcon: (icon) => this.renderIcon(icon),
    });
  }

  private renderArchiveTab(model: PanelModel): TemplateResult | typeof nothing {
    const archiveSource = this.resolveArchiveSource(model);
    if (!archiveSource && !model.selectedNvr) {
      return nothing;
    }

    const archiveItems = this._archiveRecordings?.items ?? [];
    const pageCount = pageCountForItems(archiveItems.length, ARCHIVE_PAGE_SIZE);
    const page = boundedPageIndex(this._archivePage, pageCount);

    return renderArchiveRecordings({
      title: model.selectedNvr ? "Recorder Recordings" : "24/7 Recordings",
      archiveRecordings: this._archiveRecordings,
      archiveLoading: this._archiveLoading,
      archiveError: this._archiveError,
      archiveDate: this._archiveDate,
      archiveEventCode: EVENT_FILTER_ALL,
      archiveEventTypeOptions: ARCHIVE_EVENT_TYPE_OPTIONS,
      playbackSupported: Boolean(archiveSource?.archive?.playbackUrl),
      showEventFilter: false,
      page,
      pageCount,
      visibleItems: slicePage(archiveItems, page, ARCHIVE_PAGE_SIZE),
      isLaunchingPlayback: (recording) => this.isBusy(this.playbackBusyKey(recording)),
      isPlaybackActive: (recording) => this.isPlaybackActive(recording),
      isDownloadingRecording: (recording) =>
        this.isBusy(this.archiveDownloadBusyKey(recording)),
      onSelectArchiveDate: (value) => this.selectArchiveDate(value),
      onSelectArchiveEventType: () => undefined,
      onSelectArchivePage: (nextPage) => this.selectArchivePage(nextPage),
      onLaunchPlayback: (recording) => {
        void this.launchPlaybackSession(model, recording);
      },
      onDownloadRecording: (recording) => {
        void this.downloadArchiveRecording(recording);
      },
      renderIcon: (icon) => this.renderIcon(icon),
    });
  }

  private renderBridgeMp4Tab(camera: CameraViewModel): TemplateResult {
    const recordings = this._bridgeRecordings?.items ?? [];
    const pageCount = pageCountForItems(recordings.length, MP4_PAGE_SIZE);
    const page = boundedPageIndex(this._mp4Page, pageCount);

    return renderBridgeRecordings({
      title: `${displayCameraLabel(camera)} MP4 Clips`,
      recordings: this._bridgeRecordings,
      recordingsLoading: this._bridgeRecordingsLoading,
      recordingsError: this._bridgeRecordingsError,
      recordingsDate: this._archiveDate,
      page,
      pageCount,
      visibleItems: slicePage(recordings, page, MP4_PAGE_SIZE),
      playbackSupported: true,
      isPlaybackActive: (recording) => this.isBridgeRecordingPlaybackActive(recording),
      isDownloadingRecording: (recording) =>
        this.isBusy(this.bridgeRecordingDownloadBusyKey(recording)),
      onSelectDate: (value) => this.selectArchiveDate(value),
      onSelectPage: (nextPage) => this.selectMp4Page(nextPage),
      onPlayRecording: (recording) => {
        this.playBridgeRecording(camera, recording);
      },
      onDownloadRecording: (recording) => {
        this.downloadBridgeRecording(recording);
      },
      renderIcon: (icon) => this.renderIcon(icon),
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
        if (key === "eventCode") {
          this.updateEventFilter("_eventCodeFilter", this._eventCodeFilter, value);
        }
      },
      onResetFilters: () => {
        const previousFilters = this.currentEventFilters();
        this.resetEventFilters();
        this.requestUpdate("_eventCodeFilter", previousFilters.eventCode);
      },
      onHistoryPageInput: this.handleHistoryPageInput,
      renderIcon: (icon) => this.renderIcon(icon),
    });
  }

  private currentEventFilters(): TimelineEventFilters {
    return {
      eventCode: this._eventCodeFilter,
    };
  }

  private resetEventFilters(): void {
    const defaults = defaultTimelineEventFilters();
    this._eventCodeFilter = defaults.eventCode;
  }

  private resetArchiveEventFilter(): void {
    this._archiveEventCodeFilter = defaultTimelineEventFilters().eventCode;
  }

  private updateEventFilter(
    key: "_eventCodeFilter",
    previousValue: string,
    nextValue: string,
  ): void {
    if (previousValue === nextValue) {
      return;
    }
    this._eventCodeFilter = nextValue;
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
      openExternalUrl(imageUrl);
    }
  }

  private openVtoSnapshot(vto: VtoViewModel): void {
    const imageUrl = this.resolveVtoSnapshotUrl(vto);
    if (imageUrl) {
      openExternalUrl(imageUrl);
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
      this.logMedia("card panel vto bridge recording request", {
        device_id: vto.deviceId,
        active: vto.bridgeRecordingActive,
        url: redactUrlForLog(targetUrl),
      });
      await postBridgeRequest(targetUrl);
      this.logMedia("card panel vto bridge recording request completed", {
        device_id: vto.deviceId,
        active: vto.bridgeRecordingActive,
      });
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : "VTO bridge recording request failed.";
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete(busyKey);
      this._busyActions = reducedBusy;
    }
  }

  private resetSharedSelectionViewState(): void {
    this.resetEventFilters();
    this.resetArchiveEventFilter();
    this._archivePage = 0;
    this._mp4Page = 0;
    this._selectedPlaybackStreamProfile = null;
    this._selectedPlaybackStreamSource = null;
    this._selectedCameraAudioMuted = true;
    this._selectedPlayback = null;
    this._selectedBridgeRecordingPlayback = null;
  }

  private clearCameraSelectionState(): void {
    this._selectedCameraStreamProfile = null;
    this._selectedCameraStreamSource = null;
  }

  private clearVtoSelectionState(): void {
    this._selectedVtoStreamProfile = null;
    this._selectedVtoStreamSource = null;
    this._selectedVtoStreamPlaying = false;
  }

  private applySelectionTransition(
    nextState: {
      selection: PanelSelection;
      detailTab: DetailTab;
      ptzAdjusting: boolean;
      eventHistoryPage: number;
    },
    options: {
      openInspector?: boolean;
      nvrArchiveChannelNumber?: number | null;
      cameraProfile?: string | null;
      cameraSource?: CameraViewportSource | null;
      vtoProfile?: string | null;
      vtoSource?: CameraViewportSource | null;
      vtoPlaying?: boolean;
    } = {},
  ): void {
    this._selection = nextState.selection;
    this._detailTab = nextState.detailTab;
    this._ptzAdjusting = nextState.ptzAdjusting;
    this._eventHistoryPage = nextState.eventHistoryPage;
    this._inspectorOpen = options.openInspector ?? this._inspectorOpen;
    this._nvrArchiveChannelNumber = options.nvrArchiveChannelNumber ?? null;
    this._selectedCameraStreamProfile = options.cameraProfile ?? null;
    this._selectedCameraStreamSource = options.cameraSource ?? null;
    this._selectedVtoStreamProfile = options.vtoProfile ?? null;
    this._selectedVtoStreamSource = options.vtoSource ?? null;
    this._selectedVtoStreamPlaying = options.vtoPlaying ?? false;
    void this.stopSelectedVtoMicrophone();
  }

  private selectOverview(): void {
    const previousSelection = this._selection;
    const nextState = selectOverviewState();
    this.resetSharedSelectionViewState();
    this.clearCameraSelectionState();
    this.clearVtoSelectionState();
    this.applySelectionTransition(nextState);
    this.requestUpdate("_selection", previousSelection);
  }

  private selectCamera(camera: CameraViewModel): void {
    const previousSelection = this._selection;
    const nextState = selectCameraState(camera);
    const cameraProfile =
      defaultSelectedStreamProfileKey(camera.stream) ??
      resolveSelectedCameraStreamProfile(camera, null)?.key ??
      null;
    this.resetSharedSelectionViewState();
    this.clearVtoSelectionState();
    this.applySelectionTransition(nextState, {
      openInspector: true,
      cameraProfile,
    });
    this.requestUpdate("_selection", previousSelection);
  }

  private selectNvr(nvr: NvrViewModel): void {
    const previousSelection = this._selection;
    const nextState = selectNvrState(nvr);
    const nvrArchiveChannelNumber = nvr.rooms
      .flatMap((room) => room.channels)
      .find((channel) => channel.archive?.searchUrl && channel.archive.channel !== null)
      ?.archive?.channel ?? null;
    this.resetSharedSelectionViewState();
    this.clearCameraSelectionState();
    this.clearVtoSelectionState();
    this.applySelectionTransition(nextState, {
      openInspector: true,
      nvrArchiveChannelNumber,
    });
    this.requestUpdate("_selection", previousSelection);
  }

  private selectVto(vto: VtoViewModel): void {
    const previousSelection = this._selection;
    const nextState = selectVtoState(vto);
    const vtoProfile = defaultSelectedStreamProfileKey(vto.stream);
    this.resetSharedSelectionViewState();
    this.clearCameraSelectionState();
    this.applySelectionTransition(nextState, {
      openInspector: true,
      vtoProfile,
    });
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

  private selectArchiveEventType(eventCode: string): void {
    if (eventCode === this._archiveEventCodeFilter) {
      return;
    }
    const previousEventCode = this._archiveEventCodeFilter;
    this._archiveEventCodeFilter = eventCode;
    this._archivePage = 0;
    this.requestUpdate("_archiveEventCodeFilter", previousEventCode);
  }

  private selectArchiveDate(value: string): void {
    const nextDate = normalizeArchiveDateInput(value);
    if (nextDate === this._archiveDate) {
      return;
    }
    const previousDate = this._archiveDate;
    this._archiveDate = nextDate;
    this._archivePage = 0;
    this._mp4Page = 0;
    this.requestUpdate("_archiveDate", previousDate);
  }

  private selectArchivePage(page: number): void {
    const pageCount = pageCountForItems(this._archiveRecordings?.items.length ?? 0, ARCHIVE_PAGE_SIZE);
    const nextPage = boundedPageIndex(page, pageCount);
    if (nextPage === this._archivePage) {
      return;
    }
    const previousPage = this._archivePage;
    this._archivePage = nextPage;
    this.requestUpdate("_archivePage", previousPage);
  }

  private selectMp4Page(page: number): void {
    const pageCount = pageCountForItems(this._bridgeRecordings?.items.length ?? 0, MP4_PAGE_SIZE);
    const nextPage = boundedPageIndex(page, pageCount);
    if (nextPage === this._mp4Page) {
      return;
    }
    const previousPage = this._mp4Page;
    this._mp4Page = nextPage;
    this.requestUpdate("_mp4Page", previousPage);
  }

  private async refreshArchiveRecordings(): Promise<void> {
    if (!this.hass || !this._config) {
      this.cancelArchiveRefresh();
      this._archiveRecordings = null;
      this._archiveLoading = false;
      this._archiveError = "";
      return;
    }

    if (this._selection.kind !== "camera" && this._selection.kind !== "nvr") {
      this.cancelArchiveRefresh();
      this._archiveRecordings = null;
      this._archiveLoading = false;
      this._archiveError = "";
      return;
    }

    if (
      this._selection.kind === "camera" &&
      this._detailTab !== "events" &&
      this._detailTab !== "recordings"
    ) {
      this.cancelArchiveRefresh();
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
    const { startTime, endTime } = dateRangeForArchiveDay(this._archiveDate);
    const eventCode =
      this._selection.kind === "camera" && this._detailTab === "events"
        ? this._archiveEventCodeFilter
        : EVENT_FILTER_ALL;
    const eventOnly = this._selection.kind === "camera" && this._detailTab === "events";

    try {
      this.logMedia("card panel archive recordings refresh started", {
        device_id: archiveSource.deviceId,
        channel: archiveSource.archive.channel,
        start_time: startTime,
        end_time: endTime,
        event_code: eventCode,
        event_only: eventOnly,
      });
      const recordings = await fetchArchiveRecordings(
        archiveSource.archive.searchUrl,
        {
          channel: archiveSource.archive.channel,
          startTime,
          endTime,
          limit: archiveSource.archive.defaultLimit,
          eventCode,
          eventOnly,
        },
        controller.signal,
      );
      if (
        controller.signal.aborted ||
        this._archiveAbort !== controller ||
        requestVersion !== this._archiveRequestVersion
      ) {
        return;
      }

      this._archiveRecordings = recordings;
      this._archiveError = "";
      this.logMedia("card panel archive recordings refresh completed", {
        device_id: archiveSource.deviceId,
        channel: archiveSource.archive.channel,
        count: recordings.items.length,
      });
    } catch (error) {
      if (
        controller.signal.aborted ||
        this._archiveAbort !== controller ||
        requestVersion !== this._archiveRequestVersion
      ) {
        return;
      }
      this._archiveRecordings = null;
      this._archiveError =
        error instanceof Error ? error.message : "Archive recording request failed.";
      this.logMedia("card panel archive recordings refresh failed", {
        device_id: archiveSource.deviceId,
        channel: archiveSource.archive.channel,
        error: this._archiveError,
      });
    } finally {
      if (
        this._archiveAbort === controller &&
        requestVersion === this._archiveRequestVersion
      ) {
        this._archiveAbort = undefined;
        this._archiveLoading = false;
      }
    }
  }

  private cancelArchiveRefresh(): void {
    this._archiveAbort?.abort();
    this._archiveAbort = undefined;
  }

  private async refreshBridgeRecordings(): Promise<void> {
    if (!this.hass || !this._config) {
      this.cancelMp4Refresh();
      this._bridgeRecordings = null;
      this._bridgeRecordingsLoading = false;
      this._bridgeRecordingsError = "";
      return;
    }

    if (this._selection.kind !== "camera") {
      this.cancelMp4Refresh();
      this._bridgeRecordings = null;
      this._bridgeRecordingsLoading = false;
      this._bridgeRecordingsError = "";
      return;
    }

    if (this._detailTab !== "mp4") {
      this.cancelMp4Refresh();
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
    const camera = model.selectedCamera ?? null;
    if (!camera?.recordingsUrl) {
      this.cancelMp4Refresh();
      this._bridgeRecordings = null;
      this._bridgeRecordingsLoading = false;
      this._bridgeRecordingsError = "";
      return;
    }

    this.cancelMp4Refresh();
    const controller = new AbortController();
    this._mp4Abort = controller;
    const requestVersion = ++this._mp4RequestVersion;
    this._bridgeRecordingsLoading = true;
    this._bridgeRecordingsError = "";
    const { startTime, endTime } = dateRangeForArchiveDay(this._archiveDate);

    try {
      this.logMedia("card panel bridge recordings refresh started", {
        device_id: camera.deviceId,
        start_time: startTime,
        end_time: endTime,
      });
      const recordings = await fetchBridgeRecordings(
        camera.recordingsUrl,
        {
          startTime,
          endTime,
          limit: Math.max(camera.archive?.defaultLimit ?? 0, 100),
        },
        controller.signal,
      );
      if (
        controller.signal.aborted ||
        this._mp4Abort !== controller ||
        requestVersion !== this._mp4RequestVersion
      ) {
        return;
      }
      this._bridgeRecordings = recordings;
      this._bridgeRecordingsError = "";
      this.logMedia("card panel bridge recordings refresh completed", {
        device_id: camera.deviceId,
        count: recordings.items.length,
      });
    } catch (error) {
      if (
        controller.signal.aborted ||
        this._mp4Abort !== controller ||
        requestVersion !== this._mp4RequestVersion
      ) {
        return;
      }
      this._bridgeRecordings = null;
      this._bridgeRecordingsError =
        error instanceof Error ? error.message : "Bridge MP4 request failed.";
      this.logMedia("card panel bridge recordings refresh failed", {
        device_id: camera.deviceId,
        error: this._bridgeRecordingsError,
      });
    } finally {
      if (this._mp4Abort === controller && requestVersion === this._mp4RequestVersion) {
        this._mp4Abort = undefined;
        this._bridgeRecordingsLoading = false;
      }
    }
  }

  private cancelMp4Refresh(): void {
    this._mp4Abort?.abort();
    this._mp4Abort = undefined;
  }

  private shouldRefreshTodayEventSummary(
    changedProperties: Map<PropertyKey, unknown>,
  ): boolean {
    if (!this.hass || !this._config) {
      return false;
    }

    const today = todayDateInputValue();
    if (this._todayEventSummaryDate !== today) {
      return true;
    }
    if ((changedProperties.has("_config") || changedProperties.has("hass")) && !this._todayEventSummary) {
      return true;
    }
    if (!changedProperties.has("_bridgeEvents")) {
      return false;
    }
    return Date.now() - this._todayEventSummaryRefreshedAt >= 60_000;
  }

  private async refreshTodayEventSummary(): Promise<void> {
    if (!this.hass || !this._config) {
      this.cancelTodayEventSummaryRefresh();
      this._todayEventSummary = null;
      return;
    }

    const today = todayDateInputValue();
    const baseModel = buildPanelModel(
      this.hass,
      this._config,
      this._selection,
      this._bridgeEvents,
      this._eventWindowHours,
      this._registrySnapshot,
      this._todayEventSummary,
    );
    if (baseModel.nvrs.length === 0) {
      this.cancelTodayEventSummaryRefresh();
      this._todayEventSummary = null;
      this._todayEventSummaryDate = today;
      this._todayEventSummaryRefreshedAt = Date.now();
      return;
    }

    this.cancelTodayEventSummaryRefresh();
    const controller = new AbortController();
    this._todayEventSummaryAbort = controller;
    const requestVersion = ++this._todayEventSummaryRequestVersion;
    const { startTime, endTime } = dateRangeForArchiveDay(today);

    try {
      const summaries = await Promise.all(
        baseModel.nvrs.flatMap((nvr) => {
          const summaryUrl = buildNvrEventSummaryUrl(nvr.bridgeBaseUrl, nvr.deviceId);
          if (!summaryUrl) {
            return [];
          }
          return [
            fetchNvrEventSummary(
              summaryUrl,
              {
                startTime,
                endTime,
                eventCode: "all",
              },
              controller.signal,
            ),
          ];
        }),
      );
      if (
        controller.signal.aborted ||
        this._todayEventSummaryAbort !== controller ||
        requestVersion !== this._todayEventSummaryRequestVersion
      ) {
        return;
      }
      this._todayEventSummary = summarizePanelTodayEvents(today, summaries);
      this._todayEventSummaryDate = today;
    } catch {
      if (
        controller.signal.aborted ||
        this._todayEventSummaryAbort !== controller ||
        requestVersion !== this._todayEventSummaryRequestVersion
      ) {
        return;
      }
    } finally {
      if (
        this._todayEventSummaryAbort === controller &&
        requestVersion === this._todayEventSummaryRequestVersion
      ) {
        this._todayEventSummaryAbort = undefined;
        this._todayEventSummaryRefreshedAt = Date.now();
      }
    }
  }

  private cancelTodayEventSummaryRefresh(): void {
    this._todayEventSummaryAbort?.abort();
    this._todayEventSummaryAbort = undefined;
  }

  private selectedPlaybackForCamera(camera: CameraViewModel): SelectedPlaybackState | null {
    if (!this._selectedPlayback) {
      return null;
    }
    return this._selectedPlayback.sourceDeviceId === camera.deviceId ? this._selectedPlayback : null;
  }

  private selectedBridgeRecordingPlaybackForCamera(
    camera: CameraViewModel,
  ): SelectedBridgeRecordingPlaybackState | null {
    if (!this._selectedBridgeRecordingPlayback) {
      return null;
    }
    return this._selectedBridgeRecordingPlayback.sourceDeviceId === camera.deviceId
      ? this._selectedBridgeRecordingPlayback
      : null;
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

  private archiveDownloadBusyKey(recording: { channel: number; startTime: string; endTime: string }): string {
    return `archive-download:${recording.channel}:${recording.startTime}:${recording.endTime}`;
  }

  private bridgeRecordingStopBusyKey(recording: { id: string }): string {
    return `bridge-recording-stop:${recording.id}`;
  }

  private bridgeRecordingDownloadBusyKey(recording: { id: string }): string {
    return `bridge-recording-download:${recording.id}`;
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
      this.logMedia("card panel playback session request", {
        ...this.archiveRecordingLogContext(recording),
        device_id: archiveSource.deviceId,
        url: redactUrlForLog(playbackUrl),
      });
      const session = await createPlaybackSession(
        playbackUrl,
        createPlaybackSessionFromRecording(recording),
        browserBridgeUrl,
      );
      const previousSelection = this._selection;
      const nextState = selectCameraState(archiveSource);
      const detailTab =
        this._detailTab === "events" || this._detailTab === "recordings"
          ? this._detailTab
          : nextState.detailTab;
      this.resetSharedSelectionViewState();
      this.clearVtoSelectionState();
      this.applySelectionTransition(
        {
          ...nextState,
          detailTab,
        },
        {
          openInspector: true,
          cameraProfile:
            this._selectedCameraStreamProfile ??
            defaultSelectedStreamProfileKey(archiveSource.stream) ??
            null,
          cameraSource: preserveCameraViewportSourceSelection(
            archiveSource,
            this._selectedCameraStreamProfile,
            this._selectedCameraStreamSource,
          ),
        },
      );
      this._selectedPlaybackStreamProfile =
        this._selectedPlaybackStreamProfile &&
        session.profiles[this._selectedPlaybackStreamProfile]
          ? this._selectedPlaybackStreamProfile
          : session.recommendedProfile;
      this._selectedPlaybackStreamSource =
        preservePlaybackViewportSourceSelection(
          session,
          this._selectedPlaybackStreamProfile,
          this._selectedPlaybackStreamSource,
        ) ??
        resolveInitialPlaybackViewportSource(
          session,
          this._selectedPlaybackStreamProfile,
          this._selectedPlaybackStreamSource,
        );
      this._selectedPlayback = {
        sourceDeviceId: archiveSource.deviceId,
        bridgeBaseUrl: browserBridgeUrl,
        recording,
        session,
      };
      this._selectedBridgeRecordingPlayback = null;
      this.requestUpdate("_selection", previousSelection);
      this.logMedia("card panel playback session started", {
        ...this.archiveRecordingLogContext(recording),
        device_id: archiveSource.deviceId,
        session_id: session.id,
        recommended_profile: session.recommendedProfile,
      });
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : "Playback session request failed.";
      this.logMedia("card panel playback session failed", {
        ...this.archiveRecordingLogContext(recording),
        device_id: archiveSource.deviceId,
        error: this._errorMessage,
      });
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete(busyKey);
      this._busyActions = reducedBusy;
    }
  }

  private downloadBridgeRecording(recording: BridgeRecordingClipModel): void {
    if (!recording.downloadUrl) {
      return;
    }

    const busyKey = this.bridgeRecordingDownloadBusyKey(recording);
    if (this.isBusy(busyKey)) {
      return;
    }

    const nextBusy = new Set(this._busyActions);
    nextBusy.add(busyKey);
    this._busyActions = nextBusy;

    try {
      this.logMedia("card panel bridge recording download open", {
        recording_id: recording.id,
        url: redactUrlForLog(recording.downloadUrl),
      });
      openExternalUrl(recording.downloadUrl);
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete(busyKey);
      this._busyActions = reducedBusy;
    }
  }

  private playBridgeRecording(
    camera: CameraViewModel,
    recording: BridgeRecordingClipModel,
  ): void {
    if (!recording.playbackUrl) {
      return;
    }
    this._selectedPlayback = null;
    this._selectedPlaybackStreamProfile = null;
    this._selectedPlaybackStreamSource = null;
    this._selectedBridgeRecordingPlayback = {
      sourceDeviceId: camera.deviceId,
      recording,
    };
    this._selectedCameraAudioMuted = true;
    this.logMedia("card panel bridge recording playback selected", {
      device_id: camera.deviceId,
      recording_id: recording.id,
      playback_url: recording.playbackUrl ? redactUrlForLog(recording.playbackUrl) : null,
    });
  }

  private async stopBridgeRecording(recording: BridgeRecordingClipModel): Promise<void> {
    if (!recording.stopUrl) {
      return;
    }

    const busyKey = this.bridgeRecordingStopBusyKey(recording);
    if (this.isBusy(busyKey)) {
      return;
    }

    const nextBusy = new Set(this._busyActions);
    nextBusy.add(busyKey);
    this._busyActions = nextBusy;
    this._errorMessage = "";

    try {
      this.logMedia("card panel bridge recording stop request", {
        recording_id: recording.id,
        url: redactUrlForLog(recording.stopUrl),
      });
      await postBridgeRequest(recording.stopUrl);
      await this.refreshBridgeRecordings();
      this.logMedia("card panel bridge recording stop completed", {
        recording_id: recording.id,
      });
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : "Bridge MP4 stop request failed.";
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete(busyKey);
      this._busyActions = reducedBusy;
    }
  }

  private stopSelectedPlayback(): void {
    if (this._selectedPlayback) {
      this.logMedia("card panel playback session cleared", {
        session_id: this._selectedPlayback.session.id,
        ...this.archiveRecordingLogContext(this._selectedPlayback.recording),
      });
    }
    const previousPlayback = this._selectedPlayback;
    this._selectedPlayback = null;
    this._selectedPlaybackStreamProfile = null;
    this._selectedPlaybackStreamSource = null;
    this.requestUpdate("_selectedPlayback", previousPlayback);
  }

  private stopSelectedBridgeRecordingPlayback(): void {
    if (this._selectedBridgeRecordingPlayback) {
      this.logMedia("card panel bridge recording playback cleared", {
        recording_id: this._selectedBridgeRecordingPlayback.recording.id,
      });
    }
    const previousPlayback = this._selectedBridgeRecordingPlayback;
    this._selectedBridgeRecordingPlayback = null;
    this.requestUpdate("_selectedBridgeRecordingPlayback", previousPlayback);
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

  private isBridgeRecordingPlaybackActive(recording: BridgeRecordingClipModel): boolean {
    return this._selectedBridgeRecordingPlayback?.recording.id === recording.id;
  }

  private async downloadArchiveRecording(recording: NvrArchiveRecordingModel): Promise<void> {
    if (recording.downloadUrl) {
      this.logMedia("card panel archive download open", {
        ...this.archiveRecordingLogContext(recording),
        url: redactUrlForLog(recording.downloadUrl),
      });
      openExternalUrl(recording.downloadUrl);
      return;
    }

    if (!recording.exportUrl) {
      return;
    }

    const busyKey = this.archiveDownloadBusyKey(recording);
    if (this.isBusy(busyKey)) {
      return;
    }

    const nextBusy = new Set(this._busyActions);
    nextBusy.add(busyKey);
    this._busyActions = nextBusy;
    this._errorMessage = "";

    try {
      this.logMedia("card panel archive export request", {
        ...this.archiveRecordingLogContext(recording),
        url: redactUrlForLog(recording.exportUrl),
      });
      const startedClip = await exportArchiveRecording(recording.exportUrl);
      this.logMedia("card panel archive export started", {
        ...this.archiveRecordingLogContext(recording),
        clip_id: startedClip.id,
        status: startedClip.status,
      });
      const completedClip = await waitForArchiveExportCompletion(startedClip);
      if (!completedClip.downloadUrl) {
        throw new Error("Bridge archive export completed without a download URL.");
      }
      this.patchArchiveRecordingDownloadUrl(recording, completedClip.downloadUrl);
      this.logMedia("card panel archive export completed", {
        ...this.archiveRecordingLogContext(recording),
        clip_id: completedClip.id,
        download_url: redactUrlForLog(completedClip.downloadUrl),
      });
      openExternalUrl(completedClip.downloadUrl);
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : "Bridge archive export request failed.";
      this.logMedia("card panel archive export failed", {
        ...this.archiveRecordingLogContext(recording),
        error: this._errorMessage,
      });
    } finally {
      const reducedBusy = new Set(this._busyActions);
      reducedBusy.delete(busyKey);
      this._busyActions = reducedBusy;
    }
  }

  private patchArchiveRecordingDownloadUrl(
    recording: NvrArchiveRecordingModel,
    downloadUrl: string,
  ): void {
    if (!this._archiveRecordings) {
      return;
    }

    const nextItems = this._archiveRecordings.items.map((item) =>
      item.channel === recording.channel &&
      item.startTime === recording.startTime &&
      item.endTime === recording.endTime
        ? {
            ...item,
            downloadUrl,
          }
        : item,
    );
    this._archiveRecordings = {
      ...this._archiveRecordings,
      items: nextItems,
    };

    if (
      this._selectedPlayback &&
      this._selectedPlayback.recording.channel === recording.channel &&
      this._selectedPlayback.recording.startTime === recording.startTime &&
      this._selectedPlayback.recording.endTime === recording.endTime
    ) {
      this._selectedPlayback = {
        ...this._selectedPlayback,
        recording: {
          ...this._selectedPlayback.recording,
          downloadUrl,
        },
      };
    }
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
      this.logMedia("card panel playback seek request", {
        session_id: playback.session.id,
        seek_time: seekTime.toISOString(),
        percent: bounded,
      });
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
      this._selectedPlaybackStreamProfile =
        this._selectedPlaybackStreamProfile && session.profiles[this._selectedPlaybackStreamProfile]
          ? this._selectedPlaybackStreamProfile
          : session.recommendedProfile;
      this._selectedPlaybackStreamSource = preservePlaybackViewportSourceSelection(
        session,
        this._selectedPlaybackStreamProfile,
        this._selectedPlaybackStreamSource,
      );
      this.logMedia("card panel playback seek completed", {
        session_id: session.id,
        seek_time: session.seekTime,
        recommended_profile: session.recommendedProfile,
      });
    } catch (error) {
      this._errorMessage =
        error instanceof Error ? error.message : "Playback seek request failed.";
      this.logMedia("card panel playback seek failed", {
        session_id: playback.session.id,
        seek_time: seekTime.toISOString(),
        error: this._errorMessage,
      });
    }
  }

  private archiveRecordingLogContext(recording: NvrArchiveRecordingModel): Record<string, unknown> {
    return {
      channel: recording.channel,
      start_time: recording.startTime,
      end_time: recording.endTime,
      recording_type: recording.type ?? null,
    };
  }

  private logMedia(message: string, details?: Record<string, unknown>): void {
    logCardInfo(message, {
      card: "surveillance-panel",
      selection_kind: this._selection.kind,
      ...details,
    });
  }

  private resolvePlaybackSeekUrl(bridgeBaseUrl: string | null): string {
    const bridgeUrl = buildBridgeEndpointUrl(
      bridgeBaseUrl,
      "/api/v1/nvr/playback/sessions/{session_id}/seek",
    );
    if (bridgeUrl) {
      return bridgeUrl;
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
    void this.refreshBridgeRecordings();
  }

  private async toggleSelectedCameraAudio(camera: CameraViewModel): Promise<void> {
    const nextMuted = !this._selectedCameraAudioMuted;
    const previousMuted = this._selectedCameraAudioMuted;
    this._selectedCameraAudioMuted = nextMuted;
    this.requestUpdate("_selectedCameraAudioMuted", previousMuted);
    this.syncSelectedCameraViewportAudioState(nextMuted);
    this.logMedia("card panel camera audio toggled", {
      device_id: camera.deviceId,
      muted: nextMuted,
      audio_supported: camera.audioMuteSupported,
    });

    const playbackActive =
      this._selectedPlayback?.sourceDeviceId === camera.deviceId ||
      this._selectedBridgeRecordingPlayback?.sourceDeviceId === camera.deviceId;
    if (playbackActive || !camera.audioMuteSupported) {
      return;
    }
  }

  private isBusy(key: string): boolean {
    return this._actions.isBusy(key);
  }

  private renderIcon(icon: string): TemplateResult {
    return html`<ha-icon .icon=${icon}></ha-icon>`;
  }

  private streamSourceLabel(source: CameraViewportSource): string {
    switch (source) {
      case "native":
        return "Native HA";
      case "dash":
        return "DASH";
      case "hls":
        return "HLS";
      case "mjpeg":
        return "MJPEG";
    }
  }

  private canPlaySelectedCameraAudio(
    camera: CameraViewModel,
    selectedPlayback: SelectedPlaybackState | null,
    selectedBridgeRecordingPlayback: SelectedBridgeRecordingPlaybackState | null,
    selectedProfileKey: string | null,
    selectedSource: CameraViewportSource | null,
  ): boolean {
    if (!camera.audioCodec.trim()) {
      return false;
    }

    if (selectedPlayback) {
      return this.canPlaySelectedPlaybackAudio(
        selectedPlayback.session,
        this._selectedPlaybackStreamProfile ?? selectedProfileKey,
        selectedSource,
      );
    }

    if (selectedBridgeRecordingPlayback?.recording.playbackUrl) {
      return true;
    }

    const profile = resolveSelectedCameraStreamProfile(camera, selectedProfileKey);
    if (!profile) {
      return false;
    }
    switch (selectedSource) {
      case "native":
        return Boolean(camera.cameraEntity);
      case "dash":
        return Boolean(profile.localDashUrl);
      case "hls":
        return Boolean(profile.localHlsUrl);
      default:
        return false;
    }
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

  private canPlaySelectedPlaybackAudio(
    session: NvrPlaybackSessionModel,
    selectedProfileKey: string | null,
    selectedSource: CameraViewportSource | null,
  ): boolean {
    return (
      (selectedSource === "hls" || selectedSource === "dash") &&
      availablePlaybackViewportSources(session, selectedProfileKey).includes(selectedSource)
    );
  }

  private syncSelectedCameraViewportAudioState(muted: boolean): void {
    syncViewportAudioState(
      this.renderRoot.querySelector("section.main .viewport"),
      muted,
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

    this.logMedia("card panel vto microphone enable", {
      device_id: vto.deviceId,
      offer_url: redactUrlForLog(offerUrl),
    });
    await this._intercomSession.enable(offerUrl);
  }

  private async stopSelectedVtoMicrophone(): Promise<void> {
    this.logMedia("card panel vto microphone disable");
    await this._intercomSession.disable();
  }

  private toggleOverviewVtoStream(vto: VtoViewModel): void {
    const isCurrentTile =
      this._selection.kind === "vto" && this._selection.deviceId === vto.deviceId;
    const shouldPlay = !(isCurrentTile && this._selectedVtoStreamPlaying);

    this.selectVto(vto);
    this._selectedVtoStreamPlaying = shouldPlay;
    this.logMedia("card panel vto stream toggled", {
      device_id: vto.deviceId,
      playing: shouldPlay,
    });
    if (this._selectedVtoStreamProfile === null) {
      this._selectedVtoStreamProfile = defaultSelectedStreamProfileKey(vto.stream);
    }
    if (this._selectedVtoStreamSource === null) {
      this._selectedVtoStreamSource = resolveStreamViewportSource(
        vto.stream,
        null,
        this._selectedVtoStreamProfile,
        Boolean(vto.cameraEntity),
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

function todayDateInputValue(): string {
  return toDateInputValue(new Date());
}

function normalizeArchiveDateInput(value: string): string {
  const trimmed = value.trim();
  if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) {
    const parsed = new Date(`${trimmed}T00:00:00`);
    if (!Number.isNaN(parsed.getTime()) && toDateInputValue(parsed) === trimmed) {
      return trimmed;
    }
  }
  return todayDateInputValue();
}

function dateRangeForArchiveDay(value: string): {
  startTime: string;
  endTime: string;
} {
  const normalized = normalizeArchiveDateInput(value);
  const [year, month, day] = normalized.split("-").map((part) => Number.parseInt(part, 10));
  const start = new Date(year, month - 1, day, 0, 0, 0, 0);
  const end = new Date(year, month - 1, day + 1, 0, 0, 0, 0);
  return {
    startTime: start.toISOString(),
    endTime: end.toISOString(),
  };
}

function toDateInputValue(value: Date): string {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function pageCountForItems(totalItems: number, pageSize: number): number {
  const safeSize = Math.max(1, Math.trunc(pageSize));
  return Math.max(1, Math.ceil(Math.max(0, totalItems) / safeSize));
}

function boundedPageIndex(page: number, pageCount: number): number {
  const safePage = Number.isFinite(page) ? Math.trunc(page) : 0;
  return Math.min(Math.max(safePage, 0), Math.max(pageCount - 1, 0));
}

function slicePage<T>(items: readonly T[], page: number, pageSize: number): T[] {
  const safeSize = Math.max(1, Math.trunc(pageSize));
  const safePage = boundedPageIndex(page, pageCountForItems(items.length, safeSize));
  const start = safePage * safeSize;
  return items.slice(start, start + safeSize);
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
