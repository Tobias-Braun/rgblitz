package scraper

import (
	"time"

	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
)

// RateLimiter enforces a global minimum delay between outbound playerzone requests.
// All goroutines share one instance; calling Wait() blocks until the next slot is available.
type RateLimiter struct {
	ticker *time.Ticker
}

func NewRateLimiter(interval time.Duration) *RateLimiter {
	return &RateLimiter{ticker: time.NewTicker(interval)}
}

func (r *RateLimiter) Wait() {
	start := time.Now()
	<-r.ticker.C
	metrics.RateLimitWait.Observe(time.Since(start).Seconds())
}
