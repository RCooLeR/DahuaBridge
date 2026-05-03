import { LitElement, html, type PropertyValues, type TemplateResult } from "lit";
import noUiSlider, { type API as NoUiSliderInstance } from "nouislider";
import "nouislider/dist/nouislider.css";

import type { NvrArchiveCoverageChunkModel } from "../domain/archive";

interface ArchiveSeekWindowChangeDetail {
  startIndex: number;
  endIndex: number;
}

interface ArchiveSeekChangeDetail {
  index: number;
}

export class DahuaBridgeArchiveSeek extends LitElement {
  static properties = {
    chunks: { attribute: false },
    windowStartIndex: { type: Number },
    windowEndIndex: { type: Number },
    activeIndex: { type: Number },
    disabled: { type: Boolean },
  } as const;

  chunks: readonly NvrArchiveCoverageChunkModel[] = [];
  windowStartIndex = 0;
  windowEndIndex = 0;
  activeIndex = 0;
  disabled = false;

  private _windowSlider: NoUiSliderInstance | null = null;
  private _seekSlider: NoUiSliderInstance | null = null;

  createRenderRoot(): this {
    return this;
  }

  protected firstUpdated(_changedProperties: PropertyValues): void {
    this.syncSliders();
  }

  protected updated(changedProperties: PropertyValues): void {
    if (
      changedProperties.has("chunks") ||
      changedProperties.has("windowStartIndex") ||
      changedProperties.has("windowEndIndex") ||
      changedProperties.has("activeIndex") ||
      changedProperties.has("disabled")
    ) {
      this.syncSliders();
    }
  }

  disconnectedCallback(): void {
    this.destroySliders();
    super.disconnectedCallback();
  }

  render(): TemplateResult {
    const windowStart = this.chunks[this.windowStartIndex] ?? null;
    const windowEnd = this.chunks[this.windowEndIndex] ?? null;
    const activeChunk = this.chunks[this.activeIndex] ?? null;

    return html`
      <div class="archive-seek-widget">
        <div class="archive-seek-group">
          <div class="split-row muted">
            <span>Visible window</span>
            <span>${windowStart ? formatChunkLabel(windowStart.startTime) : ""}</span>
          </div>
          <div class="archive-seek-slider archive-seek-slider-window"></div>
          <div class="split-row muted">
            <span>${windowStart ? formatChunkLabel(windowStart.startTime) : ""}</span>
            <span>${windowEnd ? formatChunkLabel(windowEnd.endTime || windowEnd.startTime) : ""}</span>
          </div>
        </div>
        <div class="archive-seek-group">
          <div class="split-row muted">
            <span>Playback point</span>
            <span>${activeChunk ? formatChunkLabel(activeChunk.startTime) : ""}</span>
          </div>
          <div class="archive-seek-slider archive-seek-slider-point"></div>
          <div class="split-row muted">
            <span>${windowStart ? formatChunkLabel(windowStart.startTime) : ""}</span>
            <span>${windowEnd ? formatChunkLabel(windowEnd.endTime || windowEnd.startTime) : ""}</span>
          </div>
        </div>
      </div>
    `;
  }

  private syncSliders(): void {
    const chunks = this.chunks;
    const maxIndex = Math.max(chunks.length - 1, 0);
    const nextWindowStart = clampIndex(this.windowStartIndex, maxIndex);
    const nextWindowEnd = Math.max(nextWindowStart, clampIndex(this.windowEndIndex, maxIndex));
    const nextActive = Math.min(
      Math.max(clampIndex(this.activeIndex, maxIndex), nextWindowStart),
      nextWindowEnd,
    );

    const windowHost = this.querySelector<HTMLDivElement>(".archive-seek-slider-window");
    const seekHost = this.querySelector<HTMLDivElement>(".archive-seek-slider-point");
    if (!windowHost || !seekHost || chunks.length === 0) {
      this.destroySliders();
      return;
    }

    if (!this._windowSlider) {
      this._windowSlider = noUiSlider.create(windowHost, {
        start: [nextWindowStart, nextWindowEnd],
        connect: true,
        step: 1,
        range: { min: 0, max: maxIndex },
        behaviour: "drag-tap",
      });
      this._windowSlider.on("change", (values) => {
        const [startIndex, endIndex] = normalizeRangeValues(values, maxIndex);
        this.dispatchEvent(
          new CustomEvent<ArchiveSeekWindowChangeDetail>("archive-window-change", {
            detail: { startIndex, endIndex },
            bubbles: true,
            composed: true,
          }),
        );
      });
    } else {
      this._windowSlider.updateOptions(
        {
          range: { min: 0, max: maxIndex },
          start: [nextWindowStart, nextWindowEnd],
          step: 1,
        },
        true,
      );
      this._windowSlider.set([nextWindowStart, nextWindowEnd]);
    }

    if (!this._seekSlider) {
      this._seekSlider = noUiSlider.create(seekHost, {
        start: nextActive,
        connect: [true, false],
        step: 1,
        range: { min: nextWindowStart, max: nextWindowEnd },
        behaviour: "tap-drag",
      });
      this._seekSlider.on("change", (values) => {
        const [index] = normalizeRangeValues(values, maxIndex);
        this.dispatchEvent(
          new CustomEvent<ArchiveSeekChangeDetail>("archive-seek-change", {
            detail: { index },
            bubbles: true,
            composed: true,
          }),
        );
      });
    } else {
      this._seekSlider.updateOptions(
        {
          range: { min: nextWindowStart, max: nextWindowEnd },
          start: nextActive,
          step: 1,
        },
        true,
      );
      this._seekSlider.set(nextActive);
    }

    windowHost.toggleAttribute("data-disabled", this.disabled);
    seekHost.toggleAttribute("data-disabled", this.disabled);
    if (this.disabled) {
      windowHost.setAttribute("aria-disabled", "true");
      seekHost.setAttribute("aria-disabled", "true");
    } else {
      windowHost.removeAttribute("aria-disabled");
      seekHost.removeAttribute("aria-disabled");
    }
  }

  private destroySliders(): void {
    this._windowSlider?.destroy();
    this._seekSlider?.destroy();
    this._windowSlider = null;
    this._seekSlider = null;
  }
}

function normalizeRangeValues(
  values: (string | number)[],
  maxIndex: number,
): [number, number] {
  const first = clampIndex(Number(values[0] ?? 0), maxIndex);
  const second = clampIndex(Number(values[1] ?? values[0] ?? 0), maxIndex);
  return [Math.min(first, second), Math.max(first, second)];
}

function clampIndex(value: number, maxIndex: number): number {
  const safeValue = Number.isFinite(value) ? Math.trunc(value) : 0;
  return Math.min(Math.max(safeValue, 0), Math.max(maxIndex, 0));
}

function formatChunkLabel(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

if (!customElements.get("dahuabridge-archive-seek")) {
  customElements.define("dahuabridge-archive-seek", DahuaBridgeArchiveSeek);
}
