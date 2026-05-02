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

export interface PanelCameraEventSummaryModel {
  rootDeviceId: string;
  channel: number;
  totalCount: number;
  humanCount: number;
  vehicleCount: number;
  animalCount: number;
  ivsCount: number;
}

export interface PanelTodayEventSummaryModel {
  windowStart: string;
  windowEnd: string;
  totalCount: number;
  humanCount: number;
  vehicleCount: number;
  animalCount: number;
  ivsCount: number;
  cameras: PanelCameraEventSummaryModel[];
}

export function summarizePanelTodayEvents(
  windowStart: string,
  windowEnd: string,
  summaries: readonly NvrEventSummaryModel[],
): PanelTodayEventSummaryModel {
  let totalCount = 0;
  let humanCount = 0;
  let vehicleCount = 0;
  let animalCount = 0;
  let ivsCount = 0;
  const cameras = new Map<string, PanelCameraEventSummaryModel>();

  for (const summary of summaries) {
    totalCount += summary.totalCount;
    for (const item of summary.items) {
      const counts = summaryCategoryCounts(item.code, item.count);
      humanCount += counts.humanCount;
      vehicleCount += counts.vehicleCount;
      animalCount += counts.animalCount;
      ivsCount += counts.ivsCount;
    }

    for (const channel of summary.channels) {
      const key = `${summary.deviceId}:${channel.channel}`;
      const camera = cameras.get(key) ?? {
        rootDeviceId: summary.deviceId,
        channel: channel.channel,
        totalCount: 0,
        humanCount: 0,
        vehicleCount: 0,
        animalCount: 0,
        ivsCount: 0,
      };
      camera.totalCount += channel.totalCount;
      for (const item of channel.items) {
        const counts = summaryCategoryCounts(item.code, item.count);
        camera.humanCount += counts.humanCount;
        camera.vehicleCount += counts.vehicleCount;
        camera.animalCount += counts.animalCount;
        camera.ivsCount += counts.ivsCount;
      }
      cameras.set(key, camera);
    }
  }

  return {
    windowStart,
    windowEnd,
    totalCount,
    humanCount,
    vehicleCount,
    animalCount,
    ivsCount,
    cameras: [...cameras.values()].sort((left, right) => (
      left.rootDeviceId.localeCompare(right.rootDeviceId) ||
      left.channel - right.channel
    )),
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
      label: "Events 24H",
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

export function findPanelCameraEventSummary(
  summary: PanelTodayEventSummaryModel | null | undefined,
  rootDeviceId: string,
  channel: number | null,
): PanelCameraEventSummaryModel | null {
  if (!summary || !rootDeviceId.trim() || !channel || channel <= 0) {
    return null;
  }

  return summary.cameras.find(
    (item) => item.rootDeviceId === rootDeviceId && item.channel === channel,
  ) ?? null;
}

function normalizeSummaryCode(value: string): string {
  return value.trim().toLowerCase();
}

function summaryCategoryCounts(
  code: string,
  count: number,
): Pick<PanelCameraEventSummaryModel, "humanCount" | "vehicleCount" | "animalCount" | "ivsCount"> {
  switch (normalizeSummaryCode(code)) {
    case "smdtypehuman":
    case "human":
    case "smartmotionhuman":
      return { humanCount: count, vehicleCount: 0, animalCount: 0, ivsCount: 0 };
    case "smdtypevehicle":
    case "vehicle":
    case "smartmotionvehicle":
      return { humanCount: 0, vehicleCount: count, animalCount: 0, ivsCount: 0 };
    case "smdtypeanimal":
    case "animal":
      return { humanCount: 0, vehicleCount: 0, animalCount: count, ivsCount: 0 };
    case "crosslinedetection":
    case "crossregiondetection":
    case "leftdetection":
    case "movedetection":
      return { humanCount: 0, vehicleCount: 0, animalCount: 0, ivsCount: count };
    default:
      return { humanCount: 0, vehicleCount: 0, animalCount: 0, ivsCount: 0 };
  }
}
