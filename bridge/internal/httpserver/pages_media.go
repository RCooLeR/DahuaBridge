package httpserver

import (
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/streams"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func renderMediaPreviewPage(entry streams.Entry, profileName string, profile streams.Profile) string {
	title := htmlEscape(entry.Name) + " Preview"
	profileLinks := buildPreviewProfileLinks(entry, profileName)
	audioNote := "Audio is available only when the browser can play the HLS stream directly."
	if strings.TrimSpace(entry.AudioCodec) == "" {
		audioNote = "The source stream does not advertise audio, so preview is video-only."
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0c1114;
      --panel: #111a1d;
      --panel-alt: #151e21;
      --text: #f1f5f2;
      --muted: #a8b8b2;
      --line: rgba(153, 174, 166, 0.2);
      --accent: #5ed0ac;
      --accent-soft: rgba(94, 208, 172, 0.14);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background: linear-gradient(180deg, #151e21 0, var(--bg) 72%%);
      color: var(--text);
      min-height: 100vh;
    }
    main {
      max-width: 1040px;
      margin: 0 auto;
      padding: 24px;
    }
    .hero {
      display: grid;
      gap: 12px;
      margin-bottom: 18px;
    }
    .eyebrow {
      display: inline-flex;
      width: fit-content;
      gap: 8px;
      padding: 6px 10px;
      border-radius: 8px;
      background: var(--accent-soft);
      color: var(--accent);
      font-size: 13px;
      letter-spacing: 0;
      text-transform: uppercase;
    }
    h1 {
      margin: 0;
      font-size: clamp(28px, 5vw, 44px);
      line-height: 1.05;
    }
    .subtle {
      color: var(--muted);
      margin: 0;
      max-width: 72ch;
    }
    .layout {
      display: grid;
      grid-template-columns: minmax(0, 2fr) minmax(280px, 1fr);
      gap: 18px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
      box-shadow: 0 18px 60px rgba(0, 0, 0, 0.28);
    }
    .player-wrap {
      position: relative;
      background: #040a10;
      aspect-ratio: 16 / 9;
    }
    video, img {
      width: 100%%;
      height: 100%%;
      object-fit: contain;
      display: none;
      background: #040a10;
    }
    .fallback-note {
      position: absolute;
      inset: auto 16px 16px 16px;
      padding: 12px 14px;
      border-radius: 8px;
      background: rgba(4, 10, 16, 0.74);
      border: 1px solid rgba(255,255,255,0.08);
      color: var(--muted);
      font-size: 14px;
      display: none;
    }
    .panel-body {
      padding: 18px;
      display: grid;
      gap: 14px;
    }
    .profile-links {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    .profile-links a {
      padding: 10px 12px;
      border-radius: 8px;
      text-decoration: none;
      color: var(--text);
      background: rgba(255,255,255,0.04);
      border: 1px solid var(--line);
      font-size: 14px;
    }
    .profile-links a.active {
      color: #04261b;
      background: var(--accent);
      border-color: var(--accent);
      font-weight: 600;
    }
    .meta {
      display: grid;
      gap: 10px;
    }
    .meta-row {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      padding-bottom: 10px;
      border-bottom: 1px solid rgba(255,255,255,0.05);
    }
    .meta-row:last-child {
      border-bottom: 0;
      padding-bottom: 0;
    }
    .meta-key {
      color: var(--muted);
    }
    code {
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 100%%;
    }
    @media (max-width: 860px) {
      .layout {
        grid-template-columns: 1fr;
      }
      main {
        padding: 16px;
      }
    }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <div class="eyebrow">Bridge Preview</div>
      <h1>%s</h1>
      <p class="subtle">This page stays on the bridge host and chooses the best browser-safe live path available for this stream. Native HLS is preferred when supported; otherwise the page falls back to low-latency MJPEG.</p>
    </section>
    <section class="layout">
      <article class="panel">
        <div class="player-wrap">
          <video id="video" controls autoplay muted playsinline preload="auto"></video>
          <img id="mjpeg" alt="%s live preview">
          <div id="fallback-note" class="fallback-note"></div>
        </div>
        <div class="panel-body">
          <div class="profile-links">%s</div>
          <p class="subtle">%s</p>
        </div>
      </article>
      <aside class="panel">
        <div class="panel-body meta">
          <div class="meta-row"><span class="meta-key">Device</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Kind</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Profile</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Video</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Audio</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Snapshot</span><code>%s</code></div>
          <div class="meta-row"><span class="meta-key">HLS</span><code>%s</code></div>
          <div class="meta-row"><span class="meta-key">MJPEG</span><code>%s</code></div>
        </div>
      </aside>
    </section>
  </main>
  <script>
    bridgePreparePage();
    const video = document.getElementById('video');
    const mjpeg = document.getElementById('mjpeg');
    const fallback = document.getElementById('fallback-note');
    const hlsURL = %q;
    const mjpegURL = %q;

    const canPlayNativeHLS = !!video.canPlayType('application/vnd.apple.mpegurl') || !!video.canPlayType('application/x-mpegURL');
    if (canPlayNativeHLS && hlsURL) {
      video.src = bridgeURL(hlsURL);
      video.style.display = 'block';
      video.play().catch(() => {});
    } else {
      mjpeg.src = bridgeURL(mjpegURL);
      mjpeg.style.display = 'block';
      fallback.style.display = 'block';
      fallback.textContent = canPlayNativeHLS ? '' : 'Native HLS is not available in this browser, so the preview is using MJPEG for low-latency playback.';
    }
  </script>
</body>
</html>`,
		title,
		htmlEscape(entry.Name),
		htmlEscape(entry.Name),
		profileLinks,
		htmlEscape(audioNote),
		htmlEscape(entry.Name),
		htmlEscape(string(entry.DeviceKind)),
		htmlEscape(profileName),
		htmlEscape(firstNonEmpty(entry.MainCodec+" "+entry.MainResolution, "unknown")),
		htmlEscape(firstNonEmpty(entry.AudioCodec, "none")),
		htmlEscape(entry.SnapshotURL),
		htmlEscape(profile.LocalHLSURL),
		htmlEscape(profile.LocalMJPEGURL),
		profile.LocalHLSURL,
		profile.LocalMJPEGURL,
	)
}

func renderWebRTCPage(entry streams.Entry, profileName string, profile streams.Profile, iceServers []mediaapi.WebRTCICEServer) string {
	title := htmlEscape(entry.Name) + " WebRTC"
	offerURL := "/api/v1/media/webrtc/" + url.PathEscape(entry.ID) + "/" + url.PathEscape(profileName) + "/offer"
	iceServersJSON := marshalWebRTCICEServers(iceServers)
	iceModeLabel := "default host candidates"
	if len(iceServers) > 0 {
		iceModeLabel = fmt.Sprintf("configured STUN/TURN (%d)", len(iceServers))
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0c1114;
      --panel: #111a1d;
      --line: rgba(153, 174, 166, 0.2);
      --text: #f1f5f2;
      --muted: #a8b8b2;
      --accent: #5ed0ac;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background: linear-gradient(180deg, #151e21 0, var(--bg) 72%%);
      color: var(--text);
      min-height: 100vh;
    }
    main {
      max-width: 920px;
      margin: 0 auto;
      padding: 24px;
      display: grid;
      gap: 18px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
    }
    .hero {
      padding: 22px;
      display: grid;
      gap: 10px;
    }
    .eyebrow {
      color: var(--accent);
      text-transform: uppercase;
      letter-spacing: 0;
      font-size: 12px;
    }
    h1 {
      margin: 0;
      font-size: clamp(28px, 5vw, 42px);
      line-height: 1.05;
    }
    p {
      margin: 0;
      color: var(--muted);
    }
    video {
      width: 100%%;
      aspect-ratio: 16 / 9;
      background: #000;
      display: block;
    }
    .meta {
      padding: 18px 22px 22px;
      display: grid;
      gap: 10px;
    }
    .row {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      padding-bottom: 10px;
      border-bottom: 1px solid rgba(255,255,255,0.06);
    }
    .row:last-child {
      border-bottom: 0;
      padding-bottom: 0;
    }
    .label {
      color: var(--muted);
    }
    code {
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 100%%;
    }
  </style>
</head>
<body>
  <main>
    <section class="panel hero">
      <div class="eyebrow">Bridge WebRTC</div>
      <h1>%s</h1>
      <p>This page negotiates direct WebRTC playback through the bridge for lower-latency live media than HLS. It is playback-only and currently does not include talkback.</p>
    </section>
    <section class="panel">
      <video id="player" autoplay playsinline controls></video>
        <div class="meta">
          <div class="row"><span class="label">Status</span><span id="status">Negotiating...</span></div>
          <div class="row"><span class="label">Profile</span><span>%s</span></div>
          <div class="row"><span class="label">Audio</span><span>%s</span></div>
          <div class="row"><span class="label">ICE</span><span>%s</span></div>
          <div class="row"><span class="label">Fallback HLS</span><code>%s</code></div>
          <div class="row"><span class="label">Fallback MJPEG</span><code>%s</code></div>
        </div>
    </section>
  </main>
  <script>
    bridgePreparePage();
    const video = document.getElementById('player');
    const statusEl = document.getElementById('status');
    const offerURL = %q;
    const iceServers = %s;
    let peer = null;
    let reconnectTimer = null;
    let reconnectAttempts = 0;

    async function waitForIceComplete(pc) {
      if (pc.iceGatheringState === 'complete') {
        return;
      }
      await new Promise(resolve => {
        const onChange = () => {
          if (pc.iceGatheringState === 'complete') {
            pc.removeEventListener('icegatheringstatechange', onChange);
            resolve();
          }
        };
        pc.addEventListener('icegatheringstatechange', onChange);
      });
    }

    function clearReconnectTimer() {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    }

    function closePeer() {
      if (!peer) {
        return;
      }
      try {
        peer.ontrack = null;
        peer.onconnectionstatechange = null;
        peer.close();
      } catch (_) {}
      peer = null;
    }

    function reconnectDelayMilliseconds() {
      return Math.min(1000 * Math.pow(2, Math.min(reconnectAttempts, 4)), 10000);
    }

    function scheduleReconnect(reason) {
      if (reconnectTimer) {
        return;
      }
      reconnectAttempts += 1;
      const delay = reconnectDelayMilliseconds();
      statusEl.textContent = 'Reconnecting in ' + Math.max(1, Math.round(delay / 1000)) + 's';
      if (reason) {
        console.warn('webrtc reconnect scheduled:', reason);
      }
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        start(true).catch(handleStartError);
      }, delay);
    }

    function handleStartError(error) {
      console.error(error);
      statusEl.textContent = 'Retrying...';
      scheduleReconnect(error && error.message ? error.message : String(error));
    }

    async function start(isReconnect) {
      clearReconnectTimer();
      closePeer();
      statusEl.textContent = isReconnect ? 'Reconnecting...' : 'Negotiating...';

      const pc = new RTCPeerConnection({ iceServers });
      peer = pc;
      const stream = new MediaStream();
      video.srcObject = stream;

      pc.addTransceiver('video', { direction: 'recvonly' });
      pc.addTransceiver('audio', { direction: 'recvonly' });
      pc.ontrack = event => {
        if (peer !== pc) {
          return;
        }
        stream.addTrack(event.track);
        reconnectAttempts = 0;
        statusEl.textContent = 'Connected';
      };
      pc.onconnectionstatechange = () => {
        if (peer !== pc || !pc.connectionState) {
          return;
        }
        switch (pc.connectionState) {
        case 'connected':
          reconnectAttempts = 0;
          statusEl.textContent = 'Connected';
          return;
        case 'new':
          statusEl.textContent = 'Negotiating...';
          return;
        case 'connecting':
          statusEl.textContent = 'Connecting...';
          return;
        case 'disconnected':
        case 'failed':
          statusEl.textContent = pc.connectionState;
          scheduleReconnect('connection ' + pc.connectionState);
          return;
        case 'closed':
          if (peer === pc) {
            scheduleReconnect('connection closed');
          }
          return;
        default:
          statusEl.textContent = pc.connectionState;
        }
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      await waitForIceComplete(pc);

      const response = await bridgeFetch(offerURL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(pc.localDescription),
      });
      if (!response.ok) {
        closePeer();
        throw new Error(await response.text());
      }

      const answer = await response.json();
      await pc.setRemoteDescription(answer);
      video.play().catch(() => {
        statusEl.textContent = 'Ready - press play if autoplay was blocked';
      });
      if (statusEl.textContent === 'Connected') {
        return;
      }
      statusEl.textContent = 'Waiting for media...';
    }

    window.addEventListener('beforeunload', () => {
      clearReconnectTimer();
      closePeer();
    });
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
        return;
      }
      if (!peer || peer.connectionState === 'failed' || peer.connectionState === 'disconnected' || peer.connectionState === 'closed') {
        scheduleReconnect('page became visible');
      }
    });

    start(false).catch(handleStartError);
  </script>
</body>
</html>`,
		title,
		htmlEscape(entry.Name),
		htmlEscape(profileName),
		htmlEscape(firstNonEmpty(entry.AudioCodec, "none")),
		htmlEscape(iceModeLabel),
		htmlEscape(profile.LocalHLSURL),
		htmlEscape(profile.LocalMJPEGURL),
		offerURL,
		iceServersJSON,
	)
}

func renderVTOIntercomPage(entry streams.Entry, profileName string, profile streams.Profile, iceServers []mediaapi.WebRTCICEServer) string {
	title := htmlEscape(entry.Name) + " Intercom"
	offerURL := "/api/v1/media/webrtc/" + url.PathEscape(entry.ID) + "/" + url.PathEscape(profileName) + "/offer"
	deviceURL := "/api/v1/devices/" + url.PathEscape(entry.ID)
	intercomStatusURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/intercom/status"
	intercomResetURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/intercom/reset"
	answerURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/call/answer"
	hangupURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/call/hangup"
	uplinkEnableURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/intercom/uplink/enable"
	uplinkDisableURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/intercom/uplink/disable"
	profileLinks := buildIntercomProfileLinks(entry, profileName)
	audioLabel := firstNonEmpty(entry.AudioCodec, "none")
	iceServersJSON := marshalWebRTCICEServers(iceServers)
	iceModeLabel := "default host candidates"
	if len(iceServers) > 0 {
		iceModeLabel = fmt.Sprintf("configured STUN/TURN (%d)", len(iceServers))
	}
	externalUplinkTargetsLabel := "none"
	if entry.Intercom != nil && entry.Intercom.ConfiguredExternalUplinkTargetCount > 0 {
		externalUplinkTargetsLabel = fmt.Sprintf("%d configured", entry.Intercom.ConfiguredExternalUplinkTargetCount)
	}
	showAnswerControl := entry.Intercom != nil && entry.Intercom.SupportsVTOCallAnswer
	showExternalUplinkControl := entry.Intercom != nil && entry.Intercom.SupportsExternalAudioExport

	lockButtonCapacity := entry.LockCount
	if lockButtonCapacity < 1 {
		lockButtonCapacity = 1
	}
	lockButtons := make([]string, 0, lockButtonCapacity)
	if entry.LockCount <= 0 {
		lockButtons = append(lockButtons, `<div class="empty-note">No lock actions were discovered for this VTO.</div>`)
	} else {
		for index := 0; index < entry.LockCount; index++ {
			label := "Open Door"
			if index > 0 {
				label = fmt.Sprintf("Open Door %d", index+1)
			}
			lockButtons = append(lockButtons, fmt.Sprintf(
				`<button class="action-button secondary" type="button" data-lock-index="%d">%s</button>`,
				index,
				htmlEscape(label),
			))
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0c1114;
      --panel: #111a1d;
      --panel-alt: #151e21;
      --line: rgba(153, 174, 166, 0.2);
      --text: #f1f5f2;
      --muted: #a8b8b2;
      --accent: #5ed0ac;
      --accent-soft: rgba(94, 208, 172, 0.14);
      --danger: #ef747b;
      --danger-soft: rgba(239, 116, 123, 0.16);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background: linear-gradient(180deg, #151e21 0%%, var(--bg) 74%%);
      color: var(--text);
      min-height: 100vh;
    }
    main {
      max-width: 1180px;
      margin: 0 auto;
      padding: 24px;
      display: grid;
      gap: 18px;
    }
    .hero {
      display: grid;
      gap: 10px;
    }
    .eyebrow {
      display: inline-flex;
      width: fit-content;
      padding: 6px 10px;
      border-radius: 8px;
      background: var(--accent-soft);
      color: var(--accent);
      font-size: 13px;
      letter-spacing: 0;
      text-transform: uppercase;
    }
    h1 {
      margin: 0;
      font-size: clamp(28px, 5vw, 44px);
      line-height: 1.04;
    }
    .subtle {
      margin: 0;
      color: var(--muted);
      max-width: 76ch;
    }
    .layout {
      display: grid;
      grid-template-columns: minmax(0, 2fr) minmax(300px, 1fr);
      gap: 18px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
      box-shadow: 0 18px 52px rgba(0, 0, 0, 0.28);
    }
    .media-frame {
      position: relative;
      background: #000;
    }
    video {
      width: 100%%;
      aspect-ratio: 16 / 9;
      display: block;
      background: #000;
    }
    .status-pill {
      position: absolute;
      top: 16px;
      left: 16px;
      padding: 8px 12px;
      border-radius: 8px;
      background: rgba(4, 10, 16, 0.82);
      border: 1px solid rgba(255,255,255,0.08);
      color: var(--text);
      font-size: 14px;
      backdrop-filter: blur(10px);
    }
    .panel-body {
      padding: 18px;
      display: grid;
      gap: 16px;
    }
    .section-title {
      margin: 0;
      font-size: 15px;
      letter-spacing: 0;
      text-transform: uppercase;
      color: var(--muted);
    }
    .profile-links {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    .profile-links a {
      padding: 10px 12px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: rgba(255,255,255,0.02);
      color: var(--text);
      text-decoration: none;
    }
    .profile-links a.active {
      background: var(--accent-soft);
      color: var(--accent);
      border-color: rgba(91, 227, 189, 0.42);
    }
    .actions {
      display: grid;
      gap: 12px;
    }
    .action-row {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    .action-button {
      appearance: none;
      border: 0;
      border-radius: 8px;
      padding: 12px 16px;
      font: inherit;
      cursor: pointer;
      color: #08111a;
      background: var(--accent);
      font-weight: 600;
      min-width: 148px;
    }
    .action-button.secondary {
      background: #e8eef5;
    }
    .action-button.danger {
      background: var(--danger);
      color: white;
    }
    .action-button:disabled {
      cursor: wait;
      opacity: 0.6;
    }
    .status-grid {
      display: grid;
      gap: 12px;
    }
    .status-row {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      padding-bottom: 10px;
      border-bottom: 1px solid rgba(255,255,255,0.07);
    }
    .status-row:last-child {
      border-bottom: 0;
      padding-bottom: 0;
    }
    .status-label {
      color: var(--muted);
    }
    .call-state {
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0;
    }
    .call-state.ringing { color: var(--accent); }
    .call-state.idle { color: var(--muted); }
    code {
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 100%%;
    }
    .toast {
      min-height: 20px;
      color: var(--muted);
      font-size: 14px;
    }
    .toast.error {
      color: var(--danger);
    }
    .empty-note {
      padding: 12px 14px;
      border-radius: 8px;
      border: 1px dashed rgba(255,255,255,0.16);
      color: var(--muted);
    }
    @media (max-width: 940px) {
      .layout {
        grid-template-columns: 1fr;
      }
      .action-row {
        flex-direction: column;
      }
      .action-button {
        width: 100%%;
      }
    }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <div class="eyebrow">Bridge VTO Intercom</div>
      <h1>%s</h1>
      <p class="subtle">This page combines low-latency WebRTC playback, live call-state refresh, answer and hangup controls, and door actions for a Dahua VTO. It can also send browser microphone audio up to the bridge session and, when configured, export that incoming RTP to external bridge-side targets. That uplink is still not directly connected through to VTO talkback.</p>
    </section>
    <section class="layout">
      <article class="panel">
        <div class="media-frame">
          <video id="player" autoplay playsinline controls></video>
          <div class="status-pill" id="webrtc-status">Negotiating media...</div>
        </div>
        <div class="panel-body">
          <div>
            <p class="section-title">Profiles</p>
            <div class="profile-links">%s</div>
          </div>
            <div class="actions">
              <p class="section-title">Actions</p>
              <div class="action-row">
                %s
                <button id="hangup-button" class="action-button danger" type="button">Hang Up Call</button>
                <button id="mic-button" class="action-button" type="button">Enable Microphone</button>
                <button id="reset-button" class="action-button secondary" type="button">Reset Bridge Session</button>
                %s
              </div>
              <div class="action-row">%s</div>
              <div id="action-toast" class="toast"></div>
          </div>
        </div>
      </article>
      <aside class="panel">
        <div class="panel-body">
          <p class="section-title">Call Session</p>
            <div class="status-grid">
              <div class="status-row"><span class="status-label">Call State</span><span id="call-state" class="call-state idle">unknown</span></div>
              <div class="status-row"><span class="status-label">Bridge Session</span><span id="bridge-session">inactive</span></div>
              <div class="status-row"><span class="status-label">Last Source</span><span id="last-source">unknown</span></div>
              <div class="status-row"><span class="status-label">Last Ring</span><span id="last-ring">unknown</span></div>
              <div class="status-row"><span class="status-label">Call Started</span><span id="last-started">unknown</span></div>
              <div class="status-row"><span class="status-label">Call Ended</span><span id="last-ended">unknown</span></div>
              <div class="status-row"><span class="status-label">Duration</span><span id="duration">unknown</span></div>
              <div class="status-row"><span class="status-label">Microphone Uplink</span><span id="mic-state">inactive</span></div>
              <div class="status-row"><span class="status-label">Forwarded RTP Packets</span><span id="forwarded-packets">0</span></div>
              <div class="status-row"><span class="status-label">External RTP Targets</span><span>%s</span></div>
              <div class="status-row"><span class="status-label">ICE</span><span>%s</span></div>
              <div class="status-row"><span class="status-label">Profile</span><span>%s</span></div>
            <div class="status-row"><span class="status-label">Video</span><span>%s</span></div>
            <div class="status-row"><span class="status-label">Audio</span><span>%s</span></div>
            <div class="status-row"><span class="status-label">Snapshot</span><code>%s</code></div>
          </div>
        </div>
      </aside>
    </section>
  </main>
  <script>
    bridgePreparePage();
    const video = document.getElementById('player');
    const webrtcStatus = document.getElementById('webrtc-status');
    const actionToast = document.getElementById('action-toast');
    const answerButton = document.getElementById('answer-button');
    const hangupButton = document.getElementById('hangup-button');
    const micButton = document.getElementById('mic-button');
    const resetButton = document.getElementById('reset-button');
    const lockButtons = Array.from(document.querySelectorAll('[data-lock-index]'));
    const offerURL = %q;
    const deviceURL = %q;
    const intercomStatusURL = %q;
    const intercomResetURL = %q;
    const answerURL = %q;
    const hangupURL = %q;
    const uplinkEnableURL = %q;
    const uplinkDisableURL = %q;
    const lockURLBase = %q;
    const iceServers = %s;
    let peer = null;
    let reconnectTimer = null;
    let reconnectAttempts = 0;
    let micStream = null;
    let micEnabled = false;
    const exportButton = document.getElementById('export-button');

    function setToast(message, isError) {
      actionToast.textContent = message;
      actionToast.className = isError ? 'toast error' : 'toast';
    }

    function formatValue(value, suffix = '') {
      if (value === null || value === undefined || value === '') {
        return 'unknown';
      }
      return String(value) + suffix;
    }

    function clearReconnectTimer() {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    }

    function closePeer() {
      if (!peer) {
        return;
      }
      try {
        peer.ontrack = null;
        peer.onconnectionstatechange = null;
        peer.close();
      } catch (_) {}
      peer = null;
    }

    function reconnectDelayMilliseconds() {
      return Math.min(1000 * Math.pow(2, Math.min(reconnectAttempts, 4)), 10000);
    }

    function scheduleReconnect(reason) {
      if (reconnectTimer) {
        return;
      }
      reconnectAttempts += 1;
      const delay = reconnectDelayMilliseconds();
      webrtcStatus.textContent = 'Reconnecting in ' + Math.max(1, Math.round(delay / 1000)) + 's';
      if (reason) {
        console.warn('intercom reconnect scheduled:', reason);
      }
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connectMedia(micEnabled, true).catch(handleMediaError);
      }, delay);
    }

    function handleMediaError(error) {
      console.error(error);
      webrtcStatus.textContent = 'Retrying...';
      scheduleReconnect(error && error.message ? error.message : String(error));
    }

    async function postAction(url, button, successMessage) {
      const previous = button.textContent;
      button.disabled = true;
      setToast('');
      try {
        const response = await bridgeFetch(url, { method: 'POST' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        setToast(successMessage, false);
        await refreshState();
        await refreshIntercomStatus();
      } catch (error) {
        setToast(error.message || String(error), true);
      } finally {
        button.disabled = false;
        button.textContent = previous;
      }
    }

    async function refreshState() {
      try {
        const response = await bridgeFetch(deviceURL, { cache: 'no-store' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        const payload = await response.json();
        const state = payload && payload.states ? payload.states[%q] : null;
        const info = state && state.info ? state.info : {};
        const callState = formatValue(info.call_state).toLowerCase();
        const callStateEl = document.getElementById('call-state');
        callStateEl.textContent = formatValue(info.call_state);
        callStateEl.className = 'call-state ' + (callState === 'ringing' ? 'ringing' : 'idle');
        document.getElementById('last-source').textContent = formatValue(info.last_call_source);
        document.getElementById('last-ring').textContent = formatValue(info.last_ring_at);
        document.getElementById('last-started').textContent = formatValue(info.last_call_started_at);
        document.getElementById('last-ended').textContent = formatValue(info.last_call_ended_at);
        document.getElementById('duration').textContent = formatValue(info.last_call_duration_seconds, info.last_call_duration_seconds ? ' s' : '');
      } catch (error) {
        setToast('State refresh failed: ' + (error.message || String(error)), true);
      }
    }

    async function refreshIntercomStatus() {
      try {
        const response = await bridgeFetch(intercomStatusURL, { cache: 'no-store' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        const status = await response.json();
        const sessionCount = Number(status.session_count || 0);
        const forwardedPackets = Number(status.uplink_forwarded_packets || 0);
        const uplinkActive = Boolean(status.uplink_active);
        const externalUplinkEnabled = Boolean(status.external_uplink_enabled);
        document.getElementById('bridge-session').textContent = sessionCount > 0 ? ('active (' + sessionCount + ')') : 'inactive';
        document.getElementById('forwarded-packets').textContent = String(forwardedPackets);
        if (exportButton) {
          exportButton.textContent = externalUplinkEnabled ? 'Disable RTP Export' : 'Enable RTP Export';
        }
        if (uplinkActive) {
          document.getElementById('mic-state').textContent = 'active in bridge';
        } else if (micEnabled) {
          document.getElementById('mic-state').textContent = 'browser armed';
        } else {
          document.getElementById('mic-state').textContent = 'inactive';
        }
      } catch (error) {
        setToast('Intercom status refresh failed: ' + (error.message || String(error)), true);
      }
    }

    async function resetBridgeSession() {
      resetButton.disabled = true;
      setToast('');
      try {
        const response = await bridgeFetch(intercomResetURL, { method: 'POST' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        clearReconnectTimer();
        closePeer();
        await refreshIntercomStatus();
        await connectMedia(micEnabled, true);
        setToast('Bridge media session reset.', false);
      } catch (error) {
        setToast('Bridge session reset failed: ' + (error.message || String(error)), true);
      } finally {
        resetButton.disabled = false;
      }
    }

    async function toggleExport() {
      if (!exportButton) {
        return;
      }
      exportButton.disabled = true;
      setToast('');
      try {
        const currentlyEnabled = exportButton.textContent.indexOf('Disable') === 0;
        const response = await bridgeFetch(currentlyEnabled ? uplinkDisableURL : uplinkEnableURL, { method: 'POST' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        await refreshIntercomStatus();
        setToast(currentlyEnabled ? 'External RTP export disabled.' : 'External RTP export enabled.', false);
      } catch (error) {
        setToast('External RTP export update failed: ' + (error.message || String(error)), true);
      } finally {
        exportButton.disabled = false;
      }
    }

    async function connectMedia(withMicrophone, isReconnect) {
      clearReconnectTimer();
      closePeer();
      webrtcStatus.textContent = isReconnect ? 'Reconnecting...' : 'Negotiating media...';

      const pc = new RTCPeerConnection({ iceServers });
      peer = pc;
      const stream = new MediaStream();
      video.srcObject = stream;

      pc.addTransceiver('video', { direction: 'recvonly' });
      pc.addTransceiver('audio', { direction: 'recvonly' });
      if (withMicrophone) {
        if (!micStream) {
          micStream = await navigator.mediaDevices.getUserMedia({ audio: true });
        }
        for (const track of micStream.getAudioTracks()) {
          pc.addTrack(track, micStream);
        }
      }
      pc.ontrack = event => {
        if (peer !== pc) {
          return;
        }
        stream.addTrack(event.track);
        reconnectAttempts = 0;
        webrtcStatus.textContent = 'Connected';
      };
      pc.onconnectionstatechange = () => {
        if (peer !== pc || !pc.connectionState) {
          return;
        }
        switch (pc.connectionState) {
        case 'connected':
          reconnectAttempts = 0;
          webrtcStatus.textContent = 'Connected';
          return;
        case 'new':
          webrtcStatus.textContent = 'Negotiating media...';
          return;
        case 'connecting':
          webrtcStatus.textContent = 'Connecting...';
          return;
        case 'disconnected':
        case 'failed':
          webrtcStatus.textContent = pc.connectionState;
          scheduleReconnect('connection ' + pc.connectionState);
          return;
        case 'closed':
          if (peer === pc) {
            scheduleReconnect('connection closed');
          }
          return;
        default:
          webrtcStatus.textContent = pc.connectionState;
        }
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      if (pc.iceGatheringState !== 'complete') {
        await new Promise(resolve => {
          const onChange = () => {
            if (pc.iceGatheringState === 'complete') {
              pc.removeEventListener('icegatheringstatechange', onChange);
              resolve();
            }
          };
          pc.addEventListener('icegatheringstatechange', onChange);
        });
      }

      const response = await bridgeFetch(offerURL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(pc.localDescription),
      });
      if (!response.ok) {
        closePeer();
        throw new Error(await response.text());
      }
      const answer = await response.json();
      await pc.setRemoteDescription(answer);
      video.play().catch(() => {
        webrtcStatus.textContent = 'Ready - press play if autoplay was blocked';
      });
      if (webrtcStatus.textContent === 'Connected') {
        return;
      }
      webrtcStatus.textContent = 'Waiting for media...';
    }

    async function toggleMicrophone() {
      const nextEnabled = !micEnabled;
      micButton.disabled = true;
      setToast('');
      try {
        if (!nextEnabled && micStream) {
          for (const track of micStream.getTracks()) {
            track.stop();
          }
          micStream = null;
        }
        await connectMedia(nextEnabled, true);
        micEnabled = nextEnabled;
        micButton.textContent = micEnabled ? 'Disable Microphone' : 'Enable Microphone';
        document.getElementById('mic-state').textContent = micEnabled ? 'browser armed' : 'inactive';
        await refreshIntercomStatus();
        if (micEnabled) {
          setToast('Browser microphone uplink is now connected to the bridge session.', false);
        } else {
          setToast('Browser microphone uplink is now disconnected.', false);
        }
      } catch (error) {
        if (!nextEnabled) {
          micStream = null;
        }
        setToast('Microphone setup failed: ' + (error.message || String(error)), true);
      } finally {
        micButton.disabled = false;
      }
    }

    if (answerButton) {
      answerButton.addEventListener('click', () => {
        postAction(answerURL, answerButton, 'Call answer requested.');
      });
    }
    hangupButton.addEventListener('click', () => {
      postAction(hangupURL, hangupButton, 'Call hangup requested.');
    });
    micButton.addEventListener('click', () => {
      toggleMicrophone();
    });
    resetButton.addEventListener('click', () => {
      resetBridgeSession();
    });
    if (exportButton) {
      exportButton.addEventListener('click', () => {
        toggleExport();
      });
    }
    for (const button of lockButtons) {
      button.addEventListener('click', () => {
        const index = button.getAttribute('data-lock-index');
        postAction(lockURLBase + '/' + index + '/unlock', button, 'Door action sent.');
      });
    }

    window.addEventListener('beforeunload', () => {
      clearReconnectTimer();
      closePeer();
    });
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
        return;
      }
      if (!peer || peer.connectionState === 'failed' || peer.connectionState === 'disconnected' || peer.connectionState === 'closed') {
        scheduleReconnect('page became visible');
      }
    });

    refreshState();
    refreshIntercomStatus();
    setInterval(refreshState, 2000);
    setInterval(refreshIntercomStatus, 2000);
    connectMedia(false, false).catch(error => {
      setToast('Media negotiation failed: ' + (error.message || String(error)), true);
      handleMediaError(error);
    });
  </script>
</body>
</html>`,
		title,
		htmlEscape(entry.Name),
		profileLinks,
		boolHTMLButton(showAnswerControl, `<button id="answer-button" class="action-button" type="button">Answer Call</button>`),
		boolHTMLButton(showExternalUplinkControl, `<button id="export-button" class="action-button secondary" type="button">Disable RTP Export</button>`),
		strings.Join(lockButtons, ""),
		htmlEscape(externalUplinkTargetsLabel),
		htmlEscape(iceModeLabel),
		htmlEscape(profileName),
		htmlEscape(firstNonEmpty(entry.MainCodec+" "+entry.MainResolution, "unknown")),
		htmlEscape(audioLabel),
		htmlEscape(entry.SnapshotURL),
		offerURL,
		deviceURL,
		intercomStatusURL,
		intercomResetURL,
		answerURL,
		hangupURL,
		uplinkEnableURL,
		uplinkDisableURL,
		"/api/v1/vto/"+url.PathEscape(entry.ID)+"/locks",
		iceServersJSON,
		entry.ID,
	)
}

func marshalWebRTCICEServers(iceServers []mediaapi.WebRTCICEServer) string {
	if len(iceServers) == 0 {
		return "[]"
	}
	body, err := json.Marshal(iceServers)
	if err != nil {
		return "[]"
	}
	return string(body)
}

func buildPreviewProfileLinks(entry streams.Entry, selectedProfile string) string {
	ordered := []string{"quality", "default", "stable", "substream"}
	links := make([]string, 0, len(entry.Profiles))
	seen := map[string]struct{}{}

	appendLink := func(profileName string) {
		profile, ok := entry.Profiles[profileName]
		if !ok {
			return
		}
		if _, ok := seen[profileName]; ok {
			return
		}
		seen[profileName] = struct{}{}

		className := ""
		if profileName == selectedProfile {
			className = ` class="active"`
		}
		label := profile.Name
		if label == "" {
			label = profileName
		}
		links = append(links, fmt.Sprintf(
			`<a%s href="/api/v1/media/preview/%s?profile=%s">%s</a>`,
			className,
			url.PathEscape(entry.ID),
			url.QueryEscape(profileName),
			htmlEscape(label),
		))
	}

	for _, profileName := range ordered {
		appendLink(profileName)
	}
	for profileName := range entry.Profiles {
		appendLink(profileName)
	}

	return strings.Join(links, "")
}

func buildIntercomProfileLinks(entry streams.Entry, selectedProfile string) string {
	ordered := []string{"quality", "default", "stable", "substream"}
	links := make([]string, 0, len(entry.Profiles))
	seen := map[string]struct{}{}

	appendLink := func(profileName string) {
		profile, ok := entry.Profiles[profileName]
		if !ok {
			return
		}
		if _, ok := seen[profileName]; ok {
			return
		}
		seen[profileName] = struct{}{}

		className := ""
		if profileName == selectedProfile {
			className = ` class="active"`
		}
		label := profile.Name
		if label == "" {
			label = profileName
		}
		links = append(links, fmt.Sprintf(
			`<a%s href="/api/v1/vto/%s/intercom?profile=%s">%s</a>`,
			className,
			url.PathEscape(entry.ID),
			url.QueryEscape(profileName),
			htmlEscape(label),
		))
	}

	for _, profileName := range ordered {
		appendLink(profileName)
	}
	for profileName := range entry.Profiles {
		appendLink(profileName)
	}

	return strings.Join(links, "")
}
