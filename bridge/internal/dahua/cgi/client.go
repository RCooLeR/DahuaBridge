package cgi

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	dahuatransport "RCooLeR/DahuaBridge/internal/dahua/transport"
	"RCooLeR/DahuaBridge/internal/metrics"
	"github.com/rs/zerolog"
)

type Client struct {
	deviceID string
	baseURL  string
	username string
	password string
	http     *http.Client
	stream   *http.Client
	metrics  *metrics.Registry
	logger   zerolog.Logger

	mu        sync.Mutex
	challenge map[string]string
	nonceSeq  int
}

func New(cfg config.DeviceConfig, metricsRegistry *metrics.Registry, loggers ...zerolog.Logger) *Client {
	logger := zerolog.Nop()
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return &Client{
		deviceID: cfg.ID,
		baseURL:  cfg.BaseURL,
		username: cfg.Username,
		password: cfg.Password,
		http:     newHTTPClient(cfg),
		stream:   newStreamClient(cfg),
		metrics:  metricsRegistry,
		logger:   logger.With().Str("component", "dahua_cgi").Str("device_id", cfg.ID).Logger(),
	}
}

func (c *Client) UpdateConfig(cfg config.DeviceConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = cfg.BaseURL
	c.username = cfg.Username
	c.password = cfg.Password
	c.http = newHTTPClient(cfg)
	c.stream = newStreamClient(cfg)
	c.challenge = nil
	c.nonceSeq = 0
}

