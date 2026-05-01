package media

import (
	"io"
	"strings"
)

type tailBuffer struct {
	limit int
	buf   []byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	if len(p) >= b.limit {
		b.buf = append(b.buf[:0], p[len(p)-b.limit:]...)
		return len(p), nil
	}
	overflow := len(b.buf) + len(p) - b.limit
	if overflow > 0 {
		copy(b.buf, b.buf[overflow:])
		b.buf = b.buf[:len(b.buf)-overflow]
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *tailBuffer) String() string {
	return strings.TrimSpace(string(b.buf))
}

func drainFFmpegStderr(r io.Reader, limit int) <-chan string {
	done := make(chan string, 1)
	go func() {
		tail := newTailBuffer(limit)
		_, _ = io.Copy(tail, r)
		done <- tail.String()
	}()
	return done
}
