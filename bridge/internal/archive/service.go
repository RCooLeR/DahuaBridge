package archive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
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
	archiveQueryLimit             = 128
	archiveSyncWindow             = time.Hour
	archiveSMDIVSSyncInterval     = 5 * time.Minute
	archivePlaybackRTSPTimeLayout = "2006_01_02_15_04_05"
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

type ClipStatusReader interface {
	GetClip(string) (mediaapi.ClipInfo, error)
}

type ClipPrefetcher interface {
	EnsureNVRArchiveClip(context.Context, string, dahua.NVRRecording) (mediaapi.ClipInfo, error)
}

type ClipDeleter interface {
	DeleteClip(context.Context, string) error
}

type syncRequest int

const (
	syncRequestFull syncRequest = iota
	syncRequestSMDIVS
)

type Service struct {
	cfg      config.ArchiveConfig
	devices  []config.DeviceConfig
	searcher Searcher
	probes   *store.ProbeStore
	clips    ClipPrefetcher
	deleter  ClipDeleter
	clipInfo ClipStatusReader
	logger   zerolog.Logger

	db      *sql.DB
	cron    *cron.Cron
	store   *SQLiteStore
	trigger chan syncRequest

	chunkRunning  atomic.Int32
	smdIVSRunning atomic.Int32
	started       bool
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	closed        bool
	closeDone     chan struct{}
	closeErr      error
	stopParent    func() bool
}

type smdIVSSyncStats struct {
	QueryCount   int
	ReturnedRows int
	AcceptedRows int
}

type archiveChunkSyncStats struct {
	QueryCount   int
	ReturnedRows int
	AcceptedRows int
}

func (stats *smdIVSSyncStats) add(other smdIVSSyncStats) {
	stats.QueryCount += other.QueryCount
	stats.ReturnedRows += other.ReturnedRows
	stats.AcceptedRows += other.AcceptedRows
}

func (stats *archiveChunkSyncStats) add(other archiveChunkSyncStats) {
	stats.QueryCount += other.QueryCount
	stats.ReturnedRows += other.ReturnedRows
	stats.AcceptedRows += other.AcceptedRows
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
	if err := os.MkdirAll(cfg.TempDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive temp directory: %w", err)
	}

	db, err := openArchiveSQLiteDB(context.Background(), cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open archive sqlite db: %w", err)
	}
	store := NewSQLiteStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init archive sqlite schema: %w", err)
	}

	serviceCtx, cancel := context.WithCancel(context.Background())
	return &Service{
		cfg:      cfg,
		devices:  append([]config.DeviceConfig(nil), devices...),
		searcher: searcher,
		probes:   probes,
		clips:    resolveClipPrefetcher(searcher),
		deleter:  resolveClipDeleter(searcher),
		clipInfo: resolveClipStatusReader(searcher),
		logger:   logger.With().Str("component", "archive").Logger(),
		db:       db,
		store:    store,
		trigger:  make(chan syncRequest, 4),
		ctx:      serviceCtx, cancel: cancel, closeDone: make(chan struct{}),
	}, nil
}

func openArchiveSQLiteDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	// DSN pragmas are applied to every physical connection the pool opens.
	// Defensive mode prevents ordinary SQL from deliberately corrupting the
	// database file. Disabling double-quoted strings also turns misspelled
	// identifiers into errors instead of silently treating them as literals.
	dsn := dbPath + "?_defensive=1&_dqs=0&_error_rc=1&_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=1"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := configureArchiveSQLiteDB(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func configureArchiveSQLiteDB(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		if strings.TrimSpace(strings.ToLower(result)) != "ok" {
			return fmt.Errorf("sqlite quick_check failed: %s", result)
		}
	}
	return rows.Err()
}

func (s *Service) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("archive service is closed")
	}
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
	s.logger.Info().
		Str("db_path", s.cfg.DBPath).
		Str("temp_dir", s.cfg.TempDir).
		Int("device_count", len(s.devices)).
		Int("prefetch_days", s.cfg.PrefetchDays).
		Int("retain_days", s.cfg.RetainDays).
		Int("max_parallel_jobs", s.cfg.MaxParallelJobs).
		Bool("prefetch_smd", s.cfg.PrefetchSMD).
		Bool("prefetch_ivs", s.cfg.PrefetchIVS).
		Bool("export_event_mp4", s.cfg.ExportEventMP4Enabled()).
		Dur("export_delay", s.cfg.ExportDelay).
		Str("cron", s.cfg.Cron).
		Msg("archive service configured")
	s.cron.Start()
	s.started = true

	s.stopParent = context.AfterFunc(ctx, s.cancel)
	s.wg.Go(func() { s.runLoop(s.ctx) })
	s.wg.Go(func() { s.smdIVSLoop(s.ctx) })
	s.queueSMDIVSSync()
	s.QueueSync()
	return nil
}

