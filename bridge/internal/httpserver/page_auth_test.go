package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os/exec"
	"testing"
	"time"
)

func TestPageBrowserAuthAndPrefixHandling(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for embedded page JavaScript tests")
	}
	script := `const assert = require('node:assert/strict');
const bridgePageAuth = { token: 'synthetic:@/token&value', queryToken: true };
global.window = { location: { href: 'https://bridge.example/proxy/admin?auth_token=stale', origin: 'https://bridge.example' } };
let lastRequest;
global.fetch = async (target, init) => { lastRequest = {target, init}; return {ok:true}; };
const attributes = {href:'/api/v1/media/preview/cam?profile=stable', src:'https://advertised.example/old-prefix/api/v1/media/snapshot/cam'};
global.document = { querySelectorAll: () => [{getAttribute: key => attributes[key], setAttribute: (key,value) => attributes[key] = value}] };
` + pageAuthScript + `
(async () => {
  let result = new URL(bridgeURL('/api/v1/media/hls/cam/stable/index.m3u8?profile=stable'));
  assert.equal(result.pathname, '/proxy/api/v1/media/hls/cam/stable/index.m3u8');
  assert.equal(result.searchParams.get('auth_token'), bridgePageAuth.token);
  assert.equal(result.searchParams.get('profile'), 'stable');
  assert.equal(bridgeURL(''), '');
  assert.equal(bridgeURL('blob:some-video'), 'blob:some-video');
  assert.equal(bridgeURL('https://external.example/video.ts'), 'https://external.example/video.ts');
  bridgePreparePage();
  for (const value of Object.values(attributes)) {
    result = new URL(value);
    assert.equal(result.origin, window.location.origin);
    assert.equal(result.searchParams.get('auth_token'), bridgePageAuth.token);
  }
  await bridgeFetch('https://advertised.example/old/api/v1/vto/door/locks/1/unlock', {method:'POST', headers:{'Content-Type':'application/json'}});
  assert.equal(new URL(lastRequest.target).pathname, '/proxy/api/v1/vto/door/locks/1/unlock');
  assert.equal(lastRequest.init.headers.get('Authorization'), 'Bearer ' + bridgePageAuth.token);
  assert.equal(lastRequest.init.headers.get('Content-Type'), 'application/json');
  await bridgeFetch('https://external.example/resource');
  assert.equal(lastRequest.init.headers.has('Authorization'), false);
  bridgePageAuth.queryToken = false;
  assert.equal(new URL(bridgeURL('/api/v1/media/snapshot/cam')).searchParams.has('auth_token'), false);
  await bridgeFetch('/api/v1/devices/recorder/probe', {method:'POST'});
  assert.equal(lastRequest.init.headers.get('Authorization'), 'Bearer ' + bridgePageAuth.token);
})().catch(error => { console.error(error); process.exitCode = 1; });
`
	if output, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("browser page authentication failed: %v\n%s", err, output)
	}
}

func TestShutdownClosesStreamingClientsAfterGracePeriod(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	server := &Server{httpServer: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(exited)
		_, _ = w.Write([]byte("stream started\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})}}
	defer server.httpServer.Close()
	go server.httpServer.Serve(listener)
	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := server.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected grace deadline, got %v", err)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("streaming HTTP handler survived forced shutdown")
	}
}
