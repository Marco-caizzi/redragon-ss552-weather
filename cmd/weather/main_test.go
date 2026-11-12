package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/weather"
)

func TestParseArgs(t *testing.T) {
	got := parseArgs([]string{"-port", "28196", "--pluginUUID", "u1", "-registerEvent=registerPlugin", "-info", `{"a":1}`, "-x"})
	want := map[string]string{"port": "28196", "pluginUUID": "u1", "registerEvent": "registerPlugin", "info": `{"a":1}`, "x": ""}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestSettingsOf(t *testing.T) {
	raw := settingsOf(json.RawMessage(`{"controller":"Keypad","settings":{"latitude":-31.4}}`))
	if string(raw) != `{"latitude":-31.4}` {
		t.Fatalf("settings = %s", raw)
	}
	if settingsOf(json.RawMessage(`nope`)) != nil {
		t.Fatal("invalid payload should yield nil settings")
	}
}

// fakeDock is a minimal StreamDock: it accepts the plugin's WebSocket, sends events and collects replies.
type fakeDock struct {
	t        *testing.T
	ln       net.Listener
	conn     net.Conn
	reader   *bufio.Reader
	received chan map[string]any
}

func newFakeDock(t *testing.T) *fakeDock {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDock{t: t, ln: ln, received: make(chan map[string]any, 64)}
	t.Cleanup(func() {
		ln.Close()
		if d.conn != nil {
			d.conn.Close()
		}
	})
	return d
}

func (d *fakeDock) port() int { return d.ln.Addr().(*net.TCPAddr).Port }

func (d *fakeDock) accept() {
	conn, err := d.ln.Accept()
	if err != nil {
		d.t.Error(err)
		return
	}
	d.conn = conn
	d.reader = bufio.NewReader(conn)
	req, err := http.ReadRequest(d.reader)
	if err != nil {
		d.t.Error(err)
		return
	}
	sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+
		base64.StdEncoding.EncodeToString(sum[:])+"\r\n\r\n")
	go d.readLoop()
}

func (d *fakeDock) readLoop() {
	for {
		var h [2]byte
		if _, err := io.ReadFull(d.reader, h[:]); err != nil {
			return
		}
		n := uint64(h[1] & 0x7F)
		switch n {
		case 126:
			var e [2]byte
			io.ReadFull(d.reader, e[:])
			n = uint64(binary.BigEndian.Uint16(e[:]))
		case 127:
			var e [8]byte
			io.ReadFull(d.reader, e[:])
			n = binary.BigEndian.Uint64(e[:])
		}
		var mask [4]byte
		io.ReadFull(d.reader, mask[:])
		payload := make([]byte, n)
		if _, err := io.ReadFull(d.reader, payload); err != nil {
			return
		}
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
		var msg map[string]any
		if json.Unmarshal(payload, &msg) == nil {
			d.received <- msg
		}
	}
}

func (d *fakeDock) send(msg string) {
	frame := []byte{0x81}
	if len(msg) < 126 {
		frame = append(frame, byte(len(msg)))
	} else {
		frame = append(frame, 126)
		frame = binary.BigEndian.AppendUint16(frame, uint16(len(msg)))
	}
	if _, err := d.conn.Write(append(frame, msg...)); err != nil {
		d.t.Fatal(err)
	}
}

// waitFor returns the first message matching match, failing after a timeout.
func (d *fakeDock) waitFor(what string, match func(map[string]any) bool) map[string]any {
	d.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case msg := <-d.received:
			if match(msg) {
				return msg
			}
		case <-deadline:
			d.t.Fatalf("timed out waiting for %s", what)
			return nil
		}
	}
}

func imageFor(context string, contains string) func(map[string]any) bool {
	return func(msg map[string]any) bool {
		if msg["event"] != "setImage" || msg["context"] != context {
			return false
		}
		payload, _ := msg["payload"].(map[string]any)
		uri, _ := payload["image"].(string)
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/svg+xml;base64,"))
		return err == nil && strings.Contains(string(data), contains)
	}
}

