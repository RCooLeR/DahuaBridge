package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// DeleteArchiveClip removes a retention-owned export even if its JSON metadata
// was lost. The archive database supplies the original stream ID and filename;
// both must identify this exact export. Manual recordings are never accepted.
func (m *Manager) DeleteArchiveClip(ctx context.Context, clip ClipInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	clip.ID = strings.TrimSpace(clip.ID)
	if clip.ID == "" || !strings.HasPrefix(clip.StreamID, "nvr_export_") || clip.FileName != clip.ID+".mp4" {
		return fmt.Errorf("invalid archive clip ownership")
	}
	videoPath, err := clipFilePath(m.cfg.ClipPath, clip.FileName)
	if err != nil {
		return err
	}
	metaPath, err := clipFilePath(m.cfg.ClipPath, clip.ID+".json")
	if err != nil {
		return err
	}
	m.mu.Lock()
	_, active := m.clipJobs[clip.ID]
	m.mu.Unlock()
	if active {
		return ErrClipAlreadyActive
	}
	// IDs are never restarted, so an absent job cannot become active after this
	// check. Existing metadata must agree before using the missing-file fallback.
	if stored, err := m.loadClip(clip.ID); err == nil {
		if stored.StreamID != clip.StreamID || stored.FileName != clip.FileName {
			return fmt.Errorf("archive clip metadata does not match ownership")
		}
	} else if !errors.Is(err, ErrClipNotFound) {
		return err
	}
	if err := removeClipStorageFile(videoPath); err != nil {
		return err
	}
	return removeClipStorageFile(metaPath)
}
