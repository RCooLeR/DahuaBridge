package rtsp

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
)

func TestDescribeAvailableAuthenticatesURLCredentialsWithoutExposingUserinfo(t *testing.T) {
	const username, password = "camera-user", "p@ss%word: !"
	const realm, nonce = "camera, lab", "challenge-nonce"
	var requests atomic.Int32
	streamURL := startStrictRTSPServer(t, func(conn net.Conn, firstLine string, headers textproto.MIMEHeader) {
		requests.Add(1)
		if !strings.HasPrefix(firstLine, "DESCRIBE rtsp://127.0.0.1:") || strings.Contains(firstLine, username) || strings.Contains(firstLine, password) {
			t.Errorf("unexpected DESCRIBE request line: %q", firstLine)
		}
		auth := headers.Get("Authorization")
		if auth == "" {
			fmt.Fprintf(conn, "RTSP/1.0 401 Unauthorized\r\nCSeq: 1\r\nWWW-Authenticate: Digest realm=%q, nonce=%q, qop=\"auth,auth-int\"\r\nContent-Length: 5\r\n\r\nretry", realm, nonce)
			return
		}
		fields, err := strictDigestParameters(strings.TrimPrefix(auth, "Digest "))
		if err != nil {
			t.Errorf("parse authorization: %v", err)
			return
		}
		requestURI := strings.Fields(firstLine)[1]
		ha1 := md5Hex(username + ":" + realm + ":" + password)
		ha2 := md5Hex("DESCRIBE:" + requestURI)
		want := md5Hex(ha1 + ":" + nonce + ":" + fields["nc"] + ":" + fields["cnonce"] + ":auth:" + ha2)
		if fields["username"] != username || fields["uri"] != requestURI || fields["response"] != want || fields["qop"] != "auth" {
			t.Error("digest did not authenticate URL credentials against the clean request URI")
			fmt.Fprint(conn, "RTSP/1.0 401 Unauthorized\r\nContent-Length: 0\r\n\r\n")
			return
		}
		fmt.Fprint(conn, "RTSP/1.0 200 OK\r\nCSeq: 2\r\nContent-Type: application/sdp\r\nContent-Length: 0\r\n\r\n")
	})
	parsed, _ := url.Parse(streamURL)
	parsed.User = url.UserPassword(username, password)
	available, err := DescribeAvailable(context.Background(), parsed.String(), time.Second, false)
	if err != nil || !available {
		t.Fatalf("authenticated DESCRIBE failed: available=%v err=%v", available, err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("expected exactly one challenge and one authenticated request, got %d", got)
	}
}

func TestDescribeAvailableSupportsBasicAuthentication(t *testing.T) {
	streamURL := startStrictRTSPServer(t, func(conn net.Conn, firstLine string, headers textproto.MIMEHeader) {
		if headers.Get("Authorization") == "" {
			fmt.Fprint(conn, "RTSP/1.0 401 Unauthorized\r\nWWW-Authenticate: Basic realm=\"camera\"\r\nContent-Length: 0\r\n\r\n")
			return
		}
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("viewer:camera-pass"))
		if headers.Get("Authorization") != want {
			t.Error("Basic authentication did not use URL credentials")
		}
		if strings.Contains(firstLine, "viewer") || strings.Contains(firstLine, "camera-pass") {
			t.Error("request URI exposed userinfo")
		}
		fmt.Fprint(conn, "RTSP/1.0 200 OK\r\nContent-Length: 0\r\n\r\n")
	})
	parsed, _ := url.Parse(streamURL)
	parsed.User = url.UserPassword("viewer", "camera-pass")
	if available, err := DescribeAvailable(context.Background(), parsed.String(), time.Second, false); err != nil || !available {
		t.Fatalf("Basic DESCRIBE failed: available=%v err=%v", available, err)
	}
}

func TestDescribeAvailableRejectsFinalUnauthorizedWithoutRetry(t *testing.T) {
	var requests atomic.Int32
	streamURL := startStrictRTSPServer(t, func(conn net.Conn, _ string, _ textproto.MIMEHeader) {
		requests.Add(1)
		fmt.Fprint(conn, "RTSP/1.0 401 Unauthorized\r\nWWW-Authenticate: Digest realm=\"camera\", nonce=\"rotating-nonce\"\r\nContent-Length: 0\r\n\r\n")
	})
	parsed, _ := url.Parse(streamURL)
	parsed.User = url.UserPassword("viewer", "rejected-secret")
	available, err := DescribeAvailable(context.Background(), parsed.String(), time.Second, false)
	if available || err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected final 401 to fail, got available=%v err=%v", available, err)
	}
	if strings.Contains(err.Error(), "rejected-secret") || strings.Contains(err.Error(), "viewer") || strings.Contains(err.Error(), "rtsp://") {
		t.Fatal("error exposed credentials or stream URL")
	}
	if requests.Load() != 2 {
		t.Fatalf("expected no retry after rejected authentication, got %d requests", requests.Load())
	}
}

