package onvif

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"RCooLeR/DahuaBridge/internal/config"
)

func TestSOAPCallUsesRandomUUIDNonce(t *testing.T) {
	var requestBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read SOAP request: %v", err)
		}
		requestBodies = append(requestBodies, string(body))
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client := &Client{username: "operator", password: "secret", http: server.Client()}
	for range 2 {
		if _, err := client.soapCall(context.Background(), server.URL, `<test/>`); err != nil {
			t.Fatalf("soapCall returned error: %v", err)
		}
	}
	if len(requestBodies) != 2 {
		t.Fatalf("SOAP request count = %d, want 2", len(requestBodies))
	}

	var previousNonce string
	for index, body := range requestBodies {
		nonceText := soapElementText(t, body, "<wsse:Nonce ", "</wsse:Nonce>")
		nonceBytes, err := base64.StdEncoding.DecodeString(nonceText)
		if err != nil {
			t.Fatalf("decode request %d nonce: %v", index, err)
		}
		if len(nonceBytes) != len(uuid.UUID{}) {
			t.Fatalf("request %d nonce length = %d, want 16", index, len(nonceBytes))
		}
		if nonceBytes[6]&0xf0 != 0x40 || nonceBytes[8]&0xc0 != 0x80 {
			t.Fatalf("request %d nonce is not an RFC 9562 version 4 UUID", index)
		}
		if nonceText == previousNonce {
			t.Fatalf("request %d reused nonce %q", index, nonceText)
		}
		previousNonce = nonceText

		created := soapElementText(t, body, "<wsu:Created>", "</wsu:Created>")
		digest := soapElementText(t, body, "<wsse:Password ", "</wsse:Password>")
		if want := wssePasswordDigest(nonceBytes, created, "secret"); digest != want {
			t.Fatalf("request %d password digest = %q, want %q", index, digest, want)
		}
	}
}

func soapElementText(t *testing.T, document string, startTag string, endTag string) string {
	t.Helper()
	start := strings.Index(document, startTag)
	if start < 0 {
		t.Fatalf("missing element start %q", startTag)
	}
	openEnd := strings.Index(document[start:], ">")
	if openEnd < 0 {
		t.Fatalf("unterminated element start %q", startTag)
	}
	start += openEnd + 1
	end := strings.Index(document[start:], endTag)
	if end < 0 {
		t.Fatalf("missing element end %q", endTag)
	}
	return document[start : start+end]
}

func TestDiscoverIntegration(t *testing.T) {
	server, counts := newTestONVIFServer(t, onvifServerOptions{
		useMedia2: true,
		profiles: []testONVIFProfile{
			{
				Token:       "Profile_Main",
				Name:        "MainStream-H264",
				Encoding:    "H264",
				Width:       1920,
				Height:      1080,
				StreamURI:   "rtsp://192.0.2.120:554/cam/realmonitor?channel=1&subtype=0",
				SnapshotURI: "http://192.0.2.120/cgi-bin/snapshot.cgi?channel=1",
			},
			{
				Token:       "Profile_Sub",
				Name:        "SubStream-H264",
				Encoding:    "H264",
				Width:       704,
				Height:      576,
				StreamURI:   "rtsp://192.0.2.120:554/cam/realmonitor?channel=1&subtype=1",
				SnapshotURI: "http://192.0.2.120/cgi-bin/snapshot.cgi?channel=1&subtype=1",
			},
			{
				Token:       "Profile_HEVC",
				Name:        "MainStream-H265",
				Encoding:    "H265",
				Width:       3840,
				Height:      2160,
				StreamURI:   "rtsp://192.0.2.120:554/cam/realmonitor?channel=1&subtype=0",
				SnapshotURI: "http://192.0.2.120/cgi-bin/snapshot.cgi?channel=1&main=hevc",
			},
		},
	})
	defer server.Close()

	client := New(config.DeviceConfig{
		ID:              "yard_ipc",
		BaseURL:         server.URL,
		Username:        "admin",
		Password:        "secret",
		OnvifEnabled:    boolPtr(true),
		OnvifServiceURL: server.URL + "/onvif/device_service",
		RequestTimeout:  time.Second,
	})

	discovery, err := client.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	if discovery.DeviceServiceURL != server.URL+"/onvif/device_service" {
		t.Fatalf("unexpected device service url %q", discovery.DeviceServiceURL)
	}
	if discovery.MediaServiceURL != server.URL+"/onvif/media2_service" {
		t.Fatalf("unexpected media service url %q", discovery.MediaServiceURL)
	}
	if len(discovery.Profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(discovery.Profiles))
	}
	if discovery.H264ProfileCount() != 2 {
		t.Fatalf("expected 2 h264 profiles, got %d", discovery.H264ProfileCount())
	}
	if discovery.H264ProfileCountForChannel(1) != 2 {
		t.Fatalf("expected 2 h264 channel profiles, got %d", discovery.H264ProfileCountForChannel(1))
	}

	best, ok := discovery.BestH264ProfileForChannel(1)
	if !ok {
		t.Fatal("expected best h264 profile")
	}
	if best.Token != "Profile_Main" {
		t.Fatalf("unexpected best profile token %q", best.Token)
	}
	if best.Subtype != 0 || best.Channel != 1 {
		t.Fatalf("unexpected best profile stream selection %+v", best)
	}
	if best.SnapshotURI != "http://192.0.2.120/cgi-bin/snapshot.cgi?channel=1" {
		t.Fatalf("unexpected best profile snapshot uri %q", best.SnapshotURI)
	}

	if counts.capabilities != 1 {
		t.Fatalf("expected 1 capabilities call, got %d", counts.capabilities)
	}
	if counts.profiles != 1 {
		t.Fatalf("expected 1 profiles call, got %d", counts.profiles)
	}
	if counts.streamURIs != 3 {
		t.Fatalf("expected 3 stream uri calls, got %d", counts.streamURIs)
	}
	if counts.snapshotURIs != 3 {
		t.Fatalf("expected 3 snapshot uri calls, got %d", counts.snapshotURIs)
	}
}

