// Package engines lets consoles share one KMP and one MADE process. Each
// console used to launch its own kmp-mcp and made-mcp over stdio; with
// engines.shared on, a per-user daemon runs one engine per launch spec and
// multiplexes the consoles' MCP sessions onto it over a private unix socket.
package engines

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

// maxMessageBytes bounds one JSON-RPC message; MADE's unpaginated lists
// reached 2.76 MB (8 October 2026).
const maxMessageBytes = 64 << 20

// muxProtocolVersion is what the multiplexer asks the engine for; the engine
// answers with the version it speaks and every client gets that answer.
const muxProtocolVersion = "2025-06-18"

type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

// mux carries the MCP sessions of several clients over one engine's stdio
// session. The engine is initialized once and every client's initialize is
// answered from that result; requests are forwarded with ids of the mux's
// own and answered back with the client's; engine notifications go to every
// client. The engine sees one client: requests it makes of its client
// (roots, sampling, elicitation) are refused, a ping is answered.
type mux struct {
	engine   io.Writer
	engineMu sync.Mutex
	init     json.RawMessage

	mu      sync.Mutex
	next    int64
	routes  map[int64]route
	clients map[*muxClient]bool
	closed  bool
}

type route struct {
	client *muxClient
	id     json.RawMessage
}

type muxClient struct {
	conn    io.ReadWriteCloser
	writeMu sync.Mutex
	// pending maps the client's request ids to the mux's, for cancellations.
	pending map[string]int64
}

func newScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), maxMessageBytes)
	return scanner
}

// startMux initializes the engine over its stdin and stdout and returns the
// mux once the engine answered; it then reads the engine until its stdout
// closes, when every client is disconnected and done is closed.
func startMux(engineIn io.Writer, engineOut io.Reader, version string) (*mux, <-chan struct{}, error) {
	m := &mux{engine: engineIn, routes: map[int64]route{}, clients: map[*muxClient]bool{}}
	scanner := newScanner(engineOut)
	params, _ := json.Marshal(map[string]any{"protocolVersion": muxProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "axlr-engines", "version": version}})
	if err := m.toEngine(message{ID: json.RawMessage(`"axlr-engines-initialize"`), Method: "initialize", Params: params}); err != nil {
		return nil, nil, err
	}
	for {
		if !scanner.Scan() {
			return nil, nil, errors.Join(errors.New("the engine closed before it initialized"), scanner.Err())
		}
		var reply message
		if json.Unmarshal(scanner.Bytes(), &reply) != nil || string(reply.ID) != `"axlr-engines-initialize"` {
			continue // a log line or an early notification
		}
		if len(reply.Result) == 0 {
			return nil, nil, fmt.Errorf("the engine refused initialize: %s", bounded(reply.Error))
		}
		m.init = append(json.RawMessage(nil), reply.Result...)
		break
	}
	if err := m.toEngine(message{Method: "notifications/initialized"}); err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for scanner.Scan() {
			m.fromEngine(scanner.Bytes())
		}
		m.close()
	}()
	return m, done, nil
}

func bounded(raw json.RawMessage) string {
	if len(raw) > 300 {
		return string(raw[:300])
	}
	return string(raw)
}

// encode writes one message as a JSON line; the fields are kept raw.
func encode(w io.Writer, msg message) error {
	fields := map[string]json.RawMessage{"jsonrpc": json.RawMessage(`"2.0"`)}
	if msg.ID != nil {
		fields["id"] = msg.ID
	}
	if msg.Method != "" {
		method, _ := json.Marshal(msg.Method)
		fields["method"] = method
	}
	for key, value := range map[string]json.RawMessage{"params": msg.Params, "result": msg.Result, "error": msg.Error} {
		if value != nil {
			fields[key] = value
		}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func (m *mux) toEngine(msg message) error {
	m.engineMu.Lock()
	defer m.engineMu.Unlock()
	return encode(m.engine, msg)
}

// clientWriteWait bounds a write to one console: the engine's output is
// read by one loop, so a console that stops reading (suspended, wedged)
// would stall every other one. It is dropped instead and reconnects.
const clientWriteWait = 10 * time.Second

func (c *muxClient) send(msg message) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if deadline, ok := c.conn.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = deadline.SetWriteDeadline(time.Now().Add(clientWriteWait))
	}
	if encode(c.conn, msg) != nil {
		_ = c.conn.Close()
	}
}