func (c *Client) GetText(ctx context.Context, path string, query url.Values) (string, error) {
	baseURL, client := c.currentHTTPState()
	endpoint := path
	reqURL := baseURL + path
	if len(query) > 0 {
		reqURL += "?" + encodeQuery(query)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.do(req, client)
	if err != nil {
		c.metrics.ObserveDahuaRequest(c.deviceID, endpoint, http.MethodGet, "transport_error")
		return "", err
	}
	defer resp.Body.Close()

	c.metrics.ObserveDahuaRequest(c.deviceID, endpoint, http.MethodGet, fmt.Sprintf("%d", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return string(body), nil
}

func (c *Client) GetKeyValues(ctx context.Context, path string, query url.Values) (map[string]string, error) {
	body, err := c.GetText(ctx, path, query)
	if err != nil {
		return nil, err
	}

	return dahua.ParseKeyValueBody(body), nil
}

func (c *Client) GetBinary(ctx context.Context, path string, query url.Values) ([]byte, string, error) {
	baseURL, client := c.currentHTTPState()
	endpoint := path
	reqURL := baseURL + path
	if len(query) > 0 {
		reqURL += "?" + encodeQuery(query)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := c.do(req, client)
	if err != nil {
		c.metrics.ObserveDahuaRequest(c.deviceID, endpoint, http.MethodGet, "transport_error")
		return nil, "", err
	}
	defer resp.Body.Close()

	c.metrics.ObserveDahuaRequest(c.deviceID, endpoint, http.MethodGet, fmt.Sprintf("%d", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("unexpected status %s", resp.Status)
	}

	return body, resp.Header.Get("Content-Type"), nil
}

func (c *Client) OpenStream(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	baseURL, client := c.currentStreamState()
	reqURL := baseURL + path
	if len(query) > 0 {
		reqURL += "?" + encodeQuery(query)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req, client)
	if err != nil {
		c.metrics.ObserveDahuaRequest(c.deviceID, path, http.MethodGet, "transport_error")
		return nil, err
	}

	c.metrics.ObserveDahuaRequest(c.deviceID, path, http.MethodGet, fmt.Sprintf("%d", resp.StatusCode))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return resp, nil
}

func (c *Client) do(req *http.Request, client *http.Client) (*http.Response, error) {
	started := time.Now()
	resp, err := c.doWithDigest(req, client)
	c.logHTTPRequest(req, resp, err, time.Since(started))
	return resp, err
}

func (c *Client) doWithDigest(req *http.Request, client *http.Client) (*http.Response, error) {
	req1 := req.Clone(req.Context())

	if auth := c.authorizationHeader(req1); auth != "" {
		req1.Header.Set("Authorization", auth)
		resp, err := client.Do(req1)
		if err == nil && resp.StatusCode != http.StatusUnauthorized {
			if shouldRetryDigestChallenge(resp) {
				resp.Body.Close()
			} else {
				return resp, nil
			}
		}
		if err != nil {
			return nil, err
		}
		if resp != nil {
			resp.Body.Close()
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge := parseDigestChallenge(resp.Header.Get("WWW-Authenticate"))
	resp.Body.Close()
	if len(challenge) == 0 {
		return nil, fmt.Errorf("digest challenge not found")
	}

	c.setChallenge(challenge)

	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", c.authorizationHeader(req2))

	return client.Do(req2)
}

func (c *Client) logHTTPRequest(req *http.Request, resp *http.Response, err error, duration time.Duration) {
	if req == nil {
		return
	}
	event := c.logger.Debug().
		Str("method", req.Method).
		Str("path", req.URL.Path).
		Dur("duration", duration)
	if req.URL.RawQuery != "" {
		event.Str("query", redactQuery(req.URL.Query()))
	}
	if resp != nil {
		event.Int("status", resp.StatusCode)
		if contentLength := strings.TrimSpace(resp.Header.Get("Content-Length")); contentLength != "" {
			event.Str("content_length", contentLength)
		}
		if contentType := strings.TrimSpace(resp.Header.Get("Content-Type")); contentType != "" {
			event.Str("content_type", contentType)
		}
	}
	if err != nil {
		event.Err(err)
	}
	event.Msg("dahua cgi request")
}

func redactQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	redacted := make(url.Values, len(query))
	for key, values := range query {
		nextValues := append([]string(nil), values...)
		if shouldRedactQueryKey(key) {
			for index := range nextValues {
				nextValues[index] = "[redacted]"
			}
		}
		redacted[key] = nextValues
	}
	return encodeQuery(redacted)
}

func shouldRedactQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "pwd") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret")
}

func shouldRetryDigestChallenge(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusForbidden || resp.Body == nil {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return strings.Contains(strings.TrimSpace(string(body)), "Authority:check failure.")
}

func encodeQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(query))
	appendKey := func(key string, values []string) {
		escapedKey := url.QueryEscape(key)
		for _, value := range values {
			pairs = append(pairs, escapedKey+"="+url.QueryEscape(value))
		}
	}

	if actionValues, ok := query["action"]; ok {
		appendKey("action", actionValues)
	}

	keys := make([]string, 0, len(query))
	for key := range query {
		if key == "action" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendKey(key, query[key])
	}

	return strings.Join(pairs, "&")
}

func (c *Client) setChallenge(challenge map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.challenge = challenge
	c.nonceSeq = 0
}

func (c *Client) authorizationHeader(req *http.Request) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.challenge) == 0 {
		return ""
	}

	c.nonceSeq++

	realm := c.challenge["realm"]
	nonce := c.challenge["nonce"]
	opaque := c.challenge["opaque"]
	algorithm := c.challenge["algorithm"]
	if algorithm == "" {
		algorithm = "MD5"
	}

	qop := pickQOP(c.challenge["qop"])
	uri := req.URL.RequestURI()
	cnonce := randomHex(16)
	nc := fmt.Sprintf("%08x", c.nonceSeq)

	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", c.username, realm, c.password))
	ha2 := md5Hex(fmt.Sprintf("%s:%s", req.Method, uri))

	var response string
	if qop != "" {
		response = md5Hex(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, nonce, nc, cnonce, qop, ha2))
	} else {
		response = md5Hex(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	}

	parts := []string{
		fmt.Sprintf(`Digest username="%s"`, c.username),
		fmt.Sprintf(`realm="%s"`, realm),
		fmt.Sprintf(`nonce="%s"`, nonce),
		fmt.Sprintf(`uri="%s"`, uri),
		fmt.Sprintf(`response="%s"`, response),
		fmt.Sprintf(`algorithm=%s`, algorithm),
	}

	if opaque != "" {
		parts = append(parts, fmt.Sprintf(`opaque="%s"`, opaque))
	}

	if qop != "" {
		parts = append(parts,
			fmt.Sprintf("qop=%s", qop),
			fmt.Sprintf("nc=%s", nc),
			fmt.Sprintf(`cnonce="%s"`, cnonce),
		)
	}

	return strings.Join(parts, ", ")
}

func (c *Client) currentHTTPState() (string, *http.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL, c.http
}

func (c *Client) currentStreamState() (string, *http.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL, c.stream
}

func newHTTPClient(cfg config.DeviceConfig) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = dahuatransport.LegacyTLSConfig(cfg.InsecureSkipTLS)
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout,
	}
}

func newStreamClient(cfg config.DeviceConfig) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = dahuatransport.LegacyTLSConfig(cfg.InsecureSkipTLS)
	return &http.Client{Transport: transport}
}

func parseDigestChallenge(header string) map[string]string {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "digest ") {
		return nil
	}

	header = strings.TrimSpace(header[len("Digest "):])
	result := make(map[string]string)
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}

	return result
}

func pickQOP(value string) string {
	for _, part := range strings.Split(value, ",") {
		qop := strings.TrimSpace(part)
		if qop == "auth" {
			return qop
		}
	}

	return strings.TrimSpace(value)
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "0000000000000000"
	}

	return hex.EncodeToString(buf)
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
