package render

import (
	"fmt"
	"math"
	"time"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/weather"
)

// Sizes of the key and knob screens on the SS-552.
const (
	KeySize    = 144
	KnobWidth  = 320
	KnobHeight = 130
)

// Mode is what a key or knob is showing.
type Mode int

const (
	ModeNow Mode = iota
	ModeHours
	ModeDays
	modeCount
)

// Next cycles Now → Hours → Days → Now.
func (m Mode) Next() Mode { return (m + 1) % modeCount }

// Status is a screen shown when there is no forecast to draw.
type Status int

const (
	StatusUnconfigured Status = iota
	StatusLoading
	StatusOffline
)

// View is everything needed to draw one screen with forecast data.
type View struct {
	Forecast *weather.Forecast
	Now      time.Time
	City     string
	Mode     Mode
	// Offset scrolls the knob's hourly or daily list.
	Offset int
	// Stale means the last refresh failed or the data is old.
	Stale bool
}

const (
	knobColumns = 4
	keyColumns  = 3
	hourWindow  = 24
)

// upcomingHours starts at the hour after now.
func upcomingHours(f *weather.Forecast, now time.Time, n int) []weather.Hour {
	return f.HoursFrom(now.Add(time.Hour), n)
}

// MaxOffset returns how far the knob list for mode can scroll.
func MaxOffset(f *weather.Forecast, now time.Time, mode Mode) int {
	if f == nil {
		return 0
	}
	var total int
	switch mode {
	case ModeHours:
		total = len(upcomingHours(f, now, hourWindow))
	case ModeDays:
		total = len(f.DaysFrom(now, 7))
	default:
		return 0
	}
	return max(0, total-knobColumns)
}

func rainColor(prob int) string {
	if prob >= 30 {
		return colorRain
	}
	return colorSubtle
}

func rainText(prob int) string {
	if prob < 0 {
		return "--"
	}
	return fmt.Sprintf("%d%%", prob)
}

// pageDots shows which of the three modes is active.
func pageDots(c *canvas, mode Mode, y float64) {
	cx := float64(c.width) / 2
	for i := 0; i < int(modeCount); i++ {
		x := cx + float64(i-1)*10
		color := colorDim
		if Mode(i) == mode {
			color = colorText
		}
		c.raw(fmt.Sprintf(`<circle cx="%s" cy="%s" r="2.5" fill="%s"/>`, num(x), num(y), color))
	}
}

// KeySVG renders the 144x144 key.
func KeySVG(v View) string {
	c := newCanvas(KeySize, KeySize)
	switch v.Mode {
	case ModeHours:
		keyHours(c, v)
	case ModeDays:
		keyDays(c, v)
	default:
		keyNow(c, v)
	}
	if v.Stale {
		c.staleDot()
	}
	return c.String()
}

func keyNow(c *canvas, v View) {
	f := v.Forecast
	cur := f.Current
	c.raw(icon(weather.ConditionFromCode(cur.Code), cur.IsDay, 6, 8, 58))
	c.text(104, 48, 36, 800, colorText, "middle", temp(cur.Temp))

	today, hasToday := f.Today(v.Now)
	if hasToday && today.RainProb >= 0 {
		c.drop(88, 59, 13)
		c.text(98, 72, 14, 600, rainColor(today.RainProb), "start", rainText(today.RainProb))
	}
	c.text(72, 92, 14, 600, colorSubtle, "middle", shorten(weather.ConditionFromCode(cur.Code).Label(), 16))
	if hasToday {
		c.text(44, 114, 17, 700, colorMin, "middle", "↓"+temp(today.Min))
		c.text(100, 114, 17, 700, colorMax, "middle", "↑"+temp(today.Max))
	}
	c.text(72, 134, 12, 600, colorSubtle, "middle", shorten(cityName(v.City), 17))
}

func keyHours(c *canvas, v View) {
	c.text(72, 16, 12, 700, colorSubtle, "middle", "Próximas horas")
	hours := upcomingHours(v.Forecast, v.Now, keyColumns)
	for i, h := range hours {
		cx := 26 + float64(i)*46
		c.text(cx, 36, 13, 700, colorText, "middle", fmt.Sprintf("%02dh", h.Time.Hour()))
		c.raw(icon(weather.ConditionFromCode(h.Code), h.IsDay, cx-18, 42, 36))
		c.text(cx, 98, 18, 800, colorText, "middle", temp(h.Temp))
		c.text(cx, 118, 12, 600, rainColor(h.RainProb), "middle", rainText(h.RainProb))
	}
	pageDots(c, ModeHours, 136)
}

func keyDays(c *canvas, v View) {
	c.text(72, 16, 12, 700, colorSubtle, "middle", "Próximos días")
	days := v.Forecast.DaysFrom(v.Now, keyColumns)
	for i, d := range days {
		cx := 26 + float64(i)*46
		c.text(cx, 36, 13, 700, colorText, "middle", dayLabel(d.Date, v.Now))
		c.raw(icon(weather.ConditionFromCode(d.Code), true, cx-18, 40, 36))
		c.text(cx, 94, 17, 800, colorMax, "middle", temp(d.Max))
		c.text(cx, 111, 14, 700, colorMin, "middle", temp(d.Min))
		c.text(cx, 127, 11, 600, rainColor(d.RainProb), "middle", rainText(d.RainProb))
	}
	pageDots(c, ModeDays, 139)
}

// KnobSVG renders the 320x130 knob screen.
func KnobSVG(v View) string {
	c := newCanvas(KnobWidth, KnobHeight)
	switch v.Mode {
	case ModeHours:
		knobHours(c, v)
	case ModeDays:
		knobDays(c, v)
	default:
		knobNow(c, v)
	}
	if v.Stale {
		c.staleDot()
	}
	return c.String()
}

