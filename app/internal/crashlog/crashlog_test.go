package crashlog

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFatalCrashIsSaved(t *testing.T) {
	if path := os.Getenv("CLASHVLESS_TEST_CRASH_LOG"); path != "" {
		if _, err := Enable(path); err != nil {
			t.Fatal(err)
		}
		panic("isolated GUI crash test")
	}
	path := filepath.Join(t.TempDir(), "crash.log")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^TestFatalCrashIsSaved$")
	cmd.Env = append(os.Environ(), "CLASHVLESS_TEST_CRASH_LOG="+path)
	if err := cmd.Run(); err == nil {
		t.Fatal("child did not crash")
	} else if _, ok := err.(*exec.ExitError); !ok {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "isolated GUI crash test") || !strings.Contains(string(data), "goroutine") {
		t.Fatalf("crash report missing panic and stack: %v\n%s", err, data)
	}
}

func TestRecoveredCrashAppendsStack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")
	if err := os.WriteFile(path, []byte("previous crash\n"), 0600); err != nil {
		t.Fatal(err)
	}
	Recovered(path, "isolated recovered panic")
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), "previous crash\n") || !strings.Contains(string(data), "isolated recovered panic") || !strings.Contains(string(data), "goroutine") {
		t.Fatalf("recovered report missing or previous report overwritten: %v\n%s", err, data)
	}
}
