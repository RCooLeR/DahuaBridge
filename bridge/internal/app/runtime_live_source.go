package app

import (
	"context"
	"fmt"
	"strings"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

func (r *runtimeServices) SetStreamLiveSource(ctx context.Context, streamID, source string) (streams.Entry, error) {
	r.liveSourceMu.Lock()
	defer r.liveSourceMu.Unlock()
	if err := ctx.Err(); err != nil {
		return streams.Entry{}, err
	}
	if source != streams.LiveSourceNVR && source != streams.LiveSourceCamera && source != streams.LiveSourceDefault {
		return streams.Entry{}, streams.ErrInvalidLiveSourceOverride
	}
	var current *streams.Entry
	for _, entry := range r.ListStreams(false) {
		if entry.ID == streamID {
			current = &entry
			break
		}
	}
	if current == nil {
		return streams.Entry{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, streamID)
	}
	if current.LiveSource == nil {
		return streams.Entry{}, streams.ErrLiveSourceUnsupported
	}
	if !r.cfg.StateStore.Enabled || strings.TrimSpace(r.cfg.StateStore.Path) == "" {
		return streams.Entry{}, streams.ErrLiveSourcePersistence
	}
	overrideSource := source
	if source == streams.LiveSourceDefault {
		overrideSource = ""
	}
	if err := r.probes.SetLiveSourceAndSave(r.cfg.StateStore.Path, streamID, overrideSource); err != nil {
		return streams.Entry{}, fmt.Errorf("%w: %v", streams.ErrLiveSourcePersistence, err)
	}
	if current.LiveSource.OverrideSource != overrideSource {
		r.clearLiveSourceHealth(streamID)
	}
	for _, entry := range r.ListStreams(false) {
		if entry.ID == streamID {
			if current.LiveSource.Source != entry.LiveSource.Source {
				r.invalidateLiveSource(entry)
			}
			r.queueLiveSourceHealth(streamID, "")
			return entry, nil
		}
	}
	return streams.Entry{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, streamID)
}

func (r *runtimeServices) GetDefaultLiveSource() streams.LiveSourceSettings {
	source, _ := r.probes.LiveSourceSettings()
	return streams.LiveSourceSettings{Source: source}
}

func (r *runtimeServices) SetDefaultLiveSource(ctx context.Context, source string) (streams.LiveSourceSettings, error) {
	r.liveSourceMu.Lock()
	defer r.liveSourceMu.Unlock()
	if err := ctx.Err(); err != nil {
		return streams.LiveSourceSettings{}, err
	}
	if source != streams.LiveSourceNVR && source != streams.LiveSourceCamera {
		return streams.LiveSourceSettings{}, streams.ErrInvalidLiveSource
	}
	if !r.cfg.StateStore.Enabled || strings.TrimSpace(r.cfg.StateStore.Path) == "" {
		return streams.LiveSourceSettings{}, streams.ErrLiveSourcePersistence
	}
	previous := make(map[string]*streams.LiveSourceSummary)
	for _, entry := range r.ListStreams(false) {
		if entry.LiveSource != nil {
			previous[entry.ID] = entry.LiveSource
		}
	}
	if err := r.probes.SetDefaultLiveSourceAndSave(r.cfg.StateStore.Path, source); err != nil {
		return streams.LiveSourceSettings{}, fmt.Errorf("%w: %v", streams.ErrLiveSourcePersistence, err)
	}
	for id, summary := range previous {
		if summary.OverrideSource == "" && summary.DefaultSource != source {
			r.clearLiveSourceHealth(id)
		}
	}
	for _, entry := range r.ListStreams(false) {
		if entry.LiveSource != nil {
			if old := previous[entry.ID]; old != nil && old.Source != entry.LiveSource.Source {
				r.invalidateLiveSource(entry)
			}
			r.queueLiveSourceHealth(entry.ID, "")
		}
	}
	return r.GetDefaultLiveSource(), nil
}

func (r *runtimeServices) invalidateLiveSource(entry streams.Entry) {
	r.mu.Lock()
	cacheKey := snapshotCacheKey("nvr", entry.RootDeviceID, entry.Channel)
	delete(r.snapshotCache, cacheKey)
	delete(r.snapshotFlight, cacheKey)
	mediaReader := r.media
	liveRelay := r.liveRelay
	r.mu.Unlock()
	if invalidator, ok := mediaReader.(interface{ InvalidateStream(string) }); ok {
		invalidator.InvalidateStream(entry.ID)
	}
	if liveRelay != nil {
		liveRelay.InvalidateStream(entry.ID)
	}
}