// fromEngine routes one line the engine wrote.
func (m *mux) fromEngine(line []byte) {
	var msg message
	if json.Unmarshal(line, &msg) != nil {
		return
	}
	switch {
	case msg.Method == "" && msg.ID != nil:
		id, err := strconv.ParseInt(string(msg.ID), 10, 64)
		m.mu.Lock()
		r, ok := m.routes[id]
		if ok {
			delete(m.routes, id)
			delete(r.client.pending, string(r.id))
		}
		live := ok && m.clients[r.client]
		m.mu.Unlock()
		if err == nil && live {
			msg.ID = r.id
			r.client.send(msg)
		}
	case msg.Method != "" && msg.ID != nil:
		reply := message{ID: msg.ID, Error: json.RawMessage(`{"code":-32601,"message":"the engine is shared by several AXLR consoles; client requests are not supported"}`)}
		if msg.Method == "ping" {
			reply = message{ID: msg.ID, Result: json.RawMessage(`{}`)}
		}
		_ = m.toEngine(reply)
	case msg.Method != "" && msg.Method != "notifications/progress":
		// Progress belongs to one request's token; the rest is for all.
		m.mu.Lock()
		clients := make([]*muxClient, 0, len(m.clients))
		for c := range m.clients {
			clients = append(clients, c)
		}
		m.mu.Unlock()
		for _, c := range clients {
			c.send(msg)
		}
	}
}

// serve carries one client's session until it disconnects.
func (m *mux) serve(conn io.ReadWriteCloser) {
	c := &muxClient{conn: conn, pending: map[string]int64{}}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = conn.Close()
		return
	}
	m.clients[c] = true
	m.mu.Unlock()
	defer m.drop(c)
	scanner := newScanner(conn)
	for scanner.Scan() {
		var msg message
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			continue
		}
		switch {
		case msg.Method == "initialize" && msg.ID != nil:
			c.send(message{ID: msg.ID, Result: m.init})
		case msg.Method == "notifications/initialized":
		case msg.Method != "" && msg.ID != nil:
			m.mu.Lock()
			m.next++
			id := m.next
			m.routes[id] = route{client: c, id: append(json.RawMessage(nil), msg.ID...)}
			c.pending[string(msg.ID)] = id
			m.mu.Unlock()
			msg.ID = json.RawMessage(strconv.FormatInt(id, 10))
			if m.toEngine(msg) != nil {
				return
			}
		case msg.Method == "notifications/cancelled":
			if msg.Params = m.cancelled(c, msg.Params); msg.Params != nil && m.toEngine(msg) != nil {
				return
			}
		case msg.Method != "" && msg.ID == nil:
			if m.toEngine(msg) != nil {
				return
			}
		}
		// A client's answer to an engine request: the mux answered it.
	}
}

// cancelled rewrites a cancellation's requestId to the mux's id; nil drops
// a cancellation of a request the engine already answered.
func (m *mux) cancelled(c *muxClient, params json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil {
		return nil
	}
	m.mu.Lock()
	id, ok := c.pending[string(bytes.TrimSpace(fields["requestId"]))]
	m.mu.Unlock()
	if !ok {
		return nil
	}
	fields["requestId"] = json.RawMessage(strconv.FormatInt(id, 10))
	rewritten, _ := json.Marshal(fields)
	return rewritten
}

// drop forgets a client; the engine's later answers to it are discarded.
func (m *mux) drop(c *muxClient) {
	m.mu.Lock()
	delete(m.clients, c)
	for id, r := range m.routes {
		if r.client == c {
			delete(m.routes, id)
		}
	}
	m.mu.Unlock()
	_ = c.conn.Close()
}

// clientCount is how many clients are connected.
func (m *mux) clientCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.clients)
}

// close disconnects every client once the engine is gone.
func (m *mux) close() {
	m.mu.Lock()
	m.closed = true
	clients := m.clients
	m.clients = map[*muxClient]bool{}
	m.mu.Unlock()
	for c := range clients {
		_ = c.conn.Close()
	}
}
