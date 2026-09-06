package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/streams"
)

func (r *runtimeServices) GetLivePreconnect() streams.LivePreconnectSettings {
	return r.probes.LivePreconnect()
}

func (r *runtimeServices) SetLivePreconnect(ctx context.Context, settings streams.LivePreconnectSettings) (streams.LivePreconnectSettings, error) {
	// Serialize with source selection: persist first, then let the relay reconcile
	// current targets. A failed disk write must not start background streams.
	r.liveSourceMu.Lock()
	defer r.liveSourceMu.Unlock()
	if err := ctx.Err(); err != nil {
		return streams.LivePreconnectSettings{}, err
	}
	if !settings.Valid() {
		return streams.LivePreconnectSettings{}, streams.ErrInvalidLivePreconnect
	}
	if !r.cfg.StateStore.Enabled || strings.TrimSpace(r.cfg.StateStore.Path) == "" {
		return streams.LivePreconnectSettings{}, streams.ErrLiveSourcePersistence
	}
	if settings != r.GetLivePreconnect() {
		if err := r.probes.SetLivePreconnectAndSave(r.cfg.StateStore.Path, settings); err != nil {
			return streams.LivePreconnectSettings{}, fmt.Errorf("%w: %v", streams.ErrLiveSourcePersistence, err)
		}
		r.mu.RLock()
		relay, ok := r.liveRelay.(interface{ WakePreconnect() })
		r.mu.RUnlock()
		if ok {
			relay.WakePreconnect()
		}
	}
	return r.GetLivePreconnect(), nil
}

// LivePreconnectTargets supplies names, not URLs, to the relay. GetStream still
// makes the actual source decision at connection time, including failover.
func (r *runtimeServices) LivePreconnectTargets() (streams.LivePreconnectSettings, []streams.LivePreconnectTarget) {
	settings := r.GetLivePreconnect()
	if settings.Mode != streams.LivePreconnectAlways || !r.cfg.Media.Enabled {
		return settings, nil
	}
	var targets []streams.LivePreconnectTarget
	// Warm reconciliation does not need recording summaries or clip-directory I/O.
	for _, entry := range r.listStreams(true, false) {
		if strings.HasPrefix(entry.ID, "nvrpb_") {
			continue
		}
		name, ok := preconnectProfile(entry, settings.Profile)
		if !ok {
			continue
		}
		// Do not turn a failing camera into a permanent background retry loop
		// that continually extends its source-health cooldown.
		if entry.LiveSource != nil {
			r.liveHealth.mu.Lock()
			health := r.liveHealth.states[entry.ID]
			blocked := health.selection.Signature == entry.LiveSourceSignature && time.Now().Before(health.failedUntil[entry.LiveSource.Source])
			r.liveHealth.mu.Unlock()
			if blocked {
				continue
			}
		}
		targets = append(targets, streams.LivePreconnectTarget{StreamID: entry.ID, Profile: name})
	}
	return settings, targets
}

func preconnectProfile(entry streams.Entry, preferred string) (string, bool) {
	// Match HA's profile fallback order without opening both profiles.
	for _, name := range []string{preferred, entry.RecommendedProfile, "quality", "stable"} {
		if name != "quality" && name != "stable" {
			continue
		}
		profile, ok := entry.Profiles[name]
		if !ok || profile.InputDuration != 0 || profile.InputPrefixURL != "" {
			continue
		}
		u, err := url.Parse(profile.StreamURL)
		if err == nil && (u.Scheme == "rtsp" || u.Scheme == "rtsps") && u.Host != "" &&
			!strings.Contains(strings.ToLower(u.Path), "playback") && !strings.HasPrefix(u.Path, "/api/v1/rtsp/live/") {
			return name, true
		}
	}
	return "", false
}
