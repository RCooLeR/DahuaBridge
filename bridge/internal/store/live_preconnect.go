package store

import "RCooLeR/DahuaBridge/internal/streams"

func (s *ProbeStore) LivePreconnect() streams.LivePreconnectSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.livePreconnect.Defaulted()
}

func (s *ProbeStore) SetLivePreconnectAndSave(path string, settings streams.LivePreconnectSettings) error {
	if !settings.Valid() {
		return streams.ErrInvalidLivePreconnect
	}
	return s.saveLiveSourceSettings(path, func(snapshot *Snapshot) {
		snapshot.LivePreconnect = settings
	})
}
