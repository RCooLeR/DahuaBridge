import type {CameraViewModel} from "../domain/model";
import {
    availableCameraViewportSources,
    type CameraViewportSource,
    resolveSelectedCameraViewportSource,
    resolveSelectedCameraStreamProfile,
} from "./surveillance-panel-media";
import type {SelectedNativePlaybackState} from "./surveillance-panel-native-playback-model";

export interface SelectedCameraLiveStreamModel {
    selectedProfileKey: string | null;
    selectedSource: CameraViewportSource | null;
    availableSources: CameraViewportSource[];
}

export function selectedCameraLiveStreamModel(
    camera: CameraViewModel,
    selectedProfileKey: string | null,
    selectedSource: CameraViewportSource | null,
    nativePlayback: SelectedNativePlaybackState | null,
): SelectedCameraLiveStreamModel {
    const selectedLiveProfile = resolveSelectedCameraStreamProfile(camera, selectedProfileKey);
    const resolvedProfileKey = nativePlayback?.profileKey ?? selectedLiveProfile?.key ?? null;
    return {
        selectedProfileKey: resolvedProfileKey,
        selectedSource:
            nativePlayback !== null
                ? "mjpeg"
                : resolveSelectedCameraViewportSource(
                    camera,
                    selectedSource,
                    resolvedProfileKey,
                ),
        availableSources:
            nativePlayback !== null
                ? (["mjpeg"] satisfies CameraViewportSource[])
                : availableCameraViewportSources(camera, resolvedProfileKey),
    };
}
