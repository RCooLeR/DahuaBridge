import { html, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import type {
  BridgeRecordingClipModel,
  BridgeRecordingClipListModel,
  NvrArchiveRecordingModel,
  NvrArchiveSearchResultModel,
} from "../domain/archive";
import type { CameraViewModel, PanelModel } from "../domain/model";
import { renderControlButton } from "./surveillance-panel-primitives";
import { EVENT_FILTER_ALL, type EventFilterOption } from "./surveillance-panel-state";

interface RenderArchiveRecordingsArgs {
  title: string;
  archiveRecordings: NvrArchiveSearchResultModel | null;
  archiveLoading: boolean;
  archiveError: string;
  archiveDate: string;
  archiveEventCode: string;
  archiveEventTypeOptions: readonly EventFilterOption[];
  playbackSupported: boolean;
  showEventFilter: boolean;
  page: number;
  pageCount: number;
  visibleItems: readonly NvrArchiveRecordingModel[];
  isLaunchingPlayback: (recording: NvrArchiveRecordingModel) => boolean;
  isPlaybackActive: (recording: NvrArchiveRecordingModel) => boolean;
  isDownloadingRecording: (recording: NvrArchiveRecordingModel) => boolean;
  onSelectArchiveDate: (value: string) => void;
  onSelectArchiveEventType: (eventCode: string) => void;
  onSelectArchivePage: (page: number) => void;
  onLaunchPlayback: (recording: NvrArchiveRecordingModel) => void;
  onDownloadRecording: (recording: NvrArchiveRecordingModel, format?: "asset" | "raw") => void;
  renderIcon: (icon: string) => TemplateResult;
}

interface RenderBridgeRecordingsArgs {
  title: string;
  recordings: BridgeRecordingClipListModel | null;
  recordingsLoading: boolean;
  recordingsError: string;
  recordingsDate: string;
  page: number;
  pageCount: number;
  visibleItems: readonly BridgeRecordingClipModel[];
  playbackSupported: boolean;
  isPlaybackActive: (recording: BridgeRecordingClipModel) => boolean;
  isDownloadingRecording: (recording: BridgeRecordingClipModel) => boolean;
  onSelectDate: (value: string) => void;
  onSelectPage: (page: number) => void;
  onPlayRecording: (recording: BridgeRecordingClipModel) => void;
  onDownloadRecording: (recording: BridgeRecordingClipModel) => void;
  renderIcon: (icon: string) => TemplateResult;
}

const archiveDateTimeFormatter = new Intl.DateTimeFormat(undefined, {
  year: "numeric",
  month: "short",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
});

export function renderArchiveRecordings({
  title,
  archiveRecordings,
  archiveLoading,
  archiveError,
  archiveDate,
  archiveEventCode,
  archiveEventTypeOptions,
  playbackSupported,
  showEventFilter,
  page,
  pageCount,
  visibleItems,
  isLaunchingPlayback,
  isPlaybackActive,
  isDownloadingRecording,
  onSelectArchiveDate,
  onSelectArchiveEventType,
  onSelectArchivePage,
  onLaunchPlayback,
  onDownloadRecording,
  renderIcon,
}: RenderArchiveRecordingsArgs): TemplateResult {
  const statusTone = archiveError ? "warning" : archiveLoading ? "info" : "success";
  const countText = archiveLoading
    ? "Loading"
    : archiveError
      ? "Archive unavailable"
      : archiveRecordings
        ? `${archiveRecordings.items.length} loaded`
        : "Not loaded";

  return html`
    <section class="events archive-panel">
      <div class="archive-head">
        <div class="panel-title">
          <span class="split-row">
            <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
              ${renderIcon("mdi:filmstrip")}
            </span>
            <span>${title}</span>
          </span>
        </div>
        <div class="chip-row archive-summary">
          <span class="badge ${statusTone}">${countText}</span>
          <span class="badge info">${formatArchiveDateBadge(archiveDate)}</span>
          ${archiveRecordings?.channel !== undefined && archiveRecordings?.channel !== null
            ? html`<span class="badge">Channel ${archiveRecordings.channel}</span>`
            : null}
          ${showEventFilter &&
          archiveEventCode.trim() &&
          archiveEventCode !== EVENT_FILTER_ALL
            ? html`<span class="badge">${archiveRecordingEventLabel(archiveEventCode)}</span>`
            : null}
        </div>
        <div class="archive-filter-row">
          <label class="event-filter archive-date-filter">
            <span class="event-filter-label">Date</span>
            <input
              class="event-filter-select archive-date-input"
              type="date"
              .value=${archiveDate}
              @change=${(event: Event) =>
                onSelectArchiveDate((event.currentTarget as HTMLInputElement).value)}
            />
          </label>
          ${showEventFilter
            ? html`
                <label class="event-filter archive-event-filter">
                  <span class="event-filter-label">Event type</span>
                  <select
                    class="event-filter-select"
                    .value=${archiveEventCode}
                    @change=${(event: Event) =>
                      onSelectArchiveEventType(
                        (event.currentTarget as HTMLSelectElement).value,
                      )}
                  >
                    ${archiveEventTypeOptions.map(
                      (option) =>
                        html`<option value=${option.value}>${option.label}</option>`,
                    )}
                  </select>
                </label>
              `
            : null}
          ${renderArchivePagination({
            page,
            pageCount,
            onSelectPage: onSelectArchivePage,
            renderIcon,
          })}
        </div>
      </div>
      ${archiveError ? html`<div class="muted">${archiveError}</div>` : null}
      <div class="events-list">
        ${visibleItems.length > 0
          ? repeat(
              visibleItems,
              (item) =>
                `${item.channel}:${item.startTime}:${item.endTime}:${item.filePath ?? item.type ?? ""}`,
              (item) => {
                const metadata = showEventFilter
                  ? [{ label: "Start", value: formatArchiveDateTime(item.startTime) }]
                  : [
                      { label: "Start", value: formatArchiveDateTime(item.startTime) },
                      { label: "End", value: formatArchiveDateTime(item.endTime) },
                    ];
                return html`
                  <div class="event-card info archive-entry-card">
                    <div class="event-card-body">
                      <div class="archive-entry-row">
                        <span class="archive-entry-main">
                          <span class="archive-entry-icon" aria-hidden="true">
                            ${renderIcon(archiveRecordingEventIcon(item, showEventFilter))}
                          </span>
                          <span class="archive-entry-meta">
                            ${repeat(
                              metadata,
                              (entry) => `${entry.label}:${entry.value}`,
                              (entry) => renderArchiveInlineDetail(entry.label, entry.value),
                            )}
                          </span>
                        </span>
                        <span class="archive-entry-actions">
                          ${playbackSupported || item.assetPlaybackUrl
                            ? renderControlButton(
                                isPlaybackActive(item) ? "Playing" : "Play",
                                isPlaybackActive(item)
                                  ? "mdi:play-circle"
                                  : "mdi:play-circle-outline",
                                () => onLaunchPlayback(item),
                                renderIcon,
                                {
                                  compact: true,
                                  disabled: isLaunchingPlayback(item),
                                  tone: "primary",
                                  active: isPlaybackActive(item),
                                },
                              )
                            : null}
                          ${item.assetDownloadUrl
                            ? renderControlButton(
                                "MP4",
                                "mdi:download",
                                () => onDownloadRecording(item, "asset"),
                                renderIcon,
                                {
                                  compact: true,
                                  disabled: isDownloadingRecording(item),
                                },
                              )
                            : null}
                          ${item.downloadUrl
                            ? renderControlButton(
                                "DAV",
                                "mdi:file-download-outline",
                                () => onDownloadRecording(item, "raw"),
                                renderIcon,
                                {
                                  compact: true,
                                  disabled: isDownloadingRecording(item),
                                },
                              )
                            : null}
                          ${!item.assetDownloadUrl && !item.downloadUrl && item.exportUrl
                            ? renderControlButton(
                                "Export MP4",
                                "mdi:download",
                                () => onDownloadRecording(item, "asset"),
                                renderIcon,
                                {
                                  compact: true,
                                  disabled: isDownloadingRecording(item),
                                },
                              )
                            : null}
                        </span>
                      </div>
                    </div>
                  </div>
                `;
              },
            )
          : html`
              <div class="event-card">
                <div class="muted">
                  ${archiveLoading
                    ? showEventFilter
                      ? "Loading events."
                      : "Loading archive recordings."
                    : showEventFilter
                      ? `No events found for ${formatArchiveDateBadge(archiveDate).toLowerCase()}.`
                      : `No recordings found for ${formatArchiveDateBadge(archiveDate).toLowerCase()}.`}
                </div>
              </div>
            `}
      </div>
    </section>
  `;
}

export function renderBridgeRecordings({
  title,
  recordings,
  recordingsLoading,
  recordingsError,
  recordingsDate,
  page,
  pageCount,
  visibleItems,
  playbackSupported,
  isPlaybackActive,
  isDownloadingRecording,
  onSelectDate,
  onSelectPage,
  onPlayRecording,
  onDownloadRecording,
  renderIcon,
}: RenderBridgeRecordingsArgs): TemplateResult {
  const statusTone = recordingsError ? "warning" : recordingsLoading ? "info" : "success";
  const countText = recordingsLoading
    ? "Loading"
    : recordingsError
      ? "MP4 unavailable"
      : recordings
        ? `${recordings.items.length} loaded`
        : "Not loaded";

  return html`
    <section class="events archive-panel">
      <div class="archive-head">
        <div class="panel-title">
          <span class="split-row">
            <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
              ${renderIcon("mdi:file-video-outline")}
            </span>
            <span>${title}</span>
          </span>
        </div>
        <div class="chip-row archive-summary">
          <span class="badge ${statusTone}">${countText}</span>
          <span class="badge info">${formatArchiveDateBadge(recordingsDate)}</span>
        </div>
        <div class="archive-filter-row">
          <label class="event-filter archive-date-filter">
            <span class="event-filter-label">Date</span>
            <input
              class="event-filter-select archive-date-input"
              type="date"
              .value=${recordingsDate}
              @change=${(event: Event) =>
                onSelectDate((event.currentTarget as HTMLInputElement).value)}
            />
          </label>
          ${renderArchivePagination({
            page,
            pageCount,
            onSelectPage,
            renderIcon,
          })}
        </div>
      </div>
      ${recordingsError ? html`<div class="muted">${recordingsError}</div>` : null}
      <div class="events-list">
        ${visibleItems.length > 0
          ? repeat(
              visibleItems,
              (item) => item.id,
              (item) => html`
                <div class="event-card info archive-entry-card">
                  <div class="event-card-body">
                    <div class="archive-entry-row">
                      <span class="archive-entry-main">
                        <span class="archive-entry-icon" aria-hidden="true">
                          ${renderIcon("mdi:file-video-outline")}
                        </span>
                        <span class="archive-entry-meta">
                          ${renderArchiveInlineDetail(
                            "Start",
                            formatArchiveDateTime(bridgeRecordingStartTime(item)),
                          )}
                          ${renderArchiveInlineDetail(
                            "End",
                            formatArchiveDateTime(bridgeRecordingEndTime(item)),
                          )}
                        </span>
                      </span>
                      <span class="archive-entry-actions">
                        ${playbackSupported && item.playbackUrl
                          ? renderControlButton(
                              isPlaybackActive(item) ? "Playing" : "Play",
                              isPlaybackActive(item)
                                ? "mdi:play-circle"
                                : "mdi:play-circle-outline",
                              () => onPlayRecording(item),
                              renderIcon,
                              {
                                compact: true,
                                tone: "primary",
                                active: isPlaybackActive(item),
                              },
                            )
                          : null}
                        ${item.downloadUrl
                          ? renderControlButton(
                              "Download",
                              "mdi:download",
                              () => onDownloadRecording(item),
                              renderIcon,
                              {
                                compact: true,
                                disabled: isDownloadingRecording(item),
                              },
                            )
                          : null}
                      </span>
                    </div>
                    ${renderBridgeRecordingStatus(item)}
                  </div>
                </div>
              `,
            )
          : html`
              <div class="event-card">
                <div class="muted">
                  ${recordingsLoading
                    ? "Loading MP4 clips."
                    : `No MP4 clips found for ${formatArchiveDateBadge(recordingsDate).toLowerCase()}.`}
                </div>
              </div>
            `}
      </div>
    </section>
  `;
}

export function resolveSelectedNvrArchiveCamera(
  model: PanelModel,
  nvrArchiveChannelNumber: number | null,
): {
  camera: CameraViewModel | null;
  nextChannelNumber: number | null;
} {
  const channels =
    model.selectedNvr?.rooms.flatMap((room) => room.channels).filter(
      (channel) => channel.archive?.searchUrl && channel.archive.channel !== null,
    ) ?? [];
  if (channels.length === 0) {
    return { camera: null, nextChannelNumber: null };
  }

  if (nvrArchiveChannelNumber !== null) {
    const matchingChannel =
      channels.find((channel) => channel.channelNumber === nvrArchiveChannelNumber) ?? null;
    if (matchingChannel) {
      return {
        camera: matchingChannel,
        nextChannelNumber: nvrArchiveChannelNumber,
      };
    }
  }

  const fallbackChannel = channels[0] ?? null;
  return {
    camera: fallbackChannel,
    nextChannelNumber: fallbackChannel?.channelNumber ?? null,
  };
}

function renderArchivePagination({
  page,
  pageCount,
  onSelectPage,
  renderIcon,
}: {
  page: number;
  pageCount: number;
  onSelectPage: (page: number) => void;
  renderIcon: (icon: string) => TemplateResult;
}): TemplateResult {
  const clampedPage = Math.max(0, Math.min(page, Math.max(pageCount - 1, 0)));

  return html`
    <div class="archive-pagination">
      ${renderControlButton(
        "Previous",
        "mdi:chevron-left",
        () => onSelectPage(clampedPage - 1),
        renderIcon,
        {
          compact: true,
          disabled: clampedPage <= 0,
        },
      )}
      <span class="badge">${clampedPage + 1}/${Math.max(pageCount, 1)}</span>
      ${renderControlButton(
        "Next",
        "mdi:chevron-right",
        () => onSelectPage(clampedPage + 1),
        renderIcon,
        {
          compact: true,
          disabled: clampedPage >= pageCount - 1,
        },
      )}
    </div>
  `;
}

function renderArchiveDetail(
  label: string,
  value: string | null,
  wide = false,
  critical = false,
): TemplateResult | null {
  if (!value?.trim()) {
    return null;
  }
  return html`
    <div class="event-detail ${wide ? "archive-detail-wide" : ""}">
      <div class="event-detail-label">${label}</div>
      <div class="event-detail-value ${critical ? "archive-status-error" : ""}">${value}</div>
    </div>
  `;
}

function renderArchiveInlineDetail(
  label: string,
  value: string | null,
): TemplateResult | null {
  if (!value?.trim()) {
    return null;
  }
  return html`
    <span class="archive-entry-detail">
      <span class="archive-entry-detail-label">${label}</span>
      <span class="archive-entry-detail-value">${value}</span>
    </span>
  `;
}

function formatArchiveDateTime(value: string | null | undefined): string {
  const text = value?.trim();
  if (!text) {
    return "-";
  }
  const parsed = new Date(text);
  if (Number.isNaN(parsed.getTime())) {
    return text;
  }
  return archiveDateTimeFormatter.format(parsed);
}

function formatArchiveDateBadge(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    return "Selected date";
  }
  const parsed = new Date(`${trimmed}T00:00:00`);
  if (Number.isNaN(parsed.getTime())) {
    return trimmed;
  }
  return parsed.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
  });
}

