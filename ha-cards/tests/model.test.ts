import { describe, expect, it } from "vitest";

import { buildPanelModel, displayCameraLabel } from "../src/domain/model";
import type { SurveillancePanelCardConfig } from "../src/types/card-config";
import type { RegistrySnapshot } from "../src/ha/registry";
import type { HomeAssistant } from "../src/types/home-assistant";

describe("buildPanelModel", () => {
  it("formats NVR channel display labels with the channel number first", () => {
    expect(
      displayCameraLabel({
        deviceKind: "nvr_channel",
        channelNumber: 4,
        label: "Entrance Gate",
      }),
    ).toBe("CH 4 - Entrance Gate");
    expect(
      displayCameraLabel({
        deviceKind: "ipc",
        channelNumber: null,
        label: "Garage",
      }),
    ).toBe("Garage");
  });

  it("builds a camera model from discovered bridge camera entities", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.west20_nvr_channel_01_camera": {
          entity_id: "camera.west20_nvr_channel_01_camera",
          state: "recording",
          attributes: {
            friendly_name: "Entrance Gate",
            bridge_device_id: "west20_nvr_channel_01",
            bridge_root_device_id: "west20_nvr",
            bridge_device_kind: "nvr_channel",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_online": {
          entity_id: "binary_sensor.west20_nvr_channel_01_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_stream_available": {
          entity_id: "binary_sensor.west20_nvr_channel_01_stream_available",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
    };

    const model = buildPanelModel(hass, config, { kind: "overview" });

    expect(model.cameras).toHaveLength(1);
    expect(model.cameras[0]?.bridgeBaseUrl).toBe("http://bridge.local:9205");
    expect(model.headerMetrics[0]?.value).toBe("1/1");
  });

  it("discovers NVR channels from bridge URLs when camera entity ids were customized", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.entry_gate_live": {
          entity_id: "camera.entry_gate_live",
          state: "recording",
          attributes: {
            friendly_name: "Entry Gate Live",
            snapshot_url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/snapshot",
            stream_source:
              "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/main.m3u8",
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_online": {
          entity_id: "binary_sensor.west20_nvr_channel_01_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
    };

    const model = buildPanelModel(hass, config, { kind: "overview" });

    expect(model.cameras).toHaveLength(1);
    expect(model.cameras[0]?.deviceId).toBe("west20_nvr_channel_01");
    expect(model.cameras[0]?.cameraEntityId).toBe("camera.entry_gate_live");
    expect(model.nvrs[0]?.deviceId).toBe("west20_nvr");
  });

  it("prefers the Home Assistant device registry name over bridge-provided sensor naming", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.west20_nvr_channel_01_camera": {
          entity_id: "camera.west20_nvr_channel_01_camera",
          state: "recording",
          attributes: {
            friendly_name: "Sensor Label",
            bridge_device_id: "west20_nvr_channel_01",
            bridge_root_device_id: "west20_nvr",
            bridge_device_kind: "nvr_channel",
            bridge_device_name: "Bridge Sensor Label",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_online": {
          entity_id: "binary_sensor.west20_nvr_channel_01_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const registrySnapshot: RegistrySnapshot = {
      entitiesByEntityId: new Map([
        [
          "camera.west20_nvr_channel_01_camera",
          { entity_id: "camera.west20_nvr_channel_01_camera", device_id: "dev-channel-1" },
        ],
      ]),
      entitiesByUniqueId: new Map(),
      entitiesByDeviceId: new Map(),
      devicesById: new Map([
        [
          "dev-channel-1",
          { id: "dev-channel-1", name_by_user: "HA Registry Camera Name" },
        ],
      ]),
      areasById: new Map(),
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
    };

    const model = buildPanelModel(
      hass,
      config,
      { kind: "overview" },
      undefined,
      undefined,
      registrySnapshot,
    );

    expect(model.cameras[0]?.label).toBe("HA Registry Camera Name");
  });

  it("derives camera controls, vto fallbacks, and HA room assignments", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.west20_nvr_channel_01_camera": {
          entity_id: "camera.west20_nvr_channel_01_camera",
          state: "recording",
          attributes: {
            friendly_name: "Entrance Gate",
            bridge_device_id: "west20_nvr_channel_01",
            bridge_root_device_id: "west20_nvr",
            bridge_device_kind: "nvr_channel",
            bridge_base_url: "http://bridge.local:9205",
            bridge_events_url: "http://bridge.local:9205/api/v1/events",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
            bridge_capture: {
              snapshot_url:
                "http://bridge.local:9205/api/v1/media/snapshot/west20_nvr_channel_01",
              active: true,
              start_recording_url:
                "http://bridge.local:9205/api/v1/media/streams/west20_nvr_channel_01/recordings",
              stop_recording_url:
                "http://bridge.local:9205/api/v1/media/recordings/clip_active/stop",
              recordings_url:
                "http://bridge.local:9205/api/v1/media/streams/west20_nvr_channel_01/recordings",
            },
            bridge_controls: {
              ptz: {
                supported: true,
                pan: true,
                tilt: true,
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/ptz",
              },
              aux: {
                supported: true,
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
              },
              recording: {
                supported: true,
                active: true,
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/recording",
              },
            },
            bridge_features: [
              {
                key: "warning_light",
                label: "Warning Light",
                group: "deterrence",
                kind: "action",
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
                supported: true,
                parameter_key: "output",
                parameter_value: "warning_light",
                actions: ["start", "stop", "pulse"],
              },
              {
                key: "ptz",
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/ptz",
              },
            ],
          },
          last_changed: now,
          last_updated: now,
        },
        "camera.front_vto_camera": {
          entity_id: "camera.front_vto_camera",
          state: "streaming",
          attributes: {
            friendly_name: "Front VTO",
            bridge_device_id: "front_vto",
            bridge_root_device_id: "front_vto",
            bridge_device_kind: "vto",
            bridge_base_url: "http://bridge.local:9205",
            bridge_events_url: "http://bridge.local:9205/api/v1/events",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/front_vto/quality",
            bridge_intercom: {
              answer_url: "http://bridge.local:9205/api/v1/vto/front_vto/call/answer",
              hangup_url: "http://bridge.local:9205/api/v1/vto/front_vto/call/hangup",
              lock_urls: [
                "http://bridge.local:9205/api/v1/vto/front_vto/locks/0/unlock",
              ],
              output_volume_url:
                "http://bridge.local:9205/api/v1/vto/front_vto/audio/output-volume",
              input_volume_url:
                "http://bridge.local:9205/api/v1/vto/front_vto/audio/input-volume",
              mute_url: "http://bridge.local:9205/api/v1/vto/front_vto/audio/mute",
              recording_url: "http://bridge.local:9205/api/v1/vto/front_vto/recording",
            },
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_online": {
          entity_id: "binary_sensor.west20_nvr_channel_01_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_nvr_config_writable": {
          entity_id: "binary_sensor.west20_nvr_channel_01_nvr_config_writable",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_channel_01_nvr_config_reason": {
          entity_id: "sensor.west20_nvr_channel_01_nvr_config_reason",
          state: "ok",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_channel_01_control_audio_authority": {
          entity_id: "sensor.west20_nvr_channel_01_control_audio_authority",
          state: "direct_ipc",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_channel_01_control_audio_semantic": {
          entity_id: "sensor.west20_nvr_channel_01_control_audio_semantic",
          state: "stream_audio_enable",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_direct_ipc_configured": {
          entity_id: "binary_sensor.west20_nvr_channel_01_direct_ipc_configured",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_channel_01_direct_ipc_configured_ip": {
          entity_id: "sensor.west20_nvr_channel_01_direct_ipc_configured_ip",
          state: "192.168.150.60",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_channel_01_direct_ipc_model": {
          entity_id: "sensor.west20_nvr_channel_01_direct_ipc_model",
          state: "DH-IPC-HFW2849S-S-IL",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.front_vto_online": {
          entity_id: "binary_sensor.front_vto_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };
    const registrySnapshot: RegistrySnapshot = {
      entitiesByEntityId: new Map([
        [
          "camera.west20_nvr_channel_01_camera",
          { entity_id: "camera.west20_nvr_channel_01_camera", device_id: "dev-channel-1" },
        ],
        [
          "camera.front_vto_camera",
          { entity_id: "camera.front_vto_camera", device_id: "dev-vto" },
        ],
      ]),
      entitiesByUniqueId: new Map(),
      entitiesByDeviceId: new Map(),
      devicesById: new Map([
        [
          "dev-channel-1",
          { id: "dev-channel-1", area_id: "area-entrance", via_device_id: "dev-nvr", name: "CH 01 Entrance Gate" },
        ],
        [
          "dev-nvr",
          { id: "dev-nvr", name: "West20 NVR" },
        ],
        [
          "dev-vto",
          { id: "dev-vto", area_id: "area-entry", name: "Front VTO" },
        ],
      ]),
      areasById: new Map([
        ["area-entrance", { area_id: "area-entrance", name: "Entrance" }],
        ["area-entry", { area_id: "area-entry", name: "Entry" }],
      ]),
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
      vto: {
        device_id: "front_vto",
      },
    };

    const model = buildPanelModel(hass, config, { kind: "overview" }, undefined, undefined, registrySnapshot);

    expect(model.cameras[0]?.supportsPtz).toBe(true);
    expect(model.cameras[0]?.roomLabel).toBe("Entrance");
    expect(model.cameras[0]?.ptzUrl).toBe(
      "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/ptz",
    );
    expect(model.cameras[0]?.eventsUrl).toBe(
      "http://bridge.local:9205/api/v1/events",
    );
    expect(model.cameras[0]?.recordingActive).toBe(true);
    expect(model.cameras[0]?.supportsRecording).toBe(true);
    expect(model.cameras[0]?.bridgeRecordingActive).toBe(true);
    expect(model.cameras[0]?.captureSnapshotUrl).toBe(
      "http://bridge.local:9205/api/v1/media/snapshot/west20_nvr_channel_01",
    );
    expect(model.cameras[0]?.recordingStartUrl).toBe(
      "http://bridge.local:9205/api/v1/media/streams/west20_nvr_channel_01/recordings",
    );
    expect(model.cameras[0]?.recordingStopUrl).toBe(
      "http://bridge.local:9205/api/v1/media/recordings/clip_active/stop",
    );
    expect(model.cameras[0]?.recording).toMatchObject({
      supported: true,
      active: true,
      mode: null,
      url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/recording",
    });
    expect(model.cameras[0]).toMatchObject({
      audioControlAuthority: "direct_ipc",
      audioControlSemantic: "stream_audio_enable",
      nvrConfigWritable: true,
      nvrConfigReason: "ok",
      directIPCConfigured: true,
      directIPCConfiguredIP: "192.168.150.60",
      directIPCModel: "DH-IPC-HFW2849S-S-IL",
    });
    expect(model.cameras[0]?.stream.source).toBe(
      "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
    );
    expect(model.cameras[0]?.aux?.targets).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "warning_light",
          label: "Warning Light",
          outputKey: "warning_light",
          preferredAction: "pulse",
        }),
      ]),
    );
    expect(model.nvrs[0]?.label).toBe("West20 NVR");
    expect(model.vto?.answerActionUrl).toBe(
      "http://bridge.local:9205/api/v1/vto/front_vto/call/answer",
    );
    expect(model.vto?.eventsUrl).toBe(
      "http://bridge.local:9205/api/v1/events",
    );
    expect(model.vto?.unlockActionUrl).toBe(
      "http://bridge.local:9205/api/v1/vto/front_vto/locks/0/unlock",
    );
  });

  it("keeps multiple VTOs in the panel model and preserves the configured preferred VTO", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.front_vto_camera": {
          entity_id: "camera.front_vto_camera",
          state: "streaming",
          attributes: {
            friendly_name: "Front VTO",
            bridge_device_id: "front_vto",
            bridge_root_device_id: "front_vto",
            bridge_device_kind: "vto",
            bridge_base_url: "http://bridge.local:9205",
            bridge_events_url: "http://bridge.local:9205/api/v1/events",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/front_vto/quality",
          },
          last_changed: now,
          last_updated: now,
        },
        "camera.gate_vto_camera": {
          entity_id: "camera.gate_vto_camera",
          state: "streaming",
          attributes: {
            friendly_name: "Gate VTO",
            bridge_device_id: "gate_vto",
            bridge_root_device_id: "gate_vto",
            bridge_device_kind: "vto",
            bridge_base_url: "http://bridge.local:9205",
            bridge_events_url: "http://bridge.local:9205/api/v1/events",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/gate_vto/quality",
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.front_vto_online": {
          entity_id: "binary_sensor.front_vto_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.gate_vto_online": {
          entity_id: "binary_sensor.gate_vto_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
      vto: {
        device_id: "gate_vto",
        label: "Gate Station",
      },
    };

    const model = buildPanelModel(hass, config, { kind: "vto", deviceId: "front_vto" });

    expect(model.vtos).toHaveLength(2);
    expect(model.vto?.deviceId).toBe("gate_vto");
    expect(model.vto?.label).toBe("Gate Station");
    expect(model.selectedVto?.deviceId).toBe("front_vto");
    expect(model.selectedVto?.label).toBe("Front VTO");
    expect(model.headerMetrics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ label: "Cameras Online", value: "2/2" }),
        expect.objectContaining({ label: "Door Stations", value: "2/2 online" }),
      ]),
    );
  });

  it("rewrites browser-facing bridge URLs when browser_bridge_url is configured", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.west20_nvr_channel_01_camera": {
          entity_id: "camera.west20_nvr_channel_01_camera",
          state: "recording",
          attributes: {
            friendly_name: "Entrance Gate",
            bridge_device_id: "west20_nvr_channel_01",
            bridge_root_device_id: "west20_nvr",
            bridge_device_kind: "nvr_channel",
            bridge_base_url: "http://127.0.0.1:9205",
            bridge_events_url: "http://127.0.0.1:9205/api/v1/events",
            snapshot_url: "http://127.0.0.1:9205/api/v1/nvr/west20_nvr/channels/1/snapshot",
            bridge_capture: {
              snapshot_url:
                "http://127.0.0.1:9205/api/v1/media/snapshot/west20_nvr_channel_01",
              start_recording_url:
                "http://127.0.0.1:9205/api/v1/media/streams/west20_nvr_channel_01/recordings",
              stop_recording_url:
                "http://127.0.0.1:9205/api/v1/media/recordings/clip123/stop",
            },
            bridge_controls: {
              ptz: {
                supported: true,
                url: "http://127.0.0.1:9205/api/v1/nvr/west20_nvr/channels/1/ptz",
              },
              aux: {
                supported: true,
                url: "http://127.0.0.1:9205/api/v1/nvr/west20_nvr/channels/1/aux",
              },
            },
            bridge_features: [
              {
                key: "warning_light",
                label: "Warning Light",
                group: "deterrence",
                kind: "action",
                url: "http://127.0.0.1:9205/api/v1/nvr/west20_nvr/channels/1/aux",
                supported: true,
                parameter_key: "output",
                parameter_value: "warning_light",
                actions: ["pulse"],
              },
            ],
            stream_source: "http://127.0.0.1:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
          },
          last_changed: now,
          last_updated: now,
        },
        "camera.front_vto_camera": {
          entity_id: "camera.front_vto_camera",
          state: "streaming",
          attributes: {
            friendly_name: "Front VTO",
            bridge_device_id: "front_vto",
            bridge_root_device_id: "front_vto",
            bridge_device_kind: "vto",
            bridge_base_url: "http://127.0.0.1:9205",
            bridge_events_url: "http://127.0.0.1:9205/api/v1/events",
            snapshot_url: "http://127.0.0.1:9205/api/v1/vto/front_vto/snapshot",
            bridge_intercom: {
              answer_url: "http://127.0.0.1:9205/api/v1/vto/front_vto/call/answer",
            },
            stream_source: "http://127.0.0.1:9205/api/v1/media/hls/front_vto/quality",
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_online": {
          entity_id: "binary_sensor.west20_nvr_channel_01_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.front_vto_online": {
          entity_id: "binary_sensor.front_vto_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
      browser_bridge_url: "https://ha.example.com/bridge",
      vto: {
        device_id: "front_vto",
      },
    };

    const model = buildPanelModel(hass, config, { kind: "overview" });

    expect(model.cameras[0]?.bridgeBaseUrl).toBe("https://ha.example.com/bridge");
    expect(model.cameras[0]?.eventsUrl).toBe("https://ha.example.com/bridge/api/v1/events");
    expect(model.cameras[0]?.snapshotUrl).toBe(
      "https://ha.example.com/bridge/api/v1/nvr/west20_nvr/channels/1/snapshot",
    );
    expect(model.cameras[0]?.captureSnapshotUrl).toBe(
      "https://ha.example.com/bridge/api/v1/media/snapshot/west20_nvr_channel_01",
    );
    expect(model.cameras[0]?.stream.source).toBe(
      "https://ha.example.com/bridge/api/v1/media/hls/west20_nvr_channel_01/quality",
    );
    expect(model.cameras[0]?.ptzUrl).toBe(
      "https://ha.example.com/bridge/api/v1/nvr/west20_nvr/channels/1/ptz",
    );
    expect(model.cameras[0]?.aux?.url).toBe(
      "https://ha.example.com/bridge/api/v1/nvr/west20_nvr/channels/1/aux",
    );
    expect(model.cameras[0]?.recordingUrl).toBeNull();
    expect(model.cameras[0]?.recordingStartUrl).toBe(
      "https://ha.example.com/bridge/api/v1/media/streams/west20_nvr_channel_01/recordings",
    );
    expect(model.cameras[0]?.recordingStopUrl).toBe(
      "https://ha.example.com/bridge/api/v1/media/recordings/clip123/stop",
    );
    expect(model.cameras[0]?.aux?.targets[0]?.url).toBe(
      "https://ha.example.com/bridge/api/v1/nvr/west20_nvr/channels/1/aux",
    );
    expect(model.vto?.eventsUrl).toBe("https://ha.example.com/bridge/api/v1/events");
    expect(model.vto?.snapshotUrl).toBe(
      "https://ha.example.com/bridge/api/v1/vto/front_vto/snapshot",
    );
    expect(model.vto?.answerActionUrl).toBe(
      "https://ha.example.com/bridge/api/v1/vto/front_vto/call/answer",
    );
  });

  it("surfaces transport detection in header metrics and labels standalone IPC cameras explicitly", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.driveway_ipc_camera": {
          entity_id: "camera.driveway_ipc_camera",
          state: "streaming",
          attributes: {
            friendly_name: "Driveway IPC",
            bridge_device_id: "driveway_ipc",
            bridge_root_device_id: "driveway_ipc",
            bridge_device_kind: "ipc",
            stream_source: "http://bridge.local:9205/api/v1/ipc/driveway_ipc/stream",
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.driveway_ipc_online": {
          entity_id: "binary_sensor.driveway_ipc_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.driveway_ipc_vehicle": {
          entity_id: "binary_sensor.driveway_ipc_vehicle",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
    };

    const model = buildPanelModel(hass, config, { kind: "overview" });

    expect(model.cameras[0]?.deviceKind).toBe("ipc");
    expect(model.cameras[0]?.kindLabel).toBe("IPC Camera");
    expect(model.cameras[0]?.detections).toEqual(
      expect.arrayContaining([expect.objectContaining({ key: "vehicle", label: "Vehicle" })]),
    );
    expect(model.headerMetrics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ label: "Transport", value: "1", tone: "info" }),
      ]),
    );
  });

  it("preserves VTO lock, alarm, and intercom session detail in the panel model", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.front_vto_camera": {
          entity_id: "camera.front_vto_camera",
          state: "streaming",
          attributes: {
            friendly_name: "Front VTO",
            bridge_device_id: "front_vto",
            bridge_root_device_id: "front_vto",
            bridge_device_kind: "vto",
            bridge_base_url: "http://bridge.local:9205",
            bridge_events_url: "http://bridge.local:9205/api/v1/events",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/front_vto/quality",
            bridge_intercom: {
              answer_url: "http://bridge.local:9205/api/v1/vto/front_vto/call/answer",
              hangup_url: "http://bridge.local:9205/api/v1/vto/front_vto/call/hangup",
              lock_urls: ["http://bridge.local:9205/api/v1/vto/front_vto/locks/0/unlock"],
              bridge_session_active: true,
              bridge_session_count: 1,
              external_uplink_enabled: true,
              bridge_uplink_active: true,
              bridge_uplink_codec: "opus",
              bridge_uplink_packets: 128,
              bridge_forwarded_packets: 120,
              bridge_forward_errors: 2,
              configured_external_uplink_target_count: 1,
              supports_unlock: true,
              supports_browser_microphone: true,
              supports_bridge_audio_uplink: true,
              supports_bridge_audio_output: true,
              supports_external_audio_export: true,
              supports_vto_call_answer: true,
              supports_vto_talkback: true,
              supports_full_call_acceptance: true,
              validation_notes: ["door_station_profile_validated"],
            },
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.front_vto_online": {
          entity_id: "binary_sensor.front_vto_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "button.front_station_unlock": {
          entity_id: "button.front_station_unlock",
          state: "unknown",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.front_door_lock_state": {
          entity_id: "sensor.front_door_lock_state",
          state: "closed",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.front_door_lock_sensor_enabled": {
          entity_id: "binary_sensor.front_door_lock_sensor_enabled",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.front_door_lock_mode": {
          entity_id: "sensor.front_door_lock_mode",
          state: "normal",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.front_lock_model_actual": {
          entity_id: "sensor.front_lock_model_actual",
          state: "Relay Lock",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.vestibule_alarm_sense_method": {
          entity_id: "sensor.vestibule_alarm_sense_method",
          state: "NO",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.vestibule_alarm_enabled": {
          entity_id: "binary_sensor.vestibule_alarm_enabled",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.vestibule_alarm_active": {
          entity_id: "binary_sensor.vestibule_alarm_active",
          state: "off",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.vestibule_alarm_model_actual": {
          entity_id: "sensor.vestibule_alarm_model_actual",
          state: "Alarm Contact",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const registryEntities = [
      {
        entity_id: "camera.front_vto_camera",
        device_id: "dev-vto",
        unique_id: "front_vto_camera",
      },
      {
        entity_id: "button.front_station_unlock",
        device_id: "dev-vto",
        unique_id: "front_vto_unlock_1",
      },
      {
        entity_id: "sensor.front_door_lock_state",
        device_id: "dev-vto-lock-1",
        unique_id: "front_vto_lock_00_state",
      },
      {
        entity_id: "binary_sensor.front_door_lock_sensor_enabled",
        device_id: "dev-vto-lock-1",
        unique_id: "front_vto_lock_00_sensor_enabled",
      },
      {
        entity_id: "sensor.front_door_lock_mode",
        device_id: "dev-vto-lock-1",
        unique_id: "front_vto_lock_00_lock_mode",
      },
      {
        entity_id: "sensor.front_lock_model_actual",
        device_id: "dev-vto-lock-1",
        unique_id: "front_vto_lock_00_model",
      },
      {
        entity_id: "sensor.vestibule_alarm_sense_method",
        device_id: "dev-vto-alarm-1",
        unique_id: "front_vto_alarm_00_sense_method",
      },
      {
        entity_id: "binary_sensor.vestibule_alarm_enabled",
        device_id: "dev-vto-alarm-1",
        unique_id: "front_vto_alarm_00_enabled",
      },
      {
        entity_id: "binary_sensor.vestibule_alarm_active",
        device_id: "dev-vto-alarm-1",
        unique_id: "front_vto_alarm_00_active",
      },
      {
        entity_id: "sensor.vestibule_alarm_model_actual",
        device_id: "dev-vto-alarm-1",
        unique_id: "front_vto_alarm_00_model",
      },
    ];
    const entitiesByDeviceId = new Map<string, Array<{ entity_id: string; device_id?: string; unique_id?: string }>>();
    for (const entry of registryEntities) {
      if (!entry.device_id) {
        continue;
      }
      const group = entitiesByDeviceId.get(entry.device_id) ?? [];
      group.push(entry);
      entitiesByDeviceId.set(entry.device_id, group);
    }

    const registrySnapshot: RegistrySnapshot = {
      entitiesByEntityId: new Map(
        registryEntities.map((entry) => [entry.entity_id, entry] as const),
      ),
      entitiesByUniqueId: new Map(
        registryEntities
          .filter((entry) => entry.unique_id)
          .map((entry) => [entry.unique_id as string, entry] as const),
      ),
      entitiesByDeviceId,
      devicesById: new Map([
        ["dev-vto", { id: "dev-vto", area_id: "area-entry", name: "Front VTO" }],
        ["dev-vto-lock-1", { id: "dev-vto-lock-1", via_device_id: "dev-vto", name: "Front Door Lock" }],
        ["dev-vto-alarm-1", { id: "dev-vto-alarm-1", via_device_id: "dev-vto", name: "Vestibule Alarm" }],
      ]),
      areasById: new Map([["area-entry", { area_id: "area-entry", name: "Entry" }]]),
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
      vto: {
        device_id: "front_vto",
      },
    };

    const model = buildPanelModel(
      hass,
      config,
      { kind: "vto", deviceId: "front_vto" },
      undefined,
      undefined,
      registrySnapshot,
    );

    expect(model.selectedVto?.locks[0]).toMatchObject({
      label: "Front Door Lock",
      stateText: "closed",
      lockMode: "normal",
      modelText: "Relay Lock",
      hasUnlockButtonEntity: true,
    });
    expect(model.selectedVto?.alarms[0]).toMatchObject({
      label: "Vestibule Alarm",
      enabled: true,
      active: false,
      senseMethod: "NO",
      modelText: "Alarm Contact",
    });
    expect(model.selectedVto?.intercom).toMatchObject({
      bridgeSessionActive: true,
      bridgeSessionCount: 1,
      externalUplinkEnabled: true,
      bridgeUplinkActive: true,
      bridgeUplinkCodec: "opus",
      bridgeForwardErrors: 2,
    });
    expect(model.selectedVto?.capabilities).toMatchObject({
      browserMicrophoneSupported: true,
      externalAudioExportSupported: true,
      talkbackSupported: true,
      fullCallAcceptanceSupported: true,
      validationNotes: ["door_station_profile_validated"],
    });
  });

  it("keeps multiple NVR topologies isolated by recorder and selection", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.west20_nvr_channel_01_camera": {
          entity_id: "camera.west20_nvr_channel_01_camera",
          state: "streaming",
          attributes: {
            friendly_name: "West Entrance",
            bridge_device_id: "west20_nvr_channel_01",
            bridge_root_device_id: "west20_nvr",
            bridge_device_kind: "nvr_channel",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
          },
          last_changed: now,
          last_updated: now,
        },
        "camera.east20_nvr_channel_02_camera": {
          entity_id: "camera.east20_nvr_channel_02_camera",
          state: "recording",
          attributes: {
            friendly_name: "East Garage",
            bridge_device_id: "east20_nvr_channel_02",
            bridge_root_device_id: "east20_nvr",
            bridge_device_kind: "nvr_channel",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/east20_nvr_channel_02/quality",
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.west20_nvr_channel_01_online": {
          entity_id: "binary_sensor.west20_nvr_channel_01_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.east20_nvr_channel_02_online": {
          entity_id: "binary_sensor.east20_nvr_channel_02_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const registrySnapshot: RegistrySnapshot = {
      entitiesByEntityId: new Map([
        [
          "camera.west20_nvr_channel_01_camera",
          { entity_id: "camera.west20_nvr_channel_01_camera", device_id: "dev-west-channel" },
        ],
        [
          "camera.east20_nvr_channel_02_camera",
          { entity_id: "camera.east20_nvr_channel_02_camera", device_id: "dev-east-channel" },
        ],
      ]),
      entitiesByUniqueId: new Map(),
      entitiesByDeviceId: new Map(),
      devicesById: new Map([
        [
          "dev-west-channel",
          {
            id: "dev-west-channel",
            area_id: "area-west-entry",
            via_device_id: "dev-west-nvr",
            name: "West Entrance Channel",
          },
        ],
        [
          "dev-east-channel",
          {
            id: "dev-east-channel",
            area_id: "area-east-garage",
            via_device_id: "dev-east-nvr",
            name: "East Garage Channel",
          },
        ],
        ["dev-west-nvr", { id: "dev-west-nvr", name: "West Recorder" }],
        ["dev-east-nvr", { id: "dev-east-nvr", name: "East Recorder" }],
      ]),
      areasById: new Map([
        ["area-west-entry", { area_id: "area-west-entry", name: "West Entry" }],
        ["area-east-garage", { area_id: "area-east-garage", name: "East Garage" }],
      ]),
    };

    const config: SurveillancePanelCardConfig = {
      type: "custom:dahuabridge-surveillance-panel",
    };

    const model = buildPanelModel(
      hass,
      config,
      { kind: "nvr", deviceId: "east20_nvr" },
      undefined,
      undefined,
      registrySnapshot,
    );

    expect(model.nvrs).toHaveLength(2);
    expect(model.nvrs.map((nvr) => nvr.deviceId)).toEqual(["east20_nvr", "west20_nvr"]);
    expect(model.sidebarItems.filter((item) => item.kind === "nvr")).toHaveLength(2);
    expect(model.selectedNvr?.deviceId).toBe("east20_nvr");
    expect(model.selectedNvr?.rooms).toHaveLength(1);
    expect(model.selectedNvr?.rooms[0]?.label).toBe("East Garage");
    expect(model.selectedNvr?.rooms[0]?.channels.map((channel) => channel.deviceId)).toEqual([
      "east20_nvr_channel_02",
    ]);
    expect(model.selectedNvr?.rooms.flatMap((room) => room.channels).every((channel) => channel.rootDeviceId === "east20_nvr")).toBe(true);
    expect(model.nvrs.find((nvr) => nvr.deviceId === "west20_nvr")?.rooms[0]?.label).toBe(
      "West Entry",
    );
  });
});
