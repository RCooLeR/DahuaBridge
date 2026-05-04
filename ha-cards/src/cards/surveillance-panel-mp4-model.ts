import type {BridgeRecordingClipModel} from "../domain/archive";
import type {CameraViewModel} from "../domain/model";

export const MP4_PAGE_SIZE = 20;

export interface SelectedBridgeRecordingPlaybackState {
    sourceDeviceId: string;
    recording: BridgeRecordingClipModel;
}

export function bridgeRecordingDownloadBusyKey(recording: { id: string }): string {
    return `bridge-recording-download:${recording.id}`;
}

export function selectedBridgeRecordingPlaybackForCamera(
    playback: SelectedBridgeRecordingPlaybackState | null,
    camera: CameraViewModel,
): SelectedBridgeRecordingPlaybackState | null {
    if (!playback) {
        return null;
    }
    return playback.sourceDeviceId === camera.deviceId ? playback : null;
}
