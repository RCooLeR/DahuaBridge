import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import {
  displayCameraLabel,
  type CameraViewModel,
} from "../domain/model";
import type { DetailTab } from "./surveillance-panel-state";
import {
  streamProfileLabel,
  streamSourceLabel,
} from "./surveillance-panel-inspector-shared";
import { renderSegmentButton } from "./surveillance-panel-primitives";
import type { Localizer } from "../localization";

export function renderCameraInspector(
  camera: CameraViewModel,
  t: Localizer,
  detailTab: DetailTab,
  eventContent: TemplateResult | typeof nothing,
  archiveContent: TemplateResult | typeof nothing,
  mp4Content: TemplateResult | typeof nothing,
  onSelectDetailTab: (tab: DetailTab) => void,
): TemplateResult {
  return html`
    <div class="detail-header">
      <div class="detail-title">${displayCameraLabel(camera)}</div>
      <div class="muted">${camera.roomLabel}</div>
    </div>
    <div class="detail-tabs">
      ${renderSegmentButton("events", t("tab.events"), detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("recordings", t("tab.recordings"), detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("mp4", "MP4", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("settings", t("tab.settings"), detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
    </div>
    <div class="detail-main">
      ${detailTab === "events" ? eventContent : nothing}
      ${detailTab === "recordings" ? archiveContent : nothing}
      ${detailTab === "mp4" ? mp4Content : nothing}
      ${detailTab === "settings"
        ? html`
            ${renderCameraStreamStatus(camera, t)}
            ${renderCameraStreamProfiles(camera, t)}
            ${renderCameraAudioBridge(camera, t)}
            ${renderCameraBridgeAdapter(camera, t)}
          `
        : nothing}
    </div>
  `;
}

function renderCameraStreamStatus(camera: CameraViewModel, t: Localizer): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">${t("inspector.streamStatus")}</div>
      <div class="chip-row">
        <span class="badge ${camera.stream.available ? "success" : "critical"}">
          ${camera.stream.available ? t("inspector.liveReady") : t("inspector.liveUnavailable")}
        </span>
        <span class="badge info">${streamProfileLabel(camera.stream.profile, t)}</span>
        <span class="badge">${camera.stream.resolution}</span>
        <span class="badge">${camera.stream.codec}</span>
        <span class="badge">${camera.stream.frameRate}</span>
        <span class="badge">${camera.stream.bitrate}</span>
        <span class="badge">${camera.stream.audioCodec}</span>
        ${camera.stream.recommendedProfile
          ? html`<span class="badge success">${t("inspector.recommendedProfile", {profile: streamProfileLabel(camera.stream.recommendedProfile, t)})}</span>`
          : nothing}
        ${camera.stream.onvifStreamUrl ? html`<span class="badge info">${t("inspector.onvifStreamReady")}</span>` : nothing}
        ${camera.stream.onvifSnapshotUrl ? html`<span class="badge info">${t("inspector.onvifSnapshotReady")}</span>` : nothing}
      </div>
      <div class="detail-inline-meta">
        ${camera.stream.preferredVideoProfile
          ? html`<span class="muted">${t("inspector.preferredProfile", {profile: streamProfileLabel(camera.stream.preferredVideoProfile, t)})}</span>`
          : nothing}
        ${camera.stream.preferredVideoSource
          ? html`<span class="muted">${t("inspector.preferredSource", {source: streamSourceLabel(camera.stream.preferredVideoSource, t)})}</span>`
          : nothing}
        <span class="muted">
          ${camera.stream.source
            ? t("inspector.primaryRouteExposed")
            : t("inspector.primaryRouteMissing")}
        </span>
      </div>
    </div>
  `;
}

function renderCameraStreamProfiles(
  camera: CameraViewModel,
  t: Localizer,
): TemplateResult | typeof nothing {
  if (camera.stream.profiles.length === 0) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">${t("inspector.streamProfiles")}</div>
      <div class="compact-list">
        ${repeat(
          camera.stream.profiles,
          (profile) => profile.key,
          (profile) => html`
            <div class="compact-card stream-profile-card">
              <div class="panel-title compact-card-head">
                <span class="sidebar-label">${profile.name}</span>
                ${profile.recommended
                  ? html`<span class="badge success">${t("inspector.recommended")}</span>`
                  : nothing}
              </div>
              <div class="chip-row">
                ${profile.subtype !== null ? html`<span class="badge">${t("inspector.subtype", {subtype: profile.subtype})}</span>` : nothing}
                ${profile.resolution ? html`<span class="badge info">${profile.resolution}</span>` : nothing}
                ${profile.frameRate !== null ? html`<span class="badge">${profile.frameRate}</span>` : nothing}
                ${profile.rtspTransport ? html`<span class="badge">${profile.rtspTransport}</span>` : nothing}
              </div>
              <div class="chip-row">
                ${profile.streamUrl ? html`<span class="badge info">RTSP</span>` : nothing}
                ${profile.localDashUrl ? html`<span class="badge success">DASH</span>` : nothing}
                ${profile.localHlsUrl ? html`<span class="badge success">HLS</span>` : nothing}
                ${profile.localMjpegUrl ? html`<span class="badge">MJPEG</span>` : nothing}
              </div>
            </div>
          `,
        )}
      </div>
    </div>
  `;
}

function renderCameraAudioBridge(
  camera: CameraViewModel,
  t: Localizer,
): TemplateResult | typeof nothing {
  if (!camera.audioMuteSupported && !camera.audioCodec.trim()) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">${t("inspector.browserStreamAudio")}</div>
      <div class="chip-row">
        <span class="badge ${camera.audioMuteSupported ? "success" : "warning"}">
          ${camera.audioMuteSupported ? t("inspector.playerAudioAvailable") : t("inspector.playerAudioUnavailable")}
        </span>
        ${camera.audioCodec.trim()
          ? html`<span class="badge info">${camera.audioCodec}</span>`
          : nothing}
      </div>
    </div>
  `;
}

function renderCameraBridgeAdapter(
  camera: CameraViewModel,
  t: Localizer,
): TemplateResult | typeof nothing {
  if (camera.bridgeBaseUrl && camera.eventsUrl) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">${t("inspector.bridgeAdapter")}</div>
      <div class="chip-row">
        <span class="badge ${camera.bridgeBaseUrl ? "success" : "warning"}">
          ${camera.bridgeBaseUrl ? t("inspector.bridgeBaseAvailable") : t("inspector.bridgeBaseUnavailable")}
        </span>
        <span class="badge ${camera.eventsUrl ? "success" : "warning"}">
          ${camera.eventsUrl ? t("inspector.eventRouteAvailable") : t("inspector.eventRouteUnavailable")}
        </span>
      </div>
      <div class="muted">
        ${t("inspector.bridgeDiagnosticsHint")}
      </div>
    </div>
  `;
}
