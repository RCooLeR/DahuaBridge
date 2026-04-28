package rtsp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
)

func TestStreamAvailableWithoutAuth(t *testing.T) {
	listener, url := startRTSPTestServer(t, func(conn net.Conn, firstLine string, headers map[string]string) {
		_, _ = fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: 1\r\nContent-Length: 0\r\n\r\n")
	})

	defer listener.Close()

	checker := NewChecker(config.DeviceConfig{RequestTimeout: 2 * time.Second})
	ok, err := checker.StreamAvailable(context.Background(), url)
	if err != nil {
		t.Fatalf("StreamAvailable returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected stream to be available")
	}
}

func TestStreamAvailableWithDigestAuthAndCache(t *testing.T) {
	var requestCount atomic.Int32
	listener, url := startRTSPTestServer(t, func(conn net.Conn, firstLine string, headers map[string]string) {
		requestCount.Add(1)
		if strings.TrimSpace(headers["authorization"]) == "" {
			_, _ = fmt.Fprintf(conn, "RTSP/1.0 401 Unauthorized\r\nCSeq: 1\r\nWWW-Authenticate: Digest realm=\"Login to rtsp\", nonce=\"abc123\", opaque=\"opaque\", qop=\"auth\"\r\nContent-Length: 0\r\n\r\n")
			return
		}
		_, _ = fmt.Fprintf(conn, "RTSP/1.0 200 OK\r\nCSeq: 1\r\nContent-Length: 0\r\n\r\n")
	})

	defer listener.Close()

	checker := NewChecker(config.DeviceConfig{
		Username:       "admin",
		Password:       "secret",
		RequestTimeout: 2 * time.Second,
	})
	ok, err := checker.StreamAvailable(context.Background(), url)
	if err != nil {
		t.Fatalf("StreamAvailable returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected stream to be available")
	}
	if got := requestCount.Load(); got != 2 {
		t.Fatalf("expected 2 requests on first probe, got %d", got)
	}

	ok, err = checker.StreamAvailable(context.Background(), url)
	if err != nil {
		t.Fatalf("StreamAvailable returned error on cached read: %v", err)
	}
	if !ok {
		t.Fatal("expected cached stream availability to remain true")
	}
	if got := requestCount.Load(); got != 2 {
		t.Fatalf("expected cached read to avoid extra requests, got %d", got)
	}
}

func startRTSPTestServer(t *testing.T, handler func(conn net.Conn, firstLine string, headers map[string]string)) (net.Listener, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				firstLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				headers := map[string]string{}
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimSpace(line)
					if line == "" {
						break
					}
					key, value, ok := strings.Cut(line, ":")
					if !ok {
						continue
					}
					headers[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
				}
				handler(conn, firstLine, headers)
			}(conn)
		}
	}()

	return listener, fmt.Sprintf("rtsp://%s/cam/realmonitor?channel=1&subtype=0", listener.Addr().String())
}
