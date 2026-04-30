import { html, type TemplateResult } from "lit";

export type ControlTone = "neutral" | "primary" | "warning" | "danger";

export function renderControlButton(
  label: string,
  icon: string,
  onClick: () => void,
  renderIcon: (icon: string) => TemplateResult,
  options: {
    disabled?: boolean;
    tone?: ControlTone;
    compact?: boolean;
    active?: boolean;
  } = {},
): TemplateResult {
  const toneClass =
    options.tone === "primary"
      ? "primary"
      : options.tone === "warning"
        ? "warning"
        : options.tone === "danger"
          ? "danger"
          : "";
  const handleClick = (event: Event): void => {
    event.preventDefault();
    event.stopPropagation();
    onClick();
  };

  return html`
    <button
      class="control-button ${toneClass}"
      data-compact=${options.compact ? "true" : "false"}
      data-active=${options.active ? "true" : "false"}
      type="button"
      title=${label}
      ?disabled=${Boolean(options.disabled)}
      aria-pressed=${String(Boolean(options.active))}
      @click=${handleClick}
    >
      ${renderIcon(icon)}
      ${label}
    </button>
  `;
}

export function renderIconButton(
  title: string,
  icon: string,
  onClick: () => void,
  renderIcon: (icon: string) => TemplateResult,
  options: {
    disabled?: boolean;
    tone?: ControlTone;
    active?: boolean;
  } = {},
): TemplateResult {
  const toneClass =
    options.tone === "primary"
      ? "primary"
      : options.tone === "warning"
        ? "warning"
        : options.tone === "danger"
          ? "danger"
          : "";
  return html`
    <button
      class="icon-button ${toneClass}"
      type="button"
      title=${title}
      ?disabled=${Boolean(options.disabled)}
      data-active=${options.active ? "true" : "false"}
      @click=${(event: Event) => {
        event.preventDefault();
        event.stopPropagation();
        onClick();
      }}
      aria-label=${title}
      aria-pressed=${String(Boolean(options.active))}
    >
      ${renderIcon(icon)}
    </button>
  `;
}

export function renderKv(label: string, value: string): TemplateResult {
  return html`
    <div class="kv">
      <div class="kv-label">${label}</div>
      <div class="kv-value">${value}</div>
    </div>
  `;
}

export function renderSegmentButton(
  key: string,
  label: string,
  activeKey: string,
  onSelect: (key: string) => void,
): TemplateResult {
  const handleClick = (event: Event): void => {
    event.preventDefault();
    event.stopPropagation();
    onSelect(key);
  };
  return html`
    <button
      class="segment ${activeKey === key ? "active" : ""}"
      type="button"
      title=${label}
      @click=${handleClick}
    >
      ${label}
    </button>
  `;
}
