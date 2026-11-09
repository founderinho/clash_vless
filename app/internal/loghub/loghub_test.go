package loghub

import (
	"testing"
	"time"
)

func TestRecordAndCompatibility(t *testing.T) {
	r := New(Warning, "route", "main", "exit %s is down", "edge")
	r.Time = time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.Local)
	if got, want := r.String(), "03:04:05.006 WARN  route/main exit edge is down"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	parsed := ParseLine(r.String())
	if parsed.Level != Warning || parsed.Module != "route" || parsed.Element != "main" || parsed.Message != r.Message {
		t.Fatalf("ParseLine() = %+v", parsed)
	}
	legacy := ParseLine("route recovered")
	if legacy.Level != Info || legacy.Message != "route recovered" {
		t.Fatalf("legacy ParseLine() = %+v", legacy)
	}
}

func TestFilter(t *testing.T) {
	r := New(Error, "xray", "core", "Connection failed for Edge")
	filter := DefaultFilter()
	if !filter.Allows(r) {
		t.Fatal("default filter hid an error")
	}
	filter.Query = "XRAY & failed | impossible"
	if !filter.Allows(r) {
		t.Fatal("AND/OR query did not match")
	}
	filter.Query = "xray & missing | impossible"
	if filter.Allows(r) {
		t.Fatal("query matched a missing term")
	}
	filter.Query = ""
	filter.Module = "route"
	if filter.Allows(r) {
		t.Fatal("module filter matched another module")
	}
	filter.Module = ""
	filter.Levels[Error] = false
	if filter.Allows(r) {
		t.Fatal("disabled level matched")
	}
}

func TestParseLineLevels(t *testing.T) {
	for _, level := range []Level{Info, Warning, Error, Debug} {
		line := New(level, "route", "main", "unreachable now").String()
		parsed := ParseLine(line)
		if parsed.Level != level || parsed.Module != "route" || parsed.Element != "main" || parsed.Message != "unreachable now" {
			t.Errorf("ParseLine(%q) = %+v", line, parsed)
		}
	}
}
