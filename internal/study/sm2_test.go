package study

import (
	"testing"
	"time"
)

var day0 = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

func TestReviewNewCardRamp(t *testing.T) {
	c := Card{}
	c = Review(c, 2, day0) // good
	if c.Interval != 1 {
		t.Fatalf("first good interval = %d, want 1", c.Interval)
	}
	if !c.Due.Equal(day0.AddDate(0, 0, 1)) {
		t.Fatalf("due = %v", c.Due)
	}
	c = Review(c, 2, day0)
	if c.Interval != 6 {
		t.Fatalf("second good interval = %d, want 6", c.Interval)
	}
	c = Review(c, 2, day0)
	if c.Interval != 15 { // 6 * 2.5
		t.Fatalf("third good interval = %d, want 15", c.Interval)
	}
	if c.Ease != DefaultEase {
		t.Fatalf("good must not change ease, got %v", c.Ease)
	}
}

func TestReviewLapseResets(t *testing.T) {
	c := Card{Interval: 40, Ease: 2.5}
	c = Review(c, 0, day0)
	if c.Interval != 1 {
		t.Fatalf("lapse interval = %d, want 1", c.Interval)
	}
	if c.Ease != 2.3 {
		t.Fatalf("lapse ease = %v, want 2.3", c.Ease)
	}
}

func TestReviewEaseFloor(t *testing.T) {
	c := Card{Interval: 5, Ease: 1.4}
	for i := 0; i < 5; i++ {
		c = Review(c, 0, day0)
	}
	if c.Ease != EaseFloor {
		t.Fatalf("ease = %v, want floor %v", c.Ease, EaseFloor)
	}
}

func TestReviewEasyGrowsFaster(t *testing.T) {
	base := Card{Interval: 10, Ease: 2.5}
	good := Review(base, 2, day0)
	easy := Review(base, 3, day0)
	hard := Review(base, 1, day0)
	if !(hard.Interval < good.Interval && good.Interval < easy.Interval) {
		t.Fatalf("expected hard < good < easy, got %d %d %d",
			hard.Interval, good.Interval, easy.Interval)
	}
	if easy.Ease <= base.Ease {
		t.Fatalf("easy should raise ease, got %v", easy.Ease)
	}
	if hard.Ease >= base.Ease {
		t.Fatalf("hard should lower ease, got %v", hard.Ease)
	}
}

func TestReviewIntervalAlwaysAdvances(t *testing.T) {
	// A low ease must never leave the interval stuck or shrinking on a pass.
	c := Card{Interval: 9, Ease: EaseFloor}
	next := Review(c, 1, day0)
	if next.Interval <= c.Interval {
		t.Fatalf("interval %d did not advance past %d", next.Interval, c.Interval)
	}
}

func TestReviewCaps(t *testing.T) {
	c := Card{Interval: 300, Ease: 2.5}
	c = Review(c, 3, day0)
	if c.Interval != 365 {
		t.Fatalf("interval = %d, want capped at 365", c.Interval)
	}
}
