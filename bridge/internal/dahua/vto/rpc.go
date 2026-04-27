package vto

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"

	"RCooLeR/DahuaBridge/internal/config"
	dahuatransport "RCooLeR/DahuaBridge/internal/dahua/transport"
)

const (
	vtoRPCLoginPath = "/RPC2_Login"
	vtoRPCPath      = "/RPC2"
)

type rpcClient struct {
	baseURL  string
	username string
	password string
	http     *http.Client

	mu      sync.Mutex
	nextID  int64
	session any
}

type rpcRequest struct {
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int64  `json:"id"`
	Session any    `json:"session"`
	Object  any    `json:"object,omitempty"`
}

type rpcResponse struct {
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Session any             `json:"session"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("rpc error code %d", e.Code)
	}
	return fmt.Sprintf("rpc error code %d: %s", e.Code, e.Message)
}

func newRPCClient(cfg config.DeviceConfig) (*rpcClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	return &rpcClient{
		baseURL:  cfg.BaseURL,
		username: cfg.Username,
		password: cfg.Password,
		http:     newRPCHTTPClient(cfg, jar),
		nextID:   1,
	}, nil
}

func (c *rpcClient) UpdateConfig(cfg config.DeviceConfig) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = cfg.BaseURL
	c.username = cfg.Username
	c.password = cfg.Password
	c.http = newRPCHTTPClient(cfg, jar)
	c.session = nil
	return nil
}

func (c *rpcClient) Call(ctx context.Context, method string, params any, target any) error {
	return c.call(ctx, c.buildRequest(method, params, nil), target)
}

func (c *rpcClient) CallObject(ctx context.Context, method string, params any, object any, target any) error {
	return c.call(ctx, c.buildRequest(method, params, object), target)
}

func (c *rpcClient) call(ctx context.Context, req rpcRequest, target any) error {
	if err := c.ensureLogin(ctx); err != nil {
		return err
	}

	if err := c.callOnce(ctx, vtoRPCPath, req, target); err != nil {
		if rpcErr, ok := err.(*rpcError); ok && isAuthRPCError(rpcErr.Code) {
			c.resetSession()
			if loginErr := c.ensureLogin(ctx); loginErr != nil {
				return loginErr
			}
			req.Session = c.currentSession()
			return c.callOnce(ctx, vtoRPCPath, req, target)
		}
		return err
	}

	return nil
}

func (c *rpcClient) ensureLogin(ctx context.Context) error {
	c.mu.Lock()
	hasSession := c.session != nil
	c.mu.Unlock()
	if hasSession {
		return nil
	}

	first := rpcRequest{
		Method:  "global.login",
		Params:  map[string]any{"userName": c.currentUsername(), "password": "", "clientType": "Web3.0"},
		ID:      c.nextRequestID(),
		Session: 0,
	}

	var firstResp rpcResponse
	if err := c.post(ctx, vtoRPCLoginPath, first, &firstResp); err != nil {
		return err
	}
	if firstResp.Error == nil || firstResp.Session == nil {
		return fmt.Errorf("vto rpc login challenge did not return session and challenge params")
	}

	var challenge struct {
		Realm      string `json:"realm"`
		Random     string `json:"random"`
		Encryption string `json:"encryption"`
	}
	if err := json.Unmarshal(firstResp.Params, &challenge); err != nil {
		return fmt.Errorf("decode vto rpc login challenge: %w", err)
	}

	username := c.currentUsername()
	passwordValue := c.currentPassword()
	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", username, challenge.Realm, passwordValue))
	password := md5Hex(fmt.Sprintf("%s:%s:%s", username, challenge.Random, ha1))

	second := rpcRequest{
		Method: "global.login",
		Params: map[string]any{
			"userName":      username,
			"password":      password,
			"clientType":    "Web3.0",
			"realm":         challenge.Realm,
			"random":        challenge.Random,
			"passwordType":  "Default",
			"authorityType": challenge.Encryption,
		},
		ID:      c.nextRequestID(),
		Session: firstResp.Session,
	}

	var secondResp rpcResponse
	if err := c.callOnce(ctx, vtoRPCLoginPath, second, &secondResp); err != nil {
		return err
	}

	c.mu.Lock()
	c.session = secondResp.Session
	c.mu.Unlock()
	return nil
}

func (c *rpcClient) callOnce(ctx context.Context, path string, req rpcRequest, target any) error {
	var rpcResp rpcResponse
	if err := c.post(ctx, path, req, &rpcResp); err != nil {
		return err
	}

	if !rpcResp.successful() {
		if rpcResp.Error != nil {
			return rpcResp.Error
		}
		return fmt.Errorf("rpc method %q returned result=false", req.Method)
	}

	if target == nil {
		return nil
	}

	if out, ok := target.(*rpcResponse); ok {
		*out = rpcResp
		return nil
	}

	if len(rpcResp.Params) == 0 {
		if rpcResp.hasStructuredResultValue() {
			if err := json.Unmarshal(rpcResp.Result, target); err != nil {
				return fmt.Errorf("decode rpc result for %q: %w", req.Method, err)
			}
		}
		return nil
	}

	if err := json.Unmarshal(rpcResp.Params, target); err != nil {
		return fmt.Errorf("decode rpc params for %q: %w", req.Method, err)
	}

	return nil
}

func (c *rpcClient) post(ctx context.Context, path string, req rpcRequest, target any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	baseURL, client := c.currentHTTPState()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}

	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("decode rpc response: %w", err)
	}
	return nil
}

func (c *rpcClient) buildRequest(method string, params any, object any) rpcRequest {
	return rpcRequest{
		Method:  method,
		Params:  params,
		ID:      c.nextRequestID(),
		Session: c.currentSession(),
		Object:  object,
	}
}

func (c *rpcClient) currentSession() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session
}

func (c *rpcClient) nextRequestID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	return id
}

func (c *rpcClient) resetSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = nil
}

func (c *rpcClient) currentHTTPState() (string, *http.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL, c.http
}

func (c *rpcClient) currentUsername() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.username
}

func (c *rpcClient) currentPassword() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.password
}

func newRPCHTTPClient(cfg config.DeviceConfig, jar http.CookieJar) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = dahuatransport.LegacyTLSConfig(cfg.InsecureSkipTLS)
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout,
		Jar:       jar,
	}
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func isAuthRPCError(code int) bool {
	return code == 287637504 || code == 287637505
}

func (r rpcResponse) successful() bool {
	if len(r.Result) == 0 {
		return false
	}

	var flag bool
	if err := json.Unmarshal(r.Result, &flag); err == nil {
		return flag
	}

	raw := strings.TrimSpace(string(r.Result))
	return raw != "" && raw != "false" && raw != "null"
}

func (r rpcResponse) hasStructuredResultValue() bool {
	if len(r.Result) == 0 {
		return false
	}

	raw := strings.TrimSpace(string(r.Result))
	return raw != "" && raw != "true" && raw != "false" && raw != "null"
}