function firstArchiveRecordingCode(recording: NvrArchiveRecordingModel): string | null {
  const candidates = [recording.type ?? "", ...recording.flags];
  for (const candidate of candidates) {
    const normalized = normalizeArchiveRecordingCode(candidate);
    if (normalized && normalized.toLowerCase() !== "event") {
      return normalized;
    }
  }
  return null;
}

function normalizeArchiveRecordingCode(value: string): string {
  let normalized = value.trim();
  if (!normalized) {
    return "";
  }
  if (normalized.toLowerCase().startsWith("event.")) {
    normalized = normalized.slice("event.".length);
  }
  return normalized.trim();
}

function archiveRecordingEventLabel(value: string): string {
  switch (normalizeArchiveRecordingCode(value).toLowerCase()) {
    case "motion":
    case "videomotion":
    case "movedetection":
    case "alarmpir":
      return "Motion";
    case "human":
    case "humandetection":
    case "smartmotionhuman":
    case "intelliframehuman":
    case "smdtypehuman":
      return "Human";
    case "vehicle":
    case "vehicledetection":
    case "smartmotionvehicle":
    case "motorvehicle":
    case "smdtypevehicle":
      return "Vehicle";
    case "animal":
    case "animaldetection":
    case "smdtypeanimal":
      return "Animal";
    case "crosslinedetection":
    case "tripwire":
      return "Cross Line";
    case "crossregiondetection":
    case "intrusion":
      return "Cross Region";
    case "leftdetection":
      return "Left Detection";
    case "access":
    case "accesscontrol":
    case "accessctl":
      return "Access";
    default:
      return humanizeCode(value);
  }
}

