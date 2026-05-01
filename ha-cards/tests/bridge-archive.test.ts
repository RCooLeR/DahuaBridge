import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchArchiveRecordings, fetchBridgeRecordings } from "../src/ha/bridge-archive";

describe("bridge archive", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("parses alternate event-array archive payloads", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        event: [
          {
            Channel: 8,
            StartTime: "2026-05-01T10:00:00Z",
            EndTime: "2026-05-01T10:00:10Z",
            FilePath: "/mnt/dvr/2026-05-01/8/10.00.00-10.00.10.dav",
            Type: "Event",
            VideoStream: "Main",
            Flags: ["Event", "Human"],
            Length: 1234,
          },
        ],
      }),
    } as Response);

    Object.defineProperty(globalThis, "window", {
      configurable: true,
      value: {
        location: {
          origin: "https://ha.example.com",
        },
      },
    });

    const result = await fetchArchiveRecordings(
      "/api/v1/nvr/west20_nvr/channels/8/archive/search",
      {
        channel: 8,
        startTime: "2026-05-01T00:00:00Z",
        endTime: "2026-05-02T00:00:00Z",
        limit: 100,
        eventCode: "human",
      },
    );

    expect(result.channel).toBe(8);
    expect(result.returnedCount).toBe(1);
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain("event=human");
    expect(result.items).toHaveLength(1);
    expect(result.items[0]).toMatchObject({
      channel: 8,
      startTime: "2026-05-01T10:00:00Z",
      endTime: "2026-05-01T10:00:10Z",
      filePath: "/mnt/dvr/2026-05-01/8/10.00.00-10.00.10.dav",
      type: "Event",
      videoStream: "Main",
      lengthBytes: 1234,
    });
    expect(result.items[0]?.flags).toEqual(["Event", "Human"]);
  });

  it("marks all-event archive searches as event-only", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        items: [],
      }),
    } as Response);

    Object.defineProperty(globalThis, "window", {
      configurable: true,
      value: {
        location: {
          origin: "https://ha.example.com",
        },
      },
    });

    await fetchArchiveRecordings(
      "/api/v1/nvr/west20_nvr/recordings",
      {
        channel: 1,
        startTime: "2026-05-01T00:00:00Z",
        endTime: "2026-05-02T00:00:00Z",
        limit: 100,
        eventCode: "__all__",
        eventOnly: true,
      },
    );

    const requestedUrl = new URL(String(fetchMock.mock.calls[0]?.[0]));
    expect(requestedUrl.searchParams.get("event_only")).toBe("true");
    expect(requestedUrl.searchParams.get("event")).toBe("all");
  });

  it("normalizes archive search templates before adding live query parameters", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        items: [],
      }),
    } as Response);

    Object.defineProperty(globalThis, "window", {
      configurable: true,
      value: {
        location: {
          origin: "https://ha.example.com",
        },
      },
    });

    await fetchArchiveRecordings(
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/recordings?channel={channel}&start={start}&end={end}&limit={limit}&event={event}",
      {
        channel: 1,
        startTime: "2026-05-01T00:00:00Z",
        endTime: "2026-05-02T00:00:00Z",
        limit: 100,
        eventCode: "human",
      },
    );

    const requestedUrl = new URL(String(fetchMock.mock.calls[0]?.[0]));
    expect(requestedUrl.searchParams.getAll("channel")).toEqual(["1"]);
    expect(requestedUrl.searchParams.getAll("start")).toEqual(["2026-05-01T00:00:00Z"]);
    expect(requestedUrl.searchParams.getAll("end")).toEqual(["2026-05-02T00:00:00Z"]);
    expect(requestedUrl.searchParams.getAll("limit")).toEqual(["100"]);
    expect(requestedUrl.searchParams.getAll("event")).toEqual(["human"]);
  });

  it("accepts null item arrays from empty event searches", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        device_id: "west20_nvr",
        channel: 1,
        returned_count: 0,
        items: null,
        event: null,
        events: null,
        recordings: null,
      }),
    } as Response);

    Object.defineProperty(globalThis, "window", {
      configurable: true,
      value: {
        location: {
          origin: "https://ha.example.com",
        },
      },
    });

    const result = await fetchArchiveRecordings(
      "/api/v1/nvr/west20_nvr/recordings",
      {
        channel: 1,
        startTime: "2026-05-01T00:00:00Z",
        endTime: "2026-05-02T00:00:00Z",
        limit: 100,
        eventOnly: true,
        eventCode: "__all__",
      },
    );

    expect(result.returnedCount).toBe(0);
    expect(result.items).toEqual([]);
  });

  it("parses bridge mp4 playback urls", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        returned_count: 1,
        items: [
          {
            id: "clip_test",
            stream_id: "nvrpb_clip_test",
            source_device_id: "west20_nvr_channel_01",
            root_device_id: "west20_nvr",
            channel: 1,
            status: "completed",
            started_at: "2026-05-01T10:00:00Z",
            ended_at: "2026-05-01T10:00:04Z",
            start_time: "2026-05-01T20:12:05Z",
            end_time: "2026-05-01T20:12:25Z",
            playback_url: "http://bridge.local:9205/api/v1/media/recordings/clip_test/play",
            download_url: "http://bridge.local:9205/api/v1/media/recordings/clip_test/download",
          },
        ],
      }),
    } as Response);

    Object.defineProperty(globalThis, "window", {
      configurable: true,
      value: {
        location: {
          origin: "https://ha.example.com",
        },
      },
    });

    const result = await fetchBridgeRecordings(
      "http://bridge.local:9205/api/v1/media/recordings?channel=1&root_device_id=west20_nvr",
      {
        startTime: "2026-05-01T00:00:00Z",
        endTime: "2026-05-02T00:00:00Z",
        limit: 100,
      },
    );

    expect(String(fetchMock.mock.calls[0]?.[0])).toContain("root_device_id=west20_nvr");
    expect(result.returnedCount).toBe(1);
    expect(result.items[0]).toMatchObject({
      id: "clip_test",
      streamId: "nvrpb_clip_test",
      sourceStartTime: "2026-05-01T20:12:05Z",
      sourceEndTime: "2026-05-01T20:12:25Z",
      playbackUrl: "http://bridge.local:9205/api/v1/media/recordings/clip_test/play",
      downloadUrl: "http://bridge.local:9205/api/v1/media/recordings/clip_test/download",
    });
  });
});
