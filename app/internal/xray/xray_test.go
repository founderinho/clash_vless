package xray

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/xtls/xray-core/infra/conf/serial"
)

func TestSOCKSInboundUsesConnectionUDPAddress(t *testing.T) {
	for _, listen := range []string{"", "127.0.0.1", "::1", "172.30.0.4", "0.0.0.0", "::"} {
		inbound := socksInbound("in", 2084, listen)
		settings, ok := inbound["settings"].(map[string]any)
		if !ok {
			t.Fatalf("listen %q: settings has unexpected type", listen)
		}
		if settings["udp"] != true {
			t.Errorf("listen %q: UDP is not enabled", listen)
		}
		if ip, exists := settings["ip"]; exists {
			t.Errorf("listen %q: UDP relay address is forced to %v", listen, ip)
		}
	}
}

func TestBuildConfigLeavesAllSOCKSUDPAddressesAutomatic(t *testing.T) {
	main := json.RawMessage(`{"protocol":"freedom"}`)
	entry := json.RawMessage(`{"protocol":"freedom"}`)
	cfg, err := BuildConfig(2084, main, entry, 2085, "none", "0.0.0.0")
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Inbounds []struct {
			Tag      string         `json:"tag"`
			Settings map[string]any `json:"settings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(cfg, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Inbounds) != 2 {
		t.Fatalf("got %d inbounds, want primary and entry", len(decoded.Inbounds))
	}
	for _, inbound := range decoded.Inbounds {
		if _, exists := inbound.Settings["ip"]; exists {
			t.Errorf("%s overrides the per-connection UDP relay address", inbound.Tag)
		}
	}
	if _, err := serial.LoadJSONConfig(bytes.NewReader(cfg)); err != nil {
		t.Fatalf("xray rejected config without a SOCKS UDP address override: %v", err)
	}
}

func TestRelayLeavesSOCKSUDPAddressAutomatic(t *testing.T) {
	cfg, err := BuildRelay(2086, 2084, "none")
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Inbounds []struct {
			Settings map[string]any `json:"settings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(cfg, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Inbounds) != 1 {
		t.Fatalf("got %d relay inbounds, want 1", len(decoded.Inbounds))
	}
	if _, exists := decoded.Inbounds[0].Settings["ip"]; exists {
		t.Fatal("loopback relay overrides the UDP relay address")
	}
	if _, err := serial.LoadJSONConfig(bytes.NewReader(cfg)); err != nil {
		t.Fatalf("xray rejected relay config without a SOCKS UDP address override: %v", err)
	}
}
