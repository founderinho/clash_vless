package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPlain(t *testing.T) {
	cases := []struct {
		url   string
		plain bool
	}{
		{"vless://x@45.1.1.1:444?type=tcp&security=none", true},
		{"vless://x@45.1.1.1:444?type=tcp", true}, // no security param → none
		{"vless://x@1.1.1.1:443?type=xhttp&security=reality&pbk=k", false},
		{"vless://x@1.1.1.1:443?type=tcp&security=reality&flow=xtls-rprx-vision", false},
		{"vless://x@1.1.1.1:443?type=tcp&security=tls", false},
		{"vless://x@1.1.1.1:443?flow=xtls-rprx-vision", false},
	}
	for _, c := range cases {
		if got := IsPlain(c.url); got != c.plain {
			t.Errorf("IsPlain(%q) = %v, want %v", c.url, got, c.plain)
		}
	}
}

func TestProtoTag(t *testing.T) {
	url := map[string]string{
		"vless://a@h:444?type=tcp&security=none":                          "plain",
		"vless://a@h:443?type=xhttp&security=reality":                     "reality·xhttp",
		"vless://a@h:443?type=tcp&security=reality&flow=xtls-rprx-vision": "vision",
		"vless://a@h:443?type=grpc&security=reality":                      "reality·grpc",
		"vless://a@h:443?type=ws&security=tls":                            "tls·ws",
	}
	for u, want := range url {
		if got := ProtoTag(u); got != want {
			t.Errorf("ProtoTag(%q) = %q, want %q", u, got, want)
		}
	}
	ob := map[string]string{
		`{"settings":{"vnext":[{"users":[{"flow":"xtls-rprx-vision"}]}]},"streamSettings":{"network":"tcp","security":"reality"}}`: "vision",
		`{"settings":{"vnext":[{"users":[{}]}]},"streamSettings":{"network":"grpc","security":"reality"}}`:                         "reality·grpc",
		`{"settings":{"vnext":[{"users":[{}]}]},"streamSettings":{"network":"ws","security":"tls"}}`:                               "tls·ws",
	}
	for j, want := range ob {
		if got := OutboundProtoTag([]byte(j)); got != want {
			t.Errorf("OutboundProtoTag = %q, want %q", got, want)
		}
	}
}

func TestListenHost(t *testing.T) {
	state := &State{}
	if got := state.ListenHost(); got != "127.0.0.1" {
		t.Fatalf("default ListenHost() = %q", got)
	}
	state.ListenAddr = "0.0.0.0"
	if got := state.ListenHost(); got != "0.0.0.0" {
		t.Fatalf("explicit LAN ListenHost() = %q", got)
	}
}

func TestListenAddrUpgrade(t *testing.T) {
	dir := t.TempDir()
	fresh, err := Load(filepath.Join(dir, "new.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh.ListenHost(); got != "127.0.0.1" {
		t.Fatalf("new install ListenHost() = %q", got)
	}
	old := filepath.Join(dir, "old.json")
	if err := os.WriteFile(old, []byte(`{"listen_port":2084}`), 0o600); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Load(old)
	if err != nil {
		t.Fatal(err)
	}
	if got := upgraded.ListenHost(); got != "0.0.0.0" {
		t.Fatalf("upgraded ListenHost() = %q, want the LAN reach it had before", got)
	}
}

func TestRenameSlot(t *testing.T) {
	state := &State{
		Slots: []Slot{
			{Name: "slot1", Port: 2084, Enabled: true},
			{Name: "slot2", Port: 2086, Enabled: true},
		},
		TunSlot: "slot1",
	}
	// Valid rename of slot1 to primary
	if err := state.RenameSlot(0, "primary"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Slots[0].Name != "primary" {
		t.Fatalf("slot name = %q, want 'primary'", state.Slots[0].Name)
	}
	if state.TunSlot != "primary" {
		t.Fatalf("TunSlot = %q, want 'primary'", state.TunSlot)
	}

	// Duplicate name
	if err := state.RenameSlot(0, "slot2"); err == nil {
		t.Fatal("expected error on duplicate slot name")
	}

	// Empty name
	if err := state.RenameSlot(0, "   "); err == nil {
		t.Fatal("expected error on empty slot name")
	}

	// Name containing ://
	if err := state.RenameSlot(0, "vless://invalid"); err == nil {
		t.Fatal("expected error on URL slot name")
	}
}
