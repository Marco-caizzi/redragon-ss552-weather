package streamdock

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// testServer is a tiny WebSocket server that records client messages and sends scripted frames.
type testServer struct {
	ln       net.Listener
	received chan []byte
	script   func(conn net.Conn, r *bufio.Reader)
}

func newTestServer(t *testing.T, script func(conn net.Conn, r *bufio.Reader)) *testServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &testServer{ln: ln, received: make(chan []byte, 16), script: script}
	go s.serve(t)
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *testServer) serve(t *testing.T) {
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	req, err := http.ReadRequest(r)
	if err != nil {
		t.Errorf("reading upgrade request: %v", err)
		return
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
	if _, err := io.WriteString(conn, resp); err != nil {
		return
	}
	s.script(conn, r)
}

func (s *testServer) addr() string { return s.ln.Addr().String() }

// Server-side frames are not masked.
func serverFrame(fin bool, opcode byte, payload []byte) []byte {
	b0 := opcode
	if fin {
		b0 |= 0x80
	}
	frame := []byte{b0}
	switch {
	case len(payload) < 126:
		frame = append(frame, byte(len(payload)))
	case len(payload) <= 0xFFFF:
		frame = append(frame, 126)
		frame = binary.BigEndian.AppendUint16(frame, uint16(len(payload)))
	default:
		frame = append(frame, 127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(len(payload)))
	}
	return append(frame, payload...)
}

// readClientFrame reads one masked client frame.
func readClientFrame(t *testing.T, r *bufio.Reader) (byte, []byte) {
	t.Helper()
	c := &Conn{reader: r}
	_, opcode, payload, err := c.readFrame()
	if err != nil {
		t.Errorf("reading client frame: %v", err)
	}
	return opcode, payload
}

func TestRegisterSendsUUID(t *testing.T) {
	got := make(chan []byte, 1)
	s := newTestServer(t, func(conn net.Conn, r *bufio.Reader) {
		op, payload := readClientFrame(t, r)
		if op != opText {
			t.Errorf("registration opcode = %d, want text", op)
		}
		got <- payload
	})
	port := s.ln.Addr().(*net.TCPAddr).Port
	c, err := Connect(port, "plugin-uuid", "registerPlugin")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	select {
	case payload := <-got:
		var msg map[string]string
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatal(err)
		}
		if msg["event"] != "registerPlugin" || msg["uuid"] != "plugin-uuid" {
			t.Fatalf("registration = %v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no registration received")
	}
}

func TestReadFragmentedPingAndLargeMessages(t *testing.T) {
	large := make([]byte, 70000) // Forces the 64-bit length encoding.
	for i := range large {
		large[i] = 'a'
	}
	pong := make(chan []byte, 1)
	s := newTestServer(t, func(conn net.Conn, r *bufio.Reader) {
		conn.Write(serverFrame(false, opText, []byte(`{"event":"dial`)))
		conn.Write(serverFrame(false, opContinuation, []byte(`Rotate","context":"k1",`)))
		conn.Write(serverFrame(true, opContinuation, []byte(`"payload":{"ticks":-2}}`)))
		conn.Write(serverFrame(true, opPing, []byte("hi")))
		op, payload := readClientFrame(t, r)
		if op == opPong {
			pong <- payload
		}
		conn.Write(serverFrame(true, opText, large))
		conn.Write(serverFrame(true, opClose, nil))
		readClientFrame(t, r) // Close echo.
	})

	ws, err := Dial(s.addr(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	c := &Client{conn: ws}

	ev, err := c.Next()
	if err != nil {
		t.Fatal(err)
	}
	var p DialPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if ev.Event != "dialRotate" || ev.Context != "k1" || p.Ticks != -2 {
		t.Fatalf("event = %+v payload = %+v", ev, p)
	}

	msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if len(msg) != len(large) {
		t.Fatalf("large message length = %d", len(msg))
	}
	select {
	case p := <-pong:
		if string(p) != "hi" {
			t.Fatalf("pong payload = %q", p)
		}
	default:
		t.Fatal("ping was not answered with pong")
	}

	if _, err := ws.ReadMessage(); err != ErrClosed {
		t.Fatalf("after close err = %v, want ErrClosed", err)
	}
}

func TestClientFramesAreMaskedWithAllLengths(t *testing.T) {
	sizes := []int{5, 200, 70000}
	got := make(chan []byte, len(sizes))
	s := newTestServer(t, func(conn net.Conn, r *bufio.Reader) {
		for range sizes {
			var header [2]byte
			if _, err := io.ReadFull(r, header[:]); err != nil {
				t.Error(err)
				return
			}
			if header[1]&0x80 == 0 {
				t.Error("client frame not masked")
			}
			length := uint64(header[1] & 0x7F)
			switch length {
			case 126:
				var ext [2]byte
				io.ReadFull(r, ext[:])
				length = uint64(binary.BigEndian.Uint16(ext[:]))
			case 127:
				var ext [8]byte
				io.ReadFull(r, ext[:])
				length = binary.BigEndian.Uint64(ext[:])
			}
			var mask [4]byte
			io.ReadFull(r, mask[:])
			payload := make([]byte, length)
			io.ReadFull(r, payload)
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
			got <- payload
		}
	})
	ws, err := Dial(s.addr(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	for _, n := range sizes {
		data := make([]byte, n)
		for i := range data {
			data[i] = byte('0' + i%10)
		}
		if err := ws.WriteText(data); err != nil {
			t.Fatal(err)
		}
		select {
		case p := <-got:
			if string(p) != string(data) {
				t.Fatalf("size %d: payload mismatch", n)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("size %d: no frame received", n)
		}
	}
}
