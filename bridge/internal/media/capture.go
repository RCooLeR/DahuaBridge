package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/rs/zerolog"
)

var (
	ErrClipNotFound       = errors.New("clip not found")
	ErrClipAlreadyActive  = errors.New("clip recording already active for stream")
	errClipStorageMissing = errors.New("clip storage path is not configured")
)

type ClipStatus string

const (
	ClipStatusRecording ClipStatus = "recording"
	ClipStatusCompleted ClipStatus = "completed"
	ClipStatusFailed    ClipStatus = "failed"
)

type ClipInfo struct {
	ID             string           `json:"id"`
	StreamID       string           `json:"stream_id"`
	RootDeviceID   string           `json:"root_device_id,omitempty"`
	SourceDeviceID string           `json:"source_device_id,omitempty"`
	DeviceKind     dahua.DeviceKind `json:"device_kind,omitempty"`
	Name           string           `json:"name,omitempty"`
	Channel        int              `json:"channel,omitempty"`
	Profile        string           `json:"profile,omitempty"`
	Status         ClipStatus       `json:"status"`
	StartedAt      time.Time        `json:"started_at"`
	EndedAt        time.Time        `json:"ended_at,omitempty"`
	SourceStartAt  time.Time        `json:"source_start_at,omitempty"`
	SourceEndAt    time.Time        `json:"source_end_at,omitempty"`
	Duration       time.Duration    `json:"duration,omitempty"`
	Bytes          int64            `json:"bytes,omitempty"`
	FileName       string           `json:"file_name,omitempty"`
	Error          string           `json:"error,omitempty"`
}

type ClipStartRequest struct {
	StreamID    string
	ProfileName string
	Duration    time.Duration
}

type ClipQuery struct {
	StreamID     string
	RootDeviceID string
	Channel      int
	StartTime    time.Time
	EndTime      time.Time
	Limit        int
}

type clipJob struct {
	info         ClipInfo
	outputPath   string
	metaPath     string
	profile      streams.Profile
	parent       *Manager
	includeAudio bool
	stdin        io.WriteCloser
	cmd          *exec.Cmd
	logger       zerolog.Logger

	mu      sync.Mutex
	done    chan struct{}
	waitErr error
}

func (m *Manager) CaptureFrame(ctx context.Context, streamID string, profileName string, scaleWidth int) ([]byte, string, error) {
	if !m.Enabled() {
		return nil, "", errors.New("media layer is disabled")
	}

	entry, profile, resolvedProfileName, err := m.resolveStream(streamID, profileName)
	if err != nil {
		return nil, "", err
	}

	w, err := m.getOrCreateMJPEGWorker(entry, resolvedProfileName, profile, scaleWidth)
	if err != nil {
		return nil, "", err
	}

	ch := make(chan []byte, 1)
	w.addSubscriber(ch)
	defer func() {
		w.removeSubscriber(ch)
	}()

	if err := w.waitUntilReady(ctx); err != nil {
		return nil, "", err
	}

	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case frame, ok := <-ch:
		if !ok {
			return nil, "", errors.New("media worker stopped before emitting a frame")
		}
		return append([]byte(nil), frame...), "image/jpeg", nil
	}
}

