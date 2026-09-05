package study

import "time"

// Card is the scheduling state of one flashcard.
type Card struct {
	Interval int     // days until the next review
	Ease     float64 // SM-2 ease factor
	Due      time.Time
}

// EaseFloor is the minimum ease factor (SM-2 uses 1.3).
const EaseFloor = 1.3

// DefaultEase is the ease a new card starts with.
const DefaultEase = 2.5

// Review applies SM-2 lite to a card for a grade of 0..3
// (0 again, 1 hard, 2 good, 3 easy) and returns the new state.
//
// Simplifications versus full SM-2: there is no separate repetition counter or
// learning-step queue. A lapse (grade 0) resets the interval to one day; the
// first successful review is 1 day, the second 6 days, and after that the
// interval is multiplied by the ease factor with a per-grade modifier.
func Review(c Card, grade int, now time.Time) Card {
	if c.Ease <= 0 {
		c.Ease = DefaultEase
	}
	switch {
	case grade <= 0:
		c.Ease -= 0.20
		c.Interval = 1
	case grade == 1:
		c.Ease -= 0.15
		c.Interval = nextInterval(c.Interval, c.Ease, 0.6)
	case grade == 2:
		c.Interval = nextInterval(c.Interval, c.Ease, 1.0)
	default:
		c.Ease += 0.15
		c.Interval = nextInterval(c.Interval, c.Ease, 1.3)
	}
	if c.Ease < EaseFloor {
		c.Ease = EaseFloor
	}
	if c.Interval < 1 {
		c.Interval = 1
	}
	if c.Interval > 365 {
		c.Interval = 365
	}
	c.Due = now.AddDate(0, 0, c.Interval)
	return c
}

// nextInterval grows an interval by ease, with the classic 1 / 6 day ramp.
func nextInterval(prev int, ease, mod float64) int {
	switch {
	case prev <= 0:
		return int(1 * mod)
	case prev == 1:
		return int(6 * mod)
	default:
		n := int(float64(prev) * ease * mod)
		if n <= prev {
			n = prev + 1
		}
		return n
	}
}
