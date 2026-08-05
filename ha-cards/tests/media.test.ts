import { describe, expect, it } from "vitest";

import {
  availableCameraViewportSources,
  availablePlaybackViewportSources,
  buildRtspPlaybackUrl,
  cameraImageSrc,
  defaultOverviewStreamProfileKey,
  defaultSelectedStreamProfileKey,
  preserveCameraViewportSourceSelection,
  preserveCameraViewportSourceSelectionOnProfileChange,
  resolveInitialPlaybackViewportSource,
  resolveBridgeFirstStreamViewportSource,
  resolveOverviewCameraViewportSource,
  resolvePreferredCameraViewportSource,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  resolvePlaybackViewportSource,
  resolveStreamViewportSource,
  vtoPreviewImageSrc,
  type CameraViewportSource,
} from "../src/cards/surveillance-panel-media";
import { selectedCameraLiveStreamModel } from "../src/cards/surveillance-panel-live-stream-model";
import type { CameraViewModel } from "../src/domain/model";

function buildCamera(overrides: Partial<CameraViewModel> = {}): CameraViewModel {
  return {
    type: "camera",
    deviceKind: "nvr_channel",
    kindLabel: "NVR Channel",
    deviceId: "west20_nvr_channel_01",
    rootDeviceId: "west20_nvr",
    channelNumber: 1,
    label: "Channel 1",
    roomLabel: "Entrance",
    cameraEntityId: "camera.west20_nvr_channel_01_camera",
    online: true,
    streamAvailable: true,
    bridgeBaseUrl: "http://bridge.local:9020",
    eventsUrl: null,
    snapshotUrl: null,
    captureSnapshotUrl: null,
    stream: {
      available: true,
      source: "http://bridge.local:9020/api/v1/media/hls/west20_nvr_channel_01/quality",
      snapshotUrl: null,
      localIntercomUrl: null,
      onvifStreamUrl: null,
      onvifSnapshotUrl: null,
      recommendedProfile: "quality",
      recommendedHaIntegration: "bridge_media",
      preferredVideoProfile: "quality",
      preferredVideoSource: null,
      resolution: "",
      codec: "",
      frameRate: "",
      bitrate: "",
      profile: "",
      audioCodec: "",
      profiles: [
        {
          key: "quality",
          name: "Quality",
          streamUrl: null,
          localMjpegUrl: "http://bridge.local:9020/api/v1/media/mjpeg/west20_nvr_channel_01/quality",
          localHlsUrl: "http://bridge.local:9020/api/v1/media/hls/west20_nvr_channel_01/quality",
          localDashUrl: null,
          localWebRtcUrl: null,
          subtype: 0,
          rtspTransport: "tcp",
          frameRate: 25,
          resolution: "2560x1440",
          recommended: true,
        },
        {
          key: "stable",
          name: "Stable",
          streamUrl: null,
          localMjpegUrl: "http://bridge.local:9020/api/v1/media/mjpeg/west20_nvr_channel_01/stable",
          localHlsUrl: null,
          localDashUrl: null,
          localWebRtcUrl: null,
          subtype: 1,
          rtspTransport: "tcp",
          frameRate: 12,
          resolution: "1280x720",
          recommended: false,
        },
      ],
    },
    detections: [],
    supportsPtz: false,
    supportsPtzPan: false,
    supportsPtzTilt: false,
    supportsPtzZoom: false,
    supportsPtzFocus: false,
    supportsAux: false,
    supportsRecording: false,
    recordingActive: false,
    bridgeRecordingActive: false,
    ptzUrl: null,
    aux: null,
    auxUrl: null,
    archive: null,
    recording: null,
    recordingUrl: null,
    recordingStartUrl: null,
    recordingStopUrl: null,
    recordingsUrl: null,
    resolution: "",
    codec: "",
    frameRate: "",
    bitrate: "",
    profile: "",
    audioCodec: "",
    microphoneAvailable: false,
    speakerAvailable: false,
    audioMuteSupported: false,
    validationNotes: [],
    nvrConfigWritable: null,
    nvrConfigReason: null,
    directIPCConfigured: false,
    directIPCConfiguredIP: null,
    directIPCIP: null,
    directIPCModel: null,
    eventCount24h: 0,
    humanCount24h: 0,
    vehicleCount24h: 0,
    ivsCount24h: 0,
    ...overrides,
  };
}

