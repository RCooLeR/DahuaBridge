package nvr

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
)

type remoteDeviceInventory struct {
	Index      int
	Address    string
	DeviceType string
	HTTPPort   int
	HTTPSPort  int
	SDKPort    int
	RTSPPort   int
	UserName   string
	Name       string
}

type directIPCTarget struct {
	Channel     int
	Address     string
	DeviceType  string
	BaseURL     string
	UserName    string
	Password    string
	HTTPPort    int
	HTTPSPort   int
	InsecureTLS bool
}

type directIPCLightProfile struct {
	Channel     int
	ProfileMode int
	Index       int
	LightType   string
}

func parseRemoteDevices(values map[string]string) map[int]remoteDeviceInventory {
	result := make(map[int]remoteDeviceInventory)
	for key, value := range values {
		index, field, ok := remoteDeviceField(key)
		if !ok {
			continue
		}
		item := result[index]
		item.Index = index
		switch field {
		case "Address":
			item.Address = strings.TrimSpace(value)
		case "DeviceType":
			item.DeviceType = strings.TrimSpace(value)
		case "HttpPort":
			item.HTTPPort = parsedIntOrZero(value)
		case "HttpsPort":
			item.HTTPSPort = parsedIntOrZero(value)
		case "Port":
			item.SDKPort = parsedIntOrZero(value)
		case "RtspPort":
			item.RTSPPort = parsedIntOrZero(value)
		case "UserName":
			item.UserName = strings.TrimSpace(value)
		case "VideoInputs[0].Name":
			item.Name = strings.TrimSpace(value)
		}
		result[index] = item
	}
	return result
}

func remoteDeviceField(key string) (int, string, bool) {
	if matches := remoteDevicePattern.FindStringSubmatch(key); len(matches) == 3 {
		index, ok := parseInt(matches[1])
		if !ok {
			return 0, "", false
		}
		return index, matches[2], true
	}
	if matches := remoteDeviceLegacyPattern.FindStringSubmatch(key); len(matches) == 3 {
		index, ok := parseInt(matches[1])
		if !ok {
			return 0, "", false
		}
		return index, matches[2], true
	}
	return 0, "", false
}

func (d *Driver) directIPCTargetForChannel(ctx context.Context, channel int) (*directIPCTarget, error) {
	cfg := d.currentConfig()
	credential, ok := cfg.DirectIPCCredential(channel)
	if !ok {
		return nil, nil
	}

	inventory, err := d.loadInventory(ctx)
	if err != nil {
		return nil, err
	}

	target := &directIPCTarget{
		Channel:     channel,
		Address:     credential.DirectIPCIP,
		BaseURL:     credential.DirectIPCBaseURL,
		UserName:    credential.DirectIPCUser,
		Password:    credential.DirectIPCPassword,
		InsecureTLS: cfg.InsecureSkipTLS,
	}

	for _, item := range inventory {
		if item.Index+1 != channel {
			continue
		}
		target.DeviceType = strings.TrimSpace(item.RemoteDevice.DeviceType)
		target.HTTPPort = item.RemoteDevice.HTTPPort
		target.HTTPSPort = item.RemoteDevice.HTTPSPort
		if strings.TrimSpace(item.RemoteDevice.Address) != "" {
			target.Address = strings.TrimSpace(item.RemoteDevice.Address)
		}
		break
	}

	if ipcCfg, ok := d.configuredIPCForAddress(target.Address, credential.DirectIPCIP); ok {
		target.BaseURL = ipcCfg.BaseURL
		target.InsecureTLS = ipcCfg.InsecureSkipTLS
		if strings.TrimSpace(target.UserName) == "" {
			target.UserName = ipcCfg.Username
		}
		if strings.TrimSpace(target.Password) == "" {
			target.Password = ipcCfg.Password
		}
		return target, nil
	}

	if strings.TrimSpace(target.BaseURL) == "" {
		target.BaseURL = directIPCBaseURL(credential.DirectIPCIP, target.Address, target.HTTPPort, target.HTTPSPort)
	}
	return target, nil
}

