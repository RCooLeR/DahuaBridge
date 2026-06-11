package httpserver

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"RCooLeR/DahuaBridge/internal/config"
)

const authTokenQueryParam = "auth_token"

func authMiddleware(cfg config.HTTPConfig) func(http.Handler) http.Handler {
	token := strings.TrimSpace(cfg.AuthToken)
	if token == "" {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authRouteOpen(r, cfg) || requestHasAuthToken(r, token, cfg.AuthQueryToken) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="dahuabridge"`)
			writeErrorPayload(w, http.StatusUnauthorized, "unauthorized", "bridge API token is required")
		})
	}
}

func authRouteOpen(r *http.Request, cfg config.HTTPConfig) bool {
	if r.Method == http.MethodOptions {
		return true
	}
	path := r.URL.Path
	switch path {
	case cfg.HealthPath, "/readyz", "/api/v1/status", cfg.MetricsPath:
		return true
	default:
		return strings.HasPrefix(path, "/admin/assets/")
	}
}

func requestHasAuthToken(r *http.Request, expected string, allowQuery bool) bool {
	if tokenMatches(bearerToken(r.Header.Get("Authorization")), expected) {
		return true
	}
	if tokenMatches(r.Header.Get("X-DahuaBridge-Token"), expected) {
		return true
	}
	if !allowQuery {
		return false
	}
	return tokenMatches(r.URL.Query().Get(authTokenQueryParam), expected) ||
		tokenMatches(r.URL.Query().Get("token"), expected)
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func tokenMatches(actual string, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	if actual == "" || expected == "" || len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func maxRequestBodyMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	if maxBytes <= 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
