package archive

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const archiveEventOverlap = 15 * time.Minute
const archiveBackfillWindowsPerPass = 2

type eventScanProgress struct{ recentEnd, backfillBefore time.Time }

func (s *SQLiteStore) loadEventScanProgress(ctx context.Context, deviceID string, channel int, code string) (eventScanProgress, error) {
	var recent, before string
	err := s.db.QueryRowContext(ctx, `SELECT recent_end, backfill_before FROM archive_event_scan_progress WHERE device_id=? AND channel=? AND event_code=?`, deviceID, channel, code).Scan(&recent, &before)
	if errors.Is(err, sql.ErrNoRows) {
		return eventScanProgress{}, nil
	}
	if err != nil {
		return eventScanProgress{}, err
	}
	end, err := time.Parse(time.RFC3339Nano, recent)
	if err != nil {
		return eventScanProgress{}, err
	}
	backfill, err := time.Parse(time.RFC3339Nano, before)
	return eventScanProgress{end, backfill}, err
}

func (s *SQLiteStore) saveEventScanProgress(ctx context.Context, deviceID string, channel int, code string, state eventScanProgress) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO archive_event_scan_progress(device_id,channel,event_code,recent_end,backfill_before) VALUES(?,?,?,?,?)
		ON CONFLICT(device_id,channel,event_code) DO UPDATE SET recent_end=excluded.recent_end,backfill_before=excluded.backfill_before`,
		deviceID, channel, code, state.recentEnd.UTC().Format(time.RFC3339Nano), state.backfillBefore.UTC().Format(time.RFC3339Nano))
	return err
}

// Recent events always run first. Historical work is limited per pass, with
// each successful window checkpointed only after its rows were committed.
// A crash between the row commit and checkpoint write repeats an idempotent
// upsert on restart; it cannot skip uncommitted events.
func (s *Service) syncIncrementalEventCode(ctx context.Context, deviceID string, channel int, code string, now time.Time) error {
	state, err := s.store.loadEventScanProgress(ctx, deviceID, channel, code)
	if err != nil {
		return err
	}
	earliest := now.In(time.Local).AddDate(0, 0, -s.cfg.PrefetchDays).Truncate(archiveSyncWindow)
	from := state.recentEnd.Add(-archiveEventOverlap)
	if state.recentEnd.IsZero() || state.recentEnd.After(now) {
		from = now.Add(-archiveEventOverlap)
		state.backfillBefore = from
	} else if from.Before(now.Add(-archiveSyncWindow)) {
		// A long outage leaves a gap; backfill it instead of blocking the
		// newest events behind all missed history.
		from = now.Add(-archiveSyncWindow)
		state.backfillBefore = from
	}
	if from.Before(earliest) {
		from = earliest
	}
	if _, err := s.syncEventWindow(ctx, deviceID, channel, code, from, now); err != nil {
		return err
	}
	state.recentEnd = now
	if state.backfillBefore.IsZero() || state.backfillBefore.Before(earliest) {
		state.backfillBefore = earliest
	}
	if err := s.store.saveEventScanProgress(ctx, deviceID, channel, code, state); err != nil {
		return err
	}
	for range archiveBackfillWindowsPerPass {
		if !state.backfillBefore.After(earliest) {
			break
		}
		from := state.backfillBefore.Add(-archiveSyncWindow)
		if from.Before(earliest) {
			from = earliest
		}
		if _, err := s.syncEventWindow(ctx, deviceID, channel, code, from, state.backfillBefore); err != nil {
			return err
		}
		state.backfillBefore = from
		if err := s.store.saveEventScanProgress(ctx, deviceID, channel, code, state); err != nil {
			return err
		}
	}
	return nil
}
