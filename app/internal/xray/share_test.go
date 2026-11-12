package xray

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestOutboundToVLESSRoundTrip(t *testing.T) {
	links := []string{
		"vless://11111111-2222-3333-4444-555555555555@example.com:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.amazon.com&fp=chrome&pbk=PUBKEY&sid=c0e4&spx=%2F&type=tcp#Vision%20exit",
		`vless://11111111-2222-3333-4444-555555555555@166.1.201.22:443?encryption=none&type=xhttp&security=reality&sni=www.amazon.com&fp=chrome&pbk=PUB&sid=c0&spx=/etT8&path=/&mode=auto&extra=%7B%22xPaddingBytes%22:%22100-1000%22%7D#xhttp`,
		"vless://11111111-2222-3333-4444-555555555555@[2001:db8::1]:8443?type=ws&security=tls&sni=cdn.example&alpn=h2,http/1.1&path=/ws&host=cdn.example&allowInsecure=1#ws",
		"vless://11111111-2222-3333-4444-555555555555@10.0.0.1:444?type=grpc&security=none&serviceName=svc#plain%20grpc",
		"vless://11111111-2222-3333-4444-555555555555@10.0.0.1:444#bare",
	}
	for _, link := range links {
		want, err := VlessToOutbound(link, "main")
		if err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		exported, dropped, err := OutboundToVLESS(want, "name")
		if err != nil {
			t.Fatalf("%s: export: %v", link, err)
		}
		if len(dropped) != 0 {
			t.Errorf("%s: unexpected dropped %v", link, dropped)
		}
		got, err := VlessToOutbound(exported, "main")
		if err != nil {
			t.Fatalf("%s: reparse %s: %v", link, exported, err)
		}
		var a, b any
		_ = json.Unmarshal(want, &a)
		_ = json.Unmarshal(got, &b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("round trip changed outbound\n link %s\n out  %s\n want %s\n got  %s", link, exported, want, got)
		}
		if !strings.HasSuffix(exported, "#name") {
			t.Errorf("name fragment missing: %s", exported)
		}
	}
}

func TestOutboundToVLESSSubscriptionNode(t *testing.T) {
	node := json.RawMessage(`{"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"auto.example","port":443,"users":[{"id":"31c660a7-eaba-4578-b237-1132a34bde31","encryption":"none","flow":"xtls-rprx-vision"}]}]},"streamSettings":{"network":"tcp","tcpSettings":{},"security":"reality","realitySettings":{"serverName":"en2.example.com","publicKey":"PUB","shortId":"8718","fingerprint":"firefox","show":false},"finalmask":{"tcp":[{"type":"fragment"}]}},"mux":{"enabled":false}}`)
	link, dropped, err := OutboundToVLESS(node, "🚀 Авто выбор")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dropped, []string{"finalmask"}) {
		t.Errorf("dropped = %v, want [finalmask]", dropped)
	}
	ob, err := VlessToOutbound(link, "proxy")
	if err != nil {
		t.Fatalf("reparse %s: %v", link, err)
	}
	for _, want := range []string{`"address":"auto.example"`, `"flow":"xtls-rprx-vision"`, `"publicKey":"PUB"`, `"fingerprint":"firefox"`} {
		if !strings.Contains(string(ob), want) {
			t.Errorf("%s missing from %s", want, ob)
		}
	}
	if !strings.Contains(link, "#%F0%9F%9A%80") {
		t.Errorf("name not escaped into fragment: %s", link)
	}
}

func TestOutboundToVLESSFlatSettings(t *testing.T) {
	node := json.RawMessage(`{"protocol":"vless","settings":{"address":"h.example","port":8443,"id":"u","encryption":"none"},"streamSettings":{"network":"raw","security":"none"}}`)
	link, _, err := OutboundToVLESS(node, "")
	if err != nil {
		t.Fatal(err)
	}
	if link != "vless://u@h.example:8443?encryption=none&security=none&type=tcp" {
		t.Errorf("link = %s", link)
	}
}

func TestOutboundToVLESSRejectsOtherProtocols(t *testing.T) {
	if _, _, err := OutboundToVLESS(json.RawMessage(`{"protocol":"trojan","settings":{}}`), "x"); err == nil {
		t.Error("trojan exported as vless")
	}
	if _, _, err := OutboundToVLESS(json.RawMessage(`{"protocol":"vless","settings":{"vnext":[{"address":"a","port":1,"users":[]}]}}`), "x"); err == nil {
		t.Error("user-less outbound exported")
	}
}
