import { css } from "lit";

export const surveillancePanelBaseStyles = css`
  :host {
    display: block;
    height: 100%;
    --db-bg: #07111b;
    --db-panel: rgba(8, 20, 33, 0.88);
    --db-panel-strong: rgba(10, 25, 41, 0.96);
    --db-panel-soft: rgba(12, 29, 45, 0.72);
    --db-border: rgba(106, 199, 255, 0.18);
    --db-border-strong: rgba(106, 199, 255, 0.32);
    --db-text: #ecf6ff;
    --db-text-soft: rgba(215, 234, 250, 0.74);
    --db-cyan: #34d8ff;
    --db-blue: #4b81ff;
    --db-green: #41d98c;
    --db-amber: #f8a11d;
    --db-purple: #bf7bff;
    --db-red: #ff5f79;
    --db-shadow: 0 22px 48px rgba(0, 0, 0, 0.36);
  }

  * {
    box-sizing: border-box;
  }

  ha-card {
    width: 100%;
    height: auto;
    aspect-ratio: 16 / 9;
    max-height: calc(100vh - 96px);
    min-height: min(760px, calc(100vh - 96px));
    background:
      radial-gradient(circle at top right, rgba(75, 129, 255, 0.12), transparent 36%),
      radial-gradient(circle at top left, rgba(52, 216, 255, 0.12), transparent 32%),
      linear-gradient(180deg, rgba(6, 16, 26, 0.98), rgba(5, 12, 20, 0.98));
    color: var(--db-text);
    border: 1px solid var(--db-border);
    border-radius: 8px;
    overflow: hidden;
    box-shadow: var(--db-shadow);
  }

  .shell {
    display: grid;
    grid-template-columns: minmax(300px, 340px) minmax(0, 1fr) minmax(300px, 340px);
    grid-template-rows: auto minmax(0, 1fr);
    grid-template-areas:
      "sidebar header header"
      "sidebar main inspector";
    gap: 16px;
    height: 100%;
    padding: 16px;
    background: rgba(4, 12, 20, 0.9);
  }

  .shell.sidebar-collapsed {
    grid-template-columns: minmax(0, 1fr) minmax(300px, 340px);
    grid-template-areas:
      "header header"
      "main inspector";
  }

  .shell.inspector-collapsed {
    grid-template-columns: minmax(300px, 340px) minmax(0, 1fr);
    grid-template-areas:
      "sidebar header"
      "sidebar main";
  }

  .shell.sidebar-collapsed.inspector-collapsed {
    grid-template-columns: minmax(0, 1fr);
    grid-template-areas:
      "header"
      "main";
  }

  .sidebar,
  .header,
  .main,
  .inspector {
    background: var(--db-panel);
    border: 1px solid var(--db-border);
    border-radius: 8px;
    backdrop-filter: blur(18px);
    min-height: 0;
  }

  .sidebar {
    grid-area: sidebar;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .shell.sidebar-collapsed .sidebar {
    display: none;
  }

  .header {
    grid-area: header;
    padding: 18px 20px;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    gap: 18px;
    align-items: center;
  }

  .main {
    grid-area: main;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .inspector {
    grid-area: inspector;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 14px;
    min-height: 0;
  }

  .shell.inspector-collapsed .inspector {
    display: none;
  }

  .section-label {
    font-size: 11px;
    text-transform: uppercase;
    color: var(--db-text-soft);
    letter-spacing: 0.08em;
  }

  .muted {
    color: var(--db-text-soft);
    font-size: 13px;
    overflow-wrap: anywhere;
  }

  .chip-row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .chip,
  .segment {
    height: 32px;
    padding: 0 12px;
    border-radius: 999px;
    border: 1px solid var(--db-border);
    background: rgba(8, 20, 33, 0.72);
    color: var(--db-text-soft);
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    transition: 140ms ease;
  }

  .chip.active,
  .segment.active {
    color: var(--db-text);
    border-color: rgba(52, 216, 255, 0.45);
    background: rgba(32, 107, 140, 0.32);
    box-shadow: inset 0 0 0 1px rgba(52, 216, 255, 0.18);
  }

  .badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-height: 24px;
    padding: 0 10px;
    border-radius: 999px;
    border: 1px solid rgba(106, 199, 255, 0.2);
    background: rgba(8, 24, 38, 0.92);
    color: var(--db-text);
    font-size: 12px;
    font-weight: 600;
    white-space: nowrap;
  }

  .badge.warning {
    border-color: rgba(248, 161, 29, 0.35);
    color: #ffd494;
  }

  .badge.info {
    border-color: rgba(75, 129, 255, 0.35);
    color: #bfd0ff;
  }

  .badge.success {
    border-color: rgba(65, 217, 140, 0.35);
    color: #c5ffe2;
  }

  .badge.critical {
    border-color: rgba(255, 95, 121, 0.35);
    color: #ffc7d1;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--db-green);
    flex: 0 0 auto;
  }

  .status-dot.warning {
    background: var(--db-amber);
  }

  .status-dot.critical {
    background: var(--db-red);
  }

  .recording-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--db-red);
    border: 2px solid rgba(255, 95, 121, 0.42);
    box-shadow:
      0 0 0 1px rgba(9, 20, 31, 0.78),
      0 0 0 4px rgba(255, 95, 121, 0.12);
    flex: 0 0 auto;
  }

  .panel {
    border: 1px solid var(--db-border);
    border-radius: 8px;
    padding: 14px;
    background: rgba(6, 16, 26, 0.72);
    display: grid;
    gap: 12px;
    min-width: 0;
    overflow: visible;
  }

  .panel-title {
    font-size: 14px;
    font-weight: 600;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
    min-width: 0;
  }

  .split-row,
  .control-row {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: center;
  }

  .split-row {
    justify-content: space-between;
  }

  .icon-button {
    width: 36px;
    height: 36px;
    border-radius: 8px;
    border: 1px solid var(--db-border);
    background: rgba(7, 18, 29, 0.84);
    color: var(--db-text);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: 140ms ease;
    padding: 0;
  }

  .control-button,
  .action-button {
    min-height: 40px;
    padding: 0 14px;
    border-radius: 8px;
    border: 1px solid var(--db-border);
    background: rgba(7, 18, 29, 0.84);
    color: var(--db-text);
    font-size: 13px;
    font-weight: 600;
    display: inline-flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    transition: 140ms ease;
  }

  .control-button[data-compact="true"] {
    min-height: 36px;
    padding: 0 12px;
    border-color: rgba(149, 212, 255, 0.24);
    background: rgba(9, 18, 30, 0.48);
    backdrop-filter: blur(16px) saturate(150%);
    -webkit-backdrop-filter: blur(16px) saturate(150%);
    box-shadow:
      0 8px 24px rgba(0, 0, 0, 0.28),
      inset 0 1px 0 rgba(255, 255, 255, 0.06);
  }

  .control-button.primary,
  .action-button.primary {
    background: rgba(23, 78, 104, 0.44);
    border-color: rgba(52, 216, 255, 0.38);
  }

  .control-button.warning,
  .icon-button.warning,
  .action-button.warning {
    border-color: rgba(248, 161, 29, 0.35);
    background: rgba(122, 76, 15, 0.22);
  }

  .control-button.danger,
  .icon-button.danger,
  .action-button.danger {
    border-color: rgba(255, 95, 121, 0.35);
    background: rgba(113, 27, 48, 0.22);
  }

  .control-button[data-active="true"],
  .icon-button[aria-pressed="true"] {
    border-color: rgba(52, 216, 255, 0.45);
    background: rgba(32, 107, 140, 0.32);
    box-shadow: inset 0 0 0 1px rgba(52, 216, 255, 0.18);
  }

  .control-button.danger[data-active="true"],
  .icon-button.danger[data-active="true"] {
    border-color: rgba(255, 95, 121, 0.5);
    background: rgba(113, 27, 48, 0.4);
    box-shadow:
      inset 0 0 0 1px rgba(255, 95, 121, 0.2),
      0 0 0 1px rgba(255, 95, 121, 0.16);
  }

  .control-button.primary[data-active="true"],
  .icon-button.primary[data-active="true"] {
    border-color: rgba(52, 216, 255, 0.52);
    background: rgba(23, 78, 104, 0.56);
  }

  .icon-button:hover:not(:disabled),
  .action-button:hover:not(:disabled),
  .control-button:hover:not(:disabled) {
    border-color: var(--db-border-strong);
    background: rgba(17, 40, 62, 0.88);
  }

  .icon-button:disabled,
  .action-button:disabled,
  .control-button:disabled {
    opacity: 0.48;
    cursor: default;
  }

  .header-chip-icon ha-icon,
  .sidebar-glyph ha-icon,
  .icon-button ha-icon,
  .control-button ha-icon,
  .badge ha-icon {
    color: currentColor;
    filter:
      drop-shadow(0 0 8px rgba(52, 216, 255, 0.5))
      drop-shadow(0 0 16px rgba(75, 129, 255, 0.28));
  }

  .key-value-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .kv {
    border: 1px solid rgba(106, 199, 255, 0.12);
    border-radius: 8px;
    padding: 10px;
    background: rgba(8, 19, 30, 0.76);
    display: grid;
    gap: 4px;
  }

  .kv-label {
    font-size: 11px;
    text-transform: uppercase;
    color: var(--db-text-soft);
    letter-spacing: 0.08em;
  }

  .kv-value {
    font-size: 14px;
    font-weight: 600;
    word-break: break-word;
  }

  .empty-state {
    flex: 1;
    min-height: 0;
    display: grid;
    place-items: center;
    padding: 24px;
    text-align: center;
    color: var(--db-text-soft);
  }

  .error-banner {
    margin: 0 18px 18px;
    padding: 12px 14px;
    border-radius: 8px;
    border: 1px solid rgba(255, 95, 121, 0.35);
    background: rgba(91, 19, 37, 0.42);
    color: #ffd2dc;
    font-size: 13px;
  }

  .toolbar {
    padding: 14px 18px 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
    border-bottom: 1px solid var(--db-border);
  }

  .search {
    width: 100%;
    height: 40px;
    border-radius: 8px;
    border: 1px solid var(--db-border);
    background: rgba(7, 18, 29, 0.9);
    color: var(--db-text);
    padding: 0 14px;
    outline: none;
  }

  .sidebar-scroll {
    flex: 1;
    overflow: auto;
    padding: 10px 12px 16px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .sidebar-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .sidebar-section-head {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .sidebar-section-icon {
    width: 24px;
    height: 24px;
    border-radius: 8px;
    border: 1px solid rgba(106, 199, 255, 0.16);
    background: rgba(10, 25, 40, 0.88);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--db-cyan);
    flex: 0 0 auto;
  }

  .sidebar-nvr {
    display: grid;
    gap: 8px;
    padding: 10px;
    border-radius: 8px;
    border: 1px solid rgba(106, 199, 255, 0.12);
    background: rgba(6, 16, 26, 0.5);
  }

  .sidebar-room-group {
    border: 1px solid rgba(106, 199, 255, 0.1);
    border-radius: 8px;
    background: rgba(8, 20, 33, 0.54);
    overflow: hidden;
  }

  .sidebar-room-group summary {
    list-style: none;
    cursor: pointer;
    padding: 10px 12px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
  }

  .sidebar-room-group summary::-webkit-details-marker {
    display: none;
  }

  .sidebar-room-summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    width: 100%;
    min-width: 0;
  }

  .sidebar-room-meta {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .sidebar-room-toggle {
    color: var(--db-text-soft);
    font-size: 12px;
    flex: 0 0 auto;
    transition: transform 140ms ease;
  }

  .sidebar-room-group[open] .sidebar-room-toggle {
    transform: rotate(180deg);
  }

  .sidebar-room-cameras {
    display: grid;
    gap: 6px;
    padding: 0 8px 8px;
  }

  .sidebar-item {
    width: 100%;
    border: 1px solid transparent;
    border-radius: 8px;
    background: rgba(7, 17, 28, 0.68);
    color: var(--db-text);
    padding: 10px 12px;
    cursor: pointer;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 12px;
    align-items: center;
    text-align: left;
    transition: 140ms ease;
  }

  .sidebar-item:hover,
  .sidebar-item.selected {
    border-color: var(--db-border-strong);
    background: rgba(17, 40, 62, 0.82);
  }

  .sidebar-item.highlighted {
    box-shadow: inset 0 0 0 1px rgba(248, 161, 29, 0.26);
  }

  .sidebar-item-title {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    min-width: 0;
  }

  .sidebar-copy {
    min-width: 0;
    display: grid;
    gap: 2px;
    flex: 1;
  }

  .sidebar-glyph {
    width: 28px;
    height: 28px;
    border-radius: 8px;
    border: 1px solid rgba(106, 199, 255, 0.16);
    background: rgba(10, 25, 40, 0.88);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--db-cyan);
    flex: 0 0 auto;
  }

  .sidebar-label {
    font-size: 13px;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .sidebar-secondary {
    font-size: 12px;
    color: var(--db-text-soft);
    white-space: normal;
    overflow-wrap: anywhere;
    line-height: 1.25;
  }

  .storage-widget {
    margin-top: auto;
    padding: 14px 18px 18px;
    border-top: 1px solid var(--db-border);
    display: grid;
    gap: 12px;
  }

  .nvr-storage-head,
  .nvr-drive-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }

  .nvr-storage-icon {
    width: 32px;
    height: 32px;
  }

  .nvr-inventory-panel,
  .nvr-room-card {
    background: rgba(7, 18, 29, 0.7);
  }

  .nvr-room-grid,
  .nvr-recording-groups,
  .nvr-channel-list,
  .nvr-drive-grid,
  .vto-accessory-grid {
    display: grid;
    gap: 12px;
  }

  .compact-list {
    display: grid;
    gap: 10px;
  }

  .compact-card {
    border: 1px solid rgba(106, 199, 255, 0.12);
    border-radius: 8px;
    padding: 12px;
    background: rgba(8, 19, 30, 0.76);
    display: grid;
    gap: 10px;
    min-width: 0;
  }

  .compact-card-head {
    align-items: flex-start;
  }

  .nvr-summary-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .nvr-summary-chip-grid {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .nvr-summary-chip {
    width: 100%;
  }

  .nvr-summary-card {
    background: rgba(7, 18, 29, 0.7);
  }

  .nvr-summary-card .panel-title {
    align-items: flex-start;
  }

  .nvr-summary-card .badge {
    max-width: 100%;
    white-space: normal;
    overflow-wrap: anywhere;
  }

  .nvr-room-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .ptz-capability-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
  }

  .aux-capability-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
  }

  .stream-profile-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
  }

  .ptz-capability-card {
    background: rgba(7, 18, 29, 0.7);
  }

  .aux-capability-card {
    background: rgba(7, 18, 29, 0.7);
  }

  .stream-profile-card {
    background: rgba(7, 18, 29, 0.7);
  }

  .nvr-drive-card {
    background: rgba(7, 18, 29, 0.7);
  }

  .vto-accessory-card {
    background: rgba(7, 18, 29, 0.7);
  }

  .vto-lock-surface {
    gap: 10px;
  }

  .vto-lock-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 14px;
    align-items: center;
  }

  .inspector-storage-drives {
    gap: 12px;
  }

  .inspector-storage-drive {
    padding: 12px;
  }

  .detail-inline-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 10px 14px;
    align-items: center;
  }

  .archive-head {
    display: grid;
    gap: 10px;
    min-width: 0;
  }

  .archive-summary {
    justify-content: flex-start;
  }

  .archive-panel {
    flex: 1 1 auto;
    min-height: 220px;
  }

  .archive-panel .events-list {
    flex: 1 1 auto;
    max-height: none;
    min-height: 0;
  }

  .archive-panel .event-detail-list {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
    align-items: start;
  }

  .archive-panel .event-card {
    min-height: auto;
  }

  .nvr-channel-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid rgba(106, 199, 255, 0.12);
    background: rgba(8, 19, 30, 0.76);
    flex-wrap: wrap;
  }

  .nvr-channel-copy {
    display: grid;
    gap: 4px;
    min-width: 0;
  }

  .storage-drives {
    display: grid;
    gap: 10px;
  }

  .storage-drive {
    display: grid;
    gap: 8px;
    padding: 10px;
    border-radius: 8px;
    border: 1px solid rgba(106, 199, 255, 0.12);
    background: rgba(9, 22, 35, 0.74);
  }

  .storage-drive-head,
  .storage-drive-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
  }

  .progress {
    width: 100%;
    height: 10px;
    border-radius: 999px;
    overflow: hidden;
    background: rgba(255, 255, 255, 0.06);
  }

  .progress-bar {
    height: 100%;
    background: linear-gradient(90deg, var(--db-cyan), var(--db-blue));
    border-radius: inherit;
  }

  .header-logo,
  .header-chip-row,
  .header-actions {
    display: flex;
    gap: 12px;
    align-items: center;
    flex-wrap: wrap;
  }

  .header-logo {
    min-width: 0;
  }

  .header-logo-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    border: 0;
    background: transparent;
    cursor: pointer;
  }

  .header-logo-button:hover {
    opacity: 0.9;
  }

  .header-logo img {
    display: block;
    flex: 0 0 auto;
  }

  .logo-copy {
    display: none;
  }

  .header-chip-row {
    min-width: 0;
    flex: 1;
  }

  .header-chip-row,
  .header-actions {
    justify-content: flex-end;
  }

  .header-chip {
    min-width: 0;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--db-border);
    background: var(--db-panel-soft);
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .header-chip-button {
    cursor: pointer;
    text-align: left;
    transition: 140ms ease;
  }

  .header-chip-button:hover:not(:disabled) {
    border-color: var(--db-border-strong);
    background: rgba(17, 40, 62, 0.88);
  }

  .header-chip-button:disabled {
    cursor: default;
    opacity: 0.72;
  }

  .header-chip-icon {
    width: 40px;
    height: 40px;
    border-radius: 10px;
    border: 1px solid rgba(106, 199, 255, 0.18);
    background: rgba(9, 24, 38, 0.9);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--db-cyan);
    flex: 0 0 auto;
  }

  .header-chip-copy {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .header-chip-label {
    font-size: 11px;
    color: var(--db-text-soft);
    line-height: 1.1;
  }

  .header-chip-value {
    font-size: 15px;
    font-weight: 600;
    line-height: 1.15;
    white-space: nowrap;
  }

  .tone-success {
    color: #9ff0c7;
  }

  .tone-warning {
    color: #ffd494;
  }

  .tone-info {
    color: #bfd0ff;
  }

  .tone-critical {
    color: #ffc7d1;
  }

  .header-actions {
    display: flex;
    gap: 8px;
  }
`;

