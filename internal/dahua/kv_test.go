package dahua

import "testing"

func TestParseKeyValueBody(t *testing.T) {
	body := "serialNumber=ABC123\nprocessor=ST7108\n\ninvalid-line\nname=West Gate\n"

	got := ParseKeyValueBody(body)

	if got["serialNumber"] != "ABC123" {
		t.Fatalf("serialNumber mismatch: got %q", got["serialNumber"])
	}

	if got["processor"] != "ST7108" {
		t.Fatalf("processor mismatch: got %q", got["processor"])
	}

	if got["name"] != "West Gate" {
		t.Fatalf("name mismatch: got %q", got["name"])
	}
}
