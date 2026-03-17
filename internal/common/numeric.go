package common

import "math"

// SafeInt64ToInt converts int64 to int without overflow.
func SafeInt64ToInt(v int64) int {
	if v > int64(math.MaxInt) {
		return math.MaxInt
	}
	if v < int64(math.MinInt) {
		return math.MinInt
	}
	return int(v)
}

// SafeUint64ToInt converts uint64 to int without overflow.
func SafeUint64ToInt(v uint64) int {
	if v > uint64(math.MaxInt) {
		return math.MaxInt
	}
	return int(v)
}
