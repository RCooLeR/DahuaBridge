package imou

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
)

const alarmTimeLayout = "2006-01-02 15:04:05"

type Service interface {
	Enabled() bool
	GetCameraStatus(context.Context, CameraStatusRequest) (CameraStatus, error)
	SetCameraStatus(context.Context, CameraStatusChange) error
	GetNightVisionMode(context.Context, NightVisionModeRequest) (NightVisionMode, error)
	SetNightVisionMode(context.Context, NightVisionModeChange) error
	ListAlarms(context.Context, AlarmQuery) ([]Alarm, error)
}

type Client struct {
	cfg        config.ImouConfig
	httpClient *http.Client

	tokenMu      sync.Mutex
	accessToken  string
	tokenExpires time.Time
}

type AuthState struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type CameraStatusRequest struct {
	DeviceID   string
	ChannelID  string
	EnableType string
}

type CameraStatusChange struct {
	DeviceID   string
	ChannelID  string
	EnableType string
	Enable     bool
}

type CameraStatus struct {
	DeviceID   string
	ChannelID  string
	EnableType string
	Enabled    bool
}

type NightVisionModeRequest struct {
	DeviceID  string
	ChannelID string
}

type NightVisionModeChange struct {
	DeviceID  string
	ChannelID string
	Mode      string
}

type NightVisionMode struct {
	DeviceID  string
	ChannelID string
	Mode      string
	Modes     []string
}

type AlarmQuery struct {
	DeviceID  string
	ChannelID string
	BeginTime time.Time
	EndTime   time.Time
	Count     int
}

type Alarm struct {
	AlarmID   string
	DeviceID  string
	ChannelID string
	Type      int
	Time      time.Time
	LocalDate string
	Token     string
}

type openAPIResponse struct {
	Result struct {
		Code any             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	} `json:"result"`
	ID string `json:"id"`
}

func NewClient(cfg config.ImouConfig) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.cfg.Enabled && c.cfg.AppID != "" && c.cfg.AppSecret != ""
}

func (c *Client) ExportAuthState() *AuthState {
	if c == nil {
		return nil
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if strings.TrimSpace(c.accessToken) == "" || c.tokenExpires.IsZero() {
		return nil
	}

	return &AuthState{
		AccessToken: c.accessToken,
		ExpiresAt:   c.tokenExpires.UTC(),
	}
}

func (c *Client) ImportAuthState(state *AuthState) {
	if c == nil || state == nil {
		return
	}

	accessToken := strings.TrimSpace(state.AccessToken)
	if accessToken == "" || state.ExpiresAt.IsZero() {
		return
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	c.accessToken = accessToken
	c.tokenExpires = state.ExpiresAt.UTC()
}

func (c *Client) GetCameraStatus(ctx context.Context, request CameraStatusRequest) (CameraStatus, error) {
	if !c.Enabled() {
		return CameraStatus{}, fmt.Errorf("imou client is disabled")
	}

	token, err := c.ensureAccessToken(ctx)
	if err != nil {
		return CameraStatus{}, err
	}

	var data struct {
		EnableType string `json:"enableType"`
		Status     string `json:"status"`
	}
	if err := c.call(ctx, "getDeviceCameraStatus", map[string]any{
		"token":      token,
		"deviceId":   strings.TrimSpace(request.DeviceID),
		"channelId":  strings.TrimSpace(request.ChannelID),
		"enableType": strings.TrimSpace(request.EnableType),
	}, &data); err != nil {
		return CameraStatus{}, err
	}

	return CameraStatus{
		DeviceID:   strings.TrimSpace(request.DeviceID),
		ChannelID:  strings.TrimSpace(request.ChannelID),
		EnableType: data.EnableType,
		Enabled:    strings.EqualFold(strings.TrimSpace(data.Status), "on"),
	}, nil
}

func (c *Client) SetCameraStatus(ctx context.Context, request CameraStatusChange) error {
	if !c.Enabled() {
		return fmt.Errorf("imou client is disabled")
	}

	token, err := c.ensureAccessToken(ctx)
	if err != nil {
		return err
	}

	return c.call(ctx, "setDeviceCameraStatus", map[string]any{
		"token":      token,
		"deviceId":   strings.TrimSpace(request.DeviceID),
		"channelId":  strings.TrimSpace(request.ChannelID),
		"enableType": strings.TrimSpace(request.EnableType),
		"enable":     request.Enable,
	}, nil)
}

func (c *Client) GetNightVisionMode(ctx context.Context, request NightVisionModeRequest) (NightVisionMode, error) {
	if !c.Enabled() {
		return NightVisionMode{}, fmt.Errorf("imou client is disabled")
	}

	token, err := c.ensureAccessToken(ctx)
	if err != nil {
		return NightVisionMode{}, err
	}

	var data struct {
		Mode  string   `json:"mode"`
		Modes []string `json:"modes"`
	}
	if err := c.call(ctx, "getNightVisionMode", map[string]any{
		"token":     token,
		"deviceId":  strings.TrimSpace(request.DeviceID),
		"channelId": strings.TrimSpace(request.ChannelID),
	}, &data); err != nil {
		return NightVisionMode{}, err
	}

	return NightVisionMode{
		DeviceID:  strings.TrimSpace(request.DeviceID),
		ChannelID: strings.TrimSpace(request.ChannelID),
		Mode:      strings.TrimSpace(data.Mode),
		Modes:     normalizeModes(data.Modes),
	}, nil
}

func (c *Client) SetNightVisionMode(ctx context.Context, request NightVisionModeChange) error {
	if !c.Enabled() {
		return fmt.Errorf("imou client is disabled")
	}

	token, err := c.ensureAccessToken(ctx)
	if err != nil {
		return err
	}

	return c.call(ctx, "setNightVisionMode", map[string]any{
		"token":     token,
		"deviceId":  strings.TrimSpace(request.DeviceID),
		"channelId": strings.TrimSpace(request.ChannelID),
		"mode":      strings.TrimSpace(request.Mode),
	}, nil)
}

func (c *Client) ListAlarms(ctx context.Context, query AlarmQuery) ([]Alarm, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("imou client is disabled")
	}

	token, err := c.ensureAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	count := query.Count
	if count <= 0 || count > 30 {
		count = 30
	}

	alarms := make([]Alarm, 0, count)
	nextAlarmID := ""
	for page := 0; page < 5; page++ {
		var data struct {
			Count       int `json:"count"`
			NextAlarmID any `json:"nextAlarmId"`
			Alarms      []struct {
				AlarmID   any    `json:"alarmId"`
				Time      int64  `json:"time"`
				ChannelID any    `json:"channelId"`
				Type      any    `json:"type"`
				DeviceID  string `json:"deviceId"`
				LocalDate string `json:"localDate"`
				Token     string `json:"token"`
			} `json:"alarms"`
		}

		params := map[string]any{
			"token":       token,
			"deviceId":    strings.TrimSpace(query.DeviceID),
			"channelId":   strings.TrimSpace(query.ChannelID),
			"beginTime":   query.BeginTime.In(time.Local).Format(alarmTimeLayout),
			"endTime":     query.EndTime.In(time.Local).Format(alarmTimeLayout),
			"count":       count,
			"nextAlarmId": nextAlarmID,
		}
		if err := c.call(ctx, "getAlarmMessage", params, &data); err != nil {
			return nil, err
		}

		for _, item := range data.Alarms {
			alarms = append(alarms, Alarm{
				AlarmID:   strings.TrimSpace(fmt.Sprint(item.AlarmID)),
				DeviceID:  firstNonEmpty(item.DeviceID, strings.TrimSpace(query.DeviceID)),
				ChannelID: strings.TrimSpace(fmt.Sprint(item.ChannelID)),
				Type:      parseInt(item.Type),
				Time:      time.Unix(item.Time, 0).UTC(),
				LocalDate: strings.TrimSpace(item.LocalDate),
				Token:     strings.TrimSpace(item.Token),
			})
		}

		nextAlarmID = strings.TrimSpace(fmt.Sprint(data.NextAlarmID))
		if len(data.Alarms) < count || nextAlarmID == "" || nextAlarmID == "-1" {
			break
		}
	}

	sort.Slice(alarms, func(i, j int) bool {
		if alarms[i].Time.Equal(alarms[j].Time) {
			return alarms[i].AlarmID < alarms[j].AlarmID
		}
		return alarms[i].Time.Before(alarms[j].Time)
	})
	return alarms, nil
}

func (c *Client) ensureAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if strings.TrimSpace(c.accessToken) != "" && time.Until(c.tokenExpires) > time.Minute {
		return c.accessToken, nil
	}

	var data struct {
		AccessToken string `json:"accessToken"`
		ExpireTime  int64  `json:"expireTime"`
	}
	if err := c.call(ctx, "accessToken", map[string]any{}, &data); err != nil {
		return "", err
	}
	c.accessToken = strings.TrimSpace(data.AccessToken)
	if c.accessToken == "" {
		return "", fmt.Errorf("imou accessToken response did not include accessToken")
	}
	expiresIn := time.Duration(data.ExpireTime) * time.Second
	if expiresIn <= 0 {
		expiresIn = 72 * time.Hour
	}
	c.tokenExpires = time.Now().Add(expiresIn)
	return c.accessToken, nil
}

