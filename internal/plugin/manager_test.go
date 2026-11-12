package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/weather"
)

type harness struct {
	t       *testing.T
	now     time.Time
	drawn   map[string]string
	fetches []weather.Query
	m       *Manager
	fc      *weather.Forecast
}

func newHarness(t *testing.T) *harness {
	data, err := os.ReadFile("../../testdata/cordoba_forecast.json")
	if err != nil {
		t.Fatal(err)
	}
	fc, err := weather.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		t:     t,
		now:   time.Date(2026, 11, 11, 18, 20, 0, 0, time.UTC), // 15:20 in Córdoba
		drawn: map[string]string{},
		fc:    fc,
	}
	h.m = NewManager(Callbacks{
		Draw:  func(ctx, svg string) { h.drawn[ctx] = svg },
		Fetch: func(q weather.Query) { h.fetches = append(h.fetches, q) },
	}, func() time.Time { return h.now })
	return h
}

func (h *harness) advance(d time.Duration) {
	h.now = h.now.Add(d)
	h.m.Tick()
}

func (h *harness) expect(ctx string, wants ...string) {
	h.t.Helper()
	for _, want := range wants {
		if !strings.Contains(h.drawn[ctx], want) {
			h.t.Fatalf("%s: %q not drawn", ctx, want)
		}
	}
}

var cordoba = ParseSettings(json.RawMessage(`{"city":"Córdoba, Argentina","latitude":-31.40648,"longitude":-64.18853,"units":"c","refresh":15}`))

func TestParseSettings(t *testing.T) {
	if !cordoba.Configured || cordoba.City != "Córdoba, Argentina" || cordoba.Refresh != 15*time.Minute || cordoba.Fahrenheit {
		t.Fatalf("settings = %+v", cordoba)
	}
	if q := cordoba.Query(); q.Latitude != -31.4065 || q.Longitude != -64.1885 {
		t.Fatalf("query = %+v", q)
	}
	s := ParseSettings(json.RawMessage(`{"latitude":"-31,4","longitude":"-64.2","units":"F","refresh":"1"}`))
	if !s.Configured || s.Latitude != -31.4 || !s.Fahrenheit || s.Refresh != 5*time.Minute || s.City == "" {
		t.Fatalf("string settings = %+v", s)
	}
	for _, raw := range []string{``, `{}`, `{"latitude":0,"longitude":0}`, `{"latitude":95,"longitude":10}`, `not json`} {
		if ParseSettings(json.RawMessage(raw)).Configured {
			t.Errorf("%q should not be configured", raw)
		}
	}
}

func TestUnconfiguredKeyAsksForCity(t *testing.T) {
	h := newHarness(t)
	h.m.Appear("k1", Key, Settings{Refresh: defaultRefresh})
	h.expect("k1", "Elegí una ciudad")
	if len(h.fetches) != 0 {
		t.Fatal("unconfigured key must not fetch")
	}
}

func TestLoadThenShowAndShareCache(t *testing.T) {
	h := newHarness(t)
	h.m.Appear("k1", Key, cordoba)
	h.m.Appear("n1", Knob, cordoba)
	h.expect("k1", "Cargando")
	if len(h.fetches) != 1 {
		t.Fatalf("fetches = %d, want 1 shared request", len(h.fetches))
	}
	h.m.FetchDone(h.fetches[0], h.fc, nil)
	h.expect("k1", ">30°<", ">Córdoba<")
	h.expect("n1", ">30°<", ">Sens. 33°<")
}

func TestRefreshInterval(t *testing.T) {
	h := newHarness(t)
	h.m.Appear("k1", Key, cordoba)
	h.m.FetchDone(h.fetches[0], h.fc, nil)
	h.advance(14 * time.Minute)
	if len(h.fetches) != 1 {
		t.Fatal("refreshed too early")
	}
	h.advance(1 * time.Minute)
	if len(h.fetches) != 2 {
		t.Fatalf("fetches = %d, want refresh after 15 min", len(h.fetches))
	}
	// While the request is in flight no duplicate is started.
	h.advance(time.Minute)
	if len(h.fetches) != 2 {
		t.Fatal("duplicate request while in flight")
	}
}

