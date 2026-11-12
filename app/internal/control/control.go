// Package control is the daemon↔client IPC. The daemon (`run`) serves a Unix
// socket that streams status/log/state events and accepts commands; the TUI and
// CLI attach as clients. Wire format is JSON values over the socket.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"clashvless/internal/engine"
	"clashvless/internal/happ"
	"clashvless/internal/loghub"
	"clashvless/internal/store"
)

// Event is a daemon→client message. Line keeps older clients compatible;
// Record carries filterable severity and origin fields to current clients.
type Event struct {
	Type   string          `json:"type"`         // "state" | "status" | "log" | "ack"
	ID     string          `json:"id,omitempty"` // the acknowledged command's ID
	State  json.RawMessage `json:"state,omitempty"`
	Status *engine.Status  `json:"status,omitempty"`
	Line   string          `json:"line,omitempty"`
	Record *loghub.Record  `json:"record,omitempty"`
}

func LogEvent(record loghub.Record) Event {
	return Event{Type: "log", Line: record.String(), Record: &record}
}

// Command is a client→daemon message.
type Command struct {
	Cmd   string          `json:"cmd"`          // "patch" | "addsub" | "rmsub" | "refetch" | "kick"
	ID    string          `json:"id,omitempty"` // when set, the daemon answers with an "ack" event once executed
	Patch json.RawMessage `json:"patch,omitempty"`
	URL   string          `json:"url,omitempty"`
	Index int             `json:"index,omitempty"`
}

// --- hub: fan events out to connected clients --------------------------------

type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

func (h *Hub) Broadcast(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default: // slow client — drop rather than stall the engine
		}
	}
}

func (h *Hub) subscribe() chan Event {
	ch := make(chan Event, 128)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// unsubscribe closes ch so its writer goroutine ends; under mu, so Broadcast
// never sends on a closed channel.
func (h *Hub) unsubscribe(ch chan Event) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// --- server (daemon side) ----------------------------------------------------

type Server struct {
	sup *engine.Supervisor
	hub *Hub
}

func NewServer(sup *engine.Supervisor, hub *Hub) *Server { return &Server{sup: sup, hub: hub} }

// Serve accepts clients on ln until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		ln.Close()
		_ = os.Remove(ln.Addr().String())
	}()
	go s.autoFetchLoop(ctx)
	backoff := 5 * time.Millisecond
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			// A transient failure such as running out of file descriptors.
			time.Sleep(backoff)
			backoff = min(2*backoff, time.Second)
			continue
		}
		backoff = 5 * time.Millisecond
		go s.handle(conn)
	}
}

// Listen binds the control socket. Bind before dialing it: on Windows a connect to
// a missing path creates a file there, and bind then fails with "address in use".
func Listen(socketPath string) (net.Listener, error) {
	_ = os.Remove(socketPath) // clear a stale socket
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(socketPath, 0o600)
	store.ReownToInvoker(socketPath) // sudo daemon → let the rootless TUI attach
	return ln, nil
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	enc := json.NewEncoder(conn)
	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	// Initial snapshot so a fresh client can render immediately.
	_ = enc.Encode(Event{Type: "state", State: s.sup.Snapshot()})
	st := s.sup.CurrentStatus()
	_ = enc.Encode(Event{Type: "status", Status: &st})

	go func() {
		for e := range ch {
			if enc.Encode(e) != nil {
				conn.Close()
				return
			}
		}
	}()

	dec := json.NewDecoder(conn)
	for {
		var cmd Command
		if dec.Decode(&cmd) != nil {
			return // client gone
		}
		s.exec(cmd)
		if cmd.ID != "" {
			select {
			case ch <- Event{Type: "ack", ID: cmd.ID}:
			default: // a client this far behind isn't reading; don't block on it
			}
		}
	}
}

