package media

// Retained archive output is a bounded file cache, independent of process slots.
// Eviction wakes the owner's cleanup task, which removes only its own directory.
type retainedOutput struct {
	key          string
	serial       uint64
	expired      chan struct{}
	removeLocked func()
}

func (m *Manager) retainHLSWorker(w *hlsWorker) *retainedOutput {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.completed = true
	return m.retainOutputLocked("hls:"+w.key, func() {
		if m.hlsWorkers[w.key] == w {
			delete(m.hlsWorkers, w.key)
		}
	})
}

func (m *Manager) retainDASHWorker(w *dashWorker) *retainedOutput {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.completed = true
	return m.retainOutputLocked("dash:"+w.key, func() {
		if m.dashWorkers[w.key] == w {
			delete(m.dashWorkers, w.key)
		}
	})
}

func (m *Manager) retainOutputLocked(key string, removeLocked func()) *retainedOutput {
	// Sequence order remains deterministic when timestamps tie or the clock moves.
	m.retentionSerial++
	retained := &retainedOutput{key: key, serial: m.retentionSerial, expired: make(chan struct{}), removeLocked: removeLocked}
	if old := m.retainedOutputs[key]; old != nil {
		old.removeLocked()
		close(old.expired)
	}
	m.retainedOutputs[key] = retained
	limit := m.cfg.MaxWorkers
	if limit <= 0 {
		limit = 32
	}
	for len(m.retainedOutputs) > limit {
		var oldest *retainedOutput
		for _, candidate := range m.retainedOutputs {
			if oldest == nil || candidate.serial < oldest.serial {
				oldest = candidate
			}
		}
		delete(m.retainedOutputs, oldest.key)
		oldest.removeLocked()
		close(oldest.expired)
	}
	m.setMediaWorkerCountLocked()
	return retained
}

func (m *Manager) releaseRetainedOutput(retained *retainedOutput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.retainedOutputs[retained.key] == retained {
		delete(m.retainedOutputs, retained.key)
	}
}
