package calendar

import (
	"testing"
	"time"
)

func TestPublishedHolidayAdjustments(t *testing.T) {
	c := New(t.TempDir())
	for _, tc := range []struct {
		date, rule string
		want       bool
	}{
		{"2026-09-20", "workday", true}, {"2026-09-20", "restday", false},
		{"2026-09-25", "workday", false}, {"2026-09-25", "holiday", true},
		{"2026-09-12", "restday", true}, {"2026-09-12", "holiday", false},
		{"2026-10-10", "workday", true}, {"2026-10-07", "workday", false},
	} {
		d, _ := time.Parse("2006-01-02", tc.date)
		got, known := c.Match(tc.rule, d)
		if !known || got != tc.want {
			t.Fatalf("%+v: %v %v", tc, got, known)
		}
	}
	if _, known := c.Match("workday", time.Date(2027, 1, 4, 0, 0, 0, 0, time.UTC)); known {
		t.Fatal("unknown year guessed")
	}
	if _, err := parse([]byte(`{"year":2027,"papers":[],"days":[]}`), 2027); err == nil {
		t.Fatal("unpublished calendar accepted")
	}
}
