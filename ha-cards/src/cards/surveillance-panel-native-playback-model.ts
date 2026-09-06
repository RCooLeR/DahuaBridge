import type {NvrArchiveRecordingModel} from "../domain/archive";
import type {CameraStreamProfileViewModel, CameraViewModel} from "../domain/model";
import {resolveSelectedCameraStreamProfile} from "./surveillance-panel-media";
import {isBridgeRtspRelayUrl} from "../ha/bridge-url";

export interface SelectedNativePlaybackState {
    sourceDeviceId: string;
    cameraEntityId: string | null;
    streamSource: string;
    fallbackStreamSource?: string | null;
    startTime: string;
    endTime: string;
    seekTime: string;
    profileKey: string | null;
}

export function selectedNativePlaybackForCamera(
    playback: SelectedNativePlaybackState | null,
    camera: CameraViewModel,
): SelectedNativePlaybackState | null {
    if (!playback) {
        return null;
    }
    return playback.sourceDeviceId === camera.deviceId ? playback : null;
}

export function createSelectedNativePlaybackState(
    camera: CameraViewModel,
    streamSource: string,
    startTime: Date | string,
    endTime: Date | string,
    seekTime: Date | string,
    profileKey: string | null,
    fallbackStreamSource: string | null = null,
): SelectedNativePlaybackState {
    return {
        sourceDeviceId: camera.deviceId,
        cameraEntityId: camera.cameraEntity?.entity_id?.trim() || camera.cameraEntityId?.trim() || null,
        streamSource,
        fallbackStreamSource,
        startTime: toIsoString(startTime),
        endTime: toIsoString(endTime),
        seekTime: toIsoString(seekTime),
        profileKey,
    };
}

export function resolveArchivePlaybackProfile(
    camera: CameraViewModel,
    selectedProfileKey: string | null,
): CameraStreamProfileViewModel | null {
    const selectedProfile = resolveSelectedCameraStreamProfile(camera, selectedProfileKey);
    return (
        (selectedProfile?.streamUrl ? selectedProfile : null) ??
        camera.stream.profiles.find(
            (profile) => profile.key === "quality" && Boolean(profile.streamUrl),
        ) ??
        camera.stream.profiles.find(
            (profile) => profile.key === "stable" && Boolean(profile.streamUrl),
        ) ??
        camera.stream.profiles.find((profile) => profile.subtype === 0 && Boolean(profile.streamUrl)) ??
        camera.stream.profiles.find((profile) => Boolean(profile.streamUrl)) ??
        selectedProfile ??
        camera.stream.profiles[0] ??
        null
    );
}

export function resolveMainArchivePlaybackProfile(
    camera: CameraViewModel,
    selectedProfileKey: string | null,
): CameraStreamProfileViewModel | null {
    return (
        camera.stream.profiles.find((profile) => profile.subtype === 0 && Boolean(profile.streamUrl)) ??
        camera.stream.profiles.find(
            (profile) => profile.key === "quality" && Boolean(profile.streamUrl),
        ) ??
        resolveArchivePlaybackProfile(camera, selectedProfileKey)
    );
}

export function archivePlaybackStreamCandidates(
    camera: CameraViewModel,
    profile: CameraStreamProfileViewModel | null,
    recording?: NvrArchiveRecordingModel,
): string[] {
    if (isBridgeRtspRelayUrl(profile?.streamUrl) || isBridgeRtspRelayUrl(camera.stream.source)) {
        // Current bridges own upstream URLs and expose a stable live relay.
        // Archive uses bridge playback sessions; only older catalogs need RTSP derivation.
        return [];
    }
    const profiles = camera.cameraEntity?.attributes.bridge_profiles;
    const rawProfile = profiles && typeof profiles === "object" && !Array.isArray(profiles)
        ? (profiles as Record<string, unknown>)[profile?.key ?? ""]
        : null;
    const raw = rawProfile && typeof rawProfile === "object" && !Array.isArray(rawProfile)
        ? rawProfile as Record<string, unknown>
        : {};
    const recordings = profile?.key === "stable"
        ? [recording?.rtspSubUrl, recording?.rtspMainUrl]
        : [recording?.rtspMainUrl, recording?.rtspSubUrl];
    const candidates: unknown[] = [raw.recorder_stream_url, profile?.recorderStreamUrl, ...recordings];
    // Legacy catalogs have only stream_url. It is a recorder URL only when
    // live input is NVR; direct-camera URLs must never become archive inputs.
    if (camera.stream.liveSource?.source !== "camera") {
        candidates.push(
            raw.stream_url,
            raw.streamUrl,
            profile?.streamUrl,
            camera.cameraEntity?.attributes.stream_source,
            camera.stream.source,
            camera.stream.onvifStreamUrl,
        );
    }
    return [...new Set(candidates.filter((value): value is string =>
        typeof value === "string" && Boolean(value.trim()),
    ).map((value) => value.trim()))];
}

export function isArchiveEventRecording(recording: NvrArchiveRecordingModel): boolean {
    const recordKind = recording.recordKind?.trim().toLowerCase() ?? "";
    if (recordKind === "event" || recordKind === "smd_ivs" || recordKind === "smd-ivs") {
        return true;
    }
    const source = recording.source?.trim().toLowerCase() ?? "";
    const recordingType = recording.type?.trim().toLowerCase() ?? "";
    return (
        source === "nvr_event" ||
        source === "smd_ivs" ||
        source === "smd-ivs" ||
        recordingType === "event" ||
        recordingType.startsWith("event.")
    );
}

export function nativePlaybackMatchesRecording(
    playback: SelectedNativePlaybackState | null,
    recording: NvrArchiveRecordingModel,
): boolean {
    return (
        playback?.startTime === recording.startTime &&
        playback.endTime === recording.endTime
    );
}

function toIsoString(value: Date | string): string {
    return typeof value === "string" ? value : value.toISOString();
}