func TestDiscoverUsesCache(t *testing.T) {
	server, counts := newTestONVIFServer(t, onvifServerOptions{
		useMedia2: false,
		profiles: []testONVIFProfile{
			{
				Token:       "Profile_Main",
				Name:        "MainStream-H264",
				Encoding:    "H264",
				Width:       1920,
				Height:      1080,
				StreamURI:   "rtsp://192.0.2.120:554/cam/realmonitor?channel=2&subtype=0",
				SnapshotURI: "http://192.0.2.120/cgi-bin/snapshot.cgi?channel=2",
			},
		},
	})
	defer server.Close()

	client := New(config.DeviceConfig{
		ID:              "west20_nvr",
		BaseURL:         server.URL,
		Username:        "admin",
		Password:        "secret",
		OnvifEnabled:    boolPtr(true),
		OnvifServiceURL: server.URL + "/onvif/device_service",
		RequestTimeout:  time.Second,
	})

	first, err := client.Discover(context.Background())
	if err != nil {
		t.Fatalf("first Discover returned error: %v", err)
	}
	second, err := client.Discover(context.Background())
	if err != nil {
		t.Fatalf("second Discover returned error: %v", err)
	}

	if first.MediaServiceURL != server.URL+"/onvif/media_service" {
		t.Fatalf("unexpected media fallback url %q", first.MediaServiceURL)
	}
	if second.MediaServiceURL != first.MediaServiceURL {
		t.Fatalf("expected cached discovery to match first result")
	}
	if counts.capabilities != 1 || counts.profiles != 1 || counts.streamURIs != 1 || counts.snapshotURIs != 1 {
		t.Fatalf("expected cached requests to reuse first discovery, got %+v", counts)
	}
}

func TestDiscoverKeepsProfileWhenStreamURIRequestFails(t *testing.T) {
	server, _ := newTestONVIFServer(t, onvifServerOptions{
		useMedia2: false,
		profiles: []testONVIFProfile{
			{
				Token:          "Profile_Main",
				Name:           "MainStream-H264",
				Encoding:       "H264",
				Width:          1920,
				Height:         1080,
				StreamURIFault: true,
				SnapshotURI:    "http://192.0.2.120/cgi-bin/snapshot.cgi?channel=1",
			},
		},
	})
	defer server.Close()

	client := New(config.DeviceConfig{
		ID:              "yard_ipc",
		BaseURL:         server.URL,
		Username:        "admin",
		Password:        "secret",
		OnvifEnabled:    boolPtr(true),
		OnvifServiceURL: server.URL + "/onvif/device_service",
		RequestTimeout:  time.Second,
	})

	discovery, err := client.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(discovery.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(discovery.Profiles))
	}
	if discovery.Profiles[0].StreamURI != "" {
		t.Fatalf("expected empty stream uri on stream-uri fault, got %q", discovery.Profiles[0].StreamURI)
	}
	if discovery.Profiles[0].SnapshotURI != "http://192.0.2.120/cgi-bin/snapshot.cgi?channel=1" {
		t.Fatalf("unexpected snapshot uri %q", discovery.Profiles[0].SnapshotURI)
	}
	if !discovery.Profiles[0].IsH264 {
		t.Fatalf("expected h264 profile to remain marked as h264")
	}
}

