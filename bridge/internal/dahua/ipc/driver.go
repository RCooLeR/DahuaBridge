package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
	"RCooLeR/DahuaBridge/internal/dahua/rpc"
	dahuartsp "RCooLeR/DahuaBridge/internal/dahua/rtsp"
	"RCooLeR/DahuaBridge/internal/onvif"
	"github.com/rs/zerolog"
)

type Driver struct {
	mu     sync.RWMutex
	cfg    config.DeviceConfig
	cgi    *cgi.Client
	rpc    *rpc.Client
	rtsp   *dahuartsp.Checker
	onvif  *onvif.Client
	logger zerolog.Logger
}

func New(cfg config.DeviceConfig, logger zerolog.Logger, client *cgi.Client) *Driver {
	return &Driver{
		cfg:    cfg,
		cgi:    client,
		rpc:    rpc.New(cfg),
		rtsp:   dahuartsp.NewChecker(cfg),
		onvif:  onvif.New(cfg),
		logger: logger.With().Str("device_id", cfg.ID).Str("device_type", string(dahua.DeviceKindIPC)).Logger(),
	}
}

func (d *Driver) ID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg.ID
}

func (d *Driver) Kind() dahua.DeviceKind {
	return dahua.DeviceKindIPC
}

func (d *Driver) PollInterval() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg.PollInterval
}

