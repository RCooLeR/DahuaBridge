import { z } from "zod";

import {
  createPlaybackSeekRequest,
  createPlaybackSessionRequest,
  type NvrArchiveRecordingModel,
  type NvrPlaybackSeekRequestModel,
  type NvrPlaybackSessionModel,
  type NvrPlaybackSessionRequestModel,
} from "../domain/archive";
import { rewriteBridgeUrl } from "./bridge-url";

const playbackProfileSchema = z.object({
  name: z.string().min(1),
  hls_url: z.string().optional().nullable(),
  mjpeg_url: z.string().optional().nullable(),
  webrtc_offer_url: z.string().optional().nullable(),
});

const playbackSessionSchema = z.object({
  id: z.string().min(1),
  stream_id: z.string().min(1),
  device_id: z.string().min(1),
  source_stream_id: z.string().optional().nullable(),
  name: z.string().min(1),
  channel: z.number().int(),
  start_time: z.string().min(1),
  end_time: z.string().min(1),
  seek_time: z.string().min(1),
  recommended_profile: z.string().min(1),
  snapshot_url: z.string().optional().nullable(),
  created_at: z.string().optional().nullable(),
  expires_at: z.string().optional().nullable(),
  profiles: z.record(z.string(), playbackProfileSchema),
});

export async function createPlaybackSession(
  playbackUrl: string,
  request: NvrPlaybackSessionRequestModel,
  browserBridgeUrl?: string | null,
  signal?: AbortSignal,
): Promise<NvrPlaybackSessionModel> {
  const response = await fetch(playbackUrl, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({
      channel: request.channel,
      start_time: request.startTime,
      end_time: request.endTime,
      ...(request.seekTime ? { seek_time: request.seekTime } : {}),
    }),
    signal,
  });

  if (!response.ok) {
    throw new Error(`Bridge playback request failed with status ${response.status}`);
  }

  const payload = playbackSessionSchema.parse(await response.json());
  return {
    id: payload.id,
    streamId: payload.stream_id,
    deviceId: payload.device_id,
    sourceStreamId: payload.source_stream_id ?? null,
    name: payload.name,
    channel: payload.channel,
    startTime: payload.start_time,
    endTime: payload.end_time,
    seekTime: payload.seek_time,
    recommendedProfile: payload.recommended_profile,
    snapshotUrl: rewriteBridgeUrl(payload.snapshot_url ?? null, browserBridgeUrl),
    createdAt: payload.created_at ?? "",
    expiresAt: payload.expires_at ?? "",
    profiles: Object.fromEntries(
      Object.entries(payload.profiles).map(([key, profile]) => [
        key,
        {
          name: profile.name,
          hlsUrl: rewriteBridgeUrl(profile.hls_url ?? null, browserBridgeUrl),
          mjpegUrl: rewriteBridgeUrl(profile.mjpeg_url ?? null, browserBridgeUrl),
          webrtcOfferUrl: rewriteBridgeUrl(profile.webrtc_offer_url ?? null, browserBridgeUrl),
        },
      ]),
    ),
  };
}

export async function seekPlaybackSession(
  sessionID: string,
  seekUrl: string,
  request: NvrPlaybackSeekRequestModel,
  browserBridgeUrl?: string | null,
  signal?: AbortSignal,
): Promise<NvrPlaybackSessionModel> {
  const response = await fetch(seekUrl.replace("{session_id}", encodeURIComponent(sessionID)), {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({
      seek_time: request.seekTime,
    }),
    signal,
  });

  if (!response.ok) {
    throw new Error(`Bridge playback seek request failed with status ${response.status}`);
  }

  const payload = playbackSessionSchema.parse(await response.json());
  return {
    id: payload.id,
    streamId: payload.stream_id,
    deviceId: payload.device_id,
    sourceStreamId: payload.source_stream_id ?? null,
    name: payload.name,
    channel: payload.channel,
    startTime: payload.start_time,
    endTime: payload.end_time,
    seekTime: payload.seek_time,
    recommendedProfile: payload.recommended_profile,
    snapshotUrl: rewriteBridgeUrl(payload.snapshot_url ?? null, browserBridgeUrl),
    createdAt: payload.created_at ?? "",
    expiresAt: payload.expires_at ?? "",
    profiles: Object.fromEntries(
      Object.entries(payload.profiles).map(([key, profile]) => [
        key,
        {
          name: profile.name,
          hlsUrl: rewriteBridgeUrl(profile.hls_url ?? null, browserBridgeUrl),
          mjpegUrl: rewriteBridgeUrl(profile.mjpeg_url ?? null, browserBridgeUrl),
          webrtcOfferUrl: rewriteBridgeUrl(profile.webrtc_offer_url ?? null, browserBridgeUrl),
        },
      ]),
    ),
  };
}

export function createPlaybackSessionFromRecording(
  recording: NvrArchiveRecordingModel,
): NvrPlaybackSessionRequestModel {
  return createPlaybackSessionRequest(
    recording.channel,
    recording.startTime,
    recording.endTime,
    recording.startTime,
  );
}

export function createPlaybackSeekRequestFromRecording(
  seekTime: string,
): NvrPlaybackSeekRequestModel {
  return createPlaybackSeekRequest(seekTime);
}

export function resolvePlaybackLaunchUrl(session: NvrPlaybackSessionModel): string | null {
  const preferredProfile =
    session.profiles[session.recommendedProfile] ?? Object.values(session.profiles)[0] ?? null;
  if (!preferredProfile) {
    return null;
  }

  return preferredProfile.hlsUrl ?? preferredProfile.mjpegUrl ?? null;
}
