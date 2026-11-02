//go:build android

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"clashvless/internal/store"
)

// platformInit: Android has no /etc/resolv.conf, so cgo-less Go would query
// 127.0.0.1:53. Resolve via the servers the app passes in (its active network's).
func platformInit(st *store.State) {
	var servers []string
	for _, s := range strings.Split(os.Getenv("CLASHVLESS_DNS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			servers = append(servers, s)
		}
	}
	servers = append(servers, "8.8.8.8", "1.1.1.1")
	// A UDP dial never fails, so rotate per query: the resolver's retries then walk
	// every server instead of hammering a dead first one.
	var next atomic.Uint32
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		s := servers[int(next.Add(1)-1)%len(servers)]
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort(s, "53"))
	}}
	if st.ListenAddr == "" {
		st.ListenAddr = "127.0.0.1" // a phone roams public Wi-Fi; don't expose the SOCKS ports
	}
}

// platformTunFd receives the VpnService tun fd (SCM_RIGHTS over the unix socket
// named by CLASHVLESS_TUN_SOCK) and hands it to xray's tun inbound. Without it
// TUN stays off — XRAY_TUN_FD would otherwise default to fd 0.
func platformTunFd(st *store.State) error {
	st.TunEnabled = false
	path := os.Getenv("CLASHVLESS_TUN_SOCK")
	if path == "" {
		return nil
	}
	_ = os.Remove(path)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return fmt.Errorf("tun fd socket: %w", err)
	}
	defer ln.Close()
	_ = ln.SetDeadline(time.Now().Add(20 * time.Second))
	c, err := ln.AcceptUnix()
	if err != nil {
		return fmt.Errorf("tun fd: no VpnService connected: %w", err)
	}
	defer c.Close()
	oob := make([]byte, syscall.CmsgSpace(4))
	_, oobn, _, _, err := c.ReadMsgUnix(make([]byte, 1), oob)
	if err != nil {
		return fmt.Errorf("tun fd recv: %w", err)
	}
	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(msgs) == 0 {
		return fmt.Errorf("tun fd: no control message (%v)", err)
	}
	fds, err := syscall.ParseUnixRights(&msgs[0])
	if err != nil || len(fds) == 0 {
		return fmt.Errorf("tun fd: no fd passed (%v)", err)
	}
	_ = os.Setenv("XRAY_TUN_FD", strconv.Itoa(fds[0]))
	st.TunEnabled = true
	st.TunDNSDirect = false // the VpnService sets DNS (8.8.8.8, rides the tunnel)
	fmt.Println("▶ android: got VpnService tun fd", fds[0])
	return nil
}
