import { html, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import { renderControlButton } from "./surveillance-panel-primitives";
import type {
  NvrArchiveRecordingModel,
  NvrArchiveSearchResultModel,
} from "../domain/archive";
import type { CameraViewModel, PanelModel } from "../domain/model";

interface RenderArchiveRecordingsArgs {
  model: PanelModel;
  archiveRecordings: NvrArchiveSearchResultModel | null;
  archiveLoading: boolean;
  archiveError: string;
  eventWindowHours: number;
  playbackSupported: boolean;
  isLaunchingPlayback: (recording: NvrArchiveRecordingModel) => boolean;
  isPlaybackActive: (recording: NvrArchiveRecordingModel) => boolean;
  onLaunchPlayback: (recording: NvrArchiveRecordingModel) => void;
  onDownloadRecording: (recording: NvrArchiveRecordingModel) => void;
  renderIcon: (icon: string) => TemplateResult;
}

export function renderArchiveRecordings({
  model,
  archiveRecordings,
  archiveLoading,
  archiveError,
  eventWindowHours,
  playbackSupported,
  isLaunchingPlayback,
  isPlaybackActive,
  onLaunchPlayback,
  onDownloadRecording,
  renderIcon,
}: RenderArchiveRecordingsArgs): TemplateResult {
  const archiveTitle = model.selectedNvr ? "Recorder Archive Search" : "Recent Recordings";

  return html`
    <section class="events archive-panel">
      <div class="archive-head">
        <div class="panel-title">
          <span class="split-row">
            <span class="header-chip-icon nvr-storage-icon" aria-hidden="true">
              ${renderIcon("mdi:filmstrip")}
            </span>
            <span>${archiveTitle}</span>
          </span>
        </div>
        <div class="chip-row archive-summary">
          <span class="badge ${archiveError ? "warning" : archiveLoading ? "info" : "success"}">
            ${archiveLoading
              ? "Loading"
              : archiveError
                ? "Archive unavailable"
                : archiveRecordings
                  ? `${archiveRecordings.returnedCount} visible`
                  : "Not loaded"}
          </span>
          <span class="badge info">${eventWindowHours}h window</span>
          ${archiveRecordings?.channel
            ? html`<span class="badge">${`Channel ${archiveRecordings.channel}`}</span>`
            : null}
        </div>
      </div>
      ${archiveError ? html`<div class="muted">${archiveError}</div>` : null}
      <div class="events-list">
        ${archiveRecordings && archiveRecordings.items.length > 0
          ? repeat(
              archiveRecordings.items,
              (item) =>
                `${item.startTime}:${item.endTime}:${item.filePath ?? item.type ?? "recording"}`,
                (item) => html`
                  <div class="event-card info">
                    <div class="event-card-body">
                      <div class="panel-title">
                        <span>${item.type ?? "Recording"}</span>
                        <span class="split-row">
                          <span class="badge info">${item.videoStream ?? "archive"}</span>
                          ${playbackSupported
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
                          ${item.downloadUrl
                            ? renderControlButton(
                                "Download",
                                "mdi:download",
                                () => onDownloadRecording(item),
                                renderIcon,
                                {
                                  compact: true,
                                },
                              )
                            : null}
                        </span>
                      </div>
                      <div class="event-detail-list">
                      <div class="event-detail">
                        <div class="event-detail-label">Start</div>
                        <div class="event-detail-value">${item.startTime}</div>
                      </div>
                      <div class="event-detail">
                        <div class="event-detail-label">End</div>
                        <div class="event-detail-value">${item.endTime}</div>
                      </div>
                      ${item.filePath
                        ? html`
                            <div class="event-detail">
                              <div class="event-detail-label">File</div>
                              <div class="event-detail-value">${item.filePath}</div>
                            </div>
                          `
                        : null}
                    </div>
                  </div>
                </div>
              `,
            )
          : html`
              <div class="event-card">
                <div class="muted">
                  ${archiveLoading
                    ? "Loading archive recordings."
                    : "No recordings found for the selected time window."}
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
