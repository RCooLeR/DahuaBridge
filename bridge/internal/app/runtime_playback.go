package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

const (
	playbackSessionTTL         = 30 * time.Minute
	playbackRTSPTimeLayout     = "2006_01_02_15_04_05"
	playbackResponseTimeLayout = time.RFC3339
)

type playbackSession struct {
	ID                 string
	DeviceID           string
	SourceStreamID     string
	Name               string
	Channel            int
	StartTime          time.Time
	EndTime            time.Time
	SeekTime           time.Time
	SnapshotURL        string
	MainCodec          string
	MainResolution     string
	SubCodec           string
	SubResolution      string
	RecommendedProfile string
	CreatedAt          time.Time
	ExpiresAt          time.Time
}

type playbackSourceMetadata struct {
	SourceStreamID     string
	Name               string
	SnapshotURL        string
	MainCodec          string
	MainResolution     string
	SubCodec           string
	SubResolution      string
	RecommendedProfile string
}

func (r *runtimeServices) CreateNVRPlaybackSession(_ context.Context, deviceID string, request dahua.NVRPlaybackSessionRequest) (dahua.NVRPlaybackSession, error) {
	session, err := r.newPlaybackSession(deviceID, request)
	if err != nil {
		return dahua.NVRPlaybackSession{}, err
	}
	return r.playbackSessionResponse(session), nil
}

func (r *runtimeServices) GetNVRPlaybackSession(sessionID string) (dahua.NVRPlaybackSession, error) {
	session, err := r.touchPlaybackSession(sessionID)
	if err != nil {
		return dahua.NVRPlaybackSession{}, err
	}
	return r.playbackSessionResponse(session), nil
}

