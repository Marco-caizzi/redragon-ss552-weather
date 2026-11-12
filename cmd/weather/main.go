// weather is a StreamDock plugin that shows Open-Meteo forecasts on keys and knobs.
//
// StreamDock launches it as:
//
//	weather.exe -port <n> -pluginUUID <id> -registerEvent <event> -info <json>
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/plugin"
	"github.com/Marco-caizzi/redragon-ss552-weather/internal/render"
	"github.com/Marco-caizzi/redragon-ss552-weather/internal/streamdock"
	"github.com/Marco-caizzi/redragon-ss552-weather/internal/weather"
)

// Action UUIDs must match manifest.json.
const (
	KeyAction  = "com.marco.weather.key"
	KnobAction = "com.marco.weather.knob"
)

const (
	version      = "1.0.1"
	tickInterval = time.Second
	maxLogSize   = 1 << 20
	searchLimit  = 10 * time.Second
	fetchLimit   = 20 * time.Second
)

func main() {
	logger := newLogger()
	args := parseArgs(os.Args[1:])
	port, _ := strconv.Atoi(args["port"])
	pluginUUID, registerEvent := args["pluginUUID"], args["registerEvent"]

	if port == 0 || pluginUUID == "" || registerEvent == "" {
		logger.Printf("missing launch arguments: %v", os.Args[1:])
		fmt.Fprintln(os.Stderr, "this program is started by StreamDock")
		os.Exit(2)
	}
	api := &weather.Client{UserAgent: "redragon-ss552-weather/" + version}
	if err := run(port, pluginUUID, registerEvent, api, logger); err != nil {
		logger.Printf("plugin stopped: %v", err)
		os.Exit(1)
	}
}

// app wires StreamDock events, the manager and network calls. All state changes run on the loop goroutine.
type app struct {
	client  *streamdock.Client
	api     *weather.Client
	manager *plugin.Manager
	logger  *log.Logger
	// post schedules work from background goroutines onto the loop.
	post chan func()
}

func run(port int, pluginUUID, registerEvent string, api *weather.Client, logger *log.Logger) error {
	client, err := streamdock.Connect(port, pluginUUID, registerEvent)
	if err != nil {
		return err
	}
	defer client.Close()
	logger.Printf("weather plugin %s connected on port %d", version, port)

	a := &app{
		client: client,
		api:    api,
		logger: logger,
		post:   make(chan func(), 32),
	}
	a.manager = plugin.NewManager(plugin.Callbacks{
		Draw:  a.draw,
		Fetch: a.fetch,
		Logf:  logger.Printf,
	}, nil)

	events := make(chan streamdock.Event)
	readErr := make(chan error, 1)
	go func() {
		for {
			ev, err := client.Next()
			if err != nil {
				readErr <- err
				return
			}
			events <- ev
		}
	}()

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case ev := <-events:
			a.handle(ev)
		case fn := <-a.post:
			fn()
		case <-ticker.C:
			a.manager.Tick()
		case err := <-readErr:
			return err
		}
	}
}

func (a *app) draw(context, svg string) {
	if err := a.client.SetImage(context, render.DataURI(svg)); err != nil {
		a.logger.Printf("setImage failed: %v", err)
	}
}

// fetch downloads in the background and hands the result back to the loop.
func (a *app) fetch(q weather.Query) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchLimit)
		defer cancel()
		f, err := a.api.Fetch(ctx, q)
		a.post <- func() { a.manager.FetchDone(q, f, err) }
	}()
}

func kindOf(action string) (plugin.Kind, bool) {
	switch action {
	case KeyAction:
		return plugin.Key, true
	case KnobAction:
		return plugin.Knob, true
	default:
		return 0, false
	}
}

