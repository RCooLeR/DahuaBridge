export interface PlaybackOperation {
  readonly signal: AbortSignal;
  isCurrent(): boolean;
}

/** Owns pending playback work so only the latest user intent may update the view. */
export class PlaybackLifecycle {
  private active: AbortController | null = null;

  begin(): PlaybackOperation {
    this.cancel();
    const controller = new AbortController();
    this.active = controller;
    return {
      signal: controller.signal,
      // HA service calls and already-resolved fetches may outlive cancellation.
      isCurrent: () => this.active === controller && !controller.signal.aborted,
    };
  }

  cancel(): void {
    this.active?.abort();
    this.active = null;
  }
}
