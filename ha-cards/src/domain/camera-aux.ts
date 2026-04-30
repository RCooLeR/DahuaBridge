export type CameraAuxActionName = "start" | "stop" | "pulse";

export interface CameraAuxFeatureSource {
    key: string | null;
    label: string | null;
    group: string | null;
    kind: string | null;
    url: string | null;
    supported: boolean;
    parameterKey: string | null;
    parameterValue: string | null;
    actions: string[];
    active: boolean | null;
    currentText: string | null;
}

export interface BridgeAuxControlSource {
    url: string | null;
    supported: boolean;
    outputs: string[];
    features: string[];
}

export interface CameraAuxActionTargetModel {
    key: string;
    label: string;
    url: string | null;
    parameterKey: string;
    parameterValue: string;
    actions: CameraAuxActionName[];
    outputKey: string;
    preferredAction: CameraAuxActionName | null;
    active: boolean | null;
    currentText: string | null;
    toggleSupported: boolean;
}

export interface CameraAuxCapabilities {
    supported: boolean;
    url: string | null;
    outputs: string[];
    features: string[];
    targets: CameraAuxActionTargetModel[];
}

export function buildCameraAuxCapabilities(
    control: BridgeAuxControlSource | null,
    features: readonly CameraAuxFeatureSource[],
): CameraAuxCapabilities | null {
    if (!control && features.length === 0) {
        return null;
    }

    const filteredTargets = features
        .filter((feature) => isAuxFeature(feature))
        .map(buildAuxActionTarget)
        .filter((target): target is CameraAuxActionTargetModel => target !== null)
        .sort((left, right) => left.label.localeCompare(right.label));

    return {
        supported: control?.supported === true || filteredTargets.length > 0,
        url: control?.url ?? filteredTargets[0]?.url ?? null,
        outputs: control?.outputs ?? [],
        features: control?.features ?? [],
        targets: filteredTargets,
    };
}

function isAuxFeature(feature: CameraAuxFeatureSource): boolean {
    if (feature.supported !== true) {
        return false;
    }
    if (feature.group === "deterrence") {
        return true;
    }
    return feature.parameterKey === "output";
}

function buildAuxActionTarget(
    feature: CameraAuxFeatureSource,
): CameraAuxActionTargetModel | null {
    const key = stringOrNull(feature.key);
    const parameterKey = stringOrNull(feature.parameterKey);
    const parameterValue = stringOrNull(feature.parameterValue);
    if (!key || parameterKey !== "output" || !parameterValue) {
        return null;
    }

    const actions = feature.actions
        .filter((action): action is CameraAuxActionName => isAuxActionName(action));

    return {
        key,
        label: stringOrNull(feature.label) ?? humanizeAuxKey(key),
        url: stringOrNull(feature.url),
        parameterKey,
        parameterValue,
        actions,
        outputKey: parameterValue,
        preferredAction: selectPreferredAction(actions),
        active: typeof feature.active === "boolean" ? feature.active : null,
        currentText: stringOrNull(feature.currentText),
        toggleSupported: actions.includes("start") && actions.includes("stop"),
    };
}

function isAuxActionName(action: string): action is CameraAuxActionName {
    return action === "start" || action === "stop" || action === "pulse";
}

function selectPreferredAction(
    actions: readonly CameraAuxActionName[],
): CameraAuxActionName | null {
    if (actions.includes("pulse")) {
        return "pulse";
    }
    if (actions.includes("start")) {
        return "start";
    }
    if (actions.includes("stop")) {
        return "stop";
    }
    return null;
}

function stringOrNull(value: string | null | undefined): string | null {
    if (typeof value !== "string") {
        return null;
    }
    const trimmed = value.trim();
    return trimmed ? trimmed : null;
}

function humanizeAuxKey(key: string): string {
    return key
        .split(/[_-]+/)
        .filter((part) => part.length > 0)
        .map((part) => part[0]!.toUpperCase() + part.slice(1))
        .join(" ");
}
