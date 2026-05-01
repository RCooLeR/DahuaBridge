import { describe, expect, it } from "vitest";

import {
  availableCameraViewportSources,
  availablePlaybackViewportSources,
  cameraImageSrc,
  defaultOverviewStreamProfileKey,
  resolveInitialPlaybackViewportSource,
  resolveOverviewCameraViewportSource,
  resolvePlaybackViewportSource,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  type CameraViewportSource,
} from "../src/cards/surveillance-panel-media";
import type { NvrPlaybackSessionModel } from "../src/domain/archive";
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
    bridgeBaseUrl: "http://bridge.local:9205",
    eventsUrl: null,
    snapshotUrl: null,
    captureSnapshotUrl: null,
    stream: {
      available: true,
      source: "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
      snapshotUrl: null,
      localIntercomUrl: null,
      onvifStreamUrl: null,
      onvifSnapshotUrl: null,
      recommendedProfile: "quality",
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
          localMjpegUrl: "http://bridge.local:9205/api/v1/media/mjpeg/west20_nvr_channel_01/quality",
          localHlsUrl: "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
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
          localMjpegUrl: "http://bridge.local:9205/api/v1/media/mjpeg/west20_nvr_channel_01/stable",
          localHlsUrl: null,
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
    audioMuted: false,
    audioMuteSupported: false,
    audioMuteActionUrl: null,
    validationNotes: [],
    audioControlAuthority: null,
    audioControlSemantic: null,
    nvrConfigWritable: null,
    nvrConfigReason: null,
    directIPCConfigured: false,
    directIPCConfiguredIP: null,
    directIPCIP: null,
    directIPCModel: null,
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
      "hls",
      "mjpeg",
    ] satisfies CameraViewportSource[]);
    expect(availableCameraViewportSources(camera, "stable")).toEqual([
      "mjpeg",
    ] satisfies CameraViewportSource[]);
    expect(resolveSelectedCameraViewportSource(camera, "hls", "stable")).toBe("mjpeg");
    expect(resolveSelectedCameraViewportSource(camera, "mjpeg", "stable")).toBe("mjpeg");
  });

  it("ignores WebRTC and keeps browser-safe viewport sources", () => {
    const camera = buildCamera({
      stream: {
        ...buildCamera().stream,
        preferredVideoSource: "webrtc",
        profiles: [
          {
            ...buildCamera().stream.profiles[0]!,
            localWebRtcUrl: "http://bridge.local:9205/api/v1/media/webrtc/west20_nvr_channel_01/quality",
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
        preferredVideoSource: "webrtc",
        profiles: [
          {
            ...buildCamera().stream.profiles[0]!,
            localWebRtcUrl: "http://bridge.local:9205/api/v1/media/webrtc/west20_nvr_channel_01/quality",
          },
          {
            ...buildCamera().stream.profiles[1]!,
            localHlsUrl: "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/stable",
          },
        ],
      },
    });

    const overviewProfileKey = defaultOverviewStreamProfileKey(camera.stream);
    expect(overviewProfileKey).toBe("stable");
    expect(resolveOverviewCameraViewportSource(camera, overviewProfileKey)).toBe("hls");
  });

  it("keeps playback on HLS/MJPEG even when the session exposes WebRTC", () => {
    const session: NvrPlaybackSessionModel = {
      id: "nvrpb_test",
      streamId: "nvrpb_test",
      deviceId: "west20_nvr",
      sourceStreamId: "west20_nvr_channel_01",
      name: "Entrance",
      channel: 1,
      startTime: "2026-05-01T10:00:00Z",
      endTime: "2026-05-01T10:10:00Z",
      seekTime: "2026-05-01T10:00:00Z",
      recommendedProfile: "quality",
      snapshotUrl: null,
      createdAt: "2026-05-01T10:00:00Z",
      expiresAt: "2026-05-01T10:10:00Z",
      profiles: {
        quality: {
          name: "quality",
          hlsUrl: "http://bridge.local:9205/api/v1/media/hls/nvrpb_test/quality/index.m3u8",
          mjpegUrl: null,
          webrtcOfferUrl: "http://bridge.local:9205/api/v1/media/webrtc/nvrpb_test/quality/offer",
        },
      },
    };

    expect(availablePlaybackViewportSources(session, "quality")).toEqual([
      "hls",
    ] satisfies CameraViewportSource[]);
    expect(resolvePlaybackViewportSource(session, "hls", "quality")).toBe("hls");
  });

  it("starts playback on HLS instead of inheriting live MJPEG", () => {
    const session: NvrPlaybackSessionModel = {
      id: "nvrpb_test",
      streamId: "nvrpb_test",
      deviceId: "west20_nvr",
      sourceStreamId: "west20_nvr_channel_01",
      name: "Entrance",
      channel: 1,
      startTime: "2026-05-01T10:00:00Z",
      endTime: "2026-05-01T10:10:00Z",
      seekTime: "2026-05-01T10:00:00Z",
      recommendedProfile: "quality",
      snapshotUrl: null,
      createdAt: "2026-05-01T10:00:00Z",
      expiresAt: "2026-05-01T10:10:00Z",
      profiles: {
        quality: {
          name: "quality",
          hlsUrl: "http://bridge.local:9205/api/v1/media/hls/nvrpb_test/quality/index.m3u8",
          mjpegUrl: "http://bridge.local:9205/api/v1/media/mjpeg/nvrpb_test/quality",
          webrtcOfferUrl: "http://bridge.local:9205/api/v1/media/webrtc/nvrpb_test/quality/offer",
        },
      },
    };

    expect(resolveInitialPlaybackViewportSource(session, "quality", "mjpeg")).toBe("hls");
    expect(resolveInitialPlaybackViewportSource(session, "quality", null)).toBe("hls");
  });

  it("prefers the bridge snapshot URL over the entity picture fallback", () => {
    expect(
      cameraImageSrc(
        {
          entity_id: "camera.west20_nvr_channel_01_camera",
          state: "streaming",
          attributes: {
            entity_picture: "/api/camera_proxy/camera.west20_nvr_channel_01_camera",
            snapshot_url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/snapshot",
          },
          last_changed: "",
          last_updated: "",
        },
        null,
      ),
    ).toBe("http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/snapshot");
  });
});
