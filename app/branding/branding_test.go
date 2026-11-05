package branding

import (
	"bytes"
	"encoding/json"
	"image/png"
	"strings"
	"testing"
)

func TestSharedAssets(t *testing.T) {
	if logo, err := Logo(); err != nil || len(logo) == 0 {
		t.Fatalf("logo: %v", err)
	} else if strings.Contains(string(logo), "<rect") {
		t.Fatal("logo must not have a rectangular backdrop")
	}
	raster, err := PNG("logo")
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(raster))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, alpha := image.At(0, 0).RGBA(); alpha != 0 {
		t.Fatal("launcher logo must have transparent corners")
	}
	windowsIcon, err := Files.ReadFile("windows/clashvless.ico")
	if err != nil || len(windowsIcon) < 6 || windowsIcon[4] != 7 {
		t.Fatalf("multi-resolution Windows icon: %v", err)
	}
	for _, name := range []string{"overview", "subscriptions", "routes", "activity", "settings", "refresh", "copy", "add", "delete"} {
		if icon, err := Icon(name); err != nil || len(icon) == 0 {
			t.Errorf("SVG icon %s: %v", name, err)
		}
		if icon, err := PNG(name); err != nil || len(icon) == 0 {
			t.Errorf("PNG icon %s: %v", name, err)
		}
	}
	tokens, err := Tokens()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(tokens, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["colors"] == nil {
		t.Fatal("color tokens are missing")
	}
}
