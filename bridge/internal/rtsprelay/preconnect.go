package rtsprelay

import (
	"context"
	"sort"
	"time"

	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/bluenviron/gortsplib/v5"
)

// Match the runtime's failed-route cooldown: a speculative reconnect must not
// repeatedly report the same failure and keep extending that recovery window.
const preconnectFailureBackoff = 60 * time.Second

type preconnectResolver interface {
	LivePreconnectTargets() (streams.LivePreconnectSettings, []streams.LivePreconnectTarget)
}

// WakePreconnect invalidates an in-flight catalog snapshot as well as requesting
// an immediate refresh. Settings and route changes remain owned by the runtime.
func (s *Server) WakePreconnect() {
	s.mu.Lock()
	s.preconnectRevision++
	s.mu.Unlock()
	s.signalPreconnect()
}

func (s *Server) signalPreconnect() {
	select {
	case s.preconnectWake <- struct{}{}:
	default:
	}
}

func (s *Server) runPreconnect() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.PreconnectInterval)
	defer ticker.Stop()
	for {
		s.reconcilePreconnect()
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		case <-s.preconnectWake:
		}
	}
}

func (s *Server) reconcilePreconnect() {
	s.mu.Lock()
	if s.closed || !s.started {
		s.mu.Unlock()
		return
	}
	revision := s.preconnectRevision
	s.mu.Unlock()
	// Catalog resolution may take runtime locks. Resolve outside the relay lock,
	// then reject the snapshot if a setting/source changed while it was read.
	settings, targets := s.resolver.(preconnectResolver).LivePreconnectTargets()
	targets = append([]streams.LivePreconnectTarget(nil), targets...)
	settings = settings.Defaulted()
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].StreamID != targets[j].StreamID {
			return targets[i].StreamID < targets[j].StreamID
		}
		return targets[i].Profile < targets[j].Profile
	})
	s.mu.Lock()
	if s.closed || s.preconnectRevision != revision {
		s.mu.Unlock()
		return
	}
	s.preconnectMode = settings.Mode
	desired := make(map[string]streams.LivePreconnectTarget)
	// Leave one slot for demand. A one-slot installation can lend its slot to
	// warming because demand atomically evicts an unused warm worker.
	budget := max(1, s.cfg.MaxStreams-1)
	if settings.Mode == streams.LivePreconnectAlways {
		for _, target := range targets {
			id, profile, ok := streamPath(pathPrefix+target.StreamID+"/"+target.Profile, "")
			if ok && len(desired) < budget {
				desired[id+":"+profile] = target
			}
		}
	}
	s.preconnectTargets = desired
	var cleanups []func()
	for _, worker := range s.workers {
		_, wanted := desired[worker.key]
		wasWarm := worker.warm
		worker.warm = wanted
		// Never-used speculative connections have no purpose once their lease
		// disappears. Viewed inputs retain the ordinary/recent idle policy.
		if wasWarm && !wanted && worker.lastViewed.IsZero() && s.noDemandLocked(worker) {
			if cleanup := s.retireLocked(worker, time.Time{}); cleanup != nil {
				cleanups = append(cleanups, cleanup)
			}
		}
	}
	now := time.Now()
	// Keep failures even when the runtime temporarily omits a cooling route
	// from its targets. Reappearing in the catalog must not bypass this backoff.
	for key, until := range s.preconnectRetry {
		if now.After(until) {
			delete(s.preconnectRetry, key)
		}
	}
	// At most two speculative connections perform the upstream handshake at a
	// time. All workers still count against the existing global stream limit.
	if settings.Mode == streams.LivePreconnectAlways {
		for _, target := range targets {
			key := target.StreamID + ":" + target.Profile
			if _, ok := desired[key]; !ok || s.workers[key] != nil || now.Before(s.preconnectRetry[key]) {
				continue
			}
			if s.preconnectStarting >= 2 || len(s.workers) >= budget {
				break
			}
			s.startWorkerLocked(target.StreamID, target.Profile, true)
		}
	}
	s.mu.Unlock()
	for _, cleanup := range cleanups {
		cleanup()
	}
}

// Both demand and warming create workers here, under s.mu. Registering the
// worker and its goroutine together prevents duplicate inputs and Add/Close races.
func (s *Server) startWorkerLocked(id, profile string, warmStart bool) *relayStream {
	ctx, cancel := context.WithCancel(s.ctx)
	key := id + ":" + profile
	_, warm := s.preconnectTargets[key]
	worker := &relayStream{key: key, id: id, profile: profile, ctx: ctx, cancel: cancel,
		ready: make(chan struct{}), lastUsed: time.Now(), readers: make(map[*gortsplib.ServerSession]reader), conns: make(map[*gortsplib.ServerConn]struct{}),
		warm: warm, warmStarting: warmStart,
	}
	if warmStart {
		s.preconnectStarting++
	}
	s.workers[key] = worker
	s.wg.Add(1)
	go s.run(worker)
	return worker
}

func (s *Server) releaseDemand(worker *relayStream) {
	s.mu.Lock()
	worker.pendingDemand--
	s.mu.Unlock()
}

func (s *Server) noDemandLocked(worker *relayStream) bool {
	return worker.pendingDemand == 0 && len(worker.readers) == 0 && len(worker.conns) == 0
}

func (s *Server) keepWarmLocked(worker *relayStream, now time.Time) bool {
	if s.preconnectMode == streams.LivePreconnectAlways {
		return worker.warm
	}
	// Recent mode follows what viewers actually opened; the preferred profile
	// only chooses proactive targets in always mode.
	return s.preconnectMode == streams.LivePreconnectRecent &&
		!worker.lastViewed.IsZero() && now.Sub(worker.lastViewed) < s.cfg.RecentKeepAlive
}

func (s *Server) idleWarmWorkerLocked() *relayStream {
	var oldest *relayStream
	for _, worker := range s.workers {
		if s.noDemandLocked(worker) && s.keepWarmLocked(worker, time.Now()) && (oldest == nil || worker.lastUsed.Before(oldest.lastUsed)) {
			oldest = worker
		}
	}
	return oldest
}

func (s *Server) finishWarmStartLocked(worker *relayStream) {
	// Successful PLAY frees the handshake slot before the long-running worker
	// exits. Failed/canceled startups free it during teardown, exactly once.
	if worker.warmStarting {
		worker.warmStarting = false
		s.preconnectStarting--
	}
}

func (s *Server) finishWorker(worker *relayStream) {
	s.mu.Lock()
	s.finishWarmStartLocked(worker)
	if !worker.retired && worker.warm && (worker.ctx.Err() == nil || worker.startTimedOut.Load()) {
		// Invalid paths and transport/authentication failures must not spin a
		// reconnect loop or repeatedly extend runtime route-health cooldowns.
		if _, exists := s.preconnectRetry[worker.key]; !exists && len(s.preconnectRetry) >= 2*s.cfg.MaxStreams {
			var oldest string
			for key, until := range s.preconnectRetry {
				if _, desired := s.preconnectTargets[key]; desired {
					continue
				}
				if oldest == "" || until.Before(s.preconnectRetry[oldest]) {
					oldest = key
				}
			}
			if oldest != "" {
				delete(s.preconnectRetry, oldest)
			}
		}
		s.preconnectRetry[worker.key] = time.Now().Add(preconnectFailureBackoff)
	}
	cleanup := s.retireLocked(worker, time.Time{})
	s.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}
	s.signalPreconnect()
}
