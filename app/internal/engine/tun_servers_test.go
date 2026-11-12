package engine

import (
	"reflect"
	"testing"

	"clashvless/internal/store"
)

func TestTunServerHostsAreSortedAndUnique(t *testing.T) {
	st := &store.State{
		Slots: []store.Slot{
			{Name: "a", Mains: []store.Main{{URL: "vless://u@b.example:443"}, {URL: "vless://u@10.0.0.1:443"}}},
			{Name: "b", Mains: []store.Main{{URL: "vless://u@b.example:8443"}, {URL: "not a url\x7f"}}},
		},
		Subs:       []store.Subscription{{Nodes: []store.Node{{Server: "a.example"}, {Server: "10.0.0.1"}, {Server: ""}}}},
		AutoSelect: true,
	}
	s := &Supervisor{st: st}
	if got, want := s.tunServerHosts(), []string{"10.0.0.1", "a.example", "b.example"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tunServerHosts() = %v, want %v", got, want)
	}
}

func TestResolveServersKeepsPreviousAddressOnFailure(t *testing.T) {
	s := &Supervisor{}
	ips, hosts := s.resolveServers([]string{"10.0.0.1", "unresolvable.invalid"}, map[string]string{"unresolvable.invalid": "192.0.2.7"})
	if len(ips) != 2 || ips[0].String() != "10.0.0.1" || ips[1].String() != "192.0.2.7" {
		t.Fatalf("ips = %v", ips)
	}
	if hosts["unresolvable.invalid"] != "192.0.2.7" {
		t.Fatalf("hosts = %v", hosts)
	}
}
