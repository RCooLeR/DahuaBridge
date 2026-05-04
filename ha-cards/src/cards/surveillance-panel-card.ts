import type {TemplateResult} from "lit";
import {html, LitElement, nothing} from "lit";
import {keyed} from "lit/directives/keyed.js";
import {repeat} from "lit/directives/repeat.js";

import {createSurveillancePanelStubConfig} from "./surveillance-panel-card-editor";
import {SurveillancePanelActions} from "./surveillance-panel-actions";
import {SurveillancePanelRuntime} from "./surveillance-panel-runtime";
import {surveillancePanelStyles} from "./surveillance-panel-styles";
import {renderCameraEventCountBadges} from "./surveillance-panel-event-badges";
import {renderSurveillancePanelEvents} from "./surveillance-panel-events";
import {renderSurveillancePanelHeader} from "./surveillance-panel-header";
import {renderSurveillancePanelSidebar} from "./surveillance-panel-sidebar";
import {renderSurveillancePanelOverview} from "./surveillance-panel-overview";
import {renderSurveillancePanelInspector} from "./surveillance-panel-inspector";
import {
    localizeIntercomError,
    localizeIntercomStatus,
} from "./surveillance-panel-intercom-status";
import {
    renderArchiveRecordings,
    renderBridgeRecordings,
    resolveSelectedNvrArchiveCamera,
} from "./surveillance-panel-archive";
import {
    availablePlaybackViewportSources,
    availableStreamViewportSources,
    cameraImageSrc,
    buildRtspPlaybackUrl,
    type CameraViewportSource,
    defaultOverviewStreamProfileKey,
    defaultSelectedStreamProfileKey,
    preservePlaybackViewportSourceSelection,
    preserveCameraViewportSourceSelectionOnProfileChange,
    renderClipPlaybackViewport,
    renderNativePlaybackViewport,
    renderTimeframePlaybackViewport,
    renderPlaybackViewport,
    renderSelectedCameraViewport,
    renderSelectedVtoViewport,
    resolveOverviewCameraViewportSource,
    resolveSelectedCameraStreamProfile,
    resolveStreamViewportSource,
    syncRemoteStreamStyles,
    syncViewportAudioState,
} from "./surveillance-panel-media";
import {
    ARCHIVE_PAGE_SIZE,
    archiveMissingUrlMessage,
    archiveModeForSelection,
    archiveUrlForMode,
    type ArchiveRecordingsMode,
} from "./surveillance-panel-archive-state-model";
import {
    archiveCurrentTimeOfDaySecond,
    archiveDefaultTimeframeEndTime,
    buildArchiveTimeframeSnapshotProxyPath,
    archiveMaxSecondForDate,
    dateRangeForArchiveDay,
    normalizeArchiveDateInput,
    secondsSinceLocalMidnight,
    signHomeAssistantPath,
    todayDateInputValue,
    toDateInputValue,
} from "./surveillance-panel-archive-seek-model";
import {renderArchiveSeekPanel as renderArchiveSeekPanelView} from "./surveillance-panel-archive-seek";
import {selectedCameraLiveStreamModel} from "./surveillance-panel-live-stream-model";
import {
    bridgeRecordingDownloadBusyKey as bridgeRecordingDownloadActionKey,
    MP4_PAGE_SIZE,
    selectedBridgeRecordingPlaybackForCamera as selectBridgeRecordingPlaybackForCamera,
    type SelectedBridgeRecordingPlaybackState,
} from "./surveillance-panel-mp4-model";
import {
    createSelectedNativePlaybackState,
    isArchiveEventRecording,
    nativePlaybackMatchesRecording,
    resolveMainArchivePlaybackProfile,
    selectedNativePlaybackForCamera as selectNativePlaybackForCamera,
    type SelectedNativePlaybackState,
} from "./surveillance-panel-native-playback-model";
import {
    renderControlButton as renderControlPrimitive,
    renderIconButton as renderIconPrimitive,
    renderSegmentButton as renderSegmentPrimitive,
    type ControlTone,
} from "./surveillance-panel-primitives";
import {
    DEFAULT_STREAM_VOLUME,
    clampStreamVolume,
    streamVolumeFromInputValue,
    streamVolumeIcon,
    streamVolumePercent,
} from "./surveillance-panel-player-audio-model";
import type {BridgeEvent} from "../ha/bridge-events";
import type {
    BridgeRecordingClipListModel,
    BridgeRecordingClipModel,
    NvrPlaybackSessionModel,
    NvrPlaybackSessionRequestModel,
    NvrArchiveExportClipModel,
    NvrArchiveRecordingModel,
    NvrArchiveSearchResultModel,
} from "../domain/archive";
import { createPlaybackSessionRequest } from "../domain/archive";
import {
    exportArchiveRecording,
    fetchArchiveRecordings,
    fetchBridgeRecordings,
    waitForArchiveExportCompletion,
} from "../ha/bridge-archive";
import {buildNvrEventSummaryUrl, fetchNvrEventSummary,} from "../ha/bridge-event-summary";
import {createPlaybackSession} from "../ha/bridge-playback";
import { buildBridgeEndpointUrl } from "../ha/bridge-url";
import {postBridgeRequest} from "../ha/actions";
import {
    BridgeIntercomSessionController,
    type BridgeIntercomSnapshot,
    resolveIntercomOfferUrl,
} from "../ha/bridge-intercom";
import type {RegistrySnapshot} from "../ha/registry";
import {
    buildPanelModel,
    type CameraViewModel,
    displayCameraLabel,
    findAuxTarget,
    type NvrViewModel,
    type PanelModel,
    type PanelSelection,
    resolveAuxTargetAction,
    type SidebarFilter,
    supportsAuxTarget,
    type VtoViewModel,
} from "../domain/model";
import {type PanelTodayEventSummaryModel, summarizePanelTodayEvents,} from "../domain/event-summary";
import {openExternalUrl} from "../utils/browser";
import {logCardInfo, redactUrlForLog} from "../utils/logging";
import {parseConfig, type SurveillancePanelCardConfig} from "../types/card-config";
import type {HomeAssistant, LovelaceCard, LovelaceCardConfig,} from "../types/home-assistant";
import {createLocalizer, resolvePanelLanguage, type Localizer} from "../localization";
import {
    boundedEventHistoryPage,
    buildTimelineEventFilterOptions,
    defaultTimelineEventFilters,
    type DetailTab,
    EVENT_FILTER_ALL,
    type EventViewMode,
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
    {hours: 1, label: "1H"},
    {hours: 6, label: "6H"},
    {hours: 24, label: "24H"},
    {hours: 168, label: "7D"},
] as const;

const ARCHIVE_EVENT_TYPE_OPTIONS = [
    {value: EVENT_FILTER_ALL, labelKey: "archive.event.all"},
    {value: "smdTypeHuman", labelKey: "archive.event.human"},
    {value: "smdTypeVehicle", labelKey: "archive.event.vehicle"},
    {value: "smdTypeAnimal", labelKey: "archive.event.animal"},
    {value: "CrossLineDetection", labelKey: "archive.event.crossLine"},
    {value: "CrossRegionDetection", labelKey: "archive.event.crossRegion"},
] as const;

const ACTION_STATE_OVERRIDE_TTL_MS = 10_000;

function isValidPlaybackSnapshotRange(startTime: Date, endTime: Date, seekTime: Date): boolean {
    return (
        !Number.isNaN(startTime.getTime()) &&
        !Number.isNaN(endTime.getTime()) &&
        !Number.isNaN(seekTime.getTime()) &&
        endTime > startTime &&
        seekTime >= startTime &&
        seekTime <= endTime
    );
}

interface TimedActionStateOverride {
    active: boolean;
    expiresAt: number;
}

interface SelectedPlaybackState {
    sourceDeviceId: string;
    recording: NvrArchiveRecordingModel | null;
    session: NvrPlaybackSessionModel;
    nativeStreamSource: string | null;
}

const INITIAL_VTO_MICROPHONE_STATE: BridgeIntercomSnapshot = {
    enabled: false,
    phase: "idle",
    statusText: "Mic inactive",
    error: "",
};

