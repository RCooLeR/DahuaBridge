import { describe, expect, it } from "vitest";

import {
  bridgeEventsToTimeline,
  collectCameraEvents,
  mergeTimelineEvents,
} from "../src/domain/events";
import type { BridgeEvent } from "../src/ha/bridge-events";
import type { HomeAssistant } from "../src/types/home-assistant";

describe("collectCameraEvents", () => {
  it("returns active detections", () => {
    const hass: HomeAssistant = {
      states: {
        "binary_sensor.west20_nvr_channel_01_motion": {
          entity_id: "binary_sensor.west20_nvr_channel_01_motion",
          state: "on",
          attributes: {},
          last_changed: new Date().toISOString(),
          last_updated: new Date().toISOString(),
        },
      },
      callService: async () => undefined,
    };

    const events = collectCameraEvents(
      hass,
      "west20_nvr_channel_01",
      "Entrance Gate",
      "Front Gate",
      "nvr_channel",
      60_000,
    );

    expect(events).toHaveLength(1);
    expect(events[0]?.title).toBe("Motion");
    expect(events[0]?.active).toBe(true);
    expect(events[0]?.roomLabel).toBe("Front Gate");
    expect(events[0]?.deviceKind).toBe("nvr_channel");
  });

  it("maps bridge events into timeline cards", () => {
    const events: BridgeEvent[] = [
      {
        device_id: "west20_nvr",
        device_kind: "nvr",
        child_id: "west20_nvr_channel_01",
        code: "VideoMotion",
        action: "Start",
        channel: 1,
        occurred_at: new Date().toISOString(),
      },
      {
        device_id: "front_vto",
        device_kind: "vto",
        code: "Call",
        action: "Stop",
        occurred_at: new Date().toISOString(),
      },
    ];

    const timeline = bridgeEventsToTimeline(
      events,
      new Map([
        [
          "west20_nvr_channel_01",
          { label: "Entrance Gate", roomLabel: "Front Gate", deviceKind: "nvr_channel" },
        ],
        ["front_vto", { label: "Front VTO", roomLabel: "Entry", deviceKind: "vto" }],
      ]),
      60_000,
    );

    expect(timeline).toHaveLength(2);
    expect(timeline[0]?.context).toBeTruthy();
    expect(timeline.some((event) => event.title === "Motion")).toBe(true);
    expect(timeline.some((event) => event.title === "Call Ended")).toBe(true);
    expect(timeline.find((event) => event.title === "Motion")?.details[0]?.label).toBe("Channel");
    expect(timeline.find((event) => event.title === "Motion")?.roomLabel).toBe("Front Gate");
  });

  it("merges synthetic and bridge events into one deduplicated timeline", () => {
    const timestamp = Date.parse("2026-04-29T12:00:00.000Z");
    const merged = mergeTimelineEvents(
      [
        {
          id: "bridge-motion",
          deviceId: "west20_nvr_channel_01",
          rootDeviceId: "west20_nvr",
          title: "Motion",
          context: "Entrance Gate",
          roomLabel: "Front Gate",
          deviceKind: "nvr_channel",
          icon: "mdi:motion-sensor",
          severity: "warning",
          active: true,
          statusText: "Active now",
          actionText: "State",
          sourceCode: "VideoMotion",
          details: [{ label: "Channel", value: "1" }],
          timestamp,
        },
      ],
      [
        {
          id: "ha-motion",
          deviceId: "west20_nvr_channel_01",
          rootDeviceId: "west20_nvr",
          title: "Motion",
          context: "Entrance Gate",
          roomLabel: "Front Gate",
          deviceKind: "nvr_channel",
          icon: "mdi:motion-sensor",
          severity: "warning",
          active: true,
          statusText: "Active now",
          actionText: "State",
          sourceCode: "motion",
          details: [],
          timestamp: timestamp + 10_000,
        },
      ],
    );

    expect(merged).toHaveLength(1);
    expect(merged[0]?.details[0]?.label).toBe("Channel");
  });
});
