//go:build android

package tun

// On Android the app's VpnService owns the device, routes and DNS, hands the fd
// to xray via XRAY_TUN_FD, and excludes this app's UID from the VPN — so every
// xray socket is already off-tun and the OS layer here is a no-op.

func Supported() bool { return true }

func Privileged() bool { return true }

func DefaultName() string { return "clashvless0" }

func FwMark() int32 { return 0 }

func UplinkDevice() string { return "" }

func SystemResolver() string { return "" }

func RemoveDevice(string) {}

func (m *Manager) osUp(cfg Config) error { return nil }

func (m *Manager) osDown() error { return nil }

func (m *Manager) osReassert() (bool, error) { return false, nil }

func (m *Manager) osGatewayChanged() (bool, string) { return false, "" }

func (m *Manager) osReapply() error { return nil }

func (m *Manager) osReapplyDNS(string, bool) error { return nil }

func currentDHCPResolver(string) string { return "" }

func HealStaleResolver() (bool, string) { return false, "" }