func (d *Driver) configuredIPCForAddress(addresses ...string) (config.DeviceConfig, bool) {
	if len(d.ipcCfgs) == 0 {
		return config.DeviceConfig{}, false
	}
	needles := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		host := normalizedDirectIPCHost(address)
		if host != "" {
			needles[host] = struct{}{}
		}
	}
	if len(needles) == 0 {
		return config.DeviceConfig{}, false
	}
	for _, ipcCfg := range d.ipcCfgs {
		host := normalizedDirectIPCHost(ipcCfg.BaseURL)
		if host == "" {
			continue
		}
		if _, ok := needles[host]; ok {
			return ipcCfg, true
		}
	}
	return config.DeviceConfig{}, false
}

func directIPCBaseURL(configuredAddress string, discoveredAddress string, httpPort int, httpsPort int) string {
	if normalized := normalizedDirectIPCBaseURL(configuredAddress); normalized != "" {
		return normalized
	}
	if authority := normalizedDirectIPCAuthority(configuredAddress); authority != "" {
		return fmt.Sprintf("http://%s", authority)
	}

	host := normalizedDirectIPCHost(configuredAddress)
	if host == "" {
		host = normalizedDirectIPCHost(discoveredAddress)
	}
	if host == "" {
		return ""
	}

	scheme := "http"
	port := httpPort
	if port <= 0 && httpsPort > 0 {
		scheme = "https"
		port = httpsPort
	}
	if port > 0 {
		return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(port)))
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func normalizedDirectIPCAuthority(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.Contains(trimmed, "://") {
		return ""
	}
	if _, _, err := net.SplitHostPort(trimmed); err == nil {
		return trimmed
	}
	return ""
}

func normalizedDirectIPCBaseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !strings.Contains(trimmed, "://") {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return ""
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func normalizedDirectIPCHost(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(parsed.Hostname())
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		return strings.TrimSpace(host)
	}
	return trimmed
}

func (d *Driver) directIPCClient(target *directIPCTarget) *cgi.Client {
	if target == nil {
		return nil
	}
	requestTimeout := d.currentConfig().RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultDirectIPCRequestTimeout
	}
	return cgi.New(config.DeviceConfig{
		ID:              fmt.Sprintf("%s_channel_%02d_direct_ipc", d.ID(), target.Channel),
		BaseURL:         target.BaseURL,
		Username:        target.UserName,
		Password:        target.Password,
		RequestTimeout:  requestTimeout,
		InsecureSkipTLS: target.InsecureTLS,
		Manufacturer:    "Dahua",
	}, d.metrics)
}

func (d *Driver) directIPCAudioWritable(ctx context.Context, channel int) bool {
	target, err := d.directIPCTargetForChannel(ctx, channel)
	return err == nil && target != nil
}

func (d *Driver) setDirectIPCAudioEnabled(ctx context.Context, channel int, enabled bool) error {
	target, err := d.directIPCTargetForChannel(ctx, channel)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("%w: direct ipc audio control is not configured on channel %d", dahua.ErrUnsupportedOperation, channel)
	}

	client := d.directIPCClient(target)
	values, err := client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"Encode"},
	})
	if err != nil {
		return err
	}

	keys := directIPCAudioEnableKeys(values)
	if len(keys) == 0 {
		return fmt.Errorf("%w: direct ipc audio control is not exposed on channel %d", dahua.ErrUnsupportedOperation, channel)
	}

	for _, tablePrefix := range []bool{true, false} {
		body, err := client.GetText(ctx, "/cgi-bin/configManager.cgi", directIPCAudioEnableQuery(keys, enabled, tablePrefix))
		if err != nil {
			if isUnsupportedRecordConfigError(err) {
				continue
			}
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(body), "OK") {
			return fmt.Errorf("direct ipc audio action returned %q", strings.TrimSpace(body))
		}
		d.InvalidateInventoryCache()
		return nil
	}
	return dahua.ErrUnsupportedOperation
}

