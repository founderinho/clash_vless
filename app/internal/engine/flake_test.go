package engine

import (
	"sort"
	"testing"
	"time"
)

func TestMeanSpeed(t *testing.T) {
	s := &Supervisor{}
	if mbps, n := s.meanSpeed("x"); mbps != 0 || n != 0 {
		t.Fatalf("meanSpeed(unknown) = %g over %d, want 0 over 0", mbps, n)
	}
	s.recordSpeed("x", 40)
	s.recordSpeed("x", 60)
	if mbps, n := s.meanSpeed("x"); mbps != 50 || n != 2 {
		t.Fatalf("meanSpeed(x) = %g over %d, want 50 over 2", mbps, n)
	}
}

func TestFlakinessAndStabilityBucket(t *testing.T) {
	s := &Supervisor{}

	// 7 pass + 3 fail within the window => 30% flaky over 10 samples.
	for i := 0; i < 7; i++ {
		s.recordProbe("n", true)
	}
	for i := 0; i < 3; i++ {
		s.recordProbe("n", false)
	}
	if pct, n := s.flakiness("n"); pct != 30 || n != 10 {
		t.Fatalf("flakiness(n) = %g%% over %d, want 30%% over 10", pct, n)
	}
	if b := s.stabilityBucket("n"); b != 3 { // 30 / 10
		t.Fatalf("stabilityBucket(n) = %d, want 3", b)
	}

	// Under-sampled (1 sample, all-fail) must NOT sink to the worst band — it lands
	// mid-pack via unknownFlakePct so a proven-steady node beats it but it beats a
	// proven-flaky one.
	s.recordProbe("m", false)
	if b := s.stabilityBucket("m"); b != 2 { // unknownFlakePct(25) / 10
		t.Fatalf("stabilityBucket(m, under-sampled) = %d, want 2", b)
	}

	// A node with no history at all is also mid-pack, not band 0.
	if pct, n := s.flakiness("none"); pct != 0 || n != 0 {
		t.Fatalf("flakiness(none) = %g%% over %d, want 0%% over 0", pct, n)
	}
	if b := s.stabilityBucket("none"); b != 2 {
		t.Fatalf("stabilityBucket(none) = %d, want 2", b)
	}

	// A steady node (0% over enough samples) sits in band 0, ahead of everyone.
	for i := 0; i < minStabSamples; i++ {
		s.recordProbe("solid", true)
	}
	if b := s.stabilityBucket("solid"); b != 0 {
		t.Fatalf("stabilityBucket(solid) = %d, want 0", b)
	}
}

func TestRankKeyOrderIsTransitive(t *testing.T) {
	a := rankKey{ok: true, measured: true, speed: 10, lat: 50 * time.Millisecond}
	b := rankKey{ok: true, lat: 100 * time.Millisecond}
	c := rankKey{ok: true, measured: true, speed: 20, lat: 150 * time.Millisecond}
	keys := []rankKey{a, b, c}
	for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		items := []rankKey{keys[order[0]], keys[order[1]], keys[order[2]]}
		sort.SliceStable(items, func(i, j int) bool { return items[i].less(items[j]) })
		if items[0] != c || items[1] != a || items[2] != b {
			t.Fatalf("input order %v sorted to %+v", order, items)
		}
	}
	if !(rankKey{ok: true, bucket: 0, lat: time.Second}).less(rankKey{ok: true, bucket: 1, measured: true, speed: 99}) {
		t.Error("a steadier band must win over speed")
	}
	if (rankKey{ok: false, measured: true, speed: 99}).less(rankKey{ok: true}) {
		t.Error("an unreachable node must rank last")
	}
}
