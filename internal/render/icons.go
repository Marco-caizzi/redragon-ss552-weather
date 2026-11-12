package render

import (
	"fmt"
	"strings"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/weather"
)

// Icons are drawn in a 64x64 space and scaled into place.

const (
	colorSun        = "#fbbf24"
	colorMoon       = "#e5e7eb"
	colorCloud      = "#d1d5db"
	colorCloudBack  = "#6b7280"
	colorStormCloud = "#9ca3af"
	colorSnow       = "#ffffff"
	colorBolt       = "#facc15"
)

func sun(cx, cy, r float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`, num(cx), num(cy), num(r), colorSun)
	inner, outer := r*1.45, r*2.05
	dirs := [][2]float64{{1, 0}, {0.7071, 0.7071}, {0, 1}, {-0.7071, 0.7071}, {-1, 0}, {-0.7071, -0.7071}, {0, -1}, {0.7071, -0.7071}}
	for _, d := range dirs {
		fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" stroke-linecap="round"/>`,
			num(cx+d[0]*inner), num(cy+d[1]*inner), num(cx+d[0]*outer), num(cy+d[1]*outer), colorSun, num(r*0.32))
	}
	return b.String()
}

// moon draws a crescent by covering part of a disc with the background color.
func moon(cx, cy, r float64) string {
	return fmt.Sprintf(`<circle cx="%s" cy="%s" r="%s" fill="%s"/><circle cx="%s" cy="%s" r="%s" fill="%s"/>`,
		num(cx), num(cy), num(r), colorMoon, num(cx+r*0.5), num(cy-r*0.4), num(r*0.82), colorBackground)
}

// cloud draws a cloud about 40 wide and 32 tall with its top-left near (dx+12, dy+16).
func cloud(dx, dy float64, color string) string {
	return fmt.Sprintf(`<g transform="translate(%s %s)" fill="%s"><rect x="12" y="34" width="40" height="15" rx="7.5"/><circle cx="23" cy="35" r="10"/><circle cx="36" cy="29" r="13"/><circle cx="46" cy="38" r="9"/></g>`,
		num(dx), num(dy), color)
}

func lines(y0, gap float64, count int, x1, x2 float64, color string, width float64) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		y := y0 + float64(i)*gap
		fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" stroke-linecap="round"/>`,
			num(x1), num(y), num(x2), num(y), color, num(width))
	}
	return b.String()
}

func streaks(length float64, color string, width float64) string {
	var b strings.Builder
	for _, x := range []float64{22, 32, 42} {
		fmt.Fprintf(&b, `<line x1="%s" y1="49" x2="%s" y2="%s" stroke="%s" stroke-width="%s" stroke-linecap="round"/>`,
			num(x+2), num(x-2), num(49+length), color, num(width))
	}
	return b.String()
}

func iconBody(c weather.Condition, isDay bool) string {
	celestial := func(cx, cy, r float64) string {
		if isDay {
			return sun(cx, cy, r)
		}
		return moon(cx, cy, r)
	}
	switch c {
	case weather.Clear:
		return celestial(32, 32, 13)
	case weather.PartlyCloudy:
		return celestial(22, 21, 9) + cloud(4, 4, colorCloud)
	case weather.Cloudy:
		return cloud(-7, -6, colorCloudBack) + cloud(3, 2, colorCloud)
	case weather.Fog:
		return cloud(0, -8, colorCloudBack) + lines(48, 7, 2, 12, 52, colorCloud, 4)
	case weather.Drizzle:
		return cloud(0, -8, colorCloud) + streaks(5, colorRain, 3.5)
	case weather.Rain:
		return cloud(0, -8, colorCloud) + streaks(11, colorRain, 3.5)
	case weather.Snow:
		var b strings.Builder
		b.WriteString(cloud(0, -8, colorCloud))
		for _, p := range [][2]float64{{22, 53}, {32, 58}, {42, 53}} {
			fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="3.2" fill="%s"/>`, num(p[0]), num(p[1]), colorSnow)
		}
		return b.String()
	case weather.Thunderstorm:
		return cloud(0, -8, colorStormCloud) + fmt.Sprintf(`<path d="M35 40 L25 54 H32 L28 64 L41 49 H34 L38 40Z" fill="%s"/>`, colorBolt)
	default:
		return fmt.Sprintf(`<text x="32" y="44" text-anchor="middle" font-family="%s" font-size="34" font-weight="800" fill="%s">?</text>`, fontFamily, colorSubtle)
	}
}

// AppIconSVG is the plugin and category icon: a partly cloudy day on a rounded tile.
func AppIconSVG() string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128"><rect width="128" height="128" rx="24" fill="#0f172a"/>%s</svg>`,
		icon(weather.PartlyCloudy, true, 8, 6, 112))
}

// icon places a weather icon with its top-left corner at (x, y), size pixels square.
func icon(c weather.Condition, isDay bool, x, y, size float64) string {
	return fmt.Sprintf(`<g transform="translate(%s %s) scale(%s)">%s</g>`, num(x), num(y), num(size/64), iconBody(c, isDay))
}
