// Package progress provides transfer throughput and ETA reporting helpers.
package progress

import (
	"fmt"
	"io"
	"time"
)

// Event describes transfer status at a point in time.
type Event struct {
	Bytes        uint64
	Total        uint64
	InstantBps   float64
	AverageBps   float64
	ETA          time.Duration
	Elapsed      time.Duration
	Done         bool
	OutputPath   string
	Direction    string
	LastChunkLen int
}

// Reporter emits human-readable progress updates.
type Reporter struct {
	w          io.Writer
	total      uint64
	direction  string
	start      time.Time
	lastTick   time.Time
	lastBytes  uint64
	minTickGap time.Duration
}

// NewReporter creates a reporter with update throttling.
func NewReporter(w io.Writer, direction string, total uint64) *Reporter {
	now := time.Now()
	return &Reporter{w: w, total: total, direction: direction, start: now, lastTick: now, minTickGap: 150 * time.Millisecond}
}

// Update prints progress at throttled intervals.
func (r *Reporter) Update(bytes uint64) {
	now := time.Now()
	if now.Sub(r.lastTick) < r.minTickGap && bytes < r.total {
		return
	}
	e := r.buildEvent(bytes, now, false, "")
	_, _ = fmt.Fprintf(r.w, "\r%s %s/%s inst:%s avg:%s eta:%s", r.direction, humanBytes(e.Bytes), humanBytes(e.Total), humanRate(e.InstantBps), humanRate(e.AverageBps), humanDuration(e.ETA))
	r.lastTick = now
	r.lastBytes = bytes
}

// Done prints final summary.
func (r *Reporter) Done(bytes uint64, outPath string) {
	now := time.Now()
	e := r.buildEvent(bytes, now, true, outPath)
	_, _ = fmt.Fprintf(r.w, "\r%s complete %s in %s avg:%s out:%s\n", r.direction, humanBytes(e.Bytes), humanDuration(e.Elapsed), humanRate(e.AverageBps), outPath)
}

func (r *Reporter) buildEvent(bytes uint64, now time.Time, done bool, outPath string) Event {
	elapsed := now.Sub(r.start)
	if elapsed <= 0 {
		elapsed = time.Millisecond
	}
	chunkDur := now.Sub(r.lastTick)
	if chunkDur <= 0 {
		chunkDur = time.Millisecond
	}
	inst := float64(bytes-r.lastBytes) / chunkDur.Seconds()
	avg := float64(bytes) / elapsed.Seconds()
	remaining := uint64(0)
	if bytes < r.total {
		remaining = r.total - bytes
	}
	eta := time.Duration(0)
	if avg > 0 && remaining > 0 {
		eta = time.Duration(float64(remaining)/avg) * time.Second
	}
	return Event{Bytes: bytes, Total: r.total, InstantBps: inst, AverageBps: avg, ETA: eta, Elapsed: elapsed, Done: done, OutputPath: outPath, Direction: r.direction}
}

func humanBytes(v uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	val := float64(v)
	u := 0
	for val >= 1024 && u < len(units)-1 {
		val /= 1024
		u++
	}
	return fmt.Sprintf("%.1f%s", val, units[u])
}

func humanRate(bps float64) string {
	if bps < 0 {
		bps = 0
	}
	return fmt.Sprintf("%s/s", humanBytes(uint64(bps)))
}

func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return d.Truncate(time.Second).String()
}
