import { describe, expect, it } from "vitest";

import {
  buildTodayEventHeaderMetrics,
  findPanelCameraEventSummary,
  summarizePanelTodayEvents,
} from "../src/domain/event-summary";

describe("event summary", () => {
  it("aggregates rolling NVR summary payloads into header metrics and per-camera counts", () => {
    const summary = summarizePanelTodayEvents("2026-05-01T10:00:00Z", "2026-05-02T10:00:00Z", [
      {
        deviceId: "west20_nvr",
        startTime: "2026-05-01T10:00:00Z",
        endTime: "2026-05-02T10:00:00Z",
        totalCount: 5,
        items: [
          { code: "smdTypeHuman", label: "Human", count: 2 },
          { code: "CrossLineDetection", label: "Cross Line", count: 1 },
        ],
        channels: [
          {
            channel: 1,
            totalCount: 3,
            items: [
              { code: "smdTypeHuman", label: "Human", count: 2 },
              { code: "CrossLineDetection", label: "Cross Line", count: 1 },
            ],
          },
        ],
      },
      {
        deviceId: "garage_nvr",
        startTime: "2026-05-01T10:00:00Z",
        endTime: "2026-05-02T10:00:00Z",
        totalCount: 4,
        items: [
          { code: "vehicle", label: "Vehicle", count: 3 },
          { code: "MoveDetection", label: "Motion Detection", count: 1 },
        ],
        channels: [
          {
            channel: 7,
            totalCount: 4,
            items: [
              { code: "vehicle", label: "Vehicle", count: 3 },
              { code: "MoveDetection", label: "Motion Detection", count: 1 },
            ],
          },
        ],
      },
    ]);

    expect(summary).toEqual({
      windowStart: "2026-05-01T10:00:00Z",
      windowEnd: "2026-05-02T10:00:00Z",
      totalCount: 9,
      humanCount: 2,
      vehicleCount: 3,
      animalCount: 0,
      ivsCount: 2,
      cameras: [
        {
          rootDeviceId: "garage_nvr",
          channel: 7,
          totalCount: 4,
          humanCount: 0,
          vehicleCount: 3,
          animalCount: 0,
          ivsCount: 1,
        },
        {
          rootDeviceId: "west20_nvr",
          channel: 1,
          totalCount: 3,
          humanCount: 2,
          vehicleCount: 0,
          animalCount: 0,
          ivsCount: 1,
        },
      ],
    });
    expect(buildTodayEventHeaderMetrics(summary)).toEqual([
      { label: "Events 24H", value: "9", tone: "warning" },
      { label: "Human", value: "2", tone: "info" },
      { label: "Vehicle", value: "3", tone: "info" },
      { label: "IVS", value: "2", tone: "warning" },
    ]);
    expect(findPanelCameraEventSummary(summary, "west20_nvr", 1)).toEqual({
      rootDeviceId: "west20_nvr",
      channel: 1,
      totalCount: 3,
      humanCount: 2,
      vehicleCount: 0,
      animalCount: 0,
      ivsCount: 1,
    });
    expect(findPanelCameraEventSummary(summary, "garage_nvr", 2)).toBeNull();
  });

  it("returns no header metrics when no summary is available", () => {
    expect(buildTodayEventHeaderMetrics(null)).toBeNull();
  });
});
