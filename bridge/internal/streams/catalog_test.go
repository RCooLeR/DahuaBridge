package streams

import (
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

func TestBuildCatalogForNVRChannel(t *testing.T) {
	catalog := BuildCatalog(CatalogInput{
		Config: config.Config{
			HomeAssistant: config.HomeAssistantConfig{
				PublicBaseURL: "http://bridge.local:8080",
			},
			Media: config.MediaConfig{
				StableFrameRate:     10,
				SubstreamFrameRate:  12,
				WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
			},
		},
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "west20_nvr",
					Kind: dahua.DeviceKindNVR,
				},
				Children: []dahua.Device{
					{
						ID:       "west20_nvr_channel_01",
						ParentID: "west20_nvr",
						Kind:     dahua.DeviceKindNVRChannel,
						Name:     "Front Gate",
						Attributes: map[string]string{
							"channel_index":   "1",
							"main_codec":      "H.265",
							"main_resolution": "3840x2160",
							"sub_codec":       "H.264",
							"sub_resolution":  "704x576",
						},
					},
				},
				States: map[string]dahua.DeviceState{
					"west20_nvr_channel_01": {
						Available: true,
						Info: map[string]any{
							"control_ptz_supported":              true,
							"control_ptz_pan":                    true,
							"control_ptz_tilt":                   true,
							"control_ptz_zoom":                   true,
							"control_ptz_focus":                  true,
							"control_ptz_commands":               []string{"left", "right", "up", "zoom_in"},
							"control_aux_supported":              true,
							"control_aux_outputs":                []string{"aux", "light", "wiper"},
							"control_aux_features":               []string{"siren", "warning_light", "wiper"},
							"control_audio_supported":            false,
							"control_audio_mute_supported":       true,
							"control_audio_volume_supported":     false,
							"control_audio_muted":                true,
							"control_audio_stream_enabled":       false,
							"control_audio_playback_supported":   true,
							"control_audio_playback_siren":       true,
							"control_audio_playback_quick_reply": false,
							"control_audio_playback_formats":     []string{"aac", "wav"},
							"control_audio_playback_file_count":  1,
							"control_recording_supported":        true,
							"control_recording_active":           true,
							"control_recording_mode":             "manual",
							"recommended_ha_integration":         "bridge_media",
							"validation_notes":                   []string{"ptz_capability_query_failed_aux_fallback_used"},
						},
					},
				},
			},
		},
		NVRConfigs: map[string]config.DeviceConfig{
			"west20_nvr": {
				BaseURL:  "http://nvr.example.local",
				Username: "admin",
				Password: "secret",
			},
		},
	})

	if len(catalog) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(catalog))
	}

	entry := catalog[0]
	if entry.RecommendedProfile != "stable" {
		t.Fatalf("expected stable recommendation, got %q", entry.RecommendedProfile)
	}
	if entry.Channel != 1 {
		t.Fatalf("expected channel 1, got %d", entry.Channel)
	}
	if !strings.Contains(entry.Profiles["stable"].StreamURL, "subtype=1") {
		t.Fatalf("expected stable profile to use subtype=1, got %q", entry.Profiles["stable"].StreamURL)
	}
	if entry.Profiles["stable"].FrameRate != 10 {
		t.Fatalf("unexpected stable frame rate %d", entry.Profiles["stable"].FrameRate)
	}
	if entry.Profiles["substream"].FrameRate != 12 {
		t.Fatalf("unexpected substream frame rate %d", entry.Profiles["substream"].FrameRate)
	}
	if entry.Profiles["stable"].SourceWidth != 704 || entry.Profiles["stable"].SourceHeight != 576 {
		t.Fatalf("unexpected stable source dimensions %+v", entry.Profiles["stable"])
	}
	if entry.Profiles["stable"].LocalHLSURL != "http://bridge.local:8080/api/v1/media/hls/west20_nvr_channel_01/stable/index.m3u8" {
		t.Fatalf("unexpected stable hls url %q", entry.Profiles["stable"].LocalHLSURL)
	}
	if entry.SnapshotURL != "http://bridge.local:8080/api/v1/nvr/west20_nvr/channels/1/snapshot" {
		t.Fatalf("unexpected snapshot url %q", entry.SnapshotURL)
	}
	if entry.LocalPreviewURL != "http://bridge.local:8080/api/v1/media/preview/west20_nvr_channel_01?profile=stable" {
		t.Fatalf("unexpected preview url %q", entry.LocalPreviewURL)
	}
	if entry.Profiles["stable"].LocalWebRTCURL != "http://bridge.local:8080/api/v1/media/webrtc/west20_nvr_channel_01/stable" {
		t.Fatalf("unexpected webrtc url %q", entry.Profiles["stable"].LocalWebRTCURL)
	}
	if entry.Controls == nil || entry.Controls.PTZ == nil || entry.Controls.Aux == nil || entry.Controls.Audio == nil || entry.Controls.Recording == nil {
		t.Fatalf("expected control summary, got %+v", entry.Controls)
	}
	if entry.Controls.PTZ.URL != "http://bridge.local:8080/api/v1/nvr/west20_nvr/channels/1/ptz" {
		t.Fatalf("unexpected ptz url %q", entry.Controls.PTZ.URL)
	}
	if !entry.Controls.PTZ.Zoom || len(entry.Controls.PTZ.Commands) != 4 {
		t.Fatalf("unexpected ptz summary %+v", entry.Controls.PTZ)
	}
	if entry.Controls.Aux.URL != "http://bridge.local:8080/api/v1/nvr/west20_nvr/channels/1/aux" {
		t.Fatalf("unexpected aux url %q", entry.Controls.Aux.URL)
	}
	if len(entry.Controls.Aux.Outputs) != 3 || entry.Controls.Aux.Outputs[0] != "aux" {
		t.Fatalf("unexpected aux outputs %+v", entry.Controls.Aux.Outputs)
	}
	if len(entry.Controls.Aux.Features) != 3 || entry.Controls.Aux.Features[0] != "siren" {
		t.Fatalf("unexpected aux features %+v", entry.Controls.Aux.Features)
	}
	if entry.Controls.Audio.Supported || !entry.Controls.Audio.Mute || entry.Controls.Audio.Volume {
		t.Fatalf("unexpected audio summary %+v", entry.Controls.Audio)
	}
	if !entry.Controls.Audio.Muted || entry.Controls.Audio.StreamAudioEnabled {
		t.Fatalf("unexpected audio mute state %+v", entry.Controls.Audio)
	}
	if !entry.Controls.Audio.PlaybackSupported || !entry.Controls.Audio.PlaybackSiren || entry.Controls.Audio.PlaybackFileCount != 1 {
		t.Fatalf("unexpected playback audio summary %+v", entry.Controls.Audio)
	}
	if entry.Controls.Recording.Supported || entry.Controls.Recording.URL != "" {
		t.Fatalf("expected manual device recording control to be hidden, got %+v", entry.Controls.Recording)
	}
	if !entry.Controls.Recording.Active || entry.Controls.Recording.Mode != "manual" {
		t.Fatalf("unexpected recording summary %+v", entry.Controls.Recording)
	}
	if len(entry.Controls.ValidationNotes) != 1 || entry.Controls.ValidationNotes[0] != "ptz_capability_query_failed_aux_fallback_used" {
		t.Fatalf("unexpected control validation notes %+v", entry.Controls.ValidationNotes)
	}
	if len(entry.Features) != 8 {
		t.Fatalf("expected 7 normalized features, got %+v", entry.Features)
	}
	archiveSearch := findFeatureByKey(entry.Features, "archive_search")
	if archiveSearch == nil || archiveSearch.Kind != "query" || archiveSearch.URL != "http://bridge.local:8080/api/v1/nvr/west20_nvr/recordings" {
		t.Fatalf("unexpected archive search feature %+v", archiveSearch)
	}
	light := findFeatureByKey(entry.Features, "light")
	if light == nil || light.ParameterKey != "output" || light.ParameterValue != "light" || light.Label != "White Light" {
		t.Fatalf("unexpected light feature %+v", light)
	}
	ptz := findFeatureByKey(entry.Features, "ptz")
	if ptz == nil || ptz.Kind != "command_set" || len(ptz.Commands) != 4 || len(ptz.Actions) != 3 {
		t.Fatalf("unexpected ptz feature %+v", ptz)
	}
	siren := findFeatureByKey(entry.Features, "siren")
	if siren == nil || siren.ParameterKey != "output" || siren.ParameterValue != "siren" || siren.URL != "http://bridge.local:8080/api/v1/nvr/west20_nvr/channels/1/aux" {
		t.Fatalf("unexpected siren feature %+v", siren)
	}
	mute := findFeatureByKey(entry.Features, "mute")
	if mute == nil || mute.Kind != "toggle" || mute.URL != "http://bridge.local:8080/api/v1/nvr/west20_nvr/channels/1/audio/mute" || mute.Active == nil || !*mute.Active {
		t.Fatalf("unexpected mute feature %+v", mute)
	}
	recording := findFeatureByKey(entry.Features, "recording")
	if recording != nil {
		t.Fatalf("expected no manual recording feature, got %+v", recording)
	}
}

