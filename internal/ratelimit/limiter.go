package ratelimit

import (
	"sync"

	"golang.org/x/time/rate"
)

// Config holds rate limit settings.
type Config struct {
	Rate  float64 // tokens per second refill rate
	Burst int     // maximum burst size (bucket capacity)
}

// Limiter tracks per-key rate limits using token buckets.
// It wraps golang.org/x/time/rate.Limiter with per-key tracking.
type Limiter struct {
	cfg      Config
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// NewLimiter creates a Limiter with the given config.
func NewLimiter(cfg Config) *Limiter {
	return &Limiter{
		cfg:      cfg,
		limiters: make(map[string]*rate.Limiter),
	}
}

// Allow checks if the given key is within rate limits.
// Returns true if allowed, false if rate-limited.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	limiter, ok := l.limiters[key]
	if !ok {
		limiter = rate.NewLimiter(rate.Limit(l.cfg.Rate), l.cfg.Burst)
		l.limiters[key] = limiter
	}
	l.mu.Unlock()

	return limiter.Allow()
}

// Reset removes all tracked keys.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limiters = make(map[string]*rate.Limiter)
}
