package engine

import (
	"fmt"
	"os"
	"sync"

	"clashvless/internal/loghub"
	xlog "github.com/xtls/xray-core/common/log"
)

type funcSink struct{ fn func(loghub.Record) }

func (h funcSink) Handle(m xlog.Message) {
	if h.fn == nil {
		return
	}
	level := loghub.Info
	element := "core"
	text := m.String()
	switch m.(type) {
	case *xlog.AccessMessage, *xlog.DNSLog:
		level, element = loghub.Debug, "access"
	}
	if message, ok := m.(*xlog.GeneralMessage); ok {
		text = fmt.Sprint(message.Content)
		switch message.Severity {
		case xlog.Severity_Error:
			level = loghub.Error
		case xlog.Severity_Warning:
			level = loghub.Warning
		case xlog.Severity_Debug:
			level = loghub.Debug
		}
	}
	h.fn(loghub.New(level, "xray", element, "%s", text))
}

// SetLogSink redirects startup and per-instance xray output as structured records.
func SetLogSink(fn func(loghub.Record)) {
	sink := funcSink{fn}
	xlog.RegisterHandler(sink)
	xlog.RegisterObserver(sink)
}

// EventFileSink forwards records to base and mirrors them to a 0600 log file.
func EventFileSink(path string, base func(loghub.Record)) func(loghub.Record) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return base
	}
	var mu sync.Mutex
	return func(record loghub.Record) {
		if base != nil {
			base(record)
		}
		mu.Lock()
		_, _ = fmt.Fprintln(f, record.String())
		mu.Unlock()
	}
}
