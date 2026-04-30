package cgi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/metrics"
)

func TestClientRetriesStaleDigestChallengeAfterAuthorityCheckFailure(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if r.Header.Get("Authorization") == "" {
				t.Fatalf("expected preemptive authorization header on first request")
			}
			http.Error(w, "Authority:check failure.", http.StatusForbidden)
		case 2:
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("expected second request to fetch a fresh challenge without authorization")
			}
			w.Header().Set("WWW-Authenticate", `Digest realm="Login to test", nonce="nonce2", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
		case 3:
			if r.Header.Get("Authorization") == "" {
				t.Fatalf("expected third request to include refreshed authorization header")
			}
			_, _ = w.Write([]byte("OK"))
		default:
			t.Fatalf("unexpected request count %d", requests)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:             "test",
		BaseURL:        server.URL,
		Username:       "user",
		Password:       "pass",
		RequestTimeout: 5 * time.Second,
	}
	client := New(cfg, metrics.New(buildinfo.Info()))
	client.setChallenge(map[string]string{
		"realm":     "Login to test",
		"nonce":     "stale",
		"qop":       "auth",
		"algorithm": "MD5",
	})

	body, err := client.GetText(context.Background(), "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"setConfig"},
		"foo":    []string{"bar"},
	})
	if err != nil {
		t.Fatalf("GetText returned error: %v", err)
	}
	if body != "OK" {
		t.Fatalf("unexpected body %q", body)
	}
	if requests != 3 {
		t.Fatalf("expected 3 requests, got %d", requests)
	}
}

func TestShouldRetryDigestChallengePreservesNonRetryResponseBody(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       io.NopCloser(bytes.NewBufferString("permission denied")),
	}

	if shouldRetryDigestChallenge(resp) {
		t.Fatalf("expected non-auth 403 body to remain terminal")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "permission denied" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestEncodeQueryPlacesActionFirst(t *testing.T) {
	t.Parallel()

	encoded := encodeQuery(url.Values{
		"RecordMode[4].Mode":       []string{"1"},
		"RecordMode[4].ModeExtra1": []string{"2"},
		"RecordMode[4].ModeExtra2": []string{"2"},
		"action":                   []string{"setConfig"},
	})

	expected := "action=setConfig&RecordMode%5B4%5D.Mode=1&RecordMode%5B4%5D.ModeExtra1=2&RecordMode%5B4%5D.ModeExtra2=2"
	if encoded != expected {
		t.Fatalf("unexpected query encoding %q", encoded)
	}
}
