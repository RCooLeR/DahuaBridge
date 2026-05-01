import { describe, expect, it } from "vitest";

import {
  buildTodayEventHeaderMetrics,
  summarizePanelTodayEvents,
} from "../src/domain/event-summary";

describe("event summary", () => {
  it("aggregates daily NVR summary payloads into header metrics", () => {
    const summary = summarizePanelTodayEvents("2026-05-01", [
      {
        deviceId: "west20_nvr",
        startTime: "2026-05-01T00:00:00Z",
        endTime: "2026-05-02T00:00:00Z",
        totalCount: 5,
        items: [
          { code: "smdTypeHuman", label: "Human", count: 2 },
          { code: "CrossLineDetection", label: "Cross Line", count: 1 },
        ],
        channels: [],
      },
      {
        deviceId: "garage_nvr",
        startTime: "2026-05-01T00:00:00Z",
        endTime: "2026-05-02T00:00:00Z",
        totalCount: 4,
        items: [
          { code: "vehicle", label: "Vehicle", count: 3 },
          { code: "MoveDetection", label: "Motion Detection", count: 1 },
        ],
        channels: [],
      },
    ]);

    expect(summary).toEqual({
      date: "2026-05-01",
      totalCount: 9,
      humanCount: 2,
      vehicleCount: 3,
      animalCount: 0,
      ivsCount: 2,
    });
    expect(buildTodayEventHeaderMetrics(summary)).toEqual([
      { label: "Events Today", value: "9", tone: "warning" },
      { label: "Human", value: "2", tone: "info" },
      { label: "Vehicle", value: "3", tone: "info" },
      { label: "IVS", value: "2", tone: "warning" },
    ]);
  });

  it("returns no header metrics when no summary is available", () => {
    expect(buildTodayEventHeaderMetrics(null)).toBeNull();
  });
});