func TestFailureBackoffStaleAndRetryOnPress(t *testing.T) {
	h := newHarness(t)
	h.m.Appear("k1", Key, cordoba)
	h.m.FetchDone(h.fetches[0], nil, errors.New("no network"))
	h.expect("k1", "Sin conexión")

	h.advance(20 * time.Second)
	if len(h.fetches) != 1 {
		t.Fatal("retried before the 30 s backoff")
	}
	h.advance(10 * time.Second)
	if len(h.fetches) != 2 {
		t.Fatal("did not retry after backoff")
	}
	h.m.FetchDone(h.fetches[1], nil, errors.New("no network"))
	// Second failure waits 60 s, but a press retries immediately.
	h.m.KeyDown("k1")
	if len(h.fetches) != 3 {
		t.Fatal("press did not retry")
	}
	h.expect("k1", "Cargando")

	h.m.FetchDone(h.fetches[2], h.fc, nil)
	h.expect("k1", ">30°<")
	if strings.Contains(h.drawn["k1"], "#f59e0b") {
		t.Fatal("fresh data must not show the stale dot")
	}
	// A failed refresh keeps the old data on screen and marks it stale.
	h.advance(15 * time.Minute)
	h.m.FetchDone(h.fetches[3], nil, errors.New("timeout"))
	h.expect("k1", ">30°<", `fill="#f59e0b"`)
}

func TestKeyPressCyclesViewsAndReturns(t *testing.T) {
	h := newHarness(t)
	h.m.Appear("k1", Key, cordoba)
	h.m.FetchDone(h.fetches[0], h.fc, nil)

	h.m.KeyDown("k1")
	h.expect("k1", "Próximas horas", ">16h<")
	h.m.KeyDown("k1")
	h.expect("k1", "Próximos días", ">Hoy<")
	h.m.KeyDown("k1")
	h.expect("k1", ">Despejado<")

	h.m.KeyDown("k1")
	h.advance(29 * time.Second)
	h.expect("k1", "Próximas horas")
	h.advance(1 * time.Second)
	h.expect("k1", ">Despejado<")
}

func TestKnobRotateScrollsAndClamps(t *testing.T) {
	h := newHarness(t)
	h.m.Appear("n1", Knob, cordoba)
	h.m.FetchDone(h.fetches[0], h.fc, nil)

	h.m.DialRotate("n1", -1) // Left from "now" stays on "now".
	h.expect("n1", ">Despejado<")
	h.m.DialRotate("n1", 1)
	h.expect("n1", ">16h<", ">19h<")
	h.m.DialRotate("n1", 7)
	h.expect("n1", ">23h<", ">Jue 00h<")
	h.m.DialRotate("n1", 100) // Clamped at the end of the 24 h window.
	h.expect("n1", ">Jue 12h<", ">Jue 15h<")
	h.m.DialRotate("n1", -100) // Past the start returns to "now".
	h.expect("n1", ">Despejado<")

	h.m.DialDown("n1")
	h.m.DialDown("n1")
	h.expect("n1", ">Hoy<", ">Sáb<")
	h.m.DialRotate("n1", 9)
	h.expect("n1", ">Sáb<", ">Mar<")

	h.advance(20 * time.Second)
	h.expect("n1", ">Despejado<")
}

func TestSettingsChangeRefetchesAndDropsOldCache(t *testing.T) {
	h := newHarness(t)
	h.m.Appear("k1", Key, cordoba)
	h.m.FetchDone(h.fetches[0], h.fc, nil)
	madrid := ParseSettings(json.RawMessage(`{"city":"Madrid, España","latitude":40.4165,"longitude":-3.7026}`))
	h.m.SettingsChanged("k1", madrid)
	if len(h.fetches) != 2 || h.fetches[1] != madrid.Query() {
		t.Fatalf("fetches = %+v", h.fetches)
	}
	h.expect("k1", "Cargando")
	if _, ok := h.m.cache[cordoba.Query()]; ok {
		t.Fatal("unused cache entry was not dropped")
	}
	// A late result for the old city is ignored.
	h.m.FetchDone(cordoba.Query(), h.fc, nil)
	h.expect("k1", "Cargando")
}

func TestDisappearedInstanceIgnoresEvents(t *testing.T) {
	h := newHarness(t)
	h.m.Appear("k1", Key, cordoba)
	h.m.Disappear("k1")
	h.m.KeyDown("k1")
	h.m.DialRotate("k1", 1)
	h.m.SettingsChanged("k1", cordoba)
	h.advance(time.Hour)
	if len(h.fetches) != 1 {
		t.Fatalf("fetches after disappear = %d", len(h.fetches))
	}
}
