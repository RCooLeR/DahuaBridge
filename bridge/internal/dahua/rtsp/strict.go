package rtsp

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	dahuatransport "RCooLeR/DahuaBridge/internal/dahua/transport"
)

// DescribeAvailable checks whether a stream accepts DESCRIBE, including one
// authentication challenge when needed. Unlike Checker's discovery probe, a
// rejected credential is unavailable. It does not SETUP or PLAY the stream.
func DescribeAvailable(ctx context.Context, streamURL string, timeout time.Duration, insecureSkipTLS bool) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(streamURL))
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "rtsp" && parsed.Scheme != "rtsps") {
		return false, fmt.Errorf("invalid rtsp stream URL")
	}
	username, password := "", ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	parsed.User = nil
	parsed.Fragment = ""
	requestURI := parsed.String()
	address := parsed.Host
	if parsed.Port() == "" {
		address = net.JoinHostPort(parsed.Hostname(), "554")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var conn net.Conn
	if parsed.Scheme == "rtsps" {
		tlsConfig := dahuatransport.LegacyTLSConfig(insecureSkipTLS)
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		}
		tlsConfig.ServerName = parsed.Hostname()
		dialer := tls.Dialer{Config: tlsConfig}
		conn, err = dialer.DialContext(probeCtx, "tcp", address)
	} else {
		var dialer net.Dialer
		conn, err = dialer.DialContext(probeCtx, "tcp", address)
	}
	if err != nil {
		return false, strictProbeError(probeCtx, "connect", err)
	}
	defer conn.Close()
	stopCancellation := context.AfterFunc(probeCtx, func() { _ = conn.Close() })
	defer stopCancellation()
	if deadline, ok := probeCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	// Bound the response bytes as well as time, including an optional challenge
	// body that must be consumed before the authenticated request.
	reader := bufio.NewReader(io.LimitReader(conn, 1024*1024))
	status, headers, err := strictDescribe(conn, reader, requestURI, "", 1)
	if err != nil {
		return false, strictProbeError(probeCtx, "describe", err)
	}
	if status == 401 {
		if username == "" {
			return false, fmt.Errorf("rtsp authentication required")
		}
		authorization, authErr := strictAuthorization(headers.Values("WWW-Authenticate"), username, password, requestURI)
		if authErr != nil {
			return false, authErr
		}
		status, _, err = strictDescribe(conn, reader, requestURI, authorization, 2)
		if err != nil {
			return false, strictProbeError(probeCtx, "authenticated describe", err)
		}
	}
	if status != 200 {
		return false, fmt.Errorf("rtsp describe returned status %d", status)
	}
	return true, nil
}

func strictProbeError(ctx context.Context, phase string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if networkErr, ok := err.(net.Error); ok && networkErr.Timeout() {
		return fmt.Errorf("rtsp %s: %w", phase, context.DeadlineExceeded)
	}
	// Response parsers and network errors can contain server-supplied text. Do
	// not return that text or the caller's credential-bearing URL to logs.
	return fmt.Errorf("rtsp %s failed", phase)
}

func strictDescribe(conn net.Conn, reader *bufio.Reader, requestURI, authorization string, sequence int) (int, textproto.MIMEHeader, error) {
	var request strings.Builder
	fmt.Fprintf(&request, "DESCRIBE %s RTSP/1.0\r\nCSeq: %d\r\nAccept: application/sdp\r\nUser-Agent: DahuaBridge/1.0\r\n", requestURI, sequence)
	if authorization != "" {
		fmt.Fprintf(&request, "Authorization: %s\r\n", authorization)
	}
	request.WriteString("\r\n")
	if _, err := io.WriteString(conn, request.String()); err != nil {
		return 0, nil, err
	}
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return 0, nil, err
	}
	if !strings.HasPrefix(statusLine, "RTSP/") {
		return 0, nil, fmt.Errorf("invalid rtsp response")
	}
	status, err := parseStatusCode(statusLine)
	if err != nil {
		return 0, nil, err
	}
	headers, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		return 0, nil, err
	}
	if status == 401 {
		length := int64(0)
		if raw := headers.Get("Content-Length"); raw != "" {
			length, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || length < 0 || length > 1024*1024 {
				return 0, nil, fmt.Errorf("invalid rtsp content length")
			}
		}
		if _, err := io.CopyN(io.Discard, reader, length); err != nil {
			return 0, nil, err
		}
	}
	return status, headers, nil
}

