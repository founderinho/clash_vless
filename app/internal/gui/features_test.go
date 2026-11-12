//go:build !android && cgo

package gui

import (
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"clashvless/internal/engine"
	"clashvless/internal/store"
)

const (
	linkA = "vless://11111111-1111-1111-1111-111111111111@a.example:443?security=reality&sni=a&pbk=k&type=tcp#A"
	linkB = "vless://22222222-2222-2222-2222-222222222222@b.example:443?security=none&type=grpc&serviceName=s#B"
	linkC = "vless://33333333-3333-3333-3333-333333333333@c.example:443?security=tls&sni=c&type=ws&path=%2F#C"
)

func iconButtons(d *dashboard, hint string) []*tooltipIconButton {
	var out []*tooltipIconButton
	for _, obj := range findRouteObjects(d.content, func(obj fyne.CanvasObject) bool {
		b, ok := obj.(*tooltipIconButton)
		return ok && b.Hint == hint
	}) {
		out = append(out, obj.(*tooltipIconButton))
	}
	return out
}

func mainNames(slot store.Slot) string {
	names := make([]string, len(slot.Mains))
	for i, m := range slot.Mains {
		names[i] = m.Name
	}
	return strings.Join(names, ",")
}

func threeExitDashboard(t *testing.T) (*dashboard, func()) {
	d, commands := routesDashboard(t)
	slot := &d.state.Slots[0]
	for _, link := range []string{linkA, linkB, linkC} {
		slot.AddMain(link)
	}
	d.selectedSlot = "first"
	d.render()
	return d, func() { applyRoutePatch(t, d, commands) }
}

func TestMoveExitChangesPriority(t *testing.T) {
	d, nextPatch := threeExitDashboard(t)
	up, down := iconButtons(d, "Raise priority"), iconButtons(d, "Lower priority")
	if len(up) != 3 || len(down) != 3 {
		t.Fatalf("move buttons: %d up, %d down", len(up), len(down))
	}
	if !up[0].Disabled || up[1].Disabled || !down[2].Disabled || down[0].Disabled {
		t.Fatal("only the first exit's raise and the last exit's lower should be disabled")
	}
	down[0].Tapped(nil)
	nextPatch()
	if got := mainNames(d.state.Slots[0]); got != "B,A,C" {
		t.Fatalf("after lowering A: %s", got)
	}
	iconButtons(d, "Raise priority")[2].Tapped(nil)
	nextPatch()
	if got := mainNames(d.state.Slots[0]); got != "B,C,A" {
		t.Fatalf("after raising C: %s", got)
	}
	iconButtons(d, "Raise priority")[0].Tapped(nil) // disabled: no patch
	if got := mainNames(d.state.Slots[0]); got != "B,C,A" {
		t.Fatalf("disabled button changed order: %s", got)
	}
}

func TestDeleteExitFollowsItAcrossConcurrentReorder(t *testing.T) {
	d, nextPatch := threeExitDashboard(t)
	iconButtons(d, "Delete exit")[1].Tapped(nil) // B
	// Another client moves B to the top while the confirmation is open.
	d.state.Slots[0].MoveMain(1, 0)
	tapRouteDialogButton(t, d, "Yes")
	nextPatch()
	if got := mainNames(d.state.Slots[0]); got != "A,C" {
		t.Fatalf("after deleting B: %s", got)
	}
}

func TestRenameExitKeepsURL(t *testing.T) {
	d, nextPatch := threeExitDashboard(t)
	iconButtons(d, "Rename exit")[2].Tapped(nil)
	entry := routeDialogEntry(t, d)
	if entry.Text != "C" {
		t.Fatalf("rename prefilled %q", entry.Text)
	}
	entry.SetText("  Cloud   exit ")
	tapRouteDialogButton(t, d, "Save")
	nextPatch()
	main := d.state.Slots[0].Mains[2]
	if main.Name != "Cloud exit" || main.URL != linkC {
		t.Fatalf("renamed main = %+v", main)
	}
	if got := mainShareLink(main).URL; !strings.HasSuffix(got, "#Cloud%20exit") || !strings.HasPrefix(got, strings.TrimSuffix(linkC, "#C")) {
		t.Fatalf("share link = %s", got)
	}
}

func TestAddSeveralExitsAtOnce(t *testing.T) {
	d, nextPatch := threeExitDashboard(t)
	d.addMain(1)
	entry := routeDialogEntry(t, d)
	entry.SetText(linkA + "\n\n  " + linkB + "  \nnot a link\n" + linkA)
	tapRouteDialogButton(t, d, "Add")
	nextPatch()
	if got := mainNames(d.state.Slots[1]); got != "A,B" {
		t.Fatalf("second slot exits = %s", got)
	}
	if !strings.Contains(d.notice.Text, "Added 2") || !strings.Contains(d.notice.Text, "1 already present") || !strings.Contains(d.notice.Text, "1 invalid") {
		t.Fatalf("notice = %q", d.notice.Text)
	}
}

func TestParseExitLinksAcceptsSubscriptionBlob(t *testing.T) {
	blob := subscriptionBlob([]shareLink{{URL: linkA}, {URL: linkB}})
	links, invalid := parseExitLinks(blob)
	if invalid != 0 || len(links) != 2 || links[0] != linkA || links[1] != linkB {
		t.Fatalf("links %v, invalid %d", links, invalid)
	}
	links, invalid = parseExitLinks(linkA + " " + linkC)
	if invalid != 0 || len(links) != 2 {
		t.Fatalf("two links on one line: %v, invalid %d", links, invalid)
	}
}

