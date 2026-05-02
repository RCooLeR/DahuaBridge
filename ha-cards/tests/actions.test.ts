import { beforeEach, describe, expect, it, vi } from "vitest";

import { SurveillancePanelActions } from "../src/cards/surveillance-panel-actions";
import type { CameraViewModel } from "../src/domain/model";
import type { HomeAssistant } from "../src/types/home-assistant";

const actionMocks = vi.hoisted(() => ({
  pressButton: vi.fn(async () => undefined),
  toggleSwitch: vi.fn(async () => undefined),
  setNumberValue: vi.fn(async () => undefined),
  postBridgeRequest: vi.fn(async () => undefined),
  readBridgeJson: vi.fn<
    () => Promise<{ items: Array<{ status: string; stop_url?: string }> }>
  >(async () => ({ items: [] })),
}));

vi.mock("../src/ha/actions", () => ({
  pressButton: actionMocks.pressButton,
  toggleSwitch: actionMocks.toggleSwitch,
  setNumberValue: actionMocks.setNumberValue,
  postBridgeRequest: actionMocks.postBridgeRequest,
  readBridgeJson: actionMocks.readBridgeJson,
}));

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
    eventsUrl: "http://bridge.local:9205/api/v1/events",
    snapshotUrl: null,
    captureSnapshotUrl: null,
    stream: {
      available: true,
      source: null,
      snapshotUrl: null,
      localIntercomUrl: null,
      onvifStreamUrl: null,
      onvifSnapshotUrl: null,
      recommendedProfile: null,
      recommendedHaIntegration: null,
      preferredVideoProfile: null,
      preferredVideoSource: null,
      resolution: "",
      codec: "",
      frameRate: "",
      bitrate: "",
      profile: "",
      audioCodec: "",
      profiles: [],
    },
    detections: [],
    supportsPtz: true,
    supportsPtzPan: true,
    supportsPtzTilt: true,
    supportsPtzZoom: false,
    supportsPtzFocus: false,
    supportsAux: true,
    supportsRecording: false,
    recordingActive: false,
    bridgeRecordingActive: false,
    ptzUrl: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/ptz",
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
    eventCount24h: 0,
    humanCount24h: 0,
    vehicleCount24h: 0,
    ...overrides,
  };
}

