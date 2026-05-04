import type {NvrArchiveSearchResultModel} from "../domain/archive";
import type {CameraViewModel} from "../domain/model";
import {createLocalizer, type Localizer} from "../localization";

export const SMD_IVS_ARCHIVE_MODE = "events" as const;

export type SmdIvsArchiveMode = typeof SMD_IVS_ARCHIVE_MODE;

export function smdIvsArchiveUrl(camera: CameraViewModel | null): string | null {
    return camera?.archive?.smdIvsUrl ?? null;
}

export function smdIvsArchiveMissingUrlMessage(t: Localizer = createLocalizer("en")): string {
    return t("archive.noEventUrl");
}

export function smdIvsArchiveItems(
    recordings: NvrArchiveSearchResultModel | null,
): NvrArchiveSearchResultModel["items"] {
    return recordings?.items ?? [];
}
