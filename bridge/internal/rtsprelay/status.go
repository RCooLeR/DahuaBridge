package rtsprelay

import (
	"net/url"
	"sort"
	"time"
)

type Status struct {
	Key, StreamID, Profile             string
	Source, SourceURL                  string
	VideoCodec, AudioCodec             string
	Channel, Viewers                   int
	StartedAt, LastPacketAt            time.Time
	BytesReceived, Bitrate, Reconnects uint64
	Ready                              bool
	Warm                               bool
	PreconnectMode                     string
}

func (s *Server) Statuses() []Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	result := make([]Status, 0, len(s.workers))
	for _, worker := range s.workers {
		bytes := worker.bytesReceived.Load()
		if elapsed := now.Sub(worker.previousSample); worker.previousSample.IsZero() {
			worker.previousSample, worker.previousBytes = now, bytes
		} else if elapsed >= time.Second {
			worker.bitrate = uint64(float64(bytes-worker.previousBytes) * 8 / elapsed.Seconds())
			worker.previousSample, worker.previousBytes = now, bytes
		}
		status := Status{Key: worker.key, StreamID: worker.id, Profile: worker.profile,
			Source: worker.source, Channel: worker.channel, StartedAt: worker.startedAt,
			VideoCodec: worker.videoCodec, AudioCodec: worker.audioCodec,
			BytesReceived: bytes, Bitrate: worker.bitrate, Ready: worker.stream != nil,
			Warm: s.keepWarmLocked(worker, now), PreconnectMode: s.preconnectMode,
		}
		if starts := s.starts[worker.key]; starts > 0 {
			status.Reconnects = starts - 1
		}
		if parsed, err := url.Parse(worker.sourceURL); err == nil {
			parsed.User = nil
			status.SourceURL = parsed.String()
		}
		if last := worker.lastPacket.Load(); last != 0 {
			status.LastPacketAt = time.Unix(0, last)
		}
		for _, reader := range worker.readers {
			if reader.playing {
				status.Viewers++
			}
		}
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}
