package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"RCooLeR/DahuaBridge/internal/config"
)

// writeHTMLPage supplies one same-bridge URL/auth helper to all embedded pages.
// Page navigation and native media elements cannot attach a Bearer header.
func writeHTMLPage(w http.ResponseWriter, cfg config.HTTPConfig, body string) {
	settings, _ := json.Marshal(struct {
		Token      string `json:"token"`
		QueryToken bool   `json:"queryToken"`
	}{Token: strings.TrimSpace(cfg.AuthToken), QueryToken: cfg.AuthQueryToken})
	bootstrap := "<script>\nconst bridgePageAuth = " + string(settings) + ";\n" + pageAuthScript + "\n</script>\n"
	body = strings.Replace(body, "</head>", bootstrap+"</head>", 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

const pageAuthScript = `
function bridgeURL(value, includeToken = true) {
  if (!value) return value;
  let target;
  try { target = new URL(value, window.location.href); } catch (_) { return value; }
  if (!['http:', 'https:'].includes(target.protocol)) return value;
  // Bridge-generated links may advertise a different host or proxy prefix.
  // Route known bridge paths through the address serving this page.
  const page = new URL(window.location.href);
  const marker = page.pathname.match(/^(.*?)(?:\/api\/v1\/|\/admin(?:\/|$))/);
  const prefix = marker ? marker[1] : '';
  const apiIndex = target.pathname.lastIndexOf('/api/v1/');
  const adminIndex = target.pathname.lastIndexOf('/admin');
  let path;
  if (apiIndex >= 0) path = target.pathname.slice(apiIndex);
  else if (adminIndex >= 0 && /^\/admin(?:\/|$)/.test(target.pathname.slice(adminIndex))) path = target.pathname.slice(adminIndex);
  else return value;
  target = new URL(prefix + path + target.search + target.hash, page.origin);
  if (includeToken && bridgePageAuth.queryToken && bridgePageAuth.token) {
    target.searchParams.delete('token');
    target.searchParams.set('auth_token', bridgePageAuth.token);
  }
  return target.toString();
}

function bridgeFetch(value, init = {}) {
  const target = bridgeURL(value, false);
  const request = new URL(target, window.location.href);
  const headers = new Headers(init.headers || {});
  if (request.origin === window.location.origin && /\/api\/v1\//.test(request.pathname) && bridgePageAuth.token) {
    headers.set('Authorization', 'Bearer ' + bridgePageAuth.token);
  }
  return fetch(target, { ...init, headers });
}

function bridgePreparePage() {
  for (const element of document.querySelectorAll('[href], [src]')) {
    for (const attr of ['href', 'src']) {
      const original = element.getAttribute(attr);
      if (!original) continue;
      const updated = bridgeURL(original);
      if (updated !== original) element.setAttribute(attr, updated);
    }
  }
}
`
