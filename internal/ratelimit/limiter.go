package ratelimit

import (
	"sync"
	"time"
)

// Config holds rate limit settings.
type Config struct {
	Rate  float64 // tokens per second refill rate
	Burst int     // maximum burst size (bucket capacity)
}

// Limiter tracks per-key rate limits using token buckets.
type Limiter struct {
	cfg     Config
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewLimiter creates a Limiter with the given config.
func NewLimiter(cfg Config) *Limiter {
	return &Limiter{
		cfg:     cfg,
		buckets: make(map[string]*bucket),
	}
}

// Allow checks if the given key is within rate limits.
// Returns true if allowed, false if rate-limited.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{
			tokens:    float64(l.cfg.Burst),
			lastCheck: now,
		}
		l.buckets[key] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * l.cfg.Rate
	if b.tokens > float64(l.cfg.Burst) {
		b.tokens = float64(l.cfg.Burst)
	}
	b.lastCheck = now

	// Check if a token is available
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

// Reset removes all tracked keys.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = make(map[string]*bucket)
}
