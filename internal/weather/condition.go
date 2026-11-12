package weather

// Condition groups WMO weather codes into what the icons can show.
type Condition int

const (
	Unknown Condition = iota
	Clear
	PartlyCloudy
	Cloudy
	Fog
	Drizzle
	Rain
	Snow
	Thunderstorm
)

// ConditionFromCode maps a WMO weather interpretation code (as returned by Open-Meteo).
func ConditionFromCode(code int) Condition {
	switch {
	case code == 0:
		return Clear
	case code == 1 || code == 2:
		return PartlyCloudy
	case code == 3:
		return Cloudy
	case code == 45 || code == 48:
		return Fog
	case code >= 51 && code <= 57:
		return Drizzle
	case (code >= 61 && code <= 67) || (code >= 80 && code <= 82):
		return Rain
	case (code >= 71 && code <= 77) || code == 85 || code == 86:
		return Snow
	case code >= 95 && code <= 99:
		return Thunderstorm
	default:
		return Unknown
	}
}

// Label returns a short Spanish description.
func (c Condition) Label() string {
	switch c {
	case Clear:
		return "Despejado"
	case PartlyCloudy:
		return "Algo nublado"
	case Cloudy:
		return "Nublado"
	case Fog:
		return "Niebla"
	case Drizzle:
		return "Llovizna"
	case Rain:
		return "Lluvia"
	case Snow:
		return "Nieve"
	case Thunderstorm:
		return "Tormenta"
	default:
		return "Sin datos"
	}
}