func (s *Server) exec(cmd Command) {
	switch cmd.Cmd {
	case "kick":
		s.sup.Kick()
	case "patch":
		var fresh store.State
		if err := json.Unmarshal(cmd.Patch, &fresh); err != nil {
			s.sup.Logf(loghub.Error, "config", "patch", "rejected settings change: %v", err)
			return
		}
		s.applyAndBroadcast(func(st *store.State) { _ = applyPatch(st, cmd.Patch) })
	case "rmsub":
		i := cmd.Index
		s.applyAndBroadcast(func(st *store.State) { st.RemoveSub(i) })
	case "addsub":
		go s.addSub(cmd.URL)
	case "refetch":
		go s.refetch()
	}
}

// applyPatch replaces every top-level field the patch names. Unmarshalling into
// the live state would merge instead: an array element that omits an omitempty
// field (a node without "raw", a sub without "fetch_err") would keep the value of
// whatever element used to sit at that index.
func applyPatch(st *store.State, patch json.RawMessage) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(patch, &keys); err != nil {
		return err
	}
	var fresh store.State
	if err := json.Unmarshal(patch, &fresh); err != nil {
		return err
	}
	dst, src := reflect.ValueOf(st).Elem(), reflect.ValueOf(&fresh).Elem()
	for i := 0; i < dst.NumField(); i++ {
		field := dst.Type().Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if !field.IsExported() || name == "" || name == "-" {
			continue
		}
		if _, ok := keys[name]; ok {
			dst.Field(i).Set(src.Field(i))
		}
	}
	return nil
}

func (s *Server) applyAndBroadcast(mut func(*store.State)) {
	s.sup.SetConfig(mut)
	if err := s.sup.Save(); err != nil {
		s.sup.Logf(loghub.Error, "config", "save", "could not save settings: %v", err)
	}
	s.sup.Kick()
	s.hub.Broadcast(Event{Type: "state", State: s.sup.Snapshot()})
}

func (s *Server) addSub(url string) {
	var st store.State
	_ = json.Unmarshal(s.sup.Snapshot(), &st)
	s.sup.Logf(loghub.Info, "subscription", "fetch", "adding subscription")
	nodes, title, err := happ.Fetch(st.Device, url, st.FetchProxyAddr())
	s.applyAndBroadcast(func(st *store.State) {
		st.AddSub(title, url)
		sb := &st.Subs[len(st.Subs)-1]
		if err == nil {
			sb.Nodes = nodes
			sb.LastFetch = time.Now()
		} else {
			sb.FetchErr = fetchError(err, url)
		}
	})
	if err != nil {
		s.sup.Logf(loghub.Error, "subscription", "fetch", "add failed: %s", fetchError(err, url))
	} else {
		s.sup.Logf(loghub.Info, "subscription", "fetch", "added %q — %d nodes", title, len(nodes))
	}
}

func (s *Server) refetch() {
	var st store.State
	_ = json.Unmarshal(s.sup.Snapshot(), &st)
	dev := st.Device
	type upd struct {
		url, title string
		nodes      []store.Node
		ok         bool
		err        error
	}
	ups := make([]upd, len(st.Subs))
	for i, sb := range st.Subs {
		nodes, title, err := happ.Fetch(dev, sb.URL, st.FetchProxyAddr())
		ups[i] = upd{sb.URL, title, nodes, err == nil, err}
	}
	s.applyAndBroadcast(func(st *store.State) {
		for _, u := range ups {
			for i := range st.Subs {
				if st.Subs[i].URL == u.url && !u.ok {
					st.Subs[i].FetchErr = fetchError(u.err, u.url)
				} else if st.Subs[i].URL == u.url {
					st.Subs[i].FetchErr = ""
					st.Subs[i].Nodes = u.nodes
					st.Subs[i].LastFetch = time.Now()
					if u.title != "" {
						st.Subs[i].Name = u.title
					}
				}
			}
		}
	})
	if len(ups) == 0 {
		s.sup.Logf(loghub.Warning, "subscription", "fetch", "no subscriptions to refresh")
	}
	for _, u := range ups {
		if u.ok {
			name := u.title
			if name == "" {
				name = subHost(u.url)
			}
			s.sup.Logf(loghub.Info, "subscription", "fetch", "refreshed %q — %d nodes", name, len(u.nodes))
		} else {
			s.sup.Logf(loghub.Error, "subscription", "fetch", "refresh failed: %s", fetchError(u.err, u.url))
		}
	}
}