func TestEndToEnd(t *testing.T) {
	forecast, err := os.ReadFile("../../testdata/cordoba_forecast.json")
	if err != nil {
		t.Fatal(err)
	}
	search, err := os.ReadFile("../../testdata/cordoba_search.json")
	if err != nil {
		t.Fatal(err)
	}
	meteo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			w.Write(search)
			return
		}
		w.Write(forecast)
	}))
	defer meteo.Close()

	dock := newFakeDock(t)
	api := &weather.Client{ForecastURL: meteo.URL + "/forecast", GeocodeURL: meteo.URL + "/search"}
	done := make(chan error, 1)
	go func() { done <- run(dock.port(), "plugin-uuid", "registerPlugin", api, log.New(io.Discard, "", 0)) }()
	dock.accept()

	dock.waitFor("registration", func(m map[string]any) bool { return m["event"] == "registerPlugin" && m["uuid"] == "plugin-uuid" })

	settings := `{"city":"Córdoba, Argentina","latitude":-31.40648,"longitude":-64.18853}`
	dock.send(`{"event":"willAppear","action":"com.marco.weather.key","context":"k1","payload":{"controller":"Keypad","settings":` + settings + `}}`)
	dock.waitFor("key with temperature", imageFor("k1", ">30°<"))

	dock.send(`{"event":"keyDown","action":"com.marco.weather.key","context":"k1","payload":{}}`)
	dock.waitFor("hourly view", imageFor("k1", "Próximas horas"))

	dock.send(`{"event":"willAppear","action":"com.marco.weather.knob","context":"n1","payload":{"controller":"Knob","settings":` + settings + `}}`)
	dock.waitFor("knob with feels-like", imageFor("n1", "Sens. 33°"))
	dock.send(`{"event":"dialRotate","action":"com.marco.weather.knob","context":"n1","payload":{"controller":"Knob","ticks":1,"pressed":false}}`)
	dock.waitFor("knob hourly list", imageFor("n1", "h<"))

	dock.send(`{"event":"willAppear","action":"com.marco.weather.key","context":"k2","payload":{"settings":{}}}`)
	dock.waitFor("unconfigured key", imageFor("k2", "Elegí una ciudad"))

	dock.send(`{"event":"sendToPlugin","action":"com.marco.weather.key","context":"k2","payload":{"cmd":"search","query":"Córdoba","lang":"es"}}`)
	reply := dock.waitFor("search results", func(m map[string]any) bool { return m["event"] == "sendToPropertyInspector" && m["context"] == "k2" })
	payload := reply["payload"].(map[string]any)
	places := payload["places"].([]any)
	first := places[0].(map[string]any)
	if len(places) != 3 || first["label"] != "Córdoba, Provincia de Córdoba, Argentina" || first["city"] != "Córdoba, Argentina" {
		t.Fatalf("search reply = %v", payload)
	}

	// Messages from the settings panel without "action" are still answered, with the instance's action.
	dock.send(`{"event":"sendToPlugin","context":"n1","payload":{"cmd":"search","query":"Córdoba","lang":"es"}}`)
	reply = dock.waitFor("search reply without action", func(m map[string]any) bool { return m["event"] == "sendToPropertyInspector" && m["context"] == "n1" })
	if reply["action"] != KnobAction {
		t.Fatalf("reply action = %v, want %s", reply["action"], KnobAction)
	}

	// Saving the chosen place configures the key.
	dock.send(`{"event":"didReceiveSettings","action":"com.marco.weather.key","context":"k2","payload":{"settings":` + settings + `}}`)
	dock.waitFor("configured key", imageFor("k2", ">30°<"))

	// Events for other plugins are ignored.
	dock.send(`{"event":"keyDown","action":"com.other.plugin","context":"k1","payload":{}}`)

	dock.conn.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("plugin did not stop after the socket closed")
	}
}
