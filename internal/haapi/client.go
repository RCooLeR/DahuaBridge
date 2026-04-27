package haapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

type ONVIFProvisionRequest struct {
	DeviceIDs []string `json:"device_ids,omitempty"`
	Force     bool     `json:"force,omitempty"`
}

type ONVIFProvisionTarget struct {
	DeviceID   string           `json:"device_id"`
	DeviceKind dahua.DeviceKind `json:"device_kind"`
	Name       string           `json:"name"`
	Host       string           `json:"host"`
	Port       int              `json:"port"`
	Username   string           `json:"-"`
	Password   string           `json:"-"`
	Reason     string           `json:"reason,omitempty"`
}

type ONVIFProvisionResult struct {
	DeviceID   string           `json:"device_id"`
	DeviceKind dahua.DeviceKind `json:"device_kind"`
	Name       string           `json:"name"`
	Host       string           `json:"host"`
	Port       int              `json:"port"`
	Status     string           `json:"status"`
	EntryID    string           `json:"entry_id,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	Error      string           `json:"error,omitempty"`
}

type Client struct {
	baseURL     string
	accessToken string
	httpClient  *http.Client
}

func New(cfg config.HomeAssistantConfig) *Client {
	return &Client{
		baseURL:     strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/"),
		accessToken: strings.TrimSpace(cfg.AccessToken),
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != "" && c.accessToken != ""
}

func (c *Client) ProvisionONVIF(ctx context.Context, target ONVIFProvisionTarget) (ONVIFProvisionResult, error) {
	result := ONVIFProvisionResult{
		DeviceID:   target.DeviceID,
		DeviceKind: target.DeviceKind,
		Name:       target.Name,
		Host:       target.Host,
		Port:       target.Port,
	}

	if !c.Enabled() {
		result.Status = "error"
		result.Error = "home assistant api is not configured"
		return result, errors.New(result.Error)
	}

	flow, err := c.startConfigFlow(ctx, "onvif")
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result, err
	}

	flow, err = c.continueConfigFlow(ctx, flow.FlowID, map[string]any{"auto": false})
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result, err
	}

	flow, err = c.continueConfigFlow(ctx, flow.FlowID, map[string]any{
		"name":     target.Name,
		"host":     target.Host,
		"port":     target.Port,
		"username": target.Username,
		"password": target.Password,
	})
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result, err
	}

	switch flow.Type {
	case "create_entry":
		result.Status = "created"
		result.EntryID = flow.Result.EntryID
		return result, nil
	case "abort":
		result.Reason = flow.Reason
		if flow.Reason == "already_configured" {
			result.Status = "already_configured"
			return result, nil
		}
		result.Status = "error"
		result.Error = flowMessage(flow)
		return result, errors.New(result.Error)
	case "form":
		result.Status = "error"
		result.Error = flowMessage(flow)
		return result, errors.New(result.Error)
	default:
		result.Status = "error"
		result.Error = fmt.Sprintf("unexpected flow result type %q", flow.Type)
		return result, errors.New(result.Error)
	}
}

type flowResponse struct {
	Type                    string            `json:"type"`
	FlowID                  string            `json:"flow_id"`
	StepID                  string            `json:"step_id"`
	Reason                  string            `json:"reason"`
	Errors                  map[string]string `json:"errors"`
	DescriptionPlaceholders map[string]string `json:"description_placeholders"`
	Result                  flowEntryResult   `json:"result"`
}

type flowEntryResult struct {
	EntryID string `json:"entry_id"`
}

func (c *Client) startConfigFlow(ctx context.Context, handler string) (flowResponse, error) {
	var response flowResponse
	if err := c.postJSON(ctx, "/api/config/config_entries/flow", map[string]any{
		"handler":               handler,
		"show_advanced_options": false,
	}, &response); err != nil {
		return flowResponse{}, err
	}
	return response, nil
}

func (c *Client) continueConfigFlow(ctx context.Context, flowID string, payload map[string]any) (flowResponse, error) {
	var response flowResponse
	if err := c.postJSON(ctx, "/api/config/config_entries/flow/"+flowID, payload, &response); err != nil {
		return flowResponse{}, err
	}
	return response, nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payloadBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(payloadBody))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("home assistant api %s returned %s: %s", path, resp.Status, message)
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payloadBody, out); err != nil {
		return fmt.Errorf("decode home assistant api %s response: %w", path, err)
	}
	return nil
}

func flowMessage(flow flowResponse) string {
	if len(flow.Errors) > 0 {
		parts := make([]string, 0, len(flow.Errors))
		for key, value := range flow.Errors {
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
		message := strings.Join(parts, ", ")
		if extra := strings.TrimSpace(flow.DescriptionPlaceholders["error"]); extra != "" {
			return message + ": " + extra
		}
		return message
	}
	if extra := strings.TrimSpace(flow.DescriptionPlaceholders["error"]); extra != "" {
		return extra
	}
	if flow.Reason != "" {
		return flow.Reason
	}
	if flow.StepID != "" {
		return fmt.Sprintf("flow requires additional input at step %q", flow.StepID)
	}
	return "unknown flow error"
}
