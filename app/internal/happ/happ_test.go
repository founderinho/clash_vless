package happ

import "testing"

func TestParseURIDecodesNameOnce(t *testing.T) {
	for fragment, want := range map[string]string{
		"DE%20100%25":                       "DE 100%",
		"A+B":                               "A+B",
		"%F0%9F%87%A9%F0%9F%87%AA%20Berlin": "🇩🇪 Berlin",
	} {
		node, ok := parseURI("vless://u@example.com:443?security=none#" + fragment)
		if !ok || node.Name != want {
			t.Errorf("fragment %q: name %q, want %q", fragment, node.Name, want)
		}
	}
}
