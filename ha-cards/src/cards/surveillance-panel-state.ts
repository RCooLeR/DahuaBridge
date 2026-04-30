import type { TimelineEvent } from "../domain/events";
import type {
  CameraViewModel,
  NvrViewModel,
  PanelModel,
  PanelSelection,
  SidebarFilter,
  VtoViewModel,
} from "../domain/model";

export type DetailTab = "overview" | "events" | "recordings" | "settings";
export type EventViewMode = "recent" | "history";
export type OverviewLayoutName = "1x1" | "2x1" | "2x2" | "3x2" | "3x3" | "4x3" | "4x4";
export type SidebarItemKind = "camera" | "vto" | "accessory" | "nvr";
export type VtoBadgeTone = "neutral" | "success" | "warning" | "info" | "critical";
export const EVENT_FILTER_ALL = "__all__";

export interface TimelineEventFilters {
  eventCode: string;
}

export interface EventFilterOption {
  value: string;
  label: string;
}

export interface TimelineEventFilterOptions {
  eventCodes: EventFilterOption[];
}

export interface SurveillanceOverviewLayout {
  name: OverviewLayoutName;
  columns: number;
  rows: number;
}

export interface SelectionStatePatch {
  selection: PanelSelection;
  detailTab: DetailTab;
  ptzAdjusting: boolean;
  eventHistoryPage: number;
}

export function selectOverviewState(): SelectionStatePatch {
  return {
    selection: { kind: "overview" },
    detailTab: "overview",
    ptzAdjusting: false,
    eventHistoryPage: 0,
  };
}

export function selectCameraState(camera: CameraViewModel): SelectionStatePatch {
  return {
    selection: { kind: "camera", deviceId: camera.deviceId },
    detailTab: "overview",
    ptzAdjusting: false,
    eventHistoryPage: 0,
  };
}

export function selectNvrState(nvr: NvrViewModel): SelectionStatePatch {
  return {
    selection: { kind: "nvr", deviceId: nvr.deviceId },
    detailTab: "overview",
    ptzAdjusting: false,
    eventHistoryPage: 0,
  };
}

export function selectVtoState(vto: VtoViewModel): SelectionStatePatch {
  return {
    selection: { kind: "vto", deviceId: vto.deviceId },
    detailTab: "overview",
    ptzAdjusting: false,
    eventHistoryPage: 0,
  };
}

export function parseHistoryPageInput(event: Event): number | null {
  const target = event.currentTarget as HTMLInputElement;
  const nextPage = Number.parseInt(target.value, 10);
  return Number.isFinite(nextPage) ? Math.max(0, nextPage) : null;
}

export function matchesSidebarFilters(
  searchText: string,
  sidebarFilter: SidebarFilter,
  label: string,
  secondary: string,
  kind: SidebarItemKind,
  highlighted: boolean,
): boolean {
  const needle = searchText.trim().toLowerCase();
  if (needle) {
    const haystack = `${label} ${secondary}`.toLowerCase();
    if (!haystack.includes(needle)) {
      return false;
    }
  }

  if (sidebarFilter === "alerts") {
    return highlighted;
  }
  if (sidebarFilter === "nvr") {
    return kind === "camera" || kind === "nvr";
  }
  if (sidebarFilter === "vto") {
    return kind === "vto" || kind === "accessory";
  }
  return true;
}

export function selectionTitle(model: PanelModel): string {
  if (model.selectedNvr) {
    return model.selectedNvr.label;
  }
  if (model.selectedCamera) {
    return `${model.selectedCamera.label} - ${model.selectedCamera.roomLabel}`;
  }
  if (model.selectedVto) {
    return model.selectedVto.label;
  }
  return model.title;
}

export function vtoBadgeClass(vto: VtoViewModel): string {
  return vtoBadgeTone(vto);
}

export function vtoBadgeTone(vto: VtoViewModel): VtoBadgeTone {
  return vto.callState === "active"
    ? "info"
    : vto.callState === "ringing"
      ? "warning"
      : vto.online
        ? "success"
        : "critical";
}

export function visibleTimelineEvents(
  events: TimelineEvent[],
  eventViewMode: EventViewMode,
  eventHistoryPage: number,
  pageSize: number,
): TimelineEvent[] {
  if (eventViewMode === "recent") {
    return events.slice(0, pageSize);
  }

  const pageCount = historyPageCount(events.length, pageSize);
  const page = boundedEventHistoryPage(eventHistoryPage, pageCount);
  const start = page * pageSize;
  return events.slice(start, start + pageSize);
}

export function historyPageCount(eventCount: number, pageSize: number): number {
  return Math.max(1, Math.ceil(eventCount / pageSize));
}

export function boundedEventHistoryPage(
  eventHistoryPage: number,
  pageCount: number,
): number {
  return Math.min(eventHistoryPage, Math.max(pageCount - 1, 0));
}

export function overviewLayoutForCount(tileCount: number): SurveillanceOverviewLayout {
  if (tileCount <= 1) {
    return { name: "1x1", columns: 1, rows: 1 };
  }
  if (tileCount <= 2) {
    return { name: "2x1", columns: 2, rows: 1 };
  }
  if (tileCount <= 4) {
    return { name: "2x2", columns: 2, rows: 2 };
  }
  if (tileCount <= 6) {
    return { name: "3x2", columns: 3, rows: 2 };
  }
  if (tileCount <= 9) {
    return { name: "3x3", columns: 3, rows: 3 };
  }
  if (tileCount <= 12) {
    return { name: "4x3", columns: 4, rows: 3 };
  }
  return { name: "4x4", columns: 4, rows: 4 };
}

export function defaultTimelineEventFilters(): TimelineEventFilters {
  return {
    eventCode: EVENT_FILTER_ALL,
  };
}

export function filterTimelineEvents(
  events: TimelineEvent[],
  filters: TimelineEventFilters,
): TimelineEvent[] {
  return events.filter((event) => {
    if (
      filters.eventCode !== EVENT_FILTER_ALL &&
      normalizeEventCode(event.sourceCode ?? event.title) !== filters.eventCode
    ) {
      return false;
    }
    return true;
  });
}

export function buildTimelineEventFilterOptions(
  events: TimelineEvent[],
): TimelineEventFilterOptions {
  const eventCodes = new Set<string>();

  for (const event of events) {
    eventCodes.add(normalizeEventCode(event.sourceCode ?? event.title));
  }

  return {
    eventCodes: [
      { value: EVENT_FILTER_ALL, label: "All event codes" },
      ...sortStrings(eventCodes).map((value) => ({
        value,
        label: humanizeEventCode(value),
      })),
    ],
  };
}

function normalizeEventCode(value: string): string {
  const normalized = value.trim().toLowerCase();
  switch (normalized) {
    case "videomotion":
      return "motion";
    case "smartmotionhuman":
      return "human";
    case "smartmotionvehicle":
      return "vehicle";
    case "crosslinedetection":
      return "tripwire";
    case "crossregiondetection":
      return "intrusion";
    case "accessctl":
    case "accesscontrol":
      return "access";
    case "call_started":
      return "call";
    default:
      return normalized.replace(/[^a-z0-9]+/g, "_");
  }
}

function humanizeEventCode(value: string): string {
  return value
    .replace(/_/g, " ")
    .replace(/\b\w/g, (match) => match.toUpperCase());
}

function sortStrings(values: Set<string>): string[] {
  return [...values].filter(Boolean).sort((left, right) => left.localeCompare(right));
}
