//go:build android && cgo

package main

// haveCgo: lookups go through bionic getaddrinfo → netd, i.e. the system
// resolver (Private DNS, per-network) — the same path the browser takes.
const haveCgo = true