export const surveillancePanelOverviewStyles = css`
  .overview-shell {
    display: grid;
    grid-template-rows: minmax(0, 1fr);
    height: 100%;
    min-height: 0;
  }

  .overview-grid {
    flex: 1;
    height: max-content;
    min-height: 0;
    overflow: auto;
    padding: 16px;
    display: grid;
    grid-template-columns: repeat(var(--overview-columns, 3), minmax(0, 1fr));
    grid-template-rows: repeat(var(--overview-rows, 3), minmax(0, 1fr));
    gap: 14px;
    align-content: stretch;
  }

  .overview-grid.layout-1x1 {
    --overview-columns: 1;
    --overview-rows: 1;
  }

  .overview-grid.layout-2x1 {
    --overview-columns: 2;
    --overview-rows: 1;
  }

  .overview-grid.layout-2x2 {
    --overview-columns: 2;
    --overview-rows: 2;
  }

  .overview-grid.layout-3x2 {
    --overview-columns: 3;
    --overview-rows: 2;
  }

  .overview-grid.layout-3x3 {
    --overview-columns: 3;
    --overview-rows: 3;
  }

  .overview-grid.layout-4x3 {
    --overview-columns: 4;
    --overview-rows: 3;
  }

  .overview-grid.layout-4x4 {
    --overview-columns: 4;
    --overview-rows: 4;
  }

  .overview-grid[class*="layout-"] .camera-tile {
    min-height: 0;
  }

  .camera-tile {
    position: relative;
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    min-height: 0;
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid var(--db-border);
    background: rgba(5, 13, 21, 0.92);
    cursor: pointer;
  }

  .camera-tile.selected {
    border-color: rgba(52, 216, 255, 0.48);
    box-shadow: inset 0 0 0 1px rgba(52, 216, 255, 0.18);
  }

  .tile-media,
  .viewport {
    position: relative;
    min-height: 0;
    background: linear-gradient(180deg, rgba(7, 19, 30, 0.25), rgba(7, 19, 30, 0.82));
  }

  .tile-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 12px;
    border-bottom: 1px solid rgba(106, 199, 255, 0.12);
    background: rgba(8, 20, 33, 0.96);
  }

  .tile-status {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }

  .tile-image,
  .tile-media img#remote-stream,
  .tile-media img.remote-stream,
  .tile-media video#remote-stream,
  .tile-media video.remote-stream,
  .viewport img,
  .viewport img#remote-stream,
  .viewport img.remote-stream,
  .viewport video#remote-stream,
  .viewport video.remote-stream,
  ha-camera-stream {
    width: 100%;
    height: 100%;
    display: block;
    object-fit: fill;
    background: rgba(4, 10, 16, 0.94);
  }

  .viewport video#remote-stream,
  .viewport video.remote-stream,
  .viewport img#remote-stream,
  .viewport img.remote-stream {
    aspect-ratio: 16 / 9;
  }

  .tile-media {
    aspect-ratio: 16 / 9;
    overflow: hidden;
    border-bottom: 1px solid rgba(106, 199, 255, 0.12);
  }

  .media-overlay {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    padding: 12px;
    background: linear-gradient(0deg, rgba(0, 0, 0, 0.62), transparent 28%);
    pointer-events: none;
  }

  .media-top,
  .media-bottom {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    pointer-events: none;
  }

  .tile-actions {
    display: flex;
    gap: 8px;
    pointer-events: auto;
  }

  .tile-controls .icon-button {
    width: 32px;
    height: 32px;
    border-radius: 999px;
    border-color: rgba(255, 255, 255, 0.14);
    background: rgba(255, 255, 255, 0.06);
  }

  .tile-title-text {
    min-width: 0;
    display: grid;
    gap: 3px;
  }

  .tile-name {
    font-size: 14px;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tile-subtitle {
    font-size: 12px;
    color: var(--db-text-soft);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tile-badges {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .tile-controls {
    position: absolute;
    right: 12px;
    bottom: 12px;
    display: flex;
    align-items: center;
    gap: 8px;
    margin-left: auto;
    padding: 6px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.18);
    background: rgba(7, 18, 29, 0.48);
    backdrop-filter: blur(14px);
    -webkit-backdrop-filter: blur(14px);
    z-index: 1;
    pointer-events: auto;
  }

  .tile-overlay-badges {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding-right: 224px;
  }
`;

