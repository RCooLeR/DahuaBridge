package rpc

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
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	dahuatransport "RCooLeR/DahuaBridge/internal/dahua/transport"
	"github.com/rs/zerolog"
)

const (
	loginPath        = "/RPC2_Login"
	rpcPath          = "/RPC2"
	rpc3LoadfilePath = "/RPC3_Loadfile"
	webUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
)

type Client struct {
	baseURL    string
	username   string
	password   string
	clientType string
	loginType  string
	http       *http.Client
	logger     zerolog.Logger

	mu      sync.Mutex
	nextID  int64
	session any
}

type Request struct {
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int64  `json:"id"`
	Session any    `json:"session,omitempty"`
	Object  any    `json:"object,omitempty"`
}

type Response struct {
	Result  json.RawMessage `json:"result"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      int64           `json:"id"`
	Session any             `json:"session,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("rpc error code %d", e.Code)
	}
	return fmt.Sprintf("rpc error code %d: %s", e.Code, e.Message)
}

func New(cfg config.DeviceConfig, loggers ...zerolog.Logger) *Client {
	logger := zerolog.Nop()
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return &Client{
		baseURL:    cfg.BaseURL,
		username:   cfg.RPCUsernameValue(),
		password:   cfg.RPCPasswordValue(),
		clientType: "Web3.0",
		loginType:  "Direct",
		http:       newHTTPClient(cfg),
		logger:     logger.With().Str("component", "dahua_rpc").Str("device_id", cfg.ID).Logger(),
		nextID:     1,
	}
}

func (c *Client) UpdateConfig(cfg config.DeviceConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = cfg.BaseURL
	c.username = cfg.RPCUsernameValue()
	c.password = cfg.RPCPasswordValue()
	c.http = newHTTPClient(cfg)
	c.session = nil
}

func (c *Client) Call(ctx context.Context, method string, params any, target any) error {
	if err := c.ensureLogin(ctx); err != nil {
		return err
	}

	req := c.buildRequest(method, params, nil)
	if err := c.callOnce(ctx, rpcPath, req, target); err != nil {
		if rpcErr, ok := err.(*Error); ok && isAuthError(rpcErr.Code) {
			c.resetSession()
			if loginErr := c.ensureLogin(ctx); loginErr != nil {
				return loginErr
			}
			return c.callOnce(ctx, rpcPath, c.buildRequest(method, params, nil), target)
		}
		return err
	}

	return nil
}

func (c *Client) CallObject(ctx context.Context, method string, params any, object any, target any) error {
	if err := c.ensureLogin(ctx); err != nil {
		return err
	}

	req := c.buildRequest(method, params, object)
	if err := c.callOnce(ctx, rpcPath, req, target); err != nil {
		if rpcErr, ok := err.(*Error); ok && isAuthError(rpcErr.Code) {
			c.resetSession()
			if loginErr := c.ensureLogin(ctx); loginErr != nil {
				return loginErr
			}
			return c.callOnce(ctx, rpcPath, c.buildRequest(method, params, object), target)
		}
		return err
	}

	return nil
}

func (c *Client) CallLoadfile(ctx context.Context, method string, params any) (*http.Response, error) {
	if err := c.ensureLogin(ctx); err != nil {
		return nil, err
	}

	req := c.buildRequest(method, params, nil)
	resp, err := c.postRaw(ctx, rpc3LoadfilePath, req)
	if err != nil {
		if rpcErr, ok := err.(*Error); ok && isAuthError(rpcErr.Code) {
			c.resetSession()
			if loginErr := c.ensureLogin(ctx); loginErr != nil {
				return nil, loginErr
			}
			return c.postRaw(ctx, rpc3LoadfilePath, c.buildRequest(method, params, nil))
		}
		return nil, err
	}
	return resp, nil
}

func (c *Client) ensureLogin(ctx context.Context) error {
	c.mu.Lock()
	hasSession := c.session != nil
	c.mu.Unlock()
	if hasSession {
		return nil
	}

	first := Request{
		Method: "global.login",
		Params: map[string]any{
			"userName":   c.currentUsername(),
			"password":   "",
			"clientType": c.clientType,
			"loginType":  c.loginType,
		},
		ID: c.nextRequestID(),
	}

	var firstResp Response
	if err := c.post(ctx, loginPath, first, &firstResp); err != nil {
		return err
	}
	c.setSession(firstResp.Session)

	var challenge struct {
		Realm      string `json:"realm"`
		Random     string `json:"random"`
		Encryption string `json:"encryption"`
	}
	if len(firstResp.Params) > 0 {
		if err := json.Unmarshal(firstResp.Params, &challenge); err != nil {
			return fmt.Errorf("decode rpc login challenge: %w", err)
		}
	}
	if challenge.Realm == "" || challenge.Random == "" {
		if firstResp.successful() {
			return nil
		}
		if firstResp.Error != nil {
			return firstResp.Error
		}
		return fmt.Errorf("rpc login challenge did not return realm/random")
	}

	second := Request{
		Method: "global.login",
		Params: map[string]any{
			"userName":      c.currentUsername(),
			"password":      defaultAuthorityAuth(c.currentUsername(), c.currentPassword(), challenge.Realm, challenge.Random),
			"clientType":    c.clientType,
			"loginType":     c.loginType,
			"authorityType": firstNonEmpty(challenge.Encryption, "Default"),
			"passwordType":  "Default",
		},
		ID:      c.nextRequestID(),
		Session: firstResp.Session,
	}

	var secondResp Response
	if err := c.callOnce(ctx, loginPath, second, &secondResp); err != nil {
		return err
	}

	c.setSession(secondResp.Session)
	return nil
}

