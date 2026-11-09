package gui

import "errors"

// ErrUnavailable means no GUI can run here, so a bare launch should use the TUI.
var ErrUnavailable = errors.New("no graphical display available — use `clashvless tui`")
