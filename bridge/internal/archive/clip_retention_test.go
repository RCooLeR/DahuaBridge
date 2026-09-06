package archive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
)

type retentionClipFiles struct {
	dir     string
	active  map[string]bool
	fail    map[string]bool
	deleted []string
}

func (f *retentionClipFiles) GetClip(id string) (media.ClipInfo, error) {
	status := media.ClipStatusFailed
	if f.active[id] {
		status = media.ClipStatusRecording
	}
	return media.ClipInfo{ID: id, Status: status}, nil
}
func (f *retentionClipFiles) DeleteClip(context.Context, string) error {
	return errors.New("legacy cleanup was used")
}
func (f *retentionClipFiles) DeleteArchiveClip(_ context.Context, clip media.ClipInfo) error {
	if f.fail[clip.ID] {
		return errors.New("temporary filesystem failure")
	}
	if f.active[clip.ID] {
		return errors.New("active clip was passed to deletion")
	}
	if err := os.Remove(filepath.Join(f.dir, clip.FileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f.deleted = append(f.deleted, clip.ID)
	return nil
}

func retentionService(t *testing.T, path string) *Service {
	t.Helper()
	return newReliabilityArchive(t, path, func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
		return dahua.NVRRecordingSearchResult{}, nil
	})
}

func retentionEvent(t *testing.T, s *Service, at time.Time, suffix string) (string, string) {
	t.Helper()
	item := dahua.NVRRecording{Source: "nvr_event", Channel: 1, StartTime: at.In(time.Local).Format(archiveTimeLayout), EndTime: at.Add(time.Minute).In(time.Local).Format(archiveTimeLayout), FilePath: "/event/" + suffix + ".dav", Type: "Event.smdTypeHuman", Flags: []string{"Event", "smdTypeHuman"}}
	if err := s.store.UpsertArchiveEvents(t.Context(), "nvr", []dahua.NVRRecording{item}, at); err != nil {
		t.Fatal(err)
	}
	return archiveRecordID("nvr", item)
}

func retentionClip(id string, at time.Time, status media.ClipStatus) media.ClipInfo {
	return media.ClipInfo{ID: id, StreamID: "nvr_export_" + id, FileName: id + ".mp4", Status: status, SourceStartAt: at, SourceEndAt: at.Add(time.Minute)}
}

func TestArchiveRetryCleanupPersistsUntilFilesDeleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.db")
	s := retentionService(t, path)
	at := time.Now().Add(-10 * 24 * time.Hour).Truncate(time.Second)
	id, kind := retentionEvent(t, s, at, "retry")
	failed, replacement := retentionClip("failed_a", at, media.ClipStatusFailed), retentionClip("completed_b", at, media.ClipStatusCompleted)
	for _, clip := range []media.ClipInfo{failed, replacement} {
		if err := s.store.UpsertClipAsset(t.Context(), kind, id, "nvr", "", clip); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, clip.FileName), []byte("partial or completed video"), 0600); err != nil {
			t.Fatal(err)
		}
		if clip.ID == failed.ID {
			if err := s.store.DeleteClipAsset(t.Context(), kind, id, "nvr"); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Retry writes happen now, but retention still uses the event source time.
	cutoff := at.Add(24 * time.Hour)
	ids, err := s.store.PruneOlderThan(t.Context(), cutoff)
	if err != nil || !reflect.DeepEqual(ids, []string{"completed_b", "failed_a"}) {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if err := s.store.UpsertClipAsset(t.Context(), kind, id, "nvr", "", failed); err == nil {
		t.Fatal("queued clip was relinked during deletion")
	}
	files := &retentionClipFiles{dir: dir, active: map[string]bool{}, fail: map[string]bool{failed.ID: true}}
	s.deleter, s.clipInfo = files, files
	s.deletePrunedClips(t.Context(), ids)
	if !reflect.DeepEqual(files.deleted, []string{replacement.ID}) {
		t.Fatalf("deleted %v", files.deleted)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = retentionService(t, path)
	files.fail[failed.ID] = false
	s.deleter, s.clipInfo = files, files
	ids, err = s.store.PruneOlderThan(t.Context(), cutoff)
	if err != nil || !reflect.DeepEqual(ids, []string{failed.ID}) {
		t.Fatalf("restart ids=%v err=%v", ids, err)
	}
	s.deletePrunedClips(t.Context(), ids)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM bridge_mp4_clips`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("remaining clips %d err %v", count, err)
	}
	for _, clip := range []media.ClipInfo{failed, replacement} {
		if _, err := os.Stat(filepath.Join(dir, clip.FileName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("orphan file %s: %v", clip.ID, err)
		}
	}
}

func TestArchiveCleanupProtectsActiveAndManualClips(t *testing.T) {
	s := retentionService(t, filepath.Join(t.TempDir(), "archive.db"))
	at := time.Now().Add(-10 * 24 * time.Hour).Truncate(time.Second)
	id, kind := retentionEvent(t, s, at, "active")
	active := retentionClip("active", at, media.ClipStatusRecording)
	if err := s.store.UpsertClipAsset(t.Context(), kind, id, "nvr", "", active); err != nil {
		t.Fatal(err)
	}
	if err := s.store.DeleteClipAsset(t.Context(), kind, id, "nvr"); err != nil {
		t.Fatal(err)
	}
	manual := retentionClip("manual", at, media.ClipStatusCompleted)
	manual.StreamID = "nvr_channel_01"
	if err := s.store.UpsertClipAsset(t.Context(), kind, id, "nvr", "", manual); err != nil {
		t.Fatal(err)
	}
	files := &retentionClipFiles{dir: t.TempDir(), active: map[string]bool{active.ID: true}, fail: map[string]bool{}}
	s.deleter, s.clipInfo = files, files
	ids, err := s.store.PruneOlderThan(t.Context(), at.Add(24*time.Hour))
	if err != nil || !reflect.DeepEqual(ids, []string{active.ID}) {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	s.deletePrunedClips(t.Context(), ids)
	if len(files.deleted) != 0 {
		t.Fatalf("deleted active clip: %v", files.deleted)
	}
	files.active[active.ID] = false
	s.deletePrunedClips(t.Context(), ids)
	if !reflect.DeepEqual(files.deleted, []string{active.ID}) {
		t.Fatalf("completed orphan not deleted: %v", files.deleted)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM bridge_mp4_clips WHERE clip_id = 'manual'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("manual metadata was removed: count=%d err=%v", count, err)
	}
}

func TestArchiveCleanupBatchBoundAndOldAttemptsMigration(t *testing.T) {
	s := retentionService(t, filepath.Join(t.TempDir(), "archive.db"))
	at := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	// Simulate exports orphaned by the previous schema, before tracking existed.
	for i := range 300 {
		id := fmt.Sprintf("old_%03d", i)
		if _, err := s.db.Exec(`INSERT INTO bridge_mp4_clips(clip_id,device_id,stream_id,file_path,end_time,status,created_at,updated_at) VALUES (?,'nvr',?, ?,?,'failed',?,?)`, id, "nvr_export_"+id, id+".mp4", at, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.store.InitSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	first, err := s.store.PruneOlderThan(t.Context(), time.Now())
	if err != nil || len(first) != archiveQueryLimit {
		t.Fatalf("first count=%d err=%v", len(first), err)
	}
	second, err := s.store.PruneOlderThan(t.Context(), time.Now())
	if err != nil || len(second) != archiveQueryLimit {
		t.Fatalf("second count=%d err=%v", len(second), err)
	}
	seen := map[string]bool{}
	for _, id := range first {
		seen[id] = true
	}
	for _, id := range second {
		if seen[id] {
			t.Fatal("failing oldest batch starved later cleanup")
		}
	}
}
