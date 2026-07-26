package output

import (
	"fmt"
	"math"
	"strconv"
)

// InfiniteRatio is the sentinel the API uses for an infinite share ratio.
//
// The API cannot send a JSON Infinity, so a member who has uploaded without
// downloading is reported as -1. The frontend's formatRatio renders it as "Inf";
// this does the same so the two surfaces agree.
const InfiniteRatio = -1

// Ratio formats a share ratio for display, translating the -1 sentinel.
//
// The non-finite cases cannot arrive over JSON, but they mirror the frontend's
// formatRatio so the two surfaces stay identical for every input rather than
// only for the ones the API happens to send today.
func Ratio(r float64) string {
	if r == InfiniteRatio || math.IsInf(r, 0) || math.IsNaN(r) {
		return "Inf"
	}
	return strconv.FormatFloat(r, 'f', 2, 64)
}

// Bytes formats a byte count in binary units.
func Bytes(b int64) string {
	if b < 0 {
		// -math.MinInt64 overflows back to itself, so negating first would
		// recurse until the stack dies. Formatting the magnitude as a uint64
		// sidesteps the asymmetry of two's complement entirely.
		return "-" + bytesUnsigned(uint64(-(b+1))+1)
	}
	return bytesUnsigned(uint64(b))
}

func bytesUnsigned(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	value := float64(b)
	// Sized for uint64: the largest representable value is just under 16 EiB.
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	i := -1
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.2f %s", value, units[i])
}
