import { z } from "zod";

import {
  createPlaybackSessionRequest,
  type NvrArchiveRecordingModel,
  type NvrPlaybackSeekRequestModel,
  type NvrPlaybackSessionModel,
  type NvrPlaybackSessionRequestModel,
} from "../domain/archive";
import { rewriteBridgeUrl } from "./bridge-url";
import { authenticatedBridgeResourceUrl } from "./bridge-resource";

const playbackProfileSchema = z.object({
  name: z.string().min(1),
  dash_url: z.string().optional().nullable(),
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
  const requestUrl = rewriteBridgeUrl(playbackUrl, browserBridgeUrl) ?? playbackUrl;
  const response = await fetch(requestUrl, {
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
      ...(request.filePath ? { file_path: request.filePath } : {}),
      ...(request.source ? { source: request.source } : {}),
      ...(request.type ? { type: request.type } : {}),
      ...(request.videoStream ? { video_stream: request.videoStream } : {}),
    }),
    signal,
  });

  if (!response.ok) {
    throw new Error(`Bridge playback request failed with status ${response.status}`);
  }

  return mapPlaybackSession(
    playbackSessionSchema.parse(await response.json()),
    browserBridgeUrl,
    playbackUrl,
  );
}

export async function seekPlaybackSession(
  sessionID: string,
  seekUrl: string,
  request: NvrPlaybackSeekRequestModel,
  browserBridgeUrl?: string | null,
  signal?: AbortSignal,
): Promise<NvrPlaybackSessionModel> {
  const endpoint = resolvePlaybackSeekUrl(seekUrl, sessionID);
  const requestUrl = rewriteBridgeUrl(endpoint, browserBridgeUrl) ?? endpoint;
  const response = await fetch(requestUrl, {
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

  return mapPlaybackSession(
    playbackSessionSchema.parse(await response.json()),
    browserBridgeUrl,
    endpoint,
  );
}

export function createPlaybackSessionFromRecording(
  recording: NvrArchiveRecordingModel,
): NvrPlaybackSessionRequestModel {
  return createPlaybackSessionRequest(
    recording.channel,
    recording.startTime,
    recording.endTime,
    recording.startTime,
    recording.filePath,
    recording.source,
    recording.type,
    recording.videoStream,
  );
}

export function resolvePlaybackLaunchUrl(session: NvrPlaybackSessionModel): string | null {
  const preferredProfile =
    session.profiles[session.recommendedProfile] ?? Object.values(session.profiles)[0] ?? null;
  if (!preferredProfile) {
    return null;
  }

  return preferredProfile.hlsUrl ?? preferredProfile.dashUrl ?? preferredProfile.mjpegUrl ?? null;
}

function mapPlaybackSession(
  payload: z.infer<typeof playbackSessionSchema>,
  browserBridgeUrl?: string | null,
  authenticatedRequestUrl?: string,
): NvrPlaybackSessionModel {
  const mediaUrl = (target: string | null | undefined): string | null =>
    authenticatedBridgeResourceUrl(target, authenticatedRequestUrl ?? browserBridgeUrl ?? "", browserBridgeUrl);
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
    snapshotUrl: mediaUrl(payload.snapshot_url),
    createdAt: payload.created_at ?? "",
    expiresAt: payload.expires_at ?? "",
    profiles: Object.fromEntries(
      Object.entries(payload.profiles).map(([key, profile]) => [
        key,
        {
          name: profile.name,
          dashUrl: mediaUrl(profile.dash_url),
          hlsUrl: mediaUrl(profile.hls_url),
          mjpegUrl: mediaUrl(profile.mjpeg_url),
          webrtcOfferUrl: mediaUrl(profile.webrtc_offer_url),
        },
      ]),
    ),
  };
}

function resolvePlaybackSeekUrl(seekUrl: string, sessionID: string): string {
  const encodedSessionID = encodeURIComponent(sessionID);
  return seekUrl
    .replace(/\{session_id\}/gi, encodedSessionID)
    .replace(/\{sessionId\}/g, encodedSessionID)
    .replace(/%7Bsession_id%7D/gi, encodedSessionID)
    .replace(/%7BsessionId%7D/g, encodedSessionID);
}
