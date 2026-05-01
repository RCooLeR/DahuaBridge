package media

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"

	"github.com/rs/zerolog"
)

const audioProbeCacheTTL = 5 * time.Minute

type audioProbeCacheEntry struct {
	CheckedAt time.Time
	HasAudio  bool
}

func (m *Manager) shouldIncludeSourceAudio(profile streams.Profile, logger zerolog.Logger) bool {
	if m == nil {
		return true
	}

	streamURL := strings.TrimSpace(profile.StreamURL)
	if streamURL == "" {
		return true
	}

	now := time.Now()
	m.audioProbeMu.Lock()
	if cached, ok := m.audioProbeCache[streamURL]; ok && now.Sub(cached.CheckedAt) < audioProbeCacheTTL {
		m.audioProbeMu.Unlock()
		return cached.HasAudio
	}
	m.audioProbeMu.Unlock()

	hasAudio, err := probeStreamHasAudio(m.cfg, profile)
	if err != nil {
		logger.Debug().Err(err).Msg("source audio probe failed; keeping audio output enabled")
		return true
	}

	m.audioProbeMu.Lock()
	m.audioProbeCache[streamURL] = audioProbeCacheEntry{
		CheckedAt: now,
		HasAudio:  hasAudio,
	}
	m.audioProbeMu.Unlock()
	return hasAudio
}

func probeStreamHasAudio(cfg config.MediaConfig, profile streams.Profile) (bool, error) {
	probeCtx, cancel := context.WithTimeout(context.Background(), audioProbeTimeout(cfg.StartTimeout))
	defer cancel()

	args := []string{
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=index",
		"-of", "default=noprint_wrappers=1:nokey=1",
		"-analyzeduration", "1000000",
		"-probesize", "1048576",
	}
	transport := strings.TrimSpace(profile.RTSPTransport)
	if transport == "" {
		transport = "tcp"
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(profile.StreamURL)), "rtsp://") {
		args = append(args, "-rtsp_transport", transport)
	}
	args = append(args, profile.StreamURL)

	cmd := exec.CommandContext(probeCtx, ffprobePath(cfg.FFmpegPath), args...)
	body, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("probe source audio: %w: %s", err, strings.TrimSpace(string(body)))
	}
	return strings.TrimSpace(string(body)) != "", nil
}

func ffprobePath(ffmpegPath string) string {
	trimmed := strings.TrimSpace(ffmpegPath)
	if trimmed == "" {
		return "ffprobe"
	}

	base := strings.ToLower(filepath.Base(trimmed))
	ext := filepath.Ext(trimmed)
	switch base {
	case "ffmpeg", "ffmpeg.exe":
		name := "ffprobe"
		if ext != "" {
			name += ext
		}
		return filepath.Join(filepath.Dir(trimmed), name)
	default:
		return "ffprobe"
	}
}

func audioProbeTimeout(startTimeout time.Duration) time.Duration {
	timeout := 3 * time.Second
	if startTimeout > 0 && startTimeout/4 < timeout {
		timeout = startTimeout / 4
	}
	if timeout < time.Second {
		return time.Second
	}
	return timeout
}