func strictAuthorization(challenges []string, username, password, requestURI string) (string, error) {
	var basic bool
	for _, header := range challenges {
		scheme, parameters, _ := strings.Cut(strings.TrimSpace(header), " ")
		if strings.EqualFold(scheme, "Basic") {
			basic = true
			continue
		}
		if !strings.EqualFold(scheme, "Digest") {
			continue
		}
		challenge, err := strictDigestParameters(parameters)
		if err != nil {
			return "", err
		}
		if challenge["realm"] == "" || challenge["nonce"] == "" {
			return "", fmt.Errorf("incomplete rtsp digest challenge")
		}
		algorithm := strings.ToUpper(challenge["algorithm"])
		if algorithm == "" {
			algorithm = "MD5"
		}
		hash := md5Hex
		switch algorithm {
		case "MD5", "MD5-SESS":
		case "SHA-256", "SHA-256-SESS":
			hash = func(value string) string {
				sum := sha256.Sum256([]byte(value))
				return hex.EncodeToString(sum[:])
			}
		default:
			return "", fmt.Errorf("unsupported rtsp digest algorithm")
		}
		qop := ""
		if challenge["qop"] != "" {
			for _, value := range strings.Split(challenge["qop"], ",") {
				if strings.TrimSpace(value) == "auth" {
					qop = "auth"
				}
			}
			if qop == "" {
				return "", fmt.Errorf("unsupported rtsp digest protection")
			}
		}
		realm, nonce := challenge["realm"], challenge["nonce"]
		cnonce, nc := randomNonceHex(), "00000001"
		ha1 := hash(username + ":" + realm + ":" + password)
		if strings.HasSuffix(algorithm, "-SESS") {
			ha1 = hash(ha1 + ":" + nonce + ":" + cnonce)
		}
		ha2 := hash("DESCRIBE:" + requestURI)
		response := hash(ha1 + ":" + nonce + ":" + ha2)
		if qop != "" {
			response = hash(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
		}
		parts := []string{
			"Digest username=" + strconv.Quote(username),
			"realm=" + strconv.Quote(realm),
			"nonce=" + strconv.Quote(nonce),
			"uri=" + strconv.Quote(requestURI),
			"response=" + strconv.Quote(response),
			"algorithm=" + algorithm,
		}
		if opaque := challenge["opaque"]; opaque != "" {
			parts = append(parts, "opaque="+strconv.Quote(opaque))
		}
		if qop != "" {
			parts = append(parts, "qop="+qop, "nc="+nc)
		}
		if qop != "" || strings.HasSuffix(algorithm, "-SESS") {
			parts = append(parts, "cnonce="+strconv.Quote(cnonce))
		}
		return strings.Join(parts, ", "), nil
	}
	if basic {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password)), nil
	}
	return "", fmt.Errorf("supported rtsp authentication challenge not found")
}

func strictDigestParameters(header string) (map[string]string, error) {
	parameters := make(map[string]string)
	start, quoted, escaped := 0, false, false
	for index := 0; index <= len(header); index++ {
		if index == len(header) || (header[index] == ',' && !quoted) {
			key, value, ok := strings.Cut(strings.TrimSpace(header[start:index]), "=")
			if !ok || quoted {
				return nil, fmt.Errorf("invalid rtsp digest challenge")
			}
			value = strings.TrimSpace(value)
			if strings.HasPrefix(value, `"`) {
				var err error
				value, err = strconv.Unquote(value)
				if err != nil {
					return nil, fmt.Errorf("invalid rtsp digest challenge")
				}
			}
			parameters[strings.ToLower(strings.TrimSpace(key))] = value
			start = index + 1
			continue
		}
		if escaped {
			escaped = false
		} else if header[index] == '\\' && quoted {
			escaped = true
		} else if header[index] == '"' {
			quoted = !quoted
		}
	}
	return parameters, nil
}