func (m *Manager) StartClip(ctx context.Context, request ClipStartRequest) (ClipInfo, error) {
	if !m.Enabled() {
		return ClipInfo{}, errors.New("media layer is disabled")
	}

	entry, profile, resolvedProfileName, err := m.resolveStream(request.StreamID, request.ProfileName)
	if err != nil {
		return ClipInfo{}, err
	}

	clipDir := strings.TrimSpace(m.cfg.ClipPath)
	if clipDir == "" {
		return ClipInfo{}, errClipStorageMissing
	}
	if err := os.MkdirAll(clipDir, 0o755); err != nil {
		return ClipInfo{}, fmt.Errorf("create clip directory: %w", err)
	}

	duration := request.Duration
	if duration < 0 {
		duration = 0
	}
	sourceStartAt, sourceEndAt := clipSourceWindow(profile.StreamURL, duration)

	job := &clipJob{
		info: ClipInfo{
			ID:             newClipID(),
			StreamID:       entry.ID,
			RootDeviceID:   entry.RootDeviceID,
			SourceDeviceID: entry.SourceDeviceID,
			DeviceKind:     entry.DeviceKind,
			Name:           entry.Name,
			Channel:        entry.Channel,
			Profile:        resolvedProfileName,
			Status:         ClipStatusRecording,
			StartedAt:      time.Now().UTC(),
			SourceStartAt:  sourceStartAt,
			SourceEndAt:    sourceEndAt,
			Duration:       duration,
			FileName:       "",
		},
		profile: profile,
		parent:  m,
		done:    make(chan struct{}),
		logger: m.logger.With().
			Str("stream_id", entry.ID).
			Str("profile", resolvedProfileName).
			Str("format", "clip").
			Logger(),
	}
	job.info.FileName = job.info.ID + ".mp4"
	job.outputPath = filepath.Join(clipDir, job.info.FileName)
	job.metaPath = filepath.Join(clipDir, job.info.ID+".json")

	m.mu.Lock()
	for _, existing := range m.clipJobs {
		if existing.info.StreamID == entry.ID && existing.info.Status == ClipStatusRecording {
			m.mu.Unlock()
			return ClipInfo{}, fmt.Errorf("%w: %s", ErrClipAlreadyActive, entry.ID)
		}
	}
	if m.cfg.MaxWorkers > 0 && m.activeWorkerCountLocked() >= m.cfg.MaxWorkers {
		err := fmt.Errorf("%w: %d active, max %d", ErrWorkerLimitReached, m.activeWorkerCountLocked(), m.cfg.MaxWorkers)
		if m.metrics != nil {
			m.metrics.ObserveMediaStart(entry.ID, resolvedProfileName, err)
		}
		m.mu.Unlock()
		return ClipInfo{}, err
	}
	m.clipJobs[job.info.ID] = job
	m.setMediaWorkerCountLocked()
	m.logWorkerInventoryLocked("added", job.status())
	if m.metrics != nil {
		m.metrics.ObserveMediaStart(entry.ID, resolvedProfileName, nil)
	}
	m.mu.Unlock()

	if err := m.persistClip(job.info); err != nil {
		job.logger.Warn().Err(err).Msg("persist initial clip metadata failed")
	}

	started := make(chan error, 1)
	go job.run(m, profile, duration, started)

	select {
	case <-ctx.Done():
		return ClipInfo{}, ctx.Err()
	case err := <-started:
		if err != nil {
			return ClipInfo{}, err
		}
		return job.snapshot(), nil
	}
}

func (m *Manager) StopClip(ctx context.Context, clipID string) (ClipInfo, error) {
	job, info, err := m.clipJob(clipID)
	if err != nil {
		return ClipInfo{}, err
	}
	if info.Status != ClipStatusRecording {
		return info, nil
	}

	if err := job.stop(ctx); err != nil {
		return ClipInfo{}, err
	}
	return job.snapshot(), nil
}

func (m *Manager) GetClip(clipID string) (ClipInfo, error) {
	if job, info, err := m.clipJob(clipID); err == nil {
		_ = job
		return info, nil
	}
	return m.loadClip(clipID)
}

func (m *Manager) FindClips(query ClipQuery) ([]ClipInfo, error) {
	items := make([]ClipInfo, 0)

	clipDir := strings.TrimSpace(m.cfg.ClipPath)
	if clipDir == "" {
		return items, nil
	}

	entries, err := os.ReadDir(clipDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entries = nil
		} else {
			return nil, fmt.Errorf("read clip directory: %w", err)
		}
	}

	seen := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		info, err := m.loadClip(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if err != nil {
			continue
		}
		if !matchesClipQuery(info, query) {
			continue
		}
		items = append(items, info)
		seen[info.ID] = struct{}{}
	}

	m.mu.Lock()
	for _, job := range m.clipJobs {
		info := job.snapshot()
		if _, ok := seen[info.ID]; ok {
			continue
		}
		if !matchesClipQuery(info, query) {
			continue
		}
		items = append(items, info)
	}
	m.mu.Unlock()

	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if query.Limit > 0 && len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

func (m *Manager) ClipFilePath(clipID string) (string, error) {
	info, err := m.GetClip(clipID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(info.FileName) == "" {
		return "", fmt.Errorf("clip %q has no file", clipID)
	}
	return filepath.Join(strings.TrimSpace(m.cfg.ClipPath), info.FileName), nil
}

func (m *Manager) ActiveClip(streamID string) (ClipInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, job := range m.clipJobs {
		if job.info.StreamID == streamID && job.info.Status == ClipStatusRecording {
			return job.snapshot(), true
		}
	}
	return ClipInfo{}, false
}

func (m *Manager) clipJob(clipID string) (*clipJob, ClipInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.clipJobs[strings.TrimSpace(clipID)]
	if !ok {
		return nil, ClipInfo{}, ErrClipNotFound
	}
	return job, job.snapshot(), nil
}