func TestBuildCatalogIncludesCredentialsWhenRequested(t *testing.T) {
	catalog := BuildCatalog(CatalogInput{
		IncludeCredentials: true,
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard",
					Kind: dahua.DeviceKindIPC,
				},
			},
		},
		IPCConfigs: map[string]config.DeviceConfig{
			"yard_ipc": {
				BaseURL:  "https://ipc20.example.local",
				Username: "admin",
				Password: "secret#1",
			},
		},
	})

	if len(catalog) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(catalog))
	}

	if !strings.Contains(catalog[0].Profiles["quality"].StreamURL, "admin:secret%231@") {
		t.Fatalf("expected credentials in quality stream url, got %q", catalog[0].Profiles["quality"].StreamURL)
	}
	if !strings.Contains(catalog[0].Profiles["quality"].StreamURL, ":554/") {
		t.Fatalf("expected default rtsp port 554, got %q", catalog[0].Profiles["quality"].StreamURL)
	}
}

func TestRecommendProfilePrefersQualityForH264MainStream(t *testing.T) {
	recommended := recommendProfile("H.264", "1920x1080", "H.264", "704x576")
	if recommended != "quality" {
		t.Fatalf("expected quality recommendation, got %q", recommended)
	}
}

func TestRecommendProfilePrefersStableForDahuaH265Variants(t *testing.T) {
	testCases := []string{
		"H.265",
		"H.265H",
		"Smart H.265+",
		"HEVC",
	}

	for _, codec := range testCases {
		if got := recommendProfile(codec, "1920x1080", "H.264", "704x576"); got != "stable" {
			t.Fatalf("expected stable recommendation for %q, got %q", codec, got)
		}
	}
}

