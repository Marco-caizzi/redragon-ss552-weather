package render

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/weather"
)

func fixture(t *testing.T) *weather.Forecast {
	t.Helper()
	data, err := os.ReadFile("../../testdata/cordoba_forecast.json")
	if err != nil {
		t.Fatal(err)
	}
	f, err := weather.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// 15:20 in Córdoba on the fixture day.
var fixtureNow = time.Date(2026, 11, 11, 18, 20, 0, 0, time.UTC)

func screens(t *testing.T) map[string]string {
	f := fixture(t)
	base := View{Forecast: f, Now: fixtureNow, City: "Córdoba, Argentina"}
	with := func(mutate func(*View)) View {
		v := base
		mutate(&v)
		return v
	}
	stormy := *f
	stormy.Current.Code, stormy.Current.Temp, stormy.Current.IsDay = 95, 24.6, false

	out := map[string]string{
		"key_1_now":         KeySVG(base),
		"key_2_hours":       KeySVG(with(func(v *View) { v.Mode = ModeHours })),
		"key_3_days":        KeySVG(with(func(v *View) { v.Mode = ModeDays })),
		"key_4_storm_stale": KeySVG(View{Forecast: &stormy, Now: fixtureNow, City: "Córdoba, Argentina", Stale: true}),
		"key_5_unconfig":    KeyStatusSVG(StatusUnconfigured),
		"key_6_offline":     KeyStatusSVG(StatusOffline),
		"knob_1_now":        KnobSVG(base),
		"knob_2_hours":      KnobSVG(with(func(v *View) { v.Mode = ModeHours; v.Offset = 7 })),
		"knob_3_days":       KnobSVG(with(func(v *View) { v.Mode = ModeDays; v.Offset = 1 })),
		"knob_4_loading":    KnobStatusSVG(StatusLoading),
	}
	// One icon per condition, for visual review of the set.
	for _, code := range []int{0, 2, 3, 45, 53, 63, 75, 95} {
		c := newCanvas(KeySize, KeySize)
		c.raw(icon(weather.ConditionFromCode(code), true, 8, 8, 128))
		out["icon_"+weather.ConditionFromCode(code).Label()] = c.String()
	}
	return out
}

func TestScreensAreWellFormedSVG(t *testing.T) {
	for name, svg := range screens(t) {
		dec := xml.NewDecoder(strings.NewReader(svg))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: invalid SVG: %v", name, err)
			}
		}
	}
}

func TestKeyNowContent(t *testing.T) {
	svg := KeySVG(View{Forecast: fixture(t), Now: fixtureNow, City: "Córdoba, Argentina"})
	for _, want := range []string{">30°<", ">Despejado<", ">↓15°<", ">↑30°<", ">2%<", ">Córdoba<"} {
		if !strings.Contains(svg, want) {
			t.Errorf("key now is missing %q", want)
		}
	}
}

func TestKeyHoursStartAfterCurrentHour(t *testing.T) {
	svg := KeySVG(View{Forecast: fixture(t), Now: fixtureNow, Mode: ModeHours})
	for _, want := range []string{">16h<", ">17h<", ">18h<"} {
		if !strings.Contains(svg, want) {
			t.Errorf("hours view is missing %q", want)
		}
	}
	if strings.Contains(svg, ">15h<") {
		t.Error("hours view must start after the current hour")
	}
}

func TestKnobHoursCrossMidnightAndClamp(t *testing.T) {
	f := fixture(t)
	// Offset 7 starts at 23h; the next column belongs to tomorrow.
	svg := KnobSVG(View{Forecast: f, Now: fixtureNow, Mode: ModeHours, Offset: 7})
	if !strings.Contains(svg, ">23h<") || !strings.Contains(svg, ">Jue 00h<") {
		t.Fatalf("expected 23h and Jue 00h columns in %s", svg)
	}
	// An out-of-range offset is clamped instead of panicking.
	if max := MaxOffset(f, fixtureNow, ModeHours); max != 20 {
		t.Fatalf("max hours offset = %d", max)
	}
	_ = KnobSVG(View{Forecast: f, Now: fixtureNow, Mode: ModeHours, Offset: 999})
	if max := MaxOffset(f, fixtureNow, ModeDays); max != 3 {
		t.Fatalf("max days offset = %d", max)
	}
}

func TestDayLabels(t *testing.T) {
	svg := KeySVG(View{Forecast: fixture(t), Now: fixtureNow, Mode: ModeDays})
	for _, want := range []string{">Hoy<", ">Jue<", ">Vie<", ">35°<", ">75%<"} {
		if !strings.Contains(svg, want) {
			t.Errorf("days view is missing %q", want)
		}
	}
}

func TestTempFormatting(t *testing.T) {
	cases := map[float64]string{29.8: "30°", -0.4: "0°", -3.6: "-4°", 0: "0°"}
	for in, want := range cases {
		if got := temp(in); got != want {
			t.Errorf("temp(%v) = %q, want %q", in, got, want)
		}
	}
}

// Set RENDER_PREVIEW_DIR to write every screen as an SVG for visual review.
func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("RENDER_PREVIEW_DIR")
	if dir == "" {
		t.Skip("RENDER_PREVIEW_DIR not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, svg := range screens(t) {
		if err := os.WriteFile(filepath.Join(dir, name+".svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
