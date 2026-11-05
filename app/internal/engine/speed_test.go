package engine

import "testing"

// With the speedtest switched off, every node must read as unmeasured so the
// pool ranking falls back to stability + ping — even nodes a misbehaving sweep
// already scored 0 (the escape hatch must not leave them sunk for the hour).
func TestMeanSpeedIgnoredWhenSpeedtestOff(t *testing.T) {
	s := &Supervisor{speeds: map[string][]speedSample{}}
	s.recordSpeed("fast", 50)
	s.recordSpeed("dead", 0)
	if v, n := s.meanSpeed("dead"); n != 1 || v != 0 {
		t.Fatalf("on: dead = %v·%d, want 0·1 (measured zero)", v, n)
	}
	if v, n := s.meanSpeed("fast"); n != 1 || v != 50 {
		t.Fatalf("on: fast = %v·%d, want 50·1", v, n)
	}
	s.speedsReady = true
	s.speedOff.Store(true)
	for _, name := range []string{"fast", "dead"} {
		if v, n := s.meanSpeed(name); n != 0 || v != 0 {
			t.Fatalf("off: %s = %v·%d, want unmeasured 0·0", name, v, n)
		}
	}
	if s.speedsSwept() {
		t.Fatal("off: speedsSwept must be false so the one-time speed upgrade never fires")
	}
	s.speedOff.Store(false)
	if v, n := s.meanSpeed("fast"); n != 1 || v != 50 {
		t.Fatalf("back on: fast = %v·%d, want samples kept (50·1)", v, n)
	}
}
