package transfer

import (
	"math"
	"time"
)

// speedTau is the EWMA time constant: a speed change is ~63% reflected after this long.
const speedTau = 3 * time.Second

// meter smooths bytes/second with a time-weighted EWMA, so uneven sampling intervals
// (ticks plus early flushes) do not skew the estimate.
type meter struct {
	speed float64
	last  int64
	at    time.Time
}

// sample folds the byte count at now into the speed and reports whether the speed moved.
func (m *meter) sample(now time.Time, bytes int64) bool {
	if m.at.IsZero() {
		m.at, m.last = now, bytes
		return false
	}
	dt := now.Sub(m.at).Seconds()
	if dt <= 0 {
		return false
	}
	inst := math.Max(float64(bytes-m.last)/dt, 0) // failed tasks roll bytes back
	prev := m.speed
	m.speed += (1 - math.Exp(-dt/speedTau.Seconds())) * (inst - m.speed)
	if inst == 0 && m.speed < 1 {
		m.speed = 0
	}
	m.at, m.last = now, bytes
	return m.speed != prev
}

// aggregate is the only goroutine that calls Emit.
func (e *Engine) aggregate() {
	defer close(e.aggDone)
	tk := time.NewTicker(e.tick)
	defer tk.Stop()
	for {
		select {
		case <-tk.C:
			e.flush(true)
		case <-e.kick:
			e.flush(false)
		case <-e.stop:
			e.flush(false)
			return
		}
	}
}

// flush emits the jobs that changed since the last flush. Only ticks sample speed, so
// early flushes triggered by state changes do not add noise.
func (e *Engine) flush(sample bool) {
	now := time.Now()
	jobs := []JobView{}
	e.mu.Lock()
	for _, j := range e.order {
		if sample && (j.state == StateRunning || j.state == StateWaiting) && j.meter.sample(now, j.doneBytes.Load()) {
			j.dirty.Store(true)
		}
		if j.dirty.Swap(false) {
			jobs = append(jobs, j.view())
		}
	}
	removed := e.removed
	e.removed = nil
	e.mu.Unlock()
	if len(jobs) == 0 && len(removed) == 0 {
		return
	}
	if removed == nil {
		removed = []string{}
	}
	if e.emit != nil {
		e.emit(Update{Jobs: jobs, Removed: removed})
	}
}
