import type {NvrArchiveSearchResultModel} from "../domain/archive";
import type {CameraViewModel} from "../domain/model";
import {createLocalizer, type Localizer} from "../localization";

export const CHUNKS_ARCHIVE_MODE = "chunks" as const;

export type ChunksArchiveMode = typeof CHUNKS_ARCHIVE_MODE;

export function chunksArchiveUrl(camera: CameraViewModel | null): string | null {
    return camera?.archive?.chunksUrl ?? null;
}

export function chunksArchiveMissingUrlMessage(t: Localizer = createLocalizer("en")): string {
    return t("archive.noChunkUrl");
}

export function chunksArchiveItems(
    recordings: NvrArchiveSearchResultModel | null,
): NvrArchiveSearchResultModel["items"] {
    return recordings?.items ?? [];
}
