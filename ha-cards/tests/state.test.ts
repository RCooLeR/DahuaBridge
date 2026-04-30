import { describe, expect, it } from "vitest";

import type { TimelineEvent } from "../src/domain/events";
import {
  buildTimelineEventFilterOptions,
  defaultTimelineEventFilters,
  filterTimelineEvents,
} from "../src/cards/surveillance-panel-state";

const BASE_EVENT: TimelineEvent = {
  id: "event-1",
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
  timestamp: Date.now(),
};

describe("timeline event filters", () => {
  it("filters events by event code only", () => {
    const events: TimelineEvent[] = [
      BASE_EVENT,
      {
        ...BASE_EVENT,
        id: "event-2",
        deviceId: "west20_ipc_01",
        rootDeviceId: "west20_ipc_01",
        roomLabel: "Garage",
        deviceKind: "ipc",
        severity: "info",
        sourceCode: "human",
        title: "Human",
      },
    ];

    const filters = defaultTimelineEventFilters();
    filters.eventCode = "human";

    const filtered = filterTimelineEvents(events, filters);
    expect(filtered).toHaveLength(1);
    expect(filtered[0]?.id).toBe("event-2");
  });

  it("builds filter options from available timeline metadata", () => {
    const options = buildTimelineEventFilterOptions([
      BASE_EVENT,
      {
        ...BASE_EVENT,
        id: "event-2",
        roomLabel: null,
        deviceKind: "vto",
        severity: "critical",
        sourceCode: "tamper",
        title: "Tamper",
      },
    ]);

    expect(options.eventCodes.some((option) => option.value === "motion")).toBe(true);
    expect(options.eventCodes.some((option) => option.value === "tamper")).toBe(true);
  });
});
