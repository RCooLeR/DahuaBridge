import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import type { TimelineEvent } from "../domain/events";
import {
  EVENT_FILTER_ALL,
  type TimelineEventFilterOptions,
  type TimelineEventFilters,
} from "./surveillance-panel-state";
import { formatEventTime } from "../utils/format";

type EventViewMode = "recent" | "history";

export interface EventWindowOption {
  hours: number;
  label: string;
}

interface RenderSurveillancePanelEventsArgs {
  eventViewMode: EventViewMode;
  eventsLoading: boolean;
  eventError: string;
  visibleEvents: TimelineEvent[];
  filteredEventCount: number;
  totalEventCount: number;
  historyPageCount: number;
  clampedHistoryPage: number;
  eventWindowHours: number;
  eventWindowOptions: readonly EventWindowOption[];
  selectedFilters: TimelineEventFilters;
  filterOptions: TimelineEventFilterOptions;
  onSelectEventMode: (mode: EventViewMode) => void;
  onSelectEventWindow: (hours: number) => void;
  onSelectFilter: (
    key: keyof TimelineEventFilters,
    value: string,
  ) => void;
  onResetFilters: () => void;
  onHistoryPageInput: (event: Event) => void;
  renderIcon: (icon: string) => TemplateResult;
}

export function renderSurveillancePanelEvents({
  eventViewMode,
  eventsLoading,
  eventError,
  visibleEvents,
  filteredEventCount,
  totalEventCount,
  historyPageCount,
  clampedHistoryPage,
  eventWindowHours,
  eventWindowOptions,
  selectedFilters,
  filterOptions,
  onSelectEventMode,
  onSelectEventWindow,
  onSelectFilter,
  onResetFilters,
  onHistoryPageInput,
  renderIcon,
}: RenderSurveillancePanelEventsArgs): TemplateResult {
  const filtersActive = selectedFilters.eventCode !== EVENT_FILTER_ALL;

  return html`
    <section class="events">
      <div class="events-toolbar">
        <div class="panel-title">
          <span>${eventViewMode === "history" ? "Event History" : "Recent Events"}</span>
          <span class="muted">
            ${eventsLoading
              ? "Syncing"
              : eventError
                ? "Bridge sync degraded"
                : `${filteredEventCount}/${totalEventCount} shown`}
          </span>
        </div>
        <div class="chip-row">
          ${renderEventModeChip("recent", "Recent", eventViewMode, onSelectEventMode)}
          ${renderEventModeChip("history", "History", eventViewMode, onSelectEventMode)}
        </div>
      </div>
      <div class="event-toolbar-row">
        <div class="chip-row">
          ${repeat(
            eventWindowOptions,
            (option) => option.hours,
            (option) =>
              renderEventWindowChip(
                option.hours,
                option.label,
                eventWindowHours,
                onSelectEventWindow,
              ),
          )}
        </div>
        ${eventViewMode === "history" && historyPageCount > 1
          ? html`
              <div class="history-range">
                <div class="event-range-meta">
                  <span class="muted">Buffered history</span>
                  <span class="badge">${clampedHistoryPage + 1}/${historyPageCount}</span>
                </div>
                <input
                  type="range"
                  min="0"
                  .max=${String(historyPageCount - 1)}
                  step="1"
                  .value=${String(clampedHistoryPage)}
                  @input=${onHistoryPageInput}
                />
              </div>
            `
          : nothing}
      </div>
      <div class="event-filter-grid">
        ${renderFilterSelect(
          "Event type",
          "eventCode",
          selectedFilters.eventCode,
          filterOptions.eventCodes,
          onSelectFilter,
        )}
      </div>
      ${filtersActive
        ? html`
            <div class="chip-row">
              <button class="chip" type="button" @click=${onResetFilters}>Reset filters</button>
            </div>
          `
        : nothing}
      ${eventError ? html`<div class="muted">${eventError}</div>` : nothing}
      <div class="events-list">
        ${visibleEvents.length === 0
          ? html`
              <div class="event-card">
                <div class="muted">
                  ${filtersActive ? "No events match the current filters." : "No recent events."}
                </div>
              </div>
            `
          : repeat(
              visibleEvents,
              (event) => event.id,
              (event) => renderTimelineEventCard(event, renderIcon, true),
            )}
      </div>
    </section>
  `;
}

function renderFilterSelect(
  label: string,
  key: keyof TimelineEventFilters,
  value: string,
  options: TimelineEventFilterOptions[keyof TimelineEventFilterOptions],
  onSelectFilter: (
    key: keyof TimelineEventFilters,
    value: string,
  ) => void,
): TemplateResult {
  return html`
    <label class="event-filter">
      <span class="event-filter-label">${label}</span>
      <select
        class="event-filter-select"
        .value=${value}
        @change=${(event: Event) =>
          onSelectFilter(key, (event.currentTarget as HTMLSelectElement).value)}
      >
        ${options.map(
          (option) => html`<option value=${option.value}>${option.label}</option>`,
        )}
      </select>
    </label>
  `;
}

function renderEventModeChip(
  mode: EventViewMode,
  label: string,
  activeMode: EventViewMode,
  onSelectEventMode: (mode: EventViewMode) => void,
): TemplateResult {
  return html`
    <button
      class="chip ${activeMode === mode ? "active" : ""}"
      type="button"
      title=${label}
      @click=${() => onSelectEventMode(mode)}
    >
      ${label}
    </button>
  `;
}

function renderEventWindowChip(
  hours: number,
  label: string,
  activeHours: number,
  onSelectEventWindow: (hours: number) => void,
): TemplateResult {
  return html`
    <button
      class="chip ${activeHours === hours ? "active" : ""}"
      type="button"
      title=${label}
      @click=${() => onSelectEventWindow(hours)}
    >
      ${label}
    </button>
  `;
}

function renderTimelineEventCard(
  event: TimelineEvent,
  renderIcon: (icon: string) => TemplateResult,
  compact = false,
): TemplateResult {
  return html`
    <div class="event-card ${event.severity}">
      <div class="event-card-body">
        <div class="split-row">
          <span class="badge ${event.severity}">
            ${renderIcon(event.icon)}
            ${event.title}
          </span>
          <span class="muted">${formatEventTime(event.timestamp)}</span>
        </div>
        ${!compact || event.sourceCode
          ? html`
              <div class="event-meta">
                <span class="badge">${event.actionText}</span>
                ${event.sourceCode ? html`<span class="badge">${event.sourceCode}</span>` : nothing}
              </div>
            `
          : nothing}
      </div>
    </div>
  `;
}
