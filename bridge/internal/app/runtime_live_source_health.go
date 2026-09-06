package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua/rtsp"
	"RCooLeR/DahuaBridge/internal/streams"
)

const (
	liveSourceHealthInterval  = 15 * time.Second
	liveSourceFailureCooldown = 60 * time.Second
	liveSourceProbeTimeout    = 4 * time.Second
	liveSourceHealthWorkers   = 4
)

type liveSourceHealthState struct {
	selection   streams.RuntimeLiveSourceState
	failedUntil map[string]time.Time
}

type liveSourceHealthRuntime struct {
	mu        sync.Mutex
	ctx       context.Context
	queue     chan string
	queued    map[string]bool
	failures  map[string]string
	states    map[string]liveSourceHealthState
	revisions map[string]uint64
	probe     func(context.Context, string) (bool, error)
	wg        sync.WaitGroup
}

func newLiveSourceHealthRuntime() liveSourceHealthRuntime {
	return liveSourceHealthRuntime{
		queue:     make(chan string, 64),
		queued:    make(map[string]bool),
		failures:  make(map[string]string),
		states:    make(map[string]liveSourceHealthState),
		revisions: make(map[string]uint64),
		probe: func(ctx context.Context, streamURL string) (bool, error) {
			return rtsp.DescribeAvailable(ctx, streamURL, liveSourceProbeTimeout, false)
		},
	}
}

