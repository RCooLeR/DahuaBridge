import type {TemplateResult} from "lit";
import {html, nothing} from "lit";
import {repeat} from "lit/directives/repeat.js";

import type {CameraViewModel} from "../domain/model";
import type {Localizer} from "../localization";
import {
    ARCHIVE_SEEK_STEP_SECONDS,
    archiveSeekDateTimeFromSecond,
    archiveSeekGridLabelStyle,
    buildArchiveSeekModel,
    parseArchiveSeekSecond,
} from "./surveillance-panel-archive-seek-model";

export interface ArchiveSeekPanelCallbacks {
    onSelectArchiveDate: (value: string) => void;
    onInputArchiveSecond: (second: number) => void;
    onStartPlayback: (camera: CameraViewModel, seekTime: Date) => void;
}

export interface ArchiveSeekPanelState {
    t: Localizer;
    camera: CameraViewModel;
    archiveDate: string;
    archiveSeekSecond: number;
    callbacks: ArchiveSeekPanelCallbacks;
}

export function renderArchiveSeekPanel({
    t,
    camera,
    archiveDate,
    archiveSeekSecond,
    callbacks,
}: ArchiveSeekPanelState): TemplateResult | typeof nothing {
    if (!camera.cameraEntityId || camera.channelNumber === null) {
        return nothing;
    }

    const seekModel = buildArchiveSeekModel(archiveDate, archiveSeekSecond);

    return html`
        <div class="slider-wrap archive-seek-panel">
            <div class="split-row">
                <span class="badge info">${t("archive.seek")}</span>
                <span class="muted">${seekModel.label}</span>
            </div>
            <label class="event-filter archive-date-filter">
                <span class="event-filter-label">${t("archive.seekDate")}</span>
                <input
                        class="event-filter-select archive-date-input"
                        type="date"
                        .value=${seekModel.date}
                        min=${seekModel.bounds.min}
                        max=${seekModel.bounds.max}
                        @change=${(event: Event) =>
                                callbacks.onSelectArchiveDate((event.currentTarget as HTMLInputElement).value)}
                />
            </label>
            <div
                    class="archive-seek-range-wrap"
                    style=${seekModel.gridStyle}
            >
                <div class="archive-seek-grid" aria-hidden="true"></div>
                <input
                        class="archive-seek-range"
                        type="range"
                        min="0"
                        max=${String(seekModel.maxSecond)}
                        step=${String(ARCHIVE_SEEK_STEP_SECONDS)}
                        list="archive-seek-30min-grid"
                        .value=${String(seekModel.second)}
                        @input=${(event: Event) => {
                            callbacks.onInputArchiveSecond(
                                    parseArchiveSeekSecond((event.currentTarget as HTMLInputElement).value),
                            );
                        }}
                        @change=${(event: Event) => {
                            const second = parseArchiveSeekSecond(
                                    (event.currentTarget as HTMLInputElement).value,
                            );
                            const seekTime = archiveSeekDateTimeFromSecond(seekModel.date, second);
                            if (seekTime) {
                                callbacks.onStartPlayback(camera, seekTime);
                            }
                        }}
                />
                <datalist id="archive-seek-30min-grid">
                    ${repeat(
                            seekModel.ticks,
                            (second) => second,
                            (second) => html`<option value=${String(second)}></option>`,
                    )}
                </datalist>
            </div>
            <div class="archive-seek-grid-labels" aria-hidden="true">
                ${repeat(
                        seekModel.labels,
                        (label) => label.hour,
                        (label) => html`
                            <span
                                    class="archive-seek-grid-label"
                                    style=${archiveSeekGridLabelStyle(label.leftPercent)}
                            >${label.hour}</span>
                        `,
                )}
            </div>
            <div class="split-row muted">
                <span>00:00</span>
                <span>23:59</span>
            </div>
        </div>
    `;
}