func TestDescribeAvailableRejectsStatusFailures(t *testing.T) {
	for _, status := range []int{403, 404, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			streamURL := startStrictRTSPServer(t, func(conn net.Conn, _ string, _ textproto.MIMEHeader) {
				fmt.Fprintf(conn, "RTSP/1.0 %d Failed\r\nContent-Length: 0\r\n\r\n", status)
			})
			available, err := DescribeAvailable(context.Background(), streamURL, time.Second, false)
			if available || err == nil || !strings.Contains(err.Error(), fmt.Sprint(status)) {
				t.Fatalf("expected status failure, got available=%v err=%v", available, err)
			}
		})
	}
}

func TestDescribeAvailableUnavailableEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	available, err := DescribeAvailable(context.Background(), "rtsp://viewer:private@"+address+"/live", time.Second, false)
	if available || err == nil {
		t.Fatalf("expected unavailable endpoint, got available=%v err=%v", available, err)
	}
	if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "viewer") {
		t.Fatal("network error exposed URL credentials")
	}
}

func TestDescribeAvailableCancellationInterruptsEstablishedRead(t *testing.T) {
	requestReceived := make(chan struct{}, 1)
	streamURL := startStrictRTSPServer(t, func(_ net.Conn, _ string, _ textproto.MIMEHeader) {
		requestReceived <- struct{}{}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := DescribeAvailable(ctx, streamURL, 5*time.Second, false)
		result <- err
	}()
	select {
	case <-requestReceived:
	case <-time.After(time.Second):
		t.Fatal("probe did not send DESCRIBE")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt the established socket read")
	}
}

func TestDescribeAvailableTimeoutBoundsWholeProbe(t *testing.T) {
	streamURL := startStrictRTSPServer(t, func(_ net.Conn, _ string, _ textproto.MIMEHeader) {})
	started := time.Now()
	available, err := DescribeAvailable(context.Background(), streamURL, 50*time.Millisecond, false)
	if available || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got available=%v err=%v", available, err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("probe exceeded its timeout")
	}
}

func TestDescribeAvailableSanitizesParserErrors(t *testing.T) {
	streamURL := startStrictRTSPServer(t, func(conn net.Conn, _ string, _ textproto.MIMEHeader) {
		fmt.Fprint(conn, "RTSP/1.0 rtsp://viewer:echoed-secret@camera/live\r\n\r\n")
	})
	for _, input := range []string{streamURL, "rtsp://viewer:private%zz@camera/live"} {
		available, err := DescribeAvailable(context.Background(), input, time.Second, false)
		if available || err == nil {
			t.Fatalf("expected parsing failure, got available=%v err=%v", available, err)
		}
		for _, secret := range []string{"viewer", "private", "echoed-secret", "rtsp://"} {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error exposed input or server response: %v", err)
			}
		}
	}
}

func TestLegacyStreamAvailableStillTreatsFinal401AsReachable(t *testing.T) {
	listener, streamURL := startRTSPTestServer(t, func(conn net.Conn, _ string, _ map[string]string) {
		fmt.Fprint(conn, "RTSP/1.0 401 Unauthorized\r\nWWW-Authenticate: Digest realm=\"camera\", nonce=\"nonce\"\r\nContent-Length: 0\r\n\r\n")
	})
	defer listener.Close()
	checker := NewChecker(config.DeviceConfig{Username: "viewer", Password: "secret", RequestTimeout: time.Second})
	available, err := checker.StreamAvailable(context.Background(), streamURL)
	if !available || err != nil {
		t.Fatalf("legacy reachability behavior changed: available=%v err=%v", available, err)
	}
}

func startStrictRTSPServer(t *testing.T, handler func(net.Conn, string, textproto.MIMEHeader)) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				reader := bufio.NewReader(conn)
				for {
					firstLine, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					headers, err := textproto.NewReader(reader).ReadMIMEHeader()
					if err != nil {
						return
					}
					handler(conn, firstLine, headers)
				}
			}()
		}
	}()
	return fmt.Sprintf("rtsp://%s/cam/realmonitor?channel=1&subtype=0", listener.Addr())
}