func (r *runtimeServices) SeekNVRPlaybackSession(_ context.Context, sessionID string, seekTime time.Time) (dahua.NVRPlaybackSession, error) {
	if seekTime.IsZero() {
		return dahua.NVRPlaybackSession{}, fmt.Errorf("seek time is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cleanupExpiredPlaybackSessionsLocked(time.Now())
	existing, ok := r.playback[sessionID]
	if !ok {
		return dahua.NVRPlaybackSession{}, fmt.Errorf("%w: %s", dahua.ErrPlaybackSessionNotFound, sessionID)
	}
	if seekTime.Before(existing.StartTime) || seekTime.After(existing.EndTime) {
		return dahua.NVRPlaybackSession{}, fmt.Errorf("seek time must be within the requested playback range")
	}

	now := time.Now()
	existing.ID = newPlaybackSessionID()
	existing.SeekTime = seekTime
	existing.CreatedAt = now
	existing.ExpiresAt = now.Add(playbackSessionTTL)
	r.playback[existing.ID] = existing

	return r.playbackSessionResponse(existing), nil
}

func (r *runtimeServices) getPlaybackStream(streamID string, profileName string, includeCredentials bool) (streams.Entry, streams.Profile, bool) {
	session, cfg, ok := r.touchPlaybackStream(streamID)
	if !ok {
		return streams.Entry{}, streams.Profile{}, false
	}

	entry := buildPlaybackEntry(r.cfg, cfg, session, includeCredentials)
	if strings.TrimSpace(profileName) == "" {
		profileName = "stable"
	}
	profile, ok := entry.Profiles[profileName]
	if !ok {
		return streams.Entry{}, streams.Profile{}, false
	}
	return entry, profile, true
}

func (r *runtimeServices) newPlaybackSession(deviceID string, request dahua.NVRPlaybackSessionRequest) (playbackSession, error) {
	if request.Channel <= 0 {
		return playbackSession{}, fmt.Errorf("channel must be >= 1")
	}
	if request.StartTime.IsZero() || request.EndTime.IsZero() {
		return playbackSession{}, fmt.Errorf("start and end time are required")
	}
	if request.EndTime.Before(request.StartTime) {
		return playbackSession{}, fmt.Errorf("end time must not be before start time")
	}

	seekTime := request.SeekTime
	if seekTime.IsZero() {
		seekTime = request.StartTime
	}
	if seekTime.Before(request.StartTime) || seekTime.After(request.EndTime) {
		return playbackSession{}, fmt.Errorf("seek time must be within the requested playback range")
	}

	r.mu.RLock()
	cfg, ok := r.nvrConfigs[deviceID]
	r.mu.RUnlock()
	if !ok {
		return playbackSession{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	if !cfg.AllowsChannel(request.Channel) {
		return playbackSession{}, fmt.Errorf("channel %d is not allowed for device %q", request.Channel, deviceID)
	}

	metadata := r.lookupPlaybackSourceMetadata(deviceID, request.Channel)
	now := time.Now()
	session := playbackSession{
		ID:                 newPlaybackSessionID(),
		DeviceID:           deviceID,
		SourceStreamID:     firstNonEmptyPlayback(metadata.SourceStreamID, fmt.Sprintf("%s_channel_%02d", deviceID, request.Channel)),
		Name:               firstNonEmptyPlayback(metadata.Name, fmt.Sprintf("Channel %d", request.Channel)),
		Channel:            request.Channel,
		StartTime:          request.StartTime,
		EndTime:            request.EndTime,
		SeekTime:           seekTime,
		SnapshotURL:        firstNonEmptyPlayback(metadata.SnapshotURL, playbackSnapshotURL(r.cfg.HomeAssistant.PublicBaseURL, deviceID, request.Channel)),
		MainCodec:          metadata.MainCodec,
		MainResolution:     metadata.MainResolution,
		SubCodec:           metadata.SubCodec,
		SubResolution:      metadata.SubResolution,
		RecommendedProfile: firstNonEmptyPlayback(metadata.RecommendedProfile, "stable"),
		CreatedAt:          now,
		ExpiresAt:          now.Add(playbackSessionTTL),
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupExpiredPlaybackSessionsLocked(now)
	r.playback[session.ID] = session
	return session, nil
}

func (r *runtimeServices) lookupPlaybackSourceMetadata(deviceID string, channel int) playbackSourceMetadata {
	for _, entry := range r.ListStreams(false) {
		if entry.DeviceKind != dahua.DeviceKindNVRChannel {
			continue
		}
		if entry.RootDeviceID != deviceID || entry.Channel != channel {
			continue
		}
		return playbackSourceMetadata{
			SourceStreamID:     entry.ID,
			Name:               entry.Name,
			SnapshotURL:        entry.SnapshotURL,
			MainCodec:          entry.MainCodec,
			MainResolution:     entry.MainResolution,
			SubCodec:           entry.SubCodec,
			SubResolution:      entry.SubResolution,
			RecommendedProfile: entry.RecommendedProfile,
		}
	}
	return playbackSourceMetadata{}
}

func (r *runtimeServices) touchPlaybackSession(sessionID string) (playbackSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cleanupExpiredPlaybackSessionsLocked(time.Now())
	session, ok := r.playback[sessionID]
	if !ok {
		return playbackSession{}, fmt.Errorf("%w: %s", dahua.ErrPlaybackSessionNotFound, sessionID)
	}
	session.ExpiresAt = time.Now().Add(playbackSessionTTL)
	r.playback[sessionID] = session
	return session, nil
}

func (r *runtimeServices) touchPlaybackStream(streamID string) (playbackSession, config.DeviceConfig, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cleanupExpiredPlaybackSessionsLocked(time.Now())
	session, ok := r.playback[streamID]
	if !ok {
		return playbackSession{}, config.DeviceConfig{}, false
	}
	cfg, ok := r.nvrConfigs[session.DeviceID]
	if !ok {
		return playbackSession{}, config.DeviceConfig{}, false
	}
	session.ExpiresAt = time.Now().Add(playbackSessionTTL)
	r.playback[streamID] = session
	return session, cfg, true
}

func (r *runtimeServices) cleanupExpiredPlaybackSessionsLocked(now time.Time) {
	for sessionID, session := range r.playback {
		if now.After(session.ExpiresAt) {
			delete(r.playback, sessionID)
		}
	}
}

func (r *runtimeServices) playbackSessionResponse(session playbackSession) dahua.NVRPlaybackSession {
	profiles := make(map[string]dahua.NVRPlaybackProfile, 4)
	for _, profileName := range []string{"quality", "default", "stable", "substream"} {
		profiles[profileName] = dahua.NVRPlaybackProfile{
			Name:           profileName,
			HLSURL:         playbackHLSURL(r.cfg.HomeAssistant.PublicBaseURL, session.ID, profileName),
			MJPEGURL:       playbackMJPEGURL(r.cfg.HomeAssistant.PublicBaseURL, session.ID, profileName),
			WebRTCOfferURL: playbackWebRTCOfferURL(r.cfg.HomeAssistant.PublicBaseURL, session.ID, profileName),
		}
	}

	return dahua.NVRPlaybackSession{
		ID:                 session.ID,
		StreamID:           session.ID,
		DeviceID:           session.DeviceID,
		SourceStreamID:     session.SourceStreamID,
		Name:               session.Name,
		Channel:            session.Channel,
		StartTime:          session.StartTime.Format(playbackResponseTimeLayout),
		EndTime:            session.EndTime.Format(playbackResponseTimeLayout),
		SeekTime:           session.SeekTime.Format(playbackResponseTimeLayout),
		RecommendedProfile: firstNonEmptyPlayback(session.RecommendedProfile, "stable"),
		SnapshotURL:        session.SnapshotURL,
		CreatedAt:          session.CreatedAt.Format(playbackResponseTimeLayout),
		ExpiresAt:          session.ExpiresAt.Format(playbackResponseTimeLayout),
		Profiles:           profiles,
	}
}

func buildPlaybackEntry(cfg config.Config, deviceCfg config.DeviceConfig, session playbackSession, includeCredentials bool) streams.Entry {
	recommended := firstNonEmptyPlayback(session.RecommendedProfile, "stable")
	return streams.Entry{
		ID:                 session.ID,
		RootDeviceID:       session.DeviceID,
		SourceDeviceID:     session.SourceStreamID,
		DeviceKind:         dahua.DeviceKindNVRChannel,
		Name:               session.Name,
		Channel:            session.Channel,
		SnapshotURL:        session.SnapshotURL,
		MainCodec:          session.MainCodec,
		MainResolution:     session.MainResolution,
		SubCodec:           session.SubCodec,
		SubResolution:      session.SubResolution,
		RecommendedProfile: recommended,
		Profiles:           buildPlaybackProfiles(cfg, deviceCfg, session, includeCredentials, recommended),
	}
}

func buildPlaybackProfiles(cfg config.Config, deviceCfg config.DeviceConfig, session playbackSession, includeCredentials bool, recommended string) map[string]streams.Profile {
	mainWidth, mainHeight, mainOK := parseResolutionForPlayback(session.MainResolution)
	subWidth, subHeight, subOK := parseResolutionForPlayback(session.SubResolution)
	if !mainOK {
		mainWidth, mainHeight = 0, 0
	}
	if !subOK {
		subWidth, subHeight = 0, 0
	}
	mainSubtype := 0
	stableSubtype := 0
	substreamSubtype := 1
	stableWidth, stableHeight := mainWidth, mainHeight
	substreamWidth, substreamHeight := subWidth, subHeight
	if strings.TrimSpace(session.SubResolution) == "" {
		substreamSubtype = mainSubtype
		substreamWidth, substreamHeight = mainWidth, mainHeight
	}

	return map[string]streams.Profile{
		"default": {
			Name:           "default",
			StreamURL:      buildPlaybackRTSPURL(deviceCfg, session.Channel, mainSubtype, session.SeekTime, session.EndTime, includeCredentials),
			LocalMJPEGURL:  playbackMJPEGURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "default"),
			LocalHLSURL:    playbackHLSURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "default"),
			LocalWebRTCURL: playbackWebRTCPageURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "default"),
			Subtype:        mainSubtype,
			SourceWidth:    mainWidth,
			SourceHeight:   mainHeight,
			Recommended:    recommended == "default",
		},
		"quality": {
			Name:           "quality",
			StreamURL:      buildPlaybackRTSPURL(deviceCfg, session.Channel, mainSubtype, session.SeekTime, session.EndTime, includeCredentials),
			LocalMJPEGURL:  playbackMJPEGURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "quality"),
			LocalHLSURL:    playbackHLSURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "quality"),
			LocalWebRTCURL: playbackWebRTCPageURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "quality"),
			Subtype:        mainSubtype,
			RTSPTransport:  "tcp",
			SourceWidth:    mainWidth,
			SourceHeight:   mainHeight,
			Recommended:    recommended == "quality",
		},
		"stable": {
			Name:           "stable",
			StreamURL:      buildPlaybackRTSPURL(deviceCfg, session.Channel, stableSubtype, session.SeekTime, session.EndTime, includeCredentials),
			LocalMJPEGURL:  playbackMJPEGURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "stable"),
			LocalHLSURL:    playbackHLSURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "stable"),
			LocalWebRTCURL: playbackWebRTCPageURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "stable"),
			Subtype:        stableSubtype,
			RTSPTransport:  "tcp",
			FrameRate:      cfg.Media.StableFrameRate,
			SourceWidth:    stableWidth,
			SourceHeight:   stableHeight,
			Recommended:    recommended == "stable",
		},
		"substream": {
			Name:           "substream",
			StreamURL:      buildPlaybackRTSPURL(deviceCfg, session.Channel, substreamSubtype, session.SeekTime, session.EndTime, includeCredentials),
			LocalMJPEGURL:  playbackMJPEGURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "substream"),
			LocalHLSURL:    playbackHLSURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "substream"),
			LocalWebRTCURL: playbackWebRTCPageURL(cfg.HomeAssistant.PublicBaseURL, session.ID, "substream"),
			Subtype:        substreamSubtype,
			RTSPTransport:  "tcp",
			FrameRate:      cfg.Media.SubstreamFrameRate,
			SourceWidth:    substreamWidth,
			SourceHeight:   substreamHeight,
			Recommended:    recommended == "substream",
		},
	}
}

