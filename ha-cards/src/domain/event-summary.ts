import type { HeaderMetric } from "./model";

export interface NvrEventSummaryCountModel {
  code: string;
  label: string;
  count: number;
}

export interface NvrEventChannelSummaryModel {
  channel: number;
  totalCount: number;
  items: NvrEventSummaryCountModel[];
}

export interface NvrEventSummaryModel {
  deviceId: string;
  startTime: string;
  endTime: string;
  totalCount: number;
  items: NvrEventSummaryCountModel[];
  channels: NvrEventChannelSummaryModel[];
}

export interface PanelTodayEventSummaryModel {
  date: string;
  totalCount: number;
  humanCount: number;
  vehicleCount: number;
  animalCount: number;
  ivsCount: number;
}

export function summarizePanelTodayEvents(
  date: string,
  summaries: readonly NvrEventSummaryModel[],
): PanelTodayEventSummaryModel {
  let totalCount = 0;
  let humanCount = 0;
  let vehicleCount = 0;
  let animalCount = 0;
  let ivsCount = 0;

  for (const summary of summaries) {
    totalCount += summary.totalCount;
    for (const item of summary.items) {
      switch (normalizeSummaryCode(item.code)) {
        case "smdtypehuman":
        case "human":
        case "smartmotionhuman":
          humanCount += item.count;
          break;
        case "smdtypevehicle":
        case "vehicle":
        case "smartmotionvehicle":
          vehicleCount += item.count;
          break;
        case "smdtypeanimal":
        case "animal":
          animalCount += item.count;
          break;
        case "crosslinedetection":
        case "crossregiondetection":
        case "leftdetection":
        case "movedetection":
          ivsCount += item.count;
          break;
      }
    }
  }

  return {
    date,
    totalCount,
    humanCount,
    vehicleCount,
    animalCount,
    ivsCount,
  };
}

export function buildTodayEventHeaderMetrics(
  summary: PanelTodayEventSummaryModel | null,
): HeaderMetric[] | null {
  if (!summary) {
    return null;
  }

  return [
    {
      label: "Events Today",
      value: String(summary.totalCount),
      tone: summary.totalCount > 0 ? "warning" : "neutral",
    },
    {
      label: "Human",
      value: String(summary.humanCount),
      tone: summary.humanCount > 0 ? "info" : "neutral",
    },
    {
      label: "Vehicle",
      value: String(summary.vehicleCount),
      tone: summary.vehicleCount > 0 ? "info" : "neutral",
    },
    {
      label: "IVS",
      value: String(summary.ivsCount),
      tone: summary.ivsCount > 0 ? "warning" : "neutral",
    },
  ];
}

function normalizeSummaryCode(value: string): string {
  return value.trim().toLowerCase();
}