func (c *Client) call(ctx context.Context, method string, params map[string]any, out any) error {
	currentTime := time.Now().UTC().Unix()
	currentNonce := nonce()
	body, err := json.Marshal(map[string]any{
		"system": map[string]any{
			"ver":   "1.0",
			"appId": c.cfg.AppID,
			"sign":  c.sign(currentTime, currentNonce),
			"time":  currentTime,
			"nonce": currentNonce,
		},
		"id":     nonce(),
		"params": params,
	})
	if err != nil {
		return fmt.Errorf("marshal imou %s request: %w", method, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/openapi/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build imou %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call imou %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("imou %s returned HTTP %d", method, resp.StatusCode)
	}

	var payload openAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode imou %s response: %w", method, err)
	}
	if code := strings.TrimSpace(fmt.Sprint(payload.Result.Code)); code != "0" {
		return fmt.Errorf("imou %s failed with code %s: %s", method, code, strings.TrimSpace(payload.Result.Msg))
	}
	if out == nil || len(payload.Result.Data) == 0 || string(payload.Result.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(payload.Result.Data, out); err != nil {
		return fmt.Errorf("decode imou %s data: %w", method, err)
	}
	return nil
}

func (c *Client) baseURL() string {
	if strings.TrimSpace(c.cfg.Endpoint) != "" {
		return strings.TrimRight(strings.TrimSpace(c.cfg.Endpoint), "/")
	}
	switch strings.ToLower(strings.TrimSpace(c.cfg.DataCenter)) {
	case "sg":
		return "https://openapi-sg.easy4ip.com"
	case "or":
		return "https://openapi-or.easy4ip.com"
	default:
		return "https://openapi-fk.easy4ip.com"
	}
}

func (c *Client) sign(currentTime int64, currentNonce string) string {
	sum := md5.Sum([]byte(fmt.Sprintf("time:%d,nonce:%s,appSecret:%s", currentTime, currentNonce, c.cfg.AppSecret)))
	return hex.EncodeToString(sum[:])
}

func nonce() string {
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}

func parseInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeModes(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}
