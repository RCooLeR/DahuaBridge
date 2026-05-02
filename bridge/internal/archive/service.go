package archive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

const (
	archiveQueryLimit = 128
	archiveSyncWindow = time.Hour
)

var (
	smdEventCodes = []string{"human", "vehicle", "animal"}
	ivsEventCodes = []string{"tripwire", "intrusion"}
)

type Searcher interface {
	NVRRecordings(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)
}

type ClipFinder interface {
	FindClips(mediaapi.ClipQuery) ([]mediaapi.ClipInfo, error)
	GetClip(string) (mediaapi.ClipInfo, error)
}

type Service struct {
	cfg      config.ArchiveConfig
	devices  []config.DeviceConfig
	searcher Searcher
	probes   *store.ProbeStore
	logger   zerolog.Logger

	db      *sql.DB
	cron    *cron.Cron
	store   *SQLiteStore
	trigger chan struct{}

	running int32
	started bool
	mu      sync.Mutex
}

func New(cfg config.ArchiveConfig, devices []config.DeviceConfig, searcher Searcher, probes *store.ProbeStore, logger zerolog.Logger) (*Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if searcher == nil {
		return nil, errors.New("archive searcher is required")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("create archive db directory: %w", err)
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive cache directory: %w", err)
	}
	if err := os.MkdirAll(cfg.TempDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive temp directory: %w", err)
	}

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open archive sqlite db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := NewSQLiteStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init archive sqlite schema: %w", err)
	}

	return &Service{
		cfg:      cfg,
		devices:  append([]config.DeviceConfig(nil), devices...),
		searcher: searcher,
		probes:   probes,
		logger:   logger.With().Str("component", "archive").Logger(),
		db:       db,
		store:    store,
		trigger:  make(chan struct{}, 1),
	}, nil
}

func (s *Service) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	s.cron = cron.New(cron.WithLocation(time.Local), cron.WithParser(parser))
	if _, err := s.cron.AddFunc(s.cfg.Cron, func() {
		s.QueueSync()
	}); err != nil {
		return fmt.Errorf("configure archive cron %q: %w", s.cfg.Cron, err)
	}
	s.cron.Start()
	s.started = true

	go s.runLoop(ctx)
	s.QueueSync()
	return nil
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		stopCtx := s.cron.Stop()
		select {
		case <-stopCtx.Done():
		case <-time.After(5 * time.Second):
		}
		s.cron = nil
	}
	s.started = false
	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

func (s *Service) QueueSync() {
	if s == nil {
		return
	}
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

func (s *Service) SyncNow(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		s.logger.Debug().Msg("archive sync already running")
		return nil
	}
	defer atomic.StoreInt32(&s.running, 0)

	startedAt := time.Now().UTC()
	s.logger.Info().
		Int("device_count", len(s.devices)).
		Int("prefetch_days", s.cfg.PrefetchDays).
		Int("retain_days", s.cfg.RetainDays).
		Str("cron", s.cfg.Cron).
		Msg("archive sync started")

	var firstErr error
	for _, device := range s.devices {
		if !device.EnabledValue() {
			continue
		}
		channels := s.channelsForDevice(device)
		if len(channels) == 0 {
			s.logger.Warn().Str("device_id", device.ID).Msg("archive sync skipped device with no resolved channels")
			continue
		}
		for _, channel := range channels {
			if err := s.syncChannel(ctx, device, channel); err != nil {
				s.logger.Error().Err(err).Str("device_id", device.ID).Int("channel", channel).Msg("archive sync channel failed")
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	if err := s.store.PruneOlderThan(ctx, startedAt.AddDate(0, 0, -s.cfg.RetainDays)); err != nil {
		s.logger.Error().Err(err).Msg("archive prune failed")
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}

	s.logger.Info().
		Dur("duration", time.Since(startedAt)).
		Msg("archive sync completed")
	return nil
}

func (s *Service) SearchRecordings(
	ctx context.Context,
	deviceID string,
	query dahua.NVRRecordingQuery,
	fallback func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error),
) (dahua.NVRRecordingSearchResult, error) {
	if s == nil || s.store == nil {
		return fallback(ctx, deviceID, query)
	}

	scope, covered := archiveScopeForQuery(query)
	if covered {
		windows := buildCoverageWindows(query.StartTime, query.EndTime, archiveSyncWindow)
		complete, err := s.store.IsCoverageComplete(ctx, scope, deviceID, query.Channel, windows)
		if err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Int("channel", query.Channel).Str("scope", scope).Msg("archive coverage lookup failed")
		} else if complete {
			result, err := s.store.SearchRecordings(ctx, deviceID, query)
			if err == nil {
				for index := range result.Items {
					ensureArchiveRecordIdentity(deviceID, &result.Items[index])
				}
				return result, nil
			}
			s.logger.Warn().Err(err).Str("device_id", deviceID).Int("channel", query.Channel).Str("scope", scope).Msg("archive sqlite search failed")
		}
	}

	result, err := fallback(ctx, deviceID, query)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, err
	}
	now := time.Now().UTC()
	if scope == "archive" {
		if err := s.store.UpsertArchiveFiles(ctx, deviceID, result.Items, now); err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Int("channel", query.Channel).Msg("archive file upsert after fallback failed")
		}
	} else if strings.HasPrefix(scope, "event:") {
		if err := s.store.UpsertArchiveEvents(ctx, deviceID, result.Items, now); err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Int("channel", query.Channel).Str("scope", scope).Msg("archive event upsert after fallback failed")
		}
	}
	if covered {
		for _, window := range buildCoverageWindows(query.StartTime, query.EndTime, archiveSyncWindow) {
			if err := s.store.MarkCoverage(ctx, scope, deviceID, query.Channel, window[0], window[1], now); err != nil {
				s.logger.Warn().Err(err).Str("device_id", deviceID).Int("channel", query.Channel).Str("scope", scope).Msg("archive coverage mark failed")
				break
			}
		}
	}
	for index := range result.Items {
		ensureArchiveRecordIdentity(deviceID, &result.Items[index])
	}
	return result, nil
}

