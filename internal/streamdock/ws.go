package streamdock

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Minimal RFC 6455 client: text messages, ping/pong and close.
// StreamDock only talks to plugins over ws://127.0.0.1, so TLS and extensions are not needed.

const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA

	maxMessageSize = 16 << 20
	websocketGUID  = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

// ErrClosed is returned after the server closes the connection.
var ErrClosed = errors.New("websocket closed")

// Conn is a client WebSocket connection.
type Conn struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

// Dial opens a WebSocket connection to ws://addr/.
func Dial(addr string, timeout time.Duration) (*Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	ws, err := handshake(conn, addr, timeout)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ws, nil
}

func handshake(conn net.Conn, host string, timeout time.Duration) (*Conn, error) {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	request := "GET / HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return nil, fmt.Errorf("reading handshake response: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("unexpected handshake status %d", resp.StatusCode)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("handshake response is missing Upgrade: websocket")
	}
	if resp.Header.Get("Sec-WebSocket-Accept") != acceptKey(key) {
		return nil, errors.New("invalid Sec-WebSocket-Accept")
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	return &Conn{conn: conn, reader: reader}, nil
}

func acceptKey(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// ReadMessage returns the next complete text or binary message.
// Ping frames are answered automatically.
func (c *Conn) ReadMessage() ([]byte, error) {
	var message []byte
	inMessage := false
	for {
		fin, opcode, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case opPing:
			if err := c.writeFrame(opPong, payload); err != nil {
				return nil, err
			}
		case opPong:
			// Unsolicited pongs are ignored.
		case opClose:
			_ = c.writeFrame(opClose, payload)
			return nil, ErrClosed
		case opText, opBinary:
			if inMessage {
				return nil, errors.New("new data frame inside a fragmented message")
			}
			message = append(message[:0], payload...)
			inMessage = true
			if fin {
				return message, nil
			}
		case opContinuation:
			if !inMessage {
				return nil, errors.New("continuation frame without a message")
			}
			if len(message)+len(payload) > maxMessageSize {
				return nil, errors.New("message too large")
			}
			message = append(message, payload...)
			if fin {
				return message, nil
			}
		default:
			return nil, fmt.Errorf("unsupported opcode %d", opcode)
		}
	}
}

func (c *Conn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	var header [2]byte
	if _, err = io.ReadFull(c.reader, header[:]); err != nil {
		return
	}
	fin = header[0]&0x80 != 0
	opcode = header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.reader, ext[:]); err != nil {
			return
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(c.reader, ext[:]); err != nil {
			return
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > maxMessageSize {
		err = errors.New("frame too large")
		return
	}

	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.reader, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(c.reader, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}

// WriteText sends a single text message. Safe for concurrent use.
func (c *Conn) WriteText(data []byte) error {
	return c.writeFrame(opText, data)
}

func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	frame := make([]byte, 0, len(payload)+14)
	frame = append(frame, 0x80|opcode)

	length := len(payload)
	switch {
	case length < 126:
		frame = append(frame, 0x80|byte(length))
	case length <= 0xFFFF:
		frame = append(frame, 0x80|126)
		frame = binary.BigEndian.AppendUint16(frame, uint16(length))
	default:
		frame = append(frame, 0x80|127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(length))
	}

	// Client frames must be masked.
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	frame = append(frame, mask[:]...)
	start := len(frame)
	frame = append(frame, payload...)
	for i := range payload {
		frame[start+i] ^= mask[i%4]
	}

	_, err := c.conn.Write(frame)
	return err
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	return c.conn.Close()
}