func (a *app) handle(ev streamdock.Event) {
	if ev.Event == "sendToPlugin" {
		// Some StreamDock versions omit "action" on messages from the settings panel.
		a.handleInspector(ev)
		return
	}
	kind, ours := kindOf(ev.Action)
	if !ours {
		return
	}
	switch ev.Event {
	case "willAppear":
		a.manager.Appear(ev.Context, kind, plugin.ParseSettings(settingsOf(ev.Payload)))
	case "willDisappear":
		a.manager.Disappear(ev.Context)
	case "didReceiveSettings":
		a.manager.SettingsChanged(ev.Context, plugin.ParseSettings(settingsOf(ev.Payload)))
	case "keyDown":
		a.manager.KeyDown(ev.Context)
	case "dialDown":
		a.manager.DialDown(ev.Context)
	case "dialRotate":
		var p streamdock.DialPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			a.logger.Printf("invalid dialRotate payload: %v", err)
			return
		}
		a.manager.DialRotate(ev.Context, p.Ticks)
	case "sendToPlugin":
		a.handleInspector(ev)
	}
}

func settingsOf(payload json.RawMessage) json.RawMessage {
	var p streamdock.SettingsPayload
	if json.Unmarshal(payload, &p) != nil {
		return nil
	}
	return p.Settings
}

// placeResult is what the settings panel receives for each search match.
type placeResult struct {
	Label     string  `json:"label"`
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// handleInspector answers city searches from the settings panel.
// Searching from Go avoids depending on CORS support inside the StreamDock web view.
func (a *app) handleInspector(ev streamdock.Event) {
	var req struct {
		Cmd   string `json:"cmd"`
		Query string `json:"query"`
		Lang  string `json:"lang"`
	}
	if json.Unmarshal(ev.Payload, &req) != nil || req.Cmd != "search" {
		a.logger.Printf("ignored sendToPlugin from %q: %s", ev.Context, ev.Payload)
		return
	}
	query := strings.TrimSpace(req.Query)
	action := ev.Action
	if action == "" {
		// Recover the action from the instance so the reply reaches the right panel.
		action = KeyAction
		if kind, ok := a.manager.KindOf(ev.Context); ok && kind == plugin.Knob {
			action = KnobAction
		}
	}
	a.logger.Printf("search %q requested by %s (%s)", query, ev.Context, action)
	reply := func(payload any) {
		if err := a.client.SendToPropertyInspector(action, ev.Context, payload); err != nil {
			a.logger.Printf("sendToPropertyInspector failed: %v", err)
		}
	}
	if len([]rune(query)) < 2 {
		reply(map[string]any{"cmd": "results", "query": query, "places": []placeResult{}})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), searchLimit)
		defer cancel()
		places, err := a.api.Search(ctx, query, req.Lang)
		a.post <- func() {
			if err != nil {
				a.logger.Printf("search %q failed: %v", query, err)
				reply(map[string]any{"cmd": "error", "query": query, "message": "No se pudo buscar. Revisá la conexión."})
				return
			}
			a.logger.Printf("search %q: %d result(s)", query, len(places))
			results := make([]placeResult, 0, len(places))
			for _, p := range places {
				results = append(results, placeResult{Label: p.Label(), City: p.ShortLabel(), Latitude: p.Latitude, Longitude: p.Longitude})
			}
			reply(map[string]any{"cmd": "results", "query": query, "places": results})
		}
	}()
}

// parseArgs reads "-name value", "--name value" and "-name=value" pairs, ignoring unknown names.
func parseArgs(argv []string) map[string]string {
	args := map[string]string{}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			args[name[:eq]] = name[eq+1:]
			continue
		}
		if i+1 < len(argv) && !isFlagName(argv[i+1]) {
			args[name] = argv[i+1]
			i++
		} else {
			args[name] = ""
		}
	}
	return args
}

func isFlagName(s string) bool {
	if !strings.HasPrefix(s, "-") || len(s) < 2 {
		return false
	}
	c := s[1]
	return c == '-' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// newLogger writes to weather.log next to the executable, starting fresh when it grows too large.
func newLogger() *log.Logger {
	var out io.Writer = io.Discard
	if exe, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(exe), "weather.log")
		flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
		if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
			flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		}
		if f, err := os.OpenFile(path, flags, 0o644); err == nil {
			out = f
		}
	}
	return log.New(out, "", log.LstdFlags)
}
