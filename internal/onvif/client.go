package onvif

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	dahuatransport "RCooLeR/DahuaBridge/internal/dahua/transport"
)

const cacheTTL = 15 * time.Minute

type Client struct {
	enabled      bool
	deviceID     string
	serviceURL   string
	username     string
	password     string
	http         *http.Client
	mu           sync.RWMutex
	cached       *Discovery
	cacheExpires time.Time
}

type Discovery struct {
	DeviceServiceURL string    `json:"device_service_url"`
	MediaServiceURL  string    `json:"media_service_url"`
	Profiles         []Profile `json:"profiles"`
	DiscoveredAt     time.Time `json:"discovered_at"`
}

type Profile struct {
	Token       string `json:"token"`
	Name        string `json:"name,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Channel     int    `json:"channel,omitempty"`
	Subtype     int    `json:"subtype,omitempty"`
	StreamURI   string `json:"stream_uri,omitempty"`
	SnapshotURI string `json:"snapshot_uri,omitempty"`
	Source      string `json:"source,omitempty"`
	IsH264      bool   `json:"is_h264"`
	IsSnapshot  bool   `json:"is_snapshot,omitempty"`
}

func New(cfg config.DeviceConfig) *Client {
	baseServiceURL := cfg.OnvifServiceURL
	if strings.TrimSpace(baseServiceURL) == "" {
		baseServiceURL = defaultServiceURL(cfg.BaseURL)
	}

	return &Client{
		enabled:    cfg.ONVIFEnabledValue(),
		deviceID:   cfg.ID,
		serviceURL: baseServiceURL,
		username:   cfg.ONVIFUsernameValue(),
		password:   cfg.ONVIFPasswordValue(),
		http:       newHTTPClient(cfg),
	}
}

func (c *Client) UpdateConfig(cfg config.DeviceConfig) {
	baseServiceURL := cfg.OnvifServiceURL
	if strings.TrimSpace(baseServiceURL) == "" {
		baseServiceURL = defaultServiceURL(cfg.BaseURL)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = cfg.ONVIFEnabledValue()
	c.deviceID = cfg.ID
	c.serviceURL = baseServiceURL
	c.username = cfg.ONVIFUsernameValue()
	c.password = cfg.ONVIFPasswordValue()
	c.http = newHTTPClient(cfg)
	c.cached = nil
	c.cacheExpires = time.Time{}
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

func (c *Client) Discover(ctx context.Context) (*Discovery, error) {
	enabled, deviceID, _, _, _, _ := c.currentState()
	if !enabled {
		return nil, fmt.Errorf("onvif is disabled for device %q", deviceID)
	}

	c.mu.RLock()
	if c.cached != nil && time.Now().Before(c.cacheExpires) {
		cached := *c.cached
		c.mu.RUnlock()
		return &cached, nil
	}
	c.mu.RUnlock()

	capabilities, err := c.getCapabilities(ctx)
	if err != nil {
		return nil, err
	}

	mediaServiceURL := firstNonEmpty(capabilities.Media2XAddr(), capabilities.MediaXAddr())
	if mediaServiceURL == "" {
		return nil, fmt.Errorf("onvif media service xaddr not found")
	}

	profiles, err := c.getProfiles(ctx, mediaServiceURL)
	if err != nil {
		return nil, err
	}

	discovery := &Discovery{
		DeviceServiceURL: c.serviceURL,
		MediaServiceURL:  mediaServiceURL,
		Profiles:         profiles,
		DiscoveredAt:     time.Now(),
	}

	c.mu.Lock()
	c.cached = discovery
	c.cacheExpires = time.Now().Add(cacheTTL)
	c.mu.Unlock()

	copied := *discovery
	return &copied, nil
}

func (d Discovery) H264ProfileCount() int {
	count := 0
	for _, profile := range d.Profiles {
		if profile.IsH264 {
			count++
		}
	}
	return count
}

func (d Discovery) H264ProfileCountForChannel(channel int) int {
	count := 0
	for _, profile := range d.Profiles {
		if profile.IsH264 && profile.Channel == channel {
			count++
		}
	}
	return count
}

func (d Discovery) BestH264ProfileForChannel(channel int) (Profile, bool) {
	candidates := make([]Profile, 0)
	for _, profile := range d.Profiles {
		if !profile.IsH264 {
			continue
		}
		if channel > 0 && profile.Channel != channel {
			continue
		}
		candidates = append(candidates, profile)
	}

	if len(candidates) == 0 {
		if channel == 1 {
			for _, profile := range d.Profiles {
				if profile.IsH264 && profile.Channel == 0 {
					candidates = append(candidates, profile)
				}
			}
		}
	}

	if len(candidates) == 0 {
		return Profile{}, false
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if scoreProfile(candidate) > scoreProfile(best) {
			best = candidate
		}
	}
	return best, true
}

func (d Discovery) ProfileMaps() []map[string]any {
	items := make([]map[string]any, 0, len(d.Profiles))
	for _, profile := range d.Profiles {
		items = append(items, map[string]any{
			"token":        profile.Token,
			"name":         profile.Name,
			"encoding":     profile.Encoding,
			"width":        profile.Width,
			"height":       profile.Height,
			"channel":      profile.Channel,
			"subtype":      profile.Subtype,
			"stream_uri":   profile.StreamURI,
			"snapshot_uri": profile.SnapshotURI,
			"is_h264":      profile.IsH264,
			"source":       profile.Source,
		})
	}
	return items
}

func defaultServiceURL(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}

	host := parsed.Hostname()
	if port := parsed.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	}

	return (&url.URL{
		Scheme: parsed.Scheme,
		Host:   host,
		Path:   "/onvif/device_service",
	}).String()
}

func (c *Client) getCapabilities(ctx context.Context) (capabilities, error) {
	_, _, serviceURL, _, _, _ := c.currentState()
	body := `<tds:GetCapabilities xmlns:tds="http://www.onvif.org/ver10/device/wsdl"><tds:Category>All</tds:Category></tds:GetCapabilities>`
	payload, err := c.soapCall(ctx, serviceURL, body)
	if err != nil {
		return capabilities{}, err
	}

	var envelope capabilitiesEnvelope
	if err := xml.Unmarshal(payload, &envelope); err != nil {
		return capabilities{}, fmt.Errorf("parse onvif capabilities: %w", err)
	}
	if envelope.Body.Fault != nil {
		return capabilities{}, fmt.Errorf("onvif capabilities fault: %s", envelope.Body.Fault.String())
	}
	return envelope.Body.GetCapabilitiesResponse.Capabilities, nil
}

func (c *Client) getProfiles(ctx context.Context, mediaServiceURL string) ([]Profile, error) {
	body := `<trt:GetProfiles xmlns:trt="http://www.onvif.org/ver10/media/wsdl"/>`
	payload, err := c.soapCall(ctx, mediaServiceURL, body)
	if err != nil {
		return nil, err
	}

	var envelope profilesEnvelope
	if err := xml.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("parse onvif profiles: %w", err)
	}
	if envelope.Body.Fault != nil {
		return nil, fmt.Errorf("onvif profiles fault: %s", envelope.Body.Fault.String())
	}

	profiles := make([]Profile, 0, len(envelope.Body.GetProfilesResponse.Profiles))
	for _, item := range envelope.Body.GetProfilesResponse.Profiles {
		streamURI, err := c.getStreamURI(ctx, mediaServiceURL, item.Token)
		if err != nil {
			streamURI = ""
		}
		snapshotURI, err := c.getSnapshotURI(ctx, mediaServiceURL, item.Token)
		if err != nil {
			snapshotURI = ""
		}
		channel, subtype := parseChannelAndSubtype(streamURI)
		profiles = append(profiles, Profile{
			Token:       item.Token,
			Name:        strings.TrimSpace(item.Name),
			Encoding:    strings.TrimSpace(item.VideoEncoderConfiguration.Encoding),
			Width:       item.VideoEncoderConfiguration.Resolution.Width,
			Height:      item.VideoEncoderConfiguration.Resolution.Height,
			Channel:     channel,
			Subtype:     subtype,
			StreamURI:   streamURI,
			SnapshotURI: snapshotURI,
			Source:      "onvif",
			IsH264:      strings.EqualFold(strings.TrimSpace(item.VideoEncoderConfiguration.Encoding), "h264"),
		})
	}

	return profiles, nil
}

func (c *Client) getStreamURI(ctx context.Context, mediaServiceURL string, token string) (string, error) {
	body := `<trt:GetStreamUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">` +
		`<trt:StreamSetup>` +
		`<tt:Stream>RTP-Unicast</tt:Stream>` +
		`<tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport>` +
		`</trt:StreamSetup>` +
		`<trt:ProfileToken>` + xmlEscape(token) + `</trt:ProfileToken>` +
		`</trt:GetStreamUri>`

	payload, err := c.soapCall(ctx, mediaServiceURL, body)
	if err != nil {
		return "", err
	}

	var envelope streamURIEnvelope
	if err := xml.Unmarshal(payload, &envelope); err != nil {
		return "", fmt.Errorf("parse onvif stream uri: %w", err)
	}
	if envelope.Body.Fault != nil {
		return "", fmt.Errorf("onvif stream uri fault: %s", envelope.Body.Fault.String())
	}
	return strings.TrimSpace(envelope.Body.GetStreamURIResponse.MediaURI.URI), nil
}