func (s *Service) EnrichRecordings(ctx context.Context, deviceID string, result *dahua.NVRRecordingSearchResult, clips ClipFinder) error {
	if s == nil || result == nil {
		return nil
	}
	for index := range result.Items {
		ensureArchiveRecordIdentity(deviceID, &result.Items[index])
		if strings.TrimSpace(result.Items[index].AssetStatus) == "" {
			result.Items[index].AssetStatus = archiveAssetStateIndexed
		}
	}
	if len(result.Items) == 0 {
		return nil
	}

	storedAssets, err := s.store.LoadClipAssets(ctx, deviceID, result.Items)
	if err != nil {
		return err
	}
	for index := range result.Items {
		item := &result.Items[index]
		applyStoredArchiveAsset(item, storedAssets[archiveRecordKey(item.RecordKind, item.ID)])
	}

	if clips == nil {
		return nil
	}

	for index := range result.Items {
		item := &result.Items[index]
		clipID := strings.TrimSpace(item.AssetClipID)
		if clipID == "" {
			continue
		}
		clip, err := clips.GetClip(clipID)
		if err != nil {
			if item.AssetStatus == archiveAssetStateReady {
				item.AssetStatus = archiveAssetStateMissing
			}
			continue
		}
		applyClipArchiveAsset(item, clip)
		if err := s.store.UpsertClipAsset(ctx, item.RecordKind, item.ID, deviceID, item.FilePath, clip); err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Str("record_id", item.ID).Str("clip_id", clip.ID).Msg("archive asset upsert failed")
		}
	}

	channel := result.Channel
	if channel <= 0 {
		channel = result.Items[0].Channel
	}
	startTime := result.StartTime
	endTime := result.EndTime
	if startTime == "" {
		startTime = result.Items[len(result.Items)-1].StartTime
	}
	if endTime == "" {
		endTime = result.Items[0].EndTime
	}

	queryStart, _ := parseArchiveLocalTime(startTime)
	queryEnd, _ := parseArchiveLocalTime(endTime)
	clipItems, err := clips.FindClips(mediaapi.ClipQuery{
		RootDeviceID: strings.TrimSpace(deviceID),
		Channel:      channel,
		StartTime:    queryStart,
		EndTime:      queryEnd,
		Limit:        max(200, len(result.Items)*4),
	})
	if err != nil {
		return err
	}

	for index := range result.Items {
		item := &result.Items[index]
		match := matchClipForRecording(*item, clipItems)
		if match == nil {
			continue
		}
		applyClipArchiveAsset(item, *match)
		if err := s.store.UpsertClipAsset(ctx, item.RecordKind, item.ID, deviceID, item.FilePath, *match); err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Str("record_id", item.ID).Str("clip_id", match.ID).Msg("archive asset upsert failed")
		}
	}
	return nil
}

func (s *Service) TrackClipExport(ctx context.Context, deviceID string, request dahua.NVRPlaybackSessionRequest, clip mediaapi.ClipInfo) error {
	if s == nil || s.store == nil {
		return nil
	}
	item := dahua.NVRRecording{
		Source:      request.Source,
		Channel:     request.Channel,
		StartTime:   request.StartTime.In(time.Local).Format(archiveTimeLayout),
		EndTime:     request.EndTime.In(time.Local).Format(archiveTimeLayout),
		FilePath:    strings.TrimSpace(request.FilePath),
		Type:        strings.TrimSpace(request.Type),
		VideoStream: strings.TrimSpace(request.VideoStream),
	}
	recordID, recordKind := archiveRecordID(deviceID, item)
	if recordID == "" {
		return nil
	}
	return s.store.UpsertClipAsset(ctx, recordKind, recordID, deviceID, item.FilePath, clip)
}

func (s *Service) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.trigger:
			if err := s.SyncNow(ctx); err != nil {
				s.logger.Error().Err(err).Msg("archive sync failed")
			}
		}
	}
}

