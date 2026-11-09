//go:build !windows

package tun

// Prepare is a no-op on platforms with built-in TUN support.
func Prepare(func(string, ...any)) error { return nil }
