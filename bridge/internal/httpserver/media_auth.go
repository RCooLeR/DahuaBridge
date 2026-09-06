package httpserver

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"RCooLeR/DahuaBridge/internal/config"
)

var hlsURIAttribute = regexp.MustCompile(`\bURI="([^"]*)"`)
var dashURIAttribute = regexp.MustCompile(`\b(media|initialization|sourceURL|index)\s*=\s*("[^"]*"|'[^']*')`)
var dashBaseURL = regexp.MustCompile(`(?s)<(?:[A-Za-z_][\w.-]*:)?BaseURL\b[^>]*>(.*?)</(?:[A-Za-z_][\w.-]*:)?BaseURL\s*>`)

// authorizeManifest carries authorization from a generated manifest to its local
// resources. Relative URI resolution does not inherit a manifest's query token.
// Header-only deployments retain their explicit auth_query_token policy.
func authorizeManifest(body []byte, r *http.Request, cfg config.HTTPConfig, dash bool) []byte {
	token := strings.TrimSpace(cfg.AuthToken)
	if !cfg.AuthQueryToken || token == "" {
		return body
	}
	resourceURL := func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.User != nil || (parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https") {
			return raw
		}
		if parsed.Host != "" && !strings.EqualFold(parsed.Host, r.Host) {
			return raw
		}
		if parsed.Path == "" {
			return raw
		}
		// Current media manifests contain paths local to their stream directory.
		// Do not append the bridge token to unrelated absolute-path resources.
		if strings.HasPrefix(parsed.Path, "/") && !strings.HasPrefix(parsed.Path, "/api/v1/media/") {
			return raw
		}
		query := parsed.Query()
		query.Del("token")
		query.Set(authTokenQueryParam, token)
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	if dash {
		// A relative SegmentTemplate can inherit an external BaseURL. Never
		// place a bridge credential on a resource that would resolve there.
		for _, match := range dashBaseURL.FindAllSubmatch(body, -1) {
			base, err := url.Parse(strings.TrimSpace(html.UnescapeString(string(match[1]))))
			if err != nil || (base.Host != "" && !strings.EqualFold(base.Host, r.Host)) {
				return body
			}
		}
		// Edit URI attributes in place to preserve namespace prefixes and the
		// rest of the manifest's XML structure.
		return dashURIAttribute.ReplaceAllFunc(body, func(match []byte) []byte {
			parts := dashURIAttribute.FindSubmatch(match)
			quoted := string(parts[2])
			raw := html.UnescapeString(quoted[1 : len(quoted)-1])
			updated := resourceURL(raw)
			if updated == raw {
				return match
			}
			return []byte(string(parts[1]) + `="` + html.EscapeString(updated) + `"`)
		})
	}
	lines := strings.Split(string(body), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			lines[index] = hlsURIAttribute.ReplaceAllStringFunc(line, func(match string) string {
				parts := hlsURIAttribute.FindStringSubmatch(match)
				return `URI="` + resourceURL(parts[1]) + `"`
			})
		} else {
			lines[index] = strings.Replace(line, trimmed, resourceURL(trimmed), 1)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
