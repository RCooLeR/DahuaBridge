package ipc

import "testing"

func TestNormalizeRPCMap(t *testing.T) {
	got := normalizeRPCMap(map[string]any{
		"info": map[string]any{
			"deviceType":   "DH-IPC-HFW1430DS1-SAW",
			"serialNumber": "AB06FCEPAGF0AB2",
		},
	}, "info")

	if stringFromAny(got["deviceType"]) != "DH-IPC-HFW1430DS1-SAW" {
		t.Fatalf("unexpected deviceType: %#v", got)
	}
	if stringFromAny(got["serialNumber"]) != "AB06FCEPAGF0AB2" {
		t.Fatalf("unexpected serialNumber: %#v", got)
	}
}

func TestParseSoftwareInfoNestedVersionMap(t *testing.T) {
	version, build := parseSoftwareInfo(normalizeRPCMap(map[string]any{
		"version": map[string]any{
			"Version":   "2.800.0000000.21.R",
			"BuildDate": "2024-03-11",
		},
	}, "version"))

	if version != "2.800.0000000.21.R" {
		t.Fatalf("unexpected version: %q", version)
	}
	if build != "2024-03-11" {
		t.Fatalf("unexpected build date: %q", build)
	}
}

func TestFirstStringInMap(t *testing.T) {
	value := firstStringInMap(map[string]any{
		"version": map[string]any{
			"Version": "V1.0",
		},
	})

	if value != "V1.0" {
		t.Fatalf("unexpected first string: %q", value)
	}
}
