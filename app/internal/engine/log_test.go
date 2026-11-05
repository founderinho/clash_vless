package engine

import (
	"testing"

	"clashvless/internal/loghub"
	xlog "github.com/xtls/xray-core/common/log"
)

func TestStructuredXraySink(t *testing.T) {
	var records []loghub.Record
	SetLogSink(func(record loghub.Record) { records = append(records, record) })
	t.Cleanup(func() {
		xlog.RegisterObserver(nil)
		xlog.RegisterHandler(funcSink{})
	})

	xlog.Record(&xlog.GeneralMessage{Severity: xlog.Severity_Warning, Content: "deprecated transport"})
	if !xlog.Observe(&xlog.AccessMessage{From: "local", To: "example.com:443", Status: xlog.AccessAccepted}) {
		t.Fatal("observer did not consume access message")
	}
	if len(records) != 2 {
		t.Fatalf("records = %d", len(records))
	}
	if records[0].Level != loghub.Warning || records[0].Module != "xray" || records[0].Message != "deprecated transport" {
		t.Fatalf("general record = %+v", records[0])
	}
	if records[1].Level != loghub.Debug || records[1].Element != "access" {
		t.Fatalf("access record = %+v", records[1])
	}
}
