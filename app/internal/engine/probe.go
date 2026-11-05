package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/net/proxy"
)

// egressURL is a tiny 204 endpoint used to prove a chain actually reaches the
// internet (not just that a TCP hop is up). HTTPS on 443 (not plain HTTP:80,
// which some exits filter) so a passing ping matches what real traffic can do.
const egressURL = "https://cp.cloudflare.com/generate_204"

// speedURLs are large downloads used for throughput tests (time- and byte-bounded),
// in preference order. A sweep starts on the first and moves down the list when one
// refuses (non-2xx): every chained test exits via the same main IP, and Cloudflare
// 429s that IP once it has pulled a few GB, while the others keep serving it.
// speedUA is sent on speedtest requests: some references (Hetzner) silently close
// the connection on Go's default "Go-http-client/1.1".
const speedUA = "curl/8.7.1"

var speedURLs = []string{
	"https://speed.cloudflare.com/__down?bytes=52428800",
	"https://fsn1-speed.hetzner.com/100MB.bin",
	"https://proof.ovh.net/files/100Mb.dat",
	"https://hel1-speed.hetzner.com/100MB.bin",
}

// ErrSpeedRefused means the speed reference answered with a non-2xx status (e.g.
// Cloudflare's 429 once one exit IP has pulled too much). That says nothing about
// the path under test, so callers must not score it as a slow/failed node.
var ErrSpeedRefused = errors.New("speed reference refused")

// SpeedThroughSOCKS downloads url through the SOCKS port for up to budget and returns
// throughput in Mbps. It's a one-shot, time- and byte-bounded speedtest. A non-2xx
// reply is ErrSpeedRefused (wrapped with the status); any other error means the
// path itself couldn't carry the transfer.
func SpeedThroughSOCKS(ctx context.Context, socksPort int, url string, budget time.Duration) (float64, error) {
	d, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), nil, &net.Dialer{Timeout: 5 * time.Second})
	if err != nil {
		return 0, err
	}
	tr := &http.Transport{
		DialContext:       func(_ context.Context, network, addr string) (net.Conn, error) { return d.Dial(network, addr) },
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: tr}
	dl, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	req, err := http.NewRequestWithContext(dl, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", speedUA) // Hetzner's speed hosts drop Go's default UA (EOF)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("%w: HTTP %d", ErrSpeedRefused, resp.StatusCode)
	}
	n, _ := io.Copy(io.Discard, resp.Body) // reads until the budget deadline cancels it
	secs := time.Since(start).Seconds()
	if n < 1<<16 || secs <= 0 {
		return 0, fmt.Errorf("only %d bytes in %.1fs", n, secs)
	}
	return float64(n*8) / secs / 1e6, nil
}

// TCPLatency measures how long a raw TCP connect to host:port takes. It's the
// cheap pre-rank used to try the fastest entries first, before the (costlier)
// end-to-end egress test.
func TCPLatency(host string, port int, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	c, err := d.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return 0, err
	}
	_ = c.Close()
	return time.Since(start), nil
}

// freePort asks the OS for an unused loopback TCP port. Throwaway probe instances
// use a fresh one each time, so a service holding any fixed port can't wedge probing.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitPort blocks until a local TCP port accepts connections, up to timeout.
func waitPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// EgressThroughSOCKS makes a 204 request through the given local SOCKS5 port and
// returns its round-trip latency, or an error if the chain can't reach the net.
func EgressThroughSOCKS(ctx context.Context, socksPort int, timeout time.Duration) (time.Duration, error) {
	d, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), nil, &net.Dialer{Timeout: timeout})
	if err != nil {
		return 0, err
	}
	tr := &http.Transport{
		DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
			return d.Dial(network, addr)
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: tr, Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, egressURL, nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("egress status %d", resp.StatusCode)
	}
	return time.Since(start), nil
}