func TestDiscoverKeepsProfileWhenSnapshotURIRequestFails(t *testing.T) {
	server, _ := newTestONVIFServer(t, onvifServerOptions{
		useMedia2: false,
		profiles: []testONVIFProfile{
			{
				Token:            "Profile_Main",
				Name:             "MainStream-H264",
				Encoding:         "H264",
				Width:            1920,
				Height:           1080,
				StreamURI:        "rtsp://192.0.2.120:554/cam/realmonitor?channel=1&subtype=0",
				SnapshotURIFault: true,
			},
		},
	})
	defer server.Close()

	client := New(config.DeviceConfig{
		ID:              "yard_ipc",
		BaseURL:         server.URL,
		Username:        "admin",
		Password:        "secret",
		OnvifEnabled:    boolPtr(true),
		OnvifServiceURL: server.URL + "/onvif/device_service",
		RequestTimeout:  time.Second,
	})

	discovery, err := client.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(discovery.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(discovery.Profiles))
	}
	if discovery.Profiles[0].SnapshotURI != "" {
		t.Fatalf("expected empty snapshot uri on snapshot-uri fault, got %q", discovery.Profiles[0].SnapshotURI)
	}
	if discovery.Profiles[0].StreamURI == "" {
		t.Fatal("expected stream uri to survive snapshot-uri fault")
	}
}

func TestParseChannelAndSubtype(t *testing.T) {
	channel, subtype := parseChannelAndSubtype("rtsp://nvr.example.local:554/cam/realmonitor?channel=3&subtype=1")
	if channel != 3 {
		t.Fatalf("unexpected channel %d", channel)
	}
	if subtype != 1 {
		t.Fatalf("unexpected subtype %d", subtype)
	}
}

func TestBestH264ProfileForChannel(t *testing.T) {
	discovery := Discovery{
		Profiles: []Profile{
			{Token: "sub", Channel: 2, Subtype: 1, Width: 704, Height: 576, Encoding: "H264", IsH264: true},
			{Token: "main", Channel: 2, Subtype: 0, Width: 1920, Height: 1080, Encoding: "H264", IsH264: true},
			{Token: "hevc", Channel: 2, Subtype: 0, Width: 3840, Height: 2160, Encoding: "H265", IsH264: false},
		},
	}

	profile, ok := discovery.BestH264ProfileForChannel(2)
	if !ok {
		t.Fatal("expected h264 profile")
	}
	if profile.Token != "main" {
		t.Fatalf("unexpected best profile token %q", profile.Token)
	}
}

func TestH264ProfileCountForChannel(t *testing.T) {
	discovery := Discovery{
		Profiles: []Profile{
			{Channel: 1, IsH264: true},
			{Channel: 1, IsH264: true},
			{Channel: 1, IsH264: false},
			{Channel: 2, IsH264: true},
		},
	}

	if got := discovery.H264ProfileCount(); got != 3 {
		t.Fatalf("unexpected h264 profile count %d", got)
	}
	if got := discovery.H264ProfileCountForChannel(1); got != 2 {
		t.Fatalf("unexpected channel h264 profile count %d", got)
	}
}

func TestMediaXAddrFallbacks(t *testing.T) {
	caps := capabilities{}
	caps.Media.XAddr = "http://192.168.1.10/onvif/media"
	caps.Media2.AttrXAddr = "http://192.168.1.10/onvif/media2"

	if caps.MediaXAddr() != "http://192.168.1.10/onvif/media" {
		t.Fatalf("unexpected media xaddr %q", caps.MediaXAddr())
	}
	if caps.Media2XAddr() != "http://192.168.1.10/onvif/media2" {
		t.Fatalf("unexpected media2 xaddr %q", caps.Media2XAddr())
	}
}

type testONVIFProfile struct {
	Token            string
	Name             string
	Encoding         string
	Width            int
	Height           int
	StreamURI        string
	SnapshotURI      string
	StreamURIFault   bool
	SnapshotURIFault bool
}

type onvifServerOptions struct {
	useMedia2 bool
	profiles  []testONVIFProfile
}

type onvifRequestCounts struct {
	mu           sync.Mutex
	capabilities int
	profiles     int
	streamURIs   int
	snapshotURIs int
}