func knobNow(c *canvas, v View) {
	f := v.Forecast
	cur := f.Current
	cond := weather.ConditionFromCode(cur.Code)
	c.raw(icon(cond, cur.IsDay, 10, 10, 78))
	c.text(146, 60, 46, 800, colorText, "middle", temp(cur.Temp))
	c.text(146, 88, 15, 600, colorSubtle, "middle", shorten(cond.Label(), 16))

	const x = 214
	if today, ok := f.Today(v.Now); ok {
		c.raw(fmt.Sprintf(`<text x="%d" y="30" font-family="%s" font-size="17" font-weight="700"><tspan fill="%s">↓%s</tspan><tspan fill="%s" dx="10">↑%s</tspan></text>`,
			x, fontFamily, colorMin, escape(temp(today.Min)), colorMax, escape(temp(today.Max))))
		if today.RainProb >= 0 {
			c.drop(x+5, 42, 13)
			c.text(x+16, 55, 14, 600, rainColor(today.RainProb), "start", "Lluvia "+rainText(today.RainProb))
		}
	}
	c.text(x, 78, 14, 600, colorSubtle, "start", "Sens. "+temp(cur.FeelsLike))
	if cur.WindKmh >= 0 {
		c.text(x, 100, 14, 600, colorSubtle, "start", fmt.Sprintf("Viento %.0f km/h", cur.WindKmh))
	}
	footer := shorten(cityName(v.City), 22)
	if !cur.Time.IsZero() {
		footer += " · " + cur.Time.Format("15:04")
	}
	c.text(12, 122, 12, 600, colorDim, "start", footer)
}

// knobColumn draws one forecast column centered at cx.
func knobColumn(c *canvas, cx float64, label string, labelSize int, cond weather.Condition, isDay bool) {
	c.text(cx, 20, labelSize, 700, colorText, "middle", label)
	c.raw(icon(cond, isDay, cx-21, 24, 42))
}

func knobHours(c *canvas, v View) {
	hours := upcomingHours(v.Forecast, v.Now, hourWindow)
	offset := clamp(v.Offset, 0, max(0, len(hours)-knobColumns))
	end := min(offset+knobColumns, len(hours))
	for i, h := range hours[offset:end] {
		cx := 40 + float64(i)*80
		label, size := fmt.Sprintf("%02dh", h.Time.Hour()), 15
		if !sameDay(h.Time, v.Now) {
			label, size = dayLabel(h.Time, v.Now)+" "+label, 13
		}
		knobColumn(c, cx, label, size, weather.ConditionFromCode(h.Code), h.IsDay)
		c.text(cx, 88, 22, 800, colorText, "middle", temp(h.Temp))
		c.text(cx, 107, 13, 600, rainColor(h.RainProb), "middle", rainText(h.RainProb))
	}
	scrollBar(c, offset, len(hours))
}

func knobDays(c *canvas, v View) {
	days := v.Forecast.DaysFrom(v.Now, 7)
	offset := clamp(v.Offset, 0, max(0, len(days)-knobColumns))
	end := min(offset+knobColumns, len(days))
	for i, d := range days[offset:end] {
		cx := 40 + float64(i)*80
		knobColumn(c, cx, dayLabel(d.Date, v.Now), 15, weather.ConditionFromCode(d.Code), true)
		c.text(cx, 86, 20, 800, colorMax, "middle", temp(d.Max))
		c.text(cx, 104, 15, 700, colorMin, "middle", temp(d.Min))
	}
	for i, d := range days[offset:end] {
		cx := 40 + float64(i)*80
		if d.RainProb >= 0 {
			c.text(cx, 119, 12, 600, rainColor(d.RainProb), "middle", rainText(d.RainProb))
		}
	}
	scrollBar(c, offset, len(days))
}

// scrollBar shows the visible window within the full list.
func scrollBar(c *canvas, offset, total int) {
	if total <= knobColumns {
		return
	}
	const x0, width, y = 20.0, 280.0, 125.0
	thumb := math.Max(24, width*float64(knobColumns)/float64(total))
	pos := x0 + (width-thumb)*float64(offset)/float64(total-knobColumns)
	c.raw(fmt.Sprintf(`<rect x="%s" y="%s" width="%s" height="3" rx="1.5" fill="%s"/>`, num(x0), num(y), num(width), "#1f2937"))
	c.raw(fmt.Sprintf(`<rect x="%s" y="%s" width="%s" height="3" rx="1.5" fill="%s"/>`, num(pos), num(y), num(thumb), colorSubtle))
}

func statusText(s Status) (string, string) {
	switch s {
	case StatusUnconfigured:
		return "Elegí una ciudad", "en la configuración"
	case StatusOffline:
		return "Sin conexión", "pulsá para reintentar"
	default:
		return "Cargando...", "Open-Meteo"
	}
}

// KeyStatusSVG renders a status message on the key.
func KeyStatusSVG(s Status) string {
	c := newCanvas(KeySize, KeySize)
	c.raw(icon(weather.Cloudy, true, 40, 12, 64))
	title, sub := statusText(s)
	c.text(72, 100, 15, 800, colorText, "middle", title)
	c.text(72, 120, 11, 600, colorSubtle, "middle", sub)
	return c.String()
}

// KnobStatusSVG renders a status message on the knob screen.
func KnobStatusSVG(s Status) string {
	c := newCanvas(KnobWidth, KnobHeight)
	c.raw(icon(weather.Cloudy, true, 24, 24, 80))
	title, sub := statusText(s)
	c.text(206, 62, 22, 800, colorText, "middle", title)
	c.text(206, 86, 14, 600, colorSubtle, "middle", sub)
	return c.String()
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
