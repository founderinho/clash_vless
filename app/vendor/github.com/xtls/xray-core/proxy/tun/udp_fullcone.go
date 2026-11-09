package tun

import (
	"io"
	"sync"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
)

// clashvless patch (grep clashvless; re-apply after any `go mod vendor`): backport of
// upstream xray-core #5888 + #6285 + #6814 (all landed after our vendored v26.3.27).
// A flow is keyed by the SOURCE addr:port only, but only the first packet's destination
// was kept, so every later packet from that source went to the FIRST destination: an app
// reusing a source port for a new peer (NTP's fixed :123, STUN, DNS, Moonlight) got the
// old peer's reply under the wrong source address and silently timed out. Each packet now
// carries its own destination (b.UDP); a reply whose b.UDP is a domain falls back to the
// flow's target; a finished flow can no longer evict the newer flow that took over its
// source; and packets are queued under the lock, so a closing flow can't panic a sender.
type packet struct {
	data []byte
	dest *net.Destination
}

// sub-handler specifically for udp connections under main handler
type udpConnectionHandler struct {
	sync.Mutex

	udpConns map[net.Destination]*udpConn

	handleConnection func(conn net.Conn, dest net.Destination)
	writePacket      func(data []byte, src net.Destination, dst net.Destination) error
}

func newUdpConnectionHandler(handleConnection func(conn net.Conn, dest net.Destination), writePacket func(data []byte, src net.Destination, dst net.Destination) error) *udpConnectionHandler {
	handler := &udpConnectionHandler{
		udpConns:         make(map[net.Destination]*udpConn),
		handleConnection: handleConnection,
		writePacket:      writePacket,
	}

	return handler
}

// HandlePacket handles UDP packets coming from tun, to forward to the dispatcher
// this custom handler support FullCone NAT of returning packets, binding connection only by the source addr:port
func (u *udpConnectionHandler) HandlePacket(src net.Destination, dst net.Destination, data []byte) bool {
	u.Lock()
	defer u.Unlock()
	conn, found := u.udpConns[src]
	if !found {
		egress := make(chan *packet, 1024)
		conn = &udpConn{handler: u, egress: egress, src: src, dst: dst}
		u.udpConns[src] = conn

		go u.handleConnection(conn, dst)
	}

	// send packet data to the egress channel, if it has buffer, or discard
	select {
	case conn.egress <- &packet{data: data, dest: &dst}:
	default:
	}

	return true
}

func (u *udpConnectionHandler) connectionFinished(conn *udpConn) {
	u.Lock()
	// Close runs twice per flow; a newer conn may already own this src.
	if u.udpConns[conn.src] == conn {
		delete(u.udpConns, conn.src)
		close(conn.egress)
	}
	u.Unlock()
}

// udp connection abstraction
type udpConn struct {
	net.Conn
	buf.Writer

	handler *udpConnectionHandler

	egress chan *packet
	src    net.Destination
	dst    net.Destination
}

// ReadMultiBuffer hands each packet over with its own destination in b.UDP
func (c *udpConn) ReadMultiBuffer() (buf.MultiBuffer, error) {
	for {
		e, ok := <-c.egress
		if !ok {
			return nil, io.EOF
		}

		b := buf.New()
		if _, err := b.Write(e.data); err != nil {
			b.Release()
			continue
		}
		b.UDP = e.dest

		return buf.MultiBuffer{b}, nil
	}
}

// Read packets from the connection
func (c *udpConn) Read(p []byte) (int, error) {
	e, ok := <-c.egress
	if !ok {
		return 0, io.EOF
	}

	n := copy(p, e.data)
	return n, nil
}

// Write returning packets back
func (c *udpConn) Write(p []byte) (int, error) {
	// sending packets back mean sending payload with source/destination reversed
	err := c.handler.writePacket(p, c.dst, c.src)
	if err != nil {
		return 0, nil
	}

	return len(p), nil
}

func (c *udpConn) Close() error {
	c.handler.connectionFinished(c)

	return nil
}

func (c *udpConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: c.dst.Address.IP(), Port: int(c.dst.Port.Value())}
}

func (c *udpConn) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: c.src.Address.IP(), Port: int(c.src.Port.Value())}
}

// Write returning packets back
func (c *udpConn) WriteMultiBuffer(mb buf.MultiBuffer) error {
	defer buf.ReleaseMulti(mb)
	for _, b := range mb {
		dst := c.dst
		if b.UDP != nil && !b.UDP.Address.Family().IsDomain() {
			dst = *b.UDP
		}

		// validate address family matches between buffer packet and the connection
		if dst.Address.Family() != c.dst.Address.Family() {
			continue
		}

		// sending packets back mean sending payload with source/destination reversed
		err := c.handler.writePacket(b.Bytes(), dst, c.src)
		if err != nil {
			// udp doesn't guarantee delivery, so in any failure we just continue to the next packet
			continue
		}
	}

	return nil
}