func (d *Driver) Probe(ctx context.Context) (*dahua.ProbeResult, error) {
	cfg := d.currentConfig()
	systemInfoRaw := map[string]any{}
	if err := d.rpc.Call(ctx, "magicBox.getSystemInfoNew", nil, &systemInfoRaw); err != nil {
		return nil, fmt.Errorf("fetch rpc system info: %w", err)
	}
	systemInfo := normalizeRPCMap(systemInfoRaw, "info")

	softwareInfoRaw := map[string]any{}
	if err := d.rpc.Call(ctx, "magicBox.getSoftwareVersion", nil, &softwareInfoRaw); err != nil {
		d.logger.Warn().Err(err).Msg("software version probe failed")
	}
	softwareInfo := normalizeRPCMap(softwareInfoRaw, "version")

	hardwareVersion := ""
	if value, err := d.callStringValue(ctx, "magicBox.getHardwareVersion", "version", "Version"); err != nil {
		d.logger.Warn().Err(err).Msg("hardware version probe failed")
	} else {
		hardwareVersion = value
	}

	deviceType := ""
	if value, err := d.callStringValue(ctx, "magicBox.getDeviceType", "type", "deviceType"); err != nil {
		d.logger.Warn().Err(err).Msg("device type probe failed")
	} else {
		deviceType = value
	}

	deviceClass := ""
	if value, err := d.callStringValue(ctx, "magicBox.getDeviceClass", "type", "deviceClass"); err != nil {
		d.logger.Warn().Err(err).Msg("device class probe failed")
	} else {
		deviceClass = value
	}

	processInfo := ""
	if value, err := d.callStringValue(ctx, "magicBox.getProcessInfo", "info", "processInfo"); err != nil {
		d.logger.Warn().Err(err).Msg("process info probe failed")
	} else {
		processInfo = value
	}

	serial := stringFromAny(systemInfo["serialNumber"])
	name := firstNonEmpty(
		stringFromAny(systemInfo["machineName"]),
		stringFromAny(systemInfo["name"]),
		cfg.Name,
		cfg.ID,
	)
	model := firstNonEmpty(
		cfg.Model,
		stringFromAny(systemInfo["deviceModel"]),
		stringFromAny(systemInfo["deviceType"]),
		deviceType,
		"IPC",
	)
	firmware, buildDate := parseSoftwareInfo(softwareInfo)

	rawSoftware, _ := json.Marshal(softwareInfoRaw)
	rawSystem, _ := json.Marshal(systemInfo)

	root := dahua.Device{
		ID:           cfg.ID,
		Name:         name,
		Manufacturer: cfg.Manufacturer,
		Model:        model,
		Serial:       serial,
		Firmware:     firmware,
		BuildDate:    buildDate,
		BaseURL:      cfg.BaseURL,
		Kind:         dahua.DeviceKindIPC,
		Attributes: map[string]string{
			"device_type":      firstNonEmpty(deviceType, stringFromAny(systemInfo["deviceType"])),
			"device_class":     deviceClass,
			"hardware_version": hardwareVersion,
			"process_info":     processInfo,
			"snapshot_path":    fmt.Sprintf("/api/v1/ipc/%s/snapshot", cfg.ID),
			"rtsp_main_url":    buildRTSPURL(cfg.BaseURL, 1, 0),
			"rtsp_sub_url":     buildRTSPURL(cfg.BaseURL, 1, 1),
			"probe_source":     "rpc2",
		},
	}

	states := map[string]dahua.DeviceState{
		cfg.ID: {
			Available: true,
			Info: map[string]any{
				"name":             name,
				"serial":           serial,
				"firmware":         firmware,
				"build_date":       buildDate,
				"device_type":      root.Attributes["device_type"],
				"device_class":     deviceClass,
				"hardware_version": hardwareVersion,
				"process_info":     processInfo,
				"rtsp_main_url":    buildRTSPURL(cfg.BaseURL, 1, 0),
				"rtsp_sub_url":     buildRTSPURL(cfg.BaseURL, 1, 1),
				"snapshot_rel_url": fmt.Sprintf("/api/v1/ipc/%s/snapshot", cfg.ID),
				"stream_available": d.streamAvailable(ctx, cfg),
			},
		},
	}

	raw := map[string]string{
		"rpc_system_info":      strings.TrimSpace(string(rawSystem)),
		"rpc_software_version": strings.TrimSpace(string(rawSoftware)),
		"device_type":          root.Attributes["device_type"],
		"device_class":         deviceClass,
		"hardware_version":     hardwareVersion,
		"process_info":         processInfo,
	}

	if d.onvif.Enabled() {
		discovery, err := d.onvif.Discover(ctx)
		if err != nil {
			d.logger.Warn().Err(err).Msg("onvif probe failed")
		} else {
			root.Attributes["onvif_h264_profile_count"] = strconv.Itoa(discovery.H264ProfileCount())
			root.Attributes["onvif_device_service_url"] = discovery.DeviceServiceURL
			root.Attributes["onvif_media_service_url"] = discovery.MediaServiceURL
			raw["onvif_device_service_url"] = discovery.DeviceServiceURL
			raw["onvif_media_service_url"] = discovery.MediaServiceURL
			raw["onvif_h264_profile_count"] = strconv.Itoa(discovery.H264ProfileCount())

			state := states[cfg.ID]
			state.Info["onvif_probed"] = true
			state.Info["onvif_device_service_url"] = discovery.DeviceServiceURL
			state.Info["onvif_media_service_url"] = discovery.MediaServiceURL
			state.Info["onvif_h264_profile_count"] = discovery.H264ProfileCount()
			state.Info["onvif_profiles"] = discovery.ProfileMaps()

			best, ok := discovery.BestH264ProfileForChannel(1)
			if ok {
				root.Attributes["onvif_h264_available"] = "true"
				root.Attributes["onvif_profile_token"] = best.Token
				root.Attributes["onvif_profile_name"] = best.Name
				root.Attributes["onvif_stream_url"] = best.StreamURI
				if best.SnapshotURI != "" {
					root.Attributes["onvif_snapshot_url"] = best.SnapshotURI
				}
				root.Attributes["recommended_ha_integration"] = "onvif"

				state.Info["onvif_h264_available"] = true
				state.Info["onvif_profile_token"] = best.Token
				state.Info["onvif_profile_name"] = best.Name
				state.Info["onvif_stream_url"] = best.StreamURI
				if best.SnapshotURI != "" {
					state.Info["onvif_snapshot_url"] = best.SnapshotURI
				}
				state.Info["recommended_ha_integration"] = "onvif"
			} else {
				root.Attributes["onvif_h264_available"] = "false"
				root.Attributes["recommended_ha_integration"] = "bridge_media"
				state.Info["onvif_h264_available"] = false
				state.Info["recommended_ha_integration"] = "bridge_media"
			}

			states[cfg.ID] = state
		}
	}

	return &dahua.ProbeResult{
		Root:   root,
		States: states,
		Raw:    raw,
	}, nil
}

