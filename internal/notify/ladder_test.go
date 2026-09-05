package notify

import (
	"testing"
	"time"
)

func mustLadder() Ladder {
	return ParseLadder([]string{"72h", "48h", "24h", "3h", "1h"})
}

func TestParseLadderSortsAndFilters(t *testing.T) {
	l := ParseLadder([]string{"24h", "bogus", "72h", "0h", "-5h", "48h", "24h"})
	want := []time.Duration{72 * time.Hour, 48 * time.Hour, 24 * time.Hour}
	if len(l) != len(want) {
		t.Fatalf("got %v, want %v", l, want)
	}
	for i := range want {
		if l[i] != want[i] {
			t.Fatalf("got %v, want %v", l, want)
		}
	}
}

// fakeClock lets the table below express "now" relative to a fixed due date.
var due = time.Date(2026, 9, 18, 23, 59, 0, 0, time.UTC)

func TestLadderActive(t *testing.T) {
	l := mustLadder()
	tests := []struct {
		name    string
		now     time.Time
		wantOK  bool
		wantDur time.Duration
	}{
		{"far out, no rung", due.Add(-100 * time.Hour), false, 0},
		{"exactly 72h", due.Add(-72 * time.Hour), true, 72 * time.Hour},
		{"just inside 72h", due.Add(-71 * time.Hour), true, 72 * time.Hour},
		{"just above 48h", due.Add(-48*time.Hour - time.Minute), true, 72 * time.Hour},
		{"exactly 48h", due.Add(-48 * time.Hour), true, 48 * time.Hour},
		{"between 48 and 24", due.Add(-30 * time.Hour), true, 48 * time.Hour},
		{"exactly 24h", due.Add(-24 * time.Hour), true, 24 * time.Hour},
		{"between 24 and 3", due.Add(-5 * time.Hour), true, 24 * time.Hour},
		{"exactly 3h", due.Add(-3 * time.Hour), true, 3 * time.Hour},
		{"between 3 and 1", due.Add(-2 * time.Hour), true, 3 * time.Hour},
		{"exactly 1h", due.Add(-1 * time.Hour), true, time.Hour},
		{"30 min left", due.Add(-30 * time.Minute), true, time.Hour},
		{"1 second left", due.Add(-time.Second), true, time.Hour},
		{"exactly due", due, false, 0},
		{"overdue", due.Add(time.Hour), false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := l.Active(due.Sub(tc.now))
			if ok != tc.wantOK || got != tc.wantDur {
				t.Errorf("Active(remaining=%v) = (%v,%v), want (%v,%v)",
					due.Sub(tc.now), got, ok, tc.wantDur, tc.wantOK)
			}
		})
	}
}

func TestLadderActiveEmpty(t *testing.T) {
	var l Ladder
	if _, ok := l.Active(time.Hour); ok {
		t.Error("empty ladder should never be active")
	}
}

func TestLadderSuperseded(t *testing.T) {
	l := mustLadder()
	tests := []struct {
		name string
		now  time.Time
		want []time.Duration
	}{
		{"nothing reached", due.Add(-100 * time.Hour), nil},
		{"only 72h reached, it is active", due.Add(-60 * time.Hour), nil},
		{"48h active, 72h superseded", due.Add(-30 * time.Hour),
			[]time.Duration{72 * time.Hour}},
		{"24h active", due.Add(-10 * time.Hour),
			[]time.Duration{72 * time.Hour, 48 * time.Hour}},
		{"1h active, four superseded", due.Add(-30 * time.Minute),
			[]time.Duration{72 * time.Hour, 48 * time.Hour, 24 * time.Hour, 3 * time.Hour}},
		{"overdue: all superseded", due.Add(time.Hour),
			[]time.Duration{72 * time.Hour, 48 * time.Hour, 24 * time.Hour, 3 * time.Hour, time.Hour}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := l.Superseded(due.Sub(tc.now))
			if len(got) != len(tc.want) {
				t.Fatalf("Superseded = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("Superseded = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestLadderFiresEachRungExactlyOnce walks a fake clock minute by minute over
// four days and asserts every rung fires once, in order.
func TestLadderFiresEachRungExactlyOnce(t *testing.T) {
	l := mustLadder()
	sent := map[time.Duration]int{}
	var order []time.Duration

	for now := due.Add(-96 * time.Hour); now.Before(due); now = now.Add(time.Minute) {
		rung, ok := l.Active(due.Sub(now))
		if !ok {
			continue
		}
		if sent[rung] == 0 {
			order = append(order, rung)
		}
		sent[rung]++
	}

	if len(order) != len(l) {
		t.Fatalf("fired %v, want all of %v", order, l)
	}
	for i := range l {
		if order[i] != l[i] {
			t.Fatalf("fire order %v, want %v", order, l)
		}
	}
}

func TestHumanize(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{72 * time.Hour, "3 days"},
		{48 * time.Hour, "2 days"},
		{47 * time.Hour, "1 day"},
		{24 * time.Hour, "1 day"},
		{3 * time.Hour, "3 hours"},
		{time.Hour, "1 hour"},
		{90 * time.Minute, "2 hours"},
		{30 * time.Minute, "30 minutes"},
		{time.Minute, "1 minute"},
		{20 * time.Second, "less than a minute"},
	}
	for _, tc := range tests {
		if got := Humanize(tc.in); got != tc.want {
			t.Errorf("Humanize(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatDue(t *testing.T) {
	d := time.Date(2026, 9, 18, 23, 59, 0, 0, time.Local)
	if got := FormatDue(d); got != "Fri 18 Sep, 23:59" {
		t.Errorf("FormatDue = %q", got)
	}
}

func TestRungKeyStable(t *testing.T) {
	if RungKey(72*time.Hour) != "72h0m0s" {
		t.Errorf("RungKey = %q", RungKey(72*time.Hour))
	}
	if RungKey(time.Hour) != "1h0m0s" {
		t.Errorf("RungKey = %q", RungKey(time.Hour))
	}
}
