import {
  postBridgeRequest,
  pressButton,
  readBridgeJson,
  setNumberValue,
  toggleSwitch,
} from "../ha/actions";
import {
  buildPtzUrl,
  buildAuxUrl,
  findAuxTarget,
  resolveAuxTargetAction,
  resolveCameraRecordingActionUrl,
  type CameraViewModel,
} from "../domain/model";
import type { HomeAssistant } from "../types/home-assistant";

interface SurveillancePanelActionHost {
  getHass(): HomeAssistant | undefined;
  getBusyActions(): ReadonlySet<string>;
  setBusyActions(next: Set<string>): void;
  setError(message: string): void;
}

export class SurveillancePanelActions {
  constructor(private readonly host: SurveillancePanelActionHost) {}

  isBusy(key: string): boolean {
    return this.host.getBusyActions().has(key);
  }

  async triggerVtoButtonAction(
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ): Promise<void> {
    const hass = this.host.getHass();
    if (!hass) {
      return;
    }

    if (this.entityExists(entityId)) {
      await this.runAction(key, async () => {
        await pressButton(hass, entityId);
      });
      return;
    }

    if (!fallbackUrl) {
      this.host.setError(
        "Control is unavailable in Home Assistant and no bridge fallback URL was provided.",
      );
      return;
    }

    await this.runAction(key, async () => {
      await postBridgeRequest(fallbackUrl);
    });
  }

  async triggerVtoSwitchAction(
    key: string,
    entityId: string,
    enabled: boolean,
    fallbackUrl: string | null,
    payloadKey: string,
  ): Promise<void> {
    const hass = this.host.getHass();
    if (!hass) {
      return;
    }

    if (this.entityExists(entityId)) {
      await this.runAction(key, async () => {
        await toggleSwitch(hass, entityId, enabled);
      });
      return;
    }

    if (!fallbackUrl) {
      this.host.setError(
        "Switch control is unavailable in Home Assistant and no bridge fallback URL was provided.",
      );
      return;
    }

    await this.runAction(key, async () => {
      await postBridgeRequest(fallbackUrl, {
        body: {
          [payloadKey]: enabled,
        },
      });
    });
  }

  async handleVtoRangeChange(
    event: Event,
    key: string,
    entityId: string,
    fallbackUrl: string | null,
  ): Promise<void> {
    const hass = this.host.getHass();
    if (!hass) {
      return;
    }

    const target = event.currentTarget as HTMLInputElement;
    const value = Number.parseFloat(target.value);
    if (!Number.isFinite(value)) {
      return;
    }

    if (this.entityExists(entityId)) {
      await this.runAction(key, async () => {
        await setNumberValue(hass, entityId, value);
      });
      return;
    }

    if (!fallbackUrl) {
      this.host.setError(
        "Volume control is unavailable in Home Assistant and no bridge fallback URL was provided.",
      );
      return;
    }

    await this.runAction(key, async () => {
      await postBridgeRequest(fallbackUrl, {
        body: {
          slot: 0,
          level: Math.round(value),
        },
      });
    });
  }

  async triggerPtzAction(
    camera: CameraViewModel,
    command: string,
  ): Promise<void> {
    const targetUrl = buildPtzUrl(camera);
    if (!targetUrl) {
      this.host.setError(
        "PTZ target URL is unavailable. The card needs bridge-derived camera attributes for direct PTZ control.",
      );
      return;
    }

    await this.runAction(`${camera.deviceId}:ptz:${command}`, async () => {
      await postBridgeRequest(targetUrl, {
        body:
          command === "stop"
            ? { command: "up", action: "stop" }
            : { command, action: "pulse", duration_ms: 450 },
      });
    });
  }

  async triggerAuxAction(
    camera: CameraViewModel,
    output: string,
    active: boolean,
  ): Promise<boolean> {
    const target = findAuxTarget(camera, output);
    const targetUrl = target?.url ?? buildAuxUrl(camera);
    if (!targetUrl) {
      this.host.setError(
        "Aux output URL is unavailable. The bridge base URL could not be derived from the camera entity.",
      );
      return false;
    }

    const action = resolveAuxTargetAction(target, active);
    if (!action) {
      this.host.setError("No compatible deterrence action is available for that output.");
      return false;
    }

    return this.runAction(`${camera.deviceId}:aux:${output}`, async () => {
      await postBridgeRequest(targetUrl, {
        body: {
          [target?.parameterKey ?? "output"]: target?.parameterValue ?? output,
          action,
          ...(action === "pulse" ? { duration_ms: 3000 } : {}),
        },
      });
    });
  }

  async triggerRecordingAction(
    camera: CameraViewModel,
    action: "start" | "stop",
  ): Promise<boolean> {
    const targetUrl =
      action === "stop"
        ? await this.resolveActiveRecordingStopUrl(camera)
        : resolveCameraRecordingActionUrl(camera, action);
    if (!targetUrl) {
      this.host.setError(
        action === "stop"
          ? "No active bridge MP4 clip is available to stop for this camera."
          : "Bridge MP4 recording is unavailable for this camera.",
      );
      return false;
    }

    return this.runAction(`${camera.deviceId}:recording:${action}`, async () => {
      await postBridgeRequest(targetUrl);
    });
  }

  private entityExists(entityId: string): boolean {
    return Boolean(this.host.getHass()?.states[entityId]);
  }

  private async resolveActiveRecordingStopUrl(
    camera: CameraViewModel,
  ): Promise<string | null> {
    const directUrl = resolveCameraRecordingActionUrl(camera, "stop");
    if (!camera.recordingsUrl) {
      return directUrl;
    }

    try {
      const payload = await readBridgeJson<BridgeRecordingsListResponse>(camera.recordingsUrl);
      const activeClip =
        payload.items.find((item) => item.status.trim().toLowerCase() === "recording") ?? null;
      if (activeClip?.stop_url) {
        return activeClip.stop_url;
      }
    } catch {
      return directUrl;
    }

    return directUrl;
  }

  private async runAction(
    key: string,
    action: () => Promise<void>,
  ): Promise<boolean> {
    if (this.isBusy(key)) {
      return false;
    }

    const nextBusy = new Set(this.host.getBusyActions());
    nextBusy.add(key);
    this.host.setBusyActions(nextBusy);
    this.host.setError("");

    try {
      await action();
      return true;
    } catch (error) {
      this.host.setError(
        error instanceof Error ? error.message : "Unexpected action failure.",
      );
      return false;
    } finally {
      const reducedBusy = new Set(this.host.getBusyActions());
      reducedBusy.delete(key);
      this.host.setBusyActions(reducedBusy);
    }
  }
}

interface BridgeRecordingListItem {
  status: string;
  stop_url?: string;
}

interface BridgeRecordingsListResponse {
  items: BridgeRecordingListItem[];
}
