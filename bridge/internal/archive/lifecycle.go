package archive

import (
	"context"
	"errors"
)

// Register manual syncs as well as scheduled jobs before Close starts waiting.
func (s *Service) beginSync(parent context.Context) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, errors.New("archive service is closed")
	}
	s.wg.Add(1)
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(s.ctx, cancel)
	return ctx, func() { stop(); cancel(); s.wg.Done() }, nil
}
