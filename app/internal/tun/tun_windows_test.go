package tun

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCommandHasNoConsoleWindow(t *testing.T) {
	cmd := command("route", "print", "-4")
	if cmd.SysProcAttr == nil {
		t.Fatal("subprocess has no Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("subprocess window is not hidden")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("subprocess can create a console window")
	}
}

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