function buildHost(hass?: HomeAssistant) {
  let busyActions = new Set<string>();
  let errorMessage = "";

  const host = {
    getHass: () => hass,
    getBusyActions: () => busyActions,
    setBusyActions: (next: Set<string>) => {
      busyActions = next;
    },
    setError: (message: string) => {
      errorMessage = message;
    },
  };

  return {
    host,
    getBusyActions: () => busyActions,
    getErrorMessage: () => errorMessage,
  };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

describe("SurveillancePanelActions", () => {
  beforeEach(() => {
    actionMocks.pressButton.mockClear();
    actionMocks.toggleSwitch.mockClear();
    actionMocks.setNumberValue.mockClear();
    actionMocks.postBridgeRequest.mockClear();
    actionMocks.readBridgeJson.mockClear();
  });

  it("uses pulse actions for non-toggle aux targets", async () => {
    const { host, getBusyActions, getErrorMessage } = buildHost();
    const actions = new SurveillancePanelActions(host);
    const camera = buildCamera({
      aux: {
        supported: true,
        url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
        outputs: ["wiper"],
        features: ["wiper"],
        targets: [
          {
            key: "wiper",
            label: "Wiper",
            url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
            parameterKey: "output",
            parameterValue: "wiper",
            outputKey: "wiper",
            actions: ["pulse"],
            preferredAction: "pulse",
            active: null,
            currentText: null,
            toggleSupported: false,
          },
        ],
      },
    });

    await actions.triggerAuxAction(camera, "wiper", false);

    expect(actionMocks.postBridgeRequest).toHaveBeenCalledWith(
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
      {
        body: {
          output: "wiper",
          action: "pulse",
          duration_ms: 3000,
        },
      },
    );
    expect(getBusyActions().size).toBe(0);
    expect(getErrorMessage()).toBe("");
  });

  it("uses start and stop actions for deterrence outputs even when metadata only advertises pulse", async () => {
    const { host } = buildHost();
    const actions = new SurveillancePanelActions(host);
    const camera = buildCamera({
      aux: {
        supported: true,
        url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
        outputs: ["warning_light"],
        features: ["warning_light"],
        targets: [
          {
            key: "warning_light",
            label: "Warning Light",
            url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
            parameterKey: "output",
            parameterValue: "warning_light",
            outputKey: "warning_light",
            actions: ["pulse"],
            preferredAction: "pulse",
            active: false,
            currentText: "Off",
            toggleSupported: false,
          },
        ],
      },
    });

    await actions.triggerAuxAction(camera, "warning_light", false);
    await actions.triggerAuxAction(camera, "warning_light", true);

    expect(actionMocks.postBridgeRequest).toHaveBeenNthCalledWith(
      1,
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
      {
        body: {
          output: "warning_light",
          action: "start",
        },
      },
    );
    expect(actionMocks.postBridgeRequest).toHaveBeenNthCalledWith(
      2,
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
      {
        body: {
          output: "warning_light",
          action: "stop",
        },
      },
    );
  });

  it("uses start and stop actions for toggle-capable aux targets", async () => {
    const { host } = buildHost();
    const actions = new SurveillancePanelActions(host);
    const camera = buildCamera({
      aux: {
        supported: true,
        url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
        outputs: ["warning_light"],
        features: ["warning_light"],
        targets: [
          {
            key: "warning_light",
            label: "Warning Light",
            url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
            parameterKey: "output",
            parameterValue: "warning_light",
            outputKey: "warning_light",
            actions: ["start", "stop"],
            preferredAction: "start",
            active: false,
            currentText: "Off",
            toggleSupported: true,
          },
        ],
      },
    });

    await actions.triggerAuxAction(camera, "warning_light", false);
    await actions.triggerAuxAction(camera, "warning_light", true);

    expect(actionMocks.postBridgeRequest).toHaveBeenNthCalledWith(
      1,
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
      {
        body: {
          output: "warning_light",
          action: "start",
        },
      },
    );
    expect(actionMocks.postBridgeRequest).toHaveBeenNthCalledWith(
      2,
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
      {
        body: {
          output: "warning_light",
          action: "stop",
        },
      },
    );
  });

  it("uses the direct light output target for white-light mode toggles", async () => {
    const { host } = buildHost();
    const actions = new SurveillancePanelActions(host);
    const camera = buildCamera({
      aux: {
        supported: true,
        url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
        outputs: ["light"],
        features: ["light"],
        targets: [
          {
            key: "light",
            label: "White Light",
            url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
            parameterKey: "output",
            parameterValue: "light",
            outputKey: "light",
            actions: ["start", "stop"],
            preferredAction: "start",
            active: false,
            currentText: "Smart Light",
            toggleSupported: true,
          },
        ],
      },
    });

    await actions.triggerAuxAction(camera, "light", false);
    await actions.triggerAuxAction(camera, "light", true);

    expect(actionMocks.postBridgeRequest).toHaveBeenNthCalledWith(
      1,
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
      {
        body: {
          output: "light",
          action: "start",
        },
      },
    );
    expect(actionMocks.postBridgeRequest).toHaveBeenNthCalledWith(
      2,
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
      {
        body: {
          output: "light",
          action: "stop",
        },
      },
    );
  });

  it("falls back to the derived aux bridge URL when no typed aux target exists", async () => {
    const { host } = buildHost();
    const actions = new SurveillancePanelActions(host);
    const camera = buildCamera({
      aux: null,
      auxUrl: null,
    });

    await actions.triggerAuxAction(camera, "siren", false);

    expect(actionMocks.postBridgeRequest).toHaveBeenCalledWith(
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
      {
        body: {
          output: "siren",
          action: "pulse",
          duration_ms: 3000,
        },
      },
    );
  });

  it("prefers Home Assistant entities for VTO button actions and falls back to bridge URLs otherwise", async () => {
    const hassWithEntity: HomeAssistant = {
      states: {
        "button.front_station_unlock": {
          entity_id: "button.front_station_unlock",
          state: "unknown",
          attributes: {},
          last_changed: "",
          last_updated: "",
        },
      },
      callService: async () => undefined,
    };
    const entityHost = buildHost(hassWithEntity);
    const entityActions = new SurveillancePanelActions(entityHost.host);

    await entityActions.triggerVtoButtonAction(
      "vto:unlock",
      "button.front_station_unlock",
      "http://bridge.local:9205/api/v1/vto/front_vto/locks/0/unlock",
    );

    expect(actionMocks.pressButton).toHaveBeenCalledWith(
      hassWithEntity,
      "button.front_station_unlock",
    );
    expect(actionMocks.postBridgeRequest).not.toHaveBeenCalled();

    const hassWithoutEntity: HomeAssistant = {
      states: {},
      callService: async () => undefined,
    };
    const fallbackHost = buildHost(hassWithoutEntity);
    const fallbackActions = new SurveillancePanelActions(fallbackHost.host);

    await fallbackActions.triggerVtoButtonAction(
      "vto:unlock",
      "button.front_station_unlock",
      "http://bridge.local:9205/api/v1/vto/front_vto/locks/0/unlock",
    );

    expect(actionMocks.postBridgeRequest).toHaveBeenCalledWith(
      "http://bridge.local:9205/api/v1/vto/front_vto/locks/0/unlock",
    );
  });

  it("surfaces a clear error when a degraded VTO switch action has neither entity nor bridge fallback", async () => {
    const hass: HomeAssistant = {
      states: {},
      callService: async () => undefined,
    };
    const { host, getErrorMessage } = buildHost(hass);
    const actions = new SurveillancePanelActions(host);

    await actions.triggerVtoSwitchAction("vto:mute", "switch.front_station_muted", true, null, "muted");

    expect(actionMocks.toggleSwitch).not.toHaveBeenCalled();
    expect(actionMocks.postBridgeRequest).not.toHaveBeenCalled();
    expect(getErrorMessage()).toBe(
      "Switch control is unavailable in Home Assistant and no bridge fallback URL was provided.",
    );
  });

  it("uses direct bridge MP4 recording URLs for camera start and stop actions", async () => {
    const { host } = buildHost();
    const actions = new SurveillancePanelActions(host);
    const camera = buildCamera({
      supportsRecording: true,
      recordingStartUrl:
        "http://bridge.local:9205/api/v1/media/streams/west20_nvr_channel_01/recordings",
      recordingStopUrl:
        "http://bridge.local:9205/api/v1/media/recordings/clip_active/stop",
      recordingsUrl:
        "http://bridge.local:9205/api/v1/media/recordings?stream_id=west20_nvr_channel_01",
    });

    await actions.triggerRecordingAction(camera, "start");
    await actions.triggerRecordingAction(camera, "stop");

    expect(actionMocks.postBridgeRequest).toHaveBeenNthCalledWith(
      1,
      "http://bridge.local:9205/api/v1/media/streams/west20_nvr_channel_01/recordings",
    );
    expect(actionMocks.postBridgeRequest).toHaveBeenNthCalledWith(
      2,
      "http://bridge.local:9205/api/v1/media/recordings/clip_active/stop",
    );
    expect(actionMocks.readBridgeJson).toHaveBeenCalledWith(
      "http://bridge.local:9205/api/v1/media/recordings?stream_id=west20_nvr_channel_01",
    );
  });

  it("prefers the current active clip stop URL from the bridge recordings list", async () => {
    actionMocks.readBridgeJson.mockResolvedValueOnce({
      items: [
        {
          status: "completed",
        },
        {
          status: "recording",
          stop_url: "http://bridge.local:9205/api/v1/media/recordings/clip_live/stop",
        },
      ],
    } as { items: Array<{ status: string; stop_url?: string }> });

    const { host } = buildHost();
    const actions = new SurveillancePanelActions(host);
    const camera = buildCamera({
      supportsRecording: true,
      recordingStopUrl:
        "http://bridge.local:9205/api/v1/media/recordings/clip_stale/stop",
      recordingsUrl:
        "http://bridge.local:9205/api/v1/media/recordings?stream_id=west20_nvr_channel_01",
    });

    await actions.triggerRecordingAction(camera, "stop");

    expect(actionMocks.postBridgeRequest).toHaveBeenCalledWith(
      "http://bridge.local:9205/api/v1/media/recordings/clip_live/stop",
    );
  });

  it("ignores duplicate recording requests while the first request is still busy", async () => {
    const bridgeCall = deferred<undefined>();
    actionMocks.postBridgeRequest.mockImplementationOnce(async () => bridgeCall.promise);

    const { host } = buildHost();
    const actions = new SurveillancePanelActions(host);
    const camera = buildCamera({
      supportsRecording: true,
      recordingStartUrl:
        "http://bridge.local:9205/api/v1/media/streams/west20_nvr_channel_01/recordings",
    });

    const first = actions.triggerRecordingAction(camera, "start");
    const second = actions.triggerRecordingAction(camera, "start");

    await Promise.resolve();
    expect(actionMocks.postBridgeRequest).toHaveBeenCalledTimes(1);

    bridgeCall.resolve(undefined);
    await Promise.all([first, second]);
  });
});
