package plugin

import (
	"time"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/render"
	"github.com/Marco-caizzi/redragon-ss552-weather/internal/weather"
)

// Kind is the type of control an action instance lives on.
type Kind int

const (
	Key Kind = iota
	Knob
)

const (
	keyIdleReturn  = 30 * time.Second
	knobIdleReturn = 20 * time.Second
	firstBackoff   = 30 * time.Second
	maxBackoff     = 10 * time.Minute
)

type instance struct {
	kind      Kind
	settings  Settings
	mode      render.Mode
	offset    int
	lastInput time.Time
	lastSVG   string
}

type cacheEntry struct {
	forecast    *weather.Forecast
	fetchedAt   time.Time
	inFlight    bool
	failures    int
	nextAttempt time.Time
}

// Callbacks connect the manager to the outside world.
type Callbacks struct {
	// Draw shows svg on the given context.
	Draw func(context, svg string)
	// Fetch starts an asynchronous forecast download; the result must come back through FetchDone.
	Fetch func(q weather.Query)
	// Logf receives diagnostics. Optional.
	Logf func(format string, args ...any)
}

// Manager tracks every weather key and knob. It is not safe for concurrent use;
// call all methods, including FetchDone, from one goroutine.
type Manager struct {
	cb        Callbacks
	now       func() time.Time
	instances map[string]*instance
	cache     map[weather.Query]*cacheEntry
}

// NewManager creates a manager. now may be nil to use time.Now.
func NewManager(cb Callbacks, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	if cb.Logf == nil {
		cb.Logf = func(string, ...any) {}
	}
	return &Manager{cb: cb, now: now, instances: map[string]*instance{}, cache: map[weather.Query]*cacheEntry{}}
}

// Appear registers an action instance.
func (m *Manager) Appear(context string, kind Kind, s Settings) {
	m.instances[context] = &instance{kind: kind, settings: s}
	m.ensureFetches()
	m.redraw(context, true)
}

// KindOf reports the control type of a registered context.
func (m *Manager) KindOf(context string) (Kind, bool) {
	inst := m.instances[context]
	if inst == nil {
		return 0, false
	}
	return inst.kind, true
}

// Disappear forgets an action instance.
func (m *Manager) Disappear(context string) {
	delete(m.instances, context)
}

// SettingsChanged applies new settings and returns the instance to the current weather.
func (m *Manager) SettingsChanged(context string, s Settings) {
	inst := m.instances[context]
	if inst == nil {
		return
	}
	inst.settings = s
	inst.mode, inst.offset = render.ModeNow, 0
	m.ensureFetches()
	m.redraw(context, true)
}

// KeyDown cycles Now → Hours → Days on a key. Without data it retries the download.
func (m *Manager) KeyDown(context string) {
	inst := m.instances[context]
	if inst == nil {
		return
	}
	if m.retryIfMissing(inst) {
		m.redrawAll()
		return
	}
	inst.mode = inst.mode.Next()
	inst.offset = 0
	inst.lastInput = m.now()
	m.redraw(context, false)
}

// DialRotate scrolls the knob list. From the current weather it opens the hourly list;
// scrolling back past the start returns to the current weather.
func (m *Manager) DialRotate(context string, ticks int) {
	inst := m.instances[context]
	if inst == nil || ticks == 0 {
		return
	}
	entry := m.cache[inst.settings.Query()]
	if entry == nil || entry.forecast == nil {
		return
	}
	inst.lastInput = m.now()
	if inst.mode == render.ModeNow {
		if ticks > 0 {
			inst.mode, inst.offset = render.ModeHours, 0
		}
		m.redraw(context, false)
		return
	}
	next := inst.offset + ticks
	if next < 0 {
		inst.mode, inst.offset = render.ModeNow, 0
	} else {
		inst.offset = min(next, render.MaxOffset(entry.forecast, m.now(), inst.mode))
	}
	m.redraw(context, false)
}

// DialDown cycles Now → Hours → Days on a knob. Without data it retries the download.
func (m *Manager) DialDown(context string) {
	m.KeyDown(context)
}

