// Package autonomy implements the AegisVision Phase 5 continuous
// autonomy primitives:
//
//   - Cron — a small cron expression parser (6-field: sec min hour dom mon
//     dow) with Next() to compute the next tick. We avoid bringing in
//     robfig/cron to keep the dependency graph small.
//
//   - Scheduler — fires Tickers for each registered Schedule; supports
//     pause/resume; emits OverlapSkipped when a previous tick is still
//     running.
//
//   - SignalRouter — multiplexes inbound bus signals (drift / slo /
//     canary regression) onto the corresponding Schedule handlers.
package autonomy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron is a parsed 6-field crontab expression: "sec min hour dom mon dow".
// Each field is either:
//   - "*"
//   - a single number
//   - a "step" of the form "*/N"
//   - a comma-separated list of numbers
//
// We intentionally do NOT support ranges or named months/dows — those
// shapes can grow indefinitely and the autonomy scheduler doesn't need
// them. Real platform schedules are either "every N minutes" or "at a
// specific minute of every hour".
type Cron struct {
	Sec, Min, Hour, DOM, Mon, DOW set
}

type set struct {
	any  bool
	step int       // 0 = no step
	vals map[int]struct{}
}

// ParseCron parses a 6-field cron expression. Returns an error on
// malformed input.
func ParseCron(expr string) (Cron, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 6 {
		return Cron{}, fmt.Errorf("cron: expected 6 fields, got %d", len(fields))
	}
	c := Cron{}
	var err error
	if c.Sec, err = parseField(fields[0], 0, 59); err != nil {
		return c, fmt.Errorf("cron sec: %w", err)
	}
	if c.Min, err = parseField(fields[1], 0, 59); err != nil {
		return c, fmt.Errorf("cron min: %w", err)
	}
	if c.Hour, err = parseField(fields[2], 0, 23); err != nil {
		return c, fmt.Errorf("cron hour: %w", err)
	}
	if c.DOM, err = parseField(fields[3], 1, 31); err != nil {
		return c, fmt.Errorf("cron dom: %w", err)
	}
	if c.Mon, err = parseField(fields[4], 1, 12); err != nil {
		return c, fmt.Errorf("cron mon: %w", err)
	}
	if c.DOW, err = parseField(fields[5], 0, 6); err != nil {
		return c, fmt.Errorf("cron dow: %w", err)
	}
	return c, nil
}

func parseField(s string, lo, hi int) (set, error) {
	if s == "*" {
		return set{any: true}, nil
	}
	if strings.HasPrefix(s, "*/") {
		n, err := strconv.Atoi(s[2:])
		if err != nil || n <= 0 {
			return set{}, errors.New("invalid step")
		}
		return set{any: true, step: n}, nil
	}
	vals := map[int]struct{}{}
	for _, part := range strings.Split(s, ",") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return set{}, fmt.Errorf("invalid value %q", part)
		}
		if n < lo || n > hi {
			return set{}, fmt.Errorf("value %d out of range [%d,%d]", n, lo, hi)
		}
		vals[n] = struct{}{}
	}
	return set{vals: vals}, nil
}

func (s set) match(n int) bool {
	if s.any && s.step == 0 {
		return true
	}
	if s.step > 0 {
		return n%s.step == 0
	}
	_, ok := s.vals[n]
	return ok
}

// Next returns the smallest time strictly after `after` at which all six
// fields match. Returns the zero value if no match is found within ~10y.
func (c Cron) Next(after time.Time) time.Time {
	// Walk forward in 1-second increments. Bounded by 10 years — anything
	// beyond is a misconfigured schedule.
	t := after.Add(time.Second).Truncate(time.Second)
	limit := after.Add(10 * 365 * 24 * time.Hour)
	for t.Before(limit) {
		if c.matches(t) {
			return t
		}
		t = t.Add(time.Second)
	}
	return time.Time{}
}

func (c Cron) matches(t time.Time) bool {
	return c.Sec.match(t.Second()) &&
		c.Min.match(t.Minute()) &&
		c.Hour.match(t.Hour()) &&
		c.DOM.match(t.Day()) &&
		c.Mon.match(int(t.Month())) &&
		c.DOW.match(int(t.Weekday()))
}
