package xray

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// OutboundToVLESS turns a vless outbound back into a share link named name. It
// returns the outbound settings a link cannot carry, so callers can say the
// exported link differs from the node.
func OutboundToVLESS(raw json.RawMessage, name string) (link string, dropped []string, err error) {
	var ob struct {
		Protocol string          `json:"protocol"`
		Settings json.RawMessage `json:"settings"`
		Stream   map[string]any  `json:"streamSettings"`
		Mux      map[string]any  `json:"mux"`
	}
	if err := json.Unmarshal(raw, &ob); err != nil {
		return "", nil, fmt.Errorf("parse outbound: %w", err)
	}
	if ob.Protocol != "vless" {
		return "", nil, fmt.Errorf("%s nodes cannot be exported as vless:// links", def(ob.Protocol, "unknown"))
	}
	server, err := vlessServer(ob.Settings)
	if err != nil {
		return "", nil, err
	}
	if server.Address == "" || server.Port == 0 || server.ID == "" {
		return "", nil, fmt.Errorf("outbound is missing address, port or id")
	}

	q := url.Values{}
	q.Set("encryption", def(server.Encryption, "none"))
	if server.Flow != "" {
		q.Set("flow", server.Flow)
	}

	stream := ob.Stream
	network := def(str(stream["network"]), "tcp")
	if network == "raw" {
		network = "tcp"
	}
	security := def(str(stream["security"]), "none")
	q.Set("type", network)
	q.Set("security", security)

	switch security {
	case "reality":
		rs := object(stream["realitySettings"])
		setQuery(q, "sni", rs["serverName"])
		setQuery(q, "pbk", rs["publicKey"])
		if q.Get("pbk") == "" {
			setQuery(q, "pbk", rs["password"])
		}
		setQuery(q, "sid", rs["shortId"])
		setQuery(q, "fp", rs["fingerprint"])
		setQuery(q, "spx", rs["spiderX"])
		dropped = append(dropped, extraKeys("realitySettings", rs, "serverName", "publicKey", "password", "shortId", "fingerprint", "spiderX")...)
	case "tls":
		ts := object(stream["tlsSettings"])
		setQuery(q, "sni", ts["serverName"])
		setQuery(q, "fp", ts["fingerprint"])
		if alpn := stringList(ts["alpn"]); len(alpn) > 0 {
			q.Set("alpn", strings.Join(alpn, ","))
		}
		if insecure, _ := ts["allowInsecure"].(bool); insecure {
			q.Set("allowInsecure", "1")
		}
		dropped = append(dropped, extraKeys("tlsSettings", ts, "serverName", "fingerprint", "alpn", "allowInsecure")...)
	case "none":
	default:
		return "", nil, fmt.Errorf("security %q cannot be exported", security)
	}

	switch network {
	case "tcp":
		for _, key := range []string{"tcpSettings", "rawSettings"} {
			ts := object(stream[key])
			if header := object(ts["header"]); len(header) > 0 && def(str(header["type"]), "none") != "none" {
				dropped = append(dropped, key+".header")
			}
			dropped = append(dropped, extraKeys(key, ts, "header")...)
		}
	case "ws":
		ws := object(stream["wsSettings"])
		setQuery(q, "path", ws["path"])
		setQuery(q, "host", ws["host"])
		if q.Get("host") == "" {
			setQuery(q, "host", object(ws["headers"])["Host"])
		}
		dropped = append(dropped, extraKeys("wsSettings", ws, "path", "host", "headers")...)
	case "grpc":
		g := object(stream["grpcSettings"])
		setQuery(q, "serviceName", g["serviceName"])
		setQuery(q, "authority", g["authority"])
		if multi, _ := g["multiMode"].(bool); multi {
			q.Set("mode", "multi")
		}
		dropped = append(dropped, extraKeys("grpcSettings", g, "serviceName", "authority", "multiMode")...)
	case "xhttp", "splithttp":
		key := network + "Settings"
		xh := object(stream[key])
		setQuery(q, "path", xh["path"])
		setQuery(q, "host", xh["host"])
		setQuery(q, "mode", xh["mode"])
		if extra, ok := xh["extra"]; ok && extra != nil {
			data, err := json.Marshal(extra)
			if err == nil {
				q.Set("extra", string(data))
			}
		}
		dropped = append(dropped, extraKeys(key, xh, "path", "host", "mode", "extra")...)
	default:
		return "", nil, fmt.Errorf("transport %q cannot be exported", network)
	}

	for key, value := range stream {
		// *Settings blocks were checked above, or are inactive and ignored by xray too.
		if key != "network" && key != "security" && !strings.HasSuffix(key, "Settings") && !empty(value) {
			dropped = append(dropped, key)
		}
	}
	if enabled, _ := ob.Mux["enabled"].(bool); enabled {
		dropped = append(dropped, "mux")
	}
	sort.Strings(dropped)

	u := url.URL{
		Scheme:   "vless",
		User:     url.User(server.ID),
		Host:     net.JoinHostPort(server.Address, strconv.Itoa(server.Port)),
		RawQuery: q.Encode(),
		Fragment: strings.TrimSpace(name),
	}
	return u.String(), dropped, nil
}

type vlessTarget struct {
	Address    string
	Port       int
	ID         string
	Flow       string
	Encryption string
}

// vlessServer reads both the vnext form and xray's flat settings form.
func vlessServer(raw json.RawMessage) (vlessTarget, error) {
	type user struct {
		ID         string `json:"id"`
		Flow       string `json:"flow"`
		Encryption string `json:"encryption"`
	}
	var settings struct {
		Vnext []struct {
			Address string `json:"address"`
			Port    int    `json:"port"`
			Users   []user `json:"users"`
		} `json:"vnext"`
		Address string `json:"address"`
		Port    int    `json:"port"`
		user
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return vlessTarget{}, fmt.Errorf("parse vless settings: %w", err)
	}
	if len(settings.Vnext) > 0 {
		server := settings.Vnext[0]
		if len(server.Users) == 0 {
			return vlessTarget{}, fmt.Errorf("outbound has no user")
		}
		u := server.Users[0]
		return vlessTarget{server.Address, server.Port, u.ID, u.Flow, u.Encryption}, nil
	}
	return vlessTarget{settings.Address, settings.Port, settings.ID, settings.Flow, settings.Encryption}, nil
}

func str(value any) string {
	s, _ := value.(string)
	return s
}

func object(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}

func setQuery(q url.Values, key string, value any) {
	if s := str(value); s != "" {
		q.Set(key, s)
	}
}

func stringList(value any) []string {
	switch v := value.(type) {
	case string:
		if v != "" {
			return strings.Split(v, ",")
		}
	case []any:
		var out []string
		for _, item := range v {
			if s := str(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func extraKeys(prefix string, m map[string]any, known ...string) []string {
	var out []string
	for key, value := range m {
		isKnown := false
		for _, k := range known {
			if key == k {
				isKnown = true
				break
			}
		}
		if !isKnown && !empty(value) {
			out = append(out, prefix+"."+key)
		}
	}
	return out
}

func empty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case bool:
		return !v
	case float64:
		return v == 0
	case map[string]any:
		return len(v) == 0
	case []any:
		return len(v) == 0
	}
	return false
}
