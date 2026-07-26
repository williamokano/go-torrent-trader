package output

import (
	"math"
	"testing"
)

// The -1 sentinel is the whole reason this helper exists: the API cannot encode a
// JSON Infinity, so a member who has uploaded and never downloaded arrives as -1.
// Printing "-1.00" would read as a negative ratio, which is nonsense.
func TestRatioRendersTheInfiniteSentinel(t *testing.T) {
	if got := Ratio(InfiniteRatio); got != "Inf" {
		t.Errorf("Ratio(-1) = %q, want Inf", got)
	}
}

func TestRatio(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{in: 0, want: "0.00"},
		{in: 1, want: "1.00"},
		{in: 2.5, want: "2.50"},
		{in: 0.333333, want: "0.33"},
		{in: 123.456, want: "123.46"},
	}
	for _, tc := range tests {
		if got := Ratio(tc.in); got != tc.want {
			t.Errorf("Ratio(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{in: 0, want: "0 B"},
		{in: 512, want: "512 B"},
		{in: 1023, want: "1023 B"},
		{in: 1024, want: "1.00 KiB"},
		{in: 1536, want: "1.50 KiB"},
		{in: 1024 * 1024, want: "1.00 MiB"},
		{in: 1024 * 1024 * 1024, want: "1.00 GiB"},
		{in: 1024 * 1024 * 1024 * 1024, want: "1.00 TiB"},
		{in: -1024, want: "-1.00 KiB"},
	}
	for _, tc := range tests {
		if got := Bytes(tc.in); got != tc.want {
			t.Errorf("Bytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The loop must not walk past the end of the unit table for a value near the
// int64 ceiling.
func TestBytesHandlesTheLargestInt64(t *testing.T) {
	got := Bytes(1 << 62)
	if got == "" {
		t.Fatal("Bytes(1<<62) returned empty")
	}
	if want := "4.00 EiB"; got != want {
		t.Errorf("Bytes(1<<62) = %q, want %q", got, want)
	}
}

// The actual extremes, not merely a large number. MinInt64 is the dangerous one:
// negating it overflows back to itself, so a naive `-b` recurses until the stack
// dies — a crash rather than an error, from one out-of-range field in a response.
func TestBytesHandlesTheInt64Extremes(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{name: "max", in: math.MaxInt64, want: "8.00 EiB"},
		{name: "min", in: math.MinInt64, want: "-8.00 EiB"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Bytes(tc.in); got != tc.want {
				t.Errorf("Bytes(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Parity with the frontend's formatRatio, which renders anything non-finite as
// "Inf" rather than letting "+Inf" or "NaN" reach a reader.
func TestRatioHandlesNonFiniteValues(t *testing.T) {
	for _, in := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		if got := Ratio(in); got != "Inf" {
			t.Errorf("Ratio(%v) = %q, want Inf", in, got)
		}
	}
}