export const surveillancePanelDetailStyles = css`
  .detail-shell {
    display: grid;
    grid-template-rows: auto auto auto;
    height: auto;
    min-height: 0;
  }

  .detail-header {
    padding: 15px 15px 0;
    display: grid;
    gap: 8px;
  }

  .detail-title {
    font-size: 20px;
    font-weight: 600;
    line-height: 1.2;
    overflow-wrap: anywhere;
  }

  .detail-tabs {
    display: flex;
    gap: 8px;
    padding: 0 2px 10px;
    flex-wrap: wrap;
  }

  .detail-main {
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 0;
    overflow: visible;
  }

  .video-panel {
    padding: 16px;
    border-bottom: 1px solid var(--db-border);
    display: grid;
    gap: 14px;
  }

  .detail-media-toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
  }

  .detail-media-group {
    align-items: center;
  }

  .detail-media-separator {
    width: 1px;
    align-self: stretch;
    min-height: 28px;
    background: rgba(106, 199, 255, 0.18);
  }

  .viewport {
    position: relative;
    border-radius: 8px;
    overflow: hidden;
    aspect-ratio: 16 / 9;
    min-height: min(54vh, 560px);
    border: 1px solid var(--db-border);
  }

  .viewport-controls {
    position: absolute;
    left: 14px;
    right: 14px;
    bottom: 14px;
    z-index: 3;
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    align-items: flex-end;
    gap: 10px;
    padding-left: min(320px, 42%);
    pointer-events: none;
  }

  .viewport-controls .control-button {
    pointer-events: auto;
  }

  .viewport.empty {
    display: grid;
    place-items: center;
    color: var(--db-text-soft);
  }

  .slider-wrap {
    display: grid;
    gap: 8px;
  }

  input[type="range"] {
    width: 100%;
    accent-color: var(--db-cyan);
    margin: 0;
  }

  .ptz-overlay {
    position: absolute;
    inset: 14px;
    pointer-events: none;
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 14px;
    align-items: end;
    z-index: 2;
  }

  .ptz-card {
    pointer-events: auto;
    width: min(290px, 100%);
    border-radius: 8px;
    border: 1px solid rgba(52, 216, 255, 0.3);
    background: rgba(5, 16, 27, 0.88);
    padding: 14px;
    display: grid;
    gap: 12px;
    box-shadow: var(--db-shadow);
  }

  .ptz-grid {
    display: grid;
    grid-template-columns: repeat(3, 48px);
    gap: 8px;
    justify-content: center;
  }

  .ptz-grid .icon-button {
    width: 48px;
    height: 48px;
    border-radius: 50%;
  }

  .events {
    padding: 14px;
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    gap: 12px;
    min-height: 0;
    border: 1px solid rgba(106, 199, 255, 0.14);
    border-radius: 8px;
    background: rgba(7, 18, 29, 0.64);
    overflow: hidden;
  }

  .events-toolbar,
  .event-toolbar-row,
  .event-range-meta,
  .event-detail-list,
  .event-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: center;
  }

  .events-toolbar,
  .event-toolbar-row {
    justify-content: space-between;
  }

  .event-filter-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .event-filter {
    display: grid;
    gap: 6px;
    min-width: 0;
  }

  .event-filter-label {
    font-size: 11px;
    text-transform: uppercase;
    color: var(--db-text-soft);
    letter-spacing: 0.08em;
  }

  .event-filter-select {
    width: 100%;
    min-width: 0;
    height: 38px;
    border-radius: 8px;
    border: 1px solid var(--db-border);
    background: rgba(7, 18, 29, 0.9);
    color: var(--db-text);
    padding: 0 12px;
    outline: none;
  }

  .events-list {
    min-height: 0;
    overflow: auto;
    display: grid;
    gap: 10px;
    padding-right: 2px;
  }

  .history-range {
    width: min(280px, 100%);
    display: grid;
    gap: 6px;
  }

  .history-range input[type="range"] {
    width: 100%;
  }

  .timeline-scroll {
    display: grid;
    gap: 10px;
    min-height: 0;
  }

  .event-card {
    border: 1px solid var(--db-border);
    border-radius: 8px;
    padding: 12px;
    background: rgba(6, 16, 26, 0.72);
    display: grid;
    gap: 8px;
  }

  .event-card-body {
    display: grid;
    gap: 8px;
  }

  .event-card.warning {
    border-color: rgba(248, 161, 29, 0.32);
  }

  .event-card.info {
    border-color: rgba(75, 129, 255, 0.32);
  }

  .event-card.success {
    border-color: rgba(65, 217, 140, 0.32);
  }

  .event-card.critical {
    border-color: rgba(255, 95, 121, 0.32);
  }

  .event-card .split-row .badge {
    padding: 2px 10px;
    border-radius: 8px;
  }
`;