// FetchDone stores the result of a download started through Callbacks.Fetch.
func (m *Manager) FetchDone(q weather.Query, f *weather.Forecast, err error) {
	entry := m.cache[q]
	if entry == nil {
		return // No instance uses this query anymore.
	}
	entry.inFlight = false
	now := m.now()
	if err != nil {
		entry.failures++
		backoff := firstBackoff << min(entry.failures-1, 5)
		entry.nextAttempt = now.Add(min(backoff, maxBackoff))
		m.cb.Logf("forecast %+v failed (%d in a row, retry in %s): %v", q, entry.failures, entry.nextAttempt.Sub(now), err)
	} else {
		entry.forecast = f
		entry.fetchedAt = now
		entry.failures = 0
		entry.nextAttempt = time.Time{}
	}
	m.redrawAll()
}

// Tick refreshes due forecasts, returns idle instances to the current weather and redraws changes.
func (m *Manager) Tick() {
	now := m.now()
	for _, inst := range m.instances {
		idle := keyIdleReturn
		if inst.kind == Knob {
			idle = knobIdleReturn
		}
		if inst.mode != render.ModeNow && now.Sub(inst.lastInput) >= idle {
			inst.mode, inst.offset = render.ModeNow, 0
		}
	}
	m.ensureFetches()
	m.redrawAll()
}

// retryIfMissing schedules an immediate download when an instance has nothing to show yet.
func (m *Manager) retryIfMissing(inst *instance) bool {
	if !inst.settings.Configured {
		return true
	}
	entry := m.cache[inst.settings.Query()]
	if entry != nil && entry.forecast != nil {
		return false
	}
	if entry != nil && !entry.inFlight {
		entry.nextAttempt = time.Time{}
		m.ensureFetches()
	}
	return true
}

// ensureFetches starts downloads for queries that are missing or older than their refresh interval,
// and drops cache entries no instance uses.
func (m *Manager) ensureFetches() {
	now := m.now()
	refresh := map[weather.Query]time.Duration{}
	for _, inst := range m.instances {
		if !inst.settings.Configured {
			continue
		}
		q := inst.settings.Query()
		if r, ok := refresh[q]; !ok || inst.settings.Refresh < r {
			refresh[q] = inst.settings.Refresh
		}
	}
	for q := range m.cache {
		if _, used := refresh[q]; !used {
			delete(m.cache, q)
		}
	}
	for q, interval := range refresh {
		entry := m.cache[q]
		if entry == nil {
			entry = &cacheEntry{}
			m.cache[q] = entry
		}
		if entry.inFlight || now.Before(entry.nextAttempt) {
			continue
		}
		if entry.forecast != nil && now.Sub(entry.fetchedAt) < interval {
			continue
		}
		entry.inFlight = true
		m.cb.Fetch(q)
	}
}

func (m *Manager) redrawAll() {
	for context := range m.instances {
		m.redraw(context, false)
	}
}

// redraw sends the screen only when it changed, unless force is set.
func (m *Manager) redraw(context string, force bool) {
	inst := m.instances[context]
	if inst == nil {
		return
	}
	svg := m.screen(inst)
	if !force && svg == inst.lastSVG {
		return
	}
	inst.lastSVG = svg
	m.cb.Draw(context, svg)
}

func (m *Manager) screen(inst *instance) string {
	status := func(s render.Status) string {
		if inst.kind == Knob {
			return render.KnobStatusSVG(s)
		}
		return render.KeyStatusSVG(s)
	}
	if !inst.settings.Configured {
		return status(render.StatusUnconfigured)
	}
	entry := m.cache[inst.settings.Query()]
	if entry == nil || entry.forecast == nil {
		if entry != nil && entry.failures > 0 && !entry.inFlight {
			return status(render.StatusOffline)
		}
		return status(render.StatusLoading)
	}

	now := m.now()
	view := render.View{
		Forecast: entry.forecast,
		Now:      now,
		City:     inst.settings.City,
		Mode:     inst.mode,
		Offset:   inst.offset,
		Stale:    entry.failures > 0 || now.Sub(entry.fetchedAt) > 2*inst.settings.Refresh,
	}
	if inst.kind == Knob {
		return render.KnobSVG(view)
	}
	return render.KeySVG(view)
}
