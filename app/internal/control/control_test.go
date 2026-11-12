package control

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"clashvless/internal/engine"
	"clashvless/internal/loghub"
	"clashvless/internal/store"
)

func TestFetchErrorRedactsSubscription(t *testing.T) {
	const url = "https://example.com/private-token"
	err := errors.New(`Get "` + url + `": timeout`)
	got := fetchError(err, url)
	if strings.Contains(got, url) || !strings.Contains(got, "<subscription URL>") {
		t.Fatalf("fetchError() = %q", got)
	}
}

func TestLogEventCompatibility(t *testing.T) {
	event := LogEvent(loghub.New(loghub.Warning, "route", "main", "unreachable"))
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Record == nil || decoded.Record.Level != loghub.Warning || decoded.Record.Module != "route" || decoded.Line == "" {
		t.Fatalf("decoded event = %+v", decoded)
	}
}

func TestListenAfterFailedDial(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "control.sock")
	if c, err := Dial(sock); err == nil {
		c.Close()
		t.Fatal("dial with no server succeeded")
	}
	ln, err := Listen(sock)
	if err != nil {
		t.Fatalf("listen after failed dial: %v", err)
	}
	defer ln.Close()
	c, err := Dial(sock)
	if err != nil {
		t.Fatalf("dial after listen: %v", err)
	}
	c.Close()
}

func TestDisconnectedClientsReleaseGoroutines(t *testing.T) {
	st, err := store.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sup := engine.NewSupervisor(st, nil, nil)
	sock := filepath.Join(t.TempDir(), "control.sock")
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go NewServer(sup, NewHub()).Serve(ctx, ln)

	connectOnce := func() {
		c, err := Dial(sock)
		if err != nil {
			t.Fatal(err)
		}
		for event := range c.Events() {
			if event.Type == "status" { // the server has finished its greeting
				break
			}
		}
		c.Close()
	}
	connectOnce()
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()
	for range 30 {
		connectOnce()
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before+3 {
		t.Fatalf("goroutines grew from %d to %d over 30 client connections", before, after)
	}
}

func TestApplyPatchReplacesFields(t *testing.T) {
	st := &store.State{
		Subs: []store.Subscription{
			{Name: "A", URL: "https://a", FetchErr: "timeout", Nodes: []store.Node{{Name: "a1", Raw: "vless://old@a:1#a1"}}},
			{Name: "B", URL: "https://b", Nodes: []store.Node{{Name: "b1", Outbound: json.RawMessage(`{"protocol":"vless"}`)}}},
		},
		ListenPort: 2084,
		Device:     store.Device{HWID: "0123456789", UA: "Happ"},
	}
	// The TUI and CLI send whole top-level fields; removing A sends just [B].
	removeA, _ := json.Marshal(map[string]any{"subs": st.Subs[1:]})
	if err := applyPatch(st, removeA); err != nil {
		t.Fatal(err)
	}
	if len(st.Subs) != 1 || st.Subs[0].FetchErr != "" || st.Subs[0].Nodes[0].Raw != "" || st.Subs[0].Nodes[0].Name != "b1" {
		t.Fatalf("subs after patch = %+v", st.Subs)
	}
	if st.ListenPort != 2084 || st.Device.HWID != "0123456789" {
		t.Fatal("fields the patch didn't name were changed")
	}
	if err := applyPatch(st, json.RawMessage(`{"listen_port":"x"}`)); err == nil || st.ListenPort != 2084 {
		t.Fatalf("a mistyped patch applied: err=%v port=%d", err, st.ListenPort)
	}
	if err := applyPatch(st, json.RawMessage(`{"pin_entry":"n","tun_slot":null}`)); err != nil || st.PinEntry != "n" {
		t.Fatalf("pin_entry = %q, err %v", st.PinEntry, err)
	}
}

func TestOneShotPatchIsConfirmed(t *testing.T) {
	st, err := store.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sup := engine.NewSupervisor(st, nil, nil)
	sock := filepath.Join(t.TempDir(), "control.sock")
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go NewServer(sup, NewHub()).Serve(ctx, ln)

	c, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	err = c.SendAndWait(Command{Cmd: "patch", Patch: json.RawMessage(`{"pin_entry":"node-x"}`)}, 3*time.Second)
	c.Close()
	if err != nil {
		t.Fatal(err)
	}
	var got store.State
	if err := json.Unmarshal(sup.Snapshot(), &got); err != nil || got.PinEntry != "node-x" {
		t.Fatalf("patch not applied when confirmed: %q (%v)", got.PinEntry, err)
	}
}
