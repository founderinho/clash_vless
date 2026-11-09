//go:build !android && cgo

package gui

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clashvless/internal/control"
	"clashvless/internal/engine"
	"clashvless/internal/store"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func routeTestDashboard(t *testing.T) *dashboard {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)
	w := a.NewWindow("Routes test")
	w.Resize(fyne.NewSize(900, 640))
	d := &dashboard{app: a, window: w, page: "routes", nav: map[string]*widget.Button{}}
	d.buildShell()
	d.state = &store.State{ListenPort: 2084, Slots: []store.Slot{
		{Name: "first", Port: 2084, Enabled: true},
		{Name: "second", Port: 2086, Enabled: true},
		{Name: "third", Port: 2088, Enabled: true},
	}}
	d.render()
	return d
}

func routesDashboard(t *testing.T) (*dashboard, <-chan control.Command) {
	t.Helper()
	d := routeTestDashboard(t)
	sock := filepath.Join(t.TempDir(), "control.sock")
	ln, err := control.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	commands := make(chan control.Command, 16)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		decoder := json.NewDecoder(conn)
		for {
			var cmd control.Command
			if decoder.Decode(&cmd) != nil {
				return
			}
			commands <- cmd
		}
	}()
	d.client, err = control.Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.client.Close() })
	return d, commands
}

func findRouteObjects(root fyne.CanvasObject, matches func(fyne.CanvasObject) bool) []fyne.CanvasObject {
	var found []fyne.CanvasObject
	seen := map[fyne.CanvasObject]bool{}
	var walk func(fyne.CanvasObject)
	walk = func(obj fyne.CanvasObject) {
		if obj == nil || seen[obj] {
			return
		}
		seen[obj] = true
		if matches(obj) {
			found = append(found, obj)
			return
		}
		switch obj := obj.(type) {
		case *fyne.Container:
			for _, child := range obj.Objects {
				walk(child)
			}
		case fyne.Widget:
			for _, child := range test.WidgetRenderer(obj).Objects() {
				walk(child)
			}
		}
	}
	walk(root)
	return found
}

func routeDialogEntry(t *testing.T, d *dashboard) *widget.Entry {
	t.Helper()
	entries := findRouteObjects(d.window.Canvas().Overlays().Top(), func(obj fyne.CanvasObject) bool {
		_, ok := obj.(*widget.Entry)
		return ok
	})
	if len(entries) != 1 {
		t.Fatalf("dialog entries = %d", len(entries))
	}
	return entries[0].(*widget.Entry)
}

func tapRouteDialogButton(t *testing.T, d *dashboard, label string) {
	t.Helper()
	buttons := findRouteObjects(d.window.Canvas().Overlays().Top(), func(obj fyne.CanvasObject) bool {
		button, ok := obj.(*widget.Button)
		return ok && button.Text == label
	})
	if len(buttons) != 1 {
		t.Fatalf("dialog %s buttons = %d", label, len(buttons))
	}
	test.Tap(buttons[0].(*widget.Button))
}

func applyRoutePatch(t *testing.T, d *dashboard, commands <-chan control.Command) {
	t.Helper()
	select {
	case cmd := <-commands:
		if cmd.Cmd != "patch" {
			t.Fatalf("unexpected command: %s", cmd.Cmd)
		}
		if err := json.Unmarshal(cmd.Patch, d.state); err != nil {
			t.Fatal(err)
		}
		d.render()
	case <-time.After(2 * time.Second):
		t.Fatal("no patch received")
	}
}

func TestAddExitToLastThenFirstSlot(t *testing.T) {
	d, commands := routesDashboard(t)
	for _, index := range []int{2, 0, 2, 0} {
		d.addMain(index)
		entry := routeDialogEntry(t, d)
		d.window.Canvas().Focus(entry)
		test.Type(entry, "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=none#Exit")
		content := d.content.Objects[0]
		for update := 0; update < 3; update++ {
			d.render()
		}
		if d.content.Objects[0] != content {
			t.Fatal("status refresh replaced the page beneath the edit dialog")
		}
		tapRouteDialogButton(t, d, "Add")
		if d.dialogOpen || d.window.Canvas().Overlays().Top() != nil {
			t.Fatal("edit dialog was not released after saving")
		}
		applyRoutePatch(t, d, commands)
	}
	if len(d.state.Slots[0].Mains) == 0 || len(d.state.Slots[2].Mains) == 0 {
		t.Fatal("exits were not saved to both slots")
	}
}