// StartLiveSourceHealth runs bounded RTSP checks for both native HA clients and
// bridge media. The returned stop function cancels probes and waits for workers.
func (r *runtimeServices) StartLiveSourceHealth(parent context.Context) func() {
	h := &r.liveHealth
	h.mu.Lock()
	if h.ctx != nil {
		h.mu.Unlock()
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	h.ctx = ctx
	for range liveSourceHealthWorkers {
		h.wg.Go(func() { r.runLiveSourceHealthWorker(ctx) })
	}
	h.wg.Go(func() {
		ticker := time.NewTicker(liveSourceHealthInterval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			for _, entry := range r.ListStreams(false) {
				if entry.LiveSource != nil {
					r.queueLiveSourceHealth(entry.ID, "")
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	h.mu.Unlock()
	return func() { cancel(); h.wg.Wait() }
}

// ReportLiveSourceFailure never probes or calls back into the media manager on
// the reporting goroutine. Workers validate the failed URL against the current
// catalog before considering a switch, so obsolete worker failures are ignored.
func (r *runtimeServices) ReportLiveSourceFailure(streamID, failedURL string) {
	if strings.TrimSpace(failedURL) != "" {
		r.queueLiveSourceHealth(streamID, failedURL)
	}
}

func (r *runtimeServices) queueLiveSourceHealth(streamID, failedURL string) {
	h := &r.liveHealth
	h.mu.Lock()
	defer h.mu.Unlock()
	if streamID == "" || h.ctx == nil || h.ctx.Err() != nil {
		return
	}
	if failedURL != "" {
		h.failures[streamID] = failedURL
	}
	if h.queued[streamID] {
		return
	}
	select {
	case h.queue <- streamID:
		h.queued[streamID] = true
	default:
		delete(h.failures, streamID)
	}
}

func (r *runtimeServices) runLiveSourceHealthWorker(ctx context.Context) {
	h := &r.liveHealth
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case streamID := <-h.queue:
			h.mu.Lock()
			failedURL := h.failures[streamID]
			delete(h.failures, streamID)
			h.mu.Unlock()
			r.checkLiveSourceHealth(ctx, streamID, failedURL)
			h.mu.Lock()
			delete(h.queued, streamID)
			pending := h.failures[streamID]
			h.mu.Unlock()
			if pending != "" {
				r.queueLiveSourceHealth(streamID, "")
			}
		}
	}
}

func (r *runtimeServices) liveSourceSelections() map[string]streams.RuntimeLiveSourceState {
	h := &r.liveHealth
	h.mu.Lock()
	defer h.mu.Unlock()
	selections := make(map[string]streams.RuntimeLiveSourceState, len(h.states))
	for id, state := range h.states {
		selections[id] = state.selection
	}
	return selections
}

func (r *runtimeServices) clearLiveSourceHealth(streamID string) {
	h := &r.liveHealth
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.states, streamID)
	delete(h.failures, streamID)
	h.revisions[streamID]++
}

func (r *runtimeServices) liveSourceEntry(streamID string) (streams.Entry, bool) {
	for _, entry := range r.ListStreams(true) {
		if entry.ID == streamID && entry.LiveSource != nil {
			return entry, true
		}
	}
	return streams.Entry{}, false
}

func (r *runtimeServices) checkLiveSourceHealth(ctx context.Context, streamID, failedURL string) {
	// Only snapshot/apply phases take the settings lock. Network work must not
	// block a user changing their preference.
	r.liveSourceMu.Lock()
	entry, ok := r.liveSourceEntry(streamID)
	h := &r.liveHealth
	h.mu.Lock()
	revision := h.revisions[streamID]
	state := h.states[streamID]
	probe := h.probe
	h.mu.Unlock()
	r.liveSourceMu.Unlock()
	if !ok || ctx.Err() != nil {
		return
	}
	if failedURL != "" && !liveSourceURLMatches(entry, failedURL) {
		return
	}
	if state.selection.Signature != entry.LiveSourceSignature {
		state = liveSourceHealthState{failedUntil: make(map[string]time.Time)}
	} else {
		cloned := make(map[string]time.Time, len(state.failedUntil))
		for source, deadline := range state.failedUntil {
			cloned[source] = deadline
		}
		state.failedUntil = cloned
	}
	current, preferred := entry.LiveSource.Source, entry.LiveSource.PreferredSource
	routes := liveSourceRouteURLs(entry)
	checked := make(map[string]bool)
	results := make(map[string]bool)
	now := time.Now()
	if failedURL != "" {
		checked[current], results[current] = true, false
		state.failedUntil[current] = now.Add(liveSourceFailureCooldown)
	}
	checkRoute := func(source string) bool {
		if checked[source] {
			return results[source]
		}
		checked[source] = true
		urls := routes[source]
		if len(urls) == 0 || now.Before(state.failedUntil[source]) {
			return false
		}
		for _, streamURL := range urls {
			probeCtx, cancel := context.WithTimeout(ctx, liveSourceProbeTimeout)
			available, err := probe(probeCtx, streamURL)
			cancel()
			if !available || err != nil {
				state.failedUntil[source] = now.Add(liveSourceFailureCooldown)
				return false
			}
		}
		delete(state.failedUntil, source)
		results[source] = true
		return true
	}
	currentHealthy := checkRoute(current)
	next := current
	if preferred != current && checkRoute(preferred) {
		next = preferred
	} else if !currentHealthy && checkRoute(otherLiveSource(current)) {
		next = otherLiveSource(current)
	}
	if ctx.Err() != nil {
		return
	}
	reason := ""
	if next != preferred {
		if preferred == streams.LiveSourceCamera && !entry.LiveSource.CameraAvailable {
			reason = entry.LiveSource.CameraUnavailableReason
		} else {
			reason = fmt.Sprintf("Preferred %s stream is unavailable; using %s.", preferred, next)
		}
	}
	state.selection = streams.RuntimeLiveSourceState{Source: next, Signature: entry.LiveSourceSignature, FallbackReason: reason}
	r.liveSourceMu.Lock()
	defer r.liveSourceMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	fresh, ok := r.liveSourceEntry(streamID)
	if !ok || fresh.LiveSourceSignature != entry.LiveSourceSignature {
		return
	}
	h.mu.Lock()
	if h.revisions[streamID] != revision {
		h.mu.Unlock()
		return
	}
	h.states[streamID] = state
	h.mu.Unlock()
	if fresh.LiveSource.Source != next {
		r.invalidateLiveSource(fresh)
	}
}

func liveSourceURLMatches(entry streams.Entry, failedURL string) bool {
	for _, profile := range entry.Profiles {
		if profile.StreamURL != "" && profile.StreamURL == failedURL {
			return true
		}
	}
	return false
}

func liveSourceRouteURLs(entry streams.Entry) map[string][]string {
	routes := map[string][]string{streams.LiveSourceNVR: {}, streams.LiveSourceCamera: {}}
	seen := make(map[string]bool)
	for _, profile := range entry.Profiles {
		for source, streamURL := range map[string]string{entry.LiveSource.Source: profile.StreamURL, otherLiveSource(entry.LiveSource.Source): profile.AlternativeStreamURL} {
			if streamURL != "" && !seen[source+"\n"+streamURL] {
				seen[source+"\n"+streamURL] = true
				routes[source] = append(routes[source], streamURL)
			}
		}
	}
	for _, urls := range routes {
		sort.Strings(urls)
	}
	return routes
}

func otherLiveSource(source string) string {
	if source == streams.LiveSourceCamera {
		return streams.LiveSourceNVR
	}
	return streams.LiveSourceCamera
}
