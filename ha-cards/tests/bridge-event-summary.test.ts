import { afterEach, describe, expect, it, vi } from "vitest";

import {
  buildNvrEventSummaryUrl,
  fetchNvrEventSummary,
} from "../src/ha/bridge-event-summary";

describe("bridge event summary", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("fetches and maps NVR event summaries", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        device_id: "west20_nvr",
        start_time: "2026-05-01T00:00:00Z",
        end_time: "2026-05-02T00:00:00Z",
        total_count: 3,
        items: [
          { code: "smdTypeHuman", label: "Human", count: 2 },
          { code: "CrossLineDetection", label: null, count: 1 },
        ],
        channels: [
          {
            channel: 1,
            total_count: 3,
            items: [
              { code: "smdTypeHuman", label: "Human", count: 2 },
              { code: "CrossLineDetection", label: "Cross Line", count: 1 },
            ],
          },
        ],
      }),
    } as Response);

    const result = await fetchNvrEventSummary(
      "https://ha.example.com/bridge/api/v1/nvr/west20_nvr/events/summary",
      {
        startTime: "2026-05-01T00:00:00Z",
        endTime: "2026-05-02T00:00:00Z",
        eventCode: " all ",
      },
    );

    const requestedUrl = new URL(String(fetchMock.mock.calls[0]?.[0]));
    expect(requestedUrl.searchParams.get("start")).toBe("2026-05-01T00:00:00Z");
    expect(requestedUrl.searchParams.get("end")).toBe("2026-05-02T00:00:00Z");
    expect(requestedUrl.searchParams.get("event")).toBe("all");
    expect(result).toEqual({
      deviceId: "west20_nvr",
      startTime: "2026-05-01T00:00:00Z",
      endTime: "2026-05-02T00:00:00Z",
      totalCount: 3,
      items: [
        { code: "smdTypeHuman", label: "Human", count: 2 },
        { code: "CrossLineDetection", label: "CrossLineDetection", count: 1 },
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
    });
  });

  it("builds browser-facing summary URLs only when both parts are available", () => {
    expect(buildNvrEventSummaryUrl("https://ha.example.com/bridge", "west20_nvr")).toBe(
      "https://ha.example.com/bridge/api/v1/nvr/west20_nvr/events/summary",
    );
    expect(buildNvrEventSummaryUrl(null, "west20_nvr")).toBeNull();
    expect(buildNvrEventSummaryUrl("https://ha.example.com/bridge", "   ")).toBeNull();
  });
});
