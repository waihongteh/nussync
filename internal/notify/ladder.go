// Package notify turns store state into Telegram notifications on a schedule.
package notify

import (
	"fmt"
	"sort"
	"time"
)

// Ladder is a set of "time before due" reminder rungs, sorted longest-first.
type Ladder []time.Duration

// ParseLadder converts Go duration strings into a sorted, de-duplicated ladder.
// Unparseable or non-positive entries are ignored.
func ParseLadder(specs []string) Ladder {
	seen := map[time.Duration]bool{}
	var l Ladder
	for _, s := range specs {
		d, err := time.ParseDuration(s)
		if err != nil || d <= 0 || seen[d] {
			continue
		}
		seen[d] = true
		l = append(l, d)
	}
	sort.Slice(l, func(i, j int) bool { return l[i] > l[j] })
	return l
}

// RungKey is the stable identifier stored in reminders_sent.
func RungKey(d time.Duration) string { return d.String() }

// Active returns the single rung that should be firing right now for a task
// with the given time remaining: the rung d where next_smaller < remaining <= d
// (next_smaller is 0 for the final rung). Returns false when the deadline is
// past, or still further out than the longest rung.
func (l Ladder) Active(remaining time.Duration) (time.Duration, bool) {
	if remaining <= 0 || len(l) == 0 {
		return 0, false
	}
	for i, d := range l {
		if remaining > d {
			continue
		}
		var next time.Duration // 0 for the last rung
		if i+1 < len(l) {
			next = l[i+1]
		}
		if remaining > next {
			return d, true
		}
	}
	return 0, false
}

// Superseded returns the rungs that have already elapsed but are longer than
// the currently active one. On first run these are marked sent without firing,
// so a freshly installed app does not blast a backlog of reminders.
func (l Ladder) Superseded(remaining time.Duration) []time.Duration {
	var out []time.Duration
	if remaining <= 0 {
		return append(out, l...)
	}
	active, ok := l.Active(remaining)
	for _, d := range l {
		if remaining > d {
			continue // not reached yet
		}
		if ok && d == active {
			continue // this is the one we want to actually send
		}
		out = append(out, d)
	}
	return out
}

// Humanize renders a remaining duration the way the reminder text reads
// ("3 days", "5 hours", "45 minutes").
func Humanize(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d >= 48*time.Hour:
		return plural(int(d.Round(time.Hour)/(24*time.Hour)), "day")
	case d >= 24*time.Hour:
		return "1 day"
	case d >= time.Hour:
		return plural(int(d.Round(time.Hour)/time.Hour), "hour")
	case d >= time.Minute:
		return plural(int(d.Round(time.Minute)/time.Minute), "minute")
	default:
		return "less than a minute"
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// FormatDue renders a due timestamp as "Thu 18 Sep, 23:59" in local time.
func FormatDue(t time.Time) string {
	return t.Local().Format("Mon 2 Jan, 15:04")
}
