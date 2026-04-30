import { describe, expect, it } from "vitest";

import {
  availableCameraViewportSources,
  cameraImageSrc,
  resolveSelectedCameraStreamProfile,
  resolveSelectedCameraViewportSource,
  type CameraViewportSource,
} from "../src/cards/surveillance-panel-media";
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
