package httpserver

import (
	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/streams"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type adminEndpoint struct {
	Method      string
	Path        string
	Description string
	Linkable    bool
}

type testBridgeChannel struct {
	ID                 string                         `json:"id"`
	DeviceID           string                         `json:"device_id"`
	Name               string                         `json:"name"`
	Channel            int                            `json:"channel"`
	SnapshotURL        string                         `json:"snapshot_url"`
	RecommendedProfile string                         `json:"recommended_profile"`
	MainVideo          string                         `json:"main_video,omitempty"`
	SubVideo           string                         `json:"sub_video,omitempty"`
	AudioCodec         string                         `json:"audio_codec,omitempty"`
	Profiles           []testBridgeProfile            `json:"profiles"`
	Controls           *streams.ChannelControlSummary `json:"controls,omitempty"`
	Features           []streams.FeatureSummary       `json:"features,omitempty"`
}

type testBridgeProfile struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	SnapshotURL string `json:"snapshot_url"`
	MJPEGURL    string `json:"mjpeg_url"`
	HLSURL      string `json:"hls_url"`
	PreviewURL  string `json:"preview_url"`
	WebRTCURL   string `json:"webrtc_url"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

func renderAdminPage(
	status httpStatus,
	probeResults []*dahua.ProbeResult,
	streamEntries []streams.Entry,
	settings map[string]any,
	workerStatuses []mediaapi.WorkerStatus,
	actionsAvailable bool,
	mediaEnabled bool,
	healthPath string,
	metricsPath string,
) string {
	endpointSections := buildAdminEndpointSections(healthPath, metricsPath)
	controlStats := summarizeAdminControlStats(streamEntries)
	deviceCards := buildAdminDeviceCards(probeResults, streamEntries)
	streamCards := buildAdminStreamCards(streamEntries)
	settingsJSON := htmlEscape(marshalIndentedJSON(settings))
	workerJSON := htmlEscape(marshalIndentedJSON(workerStatuses))
	deviceCount := len(probeResults)
	streamCount := len(streamEntries)
	workerCount := len(workerStatuses)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>DahuaBridge Admin</title>
  <link rel="stylesheet" href="/admin/assets/bootstrap.min.css">
  <style>
    :root {
      color-scheme: dark;
      --bg: #0c1114;
      --bg-soft: #151e21;
      --panel: #111a1d;
      --line: rgba(153, 174, 166, 0.2);
      --text: #f1f5f2;
      --muted: #a8b8b2;
      --accent: #5ed0ac;
      --accent-soft: rgba(94, 208, 172, 0.14);
      --warm: #d9a441;
      --danger: #ef747b;
      --shadow: 0 12px 32px rgba(0, 0, 0, 0.24);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Segoe UI", Tahoma, Geneva, Verdana, sans-serif;
      color: var(--text);
      background: linear-gradient(180deg, #151e21 0%%, #0c1114 72%%);
    }
    main {
      max-width: 1320px;
      margin: 0 auto;
      padding: 28px;
      display: grid;
      gap: 18px;
    }
    .hero {
      display: grid;
      gap: 18px;
      grid-template-columns: auto minmax(0, 1fr);
      align-items: center;
      padding: 26px 28px;
      border-radius: 8px;
      background: #121d20;
      border: 1px solid var(--line);
      box-shadow: var(--shadow);
    }
    .hero-mark {
      width: 104px;
      height: 104px;
      padding: 12px;
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      background: rgba(255,255,255,0.06);
      border: 1px solid rgba(255,255,255,0.08);
      box-shadow: inset 0 1px 0 rgba(255,255,255,0.08);
    }
    .hero-mark img {
      width: 100%%;
      height: 100%%;
      object-fit: contain;
      filter: drop-shadow(0 14px 18px rgba(0,0,0,0.30));
    }
    .hero-copy {
      display: grid;
      gap: 10px;
    }
    .eyebrow {
      display: inline-flex;
      width: fit-content;
      padding: 6px 12px;
      border-radius: 8px;
      background: var(--accent-soft);
      color: var(--accent);
      letter-spacing: 0;
      text-transform: uppercase;
      font-size: 12px;
    }
    h1 {
      margin: 0;
      font-size: clamp(34px, 5vw, 60px);
      line-height: 0.94;
      font-weight: 700;
      letter-spacing: 0;
    }
    .lead {
      margin: 0;
      max-width: 76ch;
      color: var(--muted);
      font-size: 17px;
      line-height: 1.45;
    }
    .grid {
      display: grid;
      gap: 18px;
    }
    .summary-grid {
      display: grid;
      gap: 14px;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    }
    .summary-card, .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
    }
    .summary-card {
      padding: 18px;
      display: grid;
      gap: 8px;
    }
    .summary-label {
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: 0;
      font-size: 12px;
    }
    .summary-value {
      font-size: 30px;
      line-height: 1;
      font-weight: 600;
    }
    .summary-subtle {
      color: var(--muted);
      font-size: 14px;
    }
    .panel {
      padding: 20px;
      display: grid;
      gap: 16px;
    }
    .panel h2 {
      margin: 0;
      font-size: 22px;
      font-weight: 600;
    }
    .panel p {
      margin: 0;
      color: var(--muted);
      line-height: 1.4;
    }
    .layout {
      display: grid;
      grid-template-columns: minmax(0, 1.2fr) minmax(0, 0.8fr);
      gap: 18px;
    }
    .stack {
      display: grid;
      gap: 18px;
    }
    .action-row {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    .action-row .btn {
      appearance: none;
      border-radius: 8px;
      padding: 12px 16px;
      font-weight: 600;
      box-shadow: 0 12px 28px rgba(0, 0, 0, 0.18);
    }
    .action-row .btn-success {
      color: #08100d;
      background: var(--accent);
      border-color: rgba(150, 240, 203, 0.22);
    }
    .action-row .btn-outline-light {
      color: var(--text);
      border-color: rgba(255,255,255,0.22);
      background: rgba(255,255,255,0.02);
    }
    .action-row .btn-warning {
      color: #241600;
      background: var(--warm);
      border-color: rgba(255, 210, 120, 0.24);
    }
    .action-row .btn-outline-danger {
      border-color: rgba(255, 140, 148, 0.38);
    }
    .action-row .btn:disabled {
      cursor: not-allowed;
      opacity: 0.55;
    }
    .result-box, pre {
      margin: 0;
      padding: 14px 16px;
      border-radius: 8px;
      background: rgba(0,0,0,0.22);
      border: 1px solid rgba(255,255,255,0.06);
      color: var(--text);
      overflow: auto;
      font-family: Consolas, "Courier New", monospace;
      font-size: 13px;
      line-height: 1.5;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .endpoint-group, .card-grid {
      display: grid;
      gap: 12px;
    }
    .endpoint-list {
      display: grid;
      gap: 8px;
    }
    .endpoint-row, .device-card, .stream-card {
      display: grid;
      gap: 8px;
      padding: 14px 16px;
      border-radius: 8px;
      background: rgba(255,255,255,0.02);
      border: 1px solid rgba(255,255,255,0.06);
    }
    .endpoint-row {
      grid-template-columns: auto minmax(0, 1fr);
      align-items: center;
      column-gap: 12px;
    }
    .method {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      min-width: 64px;
      padding: 6px 8px;
      border-radius: 8px;
      background: rgba(255,255,255,0.07);
      color: var(--accent);
      font-size: 12px;
      letter-spacing: 0;
      text-transform: uppercase;
    }
    .endpoint-main {
      display: grid;
      gap: 4px;
    }
    .endpoint-main a, .chip a {
      color: var(--text);
      text-decoration: none;
    }
    .endpoint-main code {
      color: var(--text);
      font-size: 13px;
    }
    .endpoint-desc {
      color: var(--muted);
      font-size: 14px;
    }
    .card-grid {
      grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    }
    .card-title {
      margin: 0;
      font-size: 19px;
      font-weight: 600;
    }
    .card-meta {
      color: var(--muted);
      font-size: 14px;
    }
    .chip-row {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }
    .chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 8px 10px;
      border-radius: 8px;
      background: rgba(150, 240, 203, 0.08);
      border: 1px solid rgba(150, 240, 203, 0.12);
      font-size: 13px;
    }
    .chip code {
      color: var(--text);
    }
    .chip.subtle {
      background: rgba(255,255,255,0.04);
      border-color: rgba(255,255,255,0.08);
    }
    .muted-note {
      color: var(--muted);
      font-size: 14px;
    }
    @media (max-width: 1120px) {
      .layout, .summary-grid {
        grid-template-columns: 1fr 1fr;
      }
      .hero {
        grid-template-columns: 1fr;
      }
    }
    @media (max-width: 760px) {
      main {
        padding: 18px;
      }
      .layout, .summary-grid {
        grid-template-columns: 1fr;
      }
      .action-row {
        flex-direction: column;
      }
      .action-row .btn {
        width: 100%%;
      }
      .hero-mark {
        width: 88px;
        height: 88px;
      }
    }
  </style>
</head>
<body data-bs-theme="dark">
  <main class="container-xxl py-4">
    <section class="hero">
      <div class="hero-mark">
        <img src="/admin/assets/logo.png" alt="DahuaBridge logo">
      </div>
      <div class="hero-copy">
        <div class="eyebrow">DahuaBridge Admin</div>
        <h1>Operator Surface</h1>
        <p class="lead">This page exposes the real bridge control surface in one place: health and runtime summaries, concrete endpoint links, redacted config, stream and device shortcuts, and the highest-value mutating actions without needing to remember URLs.</p>
      </div>
    </section>

    <section class="summary-grid">
      <article class="summary-card">
        <div class="summary-label">Readiness</div>
        <div class="summary-value">%s</div>
        <div class="summary-subtle">%s</div>
      </article>
      <article class="summary-card">
        <div class="summary-label">Devices</div>
        <div class="summary-value">%d</div>
        <div class="summary-subtle">Last update: %s</div>
      </article>
      <article class="summary-card">
        <div class="summary-label">Streams</div>
        <div class="summary-value">%d</div>
        <div class="summary-subtle">Concrete preview/intercom links listed below.</div>
      </article>
      <article class="summary-card">
        <div class="summary-label">Media Workers</div>
        <div class="summary-value">%d</div>
        <div class="summary-subtle">Media enabled: %t</div>
      </article>
      <article class="summary-card">
        <div class="summary-label">Control Surface</div>
        <div class="summary-value">%d</div>
        <div class="summary-subtle">%s</div>
      </article>
    </section>

    <section class="layout">
      <div class="stack">
        <section class="panel">
          <h2>Admin Actions</h2>
          <p>These buttons call the same authenticated bridge endpoints the rest of the system uses. Responses are shown inline exactly as the API returns them.</p>
            <div class="action-row">
              <a class="btn btn-outline-light" href="/admin/test-bridge">Open Bridge Test Page</a>
              <button type="button" class="btn btn-success" data-method="POST" data-url="/api/v1/devices/probe-all" data-success="Probe-all requested." %s>Probe All Devices</button>
            </div>
          <pre id="admin-action-result" class="result-box">No action has been run yet.</pre>
        </section>

        <section class="panel">
          <h2>Endpoint Inventory</h2>
          <p>Generic bridge routes are grouped here. Use the device and stream sections below for concrete per-device links.</p>
          %s
        </section>

        <section class="panel">
          <h2>Discovered Devices</h2>
          <p>Root-device links are built from the current probe state and stream inventory.</p>
          %s
        </section>

        <section class="panel">
          <h2>Streams</h2>
          <p>These are concrete preview, WebRTC, HLS, snapshot, and intercom shortcuts from the current stream catalog.</p>
          %s
        </section>
      </div>

      <div class="stack">
        <section class="panel">
          <h2>Redacted Settings</h2>
          <p>Passwords, access tokens, ICE credentials, and ONVIF passwords are redacted before reaching this page.</p>
          <pre>%s</pre>
        </section>

        <section class="panel">
          <h2>Media Worker Status</h2>
          <pre>%s</pre>
        </section>
      </div>
    </section>
  </main>
  <script>
    bridgePreparePage();
    const resultBox = document.getElementById('admin-action-result');
    const actionButtons = Array.from(document.querySelectorAll('[data-method][data-url]'));

    function setResult(title, payload, isError) {
      resultBox.textContent = title + "\n\n" + payload;
      resultBox.style.borderColor = isError ? 'rgba(255, 140, 148, 0.35)' : 'rgba(150, 240, 203, 0.22)';
    }

    async function runAdminAction(button) {
      const method = button.getAttribute('data-method') || 'GET';
      const url = button.getAttribute('data-url');
      const body = button.getAttribute('data-body');
      const success = button.getAttribute('data-success') || 'Action finished.';
      const previous = button.textContent;
      button.disabled = true;
      setResult('Running ' + method + ' ' + url, 'Please wait...', false);
      try {
        const response = await bridgeFetch(url, {
          method,
          headers: body ? { 'Content-Type': 'application/json' } : undefined,
          body: body || undefined,
        });
        const text = await response.text();
        if (!response.ok) {
          throw new Error(text || response.statusText || 'request failed');
        }
        setResult(success, text || '(empty response)', false);
      } catch (error) {
        setResult('Action failed', error && error.message ? error.message : String(error), true);
      } finally {
        button.disabled = false;
        button.textContent = previous;
      }
    }

    for (const button of actionButtons) {
      button.addEventListener('click', () => {
        runAdminAction(button);
      });
    }

    for (const select of document.querySelectorAll('[data-live-source-url]')) {
      select.addEventListener('change', async () => {
        const previous = select.dataset.currentSource;
        const source = select.value;
        select.disabled = true;
        try {
          const target = new URL(bridgeURL(select.dataset.liveSourceUrl), window.location.href);
          if (target.origin !== window.location.origin) throw new Error('Live source settings must use this bridge.');
          const headers = { 'Content-Type': 'application/json' };
          const params = new URL(window.location.href).searchParams;
          const token = params.get('auth_token') || params.get('token');
          if (token) headers.Authorization = 'Bearer ' + token;
          const response = await bridgeFetch(target, {
            method: 'PUT',
            headers,
            body: JSON.stringify({ source }),
          });
          if (!response.ok) throw new Error(await response.text() || response.statusText);
          select.dataset.currentSource = source;
          setResult('Live source saved', 'Live viewers will reconnect. Recorded playback stays on the NVR.', false);
        } catch (error) {
          select.value = previous;
          setResult('Could not change live source', error && error.message ? error.message : String(error), true);
        } finally {
          select.disabled = false;
        }
      });
    }
  </script>
</body>
</html>`,
		boolText(status.Ready, "Ready", "Not Ready"),
		htmlEscape(firstNonEmpty(status.LastUpdatedAt, "No successful probe yet.")),
		deviceCount,
		htmlEscape(firstNonEmpty(status.LastUpdatedAt, "unknown")),
		streamCount,
		workerCount,
		mediaEnabled,
		controlStats.ActionableEntries,
		htmlEscape(controlStats.Summary()),
		boolHTMLAttr(actionsAvailable),
		endpointSections,
		deviceCards,
		streamCards,
		settingsJSON,
		workerJSON,
	)
}

