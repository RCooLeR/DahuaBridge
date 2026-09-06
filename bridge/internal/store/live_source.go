package store

// LiveSources returns preferences independently of device probe snapshots.
func (s *ProbeStore) LiveSources() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneStringMap(s.liveSources)
}

// LiveSourceSettings reads the default and overrides together so a catalog
// cannot combine preferences from different updates.
func (s *ProbeStore) LiveSourceSettings() (string, map[string]string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source := "nvr"
	if s.defaultLiveSource == "camera" {
		source = "camera"
	}
	return source, cloneStringMap(s.liveSources)
}

// SetLiveSourceAndSave publishes the new preference only after the snapshot has
// been written successfully. The file lock also serializes periodic flushes.
func (s *ProbeStore) SetLiveSourceAndSave(path, streamID, source string) error {
	return s.saveLiveSourceSettings(path, func(snapshot *Snapshot) {
		if source == "" {
			delete(snapshot.LiveSources, streamID)
			return
		}
		if snapshot.LiveSources == nil {
			snapshot.LiveSources = make(map[string]string)
		}
		snapshot.LiveSources[streamID] = source
	})
}

func (s *ProbeStore) SetDefaultLiveSourceAndSave(path, source string) error {
	return s.saveLiveSourceSettings(path, func(snapshot *Snapshot) {
		snapshot.DefaultLiveSource = source
	})
}

func (s *ProbeStore) saveLiveSourceSettings(path string, mutate func(*Snapshot)) error {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	snapshot, revision, _ := s.snapshot(nil, false)
	mutate(&snapshot)
	if err := s.writeFile(path, snapshot, revision); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.liveSources = cloneStringMap(snapshot.LiveSources)
	s.defaultLiveSource = snapshot.DefaultLiveSource
	s.livePreconnect = snapshot.LivePreconnect.Defaulted()
	// Probes can change during disk I/O; the next periodic flush must include
	// both the new preference and any concurrently updated probe metadata.
	s.markDirty()
	return nil
}