func buildPlaybackRTSPURL(deviceCfg config.DeviceConfig, channel int, subtype int, startTime time.Time, endTime time.Time, includeCredentials bool) string {
	base, err := url.Parse(deviceCfg.BaseURL)
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
		Scheme: "rtsp",
		Host:   host,
		Path:   "/cam/realmonitor",
		RawQuery: url.Values{
			"channel":   []string{strconv.Itoa(channel)},
			"subtype":   []string{strconv.Itoa(subtype)},
			"starttime": []string{startTime.Format(playbackRTSPTimeLayout)},
			"endtime":   []string{endTime.Format(playbackRTSPTimeLayout)},
		}.Encode(),
	}
	if includeCredentials {
		rtspURL.User = url.UserPassword(deviceCfg.Username, deviceCfg.Password)
	}
	return rtspURL.String()
}

func playbackSnapshotURL(publicBaseURL string, deviceID string, channel int) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/nvr/" + url.PathEscape(deviceID) + "/channels/" + strconv.Itoa(channel) + "/snapshot"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func playbackMJPEGURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/mjpeg/" + url.PathEscape(streamID) + "?profile=" + url.QueryEscape(profile)
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func playbackHLSURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/hls/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile) + "/index.m3u8"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func playbackWebRTCPageURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/webrtc/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile)
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func playbackWebRTCOfferURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/webrtc/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile) + "/offer"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func newPlaybackSessionID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("nvrpb_%d", time.Now().UnixNano())
	}
	return "nvrpb_" + hex.EncodeToString(buffer)
}

func parseResolutionForPlayback(value string) (int, int, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, 0, false
	}

	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == 'x' || r == '*' || r == ',' || r == ' '
	})
	if len(parts) < 2 {
		return 0, 0, false
	}

	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return width, height, true
}

func firstNonEmptyPlayback(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
