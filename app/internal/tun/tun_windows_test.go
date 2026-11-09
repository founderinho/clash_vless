package tun

import (
	"strings"
	"testing"
)

func TestRunErrorIsReadable(t *testing.T) {
	err := (&Manager{}).run("netsh", "interface", "ipv6", "delete", "route", "2000::/3", "interface=no-such-adapter")
	if err == nil {
		t.Skip("netsh unexpectedly succeeded")
	}
	if strings.ContainsRune(err.Error(), '�') {
		t.Fatalf("garbled output: %q", err)
	}
	t.Log(err)
}
