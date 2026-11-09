package control

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"clashvless/internal/loghub"
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