export const surveillancePanelResponsiveStyles = css`
  @media (max-width: 1360px) {
    .shell {
      grid-template-columns: minmax(240px, 280px) minmax(0, 1fr);
      grid-template-areas:
        "header header"
        "sidebar main"
        "inspector inspector"
        "events events";
    }
  }

  @media (max-width: 980px) {
    ha-card {
      aspect-ratio: auto;
      height: auto;
      min-height: auto;
    }

    .shell {
      grid-template-columns: minmax(0, 1fr);
      grid-template-areas:
        "header"
        "sidebar"
        "main"
        "inspector";
    }

    .header {
      grid-template-columns: minmax(0, 1fr);
    }

    .header-chip-row {
      flex: initial;
    }

    .sidebar.mobile-hidden,
    .inspector.mobile-hidden {
      display: none;
    }

    .viewport {
      min-height: 300px;
    }

    .viewport-controls {
      left: 10px;
      right: 10px;
      bottom: 10px;
      justify-content: stretch;
      padding-left: 0;
    }

    .viewport-controls .control-button[data-compact="true"] {
      flex: 1 1 128px;
      justify-content: center;
    }
  }

  @media (max-width: 720px) {
    .shell {
      padding: 12px;
      gap: 12px;
    }

    .key-value-grid {
      grid-template-columns: 1fr;
    }

    .header-chip-row {
      width: 100%;
    }

    .header-chip {
      width: 100%;
    }

    .overview-grid {
      grid-template-columns: 1fr;
    }

    .ptz-capability-grid {
      grid-template-columns: 1fr;
    }

    .aux-capability-grid {
      grid-template-columns: 1fr;
    }

    .stream-profile-grid {
      grid-template-columns: 1fr;
    }

    .nvr-summary-grid {
      grid-template-columns: 1fr;
    }

    .nvr-summary-chip-grid {
      grid-template-columns: 1fr;
    }

    .event-filter-grid {
      grid-template-columns: 1fr;
    }

    .nvr-channel-row {
      align-items: flex-start;
    }
  }
`;

export const surveillancePanelStyles = [
  surveillancePanelBaseStyles,
  surveillancePanelOverviewStyles,
  surveillancePanelDetailStyles,
  surveillancePanelResponsiveStyles,
];
