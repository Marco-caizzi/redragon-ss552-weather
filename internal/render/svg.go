// Package render draws the weather key and knob screens as SVG.
package render

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	colorBackground = "#000000"
	colorText       = "#ffffff"
	colorSubtle     = "#9ca3af"
	colorDim        = "#4b5563"
	colorMin        = "#60a5fa"
	colorMax        = "#fb923c"
	colorRain       = "#38bdf8"
	colorStale      = "#f59e0b"
	fontFamily      = "Segoe UI, Arial, sans-serif"
)

type canvas struct {
	b      strings.Builder
	width  int
	height int
}

func newCanvas(width, height int) *canvas {
	c := &canvas{width: width, height: height}
	fmt.Fprintf(&c.b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&c.b, `<rect width="%d" height="%d" fill="%s"/>`, width, height, colorBackground)
	return c
}

func (c *canvas) String() string {
	return c.b.String() + `</svg>`
}

// text draws a single line. anchor is "start", "middle" or "end".
func (c *canvas) text(x, y float64, size, weight int, color, anchor, value string) {
	fmt.Fprintf(&c.b, `<text x="%s" y="%s" text-anchor="%s" font-family="%s" font-size="%d" font-weight="%d" fill="%s">%s</text>`,
		num(x), num(y), anchor, fontFamily, size, weight, color, escape(value))
}

func (c *canvas) raw(s string) { c.b.WriteString(s) }

// staleDot marks data that could not be refreshed recently.
func (c *canvas) staleDot() {
	fmt.Fprintf(&c.b, `<circle cx="%d" cy="9" r="5" fill="%s"/>`, c.width-9, colorStale)
}

// drop draws a small rain drop whose tip is at (x, y).
func (c *canvas) drop(x, y, size float64) {
	s := size / 12
	fmt.Fprintf(&c.b, `<path transform="translate(%s %s) scale(%s)" d="M0 0 C3 4 5 6.5 5 8.5 A5 5 0 0 1 -5 8.5 C-5 6.5 -3 4 0 0Z" fill="%s"/>`,
		num(x), num(y), num(s), colorRain)
}

func num(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

func shorten(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// cityName keeps only the place name from labels like "Córdoba, Argentina".
func cityName(label string) string {
	if i := strings.IndexByte(label, ','); i > 0 {
		return strings.TrimSpace(label[:i])
	}
	return strings.TrimSpace(label)
}

// temp formats a temperature as "30°".
func temp(v float64) string {
	r := math.Round(v)
	if r == 0 {
		r = 0 // Avoid "-0°".
	}
	return fmt.Sprintf("%.0f°", r)
}

var weekdays = [...]string{"Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"}

// dayLabel returns "Hoy" for the day of now and a weekday abbreviation otherwise.
func dayLabel(day, now time.Time) string {
	now = now.In(day.Location())
	if day.Year() == now.Year() && day.YearDay() == now.YearDay() {
		return "Hoy"
	}
	return weekdays[day.Weekday()]
}

func sameDay(a, b time.Time) bool {
	b = b.In(a.Location())
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// DataURI encodes an SVG for setImage.
func DataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