func TestExportDialogCopiesLinks(t *testing.T) {
	d, _ := threeExitDashboard(t)
	exports := findRouteObjects(d.content, func(obj fyne.CanvasObject) bool {
		b, ok := obj.(*widget.Button)
		return ok && b.Text == "Export"
	})
	if len(exports) != 1 {
		t.Fatalf("export buttons = %d", len(exports))
	}
	test.Tap(exports[0].(*widget.Button))
	if !d.dialogOpen {
		t.Fatal("export dialog did not open")
	}
	tapRouteDialogButton(t, d, "Copy all links")
	if got := d.app.Clipboard().Content(); got != strings.Join([]string{linkA, linkB, linkC}, "\n") {
		t.Fatalf("clipboard = %q", got)
	}
	tapRouteDialogButton(t, d, "Copy as subscription")
	decoded, err := base64.StdEncoding.DecodeString(d.app.Clipboard().Content())
	if err != nil || string(decoded) != strings.Join([]string{linkA, linkB, linkC}, "\n") {
		t.Fatalf("subscription blob decodes to %q (%v)", decoded, err)
	}
	tapRouteDialogButton(t, d, "Copy link")
	if got := d.app.Clipboard().Content(); got != linkA {
		t.Fatalf("first link = %q", got)
	}
	tapRouteDialogButton(t, d, "Close")
	if d.dialogOpen {
		t.Fatal("dialog still open")
	}
}

func TestExportNodesFromSubscription(t *testing.T) {
	links, skipped := nodeShareLinks([]store.Node{
		{Name: "raw", Raw: linkB},
		{Name: "json", Outbound: []byte(`{"protocol":"vless","settings":{"vnext":[{"address":"j.example","port":443,"users":[{"id":"u","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"none","sockopt":{"mark":1}}}`)},
		{Name: "trojan", Outbound: []byte(`{"protocol":"trojan"}`)},
	})
	if skipped != 1 || len(links) != 2 || links[0].URL != linkB {
		t.Fatalf("links %+v, skipped %d", links, skipped)
	}
	if !strings.HasPrefix(links[1].URL, "vless://u@j.example:443?") || !strings.Contains(links[1].Note, "sockopt") {
		t.Fatalf("converted link %+v", links[1])
	}
}

func TestQRCodeScansBackToLink(t *testing.T) {
	zbar, err := exec.LookPath("zbarimg")
	if err != nil {
		t.Skip("zbarimg not installed")
	}
	long := linkC + "&extra=" + strings.Repeat("x", 300)
	for _, link := range []string{linkA, long} {
		img, err := qrImage(link)
		if err != nil {
			t.Fatal(err)
		}
		// Scale up so the scanner sees clean modules.
		path := filepath.Join(t.TempDir(), "qr.png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, scaled(img, 6)); err != nil {
			t.Fatal(err)
		}
		f.Close()
		out, err := exec.Command(zbar, "--raw", "-q", path).Output()
		if err != nil {
			t.Fatalf("zbarimg: %v", err)
		}
		if got := strings.TrimRight(string(out), "\n"); got != link {
			t.Fatalf("scanned %q, want %q", got, link)
		}
	}
}

func TestSettingsDraftSurvivesRebuild(t *testing.T) {
	d := routeTestDashboard(t)
	d.drafts = map[string]string{}
	d.state.Interval, d.state.Timeout = 12, 6
	d.navigate("settings")
	entry := d.inputs["setting:interval_s"]
	if entry == nil {
		t.Fatal("interval field not registered")
	}
	test.Type(entry, "3") // typed at the start: "312"
	d.render()            // e.g. a state event from another client
	if got := d.inputs["setting:interval_s"].Text; got != "312" {
		t.Fatalf("unsaved interval after rebuild = %q", got)
	}
}

func TestStatusEventsOnlyRedrawWhatThePageShows(t *testing.T) {
	d := routeTestDashboard(t)
	d.navigate("routes")
	rendered := d.lastRender
	time.Sleep(5 * time.Millisecond)
	d.status.Country = []engine.Probe{{Name: "n", OK: true}}
	d.statusChanged()
	if d.lastRender != rendered || d.renderQueued {
		t.Fatal("a pool update redrew the routes page")
	}
	d.lastRender = time.Now().Add(-time.Minute)
	d.status.Slots = []engine.SlotStatus{{Name: "first", Tier: 1, Main: "A"}}
	d.statusChanged()
	if !d.lastRender.After(rendered) {
		t.Fatal("a slot status change did not redraw the routes page")
	}
}

func TestConnectHost(t *testing.T) {
	for in, want := range map[string]string{"0.0.0.0": "127.0.0.1", "::": "127.0.0.1", "127.0.0.1": "127.0.0.1", "192.168.1.5": "192.168.1.5"} {
		if got := connectHost(in); got != want {
			t.Errorf("connectHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func scaled(src image.Image, factor int) image.Image {
	b := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			dst.Set(x, y, src.At(b.Min.X+x/factor, b.Min.Y+y/factor))
		}
	}
	return dst
}

func TestFitRunes(t *testing.T) {
	measure := func(s string) float32 { return float32(len([]rune(s))) }
	cases := []struct {
		value string
		width float32
		want  string
	}{
		{"short", 10, "short"},
		{"Швеция exit", 8, "Швеция…"},
		{"ab · cdef", 6, "ab…"},
		{"abc", 0, "…"},
		{"", 3, ""},
	}
	for _, c := range cases {
		if got := fitRunes(c.value, c.width, measure); got != c.want {
			t.Errorf("fitRunes(%q, %v) = %q, want %q", c.value, c.width, got, c.want)
		}
	}
}