func renderAdminTestBridgePage(streamEntries []streams.Entry, actionsAvailable bool, mediaEnabled bool) string {
	channels := buildTestBridgeChannels(streamEntries)
	channelsJSON := marshalJSONForScript(channels)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>DahuaBridge Bridge Test</title>
  <link rel="stylesheet" href="/admin/assets/bootstrap.min.css">
  <style>
    :root {
      color-scheme: dark;
      --bg: #101316;
      --panel: #171d20;
      --panel-soft: #20272b;
      --line: rgba(220, 230, 224, 0.16);
      --text: #f4f7f5;
      --muted: #aeb8b3;
      --accent: #62d0aa;
      --accent-2: #d7a64a;
      --danger: #eb747a;
      --ok: rgba(98, 208, 170, 0.16);
      --warn: rgba(215, 166, 74, 0.16);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Segoe UI", Tahoma, Geneva, Verdana, sans-serif;
      background: #101316;
      color: var(--text);
    }
    main {
      max-width: 1480px;
      margin: 0 auto;
      padding: 24px;
      display: grid;
      gap: 16px;
    }
    .topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      flex-wrap: wrap;
    }
    .topbar a {
      color: var(--text);
      text-decoration: none;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 8px 12px;
      background: rgba(255,255,255,0.03);
    }
    .hero {
      display: grid;
      gap: 10px;
      padding: 22px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
    }
    .eyebrow {
      color: var(--accent);
      text-transform: uppercase;
      letter-spacing: 0;
      font-size: 12px;
    }
    h1, h2, h3, p {
      margin: 0;
    }
    h1 {
      font-size: clamp(30px, 4vw, 52px);
      line-height: 1;
      letter-spacing: 0;
      font-weight: 700;
    }
    h2 {
      font-size: 19px;
      line-height: 1.2;
      font-weight: 650;
    }
    h3 {
      font-size: 15px;
      line-height: 1.2;
      font-weight: 650;
      color: var(--accent);
    }
    .muted {
      color: var(--muted);
      line-height: 1.45;
    }
    .status-strip {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
    }
    .pill {
      display: inline-flex;
      align-items: center;
      min-height: 32px;
      padding: 7px 10px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: rgba(255,255,255,0.035);
      color: var(--muted);
      font-size: 13px;
    }
    .pill.good {
      color: var(--accent);
      background: var(--ok);
    }
    .pill.warn {
      color: var(--accent-2);
      background: var(--warn);
    }
    .workspace {
      display: grid;
      grid-template-columns: minmax(280px, 360px) minmax(0, 1fr);
      gap: 16px;
      align-items: start;
    }
    .panel {
      display: grid;
      gap: 14px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
      padding: 16px;
      min-width: 0;
    }
    .sidebar {
      position: sticky;
      top: 16px;
    }
    label {
      display: grid;
      gap: 6px;
      color: var(--muted);
      font-size: 13px;
    }
    select, input {
      width: 100%%;
      min-height: 40px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: #0f1416;
      color: var(--text);
      padding: 8px 10px;
      font: inherit;
    }
    .viewer {
      min-height: 360px;
      aspect-ratio: 16 / 9;
      display: flex;
      align-items: center;
      justify-content: center;
      overflow: hidden;
      border-radius: 8px;
      border: 1px solid rgba(255,255,255,0.09);
      background: #050607;
    }
    .viewer img, .viewer video, .viewer iframe {
      width: 100%%;
      height: 100%%;
      border: 0;
      object-fit: contain;
      background: #050607;
    }
    .viewer iframe {
      object-fit: unset;
    }
    .viewer-empty {
      color: var(--muted);
      padding: 20px;
      text-align: center;
    }
    .button-row {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }
    button {
      min-height: 38px;
      border: 1px solid rgba(255,255,255,0.16);
      border-radius: 8px;
      background: var(--panel-soft);
      color: var(--text);
      padding: 8px 11px;
      font: inherit;
      font-weight: 600;
      cursor: pointer;
    }
    button.primary {
      background: var(--accent);
      color: #07110d;
      border-color: rgba(160, 240, 205, 0.3);
    }
    button.warn {
      background: var(--accent-2);
      color: #1d1300;
      border-color: rgba(255, 220, 150, 0.3);
    }
    button.danger {
      border-color: rgba(235, 116, 122, 0.45);
      color: #ffd7da;
    }
    button:disabled {
      cursor: not-allowed;
      opacity: 0.5;
    }
    .control-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
      gap: 16px;
    }
    pre {
      margin: 0;
      min-height: 120px;
      max-height: 420px;
      overflow: auto;
      padding: 12px;
      border-radius: 8px;
      border: 1px solid rgba(255,255,255,0.08);
      background: #0a0d0e;
      color: var(--text);
      font: 13px/1.45 Consolas, "Courier New", monospace;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .log {
      min-height: 260px;
    }
    @media (max-width: 980px) {
      main {
        padding: 16px;
      }
      .workspace {
        grid-template-columns: 1fr;
      }
      .sidebar {
        position: static;
      }
      .viewer {
        min-height: 240px;
      }
    }
  </style>
</head>
<body data-bs-theme="dark">
  <main>
    <nav class="topbar">
      <a href="/admin">Admin</a>
      <a href="/api/v1/streams">Streams JSON</a>
    </nav>

    <section class="hero">
      <div class="eyebrow">DahuaBridge Admin</div>
      <h1>Bridge Test Bench</h1>
      <p class="muted">Select one NVR channel, switch stream transports, then run bridge, NVR CGI, NVR RPC, and direct IPC control attempts from the same page.</p>
      <div class="status-strip">
        <span class="pill good">%d NVR channels</span>
        <span class="pill %s">media %s</span>
        <span class="pill %s">actions %s</span>
      </div>
    </section>

    <section class="workspace">
      <aside class="panel sidebar">
        <h2>Channel</h2>
        <label>Camera channel
          <select id="channel-select"></select>
        </label>
        <label>Stream profile
          <select id="profile-select"></select>
        </label>
        <pre id="channel-meta">No channel selected.</pre>
      </aside>

      <section class="panel">
        <h2>Video</h2>
        <div class="button-row">
          <button type="button" data-stream-mode="snapshot" class="primary">Snapshot</button>
          <button type="button" data-stream-mode="mjpeg">MJPEG</button>
          <button type="button" data-stream-mode="hls">HLS</button>
          <button type="button" data-stream-mode="preview">Preview Page</button>
          <button type="button" data-stream-mode="webrtc">WebRTC Page</button>
        </div>
        <div id="viewer" class="viewer"><div class="viewer-empty">Select a channel to load video.</div></div>
      </section>
    </section>

    <section class="control-grid">
      <section class="panel">
        <h2>Bridge Controls</h2>
        <div class="button-row">
          <button type="button" data-call="controls">Read Capabilities</button>
          <button type="button" data-call="probe">Probe Device</button>
          <button type="button" data-call="refresh">Refresh NVR Inventory</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="ptz" data-command="left">PTZ Left Pulse</button>
          <button type="button" data-call="ptz" data-command="right">PTZ Right Pulse</button>
          <button type="button" data-call="ptz" data-command="up">PTZ Up Pulse</button>
          <button type="button" data-call="ptz" data-command="down">PTZ Down Pulse</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="aux" data-output="light" data-action="start" class="warn">White Light On</button>
          <button type="button" data-call="aux" data-output="light" data-action="stop">White Light Off</button>
          <button type="button" data-call="aux" data-output="warning_light" data-action="start" class="warn">Warning Light On</button>
          <button type="button" data-call="aux" data-output="warning_light" data-action="stop">Warning Light Off</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="aux" data-output="aux" data-action="pulse" data-duration="800" class="danger">Siren Pulse</button>
          <button type="button" data-call="aux" data-output="aux" data-action="stop">Siren Stop</button>
          <button type="button" data-call="aux" data-output="wiper" data-action="pulse">Wiper Pulse</button>
          <button type="button" data-call="aux" data-output="wiper" data-action="stop">Wiper Stop</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="recording" data-action="start">Recording Start</button>
          <button type="button" data-call="recording" data-action="stop">Recording Stop</button>
          <button type="button" data-call="recording" data-action="auto">Recording Auto</button>
        </div>
      </section>

      <section class="panel">
        <h2>Raw NVR PTZ CGI</h2>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_aux" data-action="start" class="danger">Aux Start</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_aux" data-action="stop">Aux Stop</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_aux" data-action="pulse" data-duration="800" class="danger">Aux Pulse</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_light" data-action="start" class="warn">Light Start</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_light" data-action="stop">Light Stop</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_light" data-action="pulse" data-duration="800">Light Pulse</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_wiper" data-action="start">Wiper Start</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_wiper" data-action="stop">Wiper Stop</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_wiper" data-action="pulse">Wiper Pulse</button>
        </div>
      </section>

      <section class="panel">
        <h2>Lighting APIs</h2>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="bridge_light" data-action="start" class="warn">Bridge Light On</button>
          <button type="button" data-call="diagnostic" data-method="bridge_light" data-action="stop">Bridge Light Off</button>
          <button type="button" data-call="diagnostic" data-method="bridge_warning_light" data-action="start" class="warn">Bridge Warning On</button>
          <button type="button" data-call="diagnostic" data-method="bridge_warning_light" data-action="stop">Bridge Warning Off</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="nvr_lighting_config" data-action="start" class="warn">NVR Lighting_V2 On</button>
          <button type="button" data-call="diagnostic" data-method="nvr_lighting_config" data-action="stop">NVR Lighting_V2 Off</button>
          <button type="button" data-call="diagnostic" data-method="nvr_video_input_light_param" data-action="start" class="warn">VideoIn Light On</button>
          <button type="button" data-call="diagnostic" data-method="nvr_video_input_light_param" data-action="stop">VideoIn Light Off</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="direct_ipc_lighting" data-action="start" class="warn">Direct IPC Lighting On</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_lighting" data-action="stop">Direct IPC Lighting Off</button>
        </div>
      </section>

      <section class="panel">
        <h2>Direct IPC PTZ CGI</h2>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_light" data-action="start" class="warn">Light ch1 Start</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_light" data-action="stop">Light ch1 Stop</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_light_ch0" data-action="start" class="warn">Light ch0 Start</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_light_ch0" data-action="stop">Light ch0 Stop</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_aux" data-action="pulse" data-duration="800" class="danger">Aux ch1 Pulse</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_aux_ch0" data-action="pulse" data-duration="800" class="danger">Aux ch0 Pulse</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_wiper" data-action="pulse">Wiper ch1 Pulse</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_wiper_ch0" data-action="pulse">Wiper ch0 Pulse</button>
        </div>
      </section>

      <section class="panel">
        <h2>Recording APIs</h2>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="record_mode" data-action="start">RecordMode Manual</button>
          <button type="button" data-call="diagnostic" data-method="record_mode" data-action="stop">RecordMode Stop</button>
          <button type="button" data-call="diagnostic" data-method="record_mode" data-action="auto">RecordMode Auto</button>
        </div>
      </section>

      <section class="panel">
        <h2>Result Log</h2>
        <pre id="result-log" class="log">No test action has run yet.</pre>
      </section>
    </section>
  </main>

  <script>
    bridgePreparePage();
    const CHANNELS = %s;
    const ACTIONS_AVAILABLE = %t;
    const MEDIA_ENABLED = %t;
    const channelSelect = document.getElementById('channel-select');
    const profileSelect = document.getElementById('profile-select');
    const channelMeta = document.getElementById('channel-meta');
    const viewer = document.getElementById('viewer');
    const resultLog = document.getElementById('result-log');
    const streamButtons = Array.from(document.querySelectorAll('[data-stream-mode]'));
    const actionButtons = Array.from(document.querySelectorAll('[data-call]'));
    let lastStreamMode = 'snapshot';

    function selectedChannel() {
      const index = Number(channelSelect.value);
      if (!Number.isFinite(index) || index < 0 || index >= CHANNELS.length) {
        return null;
      }
      return CHANNELS[index];
    }

    function selectedProfile() {
      const channel = selectedChannel();
      if (!channel || !channel.profiles || channel.profiles.length === 0) {
        return null;
      }
      return channel.profiles.find(function(profile) {
        return profile.name === profileSelect.value;
      }) || channel.profiles[0];
    }

    function pretty(value) {
      return JSON.stringify(value, null, 2);
    }

    function setViewerMessage(message) {
      viewer.innerHTML = '';
      const node = document.createElement('div');
      node.className = 'viewer-empty';
      node.textContent = message;
      viewer.appendChild(node);
    }

    function renderViewer(mode) {
      const channel = selectedChannel();
      const profile = selectedProfile();
      lastStreamMode = mode;
      viewer.innerHTML = '';
      if (!channel) {
        setViewerMessage('No channel selected.');
        return;
      }
      if (mode !== 'snapshot' && !MEDIA_ENABLED) {
        setViewerMessage('Media layer is disabled for stream transports.');
        return;
      }
      if (mode !== 'snapshot' && !profile) {
        setViewerMessage('No stream profile is available for this channel.');
        return;
      }

      if (mode === 'snapshot') {
        const img = document.createElement('img');
        img.alt = channel.name + ' snapshot';
        img.src = bridgeURL(channel.snapshot_url + '?_=' + Date.now());
        viewer.appendChild(img);
        return;
      }
      if (mode === 'mjpeg') {
        const img = document.createElement('img');
        img.alt = channel.name + ' MJPEG';
        img.src = bridgeURL(profile.mjpeg_url);
        viewer.appendChild(img);
        return;
      }
      if (mode === 'hls') {
        const video = document.createElement('video');
        video.controls = true;
        video.autoplay = true;
        video.muted = true;
        video.playsInline = true;
        video.src = bridgeURL(profile.hls_url);
        viewer.appendChild(video);
        video.play().catch(function() {});
        return;
      }
      if (mode === 'preview' || mode === 'webrtc') {
        const iframe = document.createElement('iframe');
        iframe.title = channel.name + ' ' + mode;
        iframe.src = bridgeURL(mode === 'preview' ? profile.preview_url : profile.webrtc_url);
        viewer.appendChild(iframe);
        return;
      }
      setViewerMessage('Unknown stream mode.');
    }

    function updateChannelMeta() {
      const channel = selectedChannel();
      if (!channel) {
        channelMeta.textContent = 'No channel selected.';
        return;
      }
      channelMeta.textContent = pretty({
        name: channel.name,
        stream_id: channel.id,
        device_id: channel.device_id,
        channel: channel.channel,
        recommended_profile: channel.recommended_profile,
        main_video: channel.main_video || '',
        sub_video: channel.sub_video || '',
        audio_codec: channel.audio_codec || '',
        controls: channel.controls || null,
        features: channel.features || [],
      });
    }

    function populateProfiles() {
      const channel = selectedChannel();
      profileSelect.innerHTML = '';
      if (!channel || !channel.profiles) {
        profileSelect.disabled = true;
        return;
      }
      channel.profiles.forEach(function(profile) {
        const option = document.createElement('option');
        option.value = profile.name;
        option.textContent = profile.label;
        if (profile.name === channel.recommended_profile) {
          option.selected = true;
        }
        profileSelect.appendChild(option);
      });
      profileSelect.disabled = channel.profiles.length === 0;
    }

    function populateChannels() {
      channelSelect.innerHTML = '';
      CHANNELS.forEach(function(channel, index) {
        const option = document.createElement('option');
        option.value = String(index);
        option.textContent = 'ch ' + channel.channel + ' - ' + channel.name + ' (' + channel.device_id + ')';
        channelSelect.appendChild(option);
      });
      const hasChannels = CHANNELS.length > 0;
      channelSelect.disabled = !hasChannels;
      actionButtons.forEach(function(button) {
        button.disabled = !hasChannels || !ACTIONS_AVAILABLE;
      });
      streamButtons.forEach(function(button) {
        button.disabled = !hasChannels;
      });
      populateProfiles();
      updateChannelMeta();
      renderViewer('snapshot');
      if (!hasChannels) {
        setViewerMessage('No NVR channel streams are currently in the catalog.');
      }
    }

    function logResult(title, payload, isError) {
      const stamp = new Date().toISOString();
      const body = typeof payload === 'string' ? payload : pretty(payload);
      const prior = resultLog.textContent === 'No test action has run yet.' ? '' : '\n\n' + resultLog.textContent;
      resultLog.textContent = '[' + stamp + '] ' + title + '\n' + body + prior;
      resultLog.style.borderColor = isError ? 'rgba(235, 116, 122, 0.45)' : 'rgba(98, 208, 170, 0.28)';
    }

    function channelURL(channel, suffix) {
      return '/api/v1/nvr/' + encodeURIComponent(channel.device_id) + '/channels/' + channel.channel + suffix;
    }

    async function requestJSON(method, path, payload) {
      const init = { method: method };
      if (payload !== undefined && payload !== null) {
        init.headers = { 'Content-Type': 'application/json' };
        init.body = JSON.stringify(payload);
      }
      const response = await bridgeFetch(path, init);
      const text = await response.text();
      let body = text;
      try {
        body = text ? JSON.parse(text) : {};
      } catch (error) {
        body = text;
      }
      if (!response.ok) {
        const message = typeof body === 'string' ? body : pretty(body);
        throw new Error(message || response.statusText || 'request failed');
      }
      return body;
    }

    async function runAction(button) {
      const channel = selectedChannel();
      if (!channel) {
        logResult('No channel selected', 'Select a channel first.', true);
        return;
      }
      const call = button.dataset.call;
      const previous = button.textContent;
      button.disabled = true;
      button.textContent = 'Running...';
      try {
        let method = 'POST';
        let path = '';
        let payload = null;
        if (call === 'controls') {
          method = 'GET';
          path = channelURL(channel, '/controls');
        } else if (call === 'probe') {
          path = '/api/v1/devices/' + encodeURIComponent(channel.device_id) + '/probe';
        } else if (call === 'refresh') {
          path = '/api/v1/nvr/' + encodeURIComponent(channel.device_id) + '/inventory/refresh';
        } else if (call === 'ptz') {
          path = channelURL(channel, '/ptz');
          payload = { action: 'pulse', command: button.dataset.command, speed: 3, duration_ms: 350 };
        } else if (call === 'aux') {
          path = channelURL(channel, '/aux');
          payload = {
            action: button.dataset.action,
            output: button.dataset.output,
            duration_ms: Number(button.dataset.duration || 300),
          };
        } else if (call === 'recording') {
          path = channelURL(channel, '/recording');
          payload = { action: button.dataset.action };
        } else if (call === 'diagnostic') {
          path = channelURL(channel, '/diagnostics');
          payload = {
            method: button.dataset.method,
            action: button.dataset.action,
            duration_ms: Number(button.dataset.duration || 300),
          };
        } else {
          throw new Error('unknown action ' + call);
        }
        const result = await requestJSON(method, path, payload);
        logResult(method + ' ' + path, result, false);
      } catch (error) {
        logResult('Action failed', error && error.message ? error.message : String(error), true);
      } finally {
        button.disabled = !ACTIONS_AVAILABLE;
        button.textContent = previous;
      }
    }

    channelSelect.addEventListener('change', function() {
      populateProfiles();
      updateChannelMeta();
      renderViewer(lastStreamMode);
    });
    profileSelect.addEventListener('change', function() {
      updateChannelMeta();
      renderViewer(lastStreamMode);
    });
    streamButtons.forEach(function(button) {
      button.addEventListener('click', function() {
        renderViewer(button.dataset.streamMode);
      });
    });
    actionButtons.forEach(function(button) {
      button.addEventListener('click', function() {
        runAction(button);
      });
    });
    populateChannels();
  </script>
</body>
</html>`,
		len(channels),
		boolText(mediaEnabled, "good", "warn"),
		boolText(mediaEnabled, "enabled", "disabled"),
		boolText(actionsAvailable, "good", "warn"),
		boolText(actionsAvailable, "enabled", "disabled"),
		channelsJSON,
		actionsAvailable,
		mediaEnabled,
	)
}

func buildTestBridgeChannels(streamEntries []streams.Entry) []testBridgeChannel {
	channels := make([]testBridgeChannel, 0)
	for _, entry := range streamEntries {
		if entry.DeviceKind != dahua.DeviceKindNVRChannel || entry.Channel <= 0 || strings.TrimSpace(entry.RootDeviceID) == "" {
			continue
		}
		profiles := buildTestBridgeProfiles(entry)
		channels = append(channels, testBridgeChannel{
			ID:                 entry.ID,
			DeviceID:           entry.RootDeviceID,
			Name:               firstNonEmpty(entry.Name, entry.ID),
			Channel:            entry.Channel,
			SnapshotURL:        buildTestNVRChannelSnapshotURL(entry.RootDeviceID, entry.Channel),
			RecommendedProfile: firstNonEmpty(entry.RecommendedProfile, firstTestBridgeProfileName(profiles)),
			MainVideo:          strings.TrimSpace(firstNonEmpty(entry.MainCodec+" "+entry.MainResolution, entry.MainCodec, entry.MainResolution)),
			SubVideo:           strings.TrimSpace(firstNonEmpty(entry.SubCodec+" "+entry.SubResolution, entry.SubCodec, entry.SubResolution)),
			AudioCodec:         entry.AudioCodec,
			Profiles:           profiles,
			Controls:           entry.Controls,
			Features:           entry.Features,
		})
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].DeviceID != channels[j].DeviceID {
			return channels[i].DeviceID < channels[j].DeviceID
		}
		if channels[i].Channel != channels[j].Channel {
			return channels[i].Channel < channels[j].Channel
		}
		return channels[i].ID < channels[j].ID
	})
	return channels
}

func buildTestBridgeProfiles(entry streams.Entry) []testBridgeProfile {
	names := orderedTestBridgeProfileNames(entry.Profiles, entry.RecommendedProfile)
	profiles := make([]testBridgeProfile, 0, len(names))
	for _, name := range names {
		profile := entry.Profiles[name]
		label := name
		if profile.Recommended || name == entry.RecommendedProfile {
			label += " (recommended)"
		}
		profiles = append(profiles, testBridgeProfile{
			Name:        name,
			Label:       label,
			SnapshotURL: buildTestMediaSnapshotURL(entry.ID, name),
			MJPEGURL:    buildTestMediaMJPEGURL(entry.ID, name),
			HLSURL:      buildTestMediaHLSURL(entry.ID, name),
			PreviewURL:  buildTestMediaPreviewURL(entry.ID, name),
			WebRTCURL:   buildTestMediaWebRTCURL(entry.ID, name),
			Width:       profile.SourceWidth,
			Height:      profile.SourceHeight,
		})
	}
	return profiles
}

func orderedTestBridgeProfileNames(profiles map[string]streams.Profile, recommended string) []string {
	if len(profiles) == 0 {
		return nil
	}

	ordered := make([]string, 0, len(profiles))
	seen := make(map[string]struct{}, len(profiles))
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := profiles[name]; !ok {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		ordered = append(ordered, name)
	}

	appendName(recommended)
	for _, name := range []string{"stable", "substream", "quality", "default"} {
		appendName(name)
	}
	extras := make([]string, 0, len(profiles))
	for name := range profiles {
		if _, ok := seen[name]; ok {
			continue
		}
		extras = append(extras, name)
	}
	sort.Strings(extras)
	for _, name := range extras {
		appendName(name)
	}
	return ordered
}

func firstTestBridgeProfileName(profiles []testBridgeProfile) string {
	if len(profiles) == 0 {
		return ""
	}
	return profiles[0].Name
}

func buildTestNVRChannelSnapshotURL(deviceID string, channel int) string {
	return "/api/v1/nvr/" + url.PathEscape(deviceID) + "/channels/" + strconv.Itoa(channel) + "/snapshot"
}

func buildTestMediaSnapshotURL(streamID string, profile string) string {
	return "/api/v1/media/snapshot/" + url.PathEscape(streamID) + "?profile=" + url.QueryEscape(profile)
}

func buildTestMediaMJPEGURL(streamID string, profile string) string {
	return "/api/v1/media/mjpeg/" + url.PathEscape(streamID) + "?profile=" + url.QueryEscape(profile) + "&width=960"
}

func buildTestMediaHLSURL(streamID string, profile string) string {
	return "/api/v1/media/hls/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile) + "/index.m3u8"
}

func buildTestMediaPreviewURL(streamID string, profile string) string {
	return "/api/v1/media/preview/" + url.PathEscape(streamID) + "?profile=" + url.QueryEscape(profile)
}

func buildTestMediaWebRTCURL(streamID string, profile string) string {
	return "/api/v1/media/webrtc/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile)
}

func buildAdminEndpointSections(healthPath string, metricsPath string) string {
	sections := []struct {
		Title string
		Items []adminEndpoint
	}{
		{
			Title: "Status And Inventory",
			Items: []adminEndpoint{
				{Method: "GET", Path: "/admin", Description: "Operator page", Linkable: true},
				{Method: "GET", Path: "/admin/test-bridge", Description: "NVR channel stream and control diagnostics page", Linkable: true},
				{Method: "GET", Path: healthPath, Description: "Liveness probe", Linkable: true},
				{Method: "GET", Path: "/readyz", Description: "Readiness probe", Linkable: true},
				{Method: "GET", Path: metricsPath, Description: "Prometheus metrics", Linkable: true},
				{Method: "GET", Path: "/api/v1/status", Description: "Bridge status JSON", Linkable: true},
				{Method: "GET", Path: "/api/v1/devices", Description: "Current probe results", Linkable: true},
				{Method: "GET", Path: "/api/v1/streams", Description: "Full stream catalog", Linkable: true},
				{Method: "GET", Path: "/api/v1/media/workers", Description: "Runtime media worker state", Linkable: true},
			},
		},
		{
			Title: "Events And Media",
			Items: []adminEndpoint{
				{Method: "GET", Path: "/api/v1/home-assistant/native/catalog", Description: "Bridge-native Home Assistant catalog", Linkable: true},
			},
		},
		{
			Title: "Mutating Admin APIs",
			Items: []adminEndpoint{
				{Method: "POST", Path: "/api/v1/devices/probe-all", Description: "Probe every configured device", Linkable: false},
				{Method: "POST", Path: "/api/v1/devices/{deviceID}/probe", Description: "Probe one specific device", Linkable: false},
				{Method: "POST", Path: "/api/v1/devices/{deviceID}/credentials", Description: "Rotate bridge-side device credentials", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/inventory/refresh", Description: "Refresh NVR channel/disk inventory", Linkable: false},
				{Method: "GET", Path: "/api/v1/nvr/{deviceID}/recordings?channel=1&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=25", Description: "Search NVR archive recordings by channel and time range", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/recordings/export", Description: "Export an NVR archive playback window as a bridge MP4 clip", Linkable: false},
				{Method: "GET", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/controls", Description: "Read NVR per-channel PTZ capability data", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/ptz", Description: "Send PTZ start/stop/pulse command to an NVR channel", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/aux", Description: "Send aux/light/wiper start/stop/pulse command to an NVR channel", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/recording", Description: "Set manual recording mode for an NVR channel", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/diagnostics", Description: "Run one NVR/direct-IPC diagnostic control strategy for a channel", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/playback/sessions", Description: "Create an NVR archive playback session backed by bridge media endpoints", Linkable: false},
				{Method: "GET", Path: "/api/v1/nvr/playback/sessions/{sessionID}", Description: "Inspect an active NVR archive playback session", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/playback/sessions/{sessionID}/seek", Description: "Create a new playback session starting from a different archive timestamp", Linkable: false},
				{Method: "GET", Path: "/api/v1/vto/{deviceID}/controls", Description: "Inspect detected VTO call, lock, recording, and talkback capabilities", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/call/answer", Description: "Request VTO call answer", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/call/hangup", Description: "Request VTO hangup", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/locks/{lockIndex}/unlock", Description: "Trigger VTO door unlock for one configured lock", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/recording", Description: "Set VTO automatic call recording", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/intercom/reset", Description: "Reset active bridge WebRTC intercom session", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/intercom/uplink/enable", Description: "Enable external RTP uplink forwarding for the VTO intercom session", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/intercom/uplink/disable", Description: "Disable external RTP uplink forwarding for the VTO intercom session", Linkable: false},
			},
		},
	}

	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		rows := make([]string, 0, len(section.Items))
		for _, item := range section.Items {
			target := "<code>" + htmlEscape(item.Path) + "</code>"
			if item.Linkable {
				target = `<a href="` + htmlEscape(item.Path) + `"><code>` + htmlEscape(item.Path) + `</code></a>`
			}
			rows = append(rows, fmt.Sprintf(
				`<div class="endpoint-row"><span class="method">%s</span><div class="endpoint-main">%s<div class="endpoint-desc">%s</div></div></div>`,
				htmlEscape(item.Method),
				target,
				htmlEscape(item.Description),
			))
		}
		parts = append(parts, `<div class="endpoint-group"><h3 class="card-title">`+htmlEscape(section.Title)+`</h3><div class="endpoint-list">`+strings.Join(rows, "")+`</div></div>`)
	}
	return strings.Join(parts, "")
}

type adminControlStats struct {
	ActionableEntries   int
	NVRPTZEntries       int
	NVRAuxEntries       int
	NVRRecordingEntries int
	VTORecordingEntries int
}

func (s adminControlStats) Summary() string {
	return fmt.Sprintf(
		"%d PTZ | %d aux | %d NVR rec | %d VTO rec",
		s.NVRPTZEntries,
		s.NVRAuxEntries,
		s.NVRRecordingEntries,
		s.VTORecordingEntries,
	)
}

func summarizeAdminControlStats(streamEntries []streams.Entry) adminControlStats {
	var stats adminControlStats
	for _, entry := range streamEntries {
		actionable := false
		if entry.Controls != nil {
			if entry.Controls.PTZ != nil && entry.Controls.PTZ.Supported {
				stats.NVRPTZEntries++
				actionable = true
			}
			if entry.Controls.Aux != nil && entry.Controls.Aux.Supported {
				stats.NVRAuxEntries++
				actionable = true
			}
			if entry.Controls.Recording != nil && entry.Controls.Recording.Supported {
				stats.NVRRecordingEntries++
				actionable = true
			}
		}
		if entry.Intercom != nil {
			if entry.Intercom.SupportsVTORecordingControl {
				stats.VTORecordingEntries++
				actionable = true
			}
		}
		if actionable {
			stats.ActionableEntries++
		}
	}
	return stats
}

func buildAdminDeviceCards(probeResults []*dahua.ProbeResult, streamEntries []streams.Entry) string {
	if len(probeResults) == 0 {
		return `<p class="muted-note">No devices are currently available in the probe store.</p>`
	}

	streamsByRoot := make(map[string][]streams.Entry)
	for _, entry := range streamEntries {
		if entry.RootDeviceID == "" {
			continue
		}
		streamsByRoot[entry.RootDeviceID] = append(streamsByRoot[entry.RootDeviceID], entry)
	}

	cards := make([]string, 0, len(probeResults))
	for _, result := range probeResults {
		if result == nil {
			continue
		}
		root := result.Root
		rootStreams := streamsByRoot[root.ID]
		chips := []string{
			adminLinkChip("device detail", "/api/v1/devices/"+url.PathEscape(root.ID), false),
			adminLinkChip("device streams", "/api/v1/streams?device_id="+url.QueryEscape(root.ID), true),
		}
		switch root.Kind {
		case dahua.DeviceKindVTO:
			chips = append(chips, adminLinkChip("snapshot", "/api/v1/vto/"+url.PathEscape(root.ID)+"/snapshot", false))
			chips = append(chips, buildAdminRootControlChips(root.ID, rootStreams)...)
		case dahua.DeviceKindIPC:
			chips = append(chips, adminLinkChip("snapshot", "/api/v1/ipc/"+url.PathEscape(root.ID)+"/snapshot", false))
		case dahua.DeviceKindNVR:
			if len(rootStreams) > 0 && rootStreams[0].Channel > 0 {
				recordingsURL := fmt.Sprintf(
					"/api/v1/nvr/%s/recordings?channel=%d&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=25",
					url.PathEscape(root.ID),
					rootStreams[0].Channel,
				)
				chips = append(chips, adminLinkChip("recordings", recordingsURL, true))
			}
		}

		for _, entry := range rootStreams {
			if entry.LocalPreviewURL != "" {
				chips = append(chips, adminLinkChip("preview", entry.LocalPreviewURL, true))
			}
			if entry.LocalIntercomURL != "" {
				chips = append(chips, adminLinkChip("intercom", entry.LocalIntercomURL, false))
			}
			break
		}

		metaParts := []string{
			string(root.Kind),
			"id=" + root.ID,
			"model=" + firstNonEmpty(root.Model, "unknown"),
			fmt.Sprintf("children=%d", len(result.Children)),
			fmt.Sprintf("streams=%d", len(rootStreams)),
		}
		if controlSummary := buildAdminRootControlSummary(rootStreams); controlSummary != "" {
			metaParts = append(metaParts, controlSummary)
		}
		if notes := buildAdminValidationNoteMarkup(collectAdminValidationNotes(rootStreams)); notes != "" {
			chips = append(chips, notes)
		}

		cards = append(cards, fmt.Sprintf(
			`<article class="device-card"><h3 class="card-title">%s</h3><div class="card-meta">%s</div><div class="chip-row">%s</div></article>`,
			htmlEscape(firstNonEmpty(root.Name, root.ID)),
			adminMetaLine(metaParts...),
			strings.Join(chips, ""),
		))
	}
	return `<div class="card-grid">` + strings.Join(cards, "") + `</div>`
}

func buildAdminStreamCards(streamEntries []streams.Entry) string {
	if len(streamEntries) == 0 {
		return `<p class="muted-note">No stream entries are currently available.</p>`
	}

	cards := make([]string, 0, len(streamEntries))
	for _, entry := range streamEntries {
		chips := []string{
			adminLinkChip("stream detail", "/api/v1/streams/"+url.PathEscape(entry.ID), false),
		}
		if entry.SnapshotURL != "" {
			chips = append(chips, adminLinkChip("snapshot", entry.SnapshotURL, true))
		}
		if entry.LocalPreviewURL != "" {
			chips = append(chips, adminLinkChip("preview", entry.LocalPreviewURL, false))
		}
		if entry.LocalIntercomURL != "" {
			chips = append(chips, adminLinkChip("intercom", entry.LocalIntercomURL, false))
		}
		if profile, ok := entry.Profiles[entry.RecommendedProfile]; ok {
			if profile.LocalWebRTCURL != "" {
				chips = append(chips, adminLinkChip("webrtc", profile.LocalWebRTCURL, true))
			}
			if profile.LocalHLSURL != "" {
				chips = append(chips, adminLinkChip("hls", profile.LocalHLSURL, true))
			}
			if profile.LocalMJPEGURL != "" {
				chips = append(chips, adminLinkChip("mjpeg", profile.LocalMJPEGURL, true))
			}
		}
		chips = append(chips, buildAdminStreamControlChips(entry)...)
		if liveSourceControl := buildAdminLiveSourceControl(entry); liveSourceControl != "" {
			chips = append(chips, liveSourceControl)
		}
		if notes := buildAdminValidationNoteMarkup(adminValidationNotesForEntry(entry)); notes != "" {
			chips = append(chips, notes)
		}

		videoSummary := strings.TrimSpace(firstNonEmpty(entry.MainCodec, "unknown") + " " + firstNonEmpty(entry.MainResolution, ""))
		metaParts := []string{
			string(entry.DeviceKind),
			"recommended=" + firstNonEmpty(entry.RecommendedProfile, "unknown"),
			"video=" + videoSummary,
			"audio=" + firstNonEmpty(entry.AudioCodec, "none"),
		}
		if entry.Channel > 0 {
			metaParts = append(metaParts, fmt.Sprintf("channel=%d", entry.Channel))
		}
		if controlSummary := buildAdminStreamControlSummary(entry); controlSummary != "" {
			metaParts = append(metaParts, controlSummary)
		}

		cards = append(cards, fmt.Sprintf(
			`<article class="stream-card"><h3 class="card-title">%s</h3><div class="card-meta">%s</div><div class="chip-row">%s</div></article>`,
			htmlEscape(firstNonEmpty(entry.Name, entry.ID)),
			adminMetaLine(metaParts...),
			strings.Join(chips, ""),
		))
	}
	return `<div class="card-grid">` + strings.Join(cards, "") + `</div>`
}

func buildAdminRootControlSummary(entries []streams.Entry) string {
	var parts []string
	for _, entry := range entries {
		if entry.Intercom == nil {
			continue
		}
		if entry.Intercom.SupportsVTOCallAnswer || entry.Intercom.SupportsHangup {
			parts = append(parts, "call control")
		}
		if len(entry.Intercom.LockURLs) > 0 {
			parts = append(parts, fmt.Sprintf("locks=%d", len(entry.Intercom.LockURLs)))
		}
		if entry.Intercom.SupportsVTORecordingControl {
			parts = append(parts, "auto record")
		}
		break
	}
	return strings.Join(parts, ", ")
}

func buildAdminRootControlChips(rootID string, entries []streams.Entry) []string {
	for _, entry := range entries {
		if entry.Intercom == nil {
			continue
		}
		var chips []string
		chips = append(chips, adminLinkChip("vto controls", "/api/v1/vto/"+url.PathEscape(rootID)+"/controls", false))
		if entry.Intercom.AnswerURL != "" {
			chips = append(chips, adminLinkChip("answer", entry.Intercom.AnswerURL, true))
		}
		if entry.Intercom.HangupURL != "" {
			chips = append(chips, adminLinkChip("hangup", entry.Intercom.HangupURL, true))
		}
		for index, lockURL := range entry.Intercom.LockURLs {
			chips = append(chips, adminLinkChip(fmt.Sprintf("unlock %d", index+1), lockURL, false))
		}
		if entry.Intercom.RecordingURL != "" {
			chips = append(chips, adminLinkChip("auto record", entry.Intercom.RecordingURL, true))
		}
		if entry.Intercom.BridgeSessionResetURL != "" {
			chips = append(chips, adminLinkChip("reset intercom", entry.Intercom.BridgeSessionResetURL, true))
		}
		return chips
	}
	return nil
}

func buildAdminStreamControlSummary(entry streams.Entry) string {
	var parts []string
	if entry.Controls != nil {
		if entry.Controls.PTZ != nil && entry.Controls.PTZ.Supported {
			parts = append(parts, "ptz")
		}
		if entry.Controls.Aux != nil && entry.Controls.Aux.Supported {
			auxSummary := "aux"
			if len(entry.Controls.Aux.Features) > 0 {
				auxSummary = "aux=" + strings.Join(entry.Controls.Aux.Features, ",")
			} else if len(entry.Controls.Aux.Outputs) > 0 {
				auxSummary = "aux=" + strings.Join(entry.Controls.Aux.Outputs, ",")
			}
			parts = append(parts, auxSummary)
		}
		if entry.Controls.Audio != nil {
			audioParts := make([]string, 0, 3)
			if entry.Controls.Audio.PlaybackSupported {
				audioParts = append(audioParts, "playback")
			}
			if len(audioParts) > 0 {
				parts = append(parts, "audio="+strings.Join(audioParts, ","))
			}
		}
		if entry.Controls.Recording != nil && entry.Controls.Recording.Supported {
			recordingSummary := "recording=" + firstNonEmpty(entry.Controls.Recording.Mode, "supported")
			if entry.Controls.Recording.Active {
				recordingSummary += " active"
			}
			parts = append(parts, recordingSummary)
		}
	}
	if entry.Intercom != nil {
		if entry.Intercom.SupportsVTORecordingControl {
			parts = append(parts, "vto recording")
		}
		if entry.Intercom.CallState != "" {
			parts = append(parts, "call="+entry.Intercom.CallState)
		}
	}
	return strings.Join(parts, ", ")
}

func buildAdminStreamControlChips(entry streams.Entry) []string {
	var chips []string
	if entry.Controls != nil {
		if entry.Channel > 0 {
			chips = append(chips, adminLinkChip("channel controls", fmt.Sprintf("/api/v1/nvr/%s/channels/%d/controls", url.PathEscape(entry.RootDeviceID), entry.Channel), true))
			chips = append(chips, adminLinkChip("recordings", fmt.Sprintf("/api/v1/nvr/%s/recordings?channel=%d&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=25", url.PathEscape(entry.RootDeviceID), entry.Channel), true))
		}
		if entry.Controls.PTZ != nil && entry.Controls.PTZ.URL != "" {
			chips = append(chips, adminLinkChip("ptz", entry.Controls.PTZ.URL, false))
		}
		if entry.Controls.Aux != nil && entry.Controls.Aux.URL != "" {
			chips = append(chips, adminLinkChip("aux", entry.Controls.Aux.URL, false))
		}
		if entry.Controls.Recording != nil && entry.Controls.Recording.URL != "" {
			chips = append(chips, adminLinkChip("recording", entry.Controls.Recording.URL, false))
		}
	}
	if entry.Intercom != nil {
		if entry.Intercom.AnswerURL != "" {
			chips = append(chips, adminLinkChip("answer", entry.Intercom.AnswerURL, true))
		}
		if entry.Intercom.HangupURL != "" {
			chips = append(chips, adminLinkChip("hangup", entry.Intercom.HangupURL, true))
		}
		if entry.Intercom.RecordingURL != "" {
			chips = append(chips, adminLinkChip("auto record", entry.Intercom.RecordingURL, true))
		}
	}
	return chips
}

func collectAdminValidationNotes(entries []streams.Entry) []string {
	seen := make(map[string]struct{})
	notes := make([]string, 0)
	for _, entry := range entries {
		for _, note := range adminValidationNotesForEntry(entry) {
			if note == "" {
				continue
			}
			if _, ok := seen[note]; ok {
				continue
			}
			seen[note] = struct{}{}
			notes = append(notes, note)
		}
	}
	return notes
}

func adminValidationNotesForEntry(entry streams.Entry) []string {
	notes := make([]string, 0)
	if entry.Controls != nil {
		notes = append(notes, entry.Controls.ValidationNotes...)
	}
	if entry.Intercom != nil {
		notes = append(notes, entry.Intercom.ValidationNotes...)
	}
	return notes
}

func buildAdminValidationNoteMarkup(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	summary := strings.Join(notes, " | ")
	if len(summary) > 180 {
		summary = summary[:177] + "..."
	}
	return adminTextChip("validated: "+summary, true)
}

func adminLinkChip(label string, href string, subtle bool) string {
	className := "chip"
	if subtle {
		className += " subtle"
	}
	return `<span class="` + className + `"><a href="` + htmlEscape(href) + `"><code>` + htmlEscape(label) + `</code></a></span>`
}

func adminTextChip(label string, subtle bool) string {
	className := "chip"
	if subtle {
		className += " subtle"
	}
	return `<span class="` + className + `"><code>` + htmlEscape(label) + `</code></span>`
}

func adminMetaLine(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		filtered = append(filtered, htmlEscape(strings.TrimSpace(part)))
	}
	return strings.Join(filtered, " &bull; ")
}

func marshalIndentedJSON(payload any) string {
	if payload == nil {
		return "{}"
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(body)
}

func marshalJSONForScript(payload any) string {
	body, err := json.Marshal(payload)
	if err != nil {
		return "null"
	}
	return string(body)
}

func boolText(value bool, trueText string, falseText string) string {
	if value {
		return trueText
	}
	return falseText
}

func boolHTMLAttr(enabled bool) string {
	if enabled {
		return ""
	}
	return "disabled"
}
