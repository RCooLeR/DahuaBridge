import { css, html, LitElement } from "lit";
import type { TemplateResult } from "lit";

import { parseConfig, type SurveillancePanelCardConfig } from "../types/card-config";
import type {
  HomeAssistant,
  LovelaceCardConfig,
  LovelaceCardEditor,
} from "../types/home-assistant";

type DraftConfig = Partial<SurveillancePanelCardConfig> & LovelaceCardConfig;

export function createSurveillancePanelStubConfig(): SurveillancePanelCardConfig {
  return {
    type: "custom:dahuabridge-surveillance-panel",
    title: "DahuaBridge Surveillance",
    subtitle: "Full-panel command center",
    event_lookback_hours: 12,
    bridge_event_poll_seconds: 15,
    max_events: 14,
  };
}

export class DahuaBridgeSurveillancePanelCardEditor
  extends LitElement
  implements LovelaceCardEditor
{
  static properties = {
    hass: { attribute: false },
    _config: { state: true },
    _errorMessage: { state: true },
  } as const;

  static styles = css`
    :host {
      display: block;
    }

    .editor {
      display: grid;
      gap: 16px;
      padding: 8px 0;
    }

    .section {
      display: grid;
      gap: 10px;
      padding: 14px;
      border: 1px solid rgba(255, 255, 255, 0.12);
      border-radius: 8px;
      background: rgba(255, 255, 255, 0.02);
    }

    .section-title {
      font-size: 14px;
      font-weight: 600;
    }

    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 12px;
    }

    label {
      display: grid;
      gap: 6px;
      font-size: 13px;
      font-weight: 500;
    }

    input {
      width: 100%;
      padding: 10px 12px;
      border: 1px solid rgba(255, 255, 255, 0.18);
      border-radius: 8px;
      background: rgba(0, 0, 0, 0.16);
      color: inherit;
      font: inherit;
      box-sizing: border-box;
    }

    .hint {
      font-size: 12px;
      opacity: 0.72;
      line-height: 1.4;
    }

    .error {
      padding: 10px 12px;
      border-radius: 8px;
      border: 1px solid rgba(255, 95, 121, 0.35);
      background: rgba(91, 19, 37, 0.22);
      color: #ffcad4;
      font-size: 12px;
    }
  `;

  hass?: HomeAssistant;

  private _config: DraftConfig = createSurveillancePanelStubConfig();
  private _errorMessage = "";

  setConfig(config: LovelaceCardConfig): void {
    this._config = {
      ...createSurveillancePanelStubConfig(),
      ...config,
      type: "custom:dahuabridge-surveillance-panel",
    };
    this._errorMessage = "";
  }

  render(): TemplateResult {
    return html`
      <div class="editor">
        ${this._errorMessage ? html`<div class="error">${this._errorMessage}</div>` : null}

        <div class="section">
          <div class="section-title">General</div>
          <div class="grid">
            ${this.renderTextInput("Title", this._config.title ?? "", (value) =>
              this.updateField("title", value || undefined),
            )}
            ${this.renderTextInput("Subtitle", this._config.subtitle ?? "", (value) =>
              this.updateField("subtitle", value || undefined),
            )}
            ${this.renderTextInput(
              "Browser Bridge URL",
              this._config.browser_bridge_url ?? "",
              (value) => this.updateField("browser_bridge_url", value || undefined),
            )}
            ${this.renderNumberInput(
              "Lookback Hours",
              this._config.event_lookback_hours,
              (value) => this.updateField("event_lookback_hours", value),
            )}
            ${this.renderNumberInput(
              "Poll Seconds",
              this._config.bridge_event_poll_seconds,
              (value) => this.updateField("bridge_event_poll_seconds", value),
            )}
            ${this.renderNumberInput("Visible Events", this._config.max_events, (value) =>
              this.updateField("max_events", value),
            )}
            ${this.renderTextInput(
              "Preferred VTO",
              this._config.vto?.device_id ?? "",
              (value) => this.updateVtoField("device_id", value || undefined),
            )}
          </div>
        </div>

        <div class="section">
          <div class="section-title">Discovery</div>
          <div class="hint">
            The card discovers DahuaBridge VTOs, NVR channels, rooms, sensors, and event surfaces
            from Home Assistant and the bridge-native camera metadata. Room grouping follows the HA
            area assigned to each channel device.
          </div>
          <div class="hint">
            Set Browser Bridge URL when the browser reaches the bridge through a public hostname
            instead of the bridge's internal public base URL.
          </div>
        </div>
      </div>
    `;
  }

  private renderTextInput(
    label: string,
    value: string,
    onChange: (value: string) => void,
  ): TemplateResult {
    return html`
      <label>
        <span>${label}</span>
        <input
          type="text"
          .value=${value}
          @change=${(event: Event) =>
            onChange((event.currentTarget as HTMLInputElement).value.trim())}
        />
      </label>
    `;
  }

  private renderNumberInput(
    label: string,
    value: number | undefined,
    onChange: (value: number | undefined) => void,
  ): TemplateResult {
    return html`
      <label>
        <span>${label}</span>
        <input
          type="number"
          .value=${value === undefined ? "" : String(value)}
          @change=${(event: Event) =>
            onChange(this.parseOptionalNumber((event.currentTarget as HTMLInputElement).value))}
        />
      </label>
    `;
  }

  private parseOptionalNumber(value: string): number | undefined {
    const text = value.trim();
    if (!text) {
      return undefined;
    }

    const parsed = Number.parseInt(text, 10);
    return Number.isFinite(parsed) ? parsed : undefined;
  }

  private updateField<Key extends keyof SurveillancePanelCardConfig>(
    key: Key,
    value: SurveillancePanelCardConfig[Key] | undefined,
  ): void {
    const nextConfig: DraftConfig = {
      ...this._config,
      type: "custom:dahuabridge-surveillance-panel",
      [key]: value,
    };

    if (value === undefined) {
      delete nextConfig[key];
    }

    this.commitConfig(nextConfig);
  }

  private updateVtoField(
    key: "device_id",
    value: string | undefined,
  ): void {
    const nextVto = {
      ...(this._config.vto ?? {}),
      [key]: value,
    };
    if (value === undefined) {
      delete nextVto[key];
    }

    const nextConfig: DraftConfig = {
      ...this._config,
      type: "custom:dahuabridge-surveillance-panel",
      vto: Object.keys(nextVto).length > 0 ? nextVto : undefined,
    };
    if (!nextConfig.vto) {
      delete nextConfig.vto;
    }

    this.commitConfig(nextConfig);
  }

  private commitConfig(config: DraftConfig): void {
    try {
      const parsed = parseConfig({
        ...config,
        type: "custom:dahuabridge-surveillance-panel",
      });
      this._config = parsed;
      this._errorMessage = "";
      this.dispatchEvent(
        new CustomEvent("config-changed", {
          detail: { config: parsed },
          bubbles: true,
          composed: true,
        }),
      );
    } catch (error) {
      this._config = config;
      this._errorMessage =
        error instanceof Error ? error.message : "Card configuration is invalid.";
    }
  }
}

if (!customElements.get("dahuabridge-surveillance-panel-editor")) {
  customElements.define(
    "dahuabridge-surveillance-panel-editor",
    DahuaBridgeSurveillancePanelCardEditor,
  );
}
