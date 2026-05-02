package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

type sessionDescription struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type probeResult struct {
	StreamID      string   `json:"stream_id"`
	Profile       string   `json:"profile"`
	Connection    string   `json:"connection_state"`
	Gathering     string   `json:"ice_gathering_state"`
	Tracks        []string `json:"tracks"`
	FirstTrackAt  string   `json:"first_track_at,omitempty"`
	FirstPacketAt string   `json:"first_packet_at,omitempty"`
}

func main() {
	baseURL := flag.String("base", "http://localhost:9205", "Bridge base URL")
	streamID := flag.String("stream", "", "Stream ID")
	profile := flag.String("profile", "stable", "Profile name")
	hold := flag.Duration("hold", 15*time.Second, "How long to keep the session open after packets arrive")
	timeout := flag.Duration("timeout", 30*time.Second, "Overall probe timeout")
	flag.Parse()

	if strings.TrimSpace(*streamID) == "" {
		fmt.Fprintln(os.Stderr, "stream is required")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := run(ctx, *baseURL, *streamID, *profile, *hold); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, baseURL string, streamID string, profile string, hold time.Duration) error {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}
	defer func() {
		_ = pc.Close()
	}()

	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return fmt.Errorf("add video transceiver: %w", err)
	}
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return fmt.Errorf("add audio transceiver: %w", err)
	}

	var (
		mu            sync.Mutex
		result        = probeResult{StreamID: streamID, Profile: profile}
		firstTrackAt  time.Time
		firstPacketAt time.Time
		tracksSeen    = make(map[string]struct{})
	)
	connectionReady := make(chan struct{})
	packetReady := make(chan struct{})
	var connectionOnce sync.Once
	var packetOnce sync.Once

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		mu.Lock()
		result.Connection = state.String()
		mu.Unlock()
		if state == webrtc.PeerConnectionStateConnected {
			connectionOnce.Do(func() { close(connectionReady) })
		}
	})
	pc.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		mu.Lock()
		result.Gathering = state.String()
		mu.Unlock()
	})
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		now := time.Now().UTC()
		mu.Lock()
		if firstTrackAt.IsZero() {
			firstTrackAt = now
			result.FirstTrackAt = now.Format(time.RFC3339Nano)
		}
		trackKey := fmt.Sprintf("%s/%s", strings.ToLower(track.Kind().String()), strings.TrimSpace(track.Codec().MimeType))
		if _, ok := tracksSeen[trackKey]; !ok {
			tracksSeen[trackKey] = struct{}{}
			result.Tracks = append(result.Tracks, trackKey)
		}
		mu.Unlock()

		go func() {
			for {
				if _, _, readErr := track.ReadRTP(); readErr != nil {
					return
				}
				now := time.Now().UTC()
				mu.Lock()
				if firstPacketAt.IsZero() {
					firstPacketAt = now
					result.FirstPacketAt = now.Format(time.RFC3339Nano)
				}
				mu.Unlock()
				packetOnce.Do(func() { close(packetReady) })
			}
		}()
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gatherComplete:
	}

	localDescription := pc.LocalDescription()
	if localDescription == nil {
		return fmt.Errorf("local description missing after gather")
	}
	answer, err := postOffer(ctx, strings.TrimRight(baseURL, "/"), streamID, profile, sessionDescription{
		Type: localDescription.Type.String(),
		SDP:  localDescription.SDP,
	})
	if err != nil {
		return err
	}

	answerType := webrtc.NewSDPType(answer.Type)
	if answerType == webrtc.SDPTypeUnknown {
		return fmt.Errorf("unexpected answer type %q", answer.Type)
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: answerType,
		SDP:  answer.SDP,
	}); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-connectionReady:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-packetReady:
	}

	timer := time.NewTimer(hold)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	mu.Lock()
	defer mu.Unlock()
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(payload))
	return nil
}

func postOffer(ctx context.Context, baseURL string, streamID string, profile string, offer sessionDescription) (sessionDescription, error) {
	body, err := json.Marshal(offer)
	if err != nil {
		return sessionDescription{}, fmt.Errorf("marshal offer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/v1/media/webrtc/%s/%s/offer", baseURL, streamID, profile), bytes.NewReader(body))
	if err != nil {
		return sessionDescription{}, fmt.Errorf("build offer request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return sessionDescription{}, fmt.Errorf("post offer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return sessionDescription{}, fmt.Errorf("offer request failed: %s", resp.Status)
	}

	var answer sessionDescription
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return sessionDescription{}, fmt.Errorf("decode answer: %w", err)
	}
	return answer, nil
}