func directIPCAudioEnableKeys(values map[string]string) []string {
	keys := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	for key := range values {
		matches := encodeAudioEnablePattern.FindStringSubmatch(key)
		if len(matches) != 2 {
			continue
		}
		index, ok := parseInt(matches[1])
		if !ok || index != 0 {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		keys = append(keys, "table.Encode[0].MainFormat[0].AudioEnable")
	}
	sort.Strings(keys)
	return keys
}

func directIPCAudioEnableQuery(keys []string, enabled bool, tablePrefix bool) url.Values {
	return channelAudioEnableQuery(keys, enabled, tablePrefix)
}

func directIPCLightingSupported(deviceType string) bool {
	switch strings.TrimSpace(strings.ToUpper(deviceType)) {
	case "DH-T4A-PV", "DH-H4C-GE", "DH-IPC-HFW2849S-S-IL", "DH-IPC-HFW2849S-S-IL-BE":
		return true
	default:
		return false
	}
}

func (d *Driver) directIPCLightingWritable(ctx context.Context, channel int) bool {
	target, err := d.directIPCTargetForChannel(ctx, channel)
	return err == nil && target != nil && directIPCLightingSupported(target.DeviceType)
}

func (d *Driver) setDirectIPCLightingMode(ctx context.Context, channel int, action dahua.NVRAuxAction) error {
	target, err := d.directIPCTargetForChannel(ctx, channel)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("%w: direct ipc light control is not configured on channel %d", dahua.ErrUnsupportedOperation, channel)
	}
	if !directIPCLightingSupported(target.DeviceType) {
		return fmt.Errorf("%w: direct ipc light control is not verified on device type %q", dahua.ErrUnsupportedOperation, target.DeviceType)
	}

	client := d.directIPCClient(target)
	values, err := client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"Lighting_V2"},
	})
	if err != nil {
		return err
	}

	profile, ok := directIPCLightProfileForValues(values)
	if !ok {
		return fmt.Errorf("%w: direct ipc lighting config is not exposed on channel %d", dahua.ErrUnsupportedOperation, channel)
	}

	mode := "Off"
	brightness := "100"
	switch action {
	case dahua.NVRAuxActionStart:
		mode = "Manual"
	case dahua.NVRAuxActionStop:
		mode = "Off"
	default:
		return fmt.Errorf("%w: direct ipc lighting mode only supports start and stop actions", dahua.ErrUnsupportedOperation)
	}

	query := url.Values{
		"action": []string{"setConfig"},
		fmt.Sprintf("Lighting_V2[%d][%d][%d].Mode", profile.Channel, profile.ProfileMode, profile.Index):                 []string{mode},
		fmt.Sprintf("Lighting_V2[%d][%d][%d].MiddleLight[0].Light", profile.Channel, profile.ProfileMode, profile.Index): []string{brightness},
	}
	body, err := client.GetText(ctx, "/cgi-bin/configManager.cgi", query)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(body), "OK") {
		return fmt.Errorf("direct ipc light action returned %q", strings.TrimSpace(body))
	}
	return nil
}

func directIPCLightProfileForValues(values map[string]string) (directIPCLightProfile, bool) {
	type candidate struct {
		directIPCLightProfile
		priority int
	}
	candidates := make([]candidate, 0, 4)
	for key, value := range values {
		if !strings.HasPrefix(key, "table.Lighting_V2[") || !strings.HasSuffix(key, "].LightType") {
			continue
		}
		profile, ok := parseDirectIPCLightProfileKey(key)
		if !ok {
			continue
		}
		profile.LightType = strings.TrimSpace(value)
		priority := 10
		switch profile.LightType {
		case "WhiteLight":
			priority = 0
		case "AIMixLight":
			priority = 1
		}
		candidates = append(candidates, candidate{directIPCLightProfile: profile, priority: priority})
	}
	if len(candidates) == 0 {
		return directIPCLightProfile{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		if candidates[i].ProfileMode != candidates[j].ProfileMode {
			return candidates[i].ProfileMode < candidates[j].ProfileMode
		}
		return candidates[i].Index < candidates[j].Index
	})
	return candidates[0].directIPCLightProfile, true
}

func parseDirectIPCLightProfileKey(key string) (directIPCLightProfile, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(key), ".LightType"), "table.Lighting_V2[")
	parts := strings.Split(trimmed, "][")
	if len(parts) != 3 {
		return directIPCLightProfile{}, false
	}
	channel, err := strconv.Atoi(strings.TrimSuffix(parts[0], "]"))
	if err != nil {
		return directIPCLightProfile{}, false
	}
	profileMode, err := strconv.Atoi(strings.TrimSuffix(parts[1], "]"))
	if err != nil {
		return directIPCLightProfile{}, false
	}
	index, err := strconv.Atoi(strings.TrimSuffix(parts[2], "]"))
	if err != nil {
		return directIPCLightProfile{}, false
	}
	return directIPCLightProfile{Channel: channel, ProfileMode: profileMode, Index: index}, true
}

const defaultDirectIPCRequestTimeout = 10 * time.Second
