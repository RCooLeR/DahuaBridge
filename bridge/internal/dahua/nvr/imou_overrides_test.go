package nvr

import (
	"context"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/imou"
	"github.com/rs/zerolog"
)

type imouServiceStub struct {
	statuses   []imou.CameraStatusChange
	nightModes []imou.NightVisionModeChange
	mode       imou.NightVisionMode
	audioOn    bool
}

func (s *imouServiceStub) Enabled() bool { return true }

func (s *imouServiceStub) GetCameraStatus(context.Context, imou.CameraStatusRequest) (imou.CameraStatus, error) {
	return imou.CameraStatus{Enabled: s.audioOn}, nil
}

func (s *imouServiceStub) SetCameraStatus(_ context.Context, change imou.CameraStatusChange) error {
	s.statuses = append(s.statuses, change)
	return nil
}

func (s *imouServiceStub) GetNightVisionMode(context.Context, imou.NightVisionModeRequest) (imou.NightVisionMode, error) {
	if s.mode.Mode == "" {
		return imou.NightVisionMode{Mode: "SmartLowLight", Modes: []string{"SmartLowLight", "FullColor"}}, nil
	}
	return s.mode, nil
}

func (s *imouServiceStub) SetNightVisionMode(_ context.Context, change imou.NightVisionModeChange) error {
	s.nightModes = append(s.nightModes, change)
	return nil
}

func (s *imouServiceStub) ListAlarms(context.Context, imou.AlarmQuery) ([]imou.Alarm, error) {
	return nil, nil
}

func TestAuxCapabilitiesIncludeImouOverrides(t *testing.T) {
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID: "nvr",
			ChannelImouOverrides: []config.ChannelImouOverride{
				{Channel: 5, DeviceID: "serial", ChannelID: "0", Features: []string{"light", "warning_light", "siren"}},
			},
		},
		imou:    &imouServiceStub{},
		imouCfg: config.ImouConfig{Enabled: true},
		logger:  zerolog.Nop(),
	}

	capabilities, err := driver.auxCapabilities(context.Background(), 5, dahua.NVRPTZCapabilities{})
	if err != nil {
		t.Fatalf("auxCapabilities returned error: %v", err)
	}
	if !capabilities.Supported {
		t.Fatalf("expected supported capabilities %+v", capabilities)
	}
	if len(capabilities.Outputs) != 2 || capabilities.Outputs[0] != "aux" || capabilities.Outputs[1] != "light" {
		t.Fatalf("unexpected outputs %+v", capabilities.Outputs)
	}
	if len(capabilities.Features) != 3 {
		t.Fatalf("unexpected features %+v", capabilities.Features)
	}
}

func TestDriverAuxUsesImouOverride(t *testing.T) {
	imouStub := &imouServiceStub{}
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID:               "nvr",
			ChannelAllowlist: []int{5},
			ChannelImouOverrides: []config.ChannelImouOverride{
				{Channel: 5, DeviceID: "serial", ChannelID: "0", Features: []string{"warning_light"}},
			},
		},
		imou:    imouStub,
		imouCfg: config.ImouConfig{Enabled: true},
		logger:  zerolog.Nop(),
	}

	err := driver.Aux(context.Background(), dahua.NVRAuxRequest{
		Channel: 5,
		Action:  dahua.NVRAuxActionStart,
		Output:  "warning_light",
	})
	if err != nil {
		t.Fatalf("Aux returned error: %v", err)
	}
	if len(imouStub.statuses) != 1 {
		t.Fatalf("expected 1 imou status call, got %+v", imouStub.statuses)
	}
	if imouStub.statuses[0].EnableType != "linkageWhiteLight" || !imouStub.statuses[0].Enable {
		t.Fatalf("unexpected imou change %+v", imouStub.statuses[0])
	}
}

func TestDriverAuxLightUsesNightVisionModeForImouOverride(t *testing.T) {
	imouStub := &imouServiceStub{
		mode: imou.NightVisionMode{Mode: "SmartLowLight", Modes: []string{"SmartLowLight", "FullColor", "Infrared"}},
	}
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID:               "nvr",
			ChannelAllowlist: []int{5},
			ChannelImouOverrides: []config.ChannelImouOverride{
				{Channel: 5, DeviceID: "serial", ChannelID: "0", Features: []string{"light"}},
			},
		},
		imou:    imouStub,
		imouCfg: config.ImouConfig{Enabled: true},
		logger:  zerolog.Nop(),
	}

	if err := driver.Aux(context.Background(), dahua.NVRAuxRequest{
		Channel: 5,
		Action:  dahua.NVRAuxActionStart,
		Output:  "light",
	}); err != nil {
		t.Fatalf("start Aux returned error: %v", err)
	}
	if len(imouStub.nightModes) != 1 || imouStub.nightModes[0].Mode != "FullColor" {
		t.Fatalf("unexpected night mode changes %+v", imouStub.nightModes)
	}

	if err := driver.Aux(context.Background(), dahua.NVRAuxRequest{
		Channel: 5,
		Action:  dahua.NVRAuxActionStop,
		Output:  "light",
	}); err != nil {
		t.Fatalf("stop Aux returned error: %v", err)
	}
	if len(imouStub.nightModes) != 2 || imouStub.nightModes[1].Mode != "SmartLowLight" {
		t.Fatalf("unexpected night mode changes %+v", imouStub.nightModes)
	}
}

func TestDriverSetAudioMuteUsesImouAudioEncodeControl(t *testing.T) {
	imouStub := &imouServiceStub{audioOn: true}
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID:               "nvr",
			ChannelAllowlist: []int{5},
			ChannelImouOverrides: []config.ChannelImouOverride{
				{Channel: 5, DeviceID: "serial", ChannelID: "0", Features: []string{"events"}},
			},
		},
		imou:    imouStub,
		imouCfg: config.ImouConfig{Enabled: true},
		logger:  zerolog.Nop(),
	}

	capabilities, notes := driver.audioCapabilities(context.Background(), 5)
	if !capabilities.Mute || !capabilities.StreamEnabled || capabilities.Muted {
		t.Fatalf("unexpected audio capabilities %+v", capabilities)
	}
	if len(notes) == 0 {
		t.Fatalf("expected audio capability notes, got %+v", notes)
	}

	if err := driver.SetAudioMute(context.Background(), dahua.NVRAudioRequest{
		Channel: 5,
		Muted:   true,
	}); err != nil {
		t.Fatalf("SetAudioMute returned error: %v", err)
	}
	if len(imouStub.statuses) != 1 || imouStub.statuses[0].EnableType != "audioEncodeControl" || imouStub.statuses[0].Enable {
		t.Fatalf("unexpected audio status changes %+v", imouStub.statuses)
	}
}

func TestShouldSuppressLocalEventWhenImouEventsOverrideChannel(t *testing.T) {
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID: "nvr",
			ChannelImouOverrides: []config.ChannelImouOverride{
				{Channel: 6, DeviceID: "serial", ChannelID: "1", Features: []string{"events"}},
			},
		},
		imou:    &imouServiceStub{},
		imouCfg: config.ImouConfig{Enabled: true, AlarmPollInterval: 15 * time.Second},
		logger:  zerolog.Nop(),
	}

	if !driver.shouldSuppressLocalEvent(dahua.Event{Channel: 6}) {
		t.Fatal("expected local event suppression for channel 6")
	}
	if driver.shouldSuppressLocalEvent(dahua.Event{Channel: 5}) {
		t.Fatal("did not expect local event suppression for channel 5")
	}
}