// subHost names an untitled subscription in logs: its path and query hold the token.
func subHost(subscriptionURL string) string {
	if parsed, err := url.Parse(subscriptionURL); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return "<subscription URL>"
}

func fetchError(err error, subscriptionURL string) string {
	return strings.ReplaceAll(err.Error(), subscriptionURL, "<subscription URL>")
}

// autoFetchLoop periodically refetches every subscription so the node set never
// goes stale (a stale sub keeps probing dead servers and misses fresh ones — the
// "different providers on two boxes" symptom). It checks on a coarse tick and
// refetches only when the oldest sub is past AutoFetchInterval, so it's cheap and
// self-healing: a box that was offline for days refetches shortly after start.
// Runs in both `run` and embedded daemons; stops with ctx.
func (s *Server) autoFetchLoop(ctx context.Context) {
	const checkEvery = 30 * time.Minute
	t := time.NewTicker(checkEvery)
	defer t.Stop()
	for {
		s.maybeAutoFetch()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// maybeAutoFetch refetches all subs when auto-fetch is on and the oldest sub's
// last successful fetch is past the configured interval (a never-fetched sub has
// a zero time, so it always qualifies). Silent when there's nothing to do.
func (s *Server) maybeAutoFetch() {
	var st store.State
	_ = json.Unmarshal(s.sup.Snapshot(), &st)
	if !subsStale(&st, time.Now()) {
		return
	}
	s.sup.Log("auto-fetch: subscriptions stale, refreshing")
	s.refetch()
}

// subsStale reports whether an automatic refetch is due: auto-fetch is on, there
// is at least one sub, and the OLDEST sub's last successful fetch is past the
// interval (a never-fetched sub has a zero time, so it always qualifies).
func subsStale(st *store.State, now time.Time) bool {
	if !st.AutoFetch() || len(st.Subs) == 0 {
		return false
	}
	oldest := now
	for _, sb := range st.Subs {
		if sb.LastFetch.Before(oldest) {
			oldest = sb.LastFetch
		}
	}
	return now.Sub(oldest) >= st.AutoFetchInterval()
}

// --- client side -------------------------------------------------------------

type Client struct {
	conn   net.Conn
	enc    *json.Encoder
	events chan Event
}

// Dial connects to the daemon's control socket.
func Dial(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, enc: json.NewEncoder(conn), events: make(chan Event, 128)}
	go c.readLoop()
	return c, nil
}

func (c *Client) readLoop() {
	defer close(c.events)
	dec := json.NewDecoder(c.conn)
	for {
		var e Event
		if dec.Decode(&e) != nil {
			return
		}
		c.events <- e
	}
}

// Events streams daemon events; the channel closes when the daemon disconnects.
func (c *Client) Events() <-chan Event { return c.events }

func (c *Client) Send(cmd Command) error { return c.enc.Encode(cmd) }

func (c *Client) SendPatch(patch json.RawMessage) error {
	return c.Send(Command{Cmd: "patch", Patch: patch})
}

// SendAndWait sends cmd and waits until the daemon has executed it. A one-shot
// client must not close earlier: on Windows the close can reset the connection
// before the daemon reads the command, which is then silently lost.
func (c *Client) SendAndWait(cmd Command, timeout time.Duration) error {
	cmd.ID = strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := c.Send(cmd); err != nil {
		return err
	}
	deadline := time.After(timeout)
	for {
		select {
		case e, ok := <-c.events:
			if !ok {
				return errors.New("daemon closed the connection before confirming")
			}
			if e.Type == "ack" && e.ID == cmd.ID {
				return nil
			}
		case <-deadline:
			return errors.New("daemon did not confirm the change (is it an older version? restart it)")
		}
	}
}

func (c *Client) Close() error { return c.conn.Close() }