func newTestONVIFServer(t *testing.T, options onvifServerOptions) (*httptest.Server, *onvifRequestCounts) {
	t.Helper()

	counts := &onvifRequestCounts{}
	profilesByToken := make(map[string]testONVIFProfile, len(options.profiles))
	for _, profile := range options.profiles {
		profilesByToken[profile.Token] = profile
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		body := string(bodyBytes)
		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")

		switch {
		case strings.Contains(body, "GetCapabilities"):
			counts.mu.Lock()
			counts.capabilities++
			counts.mu.Unlock()
			mediaTag := "<tt:Media><tt:XAddr>" + server.URL + "/onvif/media_service</tt:XAddr></tt:Media>"
			if options.useMedia2 {
				mediaTag += "<tt:Media2><tt:XAddr>" + server.URL + "/onvif/media2_service</tt:XAddr></tt:Media2>"
			}
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">`+
				`<soap:Body><tds:GetCapabilitiesResponse><tds:Capabilities>`+
				mediaTag+
				`</tds:Capabilities></tds:GetCapabilitiesResponse></soap:Body></soap:Envelope>`)
		case strings.Contains(body, "GetProfiles"):
			counts.mu.Lock()
			counts.profiles++
			counts.mu.Unlock()
			var profilesXML strings.Builder
			for _, profile := range options.profiles {
				profilesXML.WriteString(`<trt:Profiles token="` + profile.Token + `">` +
					`<tt:Name>` + profile.Name + `</tt:Name>` +
					`<tt:VideoEncoderConfiguration>` +
					`<tt:Encoding>` + profile.Encoding + `</tt:Encoding>` +
					`<tt:Resolution><tt:Width>` + intToString(profile.Width) + `</tt:Width><tt:Height>` + intToString(profile.Height) + `</tt:Height></tt:Resolution>` +
					`</tt:VideoEncoderConfiguration>` +
					`</trt:Profiles>`)
			}
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">`+
				`<soap:Body><trt:GetProfilesResponse>`+profilesXML.String()+`</trt:GetProfilesResponse></soap:Body></soap:Envelope>`)
		case strings.Contains(body, "GetStreamUri"):
			counts.mu.Lock()
			counts.streamURIs++
			counts.mu.Unlock()
			token := extractProfileToken(body)
			profile, ok := profilesByToken[token]
			if !ok {
				t.Fatalf("unknown stream token %q", token)
			}
			if profile.StreamURIFault {
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
					`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">`+
					`<soap:Body><soap:Fault><soap:Code><soap:Value>soap:Sender</soap:Value></soap:Code><soap:Reason><soap:Text>stream fault</soap:Text></soap:Reason></soap:Fault></soap:Body></soap:Envelope>`)
				return
			}
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="http://www.onvif.org/ver10/media/wsdl">`+
				`<soap:Body><trt:GetStreamUriResponse><trt:MediaUri><trt:Uri>`+xmlEscape(profile.StreamURI)+`</trt:Uri></trt:MediaUri></trt:GetStreamUriResponse></soap:Body></soap:Envelope>`)
		case strings.Contains(body, "GetSnapshotUri"):
			counts.mu.Lock()
			counts.snapshotURIs++
			counts.mu.Unlock()
			token := extractProfileToken(body)
			profile, ok := profilesByToken[token]
			if !ok {
				t.Fatalf("unknown snapshot token %q", token)
			}
			if profile.SnapshotURIFault {
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
					`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">`+
					`<soap:Body><soap:Fault><soap:Code><soap:Value>soap:Sender</soap:Value></soap:Code><soap:Reason><soap:Text>snapshot fault</soap:Text></soap:Reason></soap:Fault></soap:Body></soap:Envelope>`)
				return
			}
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="http://www.onvif.org/ver10/media/wsdl">`+
				`<soap:Body><trt:GetSnapshotUriResponse><trt:MediaUri><trt:Uri>`+xmlEscape(profile.SnapshotURI)+`</trt:Uri></trt:MediaUri></trt:GetSnapshotUriResponse></soap:Body></soap:Envelope>`)
		default:
			t.Fatalf("unexpected ONVIF request body: %s", body)
		}
	}))

	return server, counts
}

func extractProfileToken(body string) string {
	startTag := "<trt:ProfileToken>"
	endTag := "</trt:ProfileToken>"
	start := strings.Index(body, startTag)
	end := strings.Index(body, endTag)
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	start += len(startTag)
	return strings.TrimSpace(body[start:end])
}

func intToString(value int) string {
	return strconv.Itoa(value)
}

func boolPtr(value bool) *bool {
	return &value
}