func (s *Service) syncChannel(ctx context.Context, device config.DeviceConfig, channel int) error {
	now := time.Now().In(time.Local)
	windowStart := now.AddDate(0, 0, -s.cfg.PrefetchDays).Truncate(archiveSyncWindow)
	windowEnd := now

	for from := windowStart; from.Before(windowEnd); from = from.Add(archiveSyncWindow) {
		to := from.Add(archiveSyncWindow)
		if to.After(windowEnd) {
			to = windowEnd
		}
		if err := s.syncArchiveWindow(ctx, device.ID, channel, from, to); err != nil {
			return err
		}
		if s.cfg.PrefetchSMD {
			for _, code := range smdEventCodes {
				if err := s.syncEventWindow(ctx, device.ID, channel, code, from, to); err != nil {
					return err
				}
			}
		}
		if s.cfg.PrefetchIVS {
			for _, code := range ivsEventCodes {
				if err := s.syncEventWindow(ctx, device.ID, channel, code, from, to); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Service) syncArchiveWindow(ctx context.Context, deviceID string, channel int, startTime time.Time, endTime time.Time) error {
	result, err := s.searcher.NVRRecordings(ctx, deviceID, dahua.NVRRecordingQuery{
		Channel:   channel,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     archiveQueryLimit,
	})
	if err != nil {
		return fmt.Errorf("search archive files: %w", err)
	}
	now := time.Now().UTC()
	if err := s.store.UpsertArchiveFiles(ctx, deviceID, result.Items, now); err != nil {
		return err
	}
	return s.store.MarkCoverage(ctx, "archive", deviceID, channel, startTime, endTime, now)
}

func (s *Service) syncEventWindow(ctx context.Context, deviceID string, channel int, eventCode string, startTime time.Time, endTime time.Time) error {
	result, err := s.searcher.NVRRecordings(ctx, deviceID, dahua.NVRRecordingQuery{
		Channel:   channel,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     archiveQueryLimit,
		EventCode: eventCode,
		EventOnly: true,
	})
	if err != nil {
		return fmt.Errorf("search archive events %q: %w", eventCode, err)
	}
	now := time.Now().UTC()
	if err := s.store.UpsertArchiveEvents(ctx, deviceID, result.Items, now); err != nil {
		return err
	}
	return s.store.MarkCoverage(ctx, "event:"+normalizeArchiveEventCode(eventCode), deviceID, channel, startTime, endTime, now)
}

func (s *Service) channelsForDevice(device config.DeviceConfig) []int {
	if len(device.ChannelAllowlist) > 0 {
		return normalizeChannels(device.ChannelAllowlist)
	}
	if s.probes == nil {
		return nil
	}
	probe, ok := s.probes.Get(device.ID)
	if !ok || probe == nil {
		return nil
	}
	values := make([]int, 0, len(probe.Children))
	for _, child := range probe.Children {
		if child.Kind != dahua.DeviceKindNVRChannel {
			continue
		}
		raw := strings.TrimSpace(child.Attributes["channel_index"])
		channel, err := strconv.Atoi(raw)
		if err == nil && channel > 0 {
			values = append(values, channel)
		}
	}
	return normalizeChannels(values)
}

func normalizeChannels(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func parseArchiveLocalTime(value string) (time.Time, bool) {
	parsed, err := time.ParseInLocation(archiveTimeLayout, strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func matchClipForRecording(item dahua.NVRRecording, clips []mediaapi.ClipInfo) *mediaapi.ClipInfo {
	itemStart, okStart := parseArchiveLocalTime(item.StartTime)
	itemEnd, okEnd := parseArchiveLocalTime(item.EndTime)
	bestIndex := -1
	bestScore := -1
	for index := range clips {
		clip := clips[index]
		if item.Channel > 0 && clip.Channel > 0 && clip.Channel != item.Channel {
			continue
		}
		clipStart := clip.SourceStartAt.In(time.Local)
		if clipStart.IsZero() {
			clipStart = clip.StartedAt.In(time.Local)
		}
		clipEnd := clip.SourceEndAt.In(time.Local)
		if clipEnd.IsZero() {
			clipEnd = clip.EndedAt.In(time.Local)
		}
		if clipEnd.IsZero() {
			clipEnd = clipStart
		}
		if okStart && clipEnd.Before(itemStart) {
			continue
		}
		if okEnd && clipStart.After(itemEnd) {
			continue
		}
		score := 0
		switch clip.Status {
		case mediaapi.ClipStatusCompleted:
			score += 100
		case mediaapi.ClipStatusRecording:
			score += 50
		}
		if okStart && clipStart.Equal(itemStart) {
			score += 20
		}
		if okEnd && clipEnd.Equal(itemEnd) {
			score += 20
		}
		if strings.TrimSpace(item.FilePath) != "" && strings.TrimSpace(clip.FileName) != "" {
			score += 1
		}
		if score > bestScore {
			bestScore = score
			bestIndex = index
		}
	}
	if bestIndex < 0 {
		return nil
	}
	return &clips[bestIndex]
}