func TestCodecFamilyNormalizesDahuaCodecNames(t *testing.T) {
	testCases := map[string]string{
		"H.264":        "h264",
		"H.264B":       "h264",
		"H.264H":       "h264",
		"H.264M":       "h264",
		"Smart H.264+": "h264",
		"H.265":        "h265",
		"H.265H":       "h265",
		"Smart H.265+": "h265",
		"HEVC":         "h265",
		"MJPEG":        "mjpeg",
		"MJPG":         "mjpeg",
		"MPEG4":        "mpeg4",
		"SVAC":         "svac",
	}

	for input, want := range testCases {
		if got := codecFamily(input); got != want {
			t.Fatalf("codecFamily(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildCatalogAppliesONVIFRecommendation(t *testing.T) {
	catalog := BuildCatalog(CatalogInput{
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard",
					Kind: dahua.DeviceKindIPC,
				},
				States: map[string]dahua.DeviceState{
					"yard_ipc": {
						Available: true,
						Info: map[string]any{
							"onvif_h264_available":       true,
							"onvif_profile_token":        "Profile_1",
							"onvif_profile_name":         "MainStream-H264",
							"onvif_stream_url":           "rtsp://ipc120.example.local:554/onvif1",
							"onvif_snapshot_url":         "http://ipc120.example.local/onvif/snapshot1.jpg",
							"recommended_ha_integration": "onvif",
						},
					},
				},
			},
		},
		IPCConfigs: map[string]config.DeviceConfig{
			"yard_ipc": {
				BaseURL:  "http://ipc120.example.local",
				Username: "admin",
				Password: "secret",
			},
		},
	})

	if len(catalog) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(catalog))
	}
	entry := catalog[0]
	if entry.RecommendedHAIntegration != "onvif" {
		t.Fatalf("expected onvif recommendation, got %q", entry.RecommendedHAIntegration)
	}
	if entry.ONVIFProfileToken != "Profile_1" {
		t.Fatalf("unexpected onvif token %q", entry.ONVIFProfileToken)
	}
	if entry.ONVIFSnapshotURL != "http://ipc120.example.local/onvif/snapshot1.jpg" {
		t.Fatalf("unexpected onvif snapshot url %q", entry.ONVIFSnapshotURL)
	}
	if !entry.ONVIFH264Available {
		t.Fatal("expected onvif h264 availability")
	}
}

