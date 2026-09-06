package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
)

func TestRunBindFailureJoinsWorkWithUncanceledParent(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	yaml := fmt.Sprintf("log:\n  level: error\nhttp:\n  listen_address: %q\nmedia:\n  enabled: false\nstate_store:\n  enabled: true\n  path: %q\n  flush_interval: 1h\ndevices:\n  ipc:\n    - id: test\n      base_url: http://127.0.0.1:1\n      username: test\n      password: test\n      poll_interval: 1h\n      request_timeout: 1h\n", busy.Addr().String(), filepath.ToSlash(statePath))
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Run(parent, cfg, buildinfo.BuildInfo{}) }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("occupied HTTP port did not fail")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup failure did not join background work")
	}
	if parent.Err() != nil {
		t.Fatal("Run canceled caller-owned context")
	}
}
