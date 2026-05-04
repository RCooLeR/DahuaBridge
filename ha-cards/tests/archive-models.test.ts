import {describe, expect, it} from "vitest";

import {
    archiveMissingUrlMessage,
    archiveModeForSelection,
    archiveRecordingsForMode,
    archiveUrlForMode,
} from "../src/cards/surveillance-panel-archive-state-model";
import {
    archiveSeekDateTimeFromSecond,
    buildArchiveTimeframeProxyPath,
    buildArchiveTimeframeSnapshotProxyPath,
    parseArchiveSeekSecond,
} from "../src/cards/surveillance-panel-archive-seek-model";
import {
    createSelectedNativePlaybackState,
    isArchiveEventRecording,
    nativePlaybackMatchesRecording,
} from "../src/cards/surveillance-panel-native-playback-model";
import type {NvrArchiveRecordingModel, NvrArchiveSearchResultModel} from "../src/domain/archive";
import type {CameraViewModel} from "../src/domain/model";

describe("archive list models", () => {
    it("separates SMD/IVS event mode from recording chunk mode", () => {
        expect(archiveModeForSelection("camera", "events")).toBe("events");
        expect(archiveModeForSelection("camera", "recordings")).toBe("chunks");
        expect(archiveModeForSelection("nvr", "events")).toBe("chunks");
    });

    it("selects the bridge URL for each archive list model", () => {
        const camera = {
            archive: {
                smdIvsUrl: "http://bridge.local/archive/smd-ivs",
                chunksUrl: "http://bridge.local/archive/chunks",
            },
        } as CameraViewModel;

        expect(archiveUrlForMode(camera, "events")).toBe("http://bridge.local/archive/smd-ivs");
        expect(archiveUrlForMode(camera, "chunks")).toBe("http://bridge.local/archive/chunks");
        expect(archiveMissingUrlMessage("events")).toContain("SMD/IVS");
        expect(archiveMissingUrlMessage("chunks")).toContain("chunk");
    });

    it("only exposes loaded recordings for the active archive mode", () => {
        const recordings = {items: []} as unknown as NvrArchiveSearchResultModel;

        expect(archiveRecordingsForMode(recordings, "events", "events")).toBe(recordings);
        expect(archiveRecordingsForMode(recordings, "chunks", "events")).toBeNull();
    });
});

describe("archive seek model", () => {
    it("bounds seek seconds to one local day", () => {
        expect(parseArchiveSeekSecond("-1")).toBe(0);
        expect(parseArchiveSeekSecond("90000")).toBe(86_399);
    });

    it("builds local archive seek timestamps from date input seconds", () => {
        const seekTime = archiveSeekDateTimeFromSecond("2026-05-01", 3661);

        expect(seekTime?.getFullYear()).toBe(2026);
        expect(seekTime?.getMonth()).toBe(4);
        expect(seekTime?.getDate()).toBe(1);
        expect(seekTime?.getHours()).toBe(1);
        expect(seekTime?.getMinutes()).toBe(1);
        expect(seekTime?.getSeconds()).toBe(1);
    });

    it("builds Home Assistant timeframe camera proxy paths", () => {
        const path = buildArchiveTimeframeProxyPath(
            "camera.front_door",
            new Date(2026, 4, 1, 10, 15, 30),
            new Date(2026, 4, 1, 10, 45, 30),
            "quality",
        );

        expect(path).toBe(
            "/api/camera_proxy/camera.front_door/timeframe/?starttime=2026_05_01_10_15_30&endtime=2026_05_01_10_45_30&profile=quality",
        );
    });

    it("builds timestamped Home Assistant timeframe snapshot proxy paths", () => {
        const path = buildArchiveTimeframeSnapshotProxyPath(
            "camera.front_door",
            new Date(2026, 4, 1, 10, 15, 30),
            new Date(2026, 4, 1, 10, 45, 30),
            new Date(2026, 4, 1, 10, 20, 5),
            "quality",
        );

        expect(path).toBe(
            "/api/camera_proxy/camera.front_door/timeframe/snapshot/?starttime=2026_05_01_10_15_30&endtime=2026_05_01_10_45_30&seektime=2026_05_01_10_20_05&profile=quality",
        );
    });

});

describe("native archive playback model", () => {
    it("identifies SMD/IVS event rows for timeframe proxy playback", () => {
        expect(isArchiveEventRecording({recordKind: "smd_ivs"} as NvrArchiveRecordingModel)).toBe(true);
        expect(isArchiveEventRecording({recordKind: "chunk", type: "Regular"} as NvrArchiveRecordingModel)).toBe(false);
    });

    it("creates and matches selected native playback state", () => {
        const camera = {deviceId: "channel-1"} as CameraViewModel;
        const playback = createSelectedNativePlaybackState(
            camera,
            "/api/camera_proxy/camera.channel_1/timeframe/",
            "2026-05-01T10:00:00.000Z",
            "2026-05-01T10:30:00.000Z",
            "2026-05-01T10:00:00.000Z",
            "quality",
        );

        expect(playback.sourceDeviceId).toBe("channel-1");
        expect(nativePlaybackMatchesRecording(playback, {
            startTime: "2026-05-01T10:00:00.000Z",
            endTime: "2026-05-01T10:30:00.000Z",
        } as NvrArchiveRecordingModel)).toBe(true);
    });
});