func TestBuildCatalogForVTOIncludesLockCount(t *testing.T) {
	catalog := BuildCatalog(CatalogInput{
		Config: config.Config{
			HomeAssistant: config.HomeAssistantConfig{
				PublicBaseURL: "http://bridge.local:8080",
			},
			Media: config.MediaConfig{
				WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
			},
		},
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "front_vto",
					Name: "Front Door",
					Kind: dahua.DeviceKindVTO,
					Attributes: map[string]string{
						"lock_count": "2",
					},
				},
				States: map[string]dahua.DeviceState{
					"front_vto": {
						Available: true,
						Info: map[string]any{
							"audio_codec":                           "PCM",
							"call_state":                            "ringing",
							"last_ring_at":                          "2026-04-27T18:45:00Z",
							"last_call_started_at":                  "2026-04-27T18:45:03Z",
							"last_call_ended_at":                    "2026-04-27T18:45:21Z",
							"last_call_duration_seconds":            18,
							"last_call_source":                      "villa_panel",
							"control_audio_output_volume_supported": true,
							"control_audio_input_volume_supported":  true,
							"control_audio_mute_supported":          true,
							"control_recording_supported":           true,
							"control_audio_output_volume":           80,
							"control_audio_output_volume_levels":    []int{80, 60},
							"control_audio_input_volume":            90,
							"control_audio_input_volume_levels":     []int{90, 60},
							"control_audio_muted":                   false,
							"control_recording_auto_enabled":        true,
							"control_recording_auto_time_seconds":   11,
							"control_stream_audio_enabled":          true,
							"validation_notes":                      []string{"vto_audio_control_surface_config_backed"},
						},
					},
				},
			},
		},
		VTOConfigs: map[string]config.DeviceConfig{
			"front_vto": {
				BaseURL:  "http://vto.example.local",
				Username: "admin",
				Password: "secret",
			},
		},
		IntercomStatuses: map[string]RuntimeIntercomStatus{
			"front_vto": {
				Active:                 true,
				SessionCount:           2,
				ExternalUplinkEnabled:  true,
				UplinkActive:           true,
				UplinkCodec:            "audio/opus",
				UplinkPackets:          33,
				UplinkForwardedPackets: 30,
				UplinkForwardErrors:    1,
			},
		},
	})

	if len(catalog) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(catalog))
	}
	if catalog[0].LockCount != 2 {
		t.Fatalf("expected lock count 2, got %d", catalog[0].LockCount)
	}
	if catalog[0].LocalIntercomURL != "http://bridge.local:8080/api/v1/vto/front_vto/intercom?profile=quality" {
		t.Fatalf("unexpected intercom url %q", catalog[0].LocalIntercomURL)
	}
	if catalog[0].Intercom == nil {
		t.Fatal("expected intercom summary")
	}
	if catalog[0].Intercom.CallState != "ringing" {
		t.Fatalf("unexpected call state %q", catalog[0].Intercom.CallState)
	}
	if catalog[0].Intercom.LastCallDurationSeconds != 18 {
		t.Fatalf("unexpected call duration %d", catalog[0].Intercom.LastCallDurationSeconds)
	}
	if catalog[0].Intercom.AnswerURL != "http://bridge.local:8080/api/v1/vto/front_vto/call/answer" {
		t.Fatalf("unexpected answer url %q", catalog[0].Intercom.AnswerURL)
	}
	if catalog[0].Intercom.HangupURL != "http://bridge.local:8080/api/v1/vto/front_vto/call/hangup" {
		t.Fatalf("unexpected hangup url %q", catalog[0].Intercom.HangupURL)
	}
	if catalog[0].Intercom.ExternalUplinkEnableURL != "http://bridge.local:8080/api/v1/vto/front_vto/intercom/uplink/enable" {
		t.Fatalf("unexpected uplink enable url %q", catalog[0].Intercom.ExternalUplinkEnableURL)
	}
	if catalog[0].Intercom.BridgeSessionResetURL != "http://bridge.local:8080/api/v1/vto/front_vto/intercom/reset" {
		t.Fatalf("unexpected bridge session reset url %q", catalog[0].Intercom.BridgeSessionResetURL)
	}
	if catalog[0].Intercom.ExternalUplinkDisableURL != "http://bridge.local:8080/api/v1/vto/front_vto/intercom/uplink/disable" {
		t.Fatalf("unexpected uplink disable url %q", catalog[0].Intercom.ExternalUplinkDisableURL)
	}
	if len(catalog[0].Intercom.LockURLs) != 2 {
		t.Fatalf("expected 2 lock urls, got %d", len(catalog[0].Intercom.LockURLs))
	}
	if catalog[0].Intercom.LockURLs[1] != "http://bridge.local:8080/api/v1/vto/front_vto/locks/1/unlock" {
		t.Fatalf("unexpected second lock url %q", catalog[0].Intercom.LockURLs[1])
	}
	if !catalog[0].Intercom.SupportsHangup || !catalog[0].Intercom.SupportsBridgeSessionReset || !catalog[0].Intercom.SupportsUnlock || !catalog[0].Intercom.SupportsBrowserMicrophone {
		t.Fatalf("expected supported intercom actions, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.SupportsBridgeAudioOutput || !catalog[0].Intercom.SupportsBridgeAudioUplink {
		t.Fatalf("expected bridge audio capabilities, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.SupportsExternalAudioExport || catalog[0].Intercom.ConfiguredExternalUplinkTargetCount != 1 {
		t.Fatalf("expected configured external uplink export, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.BridgeSessionActive || catalog[0].Intercom.BridgeSessionCount != 2 || !catalog[0].Intercom.ExternalUplinkEnabled {
		t.Fatalf("expected bridge intercom runtime state, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.BridgeUplinkActive || catalog[0].Intercom.BridgeUplinkCodec != "audio/opus" || catalog[0].Intercom.BridgeForwardedPackets != 30 {
		t.Fatalf("expected bridge uplink runtime metrics, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.SupportsVTOCallAnswer {
		t.Fatalf("expected VTO call answer support, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.SupportsVTOOutputVolumeControl || !catalog[0].Intercom.SupportsVTOInputVolumeControl || !catalog[0].Intercom.SupportsVTOMuteControl || !catalog[0].Intercom.SupportsVTORecordingControl {
		t.Fatalf("expected supported VTO control extensions, got %+v", catalog[0].Intercom)
	}
	if catalog[0].Intercom.OutputVolumeURL != "http://bridge.local:8080/api/v1/vto/front_vto/audio/output-volume" ||
		catalog[0].Intercom.InputVolumeURL != "http://bridge.local:8080/api/v1/vto/front_vto/audio/input-volume" ||
		catalog[0].Intercom.MuteURL != "http://bridge.local:8080/api/v1/vto/front_vto/audio/mute" ||
		catalog[0].Intercom.RecordingURL != "http://bridge.local:8080/api/v1/vto/front_vto/recording" {
		t.Fatalf("unexpected VTO control urls %+v", catalog[0].Intercom)
	}
	if catalog[0].Intercom.OutputVolumeLevel != 80 || catalog[0].Intercom.InputVolumeLevel != 90 || catalog[0].Intercom.Muted || !catalog[0].Intercom.AutoRecordEnabled || catalog[0].Intercom.AutoRecordTimeSeconds != 11 || !catalog[0].Intercom.StreamAudioEnabled {
		t.Fatalf("unexpected VTO control state %+v", catalog[0].Intercom)
	}
	if len(catalog[0].Intercom.ValidationNotes) != 1 || catalog[0].Intercom.ValidationNotes[0] != "vto_audio_control_surface_config_backed" {
		t.Fatalf("unexpected vto validation notes %+v", catalog[0].Intercom.ValidationNotes)
	}
	if catalog[0].Intercom.SupportsVTOTalkback || catalog[0].Intercom.SupportsFullCallAcceptance {
		t.Fatalf("expected talkback and full acceptance to remain unsupported, got %+v", catalog[0].Intercom)
	}
	if len(catalog[0].Features) != 8 {
		t.Fatalf("expected 8 vto features, got %+v", catalog[0].Features)
	}
	unlock := findFeatureByKey(catalog[0].Features, "unlock")
	if unlock == nil || unlock.Kind != "targeted_action" || len(unlock.Targets) != 2 {
		t.Fatalf("unexpected unlock feature %+v", unlock)
	}
	outputVolume := findFeatureByKey(catalog[0].Features, "output_volume")
	if outputVolume == nil || outputVolume.Kind != "level" || outputVolume.CurrentValue == nil || *outputVolume.CurrentValue != 80 {
		t.Fatalf("unexpected output volume feature %+v", outputVolume)
	}
	mute := findFeatureByKey(catalog[0].Features, "mute")
	if mute == nil || mute.Active == nil || *mute.Active {
		t.Fatalf("unexpected mute feature %+v", mute)
	}
	autoRecord := findFeatureByKey(catalog[0].Features, "auto_record")
	if autoRecord == nil || autoRecord.Active == nil || !*autoRecord.Active {
		t.Fatalf("unexpected auto record feature %+v", autoRecord)
	}
}

func findFeatureByKey(features []FeatureSummary, key string) *FeatureSummary {
	for index := range features {
		if features[index].Key == key {
			return &features[index]
		}
	}
	return nil
}
