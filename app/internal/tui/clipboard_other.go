//go:build !windows

package tui

// readClipboard is only needed on Windows: other terminals paste by themselves.
func readClipboard() string { return "" }
