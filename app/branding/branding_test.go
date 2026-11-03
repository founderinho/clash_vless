package branding

import (
	"encoding/json"
	"testing"
)

func TestSharedAssets(t *testing.T) {
	if logo, err := Logo(); err != nil || len(logo) == 0 {
		t.Fatalf("logo: %v", err)
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
