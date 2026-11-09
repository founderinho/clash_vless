package tun

import (
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
)

func TestBundledWintunArchitectures(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(wintunArchive)); got != "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51" {
		t.Fatalf("upstream archive changed: %s", got)
	}
	for arch, machine := range map[string]uint16{
		"amd64": pe.IMAGE_FILE_MACHINE_AMD64, "386": pe.IMAGE_FILE_MACHINE_I386,
		"arm64": pe.IMAGE_FILE_MACHINE_ARM64, "arm": pe.IMAGE_FILE_MACHINE_ARMNT,
	} {
		t.Run(arch, func(t *testing.T) {
			dir := t.TempDir()
			if err := installWintun(dir, arch, nil); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "wintun.dll"))
			if err != nil {
				t.Fatal(err)
			}
			dll, err := pe.NewFile(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			defer dll.Close()
			if dll.Machine != machine || dll.Characteristics&pe.IMAGE_FILE_DLL == 0 {
				t.Fatalf("incorrect DLL architecture: %+v", dll.FileHeader)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Fatalf("extraction left temporary files: %v", entries)
			}
		})
	}
}

func TestExistingWintunPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wintun.dll")
	want := []byte("existing DLL")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	if err := installWintun(dir, runtime.GOARCH, func(string, ...any) { t.Error("reinstalled existing DLL") }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("existing DLL was changed: %q, %v", got, err)
	}
}

func TestWintunExtractionErrors(t *testing.T) {
	dir := t.TempDir()
	if err := installWintun(dir, "unsupported", nil); err == nil {
		t.Fatal("unsupported architecture accepted")
	}
	if err := installWintun(filepath.Join(dir, "missing"), runtime.GOARCH, nil); err == nil {
		t.Fatal("missing destination accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "wintun.dll"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := installWintun(dir, runtime.GOARCH, nil); err == nil {
		t.Fatal("directory accepted as existing DLL")
	}
}

func TestExtractedWintunLoads(t *testing.T) {
	dir := t.TempDir()
	if err := installWintun(dir, runtime.GOARCH, nil); err != nil {
		t.Fatal(err)
	}
	// Resolve an export without creating an adapter or changing host networking.
	handle, err := windows.LoadLibraryEx(filepath.Join(dir, "wintun.dll"), 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.FreeLibrary(handle)
	if _, err := windows.GetProcAddress(handle, "WintunCreateAdapter"); err != nil {
		t.Fatal(err)
	}
}

func TestStandaloneWintun(t *testing.T) {
	if os.Getenv("CLASHVLESS_WINTUN_TEST_CHILD") == "1" {
		if err := Prepare(t.Logf); err != nil {
			t.Fatal(err)
		}
		if version := wintun.Version(); version == "unknown" {
			t.Fatal("xray's Wintun dependency could not load the extracted DLL")
		} else {
			t.Logf("Wintun version: %s", version)
		}
		return
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	standalone := filepath.Join(dir, "standalone.exe")
	if err := os.WriteFile(standalone, data, 0700); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(standalone, "-test.run=^TestStandaloneWintun$", "-test.v")
	child.Env = append(os.Environ(), "CLASHVLESS_WINTUN_TEST_CHILD=1")
	child.Dir = t.TempDir()
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("standalone startup: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
	if _, err := os.Stat(filepath.Join(dir, "wintun.dll")); err != nil {
		t.Fatal("DLL was not extracted beside the standalone executable:", err)
	}
}