func (c *Client) buildRequest(method string, params any, object any) Request {
	return Request{
		Method:  method,
		Params:  params,
		ID:      c.nextRequestID(),
		Session: c.currentSession(),
		Object:  object,
	}
}

func (c *Client) callOnce(ctx context.Context, path string, req Request, target any) error {
	var rpcResp Response
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

	if out, ok := target.(*Response); ok {
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

func (c *Client) post(ctx context.Context, path string, req Request, target *Response) error {
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
	c.applyBrowserHeaders(httpReq, req.Session)

	started := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		c.logRPCRequest(path, req.Method, req.ID, 0, 0, time.Since(started), err)
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logRPCRequest(path, req.Method, req.ID, resp.StatusCode, 0, time.Since(started), err)
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(payload)))
		c.logRPCRequest(path, req.Method, req.ID, resp.StatusCode, len(payload), time.Since(started), err)
		return err
	}

	if err := json.Unmarshal(payload, target); err != nil {
		err = fmt.Errorf("decode rpc response: %w", err)
		c.logRPCRequest(path, req.Method, req.ID, resp.StatusCode, len(payload), time.Since(started), err)
		return err
	}
	c.logRPCRequest(path, req.Method, req.ID, resp.StatusCode, len(payload), time.Since(started), nil)
	return nil
}

func (c *Client) postRaw(ctx context.Context, path string, req Request) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	baseURL, client := c.currentHTTPState()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyBrowserHeaders(httpReq, req.Session)

	started := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		c.logRPCRequest(path, req.Method, req.ID, 0, 0, time.Since(started), err)
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		payload, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(payload)))
		c.logRPCRequest(path, req.Method, req.ID, resp.StatusCode, len(payload), time.Since(started), err)
		return nil, err
	}

	c.logRPCRequest(path, req.Method, req.ID, resp.StatusCode, 0, time.Since(started), nil)
	return resp, nil
}

func (c *Client) logRPCRequest(path string, method string, requestID int64, status int, payloadBytes int, duration time.Duration, err error) {
	event := c.logger.Debug().
		Str("path", path).
		Str("rpc_method", method).
		Int64("request_id", requestID).
		Dur("duration", duration)
	if status > 0 {
		event.Int("status", status)
	}
	if payloadBytes > 0 {
		event.Int("payload_bytes", payloadBytes)
	}
	if err != nil {
		event.Err(err)
	}
	event.Msg("dahua rpc request")
}

func (c *Client) currentSession() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session
}

func (c *Client) setSession(session any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = session
}

func (c *Client) resetSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = nil
}

func (c *Client) currentHTTPState() (string, *http.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL, c.http
}

func (c *Client) currentUsername() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.username
}

func (c *Client) currentPassword() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.password
}

func newHTTPClient(cfg config.DeviceConfig) *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		jar = nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = dahuatransport.LegacyTLSConfig(cfg.InsecureSkipTLS)
	return &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   cfg.RequestTimeout,
	}
}

func (c *Client) nextRequestID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	return id
}

func defaultAuthorityAuth(username string, password string, realm string, random string) string {
	ha1 := md5Upper(username + ":" + realm + ":" + password)
	return md5Upper(username + ":" + random + ":" + ha1)
}

func md5Upper(value string) string {
	sum := md5.Sum([]byte(value))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sessionHeaderValue(session any) string {
	if session == nil {
		return ""
	}
	switch typed := session.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(session)
	}
}

func (c *Client) applyBrowserHeaders(req *http.Request, session any) {
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "uk,en-US;q=0.9,en;q=0.8,ru;q=0.7,fr;q=0.6")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("User-Agent", webUserAgent)

	baseURL, _ := c.currentHTTPState()
	origin := strings.TrimRight(baseURL, "/")
	if origin != "" {
		req.Header.Set("Origin", origin)
		req.Header.Set("Referer", origin+"/")
	}

	sessionValue := strings.TrimSpace(sessionHeaderValue(session))
	if sessionValue == "" {
		return
	}
	req.Header.Set("X-Api-Session", sessionValue)
	req.AddCookie(&http.Cookie{
		Name:  "WebClientHttpSessionID",
		Value: sessionValue,
		Path:  "/",
	})
}

func isAuthError(code int) bool {
	return code == 287637504 || code == 287637505
}

func (r Response) successful() bool {
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

func (r Response) hasStructuredResultValue() bool {
	if len(r.Result) == 0 {
		return false
	}

	raw := strings.TrimSpace(string(r.Result))
	return raw != "" && raw != "true" && raw != "false" && raw != "null"
}
