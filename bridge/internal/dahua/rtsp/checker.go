package rtsp

import (
	"bufio"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	dahuatransport "RCooLeR/DahuaBridge/internal/dahua/transport"
)

const (
	successCacheTTL = 2 * time.Minute
	failureCacheTTL = 30 * time.Second
)

type Checker struct {
	mu                sync.Mutex
	username          string
	password          string
	insecureSkipTLS   bool
	requestTimeout    time.Duration
	cachedByStreamURL map[string]cachedResult
}

type cachedResult struct {
	available bool
	expiresAt time.Time
}

func NewChecker(cfg config.DeviceConfig) *Checker {
	return &Checker{
		username:          cfg.Username,
		password:          cfg.Password,
		insecureSkipTLS:   cfg.InsecureSkipTLS,
		requestTimeout:    cfg.RequestTimeout,
		cachedByStreamURL: make(map[string]cachedResult),
	}
}

func (c *Checker) UpdateConfig(cfg config.DeviceConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.username = cfg.Username
	c.password = cfg.Password
	c.insecureSkipTLS = cfg.InsecureSkipTLS
	c.requestTimeout = cfg.RequestTimeout
	c.cachedByStreamURL = make(map[string]cachedResult)
}

func (c *Checker) StreamAvailable(ctx context.Context, streamURLs ...string) (bool, error) {
	var lastErr error
	for _, streamURL := range streamURLs {
		streamURL = strings.TrimSpace(streamURL)
		if streamURL == "" {
			continue
		}
		available, err := c.checkStream(ctx, streamURL)
		if available {
			return true, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	return false, lastErr
}

func (c *Checker) checkStream(ctx context.Context, streamURL string) (bool, error) {
	if available, ok := c.cached(streamURL); ok {
		return available, nil
	}

	available, err := c.describeWithDigest(ctx, streamURL)
	c.store(streamURL, available)
	return available, err
}

func (c *Checker) cached(streamURL string) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.cachedByStreamURL[streamURL]
	if !ok || time.Now().After(cached.expiresAt) {
		return false, false
	}
	return cached.available, true
}

func (c *Checker) store(streamURL string, available bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := failureCacheTTL
	if available {
		ttl = successCacheTTL
	}
	c.cachedByStreamURL[streamURL] = cachedResult{
		available: available,
		expiresAt: time.Now().Add(ttl),
	}
}

func (c *Checker) describeWithDigest(ctx context.Context, streamURL string) (bool, error) {
	statusCode, headers, err := c.describe(ctx, streamURL, "")
	if err != nil {
		return false, err
	}
	if statusCode == 200 {
		return true, nil
	}
	if statusCode != 401 {
		return false, fmt.Errorf("unexpected rtsp status %d", statusCode)
	}

	challenge := parseDigestChallenge(headers.Get("WWW-Authenticate"))
	if len(challenge) == 0 {
		return false, fmt.Errorf("rtsp digest challenge not found")
	}

	authHeader := authorizationHeader(c.username, c.password, "DESCRIBE", streamURL, challenge, 1)
	statusCode, _, err = c.describe(ctx, streamURL, authHeader)
	if err != nil {
		return false, err
	}
	if statusCode == 200 {
		return true, nil
	}
	if statusCode == 401 {
		// Some Dahua RTSP endpoints keep rotating the digest nonce for lightweight
		// DESCRIBE probes even though real media clients can still open the stream.
		// Treat that as "service reachable" so higher layers don't hide all streams.
		return true, nil
	}
	return false, fmt.Errorf("unexpected rtsp auth status %d", statusCode)
}

func (c *Checker) describe(ctx context.Context, streamURL string, authHeader string) (int, textproto.MIMEHeader, error) {
	parsed, err := url.Parse(streamURL)
	if err != nil {
		return 0, nil, fmt.Errorf("parse rtsp url: %w", err)
	}
	host := parsed.Host
	if host == "" {
		return 0, nil, fmt.Errorf("rtsp url missing host")
	}
	if parsed.Port() == "" {
		host = net.JoinHostPort(parsed.Hostname(), "554")
	}

	timeout := c.requestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var conn net.Conn
	switch strings.ToLower(parsed.Scheme) {
	case "rtsps":
		dialer := &net.Dialer{}
		tlsConfig := dahuatransport.LegacyTLSConfig(c.insecureSkipTLS)
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		}
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = parsed.Hostname()
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", host, tlsConfig)
	default:
		var dialer net.Dialer
		conn, err = dialer.DialContext(dialCtx, "tcp", host)
	}
	if err != nil {
		return 0, nil, err
	}
	defer conn.Close()

	if deadline, ok := dialCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	var request strings.Builder
	request.WriteString(fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\n", streamURL))
	request.WriteString("CSeq: 1\r\n")
	request.WriteString("Accept: application/sdp\r\n")
	request.WriteString("User-Agent: DahuaBridge/1.0\r\n")
	if authHeader != "" {
		request.WriteString("Authorization: ")
		request.WriteString(authHeader)
		request.WriteString("\r\n")
	}
	request.WriteString("\r\n")

	if _, err := conn.Write([]byte(request.String())); err != nil {
		return 0, nil, err
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return 0, nil, err
	}
	statusCode, err := parseStatusCode(statusLine)
	if err != nil {
		return 0, nil, err
	}

	tp := textproto.NewReader(reader)
	headers, err := tp.ReadMIMEHeader()
	if err != nil {
		return 0, nil, err
	}
	return statusCode, headers, nil
}

func parseStatusCode(statusLine string) (int, error) {
	fields := strings.Fields(strings.TrimSpace(statusLine))
	if len(fields) < 2 {
		return 0, fmt.Errorf("invalid rtsp status line %q", strings.TrimSpace(statusLine))
	}
	statusCode, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("invalid rtsp status line %q: %w", strings.TrimSpace(statusLine), err)
	}
	return statusCode, nil
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

func authorizationHeader(username string, password string, method string, requestURI string, challenge map[string]string, nonceSeq int) string {
	realm := challenge["realm"]
	nonce := challenge["nonce"]
	opaque := challenge["opaque"]
	algorithm := challenge["algorithm"]
	if algorithm == "" {
		algorithm = "MD5"
	}

	qop := pickQOP(challenge["qop"])
	cnonce := randomHex(16)
	nc := fmt.Sprintf("%08x", nonceSeq)

	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", username, realm, password))
	ha2 := md5Hex(fmt.Sprintf("%s:%s", method, requestURI))

	var response string
	if qop != "" {
		response = md5Hex(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, nonce, nc, cnonce, qop, ha2))
	} else {
		response = md5Hex(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	}

	parts := []string{
		fmt.Sprintf(`Digest username="%s"`, username),
		fmt.Sprintf(`realm="%s"`, realm),
		fmt.Sprintf(`nonce="%s"`, nonce),
		fmt.Sprintf(`uri="%s"`, requestURI),
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