export class DahuaBridgeSurveillancePanelCard
    extends LitElement
    implements LovelaceCard {
    private _remoteStreamSyncTimer: number | null = null;
    private _viewportAudioSyncTimer: number | null = null;
    private _archiveAbort?: AbortController;
    private _archiveRequestVersion = 0;
    private _mp4Abort?: AbortController;
    private _mp4RequestVersion = 0;
    private _todayEventSummaryAbort?: AbortController;
    private _todayEventSummaryRequestVersion = 0;
    private _todayEventSummaryRefreshedAt = 0;
    private _todayEventSummaryRefreshTimer: number | null = null;
    private _suppressNextArchiveRefresh = false;

    static async getConfigElement(): Promise<HTMLElement> {
        return document.createElement("dahuabridge-surveillance-panel-editor");
    }

    static getStubConfig(): SurveillancePanelCardConfig {
        return createSurveillancePanelStubConfig();
    }

    static properties = {
        hass: {attribute: false},
        _config: {state: true},
        _bridgeEvents: {state: true},
        _registrySnapshot: {state: true},
        _eventsLoading: {state: true},
        _eventError: {state: true},
        _eventHistoryPage: {state: true},
        _eventViewMode: {state: true},
        _eventWindowHours: {state: true},
        _eventCodeFilter: {state: true},
        _archiveEventCodeFilter: {state: true},
        _archiveDate: {state: true},
        _archiveSeekSecond: {state: true},
        _archivePage: {state: true},
        _selection: {state: true},
        _sidebarFilter: {state: true},
        _searchText: {state: true},
        _detailTab: {state: true},
        _ptzAdjusting: {state: true},
        _busyActions: {state: true},
        _errorMessage: {state: true},
        _sidebarOpen: {state: true},
        _inspectorOpen: {state: true},
        _nvrArchiveChannelNumber: {state: true},
        _smdIvsRecordings: {state: true},
        _smdIvsLoading: {state: true},
        _smdIvsError: {state: true},
        _chunkRecordings: {state: true},
        _chunkLoading: {state: true},
        _chunkError: {state: true},
        _archiveRecordingsMode: {state: true},
        _bridgeRecordings: {state: true},
        _bridgeRecordingsLoading: {state: true},
        _bridgeRecordingsError: {state: true},
        _mp4Page: {state: true},
        _todayEventSummary: {state: true},
        _selectedCameraStreamProfile: {state: true},
        _selectedCameraStreamSource: {state: true},
        _selectedPlaybackStreamProfile: {state: true},
        _selectedPlaybackStreamSource: {state: true},
        _selectedCameraAudioMuted: {state: true},
        _selectedCameraVolume: {state: true},
        _overviewCameraAudioMuted: {state: true},
        _selectedVtoStreamProfile: {state: true},
        _selectedVtoStreamSource: {state: true},
        _selectedVtoStreamPlaying: {state: true},
        _selectedVtoMicrophoneState: {state: true},
        _selectedPlayback: {state: true},
        _selectedBridgeRecordingPlayback: {state: true},
        _selectedNativePlayback: {state: true},
        _auxStateOverrides: {state: true},
        _recordingStateOverrides: {state: true},
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
    private _archiveSeekSecond = archiveCurrentTimeOfDaySecond();
    private _archivePage = 0;
    private _selection: PanelSelection = {kind: "overview"};
    private _sidebarFilter: SidebarFilter = "all";
    private _searchText = "";
    private _detailTab: DetailTab = "overview";
    private _ptzAdjusting = false;
    private _busyActions = new Set<string>();
    private _errorMessage = "";
    private _sidebarOpen = true;
    private _inspectorOpen = true;
    private _nvrArchiveChannelNumber: number | null = null;
    private _smdIvsRecordings: NvrArchiveSearchResultModel | null = null;
    private _smdIvsLoading = false;
    private _smdIvsError = "";
    private _chunkRecordings: NvrArchiveSearchResultModel | null = null;
    private _chunkLoading = false;
    private _chunkError = "";
    private _archiveRecordingsMode: ArchiveRecordingsMode | null = null;
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
    private _selectedCameraVolume = DEFAULT_STREAM_VOLUME;
    private _overviewCameraAudioMuted: Record<string, boolean> = {};
    private _selectedVtoStreamProfile: string | null = null;
    private _selectedVtoStreamSource: CameraViewportSource | null = null;
    private _selectedVtoStreamPlaying = false;
    private _selectedVtoMicrophoneState = INITIAL_VTO_MICROPHONE_STATE;
    private _selectedPlayback: SelectedPlaybackState | null = null;
    private _selectedBridgeRecordingPlayback: SelectedBridgeRecordingPlaybackState | null = null;
    private _selectedNativePlayback: SelectedNativePlaybackState | null = null;
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
                this._errorMessage = localizeIntercomError(snapshot.error, this.t());
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
        this._archiveSeekSecond = archiveCurrentTimeOfDaySecond();
        this._archivePage = 0;
        this._selection = {kind: "overview"};
        this._detailTab = "overview";
        this._ptzAdjusting = false;
        this._errorMessage = "";
        this._nvrArchiveChannelNumber = null;
        this._smdIvsRecordings = null;
        this._smdIvsLoading = false;
        this._smdIvsError = "";
        this._chunkRecordings = null;
        this._chunkLoading = false;
        this._chunkError = "";
        this._archiveRecordingsMode = null;
        this._bridgeRecordings = null;
        this._bridgeRecordingsLoading = false;
        this._bridgeRecordingsError = "";
        this._mp4Page = 0;
        this._todayEventSummary = null;
        this._todayEventSummaryRefreshedAt = 0;
        this._selectedCameraStreamProfile = null;
        this._selectedCameraStreamSource = null;
        this._selectedPlaybackStreamProfile = null;
        this._selectedPlaybackStreamSource = null;
        this._selectedCameraAudioMuted = true;
        this._selectedCameraVolume = DEFAULT_STREAM_VOLUME;
        this._overviewCameraAudioMuted = {};
        this._selectedVtoStreamProfile = null;
        this._selectedVtoStreamSource = null;
        this._selectedVtoStreamPlaying = false;
        this._selectedVtoMicrophoneState = INITIAL_VTO_MICROPHONE_STATE;
        this._selectedPlayback = null;
        this._selectedBridgeRecordingPlayback = null;
        this._selectedNativePlayback = null;
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
        if (this._viewportAudioSyncTimer !== null) {
            window.clearTimeout(this._viewportAudioSyncTimer);
            this._viewportAudioSyncTimer = null;
        }
        this.cancelArchiveRefresh();
        this.cancelMp4Refresh();
        this.cancelTodayEventSummaryRefresh();
        void this.clearCameraNativePlaybackSource(this._selectedNativePlayback?.cameraEntityId ?? null);
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
        if (this.shouldRefreshArchiveRecordings(changedProperties)) {
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

        if (this.shouldSyncMediaAfterUpdate(changedProperties)) {
            this.scheduleRemoteStreamStyleSync();
            this.scheduleViewportAudioSync();
        }
        this.maybeStartSelectedVtoVideo(changedProperties);
    }

    getCardSize(): number {
        return 20;
    }

    render(): TemplateResult {
        if (!this._config) {
            const t = createLocalizer("en");
            return html`
                <ha-card>
                    <div class="empty-state">${t("panel.configMissing")}</div>
                </ha-card>`;
        }

        if (!this.hass) {
            const t = createLocalizer("en");
            return html`
                <ha-card>
                    <div class="empty-state">${t("panel.hassMissing")}</div>
                </ha-card>`;
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
        const t = createLocalizer(model.language);
        return renderSurveillancePanelSidebar({
            model,
            t,
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
        const t = createLocalizer(model.language);
        const inspectorAvailable =
            model.selectedCamera !== null || model.selectedNvr !== null || model.selectedVto !== null;
        return renderSurveillancePanelHeader({
            model,
            t,
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
        const t = createLocalizer(model.language);
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
                selectBridgeRecordingPlaybackForCamera(this._selectedBridgeRecordingPlayback, camera);
            const selectedNativePlayback = selectNativePlaybackForCamera(this._selectedNativePlayback, camera);
            const playbackActive =
                selectedPlayback !== null ||
                selectedBridgeRecordingPlayback !== null ||
                selectedNativePlayback !== null;
            const liveStreamModel =
                selectedPlayback === null
                    ? selectedCameraLiveStreamModel(
                        camera,
                        this._selectedCameraStreamProfile,
                        this._selectedCameraStreamSource,
                        selectedNativePlayback,
                    )
                    : null;
            const selectedPlaybackProfileKey =
                selectedPlayback !== null
                    ? this._selectedPlaybackStreamProfile ?? selectedPlayback.session.recommendedProfile
                    : null;
            const selectedStreamProfileKey =
                selectedPlayback !== null
                    ? selectedPlaybackProfileKey
                    : liveStreamModel?.selectedProfileKey ?? null;
            const selectedStreamSource =
                selectedPlayback !== null
                    ? this.resolveSelectedPlaybackViewportSource(
                        selectedPlayback,
                        camera,
                        this._selectedPlaybackStreamSource,
                        selectedStreamProfileKey,
                    )
                    : liveStreamModel?.selectedSource ?? null;
            const availableStreamSources =
                selectedPlayback !== null
                    ? this.availableSelectedPlaybackViewportSources(selectedPlayback, camera, selectedStreamProfileKey)
                    : liveStreamModel?.availableSources ?? [];
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
                                ${displayCameraLabel(camera)} - ${camera.roomLabel} -
                                ${playbackActive ? t("view.playback") : t("view.live")}
                            </div>
                            <div class="split-row">
                <span class="badge ${camera.online ? "success" : "critical"}">
                  ${camera.online ? t("state.online") : t("state.offline")}
                </span>
                                ${camera.recordingActive
                                        ? html`<span class="badge critical">${t("badge.nvrRecording")}</span>`
                                        : nothing}
                                ${bridgeRecordingActive
                                        ? html`<span class="badge warning">${t("badge.mp4Clip")}</span>`
                                        : nothing}
                                ${repeat(
                                        camera.detections,
                                        (badge) => badge.key,
                                        (badge) =>
                                                html`<span class="badge ${badge.tone}">${badge.label}</span>`,
                                )}
                                ${camera.microphoneAvailable && audioCodecLabel
                                        ? html`<span class="badge info">${t("badge.micCodec", {codec: audioCodecLabel})}</span>`
                                        : nothing}
                                ${camera.speakerAvailable
                                        ? html`<span class="badge neutral">${t("badge.speaker")}</span>`
                                        : nothing}
                            </div>
                        </div>
                        <div class="video-panel">
                            ${((selectedPlayback === null && selectedNativePlayback === null && camera.stream.profiles.length > 1) ||
                                    (selectedPlayback !== null && playbackProfileKeys.length > 1) ||
                                    availableStreamSources.length > 1 ||
                                    hasCameraEventCounts(camera))
                                    ? html`
                                        <div class="detail-media-toolbar">
                                            ${selectedPlayback === null && selectedNativePlayback === null && camera.stream.profiles.length > 1
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
                                                                                                ) ??
                                                                                                this.resolveInitialSelectedPlaybackViewportSource(
                                                                                                        selectedPlayback,
                                                                                                        camera,
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
                                            ${((selectedPlayback === null && selectedNativePlayback === null && camera.stream.profiles.length > 1) ||
                                                    (selectedPlayback !== null && playbackProfileKeys.length > 1)) &&
                                            availableStreamSources.length > 1
                                                    ? html`
                                                        <div class="detail-media-separator" aria-hidden="true"></div>`
                                                    : nothing}
                                            ${availableStreamSources.length > 1
                                                    ? html`
                                                        <div class="detail-media-group chip-row">
                                                            ${availableStreamSources.map((source) =>
                                                                    renderSegmentPrimitive(
                                                                            source,
                                                                            this.streamSourceLabel(source, t),
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
                                            ${hasCameraEventCounts(camera)
                                                    ? html`
                                                        <div class="detail-media-group detail-media-group-spacer"></div>
                                                        <div class="detail-media-group detail-media-event-counts">
                                                            ${renderCameraEventCountBadges(camera, "inline", t)}
                                                        </div>
                                                    `
                                                    : nothing}
                                        </div>
                                    `
                                    : nothing}
                            <div class="viewport">
                                ${selectedPlayback
                                        ? renderPlaybackViewport(
                                                this.hass,
                                                camera.cameraEntity,
                                                selectedPlayback.session,
                                                selectedStreamProfileKey,
                                                selectedStreamSource,
                                                this._selectedCameraAudioMuted,
                                                this._selectedCameraVolume,
                                                selectedPlayback.nativeStreamSource,
                                                t,
                                                camera.stream.fallbacksEnabled,
                                        )
                                        : selectedBridgeRecordingPlayback?.recording.playbackUrl
                                                ? renderClipPlaybackViewport(
                                                        selectedBridgeRecordingPlayback.recording.playbackUrl,
                                                        displayCameraLabel(camera),
                                                        this._selectedCameraAudioMuted,
                                                        this._selectedCameraVolume,
                                                )
                                                : selectedNativePlayback?.streamSource
                                                 ? keyed(
                                                                selectedNativePlayback.streamSource,
                                                                this.renderSelectedNativePlaybackViewport(camera, selectedNativePlayback, t),
                                                        )
                                                : renderSelectedCameraViewport(
                                                        this.hass,
                                                        camera,
                                                        selectedStreamProfileKey,
                                                        this._selectedCameraStreamSource,
                                                        this._selectedCameraAudioMuted,
                                                        this._selectedCameraVolume,
                                                        {
                                                            t,
                                                        },
                                                )}
                                ${!playbackActive && this._ptzAdjusting && camera.supportsPtz
                                        ? this.renderPtzOverlay(camera, t)
                                        : nothing}
                                <div class="viewport-controls">
                                    ${selectedPlayback
                                            ? html`
                                                ${this.renderViewportIconButton(
                                                        t("button.returnLive"),
                                                        "mdi:cctv",
                                                        () => this.stopSelectedPlayback(),
                                                        {
                                                            tone: "primary",
                                                        },
                                                )}
                                                ${this.hasSnapshot(camera)
                                                        ? this.renderViewportIconButton(
                                                                t("button.snapshot"),
                                                                "mdi:camera",
                                                                () => this.openSnapshot(camera),
                                                        )
                                                        : nothing}
                                                ${selectedPlayback.recording?.downloadUrl ||
                                                selectedPlayback.recording?.exportUrl
                                                        ? this.renderViewportIconButton(
                                                                selectedPlayback.recording.downloadUrl ? t("button.download") : t("button.export"),
                                                                "mdi:download",
                                                                () => {
                                                                    if (selectedPlayback.recording) {
                                                                        void this.downloadArchiveRecording(selectedPlayback.recording);
                                                                    }
                                                                },
                                                                {
                                                                    disabled: selectedPlayback.recording
                                                                            ? this.isBusy(
                                                                                    this.archiveDownloadBusyKey(
                                                                                            selectedPlayback.recording,
                                                                                    ),
                                                                            )
                                                                            : false,
                                                                },
                                                        )
                                                        : nothing}
                                                ${this.renderSelectedCameraAudioControl(camera, t)}
                                            `
                                            : selectedBridgeRecordingPlayback
                                                    ? html`
                                                        ${this.renderViewportIconButton(
                                                                t("button.returnLive"),
                                                                "mdi:cctv",
                                                                () => this.stopSelectedBridgeRecordingPlayback(),
                                                                {
                                                                    tone: "primary",
                                                                },
                                                         )}
                                                        ${this.hasSnapshot(camera)
                                                                ? this.renderViewportIconButton(
                                                                        t("button.snapshot"),
                                                                        "mdi:camera",
                                                                        () => this.openSnapshot(camera),
                                                                )
                                                                : nothing}
                                                        ${selectedBridgeRecordingPlayback.recording.downloadUrl
                                                                ? this.renderViewportIconButton(
                                                                        t("button.download"),
                                                                        "mdi:download",
                                                                                () =>
                                                                                        this.downloadBridgeRecording(
                                                                                                selectedBridgeRecordingPlayback.recording,
                                                                                        ),
                                                                        {
                                                                            disabled: this.isBusy(
                                                                                    bridgeRecordingDownloadActionKey(
                                                                                            selectedBridgeRecordingPlayback.recording,
                                                                                    ),
                                                                            ),
                                                                        },
                                                                )
                                                                : nothing}
                                                        ${this.renderSelectedCameraAudioControl(camera, t)}
                                                    `
                                                     : selectedNativePlayback
                                                             ? html`
                                                                ${this.renderViewportIconButton(
                                                                        t("button.returnLive"),
                                                                        "mdi:cctv",
                                                                        () => this.stopSelectedNativePlayback(),
                                                                        {
                                                                            tone: "primary",
                                                                         },
                                                                 )}
                                                                 ${this.hasSnapshot(camera)
                                                                         ? this.renderViewportIconButton(
                                                                                 t("button.snapshot"),
                                                                                 "mdi:camera",
                                                                                 () => this.openSnapshot(camera),
                                                                         )
                                                                         : nothing}
                                                                 ${this.renderSelectedCameraAudioControl(camera, t)}
                                                             `
                                                             : html`
                                                                ${this.hasSnapshot(camera)
                                                                        ? this.renderViewportIconButton(
                                                                                t("button.snapshot"),
                                                                                "mdi:camera",
                                                                                () => this.openSnapshot(camera),
                                                                        )
                                                                        : nothing}
                                                                ${camera.supportsRecording
                                                                        ? this.renderViewportIconButton(
                                                                                bridgeRecordingActive ? t("button.stopMp4") : t("button.startMp4"),
                                                                                bridgeRecordingActive
                                                                                        ? "mdi:record-rec"
                                                                                        : "mdi:record-circle-outline",
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
                                                                                    active: bridgeRecordingActive,
                                                                                },
                                                                        )
                                                                        : nothing}
                                                                ${camera.supportsAux && lightAvailable
                                                                        ? this.renderViewportIconButton(
                                                                                lightActive ? t("button.lightSmart") : t("button.lightWhite"),
                                                                                "mdi:lightbulb-on-outline",
                                                                                () => this.triggerAuxAction(camera, "light"),
                                                                                {
                                                                                    disabled: this.isBusy(`${camera.deviceId}:aux:light`),
                                                                                    tone: lightActive ? "primary" : undefined,
                                                                                    active: lightActive,
                                                                                },
                                                                        )
                                                                        : nothing}
                                                                ${camera.supportsAux && warningLightAvailable
                                                                        ? this.renderViewportIconButton(
                                                                                warningLightActive ? t("button.warningLightOff") : t("button.warningLightOn"),
                                                                                "mdi:alarm-light-outline",
                                                                                () => this.triggerAuxAction(camera, "warning_light"),
                                                                                {
                                                                                    disabled: this.isBusy(`${camera.deviceId}:aux:warning_light`),
                                                                                    tone: warningLightActive ? "warning" : undefined,
                                                                                    active: warningLightActive,
                                                                                },
                                                                        )
                                                                        : nothing}
                                                                ${camera.supportsAux && sirenAvailable
                                                                        ? this.renderViewportIconButton(
                                                                                sirenActive ? t("button.sirenOff") : t("button.sirenOn"),
                                                                                "mdi:bullhorn",
                                                                                () => this.triggerAuxAction(camera, "siren"),
                                                                                {
                                                                                    tone: "warning",
                                                                                    disabled: this.isBusy(`${camera.deviceId}:aux:siren`),
                                                                                    active: sirenActive,
                                                                                },
                                                                        )
                                                                        : nothing}
                                                                ${cameraAudioAvailable
                                                                        ? this.renderSelectedCameraAudioControl(camera, t)
                                                                        : nothing}
                                                                ${camera.supportsPtz
                                                                        ? this.renderViewportIconButton(
                                                                                this._ptzAdjusting ? t("button.closePtz") : t("button.ptzControls"),
                                                                                "mdi:axis-arrow",
                                                                                () => {
                                                                                    const previousPtzAdjusting = this._ptzAdjusting;
                                                                                    this._ptzAdjusting = !this._ptzAdjusting;
                                                                                    this.requestUpdate("_ptzAdjusting", previousPtzAdjusting);
                                                                                },
                                                                                {
                                                                                    tone: this._ptzAdjusting ? "primary" : "neutral",
                                                                                    active: this._ptzAdjusting,
                                                                                },
                                                                        )
                                                                        : nothing}
                                                            `}
                                </div>
                            </div>
                            ${renderArchiveSeekPanelView({
                                t,
                                camera,
                                archiveDate: this._archiveDate,
                                archiveSeekSecond: this._archiveSeekSecond,
                                callbacks: {
                                    onSelectArchiveDate: (value) => this.selectArchiveDate(value),
                                    onInputArchiveSecond: (second) => {
                                        this._archiveSeekSecond = second;
                                    },
                                    onStartPlayback: (targetCamera, seekTime) => {
                                        void this.startNativeArchivePlayback(targetCamera, seekTime);
                                    },
                                },
                            })}
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
            const effectiveVtoStreamSource = resolveStreamViewportSource(
                vto.stream,
                this._selectedVtoStreamSource,
                this._selectedVtoStreamProfile,
                Boolean(vto.cameraEntity),
                vto.stream.fallbacksEnabled,
            );

            return html`
                <section class="main">
                    <div class="detail-shell">
                        <div class="detail-header">
                            <div class="detail-title">
                                ${vto.label} - ${vto.roomLabel} - ${vto.online ? t("state.online") : t("state.offline")} -
                                ${vto.callStateText}
                            </div>
                            <div class="split-row">
                <span class="badge ${vtoBadgeClass(vto)}">
                  ${vto.callStateText}
                                </span>
                                <span class="badge ${this.selectedVtoMicrophoneBadgeTone()}">
                  ${localizeIntercomStatus(this._selectedVtoMicrophoneState, t)}
                </span>
                                ${vto.doorbell
                                        ? html`<span class="badge warning">${t("badge.doorbell")}</span>`
                                        : nothing}
                                ${vto.tamper
                                        ? html`<span class="badge critical">${t("badge.tamper")}</span>`
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
                                                                                        vto.stream.fallbacksEnabled,
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
                                                    ? html`
                                                        <div class="detail-media-separator" aria-hidden="true"></div>`
                                                    : nothing}
                                            ${availableVtoStreamSources.length > 0
                                                    ? html`
                                                        <div class="detail-media-group chip-row">
                                                            ${availableVtoStreamSources.map((source) =>
                                                                    renderSegmentPrimitive(
                                                                            source,
                                                                            this.streamSourceLabel(source, t),
                                                                            effectiveVtoStreamSource ?? "",
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
                                        this.hass,
                                        vto,
                                        this._selectedVtoStreamPlaying,
                                        this._selectedVtoStreamProfile,
                                        effectiveVtoStreamSource,
                                        t,
                                        vto.stream.fallbacksEnabled,
                                )}
                                <div class="viewport-controls">
                                    ${this.hasVtoSnapshot(vto)
                                            ? this.renderViewportIconButton(
                                                    t("button.snapshot"),
                                                    "mdi:camera",
                                                    () => this.openVtoSnapshot(vto),
                                            )
                                            : nothing}
                                    ${vto.recordingStartUrl || vto.recordingStopUrl
                                            ? this.renderViewportIconButton(
                                                    vto.bridgeRecordingActive ? t("button.stopMp4") : t("button.startMp4"),
                                                    vto.bridgeRecordingActive ? "mdi:record-rec" : "mdi:record-circle-outline",
                                                    () => void this.triggerVtoBridgeRecording(vto),
                                                    {
                                                        tone: vto.bridgeRecordingActive ? "danger" : "warning",
                                                        disabled: this.isBusy("vto:bridge_recording"),
                                                        active: vto.bridgeRecordingActive,
                                                    },
                                            )
                                            : nothing}
                                    ${this.hasPlayableVtoStream(vto)
                                            ? this.renderViewportIconButton(
                                                    this._selectedVtoStreamPlaying ? t("button.stopStream") : t("button.playStream"),
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
                                                        active: this._selectedVtoStreamPlaying,
                                                    },
                                            )
                                            : nothing}
                                    ${vto.capabilities.browserMicrophoneSupported && this.hasAvailableVtoIntercom(vto)
                                            ? this.renderViewportIconButton(
                                                    this._selectedVtoMicrophoneState.enabled ? t("button.disableMic") : t("button.enableMic"),
                                                    this._selectedVtoMicrophoneState.enabled ? "mdi:microphone-off" : "mdi:microphone",
                                                    () => {
                                                        void (this._selectedVtoMicrophoneState.enabled
                                                                ? this.stopSelectedVtoMicrophone()
                                                                : this.startSelectedVtoMicrophone(vto));
                                                    },
                                                    {
                                                        tone: this._selectedVtoMicrophoneState.enabled ? "warning" : "neutral",
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
                                                            this.renderViewportIconButton(
                                                                    vto.locks.length === 1
                                                                            ? t("button.unlock")
                                                                            : t("button.unlockTarget", {target: lock.label}),
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
                                                                    },
                                                            ),
                                            )
                                            : vto.hasUnlockButtonEntity || vto.unlockActionUrl
                                                    ? this.renderViewportIconButton(
                                                            t("button.unlock"),
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
                                                            },
                                                    )
                                                    : nothing}
                                    ${vto.callState === "ringing" && (vto.hasAnswerButtonEntity || vto.answerActionUrl)
                                            ? this.renderViewportIconButton(
                                                    t("button.answerCall"),
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
                                                    },
                                            )
                                            : nothing}
                                    ${(vto.callState === "ringing" || vto.callState === "active") &&
                                    (vto.hasHangupButtonEntity || vto.hangupActionUrl)
                                            ? this.renderViewportIconButton(
                                                    t("button.hangUp"),
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
                            <div class="panel-title">${t("panel.noDevicesTitle")}</div>
                            <div class="muted">
                                ${t("panel.noDevicesBody")}
                            </div>
                            <div class="muted">
                                ${t("panel.noDevicesHint")}
                            </div>
                        </div>
                    </div>
                </section>
            `;
        }

        const layout = overviewLayoutForCount(overviewTiles.length);

        return renderSurveillancePanelOverview({
            t,
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
            onToggleCameraAudio: (camera) => this.toggleOverviewCameraAudio(camera),
            onVtoUnlock: (vto) =>
                this.triggerVtoButtonAction("vto:unlock", vto.unlockButtonEntityId, vto.unlockActionUrl),
            onVtoAnswer: (vto) =>
                this.triggerVtoButtonAction("vto:answer", vto.answerButtonEntityId, vto.answerActionUrl),
            onVtoHangup: (vto) =>
                this.triggerVtoButtonAction("vto:hangup", vto.hangupButtonEntityId, vto.hangupActionUrl),
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
                return renderSelectedCameraViewport(
                    this.hass,
                    camera,
                    profileKey,
                    source,
                    this.isOverviewCameraMuted(camera),
                    DEFAULT_STREAM_VOLUME,
                    {
                        controls: false,
                        preload: "none",
                        fallbackOrder: ["hls", "dash"],
                        includeSubstreamFallback: false,
                        manageAudioExternally: true,
                        t,
                    },
                );
            },
            isCameraMuted: (camera) => this.isOverviewCameraMuted(camera),
            cameraImageSrc: (cameraEntity, snapshotUrl) =>
                cameraImageSrc(cameraEntity, snapshotUrl),
            renderVtoViewport: (vto, playing) =>
                renderSelectedVtoViewport(
                    this.hass,
                    vto,
                    playing,
                    this._selectedVtoStreamProfile,
                    this._selectedVtoStreamSource,
                    t,
                    vto.stream.fallbacksEnabled,
                ),
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
        const t = createLocalizer(model.language);

        return renderSurveillancePanelInspector({
            model,
            t,
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
                ? this.renderBridgeMp4Tab(model.selectedCamera, t)
                : nothing,
            renderIcon: (icon) => this.renderIcon(icon),
            isBusy: (key) => this.isBusy(key),
            onSelectDetailTab: (tab) => {
                const previousDetailTab = this._detailTab;
                this._detailTab = tab;
                this.requestUpdate("_detailTab", previousDetailTab);
            },
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
        const t = createLocalizer(model.language);

        const archiveRecordings = this._smdIvsRecordings;
        const archiveItems = archiveRecordings?.items ?? [];
        const pageCount = pageCountForItems(archiveItems.length, ARCHIVE_PAGE_SIZE);
        const page = boundedPageIndex(this._archivePage, pageCount);

        return renderArchiveRecordings({
            t,
            archiveRecordings,
            archiveLoading: this._smdIvsLoading,
            archiveError: this._smdIvsError,
            archiveDate: this._archiveDate,
            archiveEventCode: this._archiveEventCodeFilter,
            archiveEventTypeOptions: localizedArchiveEventTypeOptions(t),
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
                void this.launchArchivePlayback(model, recording);
            },
            onDownloadRecording: (recording, format) => {
                void this.downloadArchiveRecording(recording, format);
            },
            renderIcon: (icon) => this.renderIcon(icon),
        });
    }

    private renderArchiveTab(model: PanelModel): TemplateResult | typeof nothing {
        const t = createLocalizer(model.language);
        const archiveSource = this.resolveArchiveSource(model);
        if (!archiveSource && !model.selectedNvr) {
            return nothing;
        }

        const archiveRecordings = this._chunkRecordings;
        const archiveItems = archiveRecordings?.items ?? [];
        const pageCount = pageCountForItems(archiveItems.length, ARCHIVE_PAGE_SIZE);
        const page = boundedPageIndex(this._archivePage, pageCount);

        return renderArchiveRecordings({
            t,
            archiveRecordings,
            archiveLoading: this._chunkLoading,
            archiveError: this._chunkError,
            archiveDate: this._archiveDate,
            archiveEventCode: EVENT_FILTER_ALL,
            archiveEventTypeOptions: localizedArchiveEventTypeOptions(t),
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
                void this.launchArchivePlayback(model, recording);
            },
            onDownloadRecording: (recording, format) => {
                void this.downloadArchiveRecording(recording, format);
            },
            renderIcon: (icon) => this.renderIcon(icon),
        });
    }

    private renderBridgeMp4Tab(camera: CameraViewModel, t: Localizer): TemplateResult {
        const recordings = this._bridgeRecordings?.items ?? [];
        const pageCount = pageCountForItems(recordings.length, MP4_PAGE_SIZE);
        const page = boundedPageIndex(this._mp4Page, pageCount);

        return renderBridgeRecordings({
            t,
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
                this.isBusy(bridgeRecordingDownloadActionKey(recording)),
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
        const t = createLocalizer(model.language);
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
            t,
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

    private renderPtzOverlay(camera: CameraViewModel, t: Localizer): TemplateResult {
        return html`
            <div class="ptz-overlay">
                <div class="ptz-card">
                    <div class="panel-title">
                        <span>${t("button.ptzControls")}</span>
                        <span class="badge info">${t("ptz.adjusting")}</span>
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
                                t("button.zoomIn"),
                                "mdi:magnify-plus-outline",
                                () => this.triggerPtzAction(camera, "zoom_in"),
                                {disabled: !camera.supportsPtzZoom},
                        )}
                        ${this.renderControlButton(
                                t("button.zoomOut"),
                                "mdi:magnify-minus-outline",
                                () => this.triggerPtzAction(camera, "zoom_out"),
                                {disabled: !camera.supportsPtzZoom},
                        )}
                        ${this.renderControlButton(
                                t("button.focusFar"),
                                "mdi:crosshairs-plus",
                                () => this.triggerPtzAction(camera, "focus_far"),
                                {disabled: !camera.supportsPtzFocus},
                        )}
                        ${this.renderControlButton(
                                t("button.focusNear"),
                                "mdi:crosshairs",
                                () => this.triggerPtzAction(camera, "focus_near"),
                                {disabled: !camera.supportsPtzFocus},
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
            {disabled},
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

    private renderViewportIconButton(
        label: string,
        icon: string,
        onClick: () => void,
        options: {
            disabled?: boolean;
            tone?: ControlTone;
            active?: boolean;
        } = {},
    ): TemplateResult {
        return renderIconPrimitive(
            label,
            icon,
            onClick,
            (nextIcon) => this.renderIcon(nextIcon),
            options,
        );
    }

    private renderSelectedNativePlaybackViewport(
        camera: CameraViewModel,
        playback: SelectedNativePlaybackState,
        t: Localizer,
    ): TemplateResult {
        const streamSource = playback.streamSource.trim();
        if (isRtspPlaybackStreamSource(streamSource)) {
            return renderNativePlaybackViewport(
                this.hass,
                camera.cameraEntity,
                streamSource,
                displayCameraLabel(camera),
                this._selectedCameraAudioMuted,
                this._selectedCameraVolume,
                t,
                camera.stream.fallbacksEnabled
                    ? playback.fallbackStreamSource ?? null
                    : null,
            );
        }
        return renderTimeframePlaybackViewport(
            streamSource,
            displayCameraLabel(camera),
            this._selectedCameraAudioMuted,
            this._selectedCameraVolume,
            t,
        );
    }

    private renderSelectedCameraAudioControl(camera: CameraViewModel, t: Localizer): TemplateResult {
        const volume = clampStreamVolume(this._selectedCameraVolume);
        const muted = this._selectedCameraAudioMuted || volume <= 0;
        const title = muted ? t("button.enableStreamAudio") : t("button.disableStreamAudio");
        const stopControlEvent = (event: Event): void => {
            event.stopPropagation();
        };
        const handleVolumeInput = (event: Event): void => {
            event.preventDefault();
            event.stopPropagation();
            this.setSelectedCameraVolume(
                streamVolumeFromInputValue((event.currentTarget as HTMLInputElement).value),
            );
        };

        return html`
            <div class="stream-volume-control" @click=${stopControlEvent}>
                ${this.renderViewportIconButton(
                        title,
                        streamVolumeIcon(muted, volume),
                        () => void this.toggleSelectedCameraAudio(camera),
                        {
                            tone: muted ? "neutral" : "primary",
                            active: !muted,
                        },
                )}
                <div class="volume-popover" @click=${stopControlEvent} @pointerdown=${stopControlEvent}>
                    <input
                            class="volume-slider"
                            type="range"
                            min="0"
                            max="100"
                            step="1"
                            .value=${String(streamVolumePercent(volume))}
                            title=${t("button.streamVolume")}
                            aria-label=${t("button.streamVolume")}
                            @input=${handleVolumeInput}
                            @change=${handleVolumeInput}
                    />
                </div>
            </div>
        `;
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

    private scheduleViewportAudioSync(): void {
        if (this._viewportAudioSyncTimer !== null) {
            window.clearTimeout(this._viewportAudioSyncTimer);
        }
        const syncDelays = [0, 50, 150, 400, 1000, 2500, 5000, 10000, 20000, 45000, 90000];
        const runSyncAt = (index: number): void => {
            this.syncSelectedCameraViewportAudioState(
                    this._selectedCameraAudioMuted,
                    this._selectedCameraVolume,
            );
            this.syncOverviewCameraViewportAudioState();
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
            "_selection",
            "_selectedCameraStreamProfile",
            "_selectedCameraStreamSource",
            "_selectedPlaybackStreamProfile",
            "_selectedPlaybackStreamSource",
            "_selectedPlayback",
            "_selectedBridgeRecordingPlayback",
            "_selectedNativePlayback",
            "_selectedCameraAudioMuted",
            "_selectedCameraVolume",
            "_overviewCameraAudioMuted",
            "_selectedVtoStreamProfile",
            "_selectedVtoStreamSource",
            "_selectedVtoStreamPlaying",
        ].some((key) => changedProperties.has(key));
    }

    private async openSnapshot(camera: CameraViewModel): Promise<void> {
        const playbackSnapshotPath = this.resolvePlaybackSnapshotProxyPath(camera);
        const imageUrl = playbackSnapshotPath
            ? await signHomeAssistantPath(this.hass, playbackSnapshotPath)
            : this.resolveLiveSnapshotUrl(camera);
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
        return (
            this.resolvePlaybackSnapshotProxyPath(camera).length > 0 ||
            this.resolveLiveSnapshotUrl(camera).length > 0
        );
    }

    private hasVtoSnapshot(vto: VtoViewModel): boolean {
        return this.resolveVtoSnapshotUrl(vto).length > 0;
    }

    private resolvePlaybackSnapshotProxyPath(camera: CameraViewModel): string {
        const entityID = camera.cameraEntity?.entity_id?.trim() ?? "";
        if (!entityID) {
            return "";
        }

        const nativePlayback = selectNativePlaybackForCamera(this._selectedNativePlayback, camera);
        if (nativePlayback) {
            const startTime = new Date(nativePlayback.startTime);
            const endTime = nativePlayback.endTime
                ? new Date(nativePlayback.endTime)
                : archiveDefaultTimeframeEndTime(startTime);
            const seekTime = new Date(nativePlayback.seekTime);
            if (isValidPlaybackSnapshotRange(startTime, endTime, seekTime)) {
                return buildArchiveTimeframeSnapshotProxyPath(
                    entityID,
                    startTime,
                    endTime,
                    seekTime,
                    nativePlayback.profileKey,
                );
            }
        }

        const bridgePlayback = selectBridgeRecordingPlaybackForCamera(
            this._selectedBridgeRecordingPlayback,
            camera,
        );
        const recording = bridgePlayback?.recording ?? null;
        if (!recording) {
            return "";
        }
        const startTime = new Date(recording.sourceStartTime ?? recording.startedAt);
        const endTime = recording.sourceEndTime || recording.endedAt
            ? new Date(recording.sourceEndTime ?? recording.endedAt ?? "")
            : archiveDefaultTimeframeEndTime(startTime);
        if (!isValidPlaybackSnapshotRange(startTime, endTime, startTime)) {
            return "";
        }
        return buildArchiveTimeframeSnapshotProxyPath(
            entityID,
            startTime,
            endTime,
            startTime,
            recording.profile ?? null,
        );
    }

    private resolveLiveSnapshotUrl(camera: CameraViewModel): string {
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
            this._errorMessage = this.t()("error.bridgeMp4VtoUnavailable");
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
                error instanceof Error ? error.message : this.t()("error.vtoBridgeRecordingFailed");
        } finally {
            const reducedBusy = new Set(this._busyActions);
            reducedBusy.delete(busyKey);
            this._busyActions = reducedBusy;
        }
    }

    private resetSharedSelectionViewState(options: { preserveArchive?: boolean } = {}): void {
        this.resetEventFilters();
        if (!options.preserveArchive) {
            this.resetArchiveEventFilter();
            this._archivePage = 0;
            this.clearArchiveListState();
        }
        this._mp4Page = 0;
        this._selectedCameraAudioMuted = true;
        this._selectedPlaybackStreamProfile = null;
        this._selectedPlaybackStreamSource = null;
        this._selectedPlayback = null;
        this._selectedBridgeRecordingPlayback = null;
        void this.clearCameraNativePlaybackSource(this._selectedNativePlayback?.cameraEntityId ?? null);
        this._selectedNativePlayback = null;
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
            .find((channel) => channel.archive?.chunksUrl && channel.archive.channel !== null)
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
        this._archiveSeekSecond = Math.min(
            this._archiveSeekSecond,
            archiveMaxSecondForDate(nextDate),
        );
        this._archivePage = 0;
        this._mp4Page = 0;
        this.clearArchiveListState();
        this.requestUpdate("_archiveDate", previousDate);
    }

    private selectArchivePage(page: number): void {
        const archiveRecordings =
            this._detailTab === "events" ? this._smdIvsRecordings : this._chunkRecordings;
        const pageCount = pageCountForItems(archiveRecordings?.items.length ?? 0, ARCHIVE_PAGE_SIZE);
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

    private shouldRefreshArchiveRecordings(
        changedProperties: Map<PropertyKey, unknown>,
    ): boolean {
        const shouldRefresh =
            changedProperties.has("_selection") ||
            changedProperties.has("_config") ||
            changedProperties.has("_detailTab") ||
            changedProperties.has("_archiveDate") ||
            changedProperties.has("_archiveEventCodeFilter") ||
            changedProperties.has("_nvrArchiveChannelNumber");
        if (!shouldRefresh) {
            return false;
        }
        if (this._suppressNextArchiveRefresh) {
            this._suppressNextArchiveRefresh = false;
            return false;
        }
        return true;
    }

    private suppressNextArchiveRefresh(): void {
        this._suppressNextArchiveRefresh = true;
        window.setTimeout(() => {
            this._suppressNextArchiveRefresh = false;
        }, 0);
    }

    private clearArchiveListState(): void {
        const hadArchiveState =
            this._smdIvsRecordings !== null ||
            this._smdIvsLoading ||
            this._smdIvsError !== "" ||
            this._chunkRecordings !== null ||
            this._chunkLoading ||
            this._chunkError !== "" ||
            this._archiveRecordingsMode !== null;
        this._smdIvsRecordings = null;
        this._smdIvsLoading = false;
        this._smdIvsError = "";
        this._chunkRecordings = null;
        this._chunkLoading = false;
        this._chunkError = "";
        this._archiveRecordingsMode = null;
        if (hadArchiveState) {
            this.requestUpdate();
        }
    }

    private setArchiveListState(
        mode: ArchiveRecordingsMode,
        state: {
            recordings?: NvrArchiveSearchResultModel | null;
            loading?: boolean;
            error?: string;
        },
    ): void {
        let changed = false;
        if (mode === "events") {
            if ("recordings" in state) {
                const nextRecordings = state.recordings ?? null;
                changed ||= this._smdIvsRecordings !== nextRecordings;
                this._smdIvsRecordings = nextRecordings;
            }
            if (typeof state.loading === "boolean") {
                changed ||= this._smdIvsLoading !== state.loading;
                this._smdIvsLoading = state.loading;
            }
            if (typeof state.error === "string") {
                changed ||= this._smdIvsError !== state.error;
                this._smdIvsError = state.error;
            }
            if (changed) {
                this.requestUpdate();
            }
            return;
        }

        if ("recordings" in state) {
            const nextRecordings = state.recordings ?? null;
            changed ||= this._chunkRecordings !== nextRecordings;
            this._chunkRecordings = nextRecordings;
        }
        if (typeof state.loading === "boolean") {
            changed ||= this._chunkLoading !== state.loading;
            this._chunkLoading = state.loading;
        }
        if (typeof state.error === "string") {
            changed ||= this._chunkError !== state.error;
            this._chunkError = state.error;
        }
        if (changed) {
            this.requestUpdate();
        }
    }

    private async refreshArchiveRecordings(): Promise<void> {
        if (!this.hass || !this._config) {
            this.cancelArchiveRefresh();
            this.clearArchiveListState();
            return;
        }

        if (this._selection.kind !== "camera" && this._selection.kind !== "nvr") {
            this.cancelArchiveRefresh();
            this.clearArchiveListState();
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
        const archiveMode = archiveModeForSelection(this._selection.kind, this._detailTab);
        const eventOnly = archiveMode === "events";
        const archiveSearchUrl = archiveUrlForMode(archiveSource, archiveMode);
        if (!archiveSource?.archive || !archiveSearchUrl || archiveSource.archive.channel === null) {
            this.cancelArchiveRefresh();
            this._archiveRecordingsMode = archiveMode;
            this.setArchiveListState(archiveMode, {
                recordings: null,
                loading: false,
                error: archiveMissingUrlMessage(archiveMode, this.t()),
            });
            return;
        }

        this.cancelArchiveRefresh();
        const controller = new AbortController();
        this._archiveAbort = controller;
        const requestVersion = ++this._archiveRequestVersion;
        this._archiveRecordingsMode = archiveMode;
        this.setArchiveListState(archiveMode, {
            recordings: null,
            loading: true,
            error: "",
        });
        const {startTime, endTime} = dateRangeForArchiveDay(this._archiveDate);
        const eventCode =
            this._selection.kind === "camera" && this._detailTab === "events"
                ? this._archiveEventCodeFilter
                : EVENT_FILTER_ALL;

        try {
            this.logMedia("card panel archive recordings refresh started", {
                device_id: archiveSource.deviceId,
                channel: archiveSource.archive.channel,
                start_time: startTime,
                end_time: endTime,
                event_code: eventCode,
                event_only: eventOnly,
                archive_mode: archiveMode,
                url: redactUrlForLog(archiveSearchUrl),
            });
            const recordings = await fetchArchiveRecordings(
                archiveSearchUrl,
                {
                    channel: archiveSource.archive.channel,
                    startTime,
                    endTime,
                    limit: archiveSource.archive.defaultLimit,
                    eventCode,
                    eventOnly,
                    dbOnly: eventOnly,
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

            this.setArchiveListState(archiveMode, {
                recordings,
                error: "",
            });
            this.logMedia("card panel archive recordings refresh completed", {
                device_id: archiveSource.deviceId,
                channel: archiveSource.archive.channel,
                count: recordings.items.length,
                archive_mode: archiveMode,
            });
        } catch (error) {
            if (
                controller.signal.aborted ||
                this._archiveAbort !== controller ||
                requestVersion !== this._archiveRequestVersion
            ) {
                return;
            }
            const archiveError =
                error instanceof Error ? error.message : this.t()("error.archiveRequestFailed");
            this.setArchiveListState(archiveMode, {
                recordings: null,
                error: archiveError,
            });
            this.logMedia("card panel archive recordings refresh failed", {
                device_id: archiveSource.deviceId,
                channel: archiveSource.archive.channel,
                error: archiveError,
            });
        } finally {
            if (
                this._archiveAbort === controller &&
                requestVersion === this._archiveRequestVersion
            ) {
                this._archiveAbort = undefined;
                this._archiveRecordingsMode = null;
                this.setArchiveListState(archiveMode, {loading: false});
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
        this._bridgeRecordings = null;
        this._bridgeRecordingsLoading = true;
        this._bridgeRecordingsError = "";
        const {startTime, endTime} = dateRangeForArchiveDay(this._archiveDate);

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
            const filteredItems = recordings.items.filter(
                (item) => !item.streamId.trim().toLowerCase().startsWith("nvr_export_"),
            );
            if (
                controller.signal.aborted ||
                this._mp4Abort !== controller ||
                requestVersion !== this._mp4RequestVersion
            ) {
                return;
            }
            this._bridgeRecordings = {
                ...recordings,
                returnedCount: filteredItems.length,
                items: filteredItems,
            };
            this._bridgeRecordingsError = "";
            this.logMedia("card panel bridge recordings refresh completed", {
                device_id: camera.deviceId,
                count: filteredItems.length,
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
                error instanceof Error ? error.message : this.t()("error.bridgeMp4RequestFailed");
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

        if (changedProperties.has("_config")) {
            return true;
        }
        if (changedProperties.has("hass") && !this._todayEventSummary) {
            return true;
        }
        if (!this._todayEventSummary) {
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
            this._todayEventSummaryRefreshedAt = Date.now();
            return;
        }

        this.cancelTodayEventSummaryRefresh();
        const controller = new AbortController();
        this._todayEventSummaryAbort = controller;
        const requestVersion = ++this._todayEventSummaryRequestVersion;
        const endTime = new Date();
        const startTime = new Date(endTime.getTime() - (24 * 60 * 60 * 1000));

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
                                startTime: startTime.toISOString(),
                                endTime: endTime.toISOString(),
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
            this._todayEventSummary = summarizePanelTodayEvents(
                startTime.toISOString(),
                endTime.toISOString(),
                summaries,
            );
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
                this._todayEventSummaryRefreshTimer = window.setTimeout(() => {
                    this._todayEventSummaryRefreshTimer = null;
                    void this.refreshTodayEventSummary();
                }, 60_000);
            }
        }
    }

    private cancelTodayEventSummaryRefresh(): void {
        this._todayEventSummaryAbort?.abort();
        this._todayEventSummaryAbort = undefined;
        if (this._todayEventSummaryRefreshTimer !== null) {
            window.clearTimeout(this._todayEventSummaryRefreshTimer);
            this._todayEventSummaryRefreshTimer = null;
        }
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

    private selectedPlaybackForCamera(camera: CameraViewModel): SelectedPlaybackState | null {
        if (!this._selectedPlayback) {
            return null;
        }
        return this._selectedPlayback.sourceDeviceId === camera.deviceId ? this._selectedPlayback : null;
    }

    private availableSelectedPlaybackViewportSources(
        playback: SelectedPlaybackState,
        camera: CameraViewModel,
        selectedProfileKey: string | null,
    ): CameraViewportSource[] {
        const bridgeSources = availablePlaybackViewportSources(playback.session, selectedProfileKey);
        return this.canUseSelectedPlaybackNativeSource(playback, camera)
            ? ["native", ...bridgeSources]
            : bridgeSources;
    }

    private resolveSelectedPlaybackViewportSource(
        playback: SelectedPlaybackState,
        camera: CameraViewModel,
        selectedSource: CameraViewportSource | null,
        selectedProfileKey: string | null,
    ): CameraViewportSource | null {
        const availableSources = this.availableSelectedPlaybackViewportSources(
            playback,
            camera,
            selectedProfileKey,
        );
        if (selectedSource && availableSources.includes(selectedSource)) {
            return selectedSource;
        }
        return availableSources[0] ?? null;
    }

    private resolveInitialSelectedPlaybackViewportSource(
        playback: SelectedPlaybackState,
        camera: CameraViewModel,
        selectedProfileKey: string | null,
        previousSource: CameraViewportSource | null,
    ): CameraViewportSource | null {
        const availableSources = this.availableSelectedPlaybackViewportSources(
            playback,
            camera,
            selectedProfileKey,
        );
        if (availableSources.includes("native")) {
            return "native";
        }
        if (availableSources.includes("hls")) {
            return "hls";
        }
        if (availableSources.includes("dash")) {
            return "dash";
        }
        if (previousSource && availableSources.includes(previousSource)) {
            return previousSource;
        }
        return availableSources[0] ?? null;
    }

    private canUseSelectedPlaybackNativeSource(
        playback: SelectedPlaybackState,
        camera: CameraViewModel,
    ): boolean {
        return Boolean(camera.cameraEntity && playback.nativeStreamSource?.trim());
    }

    private playbackBusyKey(recording: {
        id?: string | null;
        channel: number;
        startTime: string;
        endTime: string
    }): string {
        if (recording.id) {
            return `playback:${recording.id}`;
        }
        return `playback:${recording.channel}:${recording.startTime}:${recording.endTime}`;
    }

    private archiveDownloadBusyKey(recording: {
        id?: string | null;
        channel: number;
        startTime: string;
        endTime: string
    }): string {
        if (recording.id) {
            return `archive-download:${recording.id}`;
        }
        return `archive-download:${recording.channel}:${recording.startTime}:${recording.endTime}`;
    }

    private async launchArchivePlayback(
        model: PanelModel,
        recording: NvrArchiveRecordingModel,
    ): Promise<void> {
        const archiveSource = this.resolveArchiveSource(model);
        if (isArchiveEventRecording(recording)) {
            if (archiveSource) {
                this.activateArchiveSourceForPlayback(archiveSource);
                await this.startNativeArchiveEventPlayback(archiveSource, recording);
                return;
            }
            if (recording.assetPlaybackUrl) {
                this.playIndexedArchiveRecording(
                    model,
                    recording,
                    null,
                    null,
                );
                return;
            }
            if (recording.exportUrl) {
                await this.launchArchiveClipPlayback(model, recording);
                return;
            }
        }

        await this.launchArchiveMp4Playback(model, recording);
    }

    private async launchArchiveMp4Playback(
        model: PanelModel,
        recording: NvrArchiveRecordingModel,
    ): Promise<void> {
        const archiveSource = this.resolveArchiveSource(model);
        if (recording.assetPlaybackUrl) {
            this.playIndexedArchiveRecording(
                model,
                recording,
                archiveSource?.deviceId ?? null,
                archiveSource?.rootDeviceId ?? null,
            );
            return;
        }
        if (recording.exportUrl) {
            await this.launchArchiveClipPlayback(model, recording);
            return;
        }

        this._errorMessage = this.t()("error.smdPlaybackUnavailable");
    }

    private async launchArchiveClipPlayback(
        model: PanelModel,
        recording: NvrArchiveRecordingModel,
    ): Promise<void> {
        if (recording.assetPlaybackUrl) {
            const archiveSource = this.resolveArchiveSource(model);
            this.playIndexedArchiveRecording(model, recording, archiveSource?.deviceId ?? null, archiveSource?.rootDeviceId ?? null);
            return;
        }
        const archiveSource = this.resolveArchiveSource(model);
        const browserBridgeUrl = archiveSource?.bridgeBaseUrl ?? null;
        if (!archiveSource || !recording.exportUrl) {
            this._errorMessage = this.t()("error.playbackSourceUnavailable");
            return;
        }

        const busyKey = this.playbackBusyKey(recording);
        if (this.isBusy(busyKey)) {
            return;
        }

        const nextBusy = new Set(this._busyActions);
        nextBusy.add(busyKey);
        this._busyActions = nextBusy;
        this._errorMessage = "";

        try {
            this.logMedia("card panel archive clip playback export request", {
                ...this.archiveRecordingLogContext(recording),
                device_id: archiveSource.deviceId,
                url: redactUrlForLog(recording.exportUrl),
            });
            const startedClip = await exportArchiveRecording(recording.exportUrl, browserBridgeUrl);
            const completedClip = await waitForArchiveExportCompletion(startedClip, browserBridgeUrl);
            if (!completedClip.playbackUrl) {
                throw new Error(this.t()("error.archiveExportNoPlayback"));
            }
            this.patchArchiveRecordingExportClip(recording, completedClip);
            this._selectedPlayback = null;
            this._selectedPlaybackStreamProfile = null;
            this._selectedPlaybackStreamSource = null;
            void this.clearCameraNativePlaybackSource(this._selectedNativePlayback?.cameraEntityId ?? null);
            this._selectedNativePlayback = null;
            this._selectedBridgeRecordingPlayback = {
                sourceDeviceId: archiveSource.deviceId,
                recording: {
                    id: completedClip.id,
                    streamId: completedClip.id,
                    rootDeviceId: archiveSource.rootDeviceId,
                    sourceDeviceId: archiveSource.deviceId,
                    deviceKind: "nvr_channel",
                    name: displayCameraLabel(archiveSource),
                    channel: recording.channel,
                    profile: "stable",
                    status: completedClip.status,
                    startedAt: recording.startTime,
                    endedAt: null,
                    sourceStartTime: recording.startTime,
                    sourceEndTime: recording.endTime,
                    durationMs: completedClip.durationMs,
                    bytes: null,
                    fileName: null,
                    playbackUrl: completedClip.playbackUrl,
                    downloadUrl: completedClip.downloadUrl,
                    error: completedClip.error,
                },
            };
            this._selectedCameraAudioMuted = true;
            this.logMedia("card panel archive clip playback selected", {
                ...this.archiveRecordingLogContext(recording),
                device_id: archiveSource.deviceId,
                clip_id: completedClip.id,
                playback_url: redactUrlForLog(completedClip.playbackUrl),
            });
        } catch (error) {
            this._errorMessage =
                error instanceof Error ? error.message : this.t()("error.archivePlaybackExportFailed");
            this.logMedia("card panel archive clip playback failed", {
                ...this.archiveRecordingLogContext(recording),
                device_id: archiveSource?.deviceId,
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

        const busyKey = bridgeRecordingDownloadActionKey(recording);
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
        void this.clearCameraNativePlaybackSource(this._selectedNativePlayback?.cameraEntityId ?? null);
        this._selectedNativePlayback = null;
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

    private activateArchiveSourceForPlayback(archiveSource: CameraViewModel): void {
        const previousSelection = this._selection;
        const nextState = selectCameraState(archiveSource);
        const detailTab =
            this._detailTab === "events" || this._detailTab === "recordings"
                ? this._detailTab
                : nextState.detailTab;
        const cameraProfile =
            this._selectedCameraStreamProfile ??
            defaultSelectedStreamProfileKey(archiveSource.stream) ??
            null;
        this.resetSharedSelectionViewState();
        this.clearVtoSelectionState();
        this.applySelectionTransition(
            {
                ...nextState,
                detailTab,
            },
            {
                openInspector: true,
                cameraProfile,
                cameraSource: "native",
            },
        );
        this.requestUpdate("_selection", previousSelection);
    }

    private stopSelectedPlayback(): void {
        if (this._selectedPlayback) {
            this.logMedia("card panel playback session cleared", {
                device_id: this._selectedPlayback.sourceDeviceId,
                session_id: this._selectedPlayback.session.id,
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

    private stopSelectedNativePlayback(): void {
        if (this._selectedNativePlayback) {
            this.logMedia("card panel native playback cleared", {
                device_id: this._selectedNativePlayback.sourceDeviceId,
                seek_time: this._selectedNativePlayback.seekTime,
            });
        }
        const previousPlayback = this._selectedNativePlayback;
        this._selectedNativePlayback = null;
        this.requestUpdate("_selectedNativePlayback", previousPlayback);
        void this.clearCameraNativePlaybackSource(previousPlayback?.cameraEntityId ?? null);
    }

    private async setCameraNativePlaybackSource(
        camera: CameraViewModel,
        streamSource: string,
    ): Promise<boolean> {
        const entityId = camera.cameraEntity?.entity_id?.trim() || camera.cameraEntityId.trim();
        if (!this.hass || !entityId) {
            this._errorMessage = this.t()("error.historicalUrlUnavailable");
            return false;
        }

        try {
            await this.hass.callService(
                "dahuabridge",
                "set_native_playback_source",
                {stream_source: streamSource},
                {entity_id: entityId},
            );
            return true;
        } catch (error) {
            this._errorMessage =
                error instanceof Error ? error.message : this.t()("error.historicalUrlUnavailable");
            this.logMedia("card panel native playback source service failed", {
                device_id: camera.deviceId,
                entity_id: entityId,
                stream_source: redactUrlForLog(streamSource),
                error: error instanceof Error ? error.message : String(error),
            });
            return false;
        }
    }

    private async clearCameraNativePlaybackSource(entityId: string | null): Promise<void> {
        const normalizedEntityId = entityId?.trim() ?? "";
        if (!this.hass || !normalizedEntityId) {
            return;
        }

        try {
            await this.hass.callService(
                "dahuabridge",
                "clear_native_playback_source",
                {},
                {entity_id: normalizedEntityId},
            );
        } catch (error) {
            this.logMedia("card panel native playback source clear failed", {
                entity_id: normalizedEntityId,
                error: error instanceof Error ? error.message : String(error),
            });
        }
    }

    private isPlaybackActive(recording: NvrArchiveRecordingModel): boolean {
        if (
            this._selectedPlayback?.recording &&
            this.isSameArchiveRecording(this._selectedPlayback.recording, recording)
        ) {
            return true;
        }
        if (nativePlaybackMatchesRecording(this._selectedNativePlayback, recording)) {
            return true;
        }
        return Boolean(
            recording.assetClipId &&
            this._selectedBridgeRecordingPlayback?.recording.id === recording.assetClipId,
        );
    }

    private isBridgeRecordingPlaybackActive(recording: BridgeRecordingClipModel): boolean {
        return this._selectedBridgeRecordingPlayback?.recording.id === recording.id;
    }

    private async downloadArchiveRecording(
        recording: NvrArchiveRecordingModel,
        format: "asset" | "raw" = "asset",
    ): Promise<void> {
        if (format === "raw" && recording.downloadUrl) {
            this.logMedia("card panel archive download open", {
                ...this.archiveRecordingLogContext(recording),
                url: redactUrlForLog(recording.downloadUrl),
            });
            openExternalUrl(recording.downloadUrl);
            return;
        }

        if (recording.assetDownloadUrl) {
            this.logMedia("card panel archive asset download open", {
                ...this.archiveRecordingLogContext(recording),
                clip_id: recording.assetClipId,
                url: redactUrlForLog(recording.assetDownloadUrl),
            });
            openExternalUrl(recording.assetDownloadUrl);
            return;
        }

        if (recording.downloadUrl && !recording.exportUrl) {
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
                throw new Error(this.t()("error.archiveExportNoDownload"));
            }
            this.patchArchiveRecordingExportClip(recording, completedClip);
            this.logMedia("card panel archive export completed", {
                ...this.archiveRecordingLogContext(recording),
                clip_id: completedClip.id,
                download_url: redactUrlForLog(completedClip.downloadUrl),
            });
            openExternalUrl(completedClip.downloadUrl);
        } catch (error) {
            this._errorMessage =
                error instanceof Error ? error.message : this.t()("error.archiveExportFailed");
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

    private patchArchiveRecordingExportClip(
        recording: NvrArchiveRecordingModel,
        clip: NvrArchiveExportClipModel,
    ): void {
        this._smdIvsRecordings = this.patchArchiveRecordingList(
            this._smdIvsRecordings,
            recording,
            clip,
        );
        this._chunkRecordings = this.patchArchiveRecordingList(
            this._chunkRecordings,
            recording,
            clip,
        );
    }

    private patchArchiveRecordingList(
        recordings: NvrArchiveSearchResultModel | null,
        recording: NvrArchiveRecordingModel,
        clip: NvrArchiveExportClipModel,
    ): NvrArchiveSearchResultModel | null {
        if (!recordings) {
            return null;
        }

        return {
            ...recordings,
            items: recordings.items.map((item) =>
                this.isSameArchiveRecording(item, recording)
                    ? {
                        ...item,
                        assetClipId: clip.id,
                        assetStatus: clip.status,
                        assetPlaybackUrl: clip.playbackUrl,
                        assetDownloadUrl: clip.downloadUrl,
                    }
                    : item,
            ),
        };
    }

    private playIndexedArchiveRecording(
        model: PanelModel,
        recording: NvrArchiveRecordingModel,
        sourceDeviceId: string | null,
        rootDeviceId: string | null,
    ): void {
        if (!recording.assetPlaybackUrl || !recording.assetClipId) {
            return;
        }
        const archiveSource = this.resolveArchiveSource(model);
        const recordingName = archiveSource
            ? displayCameraLabel(archiveSource)
            : `Archive Channel ${recording.channel}`;
        this._selectedPlayback = null;
        this._selectedPlaybackStreamProfile = null;
        this._selectedPlaybackStreamSource = null;
        void this.clearCameraNativePlaybackSource(this._selectedNativePlayback?.cameraEntityId ?? null);
        this._selectedNativePlayback = null;
        this._selectedBridgeRecordingPlayback = {
            sourceDeviceId: sourceDeviceId ?? "",
            recording: {
                id: recording.assetClipId,
                streamId: recording.assetClipId,
                rootDeviceId,
                sourceDeviceId,
                deviceKind: "nvr_channel",
                name: recordingName,
                channel: recording.channel,
                profile: "stable",
                status: recording.assetStatus ?? "completed",
                startedAt: recording.startTime,
                endedAt: null,
                sourceStartTime: recording.startTime,
                sourceEndTime: recording.endTime,
                durationMs: null,
                bytes: null,
                fileName: null,
                playbackUrl: recording.assetPlaybackUrl ?? null,
                downloadUrl: recording.assetDownloadUrl ?? null,
                error: recording.assetError ?? null,
            },
        };
        this._selectedCameraAudioMuted = true;
        this.logMedia("card panel indexed archive playback selected", {
            ...this.archiveRecordingLogContext(recording),
            clip_id: recording.assetClipId,
            playback_url: redactUrlForLog(recording.assetPlaybackUrl),
        });
    }

    private async startBridgeArchivePlaybackSession(
        camera: CameraViewModel,
        request: NvrPlaybackSessionRequestModel,
        recording: NvrArchiveRecordingModel | null,
        nativeStreamSource: string | null,
        logContext: Record<string, unknown>,
    ): Promise<boolean> {
        const playbackUrl = this.playbackSessionsUrlForCamera(camera);
        if (!playbackUrl) {
            return false;
        }

        try {
            const session = await createPlaybackSession(
                playbackUrl,
                request,
                this.browserBridgeUrlForPlayback(camera, playbackUrl),
            );
            const selectedProfileKey = this.resolvePlaybackSessionProfileKey(session);
            const nextPlayback: SelectedPlaybackState = {
                sourceDeviceId: camera.deviceId,
                recording,
                session,
                nativeStreamSource,
            };
            const selectedSource =
                preservePlaybackViewportSourceSelection(
                    session,
                    selectedProfileKey,
                    this._selectedPlaybackStreamSource,
                ) ??
                this.resolveInitialSelectedPlaybackViewportSource(
                    nextPlayback,
                    camera,
                    selectedProfileKey,
                    this._selectedPlaybackStreamSource,
                );
            if (!selectedSource) {
                this.logMedia("card panel playback session has no playable sources", {
                    ...logContext,
                    device_id: camera.deviceId,
                    session_id: session.id,
                });
                return false;
            }

            this._selectedPlaybackStreamProfile = selectedProfileKey;
            this._selectedPlaybackStreamSource = selectedSource;
            this._selectedPlayback = nextPlayback;
            this._selectedBridgeRecordingPlayback = null;
            void this.clearCameraNativePlaybackSource(this._selectedNativePlayback?.cameraEntityId ?? null);
            this._selectedNativePlayback = null;
            this._selectedCameraAudioMuted = true;
            this._errorMessage = "";
            this.logMedia("card panel playback session selected", {
                ...logContext,
                device_id: camera.deviceId,
                session_id: session.id,
                profile_key: selectedProfileKey,
                source: selectedSource,
                native_stream_source: nativeStreamSource ? redactUrlForLog(nativeStreamSource) : null,
            });
            return true;
        } catch (error) {
            this.logMedia("card panel playback session failed", {
                ...logContext,
                device_id: camera.deviceId,
                playback_url: redactUrlForLog(playbackUrl),
                error: error instanceof Error ? error.message : String(error),
            });
            return false;
        }
    }

    private resolvePlaybackSessionProfileKey(session: NvrPlaybackSessionModel): string | null {
        if (
            this._selectedPlaybackStreamProfile &&
            session.profiles[this._selectedPlaybackStreamProfile]
        ) {
            return this._selectedPlaybackStreamProfile;
        }
        if (session.recommendedProfile && session.profiles[session.recommendedProfile]) {
            return session.recommendedProfile;
        }
        return Object.keys(session.profiles)[0] ?? null;
    }

    private playbackSessionsUrlForCamera(camera: CameraViewModel): string | null {
        const attributeUrl = stringCameraEntityAttribute(camera, "bridge_playback_sessions_url");
        if (attributeUrl) {
            return attributeUrl;
        }
        if (!camera.bridgeBaseUrl || !camera.rootDeviceId.trim()) {
            return null;
        }
        return buildBridgeEndpointUrl(
            camera.bridgeBaseUrl,
            `/api/v1/nvr/${encodeURIComponent(camera.rootDeviceId)}/playback/sessions`,
        );
    }

    private browserBridgeUrlForPlayback(camera: CameraViewModel, playbackUrl: string): string | null {
        return camera.bridgeBaseUrl ?? browserBridgeUrlFromRequestUrl(playbackUrl);
    }

    private buildNativeArchivePlaybackSource(
        camera: CameraViewModel,
        seekTime: Date,
        endTime: Date | null,
        selectedProfileKey: string | null,
        recording?: NvrArchiveRecordingModel,
    ): string | null {
        const profile = resolveMainArchivePlaybackProfile(camera, selectedProfileKey);
        const candidates = [
            rawCameraProfileStreamUrl(camera, profile?.key ?? selectedProfileKey),
            profile?.streamUrl ?? null,
            stringCameraEntityAttribute(camera, "stream_source"),
            camera.stream.source,
            ...archiveRecordingRtspCandidates(recording, profile?.key ?? selectedProfileKey),
            camera.stream.onvifStreamUrl,
        ];
        let firstPlaybackUrl: string | null = null;
        for (const streamUrl of candidates) {
            const playbackUrl = buildRtspPlaybackUrl({
                streamUrl,
                channel: camera.channelNumber,
                subtype: profile?.subtype ?? null,
                seekTime: seekTime.toISOString(),
                endTime: endTime ? endTime.toISOString() : null,
            });
            if (playbackUrl) {
                if (rtspUrlHasCredentials(playbackUrl)) {
                    return playbackUrl;
                }
                firstPlaybackUrl ??= playbackUrl;
            }
        }
        return firstPlaybackUrl;
    }

    private async startNativeArchivePlayback(
        camera: CameraViewModel,
        seekTime: Date,
    ): Promise<boolean> {
        const endTime = archiveDefaultTimeframeEndTime(seekTime);
        const selectedProfile = resolveMainArchivePlaybackProfile(
            camera,
            this._selectedCameraStreamProfile,
        );
        const nativeStreamSource = this.buildNativeArchivePlaybackSource(
            camera,
            seekTime,
            null,
            selectedProfile?.key ?? null,
        );
        const previousNativeEntityId = this._selectedNativePlayback?.cameraEntityId ?? null;
        const nextNativeEntityId =
            camera.cameraEntity?.entity_id?.trim() || camera.cameraEntityId.trim() || null;
        if (!nativeStreamSource) {
            await this.clearCameraNativePlaybackSource(previousNativeEntityId);
            this._errorMessage = this.t()("error.historicalUrlUnavailable");
            this.logMedia("card panel native archive playback unavailable", {
                device_id: camera.deviceId,
                channel: camera.channelNumber,
                seek_time: seekTime.toISOString(),
                profile_key: selectedProfile?.key ?? null,
                rtsp_source: null,
            });
            return false;
        }
        if (!await this.setCameraNativePlaybackSource(camera, nativeStreamSource)) {
            await this.clearCameraNativePlaybackSource(previousNativeEntityId);
            return false;
        }
        if (previousNativeEntityId && previousNativeEntityId !== nextNativeEntityId) {
            await this.clearCameraNativePlaybackSource(previousNativeEntityId);
        }

        this._selectedPlayback = null;
        this._selectedPlaybackStreamProfile = null;
        this._selectedPlaybackStreamSource = null;
        this._selectedBridgeRecordingPlayback = null;
        this._selectedNativePlayback = createSelectedNativePlaybackState(
            camera,
            nativeStreamSource,
            seekTime,
            endTime,
            seekTime,
            selectedProfile?.key ?? null,
        );
        this.suppressNextArchiveRefresh();
        this._archiveDate = toDateInputValue(seekTime);
        this._archiveSeekSecond = secondsSinceLocalMidnight(seekTime);
        this._archivePage = 0;
        this._mp4Page = 0;
        this._selectedCameraAudioMuted = true;
        this._errorMessage = "";
        this.logMedia("card panel native archive playback selected", {
            device_id: camera.deviceId,
            channel: camera.channelNumber,
            seek_time: seekTime.toISOString(),
            stream_source: redactUrlForLog(nativeStreamSource),
            playback_mode: "native_rtsp",
        });
        return true;
    }

    private async startNativeArchiveEventPlayback(
        camera: CameraViewModel,
        recording: NvrArchiveRecordingModel,
    ): Promise<boolean> {
        const startTime = new Date(recording.startTime);
        const endTime = new Date(recording.endTime);
        if (
            Number.isNaN(startTime.getTime()) ||
            Number.isNaN(endTime.getTime()) ||
            endTime <= startTime
        ) {
            this._errorMessage = this.t()("error.historicalEventInvalid");
            return false;
        }

        const selectedProfile = resolveMainArchivePlaybackProfile(
            camera,
            this._selectedCameraStreamProfile,
        );
        const nativeStreamSource = this.buildNativeArchivePlaybackSource(
            camera,
            startTime,
            null,
            selectedProfile?.key ?? null,
            recording,
        );
        const previousNativeEntityId = this._selectedNativePlayback?.cameraEntityId ?? null;
        const nextNativeEntityId =
            camera.cameraEntity?.entity_id?.trim() || camera.cameraEntityId.trim() || null;
        if (!nativeStreamSource) {
            await this.clearCameraNativePlaybackSource(previousNativeEntityId);
            this._errorMessage = this.t()("error.historicalUrlUnavailable");
            this.logMedia("card panel native event playback unavailable", {
                ...this.archiveRecordingLogContext(recording),
                device_id: camera.deviceId,
                profile_key: selectedProfile?.key ?? null,
                rtsp_source: null,
            });
            return false;
        }
        if (!await this.setCameraNativePlaybackSource(camera, nativeStreamSource)) {
            await this.clearCameraNativePlaybackSource(previousNativeEntityId);
            return false;
        }
        if (previousNativeEntityId && previousNativeEntityId !== nextNativeEntityId) {
            await this.clearCameraNativePlaybackSource(previousNativeEntityId);
        }

        this._selectedPlayback = null;
        this._selectedPlaybackStreamProfile = null;
        this._selectedPlaybackStreamSource = null;
        this._selectedBridgeRecordingPlayback = null;
        this._selectedNativePlayback = createSelectedNativePlaybackState(
            camera,
            nativeStreamSource,
            recording.startTime,
            recording.endTime,
            recording.startTime,
            selectedProfile?.key ?? null,
        );
        this.suppressNextArchiveRefresh();
        this._archiveDate = toDateInputValue(startTime);
        this._archiveSeekSecond = secondsSinceLocalMidnight(startTime);
        this._selectedCameraAudioMuted = true;
        this._errorMessage = "";
        this.logMedia("card panel native event playback selected", {
            ...this.archiveRecordingLogContext(recording),
            device_id: camera.deviceId,
            stream_url: redactUrlForLog(nativeStreamSource),
            playback_mode: "native_rtsp",
            profile_key: selectedProfile?.key ?? null,
        });
        return true;
    }

    private archiveRecordingLogContext(recording: NvrArchiveRecordingModel): Record<string, unknown> {
        return {
            recording_id: recording.id,
            record_kind: recording.recordKind ?? null,
            channel: recording.channel,
            start_time: recording.startTime,
            end_time: recording.endTime,
            recording_type: recording.type ?? null,
            asset_status: recording.assetStatus ?? null,
            asset_clip_id: recording.assetClipId ?? null,
        };
    }

    private isSameArchiveRecording(
        left: NvrArchiveRecordingModel,
        right: NvrArchiveRecordingModel,
    ): boolean {
        const leftID = left.id?.trim() ?? "";
        const rightID = right.id?.trim() ?? "";
        if (leftID && rightID) {
            const leftKind = left.recordKind?.trim().toLowerCase() ?? "";
            const rightKind = right.recordKind?.trim().toLowerCase() ?? "";
            return leftID === rightID && leftKind === rightKind;
        }
        return (
            left.channel === right.channel &&
            left.startTime === right.startTime &&
            left.endTime === right.endTime &&
            (left.filePath ?? "") === (right.filePath ?? "") &&
            (left.recordKind ?? "") === (right.recordKind ?? "")
        );
    }

    private logMedia(message: string, details?: Record<string, unknown>): void {
        logCardInfo(message, {
            card: "surveillance-panel",
            selection_kind: this._selection.kind,
            ...details,
        });
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
        if (!nextMuted && this._selectedCameraVolume <= 0) {
            const previousVolume = this._selectedCameraVolume;
            this._selectedCameraVolume = DEFAULT_STREAM_VOLUME;
            this.requestUpdate("_selectedCameraVolume", previousVolume);
        }
        this.syncSelectedCameraViewportAudioState(nextMuted, this._selectedCameraVolume);
        this.logMedia("card panel camera audio toggled", {
            device_id: camera.deviceId,
            muted: nextMuted,
            volume: streamVolumePercent(this._selectedCameraVolume),
        });
    }

    private setSelectedCameraVolume(volume: number): void {
        const nextVolume = clampStreamVolume(volume);
        const previousVolume = this._selectedCameraVolume;
        const nextMuted = nextVolume <= 0 ? true : false;
        const previousMuted = this._selectedCameraAudioMuted;

        this._selectedCameraVolume = nextVolume;
        if (previousVolume !== nextVolume) {
            this.requestUpdate("_selectedCameraVolume", previousVolume);
        }
        this._selectedCameraAudioMuted = nextMuted;
        if (previousMuted !== nextMuted) {
            this.requestUpdate("_selectedCameraAudioMuted", previousMuted);
        }
        this.syncSelectedCameraViewportAudioState(nextMuted, nextVolume);
    }

    private toggleOverviewCameraAudio(camera: CameraViewModel): void {
        const nextMuted = !this.isOverviewCameraMuted(camera);
        const previousMuted = this._overviewCameraAudioMuted;
        this._overviewCameraAudioMuted = {
            ...this._overviewCameraAudioMuted,
            [camera.deviceId]: nextMuted,
        };
        this.requestUpdate("_overviewCameraAudioMuted", previousMuted);
        this.syncOverviewCameraViewportAudioState(camera.deviceId, nextMuted);
        window.requestAnimationFrame(() => {
            this.syncOverviewCameraViewportAudioState(camera.deviceId, nextMuted);
        });
        this.logMedia("card panel overview camera audio toggled", {
            device_id: camera.deviceId,
            muted: nextMuted,
        });
    }

    private isBusy(key: string): boolean {
        return this._actions.isBusy(key);
    }

    private t(): Localizer {
        return createLocalizer(this.hass ? resolvePanelLanguage(this.hass) : "en");
    }

    private renderIcon(icon: string): TemplateResult {
        return html`
            <ha-icon .icon=${icon}></ha-icon>`;
    }

    private streamSourceLabel(source: CameraViewportSource, t: Localizer = createLocalizer("en")): string {
        switch (source) {
            case "native":
                return t("source.nativeHa");
            case "dash":
                return t("source.dash");
            case "hls":
                return t("source.hls");
            case "mjpeg":
                return t("source.mjpeg");
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
            if (selectedSource === "native") {
                return this.canUseSelectedPlaybackNativeSource(selectedPlayback, camera);
            }
            if (selectedSource === "hls" || selectedSource === "dash") {
                return availablePlaybackViewportSources(
                    selectedPlayback.session,
                    selectedProfileKey,
                ).includes(selectedSource);
            }
            return false;
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
        if (!vto.streamAvailable) {
            return false;
        }
        const isSelectedVto =
            this._selection.kind === "vto" && this._selection.deviceId === vto.deviceId;
        const profileKey = isSelectedVto
            ? this._selectedVtoStreamProfile
            : defaultSelectedStreamProfileKey(vto.stream);
        const selectedSource = isSelectedVto ? this._selectedVtoStreamSource : null;
        return (
            resolveStreamViewportSource(
                vto.stream,
                selectedSource,
                profileKey,
                Boolean(vto.cameraEntity),
                vto.stream.fallbacksEnabled,
            ) !== null
        );
    }

    private syncSelectedCameraViewportAudioState(
        muted: boolean,
        volume = this._selectedCameraVolume,
    ): void {
        syncViewportAudioState(
            this.renderRoot.querySelector("section.main .viewport"),
            muted,
            volume,
        );
    }

    private isOverviewCameraMuted(camera: CameraViewModel): boolean {
        return this._overviewCameraAudioMuted[camera.deviceId] ?? true;
    }

    private syncOverviewCameraViewportAudioState(
        deviceID?: string,
        muted?: boolean,
    ): void {
        const tiles = this.renderRoot.querySelectorAll<HTMLElement>("article.camera-tile[data-device-id]");
        for (const tile of tiles) {
            const currentDeviceID = tile.dataset.deviceId ?? "";
            if (deviceID && currentDeviceID !== deviceID) {
                continue;
            }
            syncViewportAudioState(
                tile.querySelector(".tile-media"),
                muted ?? (this._overviewCameraAudioMuted[currentDeviceID] ?? true),
                DEFAULT_STREAM_VOLUME,
            );
        }
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
            this._errorMessage = this.t()("error.intercomOfferUnavailable");
            return;
        }

        this.logMedia("card panel vto microphone enable", {
            device_id: vto.deviceId,
            offer_url: redactUrlForLog(offerUrl),
        });
        await this._intercomSession.enable(offerUrl);
    }

    private async stopSelectedVtoMicrophone(): Promise<void> {
        const snapshot = this._intercomSession.currentSnapshot();
        if (!snapshot.enabled && snapshot.phase === "idle") {
            return;
        }

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
                vto.stream.fallbacksEnabled,
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

function hasCameraEventCounts(camera: CameraViewModel): boolean {
    return (
        camera.humanCount24h > 0 ||
        camera.vehicleCount24h > 0 ||
        camera.ivsCount24h > 0
    );
}

function pageCountForItems(totalItems: number, pageSize: number): number {
    const safeSize = Math.max(1, Math.trunc(pageSize));
    return Math.max(1, Math.ceil(Math.max(0, totalItems) / safeSize));
}

function localizedArchiveEventTypeOptions(t: Localizer): Array<{ value: string; label: string }> {
    return ARCHIVE_EVENT_TYPE_OPTIONS.map((option) => ({
        value: option.value,
        label: t(option.labelKey),
    }));
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

function stringCameraEntityAttribute(camera: CameraViewModel, key: string): string | null {
    const value = camera.cameraEntity?.attributes[key];
    return typeof value === "string" && value.trim() ? value.trim() : null;
}

function rtspUrlHasCredentials(value: string): boolean {
    try {
        const parsed = new URL(value);
        return Boolean(parsed.username || parsed.password);
    } catch {
        return false;
    }
}

function isRtspPlaybackStreamSource(value: string): boolean {
    try {
        return new URL(value).protocol.toLowerCase() === "rtsp:";
    } catch {
        return false;
    }
}

function archiveRecordingRtspCandidates(
    recording: NvrArchiveRecordingModel | undefined,
    profileKey: string | null | undefined,
): string[] {
    if (!recording) {
        return [];
    }
    const ordered =
        profileKey?.trim() === "stable"
            ? [recording.rtspSubUrl, recording.rtspMainUrl]
            : [recording.rtspMainUrl, recording.rtspSubUrl];
    const seen = new Set<string>();
    return ordered.flatMap((candidate) => {
        const normalized = candidate?.trim() ?? "";
        if (!normalized || seen.has(normalized)) {
            return [];
        }
        seen.add(normalized);
        return [normalized];
    });
}

function rawCameraProfileStreamUrl(
    camera: CameraViewModel,
    profileKey: string | null | undefined,
): string | null {
    const normalizedProfileKey = profileKey?.trim() ?? "";
    if (!normalizedProfileKey) {
        return null;
    }
    const profiles = camera.cameraEntity?.attributes.bridge_profiles;
    if (!isRecord(profiles)) {
        return null;
    }
    const profile = profiles[normalizedProfileKey];
    if (!isRecord(profile)) {
        return null;
    }
    return stringRecordValue(profile, "stream_url") ?? stringRecordValue(profile, "streamUrl");
}

function stringRecordValue(record: Record<string, unknown>, key: string): string | null {
    const value = record[key];
    return typeof value === "string" && value.trim() ? value.trim() : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

function browserBridgeUrlFromRequestUrl(value: string): string | null {
    try {
        const origin =
            typeof window !== "undefined" && window.location?.origin
                ? window.location.origin
                : "http://localhost";
        const url = new URL(value, origin);
        const apiPathIndex = url.pathname.indexOf("/api/");
        const bridgePath = apiPathIndex > 0 ? url.pathname.slice(0, apiPathIndex) : "";
        return `${url.protocol}//${url.host}${bridgePath.replace(/\/+$/, "")}`;
    } catch {
        return null;
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