describe("camera media helpers", () => {
  it("prefers the configured or recommended profile", () => {
    const camera = buildCamera();

    expect(resolveSelectedCameraStreamProfile(camera, null)?.key).toBe("quality");
    expect(resolveSelectedCameraStreamProfile(camera, "stable")?.key).toBe("stable");
  });

  it("lists available viewport sources for the selected profile and falls back when needed", () => {
    const camera = buildCamera({
      cameraEntity: {
        entity_id: "camera.west20_nvr_channel_01_camera",
        state: "streaming",
        attributes: {},
        last_changed: "",
        last_updated: "",
      },
    });

    expect(availableCameraViewportSources(camera, "quality")).toEqual([
      "native",
      "hls",
      "mjpeg",
    ] satisfies CameraViewportSource[]);
    expect(availableCameraViewportSources(camera, "stable")).toEqual([
      "native",
      "mjpeg",
    ] satisfies CameraViewportSource[]);
    expect(resolveSelectedCameraViewportSource(camera, "hls", "stable")).toBe("mjpeg");
    expect(resolveSelectedCameraViewportSource(camera, "mjpeg", "stable")).toBe("mjpeg");
  });

  it("ignores WebRTC and keeps browser-safe viewport sources", () => {
    const camera = buildCamera({
      stream: {
        ...buildCamera().stream,
        recommendedProfile: "stable",
        preferredVideoProfile: null,
        preferredVideoSource: "webrtc",
        profiles: [
          {
            ...buildCamera().stream.profiles[0]!,
            localWebRtcUrl: "http://bridge.local:9020/api/v1/media/webrtc/west20_nvr_channel_01/quality",
          },
          ...buildCamera().stream.profiles.slice(1),
        ],
      },
    });

    expect(availableCameraViewportSources(camera, "quality")).toEqual([
      "hls",
      "mjpeg",
    ] satisfies CameraViewportSource[]);
    expect(resolveSelectedCameraViewportSource(camera, "mjpeg", "quality")).toBe("mjpeg");
    expect(resolveSelectedCameraViewportSource(camera, null, "quality")).toBe("hls");
  });

  it("prefers low-bandwidth sources for overview tiles", () => {
    const camera = buildCamera({
      stream: {
        ...buildCamera().stream,
        recommendedProfile: "stable",
        preferredVideoProfile: null,
        preferredVideoSource: "webrtc",
        profiles: [
          {
            ...buildCamera().stream.profiles[0]!,
            localWebRtcUrl: "http://bridge.local:9020/api/v1/media/webrtc/west20_nvr_channel_01/quality",
          },
          {
            ...buildCamera().stream.profiles[1]!,
            localHlsUrl: "http://bridge.local:9020/api/v1/media/hls/west20_nvr_channel_01/stable",
          },
        ],
      },
    });

    const overviewProfileKey = defaultOverviewStreamProfileKey(camera.stream);
    expect(overviewProfileKey).toBe("stable");
    expect(resolveOverviewCameraViewportSource(camera, overviewProfileKey)).toBe("hls");
  });

  it("uses the configured overview source before HLS, DASH, and snapshots", () => {
    const camera = buildCamera({
      cameraEntity: {
        entity_id: "camera.west20_nvr_channel_01_camera",
        state: "streaming",
        attributes: {},
        last_changed: "",
        last_updated: "",
      },
      stream: {
        ...buildCamera().stream,
        recommendedHaIntegration: "native",
        preferredVideoSource: "rtsp",
      },
    });

    const overviewProfileKey = defaultOverviewStreamProfileKey(camera.stream);
    expect(resolveOverviewCameraViewportSource(camera, overviewProfileKey)).toBe("native");
    expect(resolvePreferredCameraViewportSource(camera, overviewProfileKey)).toBe("native");

    const hlsPreferredCamera = buildCamera({
      ...camera,
      stream: {
        ...camera.stream,
        preferredVideoSource: "hls",
      },
    });
    expect(resolveOverviewCameraViewportSource(hlsPreferredCamera, overviewProfileKey)).toBe("hls");
    expect(resolvePreferredCameraViewportSource(hlsPreferredCamera, overviewProfileKey)).toBe("hls");
  });

  it("falls back to native overview when it is the only available source", () => {
    const camera = buildCamera({
      cameraEntity: {
        entity_id: "camera.west20_nvr_channel_01_camera",
        state: "streaming",
        attributes: {},
        last_changed: "",
        last_updated: "",
      },
      stream: {
        ...buildCamera().stream,
        profiles: [
          {
            ...buildCamera().stream.profiles[0]!,
            localDashUrl: null,
            localHlsUrl: null,
            localMjpegUrl: null,
          },
        ],
      },
    });

    expect(resolveOverviewCameraViewportSource(camera, "quality")).toBe("native");
  });

  it("uses the configured source for the selected detail player", () => {
    const camera = buildCamera({
      cameraEntity: {
        entity_id: "camera.west20_nvr_channel_01_camera",
        state: "streaming",
        attributes: {},
        last_changed: "",
        last_updated: "",
      },
      stream: {
        ...buildCamera().stream,
        recommendedHaIntegration: "native",
        preferredVideoSource: "rtsp",
      },
    });

    expect(selectedCameraLiveStreamModel(camera, null, null, null).selectedSource).toBe("native");
    expect(selectedCameraLiveStreamModel(camera, null, "hls", null).selectedSource).toBe("hls");
    expect(selectedCameraLiveStreamModel(camera, null, "native", null).selectedSource).toBe("native");
  });

  it("uses configured native RTSP for VTO-style stream source selection", () => {
    const stream = {
      ...buildCamera().stream,
      recommendedHaIntegration: "native",
      preferredVideoSource: "rtsp",
    };

    expect(resolveStreamViewportSource(stream, null, "quality", true)).toBe("native");
  });

  it("prefers bridge media for VTO auto-selection but preserves explicit native", () => {
    const stream = {
      ...buildCamera().stream,
      recommendedHaIntegration: "native",
      preferredVideoSource: "rtsp",
    };
    const overviewProfileKey = defaultOverviewStreamProfileKey(stream);

    expect(resolveBridgeFirstStreamViewportSource(stream, null, overviewProfileKey, true)).toBe(
      "hls",
    );
    expect(
      resolveBridgeFirstStreamViewportSource(stream, "native", overviewProfileKey, true),
    ).toBe("native");
  });

  it("falls back to native for VTO only when no bridge media source exists", () => {
    const stream = {
      ...buildCamera().stream,
      profiles: [
        {
          ...buildCamera().stream.profiles[0]!,
          localDashUrl: null,
          localHlsUrl: null,
          localMjpegUrl: null,
        },
      ],
    };

    expect(resolveBridgeFirstStreamViewportSource(stream, null, "quality", true)).toBe("native");
  });

  it("defaults the selected camera view to the main stream even when stable is recommended", () => {
    const camera = buildCamera({
      stream: {
        ...buildCamera().stream,
        recommendedProfile: "stable",
        preferredVideoProfile: null,
      },
    });

    expect(defaultSelectedStreamProfileKey(camera.stream)).toBe("quality");
  });

  it("defaults the selected camera view to the configured profile when present", () => {
    const camera = buildCamera({
      stream: {
        ...buildCamera().stream,
        recommendedProfile: "quality",
        preferredVideoProfile: "stable",
      },
    });

    expect(defaultSelectedStreamProfileKey(camera.stream)).toBe("stable");
  });

  it("keeps direct native playback audio-visible in the selected camera model", () => {
    const camera = buildCamera();

    expect(
      selectedCameraLiveStreamModel(camera, null, null, {
        sourceDeviceId: camera.deviceId,
        cameraEntityId: camera.cameraEntityId,
        streamSource:
          "rtsp://user:pass@192.0.2.10:554/cam/playback?channel=1&subtype=0&starttime=2026_05_01_10_15_30",
        startTime: "2026-05-01T10:15:30.000Z",
        endTime: "2026-05-01T10:45:30.000Z",
        seekTime: "2026-05-01T10:15:30.000Z",
        profileKey: "quality",
      }).selectedSource,
    ).toBe("native");
  });

  it("preserves an explicit source selection only while it stays valid", () => {
    const camera = buildCamera({
      cameraEntity: {
        entity_id: "camera.west20_nvr_channel_01_camera",
        state: "streaming",
        attributes: {},
        last_changed: "",
        last_updated: "",
      },
    });
    expect(preserveCameraViewportSourceSelection(camera, "quality", "mjpeg")).toBe("mjpeg");
    expect(preserveCameraViewportSourceSelection(camera, "stable", "hls")).toBeNull();
    expect(preserveCameraViewportSourceSelectionOnProfileChange(camera, "stable", "native")).toBe(
      "native",
    );
  });

  it("prefers the bridge snapshot URL over the entity picture fallback", () => {
    expect(
      cameraImageSrc(
        {
          entity_id: "camera.west20_nvr_channel_01_camera",
          state: "streaming",
          attributes: {
            entity_picture: "/api/camera_proxy/camera.west20_nvr_channel_01_camera",
            snapshot_url: "http://bridge.local:9020/api/v1/nvr/west20_nvr/channels/1/snapshot",
          },
          last_changed: "",
          last_updated: "",
        },
        null,
      ),
    ).toBe("http://bridge.local:9020/api/v1/nvr/west20_nvr/channels/1/snapshot");
  });

  it("uses the direct VTO snapshot for previews before the media capture endpoint", () => {
    expect(
      vtoPreviewImageSrc({
        cameraEntity: {
          entity_id: "camera.front_vto_camera",
          state: "streaming",
          attributes: {
            snapshot_url: "https://ha.example.com/bridge/api/v1/vto/front_vto/snapshot",
          },
          last_changed: "",
          last_updated: "",
        },
        snapshotUrl: "https://ha.example.com/bridge/api/v1/vto/front_vto/snapshot",
        captureSnapshotUrl: "https://ha.example.com/bridge/api/v1/media/snapshot/front_vto",
      }),
    ).toBe("https://ha.example.com/bridge/api/v1/vto/front_vto/snapshot");
  });

  it("orders playback session sources as HLS, DASH, then MJPEG", () => {
    const session = {
      id: "nvrpb_test",
      streamId: "nvrpb_test",
      deviceId: "west20_nvr",
      sourceStreamId: "west20_nvr_channel_01",
      name: "Channel 1",
      channel: 1,
      startTime: "2026-05-01T10:15:30Z",
      endTime: "2026-05-01T10:45:30Z",
      seekTime: "2026-05-01T10:15:30Z",
      recommendedProfile: "quality",
      snapshotUrl: null,
      createdAt: "2026-05-01T10:15:30Z",
      expiresAt: "2026-05-01T10:45:30Z",
      profiles: {
        quality: {
          name: "Quality",
          dashUrl: "http://bridge.local:9020/api/v1/media/dash/nvrpb_test/quality/manifest.mpd",
          hlsUrl: "http://bridge.local:9020/api/v1/media/hls/nvrpb_test/quality/index.m3u8",
          mjpegUrl: "http://bridge.local:9020/api/v1/media/mjpeg/nvrpb_test?profile=quality",
          webrtcOfferUrl: null,
        },
      },
    };

    expect(availablePlaybackViewportSources(session, "quality")).toEqual([
      "hls",
      "dash",
      "mjpeg",
    ] satisfies CameraViewportSource[]);
    expect(resolveInitialPlaybackViewportSource(session, "quality", null)).toBe("hls");
    expect(resolvePlaybackViewportSource(session, "dash", "quality")).toBe("dash");
  });

  it("builds direct Dahua RTSP playback URLs with seek timestamps", () => {
    const start = new Date(2026, 4, 1, 10, 15, 30);
    const end = new Date(2026, 4, 1, 10, 45, 30);

    expect(
      buildRtspPlaybackUrl({
        streamUrl: "rtsp://user:pass@192.0.2.10:554/cam/realmonitor?channel=1&subtype=0",
        channel: 4,
        subtype: 1,
        seekTime: start.toISOString(),
        endTime: end.toISOString(),
      }),
    ).toBe(
      "rtsp://user:pass@192.0.2.10:554/cam/playback?channel=4&subtype=1&starttime=2026_05_01_10_15_30&endtime=2026_05_01_10_45_30",
    );
  });

  it("omits RTSP playback endtime when direct archive playback is open ended", () => {
    const start = new Date(2026, 4, 4, 4, 30, 0);

    expect(
      buildRtspPlaybackUrl({
        streamUrl: "rtsp://user:pass@192.0.2.10:554/cam/realmonitor?channel=1&subtype=0",
        channel: 1,
        subtype: 0,
        seekTime: start.toISOString(),
        endTime: null,
      }),
    ).toBe(
      "rtsp://user:pass@192.0.2.10:554/cam/playback?channel=1&subtype=0&starttime=2026_05_04_04_30_00",
    );
  });
});