func (c *Client) getSnapshotURI(ctx context.Context, mediaServiceURL string, token string) (string, error) {
	body := `<trt:GetSnapshotUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl">` +
		`<trt:ProfileToken>` + xmlEscape(token) + `</trt:ProfileToken>` +
		`</trt:GetSnapshotUri>`

	payload, err := c.soapCall(ctx, mediaServiceURL, body)
	if err != nil {
		return "", err
	}

	var envelope snapshotURIEnvelope
	if err := xml.Unmarshal(payload, &envelope); err != nil {
		return "", fmt.Errorf("parse onvif snapshot uri: %w", err)
	}
	if envelope.Body.Fault != nil {
		return "", fmt.Errorf("onvif snapshot uri fault: %s", envelope.Body.Fault.String())
	}
	return strings.TrimSpace(envelope.Body.GetSnapshotURIResponse.MediaURI.URI), nil
}

func (c *Client) soapCall(ctx context.Context, endpoint string, body string) ([]byte, error) {
	_, _, _, username, password, client := c.currentState()
	created := time.Now().UTC().Format(time.RFC3339Nano)
	nonceBytes := []byte(strconv.FormatInt(time.Now().UnixNano(), 10))
	nonceEncoded := base64.StdEncoding.EncodeToString(nonceBytes)
	passwordDigest := wssePasswordDigest(nonceBytes, created, password)

	envelope := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" ` +
		`xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" ` +
		`xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">` +
		`<soap:Header>` +
		`<wsse:Security soap:mustUnderstand="1">` +
		`<wsse:UsernameToken>` +
		`<wsse:Username>` + xmlEscape(username) + `</wsse:Username>` +
		`<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">` + passwordDigest + `</wsse:Password>` +
		`<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">` + nonceEncoded + `</wsse:Nonce>` +
		`<wsu:Created>` + created + `</wsu:Created>` +
		`</wsse:UsernameToken>` +
		`</wsse:Security>` +
		`</soap:Header>` +
		`<soap:Body>` + body + `</soap:Body>` +
		`</soap:Envelope>`

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(envelope))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `application/soap+xml; charset=utf-8`)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("onvif unexpected status %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}

	return payload, nil
}

