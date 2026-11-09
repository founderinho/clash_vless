// Package crashlog preserves fatal diagnostics for builds without a console.
package crashlog

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"
)

func Enable(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		return nil, err
	}
	return func() { _ = debug.SetCrashOutput(nil, debug.CrashOptions{}) }, nil
}

func Recovered(path string, value any) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s recovered GUI panic: %v\n%s\n", time.Now().Format(time.RFC3339), value, debug.Stack())
}
