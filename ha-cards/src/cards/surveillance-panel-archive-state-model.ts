import type {NvrArchiveSearchResultModel} from "../domain/archive";
import type {CameraViewModel, PanelSelection} from "../domain/model";
import {createLocalizer, type Localizer} from "../localization";
import type {DetailTab} from "./surveillance-panel-state";
import {
    SMD_IVS_ARCHIVE_MODE,
    smdIvsArchiveMissingUrlMessage,
    smdIvsArchiveUrl,
    type SmdIvsArchiveMode,
} from "./surveillance-panel-smd-ivs-model";
import {
    CHUNKS_ARCHIVE_MODE,
    chunksArchiveMissingUrlMessage,
    chunksArchiveUrl,
    type ChunksArchiveMode,
} from "./surveillance-panel-chunks-model";

export const ARCHIVE_PAGE_SIZE = 20;

export type ArchiveRecordingsMode = SmdIvsArchiveMode | ChunksArchiveMode;

export function archiveModeForSelection(
    selectionKind: PanelSelection["kind"],
    detailTab: DetailTab,
): ArchiveRecordingsMode {
    return selectionKind === "camera" && detailTab === "events"
        ? SMD_IVS_ARCHIVE_MODE
        : CHUNKS_ARCHIVE_MODE;
}

export function archiveUrlForMode(
    camera: CameraViewModel | null,
    mode: ArchiveRecordingsMode,
): string | null {
    return mode === SMD_IVS_ARCHIVE_MODE
        ? smdIvsArchiveUrl(camera)
        : chunksArchiveUrl(camera);
}

export function archiveMissingUrlMessage(
    mode: ArchiveRecordingsMode,
    t: Localizer = createLocalizer("en"),
): string {
    return mode === SMD_IVS_ARCHIVE_MODE
        ? smdIvsArchiveMissingUrlMessage(t)
        : chunksArchiveMissingUrlMessage(t);
}

export function archiveRecordingsForMode(
    recordings: NvrArchiveSearchResultModel | null,
    currentMode: ArchiveRecordingsMode | null,
    requestedMode: ArchiveRecordingsMode,
): NvrArchiveSearchResultModel | null {
    return currentMode === requestedMode ? recordings : null;
}

export function isArchiveModeLoading(
    currentMode: ArchiveRecordingsMode | null,
    loading: boolean,
    requestedMode: ArchiveRecordingsMode,
): boolean {
    return currentMode === requestedMode && loading;
}

export function archiveErrorForMode(
    currentMode: ArchiveRecordingsMode | null,
    error: string,
    requestedMode: ArchiveRecordingsMode,
): string {
    return currentMode === requestedMode ? error : "";
}
