import type {HomeAssistant} from "../types/home-assistant";
import {logCardInfo} from "../utils/logging";

export const ARCHIVE_SEEK_STEP_SECONDS = 1;

const ARCHIVE_SEEK_MAX_LOOKBACK_DAYS = 90;
const ARCHIVE_SEEK_GRID_STEP_SECONDS = 60 * 30;
const ARCHIVE_SEEK_GRID_LABEL_HOURS = [2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22] as const;

export interface ArchiveSeekGridLabel {
    hour: number;
    leftPercent: number;
}

export interface ArchiveSeekModel {
    date: string;
    second: number;
    maxSecond: number;
    bounds: {
        min: string;
        max: string;
    };
    label: string;
    gridStyle: string;
    ticks: number[];
    labels: ArchiveSeekGridLabel[];
}

export function buildArchiveSeekModel(archiveDate: string, second: number): ArchiveSeekModel {
    const date = normalizeArchiveDateInput(archiveDate);
    const maxSecond = archiveMaxSecondForDate(date);
    const boundedSecond = Math.min(parseArchiveSeekSecond(String(second)), maxSecond);
    return {
        date,
        second: boundedSecond,
        maxSecond,
        bounds: archiveSeekDateBounds(),
        label: formatSeekSecondLabel(date, boundedSecond),
        gridStyle: archiveSeekGridStyle(maxSecond),
        ticks: archiveSeekTickSeconds(maxSecond),
        labels: archiveSeekGridLabels(maxSecond),
    };
}

export function todayDateInputValue(): string {
    return toDateInputValue(new Date());
}

export function normalizeArchiveDateInput(value: string): string {
    const trimmed = value.trim();
    if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) {
        const parsed = new Date(`${trimmed}T00:00:00`);
        if (!Number.isNaN(parsed.getTime()) && toDateInputValue(parsed) === trimmed) {
            return trimmed;
        }
    }
    return todayDateInputValue();
}

