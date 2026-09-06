package httpserver

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
)

func TestAuthenticatedManifestResourceChains(t *testing.T) {
	const token = "test:@/token&value"
	media := stubMediaReader{
		enabled: true,
		hlsPlaylist: func(context.Context, string, string) ([]byte, error) {
			return []byte("#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4?version=1\"\n#EXTINF:2,\nsegment_001.ts\n"), nil
		},
		hlsSegment: func(context.Context, string, string, string) ([]byte, string, error) {
			return []byte("media"), "video/mp2t", nil
		},
		dashManifest: func(context.Context, string, string) ([]byte, error) {
			return []byte(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011"><Period><SegmentTemplate initialization="init-$RepresentationID$.m4s" media="chunk-$RepresentationID$-$Number%05d$.m4s?version=1&amp;mode=live"/></Period></MPD>`), nil
		},
		dashAsset: func(context.Context, string, string, string) ([]byte, string, error) {
			return []byte("media"), "video/mp4", nil
		},
	}
	cfg := config.HTTPConfig{HealthPath: "/healthz", MetricsPath: "/metrics", AuthToken: token, AuthQueryToken: true}
	server := newTestServerWithConfig(cfg, stubSnapshotReader{}, media, stubActionReader{}, stubEventReader{})
	endpoint := httptest.NewServer(server.httpServer.Handler)
	defer endpoint.Close()
	for _, kind := range []string{"hls", "dash"} {
		t.Run(kind, func(t *testing.T) {
			name := "index.m3u8"
			if kind == "dash" {
				name = "manifest.mpd"
			}
			manifestURL, _ := url.Parse(endpoint.URL + "/api/v1/media/" + kind + "/cam1/stable/" + name + "?auth_token=" + url.QueryEscape(token))
			response, err := http.Get(manifestURL.String())
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("manifest returned %d", response.StatusCode)
			}
			var resources []string
			if kind == "hls" {
				resources = append(resources, hlsURIAttribute.FindStringSubmatch(string(body))[1])
				for _, line := range strings.Split(string(body), "\n") {
					if line != "" && !strings.HasPrefix(line, "#") {
						resources = append(resources, line)
					}
				}
			} else {
				decoder := xml.NewDecoder(strings.NewReader(string(body)))
				for {
					item, err := decoder.Token()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatalf("invalid rewritten XML: %v", err)
					}
					if start, ok := item.(xml.StartElement); ok {
						for _, attr := range start.Attr {
							if attr.Name.Local == "media" || attr.Name.Local == "initialization" {
								resources = append(resources, strings.NewReplacer("$RepresentationID$", "0", "$Number%05d$", "00001").Replace(attr.Value))
							}
						}
					}
				}
			}
			if len(resources) != 2 {
				t.Fatalf("expected initialization and media URLs, got %d", len(resources))
			}
			for _, resource := range resources {
				relative, err := url.Parse(resource)
				if err != nil {
					t.Fatal(err)
				}
				if relative.Query().Get(authTokenQueryParam) != token {
					t.Fatal("manifest resource lost authentication")
				}
				absolute := manifestURL.ResolveReference(relative)
				asset, err := http.Get(absolute.String())
				if err != nil {
					t.Fatal(err)
				}
				asset.Body.Close()
				if asset.StatusCode != http.StatusOK {
					t.Fatalf("resource returned %d", asset.StatusCode)
				}
				absolute.RawQuery = ""
				denied, err := http.Get(absolute.String())
				if err != nil {
					t.Fatal(err)
				}
				denied.Body.Close()
				if denied.StatusCode != http.StatusUnauthorized {
					t.Fatal("unauthenticated resource bypassed protection")
				}
			}
		})
	}
}

func TestManifestAuthorizationPreservesExternalResourcesAndDisabledQueryAuth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://bridge/api/v1/media/hls/cam/quality/index.m3u8", nil)
	for _, body := range []string{
		"#EXTM3U\nhttps://external.example/segment.ts\n",
		`<MPD><BaseURL>https://external.example/</BaseURL><SegmentTemplate media="segment.m4s"/></MPD>`,
	} {
		result := authorizeManifest([]byte(body), request, config.HTTPConfig{AuthToken: "private", AuthQueryToken: true}, strings.HasPrefix(body, "<"))
		if string(result) != body {
			t.Fatal("external media resource gained bridge credentials")
		}
	}
	body := []byte("#EXTM3U\nsegment.ts\n")
	if string(authorizeManifest(body, request, config.HTTPConfig{AuthToken: "private"}, false)) != string(body) {
		t.Fatal("query authentication policy was overridden")
	}
}