func wssePasswordDigest(nonce []byte, created string, password string) string {
	sum := sha1.Sum(append(append(append([]byte{}, nonce...), []byte(created)...), []byte(password)...))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func xmlEscape(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

func parseChannelAndSubtype(streamURI string) (int, int) {
	parsed, err := url.Parse(strings.TrimSpace(streamURI))
	if err != nil {
		return 0, 0
	}

	channel := firstQueryInt(parsed.Query(), "channel", "Channel", "chn")
	subtype := firstQueryInt(parsed.Query(), "subtype", "SubType", "sub_type")
	return channel, subtype
}

func firstQueryInt(values url.Values, keys ...string) int {
	for _, key := range keys {
		if raw := strings.TrimSpace(values.Get(key)); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func scoreProfile(profile Profile) int {
	score := profile.Width * profile.Height
	if profile.Subtype == 0 {
		score += 1_000_000_000
	}
	if profile.Channel > 0 {
		score += 1_000_000
	}
	return score
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (c *Client) currentState() (bool, string, string, string, string, *http.Client) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled, c.deviceID, c.serviceURL, c.username, c.password, c.http
}

func newHTTPClient(cfg config.DeviceConfig) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = dahuatransport.LegacyTLSConfig(cfg.InsecureSkipTLS)
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout,
	}
}

type capabilitiesEnvelope struct {
	Body struct {
		Fault                   *soapFault              `xml:"Fault"`
		GetCapabilitiesResponse getCapabilitiesResponse `xml:"GetCapabilitiesResponse"`
	} `xml:"Body"`
}

type getCapabilitiesResponse struct {
	Capabilities capabilities `xml:"Capabilities"`
}

type capabilities struct {
	Media struct {
		XAddr     string `xml:"XAddr"`
		AttrXAddr string `xml:"XAddr,attr"`
	} `xml:"Media"`
	Media2 struct {
		XAddr     string `xml:"XAddr"`
		AttrXAddr string `xml:"XAddr,attr"`
	} `xml:"Media2"`
}

func (c capabilities) MediaXAddr() string {
	return firstNonEmpty(c.Media.XAddr, c.Media.AttrXAddr)
}

func (c capabilities) Media2XAddr() string {
	return firstNonEmpty(c.Media2.XAddr, c.Media2.AttrXAddr)
}

type profilesEnvelope struct {
	Body struct {
		Fault               *soapFault          `xml:"Fault"`
		GetProfilesResponse getProfilesResponse `xml:"GetProfilesResponse"`
	} `xml:"Body"`
}

type getProfilesResponse struct {
	Profiles []profileXML `xml:"Profiles"`
}

type profileXML struct {
	Token                     string                `xml:"token,attr"`
	Name                      string                `xml:"Name"`
	VideoEncoderConfiguration videoEncoderConfigXML `xml:"VideoEncoderConfiguration"`
}

type videoEncoderConfigXML struct {
	Encoding   string        `xml:"Encoding"`
	Resolution resolutionXML `xml:"Resolution"`
}

type resolutionXML struct {
	Width  int `xml:"Width"`
	Height int `xml:"Height"`
}

type streamURIEnvelope struct {
	Body struct {
		Fault                *soapFault           `xml:"Fault"`
		GetStreamURIResponse getStreamURIResponse `xml:"GetStreamUriResponse"`
	} `xml:"Body"`
}

type getStreamURIResponse struct {
	MediaURI mediaURI `xml:"MediaUri"`
}

type mediaURI struct {
	URI string `xml:"Uri"`
}

type snapshotURIEnvelope struct {
	Body struct {
		Fault                  *soapFault             `xml:"Fault"`
		GetSnapshotURIResponse getSnapshotURIResponse `xml:"GetSnapshotUriResponse"`
	} `xml:"Body"`
}

type getSnapshotURIResponse struct {
	MediaURI mediaURI `xml:"MediaUri"`
}

type soapFault struct {
	Reason struct {
		Text string `xml:"Text"`
	} `xml:"Reason"`
	Code struct {
		Value string `xml:"Value"`
	} `xml:"Code"`
}

func (f soapFault) String() string {
	return firstNonEmpty(f.Reason.Text, f.Code.Value, "unknown soap fault")
}
