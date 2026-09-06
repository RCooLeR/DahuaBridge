package media

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/rs/zerolog"
)

func TestConcurrentMJPEGDisconnectAndFrames(t *testing.T) {
	m := New(config.MediaConfig{IdleTimeout: time.Hour}, testResolver{}, zerolog.Nop(), nil)
	defer m.Close()
	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()
	w := &worker{parent: m, ctx: ctx, cancel: cancel, logger: zerolog.Nop(), subscribers: map[chan []byte]struct{}{}, ready: make(chan struct{})}
	w.addSubscriber(make(chan []byte, 1))
	var workers sync.WaitGroup
	workers.Go(func() {
		for range 10000 {
			w.publishFrame([]byte{1, 2, 3})
		}
	})
	workers.Go(func() {
		for range 10000 {
			ch := make(chan []byte, 1)
			w.addSubscriber(ch)
			w.removeSubscriber(ch)
		}
	})
	workers.Wait()
	w.closeSubscribers()
	ch := make(chan []byte, 1)
	w.addSubscriber(ch)
	if _, ok := <-ch; ok {
		t.Fatal("subscriber attached after worker teardown")
	}
}

func TestMediaCloseCancelsAndJoinsTasksAndRejectsNewWork(t *testing.T) {
	m := New(config.MediaConfig{Enabled: true}, testResolver{}, zerolog.Nop(), nil)
	finished := make(chan struct{})
	if !m.runTask(func() { <-m.ctx.Done(); close(finished) }) {
		t.Fatal("task rejected before shutdown")
	}
	m.Close()
	select {
	case <-finished:
	default:
		t.Fatal("Close returned before child cleanup")
	}
	if m.runTask(func() { t.Error("ran a task after shutdown") }) {
		t.Fatal("accepted a task after shutdown")
	}
	entry, profile := streams.Entry{ID: "cam"}, streams.Profile{}
	if _, err := m.getOrCreateHLSWorker(entry, "stable", profile); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("HLS after Close: %v", err)
	}
	if _, err := m.getOrCreateDASHWorker(entry, "stable", profile); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("DASH after Close: %v", err)
	}
	if _, err := m.getOrCreateMJPEGWorker(entry, "stable", profile, 0); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("MJPEG after Close: %v", err)
	}
	m.Close()
}

func TestRetainedArchiveOutputsDoNotConsumeProcessSlotsAndAreBounded(t *testing.T) {
	m := New(config.MediaConfig{MaxWorkers: 2}, testResolver{}, zerolog.Nop(), nil)
	defer m.Close()
	hls := &hlsWorker{key: "one:stable", parent: m}
	dash := &dashWorker{key: "two:stable", parent: m}
	m.hlsWorkers[hls.key] = hls
	m.dashWorkers[dash.key] = dash
	first := m.retainHLSWorker(hls)
	m.retainDASHWorker(dash)
	if m.activeWorkerCountLocked() != 0 {
		t.Fatal("completed output consumed active process capacity")
	}
	later := &hlsWorker{key: "three:stable", parent: m}
	m.hlsWorkers[later.key] = later
	m.retainHLSWorker(later)
	if len(m.retainedOutputs) != 2 || m.hlsWorkers[hls.key] != nil {
		t.Fatal("retained output cache exceeded capacity or evicted wrong entry")
	}
	select {
	case <-first.expired:
	default:
		t.Fatal("evicted output cleanup was not signaled")
	}
}