function archiveRecordingEventIcon(
  recording: NvrArchiveRecordingModel,
  eventMode: boolean,
): string {
  if (!eventMode) {
    return "mdi:filmstrip-box-multiple";
  }
  switch (normalizeArchiveRecordingCode(firstArchiveRecordingCode(recording) ?? "").toLowerCase()) {
    case "human":
    case "humandetection":
    case "smartmotionhuman":
    case "intelliframehuman":
    case "smdtypehuman":
      return "mdi:account";
    case "vehicle":
    case "vehicledetection":
    case "smartmotionvehicle":
    case "motorvehicle":
    case "smdtypevehicle":
      return "mdi:car";
    case "animal":
    case "animaldetection":
    case "smdtypeanimal":
      return "mdi:paw";
    case "crosslinedetection":
    case "tripwire":
      return "mdi:vector-line";
    case "crossregiondetection":
    case "intrusion":
      return "mdi:vector-square";
    case "leftdetection":
      return "mdi:exit-run";
    case "motion":
    case "videomotion":
    case "movedetection":
    case "alarmpir":
      return "mdi:motion-sensor";
    default:
      return "mdi:bell-alert-outline";
  }
}

function renderBridgeRecordingStatus(recording: BridgeRecordingClipModel): TemplateResult | null {
  if (recording.error?.trim()) {
    return html`<div class="archive-entry-foot archive-status-error">${recording.error}</div>`;
  }
  return null;
}

function bridgeRecordingStartTime(recording: BridgeRecordingClipModel): string {
  return recording.sourceStartTime ?? recording.startedAt;
}

function bridgeRecordingEndTime(recording: BridgeRecordingClipModel): string | null {
  return recording.sourceEndTime ?? recording.endedAt;
}

function humanizeCode(value: string): string {
  return value
    .trim()
    .replace(/[_-]+/g, " ")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/\b\w/g, (match) => match.toUpperCase());
}
