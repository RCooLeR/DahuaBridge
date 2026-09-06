package rtsprelay

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/streams"
)

type warmResolver struct {
	mockResolver
	warmMu      sync.Mutex
	settings    streams.LivePreconnectSettings
	targets     []streams.LivePreconnectTarget
	warmGate    <-chan struct{}
	warmEntered chan struct{}
}

func (r *warmResolver) LivePreconnectTargets() (streams.LivePreconnectSettings, []streams.LivePreconnectTarget) {
	r.warmMu.Lock()
	settings, targets, gate := r.settings, append([]streams.LivePreconnectTarget(nil), r.targets...), r.warmGate
	r.warmMu.Unlock()
	if r.warmEntered != nil {
		select {
		case r.warmEntered <- struct{}{}:
		default:
		}
	}
	if gate != nil {
		<-gate
	}
	return settings, targets
}

func (r *warmResolver) set(mode, profile string, targets ...streams.LivePreconnectTarget) {
	r.warmMu.Lock()
	r.settings = streams.LivePreconnectSettings{Mode: mode, Profile: profile}
	r.targets = targets
	r.warmMu.Unlock()
}

func waitWarm(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !predicate() {
		if time.Now().After(deadline) {
			t.Fatal("preconnect condition did not converge")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func warmTarget(id, profile string) streams.LivePreconnectTarget {
	return streams.LivePreconnectTarget{StreamID: id, Profile: profile}
}

func TestAlwaysPreconnectSharesInputAndDisablePreservesViewers(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "secret")
	r := &warmResolver{mockResolver: mockResolver{url: upstream.URL()}}
	r.set("always", "quality", warmTarget("cam1", "quality"))
	relay := newTestRelay(t, r, Config{IdleTimeout: 40 * time.Millisecond, PreconnectInterval: time.Hour})
	waitWarm(t, func() bool {
		statuses := relay.Statuses()
		return len(statuses) == 1 && statuses[0].Ready && statuses[0].Warm && statuses[0].BytesReceived > 0 && statuses[0].Viewers == 0
	})
	relay.mu.Lock()
	w := relay.workers["cam1:quality"]
	w.lastUsed = time.Now().Add(-time.Hour)
	relay.mu.Unlock()
	relay.retireBefore(w, time.Now())
	first, packets := openReader(t, relayURL(relay, "cam1", "quality", ""))
	second, packets2 := openReader(t, relay.LocalStreamURL("cam1", "quality"))
	expectPacket(t, packets, 0xA1)
	expectPacket(t, packets2, 0xA1)
	if upstream.plays.Load() != 1 {
		t.Fatal("preconnected readers opened duplicate upstreams")
	}
	r.set("off", "auto")
	relay.WakePreconnect()
	waitWarm(t, func() bool { statuses := relay.Statuses(); return len(statuses) == 1 && !statuses[0].Warm })
	expectPacket(t, packets, 0xA1)
	first.Close()
	second.Close()
	waitWarm(t, func() bool { return len(relay.Statuses()) == 0 })
}

func TestRecentPreconnectOnlyRetainsViewedInputs(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	r := &warmResolver{mockResolver: mockResolver{url: upstream.URL()}}
	// The selected always profile must not exclude another profile that an
	// actual viewer opened (for example, a quality inspector over stable HA).
	r.set("recent", "stable", warmTarget("unused", "stable"))
	relay := newTestRelay(t, r, Config{IdleTimeout: 40 * time.Millisecond, RecentKeepAlive: time.Minute, PreconnectInterval: time.Hour})
	waitWarm(t, func() bool { relay.mu.Lock(); defer relay.mu.Unlock(); return relay.preconnectMode == "recent" })
	if r.calls.Load() != 0 {
		t.Fatal("recent mode started an unused stream")
	}
	client, packets := openReader(t, relayURL(relay, "cam1", "quality", ""))
	expectPacket(t, packets, 0xA1)
	client.Close()
	waitWarm(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		w := relay.workers["cam1:quality"]
		return w != nil && relay.noDemandLocked(w)
	})
	relay.mu.Lock()
	w := relay.workers["cam1:quality"]
	w.lastUsed = time.Now().Add(-time.Hour)
	relay.mu.Unlock()
	relay.retireBefore(w, time.Now())
	if len(relay.Statuses()) != 1 {
		t.Fatal("recently viewed input expired at ordinary idle timeout")
	}
	relay.mu.Lock()
	w.lastViewed = time.Now().Add(-2 * time.Minute)
	relay.mu.Unlock()
	relay.retireBefore(w, time.Now())
	waitWarm(t, func() bool { return len(relay.Statuses()) == 0 })
	if r.calls.Load() != 1 {
		t.Fatal("recent mode reconnected an expired input")
	}
}

func TestPreconnectProfileChangeDoesNotCutActiveInput(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	r := &warmResolver{mockResolver: mockResolver{url: upstream.URL()}}
	r.set("always", "quality", warmTarget("cam1", "quality"))
	relay := newTestRelay(t, r, Config{MaxStreams: 3, PreconnectInterval: time.Hour})
	waitWarm(t, func() bool { return upstream.plays.Load() == 1 })
	client, packets := openReader(t, relayURL(relay, "cam1", "quality", ""))
	defer client.Close()
	r.set("always", "stable", warmTarget("cam1", "stable"))
	relay.WakePreconnect()
	waitWarm(t, func() bool { return upstream.plays.Load() == 2 })
	expectPacket(t, packets, 0xA1)
	statuses := relay.Statuses()
	for _, status := range statuses {
		if status.Profile == "quality" && status.Warm {
			t.Fatal("old profile retained warm lease")
		}
	}
}

func TestDemandCanEvictWarmInputButPendingDemandIsProtected(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	r := &warmResolver{mockResolver: mockResolver{url: upstream.URL()}}
	r.set("always", "quality", warmTarget("cam1", "quality"))
	relay := newTestRelay(t, r, Config{MaxStreams: 1, PreconnectInterval: time.Hour})
	waitWarm(t, func() bool { return upstream.plays.Load() == 1 })
	w, err := relay.getStreamForDemand(pathPrefix+"cam1/quality", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relay.getStreamForDemand(pathPrefix+"cam2/quality", "", true); err == nil {
		t.Fatal("pending DESCRIBE reservation was evicted")
	}
	relay.releaseDemand(w)
	client, packets := openReader(t, relayURL(relay, "cam2", "quality", ""))
	defer client.Close()
	expectPacket(t, packets, 0xA1)
	relay.WakePreconnect()
	waitWarm(t, func() bool { statuses := relay.Statuses(); return len(statuses) == 1 && statuses[0].StreamID == "cam2" })
	if _, err := relay.getStreamForDemand(pathPrefix+"cam3/quality", "", true); err == nil {
		t.Fatal("active reader was evicted")
	}
}

func TestPreconnectBoundsConcurrentStartsAndReservesCapacity(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	gate := make(chan struct{})
	var releaseGate sync.Once
	defer releaseGate.Do(func() { close(gate) })
	r := &warmResolver{mockResolver: mockResolver{url: upstream.URL(), gate: gate}}
	r.set("always", "quality", warmTarget("cam1", "quality"), warmTarget("cam2", "quality"), warmTarget("cam3", "quality"), warmTarget("cam4", "quality"))
	relay := newTestRelay(t, r, Config{MaxStreams: 4, PreconnectInterval: time.Hour})
	waitWarm(t, func() bool { return r.calls.Load() == 2 })
	relay.mu.Lock()
	starting, count := relay.preconnectStarting, len(relay.workers)
	relay.mu.Unlock()
	if starting != 2 || count != 2 {
		t.Fatalf("starting=%d workers=%d", starting, count)
	}
	releaseGate.Do(func() { close(gate) })
	waitWarm(t, func() bool { return upstream.plays.Load() == 3 })
	if len(relay.Statuses()) != 3 {
		t.Fatal("warm workers consumed reserved viewer capacity")
	}
}

func TestPreconnectFailureBackoffAndRouteInvalidation(t *testing.T) {
	dead := newMockUpstream(t, 0xA1, "")
	oldURL := dead.URL()
	dead.Close()
	r := &warmResolver{mockResolver: mockResolver{url: oldURL}}
	r.set("always", "quality", warmTarget("cam1", "quality"))
	relay := newTestRelay(t, r, Config{PreconnectInterval: time.Hour})
	waitWarm(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return len(relay.workers) == 0 && relay.preconnectRetry["cam1:quality"].After(time.Now().Add(55*time.Second))
	})
	for range 10 {
		relay.reconcilePreconnect()
	}
	if r.calls.Load() != 1 {
		t.Fatalf("failed warm input spun reconnects: %d", r.calls.Load())
	}
	upstream := newMockUpstream(t, 0xB2, "")
	r.mu.Lock()
	r.url = upstream.URL()
	r.mu.Unlock()
	relay.InvalidateStream("cam1")
	waitWarm(t, func() bool { return upstream.plays.Load() == 1 })
	_, packets := openReader(t, relayURL(relay, "cam1", "quality", ""))
	expectPacket(t, packets, 0xB2)
}

func TestStalePreconnectSnapshotCannotUndoDisable(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	gate := make(chan struct{})
	var releaseGate sync.Once
	defer releaseGate.Do(func() { close(gate) })
	r := &warmResolver{mockResolver: mockResolver{url: upstream.URL()}, warmGate: gate, warmEntered: make(chan struct{}, 1)}
	r.set("always", "quality", warmTarget("cam1", "quality"))
	relay := newTestRelay(t, r, Config{PreconnectInterval: time.Hour})
	select {
	case <-r.warmEntered:
	case <-time.After(time.Second):
		t.Fatal("provider not called")
	}
	r.set("off", "auto")
	relay.WakePreconnect()
	releaseGate.Do(func() { close(gate) })
	waitWarm(t, func() bool { relay.mu.Lock(); defer relay.mu.Unlock(); return relay.preconnectMode == "off" })
	if r.calls.Load() != 0 {
		t.Fatal("old provider result reopened a disabled input")
	}
}

func TestConcurrentWarmAndDemandStartupSharesOneWorker(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	gate := make(chan struct{})
	var releaseGate sync.Once
	defer releaseGate.Do(func() { close(gate) })
	r := &warmResolver{mockResolver: mockResolver{url: upstream.URL(), gate: gate}}
	r.set("always", "quality", warmTarget("cam1", "quality"))
	relay := newTestRelay(t, r, Config{PreconnectInterval: time.Hour})
	waitWarm(t, func() bool { return r.calls.Load() == 1 })
	done := make(chan error, 1)
	go func() {
		worker, err := relay.getStreamForDemand(pathPrefix+"cam1/quality", "", true)
		if err == nil {
			relay.releaseDemand(worker)
		}
		done <- err
	}()
	waitWarm(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return relay.workers["cam1:quality"].pendingDemand == 1
	})
	releaseGate.Do(func() { close(gate) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shared startup did not complete")
	}
	if r.calls.Load() != 1 || upstream.plays.Load() != 1 {
		t.Fatal("simultaneous warm and demand startup duplicated upstream")
	}
}

func TestCloseInterruptsPreconnectHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var serverWG sync.WaitGroup
	accepted := make(chan struct{}, 2)
	serverWG.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			serverWG.Go(func() { defer conn.Close(); accepted <- struct{}{}; _, _ = io.Copy(io.Discard, conn) })
		}
	})
	defer func() { listener.Close(); serverWG.Wait() }()
	r := &warmResolver{mockResolver: mockResolver{url: "rtsp://" + listener.Addr().String() + "/live"}}
	r.set("always", "quality", warmTarget("cam1", "quality"))
	relay := newTestRelay(t, r, Config{StartTimeout: time.Hour, PreconnectInterval: time.Hour})
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("warm handshake not started")
	}
	done := make(chan struct{})
	go func() { relay.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not join canceled warming")
	}
	relay.WakePreconnect()
	if len(relay.Statuses()) != 0 {
		t.Fatal("shutdown reopened warm stream")
	}
}
