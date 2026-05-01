package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
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
	info       ClipInfo
	outputPath string
	metaPath   string
	stdin      io.WriteCloser
	cmd        *exec.Cmd
	logger     zerolog.Logger

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
		w.stopNowIfIdle()
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
		done: make(chan struct{}),
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

func (m *Manager) persistClip(info ClipInfo) error {
	if strings.TrimSpace(m.cfg.ClipPath) == "" {
		return errClipStorageMissing
	}
	if err := os.MkdirAll(m.cfg.ClipPath, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.cfg.ClipPath, info.ID+".json"), body, 0o644)
}

func (m *Manager) loadClip(clipID string) (ClipInfo, error) {
	clipID = strings.TrimSpace(clipID)
	if clipID == "" {
		return ClipInfo{}, ErrClipNotFound
	}
	body, err := os.ReadFile(filepath.Join(strings.TrimSpace(m.cfg.ClipPath), clipID+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ClipInfo{}, ErrClipNotFound
		}
		return ClipInfo{}, err
	}
	var info ClipInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return ClipInfo{}, err
	}
	return info, nil
}

func (job *clipJob) run(parent *Manager, profile streams.Profile, duration time.Duration, started chan<- error) {
	defer close(job.done)
	defer parent.removeClipJob(job.info.ID, job)

	args := buildClipFFmpegArgs(parent.cfg, profile, duration, job.outputPath)
	job.logger.Debug().Strs("ffmpeg_args", redactFFmpegArgs(args)).Msg("starting clip worker")

	cmd := exec.Command(parent.cfg.FFmpegPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		started <- fmt.Errorf("ffmpeg stdin pipe: %w", err)
		job.complete(parent, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		started <- fmt.Errorf("ffmpeg stderr pipe: %w", err)
		job.complete(parent, err)
		return
	}
	cmd.Stdout = io.Discard

	job.mu.Lock()
	job.stdin = stdin
	job.cmd = cmd
	job.mu.Unlock()

	if err := cmd.Start(); err != nil {
		started <- fmt.Errorf("start ffmpeg: %w", err)
		job.complete(parent, err)
		return
	}
	started <- nil

	stderrText, _ := io.ReadAll(io.LimitReader(stderr, 64*1024))
	waitErr := cmd.Wait()
	if waitErr != nil && len(stderrText) > 0 {
		waitErr = fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(string(stderrText)))
	}
	job.complete(parent, waitErr)
}

func (job *clipJob) complete(parent *Manager, waitErr error) {
	job.mu.Lock()
	defer job.mu.Unlock()

	job.waitErr = waitErr
	job.info.EndedAt = time.Now().UTC()
	if waitErr != nil {
		job.info.Status = ClipStatusFailed
		job.info.Error = waitErr.Error()
	} else {
		job.info.Status = ClipStatusCompleted
		job.info.Error = ""
	}

	if stat, err := os.Stat(job.outputPath); err == nil {
		job.info.Bytes = stat.Size()
	}
	if err := parent.persistClip(job.info); err != nil {
		job.logger.Warn().Err(err).Msg("persist clip metadata failed")
	}
	if waitErr != nil {
		job.logger.Error().Err(waitErr).Msg("clip worker stopped")
	}
}

func (job *clipJob) stop(ctx context.Context) error {
	job.mu.Lock()
	stdin := job.stdin
	cmd := job.cmd
	done := job.done
	job.mu.Unlock()

	if stdin != nil {
		_, _ = io.WriteString(stdin, "q\n")
		_ = stdin.Close()
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	case <-timer.C:
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return nil
	}
}

func (job *clipJob) snapshot() ClipInfo {
	job.mu.Lock()
	defer job.mu.Unlock()
	return job.info
}

func (m *Manager) removeClipJob(id string, job *clipJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.clipJobs[id]; ok && existing == job {
		delete(m.clipJobs, id)
		m.setMediaWorkerCountLocked()
	}
}

func matchesClipQuery(info ClipInfo, query ClipQuery) bool {
	if query.StreamID != "" && info.StreamID != query.StreamID {
		return false
	}
	if query.RootDeviceID != "" && info.RootDeviceID != query.RootDeviceID {
		return false
	}
	if query.Channel > 0 && info.Channel != query.Channel {
		return false
	}
	if !query.StartTime.IsZero() {
		_, end := clipQueryWindow(info)
		if end.Before(query.StartTime) {
			return false
		}
	}
	start, _ := clipQueryWindow(info)
	if !query.EndTime.IsZero() && start.After(query.EndTime) {
		return false
	}
	return true
}

func clipQueryWindow(info ClipInfo) (time.Time, time.Time) {
	start := info.SourceStartAt
	if start.IsZero() {
		start = info.StartedAt
	}
	end := info.SourceEndAt
	if end.IsZero() {
		end = info.EndedAt
	}
	if end.IsZero() {
		end = start
	}
	return start, end
}

func clipSourceWindow(streamURL string, duration time.Duration) (time.Time, time.Time) {
	sourceURL, err := url.Parse(strings.TrimSpace(streamURL))
	if err != nil {
		return time.Time{}, time.Time{}
	}
	query := sourceURL.Query()
	startTime, err := time.Parse("2006_01_02_15_04_05", strings.TrimSpace(query.Get("starttime")))
	if err != nil {
		return time.Time{}, time.Time{}
	}
	endTime, err := time.Parse("2006_01_02_15_04_05", strings.TrimSpace(query.Get("endtime")))
	if err != nil {
		endTime = time.Time{}
	}
	if duration > 0 {
		durationEnd := startTime.Add(duration)
		if endTime.IsZero() || durationEnd.Before(endTime) {
			endTime = durationEnd
		}
	}
	return startTime.UTC(), endTime.UTC()
}

func buildClipFFmpegArgs(cfg config.MediaConfig, profile streams.Profile, duration time.Duration, outputPath string) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(cfg),
	}
	args = append(args, buildRTSPInputArgs(profile, cfg.InputPreset)...)
	if duration > 0 {
		args = append(args, "-t", formatFFmpegSeconds(duration))
	}
	args = append(args,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
		"-profile:v", "high",
		"-tag:v", "avc1",
		"-c:a", "aac",
		"-b:a", "128k",
		"-ac", "2",
		"-ar", "48000",
		"-movflags", "+faststart",
		"-y",
	)
	args = append(args, outputPath)
	return args
}

func newClipID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "clip_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return "clip_" + hex.EncodeToString(buffer)
}
