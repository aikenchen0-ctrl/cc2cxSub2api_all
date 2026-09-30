package main

import (
	"encoding/json"
	"math"
	"testing"
)

func TestNanosToCentsDoesNotOverflow(t *testing.T) {
	for _, tc := range []struct{ nanos, want int64 }{
		{math.MaxInt64, 922337203685}, {math.MinInt64, -922337203685},
		{4_999_999, 0}, {5_000_000, 1}, {15_000_000, 2},
		{-4_999_999, 0}, {-5_000_000, -1}, {-15_000_000, -2},
	} {
		if got := nanosToCents(tc.nanos); got != tc.want {
			t.Errorf("nanosToCents(%d) = %d, want %d", tc.nanos, got, tc.want)
		}
	}
}

func TestDecimalNanosRejectsRoundedIntegerOverflow(t *testing.T) {
	for _, raw := range []string{`9223372036.854776`, `"9223372036.854776"`, `-9223372036.854778`} {
		if got, ok := decimalNanos(json.RawMessage(raw)); ok {
			t.Errorf("out-of-range cost %s accepted as %d", raw, got)
		}
	}
	for _, raw := range []string{`0.25`, `"0.25"`, `2.5e-1`} {
		if got, ok := decimalNanos(json.RawMessage(raw)); !ok || got != 250_000_000 {
			t.Errorf("valid cost %s = %d, %v", raw, got, ok)
		}
	}
}
