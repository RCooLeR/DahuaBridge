package media

import (
	"context"
	"errors"
)

var ErrManagerClosed = errors.New("media manager is closed")

// Close prevents new work, cancels inputs and waits for FFmpeg and media cleanup.
// Clip commands receive a graceful quit request before their bounded kill delay.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.closed = true
	m.cancel()
	m.mu.Unlock()
	m.wg.Wait()
}

// runTask is for callbacks that may arrive after their owning session closed.
// Its registration shares the shutdown lock, so Wait cannot race a zero-count Add.
func (m *Manager) runTask(fn func()) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.wg.Go(fn)
	return true
}

func (m *Manager) context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}
