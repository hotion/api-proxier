package proxy

import (
	"github.com/sony/gobreaker"
)

// reference to: https://github.com/sony/gobreaker
// [TODO](done): support config ...

const (
	// CntRequests ...
	CntRequests = 10
	// FailureRatio ...
	FailureRatio = 0.6
)

// defaultReadyToTrip is for gobreaker.Setting
//
// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state.
// If ReadyToTrip returns true, CircuitBreaker will be placed into the open state.
// If ReadyToTrip is nil, default ReadyToTrip is used.
// Default ReadyToTrip returns true when the number of consecutive failures is more than 5.
//
func defaultReadyToTrip(counts gobreaker.Counts) bool {
	failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
	return counts.Requests >= CntRequests && failureRatio >= FailureRatio
}

func genReadyToTrip(cnt uint32, ratio float64) func(counts gobreaker.Counts) bool {
	return func(counts gobreaker.Counts) bool {
		failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
		return counts.Requests >= cnt && failureRatio >= ratio
	}
}
