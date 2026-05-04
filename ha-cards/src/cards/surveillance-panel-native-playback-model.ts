import type {NvrArchiveRecordingModel} from "../domain/archive";
import type {CameraStreamProfileViewModel, CameraViewModel} from "../domain/model";
import {resolveSelectedCameraStreamProfile} from "./surveillance-panel-media";

export interface SelectedNativePlaybackState {
    sourceDeviceId: string;
    streamSource: string;
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
): SelectedNativePlaybackState {
    return {
        sourceDeviceId: camera.deviceId,
        streamSource,
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
