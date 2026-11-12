// Package streamdock implements the plugin side of the StreamDock (Stream Deck SDK v1) protocol.
package streamdock

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event is an incoming message from StreamDock.
type Event struct {
	Event   string          `json:"event"`
	Action  string          `json:"action"`
	Context string          `json:"context"`
	Device  string          `json:"device"`
	Payload json.RawMessage `json:"payload"`
}

// DialPayload is the payload of dialRotate, dialDown and dialUp.
type DialPayload struct {
	Controller string          `json:"controller"`
	Settings   json.RawMessage `json:"settings"`
	Ticks      int             `json:"ticks"`
	Pressed    bool            `json:"pressed"`
}

// Client is a registered plugin connection.
type Client struct {
	conn *Conn
}

// Connect opens the socket on the given port and registers the plugin.
func Connect(port int, pluginUUID, registerEvent string) (*Client, error) {
	conn, err := Dial(fmt.Sprintf("127.0.0.1:%d", port), 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to StreamDock: %w", err)
	}
	c := &Client{conn: conn}
	if err := c.send(map[string]any{"event": registerEvent, "uuid": pluginUUID}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("registering plugin: %w", err)
	}
	return c, nil
}

// Next blocks until the next event arrives. Unparseable messages are skipped.
func (c *Client) Next() (Event, error) {
	for {
		data, err := c.conn.ReadMessage()
		if err != nil {
			return Event{}, err
		}
		var ev Event
		if json.Unmarshal(data, &ev) == nil && ev.Event != "" {
			return ev, nil
		}
	}
}

// SetImage sets the image of a key or knob screen. image must be a data URI.
func (c *Client) SetImage(context, image string) error {
	return c.send(map[string]any{
		"event":   "setImage",
		"context": context,
		"payload": map[string]any{"image": image, "target": 0},
	})
}

// SettingsPayload is the payload shape of willAppear and didReceiveSettings.
type SettingsPayload struct {
	Settings   json.RawMessage `json:"settings"`
	Controller string          `json:"controller"`
}

// SendToPropertyInspector delivers a payload to the settings panel of one action instance.
func (c *Client) SendToPropertyInspector(action, context string, payload any) error {
	return c.send(map[string]any{
		"event":   "sendToPropertyInspector",
		"action":  action,
		"context": context,
		"payload": payload,
	})
}

// LogMessage writes to the StreamDock debug window.
func (c *Client) LogMessage(message string) error {
	return c.send(map[string]any{
		"event":   "logMessage",
		"payload": map[string]any{"message": message},
	})
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.conn.WriteText(data)
}
