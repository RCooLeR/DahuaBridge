import { html, nothing, type TemplateResult } from "lit";
import { repeat } from "lit/directives/repeat.js";

import {
  displayCameraLabel,
  type CameraViewModel,
} from "../domain/model";
import type { DetailTab } from "./surveillance-panel-state";
import {
  audioAuthorityLabel,
  audioAuthorityTone,
  audioSemanticLabel,
  streamProfileLabel,
  streamSourceLabel,
} from "./surveillance-panel-inspector-shared";
import { renderSegmentButton } from "./surveillance-panel-primitives";

export function renderCameraInspector(
  camera: CameraViewModel,
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
      ${renderSegmentButton("events", "Events", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("recordings", "Recordings", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("mp4", "MP4", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
      ${renderSegmentButton("settings", "Settings", detailTab, (tab) =>
        onSelectDetailTab(tab as DetailTab),
      )}
    </div>
    <div class="detail-main">
      ${detailTab === "events" ? eventContent : nothing}
      ${detailTab === "recordings" ? archiveContent : nothing}
      ${detailTab === "mp4" ? mp4Content : nothing}
      ${detailTab === "settings"
        ? html`
            ${renderCameraStreamStatus(camera)}
            ${renderCameraStreamProfiles(camera)}
            ${renderCameraAudioBridge(camera)}
            ${renderCameraBridgeAdapter(camera)}
          `
        : nothing}
    </div>
  `;
}

function renderCameraStreamStatus(camera: CameraViewModel): TemplateResult {
  return html`
    <div class="panel">
      <div class="panel-title">Stream Status</div>
      <div class="chip-row">
        <span class="badge ${camera.stream.available ? "success" : "critical"}">
          ${camera.stream.available ? "Live stream ready" : "Live stream unavailable"}
        </span>
        <span class="badge info">${streamProfileLabel(camera.stream.profile)}</span>
        <span class="badge">${camera.stream.resolution}</span>
        <span class="badge">${camera.stream.codec}</span>
        <span class="badge">${camera.stream.frameRate}</span>
        <span class="badge">${camera.stream.bitrate}</span>
        <span class="badge">${camera.stream.audioCodec}</span>
        ${camera.stream.recommendedProfile
          ? html`<span class="badge success">Recommended ${streamProfileLabel(camera.stream.recommendedProfile)}</span>`
          : nothing}
        ${camera.stream.onvifStreamUrl ? html`<span class="badge info">ONVIF stream ready</span>` : nothing}
        ${camera.stream.onvifSnapshotUrl ? html`<span class="badge info">ONVIF snapshot ready</span>` : nothing}
      </div>
      <div class="detail-inline-meta">
        ${camera.stream.preferredVideoProfile
          ? html`<span class="muted">Preferred profile: ${streamProfileLabel(camera.stream.preferredVideoProfile)}</span>`
          : nothing}
        ${camera.stream.preferredVideoSource
          ? html`<span class="muted">Preferred source: ${streamSourceLabel(camera.stream.preferredVideoSource)}</span>`
          : nothing}
        <span class="muted">
          ${camera.stream.source
            ? "Primary stream route is exposed."
            : "No primary stream route is exposed."}
        </span>
      </div>
    </div>
  `;
}

function renderCameraStreamProfiles(
  camera: CameraViewModel,
): TemplateResult | typeof nothing {
  if (camera.stream.profiles.length === 0) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">Stream Profiles</div>
      <div class="compact-list">
        ${repeat(
          camera.stream.profiles,
          (profile) => profile.key,
          (profile) => html`
            <div class="compact-card stream-profile-card">
              <div class="panel-title compact-card-head">
                <span class="sidebar-label">${profile.name}</span>
                ${profile.recommended
                  ? html`<span class="badge success">Recommended</span>`
                  : nothing}
              </div>
              <div class="chip-row">
                ${profile.subtype !== null ? html`<span class="badge">Subtype ${profile.subtype}</span>` : nothing}
                ${profile.resolution ? html`<span class="badge info">${profile.resolution}</span>` : nothing}
                ${profile.frameRate !== null ? html`<span class="badge">${profile.frameRate}</span>` : nothing}
                ${profile.rtspTransport ? html`<span class="badge">${profile.rtspTransport}</span>` : nothing}
              </div>
              <div class="chip-row">
                ${profile.streamUrl ? html`<span class="badge info">RTSP</span>` : nothing}
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
): TemplateResult | typeof nothing {
  if (!camera.audioMuteSupported && !camera.audioControlAuthority) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">Audio Bridge</div>
      <div class="chip-row">
        <span class="badge ${camera.audioMuteSupported ? "success" : "warning"}">
          ${camera.audioMuteSupported ? "Browser mute supported" : "Browser mute unavailable"}
        </span>
        ${camera.audioControlAuthority
          ? html`
              <span class="badge ${audioAuthorityTone(camera.audioControlAuthority)}">
                ${audioAuthorityLabel(camera.audioControlAuthority)}
              </span>
            `
          : nothing}
        ${camera.audioControlSemantic
          ? html`<span class="badge info">${audioSemanticLabel(camera.audioControlSemantic)}</span>`
          : nothing}
      </div>
    </div>
  `;
}

function renderCameraBridgeAdapter(
  camera: CameraViewModel,
): TemplateResult | typeof nothing {
  if (camera.bridgeBaseUrl && camera.eventsUrl) {
    return nothing;
  }

  return html`
    <div class="panel">
      <div class="panel-title">Bridge Adapter</div>
      <div class="chip-row">
        <span class="badge ${camera.bridgeBaseUrl ? "success" : "warning"}">
          ${camera.bridgeBaseUrl ? "Bridge base URL available" : "Bridge base URL unavailable"}
        </span>
        <span class="badge ${camera.eventsUrl ? "success" : "warning"}">
          ${camera.eventsUrl ? "Event route available" : "Event route unavailable"}
        </span>
      </div>
      <div class="muted">
        Bridge diagnostics are only shown here when a required route is missing.
      </div>
    </div>
  `;
}
