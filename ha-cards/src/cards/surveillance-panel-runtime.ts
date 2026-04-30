import type { PanelModel, PanelSelection } from "../domain/model";
import { buildPanelModel } from "../domain/model";
import { fetchBridgeEvents, type BridgeEvent, type BridgeEventQuery } from "../ha/bridge-events";
import { fetchRegistrySnapshot, type RegistrySnapshot } from "../ha/registry";
import type { SurveillancePanelCardConfig } from "../types/card-config";
import type { HomeAssistant } from "../types/home-assistant";

interface BridgeEventRequest {
  eventsUrl: string;
  query: BridgeEventQuery;
}

interface SurveillancePanelRuntimeHost {
  isConnected(): boolean;
  getHass(): HomeAssistant | undefined;
  getConfig(): SurveillancePanelCardConfig | undefined;
  getSelection(): PanelSelection;
  getEventWindowHours(): number;
  getRegistrySnapshot(): RegistrySnapshot | null;
  setRegistrySnapshot(snapshot: RegistrySnapshot | null): void;
  setBridgeEvents(events: BridgeEvent[] | null): void;
  setEventsLoading(loading: boolean): void;
  setEventError(message: string): void;
  requestUpdate(): void;
}

export class SurveillancePanelRuntime {
  private eventPollHandle?: number;
  private eventAbort?: AbortController;

  constructor(private readonly host: SurveillancePanelRuntimeHost) {}

  connected(): void {
    void this.refreshRegistrySnapshot();
    this.restartEventPolling();
  }

  disconnected(): void {
    this.stopEventPolling();
  }

  handleHassChanged(): void {
    void this.refreshRegistrySnapshot();
  }

  handleEventContextChanged(): void {
    this.restartEventPolling();
  }

  needsPollingRestartAfterHassChange(): boolean {
    return this.eventPollHandle === undefined;
  }

  logInitializedModels(_model: PanelModel): void {}

  private restartEventPolling(): void {
    this.stopEventPolling();

    if (!this.host.isConnected()) {
      return;
    }

    const hass = this.host.getHass();
    const config = this.host.getConfig();
    if (!hass || !config) {
      return;
    }

    void this.refreshBridgeEvents();
    this.eventPollHandle = window.setInterval(() => {
      void this.refreshBridgeEvents();
    }, this.bridgeEventPollMs(config));
  }

  private stopEventPolling(): void {
    this.eventAbort?.abort();
    this.eventAbort = undefined;

    if (this.eventPollHandle !== undefined) {
      window.clearInterval(this.eventPollHandle);
      this.eventPollHandle = undefined;
    }
  }

  private async refreshRegistrySnapshot(): Promise<void> {
    const hass = this.host.getHass();
    if (!hass?.callWS && !hass?.connection?.sendMessagePromise) {
      this.host.setRegistrySnapshot(null);
      console.warn(
        "[DahuaBridgeCard] Home Assistant registry websocket API is unavailable. Room mapping will fall back to Unassigned.",
      );
      return;
    }

    try {
      const registrySnapshot = await fetchRegistrySnapshot(hass);
      if (registrySnapshot === null) {
        this.host.setRegistrySnapshot(null);
        console.warn(
          "[DahuaBridgeCard] Home Assistant registry snapshot returned no data. Room mapping will fall back to Unassigned.",
        );
        this.host.requestUpdate();
        return;
      }

      this.host.setRegistrySnapshot(registrySnapshot);
      this.host.requestUpdate();
    } catch (error) {
      this.host.setRegistrySnapshot(null);
      console.warn(
        "[DahuaBridgeCard] Failed to load Home Assistant registry snapshot. Room mapping will fall back to Unassigned.",
        error,
      );
      this.host.requestUpdate();
    }
  }

  private bridgeEventPollMs(config: SurveillancePanelCardConfig): number {
    const seconds = config.bridge_event_poll_seconds ?? 15;
    return seconds * 1000;
  }

  private async refreshBridgeEvents(): Promise<void> {
    const request = this.bridgeEventRequest();
    if (!request) {
      this.host.setBridgeEvents(null);
      this.host.setEventError("");
      this.host.setEventsLoading(false);
      return;
    }

    this.eventAbort?.abort();
    const controller = new AbortController();
    this.eventAbort = controller;
    this.host.setEventsLoading(true);

    try {
      const events = await fetchBridgeEvents(request.eventsUrl, request.query, controller.signal);
      if (this.eventAbort !== controller) {
        return;
      }

      this.host.setBridgeEvents(events);
      this.host.setEventError("");
    } catch (error) {
      if (controller.signal.aborted || this.eventAbort !== controller) {
        return;
      }

      this.host.setEventError(
        error instanceof Error ? error.message : "Bridge event sync failed.",
      );
    } finally {
      if (this.eventAbort === controller) {
        this.eventAbort = undefined;
        this.host.setEventsLoading(false);
      }
    }
  }

  private bridgeEventRequest(): BridgeEventRequest | null {
    const hass = this.host.getHass();
    const config = this.host.getConfig();
    if (!hass || !config) {
      return null;
    }

    const model = buildPanelModel(
      hass,
      config,
      this.host.getSelection(),
      undefined,
      this.host.getEventWindowHours(),
      this.host.getRegistrySnapshot(),
    );
    const limit = Math.max((config.max_events ?? 14) * 12, 120);

    if (model.selectedCamera?.eventsUrl) {
      return {
        eventsUrl: model.selectedCamera.eventsUrl,
        query: {
          limit,
          childId: model.selectedCamera.deviceId,
        },
      };
    }

    if (model.selectedNvr?.eventsUrl) {
      return {
        eventsUrl: model.selectedNvr.eventsUrl,
        query: {
          limit,
          deviceId: model.selectedNvr.deviceId,
        },
      };
    }

    if (model.selectedVto?.eventsUrl) {
      return {
        eventsUrl: model.selectedVto.eventsUrl,
        query: {
          limit,
          deviceId: model.selectedVto.deviceId,
        },
      };
    }

    const eventsUrl = model.vtos[0]?.eventsUrl ?? model.cameras[0]?.eventsUrl;
    if (!eventsUrl) {
      return null;
    }

    return {
      eventsUrl,
      query: { limit },
    };
  }
}