func (d *Driver) callStringValue(ctx context.Context, method string, keys ...string) (string, error) {
	payload := map[string]any{}
	if err := d.rpc.Call(ctx, method, nil, &payload); err != nil {
		return "", err
	}

	for _, key := range keys {
		if value := stringFromAny(payload[key]); value != "" {
			return value, nil
		}
	}

	for _, key := range keys {
		if nested, ok := payload[key].(map[string]any); ok {
			for _, nestedKey := range keys {
				if value := stringFromAny(nested[nestedKey]); value != "" {
					return value, nil
				}
			}
			if value := firstStringInMap(nested); value != "" {
				return value, nil
			}
		}
	}

	if value := firstStringInMap(payload); value != "" {
		return value, nil
	}

	return "", nil
}

func (d *Driver) Snapshot(ctx context.Context, _ int) ([]byte, string, error) {
	body, contentType, err := d.cgi.GetBinary(ctx, "/cgi-bin/snapshot.cgi", nil)
	if err == nil {
		return body, contentType, nil
	}

	return nil, "", fmt.Errorf("fetch ipc snapshot: %w", err)
}

func (d *Driver) UpdateConfig(cfg config.DeviceConfig) error {
	d.mu.Lock()
	d.cfg = cfg
	d.mu.Unlock()
	d.cgi.UpdateConfig(cfg)
	d.rpc.UpdateConfig(cfg)
	d.rtsp.UpdateConfig(cfg)
	d.onvif.UpdateConfig(cfg)
	return nil
}

func parseSoftwareInfo(values map[string]any) (string, string) {
	version := firstNonEmpty(
		stringFromAny(values["version"]),
		stringFromAny(values["Version"]),
	)
	build := firstNonEmpty(
		stringFromAny(values["build"]),
		stringFromAny(values["Build"]),
		stringFromAny(values["buildDate"]),
		stringFromAny(values["BuildDate"]),
	)
	return version, build
}

func normalizeRPCMap(values map[string]any, nestedKey string) map[string]any {
	if nested, ok := values[nestedKey].(map[string]any); ok {
		return nested
	}
	return values
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func firstStringInMap(values map[string]any) string {
	for _, value := range values {
		if nested, ok := value.(map[string]any); ok {
			if candidate := firstStringInMap(nested); candidate != "" {
				return candidate
			}
			continue
		}
		if candidate := stringFromAny(value); candidate != "" {
			return candidate
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildRTSPURL(baseURL string, channel int, subtype int) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}

	host := parsed.Hostname()
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		host = net.JoinHostPort(host, port)
	} else {
		host = net.JoinHostPort(host, "554")
	}

	rtspURL := &url.URL{
		Scheme:   "rtsp",
		Host:     host,
		Path:     "/cam/realmonitor",
		RawQuery: url.Values{"channel": []string{strconv.Itoa(channel)}, "subtype": []string{strconv.Itoa(subtype)}}.Encode(),
	}
	return rtspURL.String()
}

func (d *Driver) streamAvailable(ctx context.Context, cfg config.DeviceConfig) bool {
	available, err := d.rtsp.StreamAvailable(
		ctx,
		buildRTSPURL(cfg.BaseURL, 1, 1),
		buildRTSPURL(cfg.BaseURL, 1, 0),
	)
	if err != nil {
		d.logger.Debug().Err(err).Msg("rtsp availability probe failed")
	}
	return available
}

var _ dahua.Driver = (*Driver)(nil)
var _ dahua.SnapshotProvider = (*Driver)(nil)
var _ dahua.ConfigurableDriver = (*Driver)(nil)

func (d *Driver) currentConfig() config.DeviceConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}
