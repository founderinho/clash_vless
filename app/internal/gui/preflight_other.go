//go:build !windows

package gui

// Preflight checks that the GUI can open a window. Only Windows needs it so far.
func Preflight() error { return nil }