export function dateRangeForArchiveDay(value: string): {
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

export function toDateInputValue(value: Date): string {
    const year = value.getFullYear();
    const month = String(value.getMonth() + 1).padStart(2, "0");
    const day = String(value.getDate()).padStart(2, "0");
    return `${year}-${month}-${day}`;
}

export function archiveSeekDateBounds(): { min: string; max: string } {
    const max = new Date();
    const min = new Date(max);
    min.setDate(max.getDate() - ARCHIVE_SEEK_MAX_LOOKBACK_DAYS);
    return {
        min: toDateInputValue(min),
        max: toDateInputValue(max),
    };
}

export function parseArchiveSeekSecond(value: string): number {
    const parsed = Number.parseInt(value, 10);
    if (!Number.isFinite(parsed)) {
        return 0;
    }
    return Math.min(Math.max(parsed, 0), 86_399);
}

export function archiveSeekGridStyle(maxSeekSecond: number): string {
    const boundedMaxSecond = Math.max(
        ARCHIVE_SEEK_STEP_SECONDS,
        parseArchiveSeekSecond(String(maxSeekSecond)),
    );
    const halfHourPercent = (ARCHIVE_SEEK_GRID_STEP_SECONDS / boundedMaxSecond) * 100;
    const hourPercent = ((ARCHIVE_SEEK_GRID_STEP_SECONDS * 2) / boundedMaxSecond) * 100;
    return [
        `--archive-seek-grid-half-hour: ${halfHourPercent.toFixed(4)}%`,
        `--archive-seek-grid-hour: ${hourPercent.toFixed(4)}%`,
    ].join("; ");
}

export function archiveSeekTickSeconds(maxSeekSecond: number): number[] {
    const maxSecond = parseArchiveSeekSecond(String(maxSeekSecond));
    const ticks: number[] = [];
    for (let second = 0; second <= maxSecond; second += ARCHIVE_SEEK_GRID_STEP_SECONDS) {
        ticks.push(second);
    }
    if (ticks.length === 0 || ticks[ticks.length - 1] !== maxSecond) {
        ticks.push(maxSecond);
    }
    return ticks;
}

export function archiveSeekGridLabels(maxSeekSecond: number): ArchiveSeekGridLabel[] {
    const maxSecond = parseArchiveSeekSecond(String(maxSeekSecond));
    if (maxSecond <= 0) {
        return [];
    }
    return ARCHIVE_SEEK_GRID_LABEL_HOURS
        .map((hour) => ({
            hour,
            second: hour * 60 * 60,
        }))
        .filter((label) => label.second <= maxSecond)
        .map((label) => ({
            hour: label.hour,
            leftPercent: (label.second / maxSecond) * 100,
        }));
}

export function archiveSeekGridLabelStyle(leftPercent: number): string {
    return `left: ${Math.min(Math.max(leftPercent, 0), 100).toFixed(4)}%`;
}

export function archiveCurrentTimeOfDaySecond(): number {
    return secondsSinceLocalMidnight(new Date());
}

export function archiveMaxSecondForDate(archiveDate: string): number {
    return normalizeArchiveDateInput(archiveDate) === todayDateInputValue()
        ? archiveCurrentTimeOfDaySecond()
        : 86_399;
}

export function archiveSeekDateTimeFromSecond(archiveDate: string, second: number): Date | null {
    const normalizedDate = normalizeArchiveDateInput(archiveDate);
    const [year, month, day] = normalizedDate
        .split("-")
        .map((part) => Number.parseInt(part, 10));
    if (!year || !month || !day) {
        return null;
    }
    const boundedSecond = Math.min(
        parseArchiveSeekSecond(String(second)),
        archiveMaxSecondForDate(normalizedDate),
    );
    const hours = Math.floor(boundedSecond / 3600);
    const minutes = Math.floor((boundedSecond % 3600) / 60);
    const seconds = boundedSecond % 60;
    return new Date(year, month - 1, day, hours, minutes, seconds, 0);
}

export function secondsSinceLocalMidnight(value: Date): number {
    return value.getHours() * 3600 + value.getMinutes() * 60 + value.getSeconds();
}

export function archiveDefaultTimeframeEndTime(startTime: Date): Date {
    return new Date(startTime.getTime() + 30 * 60 * 1000);
}

export function buildArchiveTimeframeProxyPath(
    entityID: string,
    startTime: Date,
    endTime: Date,
    profileKey: string | null,
): string {
    const params = new URLSearchParams();
    params.set("starttime", formatArchiveProxyTimestamp(startTime));
    params.set("endtime", formatArchiveProxyTimestamp(endTime));
    if (profileKey?.trim()) {
        params.set("profile", profileKey.trim());
    }
    return `/api/camera_proxy/${encodeURIComponent(entityID)}/timeframe/?${params.toString()}`;
}

export function buildArchiveTimeframeSnapshotProxyPath(
    entityID: string,
    startTime: Date,
    endTime: Date,
    seekTime: Date,
    profileKey: string | null,
): string {
    const params = new URLSearchParams();
    params.set("starttime", formatArchiveProxyTimestamp(startTime));
    params.set("endtime", formatArchiveProxyTimestamp(endTime));
    params.set("seektime", formatArchiveProxyTimestamp(seekTime));
    if (profileKey?.trim()) {
        params.set("profile", profileKey.trim());
    }
    return `/api/camera_proxy/${encodeURIComponent(entityID)}/timeframe/snapshot/?${params.toString()}`;
}

export function formatArchiveProxyTimestamp(value: Date): string {
    const parts = [
        value.getFullYear(),
        value.getMonth() + 1,
        value.getDate(),
        value.getHours(),
        value.getMinutes(),
        value.getSeconds(),
    ];
    return parts
        .map((part, index) => (index === 0 ? String(part) : String(part).padStart(2, "0")))
        .join("_");
}

export async function signHomeAssistantPath(
    hass: HomeAssistant | undefined,
    path: string,
): Promise<string> {
    const message = {type: "auth/sign_path", path, expires: 300};
    try {
        const callWS = hass?.callWS?.bind(hass);
        if (callWS) {
            const response = await callWS<{path?: string}>(message);
            return response.path?.trim() || path;
        }

        const sendMessage = hass?.connection?.sendMessagePromise?.bind(hass.connection);
        if (sendMessage) {
            const response = await sendMessage<{path?: string}>(message);
            return response.path?.trim() || path;
        }
    } catch (error) {
        logCardInfo("card panel archive proxy signing failed", {
            path,
            error: error instanceof Error ? error.message : String(error),
        });
    }
    return path;
}

export function formatSeekSecondLabel(archiveDate: string, second: number): string {
    const seekTime = archiveSeekDateTimeFromSecond(archiveDate, second);
    if (!seekTime) {
        return "";
    }
    return seekTime.toLocaleString();
}