func TestAddExitKeepsSlotIdentityDuringUpdates(t *testing.T) {
	d, commands := routesDashboard(t)
	d.addMain(2)
	routeDialogEntry(t, d).SetText("vless://uuid@example.com:443#Exit")
	d.state.Slots[0], d.state.Slots[2] = d.state.Slots[2], d.state.Slots[0]
	d.render()
	tapRouteDialogButton(t, d, "Add")
	applyRoutePatch(t, d, commands)
	if len(d.state.Slots[0].Mains) != 1 || len(d.state.Slots[2].Mains) != 0 {
		t.Fatal("exit was saved to a different slot after the state update")
	}
}

func TestAddExitToRemovedSlot(t *testing.T) {
	d, commands := routesDashboard(t)
	d.addMain(2)
	routeDialogEntry(t, d).SetText("vless://uuid@example.com:443#Exit")
	d.state.Slots = d.state.Slots[:2]
	d.render()
	tapRouteDialogButton(t, d, "Add")
	if d.dialogOpen || d.notice.Text != "This slot was removed while editing" {
		t.Fatal("removed slot was not reported safely")
	}
	select {
	case <-commands:
		t.Fatal("patch sent for a removed slot")
	default:
	}
	d.addMain(2)
	if d.window.Canvas().Overlays().Top() != nil {
		t.Fatal("dialog opened for a stale slot")
	}
}

func TestCancelExitDialogRefreshesLatestState(t *testing.T) {
	d, _ := routesDashboard(t)
	d.addMain(0)
	content := d.content.Objects[0]
	d.state.Slots[0].Enabled = false
	d.render()
	d.addMain(2)
	if len(d.window.Canvas().Overlays().List()) != 1 {
		t.Fatal("a second edit dialog opened over the first")
	}
	tapRouteDialogButton(t, d, "Cancel")
	if d.dialogOpen || d.content.Objects[0] == content {
		t.Fatal("latest state was not rendered after canceling")
	}
	d.addMain(2)
	if d.window.Canvas().Overlays().Top() == nil {
		t.Fatal("next slot dialog could not open")
	}
	tapRouteDialogButton(t, d, "Cancel")
}

func TestRoutesKeepWindowWidthWithLongDetails(t *testing.T) {
	longName := strings.Repeat("vless://uuid@example.com:443?security=reality#Выход ", 12)
	longHost := strings.Repeat("segment.", 25) + "example.com"
	for _, tc := range []struct {
		name, slot, exit, host string
	}{
		{"exit name", "first", longName, "example.com"},
		{"server address", "first", "Example exit", longHost},
		{"slot name", longName, "Example exit", "example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := routeTestDashboard(t)
			d.state.Slots[0].Name = tc.slot
			d.state.Slots[0].Mains = []store.Main{{Name: tc.exit, URL: "vless://uuid@" + tc.host + ":443?security=reality", Enabled: true}}
			for _, connected := range []bool{false, true, false, true} {
				d.status = engine.Status{}
				if connected {
					d.status = engine.Status{Tier: 1, TunUp: true, Slots: []engine.SlotStatus{{Name: tc.slot, Tier: 1, Main: tc.exit, Egress: 30 * time.Millisecond}}}
				}
				d.render()
				if width := d.window.Content().MinSize().Width; width > 760 {
					t.Fatalf("connected=%t: route text forces window width %.0f", connected, width)
				}
				d.window.Resize(fyne.NewSize(760, 640))
				if width := d.window.Canvas().Size().Width; width > 760 {
					t.Fatalf("connected=%t: cannot shrink window below %.0f", connected, width)
				}
			}
		})
	}
}
