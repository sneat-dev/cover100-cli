package model

import "testing"

func TestMetricAdd(t *testing.T) {
	got := (Metric{Covered: 2, Total: 5}).Add(Metric{Covered: 1, Total: 3})
	if want := (Metric{Covered: 3, Total: 8}); got != want {
		t.Errorf("Add() = %+v, want %+v", got, want)
	}
}

func TestMetricPercent(t *testing.T) {
	if pct, ok := (Metric{Covered: 1, Total: 4}).Percent(); !ok || pct != 0.25 {
		t.Errorf("Percent() = (%v, %v), want (0.25, true)", pct, ok)
	}
	// An unmeasured dimension is not 0%: the UI must be able to say "not
	// measured" rather than displaying a false failure.
	if pct, ok := (Metric{}).Percent(); ok || pct != 0 {
		t.Errorf("Percent() of an empty metric = (%v, %v), want (0, false)", pct, ok)
	}
}

func TestMetricAny(t *testing.T) {
	tests := []struct {
		metric Metric
		want   int
	}{
		{Metric{Covered: 1, Total: 10}, 1},
		{Metric{Covered: 0, Total: 10}, 0},
		{Metric{}, 0},
		{Metric{Covered: 0, Total: 0}, 0},
	}
	for _, tc := range tests {
		if got := tc.metric.Any(); got != tc.want {
			t.Errorf("%+v.Any() = %d, want %d", tc.metric, got, tc.want)
		}
	}
}