// Close joins both scheduled jobs and manual syncs before closing SQLite.
// beginSync registers under s.mu, which also guards the transition to closed.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-s.closeDone
		return s.closeErr
	}
	s.closed = true
	s.started = false
	cronJob := s.cron
	stopParent := s.stopParent
	s.mu.Unlock()
	if stopParent != nil {
		stopParent()
	}
	s.cancel()
	if cronJob != nil {
		<-cronJob.Stop().Done()
	}
	s.wg.Wait()
	s.closeErr = s.db.Close()
	close(s.closeDone)
	return s.closeErr
}

func (s *Service) QueueSync() {
	if s == nil {
		return
	}
	select {
	case s.trigger <- syncRequestFull:
	default:
	}
}

func (s *Service) queueSMDIVSSync() {
	if s == nil {
		return
	}
	select {
	case s.trigger <- syncRequestSMDIVS:
	default:
	}
}

func (s *Service) SyncNow(ctx context.Context) error {
	if s == nil {
		return nil
	}
	ctx, finish, err := s.beginSync(ctx)
	if err != nil {
		return err
	}
	defer finish()
	if !s.chunkRunning.CompareAndSwap(0, 1) {
		s.logger.Debug().Msg("archive recording chunk sync already running")
		return nil
	}
	defer s.chunkRunning.Store(0)

	startedAt := time.Now().UTC()
	s.logger.Info().
		Int("device_count", len(s.devices)).
		Int("prefetch_days", s.cfg.PrefetchDays).
		Int("retain_days", s.cfg.RetainDays).
		Str("cron", s.cfg.Cron).
		Msg("archive recording chunk sync started")

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
		s.logger.Info().
			Str("device_id", device.ID).
			Ints("channels", channels).
			Msg("archive recording chunk device channels resolved")
		for _, channel := range channels {
			if err := s.syncChannel(ctx, device, channel); err != nil {
				s.logger.Error().Err(err).Str("device_id", device.ID).Int("channel", channel).Msg("archive sync channel failed")
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	prunedClipIDs, err := s.store.PruneOlderThan(ctx, startedAt.AddDate(0, 0, -s.cfg.RetainDays))
	if err != nil {
		s.logger.Error().Err(err).Msg("archive prune failed")
		if firstErr == nil {
			firstErr = err
		}
	} else {
		s.deletePrunedClips(ctx, prunedClipIDs)
	}
	if firstErr != nil {
		return firstErr
	}

	s.logger.Info().
		Dur("duration", time.Since(startedAt)).
		Msg("archive recording chunk sync completed")
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
		result, err := s.store.SearchRecordings(ctx, deviceID, query)
		if err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Int("channel", query.Channel).Str("scope", scope).Msg("archive sqlite search failed")
			return dahua.NVRRecordingSearchResult{}, err
		}
		for index := range result.Items {
			ensureArchiveRecordIdentity(deviceID, &result.Items[index])
		}
		return result, nil
	}

	return dahua.NVRRecordingSearchResult{
		DeviceID:  strings.TrimSpace(deviceID),
		Channel:   query.Channel,
		StartTime: query.StartTime.In(time.Local).Format(archiveTimeLayout),
		EndTime:   query.EndTime.In(time.Local).Format(archiveTimeLayout),
		Limit:     query.Limit,
		Items:     []dahua.NVRRecording{},
	}, nil
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
		if !clipMatchesRecordingWindow(*item, clip) {
			clearArchiveAsset(item)
			if err := s.store.DeleteClipAsset(ctx, item.RecordKind, item.ID, deviceID); err != nil {
				s.logger.Warn().Err(err).Str("device_id", deviceID).Str("record_id", item.ID).Str("clip_id", clip.ID).Msg("archive asset cleanup failed")
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
		if strings.TrimSpace(item.AssetClipID) != "" {
			continue
		}
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

func (s *Service) EventSummary(
	ctx context.Context,
	deviceID string,
	startTime time.Time,
	endTime time.Time,
	eventCode string,
) (dahua.NVREventSummary, error) {
	summary := dahua.NVREventSummary{
		DeviceID:  strings.TrimSpace(deviceID),
		StartTime: startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
		Items:     []dahua.NVREventSummaryItem{},
		Channels:  []dahua.NVREventChannelSummary{},
	}
	if s == nil || s.store == nil {
		return summary, errors.New("archive store is not configured")
	}

	countsByChannel, err := s.store.LoadEventSummaryCounts(ctx, deviceID, startTime, endTime, eventCode)
	if err != nil {
		return summary, err
	}

	channels := s.channelsForSummary(deviceID, countsByChannel)
	totalByCode := make(map[string]int)
	for _, channel := range channels {
		items := makeNVREventSummaryItems(countsByChannel[channel])
		summary.Channels = append(summary.Channels, dahua.NVREventChannelSummary{
			Channel:    channel,
			TotalCount: countNVREventSummaryItems(items),
			Items:      items,
		})
		for _, item := range items {
			totalByCode[item.Code] += item.Count
		}
	}
	summary.Items = makeNVREventSummaryItems(totalByCode)
	summary.TotalCount = countNVREventSummaryItems(summary.Items)
	return summary, nil
}

func (s *Service) ArchiveCoverage(
	ctx context.Context,
	deviceID string,
	channel int,
) (dahua.NVRArchiveCoverage, error) {
	coverage := dahua.NVRArchiveCoverage{
		DeviceID: strings.TrimSpace(deviceID),
		Channel:  channel,
		Chunks:   []dahua.NVRArchiveCoverageChunk{},
	}
	if s == nil || s.store == nil {
		return coverage, errors.New("archive store is not configured")
	}

	chunks, err := s.store.LoadArchiveCoverage(ctx, deviceID, channel)
	if err != nil {
		return coverage, err
	}
	coverage.ChunkCount = len(chunks)
	if first, last, ok := firstAndLastCoverageChunk(chunks); ok {
		coverage.StartTime = first.StartTime.UTC().Format(time.RFC3339)
		coverage.EndTime = last.EndTime.UTC().Format(time.RFC3339)
	}
	for _, chunk := range chunks {
		coverage.Chunks = append(coverage.Chunks, dahua.NVRArchiveCoverageChunk{
			StartTime: chunk.StartTime.UTC().Format(time.RFC3339),
			EndTime:   chunk.EndTime.UTC().Format(time.RFC3339),
		})
	}
	return coverage, nil
}

func (s *Service) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-s.trigger:
			switch request {
			case syncRequestSMDIVS:
				s.wg.Go(func() { s.runSMDIVSSync(ctx) })
			default:
				s.wg.Go(func() { s.runChunkSync(ctx) })
			}
		}
	}
}

