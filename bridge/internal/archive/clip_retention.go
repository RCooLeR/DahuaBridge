package archive

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	mediaapi "RCooLeR/DahuaBridge/internal/media"
)

// Only bridge archive exports belong to automatic archive retention. A manual
// live recording can match an event window without transferring its ownership.
func rememberArchiveClipOwnership(ctx context.Context, tx *sql.Tx, kind, recordID, deviceID string, clip mediaapi.ClipInfo) error {
	if !strings.HasPrefix(strings.TrimSpace(clip.StreamID), "nvr_export_") {
		return nil
	}
	retainFrom := clip.SourceEndAt
	if kind == "smd_ivs" {
		var eventEnd string
		err := tx.QueryRowContext(ctx, `SELECT end_time FROM smd_ivs_events WHERE device_id = ? AND event_id = ?`, deviceID, recordID).Scan(&eventEnd)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if parsed, ok := parseArchiveLocalTime(eventEnd); ok {
			retainFrom = parsed
		}
	}
	if retainFrom.IsZero() {
		retainFrom = clip.SourceStartAt
	}
	if retainFrom.IsZero() {
		retainFrom = clip.StartedAt
	}
	if retainFrom.IsZero() {
		retainFrom = time.Now()
	}
	// The first association records the source window. Retry/reconciliation
	// updates must not restart the retention clock.
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO archive_clip_retention(clip_id, retain_from) VALUES (?, ?)`,
		strings.TrimSpace(clip.ID), retainFrom.UTC().Format(time.RFC3339Nano))
	return err
}

// Acknowledge only after filesystem deletion succeeds. Keeping ownership and
// clip metadata until then makes failed deletions retryable across restarts.
func (s *SQLiteStore) finishClipCleanup(ctx context.Context, clipID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM bridge_mp4_clips WHERE clip_id = ?
		AND EXISTS (SELECT 1 FROM archive_clip_retention WHERE clip_id = ? AND queued = 1)
		AND NOT EXISTS (SELECT 1 FROM smd_ivs_events WHERE mp4_clip_id = ?)`, clipID, clipID, clipID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM archive_clip_retention WHERE clip_id = ? AND queued = 1
		AND NOT EXISTS (SELECT 1 FROM bridge_mp4_clips WHERE clip_id = ?)`, clipID, clipID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) clipForCleanup(ctx context.Context, clipID string) (mediaapi.ClipInfo, error) {
	var clip mediaapi.ClipInfo
	err := s.db.QueryRowContext(ctx, `SELECT c.clip_id, c.stream_id, c.file_path
		FROM bridge_mp4_clips c JOIN archive_clip_retention r ON c.clip_id = r.clip_id
		WHERE c.clip_id = ? AND r.queued = 1`, clipID).Scan(&clip.ID, &clip.StreamID, &clip.FileName)
	return clip, err
}
