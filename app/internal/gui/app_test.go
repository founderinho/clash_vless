//go:build !android && cgo

package gui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"clashvless/internal/engine"
	"clashvless/internal/loghub"
	"clashvless/internal/store"
)

func TestRenderAllPages(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	w := a.NewWindow("Test")
	d := &dashboard{app: a, window: w, page: "overview", nav: map[string]*widget.Button{}, logFilter: loghub.DefaultFilter()}
	d.logs = []loghub.Record{
		loghub.New(loghub.Info, "daemon", "lifecycle", "started"),
		loghub.New(loghub.Error, "route", "main", "all tiers unreachable"),
		loghub.New(loghub.Debug, "xray", "core", "debug detail"),
	}
	d.buildShell()
	d.state = &store.State{
		Slots:  []store.Slot{{Name: "main", Port: 2084, Enabled: true, Mains: []store.Main{{Name: "Example exit", URL: "vless://uuid@example.com:443?security=reality#Example", Enabled: true, AllowNoHop: true}}}},
		Subs:   []store.Subscription{{Name: "Example provider", LastFetch: time.Now()}},
		Device: store.Device{OS: "Android", Model: "Test", UA: store.DefaultUA, Locale: "en"},
	}
	d.status = engine.Status{
		Tier: 1, Main: "Example exit", Egress: 42 * time.Millisecond,
		Slots:   []engine.SlotStatus{{Name: "main", Port: 2084, Tier: 1, Main: "Example exit"}},
		Country: []engine.Probe{{Name: "Example node", Server: "example.com", OK: true, Latency: 30 * time.Millisecond, Speed: 100}},
	}
	for _, page := range []string{"overview", "subscriptions", "routes", "activity", "settings"} {
		t.Run(page, func(t *testing.T) {
			d.navigate(page)
			if len(d.content.Objects) != 1 {
				t.Fatalf("content objects = %d", len(d.content.Objects))
			}
			if width := d.content.MinSize().Width; width > 600 {
				t.Fatalf("page requires excessive width: %.0f", width)
			}
		})
	}
	for _, section := range []string{"Network & failover", "VPN", "Subscriptions", "Device"} {
		t.Run("settings/"+section, func(t *testing.T) {
			d.settingsSection = section
			d.navigate("settings")
			if len(d.content.Objects) != 1 {
				t.Fatalf("content objects = %d", len(d.content.Objects))
			}
		})
	}
}

func TestConnectionSummary(t *testing.T) {
	cases := []struct {
		name     string
		wanted   bool
		status   engine.Status
		headline string
		good     bool
		label    string
	}{
		{"tunnel failed but a route works", true, engine.Status{Tier: 2, TunErr: "TUN: boom"}, "Not protected", false, "VPN failed"},
		{"route works, tunnel still coming up", true, engine.Status{Tier: 2}, "Connecting", false, "VPN on · Connecting"},
		{"tunnel up with a route", true, engine.Status{Tier: 2, TunUp: true}, "Connected", true, "VPN on · Connected"},
		{"tunnel up, every exit down", true, engine.Status{TunUp: true}, "No route", false, "VPN on · No route"},
		{"VPN off, proxy works", false, engine.Status{Tier: 2}, "Proxy ready", true, "VPN off"},
		{"VPN off, nothing works", false, engine.Status{}, "Disconnected", false, "VPN off"},
	}
	for _, c := range cases {
		headline, good, label := connectionSummary(c.wanted, c.status)
		if headline != c.headline || good != c.good || label != c.label {
			t.Errorf("%s: got %q %v %q", c.name, headline, good, label)
		}
	}
}