func (s *Service) runChunkSync(ctx context.Context) {
	if err := s.SyncNow(ctx); err != nil {
		s.logger.Error().Err(err).Msg("archive sync failed")
	}
}

func (s *Service) runSMDIVSSync(ctx context.Context) {
	if err := s.SyncRecentEventsNow(ctx); err != nil {
		s.logger.Error().Err(err).Msg("archive smd_ivs sync failed")
	}
}

func (s *Service) smdIVSLoop(ctx context.Context) {
	ticker := time.NewTicker(archiveSMDIVSSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.queueSMDIVSSync()
		}
	}
}

func (s *Service) SyncRecentEventsNow(ctx context.Context) error {
	return s.SyncSMDIVSNow(ctx)
}

func (s *Service) SyncSMDIVSNow(ctx context.Context) error {
	if s == nil {
		return nil
	}
	ctx, finish, err := s.beginSync(ctx)
	if err != nil {
		return err
	}
	defer finish()
	if !s.smdIVSRunning.CompareAndSwap(0, 1) {
		s.logger.Debug().Msg("archive smd_ivs sync already running")
		return nil
	}
	defer s.smdIVSRunning.Store(0)

	if !s.cfg.PrefetchSMD && !s.cfg.PrefetchIVS {
		return nil
	}

	startedAt := time.Now().UTC()
	s.logger.Info().
		Int("device_count", len(s.devices)).
		Int("prefetch_days", s.cfg.PrefetchDays).
		Int("max_parallel_jobs", s.cfg.MaxParallelJobs).
		Bool("prefetch_smd", s.cfg.PrefetchSMD).
		Bool("prefetch_ivs", s.cfg.PrefetchIVS).
		Bool("export_event_mp4", s.cfg.ExportEventMP4Enabled()).
		Dur("export_delay", s.cfg.ExportDelay).
		Str("db_path", s.cfg.DBPath).
		Msg("archive smd_ivs sync started")

	var firstErr error
	enabledDevices := 0
	scannedChannels := 0
	for _, device := range s.devices {
		if !device.EnabledValue() {
			s.logger.Debug().Str("device_id", device.ID).Msg("archive smd_ivs sync skipped disabled device")
			continue
		}
		enabledDevices++
		channels := s.channelsForDevice(device)
		if len(channels) == 0 {
			s.logger.Warn().
				Str("device_id", device.ID).
				Int("allowlist_count", len(device.ChannelAllowlist)).
				Bool("probe_store_configured", s.probes != nil).
				Msg("archive smd_ivs sync skipped device with no resolved channels")
			continue
		}
		scannedChannels += len(channels)
		s.logger.Info().
			Str("device_id", device.ID).
			Ints("channels", channels).
			Msg("archive smd_ivs device channels resolved")
		for _, channel := range channels {
			if err := s.syncSMDIVSChannel(ctx, device.ID, channel); err != nil {
				s.logger.Error().Err(err).Str("device_id", device.ID).Int("channel", channel).Msg("archive smd_ivs sync failed")
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	if err := s.prefetchPendingEventAssets(ctx); err != nil {
		s.logger.Error().Err(err).Msg("archive smd_ivs pending mp4 prefetch failed")
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		s.logger.Info().
			Int("enabled_device_count", enabledDevices).
			Int("channel_count", scannedChannels).
			Dur("duration", time.Since(startedAt)).
			Msg("archive smd_ivs sync completed")
	}
	return firstErr
}

func (s *Service) syncChannel(ctx context.Context, device config.DeviceConfig, channel int) error {
	now := time.Now().In(time.Local)
	windowStart := now.AddDate(0, 0, -s.cfg.PrefetchDays).Truncate(archiveSyncWindow)
	windowEnd := now
	startedAt := time.Now()
	windowCount := 0
	stats := archiveChunkSyncStats{}

	err := forArchiveSyncWindows(windowStart, windowEnd, func(from, to time.Time) error {
		windowCount++
		windowStats, err := s.syncArchiveWindow(ctx, device.ID, channel, from, to)
		stats.add(windowStats)
		return err
	})
	if err != nil {
		return err
	}
	s.logger.Info().
		Str("device_id", device.ID).
		Int("channel", channel).
		Str("window_start", windowStart.In(time.Local).Format(archiveTimeLayout)).
		Str("window_end", windowEnd.In(time.Local).Format(archiveTimeLayout)).
		Int("window_count", windowCount).
		Int("query_count", stats.QueryCount).
		Int("returned_rows", stats.ReturnedRows).
		Int("accepted_rows", stats.AcceptedRows).
		Dur("duration", time.Since(startedAt)).
		Msg("archive recording chunk channel sync completed")
	return nil
}

func (s *Service) syncArchiveWindow(ctx context.Context, deviceID string, channel int, startTime time.Time, endTime time.Time) (archiveChunkSyncStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	stats := archiveChunkSyncStats{QueryCount: 1}
	result, err := s.searcher.NVRRecordings(ctx, deviceID, dahua.NVRRecordingQuery{
		Channel:   channel,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     archiveQueryLimit,
		ScanAll:   true,
	})
	if err != nil {
		return stats, fmt.Errorf("search archive files: %w", err)
	}
	stats.ReturnedRows = len(result.Items)
	stats.AcceptedRows = countArchiveChunkRecordings(result.Items)
	now := time.Now().UTC()
	if err := s.store.UpsertArchiveFiles(ctx, deviceID, result.Items, now); err != nil {
		return stats, err
	}
	if stats.ReturnedRows > 0 || stats.AcceptedRows > 0 {
		s.logger.Info().
			Str("device_id", deviceID).
			Int("channel", channel).
			Str("window_start", startTime.In(time.Local).Format(archiveTimeLayout)).
			Str("window_end", endTime.In(time.Local).Format(archiveTimeLayout)).
			Int("returned_rows", stats.ReturnedRows).
			Int("accepted_rows", stats.AcceptedRows).
			Msg("archive recording chunk query stored rows")
	} else {
		s.logger.Debug().
			Str("device_id", deviceID).
			Int("channel", channel).
			Str("window_start", startTime.In(time.Local).Format(archiveTimeLayout)).
			Str("window_end", endTime.In(time.Local).Format(archiveTimeLayout)).
			Msg("archive recording chunk query returned no rows")
	}
	return stats, nil
}

func (s *Service) syncEventWindow(ctx context.Context, deviceID string, channel int, eventCode string, startTime time.Time, endTime time.Time) (smdIVSSyncStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	stats := smdIVSSyncStats{QueryCount: 1}
	result, err := s.searcher.NVRRecordings(ctx, deviceID, dahua.NVRRecordingQuery{
		Channel:   channel,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     archiveQueryLimit,
		ScanAll:   true,
		EventCode: eventCode,
		EventOnly: true,
	})
	if err != nil {
		return stats, fmt.Errorf("search archive events %q: %w", eventCode, err)
	}
	stats.ReturnedRows = len(result.Items)
	stats.AcceptedRows = countSMDIVSRecordings(result.Items)
	now := time.Now().UTC()
	s.populateArchiveEventRTSPURLs(deviceID, result.Items)
	if err := s.store.UpsertArchiveEvents(ctx, deviceID, result.Items, now); err != nil {
		return stats, err
	}
	if err := s.prefetchEventAssets(ctx, deviceID, result.Items); err != nil {
		return stats, err
	}
	if stats.ReturnedRows > 0 || stats.AcceptedRows > 0 {
		s.logger.Info().
			Str("device_id", deviceID).
			Int("channel", channel).
			Str("event_code", eventCode).
			Str("window_start", startTime.In(time.Local).Format(archiveTimeLayout)).
			Str("window_end", endTime.In(time.Local).Format(archiveTimeLayout)).
			Int("returned_rows", stats.ReturnedRows).
			Int("accepted_rows", stats.AcceptedRows).
			Msg("archive smd_ivs event query stored rows")
	} else {
		s.logger.Debug().
			Str("device_id", deviceID).
			Int("channel", channel).
			Str("event_code", eventCode).
			Str("window_start", startTime.In(time.Local).Format(archiveTimeLayout)).
			Str("window_end", endTime.In(time.Local).Format(archiveTimeLayout)).
			Msg("archive smd_ivs event query returned no rows")
	}
	return stats, nil
}

func (s *Service) syncSMDIVSChannel(ctx context.Context, deviceID string, channel int) error {
	startedAt := time.Now()
	codes := make([]string, 0, 5)
	if s.cfg.PrefetchSMD {
		codes = append(codes, smdEventCodes...)
	}
	if s.cfg.PrefetchIVS {
		codes = append(codes, ivsEventCodes...)
	}
	var firstErr error
	for _, code := range codes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.syncIncrementalEventCode(ctx, deviceID, channel, code, startedAt); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.logger.Info().
		Str("device_id", deviceID).
		Int("channel", channel).
		Dur("duration", time.Since(startedAt)).
		Msg("archive incremental smd_ivs channel sync completed")
	return firstErr
}

func forArchiveSyncWindows(windowStart time.Time, windowEnd time.Time, fn func(time.Time, time.Time) error) error {
	if !windowEnd.After(windowStart) {
		return nil
	}
	for to := windowEnd; to.After(windowStart); {
		from := to.Add(-archiveSyncWindow)
		if from.Before(windowStart) {
			from = windowStart
		}
		if err := fn(from, to); err != nil {
			return err
		}
		to = from
	}
	return nil
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

func (s *Service) channelsForSummary(deviceID string, countsByChannel map[int]map[string]int) []int {
	for _, device := range s.devices {
		if device.ID != deviceID {
			continue
		}
		channels := s.channelsForDevice(device)
		if len(channels) > 0 {
			return channels
		}
		break
	}
	return normalizeChannels(archiveCountMapKeys(countsByChannel))
}

func makeNVREventSummaryItems(countByCode map[string]int) []dahua.NVREventSummaryItem {
	if len(countByCode) == 0 {
		return []dahua.NVREventSummaryItem{}
	}
	items := make([]dahua.NVREventSummaryItem, 0, len(countByCode))
	for _, code := range []string{"human", "vehicle", "animal", "tripwire", "intrusion"} {
		count := countByCode[code]
		if count <= 0 {
			continue
		}
		items = append(items, dahua.NVREventSummaryItem{
			Code:  code,
			Label: archiveEventSummaryLabel(code),
			Count: count,
		})
	}
	return items
}

func countNVREventSummaryItems(items []dahua.NVREventSummaryItem) int {
	total := 0
	for _, item := range items {
		total += item.Count
	}
	return total
}

func archiveEventSummaryLabel(code string) string {
	switch normalizeArchiveEventCode(code) {
	case "human":
		return "Human"
	case "vehicle":
		return "Vehicle"
	case "animal":
		return "Animal"
	case "tripwire":
		return "Cross Line"
	case "intrusion":
		return "Cross Region"
	default:
		return strings.TrimSpace(code)
	}
}

func resolveClipPrefetcher(searcher Searcher) ClipPrefetcher {
	prefetcher, ok := searcher.(ClipPrefetcher)
	if !ok {
		return nil
	}
	return prefetcher
}

func resolveClipDeleter(searcher Searcher) ClipDeleter {
	deleter, ok := searcher.(ClipDeleter)
	if !ok {
		return nil
	}
	return deleter
}

func resolveClipStatusReader(searcher Searcher) ClipStatusReader {
	reader, ok := searcher.(ClipStatusReader)
	if !ok {
		return nil
	}
	return reader
}

func (s *Service) deletePrunedClips(ctx context.Context, clipIDs []string) {
	if s == nil || s.deleter == nil || len(clipIDs) == 0 {
		return
	}
	for _, clipID := range clipIDs {
		clipID = strings.TrimSpace(clipID)
		if clipID == "" {
			continue
		}
		if s.clipInfo != nil {
			clip, err := s.clipInfo.GetClip(clipID)
			if err == nil && isActiveArchiveAssetState(string(clip.Status)) {
				continue
			}
			if err != nil && !errors.Is(err, mediaapi.ErrClipNotFound) && !errors.Is(err, os.ErrNotExist) {
				s.logger.Warn().Err(err).Str("clip_id", clipID).Msg("archive pruned clip status unavailable; cleanup will retry")
				continue
			}
		}
		var deleteErr error
		if deleter, ok := s.deleter.(interface {
			DeleteArchiveClip(context.Context, mediaapi.ClipInfo) error
		}); ok {
			clip, err := s.store.clipForCleanup(ctx, clipID)
			if err == nil {
				deleteErr = deleter.DeleteArchiveClip(ctx, clip)
			} else if !errors.Is(err, sql.ErrNoRows) {
				deleteErr = err
			}
		} else {
			deleteErr = s.deleter.DeleteClip(ctx, clipID)
		}
		if deleteErr != nil && !errors.Is(deleteErr, mediaapi.ErrClipNotFound) && !errors.Is(deleteErr, os.ErrNotExist) {
			s.logger.Warn().Err(deleteErr).Str("clip_id", clipID).Msg("archive pruned clip cleanup failed")
			continue
		}
		if err := s.store.finishClipCleanup(ctx, clipID); err != nil {
			s.logger.Warn().Err(err).Str("clip_id", clipID).Msg("archive pruned clip cleanup acknowledgement failed")
		}
	}
}

func parseArchiveLocalTime(value string) (time.Time, bool) {
	parsed, err := time.ParseInLocation(archiveTimeLayout, strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func (s *Service) prefetchEventAssets(ctx context.Context, deviceID string, items []dahua.NVRRecording) error {
	if s == nil || s.store == nil || s.clips == nil || len(items) == 0 {
		return nil
	}
	if !s.cfg.ExportEventMP4Enabled() {
		return nil
	}

	// Complete recorder scans can contain many pages. Export only a bounded
	// candidate batch here; remaining rows stay eligible for the pending queue.
	candidateLimit := max(archiveQueryLimit, max(1, s.cfg.MaxParallelJobs)*4)
	pending := make([]dahua.NVRRecording, 0, min(len(items), candidateLimit))
	for _, item := range items {
		ensureArchiveRecordIdentity(deviceID, &item)
		if !s.shouldPrefetchArchiveEventClip(item) {
			continue
		}
		pending = append(pending, item)
		if len(pending) >= candidateLimit {
			break
		}
	}
	if len(pending) == 0 {
		return nil
	}

	if err := s.reconcileClipAssets(ctx); err != nil {
		s.logger.Warn().Err(err).Msg("archive event active clip refresh failed")
	}
	storedAssets, err := s.store.LoadClipAssets(ctx, deviceID, pending)
	if err != nil {
		return err
	}
	activeJobs, err := s.store.CountActiveClipJobs(ctx)
	if err != nil {
		return err
	}
	maxActiveJobs := max(0, s.cfg.MaxParallelJobs)

	for _, item := range pending {
		if maxActiveJobs > 0 && activeJobs >= maxActiveJobs {
			break
		}
		stored := storedAssets[archiveRecordKey(item.RecordKind, item.ID)]
		status := normalizeArchiveAssetState(stored.Status)
		if stored.ClipID != "" {
			switch status {
			case archiveAssetStateReady, archiveAssetStateQueued, archiveAssetStateDownloading, archiveAssetStateTranscoding:
				continue
			}
		}
		clip, err := s.clips.EnsureNVRArchiveClip(ctx, deviceID, item)
		if err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Str("record_id", item.ID).Str("file_path", item.FilePath).Msg("archive event clip prefetch failed")
			continue
		}
		if err := s.store.UpsertClipAsset(ctx, item.RecordKind, item.ID, deviceID, item.FilePath, clip); err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Str("record_id", item.ID).Str("clip_id", clip.ID).Msg("archive prefetched asset upsert failed")
			continue
		}
		if isActiveArchiveAssetState(string(clip.Status)) {
			activeJobs++
		}
	}
	return nil
}

func (s *Service) prefetchPendingEventAssets(ctx context.Context) error {
	if s == nil || s.store == nil || s.clips == nil {
		return nil
	}
	if !s.cfg.ExportEventMP4Enabled() {
		return nil
	}
	if err := s.reconcileClipAssets(ctx); err != nil {
		s.logger.Warn().Err(err).Msg("archive event active clip refresh failed")
	}
	activeJobs, err := s.store.CountActiveClipJobs(ctx)
	if err != nil {
		return err
	}
	maxActiveJobs := max(0, s.cfg.MaxParallelJobs)
	limit := archiveQueryLimit
	if maxActiveJobs > 0 {
		if activeJobs >= maxActiveJobs {
			s.logger.Info().
				Int("active_jobs", activeJobs).
				Int("max_parallel_jobs", maxActiveJobs).
				Msg("archive smd_ivs pending mp4 prefetch waiting for active jobs")
			return nil
		}
		limit = maxActiveJobs - activeJobs
	}
	cutoff := time.Now().In(time.Local).AddDate(0, 0, -s.cfg.PrefetchDays)
	readyBefore := time.Now().In(time.Local).Add(-s.cfg.ExportDelay)
	candidates, err := s.store.LoadPendingEventClipCandidates(ctx, cutoff, readyBefore, limit, archiveEventVideoExportAllowlists(s.cfg))
	if err != nil {
		return err
	}
	s.logger.Info().
		Int("active_jobs", activeJobs).
		Int("max_parallel_jobs", maxActiveJobs).
		Int("candidate_limit", limit).
		Int("candidate_count", len(candidates)).
		Str("cutoff", cutoff.Format(archiveTimeLayout)).
		Str("ready_before", readyBefore.Format(archiveTimeLayout)).
		Dur("export_delay", s.cfg.ExportDelay).
		Msg("archive smd_ivs pending mp4 candidates loaded")
	if len(candidates) == 0 {
		return nil
	}

	itemsByDevice := make(map[string][]dahua.NVRRecording)
	deviceOrder := make([]string, 0)
	for _, candidate := range candidates {
		deviceID := strings.TrimSpace(candidate.DeviceID)
		if deviceID == "" {
			continue
		}
		if _, ok := itemsByDevice[deviceID]; !ok {
			deviceOrder = append(deviceOrder, deviceID)
		}
		itemsByDevice[deviceID] = append(itemsByDevice[deviceID], candidate.Item)
	}
	for _, deviceID := range deviceOrder {
		if err := s.prefetchEventAssets(ctx, deviceID, itemsByDevice[deviceID]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) reconcileClipAssets(ctx context.Context) error {
	if s == nil || s.store == nil || s.clipInfo == nil {
		return nil
	}
	limit := max(archiveQueryLimit, max(1, s.cfg.MaxParallelJobs)*4)
	assets, err := s.store.LoadEventClipAssetsForReconciliation(ctx, limit)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		clip, err := s.clipInfo.GetClip(asset.ClipID)
		if err != nil {
			if deleteErr := s.store.DeleteClipAsset(ctx, asset.RecordKind, asset.RecordID, asset.DeviceID); deleteErr != nil {
				s.logger.Warn().Err(deleteErr).Str("device_id", asset.DeviceID).Str("record_id", asset.RecordID).Str("clip_id", asset.ClipID).Msg("archive stale active clip cleanup failed")
			}
			continue
		}
		if strings.TrimSpace(clip.ID) == "" {
			continue
		}
		if err := s.store.UpsertClipAsset(ctx, asset.RecordKind, asset.RecordID, asset.DeviceID, asset.SourceFilePath, clip); err != nil {
			s.logger.Warn().Err(err).Str("device_id", asset.DeviceID).Str("record_id", asset.RecordID).Str("clip_id", asset.ClipID).Msg("archive active clip refresh upsert failed")
		}
	}
	return nil
}

func countSMDIVSRecordings(items []dahua.NVRRecording) int {
	count := 0
	for _, item := range items {
		if isSMDIVSRecording(item) {
			count++
		}
	}
	return count
}

func countArchiveChunkRecordings(items []dahua.NVRRecording) int {
	count := 0
	for _, item := range items {
		if shouldTreatAsEvent(item) {
			continue
		}
		if strings.TrimSpace(item.FilePath) == "" || item.Channel <= 0 {
			continue
		}
		if strings.TrimSpace(item.StartTime) == "" || strings.TrimSpace(item.EndTime) == "" {
			continue
		}
		count++
	}
	return count
}

func shouldPrefetchArchiveEventClip(item dahua.NVRRecording) bool {
	if normalizeArchiveRecordKind(item.RecordKind) != "smd_ivs" {
		return false
	}
	if item.Channel <= 0 {
		return false
	}
	startTime, okStart := parseArchiveLocalTime(item.StartTime)
	endTime, okEnd := parseArchiveLocalTime(item.EndTime)
	return okStart && okEnd && endTime.After(startTime)
}

func (s *Service) shouldPrefetchArchiveEventClip(item dahua.NVRRecording) bool {
	if !shouldPrefetchArchiveEventClip(item) {
		return false
	}
	if !archiveEventVideoExportDelayElapsed(item, time.Now(), s.cfg.ExportDelay) {
		return false
	}
	return archiveEventVideoChannelAllowed(archiveEventVideoExportAllowlists(s.cfg), item)
}

func archiveEventVideoChannelAllowed(allowlists map[string][]int, item dahua.NVRRecording) bool {
	if len(allowlists) == 0 {
		return true
	}
	code := archiveEventCodeForRecording(item)
	if code != "" {
		if channels, ok := allowlists[code]; ok {
			return slices.Contains(channels, item.Channel)
		}
	}
	if channels, ok := allowlists["all"]; ok {
		return slices.Contains(channels, item.Channel)
	}
	return true
}

func archiveEventVideoExportAllowlists(cfg config.ArchiveConfig) map[string][]int {
	allowlists := make(map[string][]int)
	if len(cfg.ExportSMDPerson) > 0 {
		allowlists["human"] = normalizeArchiveEventVideoChannels(cfg.ExportSMDPerson)
	}
	if len(cfg.ExportSMDTransport) > 0 {
		allowlists["vehicle"] = normalizeArchiveEventVideoChannels(cfg.ExportSMDTransport)
	}
	if len(cfg.ExportSMDAnimal) > 0 {
		allowlists["animal"] = normalizeArchiveEventVideoChannels(cfg.ExportSMDAnimal)
	}
	if len(cfg.ExportIVS) > 0 {
		channels := normalizeArchiveEventVideoChannels(cfg.ExportIVS)
		allowlists["tripwire"] = channels
		allowlists["intrusion"] = channels
	}
	if len(allowlists) == 0 {
		return nil
	}
	return allowlists
}

func archiveEventVideoExportDelayElapsed(item dahua.NVRRecording, now time.Time, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	endTime, ok := parseArchiveLocalTime(item.EndTime)
	if !ok {
		return false
	}
	readyAt := endTime.Add(delay)
	return !readyAt.After(now.In(time.Local))
}

func normalizeArchiveEventVideoChannels(channels []int) []int {
	if len(channels) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(channels))
	normalized := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel <= 0 {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		normalized = append(normalized, channel)
	}
	slices.Sort(normalized)
	return normalized
}

func archiveEventCodeForRecording(item dahua.NVRRecording) string {
	if code := normalizeArchiveEventCode(item.Type); code != "" {
		return code
	}
	for _, flag := range item.Flags {
		if code := normalizeArchiveEventCode(flag); code != "" {
			return code
		}
	}
	return ""
}

func (s *Service) populateArchiveEventRTSPURLs(deviceID string, items []dahua.NVRRecording) {
	if len(items) == 0 {
		return
	}
	device, ok := s.deviceConfig(deviceID)
	if !ok {
		return
	}
	for index := range items {
		item := &items[index]
		startTime, okStart := parseArchiveLocalTime(item.StartTime)
		endTime, okEnd := parseArchiveLocalTime(item.EndTime)
		if item.Channel <= 0 || !okStart || !okEnd || !endTime.After(startTime) {
			continue
		}
		item.RTSPMainURL = buildArchiveEventRTSPURL(device, item.Channel, 0, startTime, endTime, true)
		item.RTSPSubURL = buildArchiveEventRTSPURL(device, item.Channel, 1, startTime, endTime, true)
	}
}

func (s *Service) deviceConfig(deviceID string) (config.DeviceConfig, bool) {
	deviceID = strings.TrimSpace(deviceID)
	for _, device := range s.devices {
		if strings.TrimSpace(device.ID) == deviceID {
			return device, true
		}
	}
	return config.DeviceConfig{}, false
}

func buildArchiveEventRTSPURL(device config.DeviceConfig, channel int, subtype int, startTime time.Time, endTime time.Time, includeCredentials bool) string {
	base, err := url.Parse(device.BaseURL)
	if err != nil || base.Hostname() == "" {
		return ""
	}

	host := base.Hostname()
	if port := base.Port(); port != "" && port != "80" && port != "443" {
		host = net.JoinHostPort(host, port)
	} else {
		host = net.JoinHostPort(host, "554")
	}

	rtspURL := &url.URL{
		Scheme:   "rtsp",
		Host:     host,
		Path:     "/cam/playback",
		RawQuery: buildArchiveEventRTSPQuery(channel, subtype, startTime.In(time.Local), endTime.In(time.Local)),
	}
	if includeCredentials {
		rtspURL.User = url.UserPassword(device.Username, device.Password)
	}
	return rtspURL.String()
}

func buildArchiveEventRTSPQuery(channel int, subtype int, startTime time.Time, endTime time.Time) string {
	parts := []string{
		"channel=" + url.QueryEscape(strconv.Itoa(channel)),
		"subtype=" + url.QueryEscape(strconv.Itoa(subtype)),
		"starttime=" + url.QueryEscape(startTime.Format(archivePlaybackRTSPTimeLayout)),
	}
	if !endTime.IsZero() {
		parts = append(parts, "endtime="+url.QueryEscape(endTime.Format(archivePlaybackRTSPTimeLayout)))
	}
	return strings.Join(parts, "&")
}

func matchClipForRecording(item dahua.NVRRecording, clips []mediaapi.ClipInfo) *mediaapi.ClipInfo {
	bestIndex := -1
	bestScore := -1
	for index := range clips {
		clip := clips[index]
		if item.Channel > 0 && clip.Channel > 0 && clip.Channel != item.Channel {
			continue
		}
		if !clipMatchesRecordingWindow(item, clip) {
			continue
		}
		score := 0
		switch clip.Status {
		case mediaapi.ClipStatusCompleted:
			score += 100
		case mediaapi.ClipStatusRecording:
			score += 50
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

func clipMatchesRecordingWindow(item dahua.NVRRecording, clip mediaapi.ClipInfo) bool {
	itemStart, okStart := parseArchiveLocalTime(item.StartTime)
	itemEnd, okEnd := parseArchiveLocalTime(item.EndTime)
	if !okStart || !okEnd || !itemEnd.After(itemStart) {
		return false
	}

	clipStart := clip.SourceStartAt.In(time.Local)
	clipEnd := clip.SourceEndAt.In(time.Local)
	if clipStart.IsZero() || clipEnd.IsZero() || !clipEnd.After(clipStart) {
		clipStart = clip.StartedAt.In(time.Local)
		clipEnd = clip.EndedAt.In(time.Local)
		if clipStart.IsZero() || clipEnd.IsZero() || !clipEnd.After(clipStart) {
			return false
		}
	}

	const tolerance = 2 * time.Second
	return timesApproxEqual(clipStart, itemStart, tolerance) &&
		timesApproxEqual(clipEnd, itemEnd, tolerance)
}

func timesApproxEqual(left time.Time, right time.Time, tolerance time.Duration) bool {
	if left.IsZero() || right.IsZero() {
		return false
	}
	delta := left.Sub(right)
	if delta < 0 {
		delta = -delta
	}
	return delta <= tolerance
}
