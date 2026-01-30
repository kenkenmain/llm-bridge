package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestLimiter_AllowWithinBurst(t *testing.T) {
	l := NewLimiter(Config{Rate: 1.0, Burst: 3})

	// First 3 calls should succeed (within burst)
	for i := 0; i < 3; i++ {
		if !l.Allow("user1") {
			t.Errorf("call %d: expected allow within burst", i+1)
		}
	}
}

func TestLimiter_DenyAfterBurstExhausted(t *testing.T) {
	l := NewLimiter(Config{Rate: 0.1, Burst: 2})

	// Exhaust the burst
	l.Allow("user1")
	l.Allow("user1")

	// Next call should be denied
	if l.Allow("user1") {
		t.Error("expected deny after burst exhausted")
	}
}

func TestLimiter_RefillOverTime(t *testing.T) {
	l := NewLimiter(Config{Rate: 10.0, Burst: 2})

	// Exhaust burst
	l.Allow("user1")
	l.Allow("user1")

	if l.Allow("user1") {
		t.Error("expected deny immediately after burst exhausted")
	}

	// Wait for refill (at 10 tokens/sec, 200ms should refill ~2 tokens)
	time.Sleep(250 * time.Millisecond)

	if !l.Allow("user1") {
		t.Error("expected allow after refill period")
	}
}

func TestLimiter_DifferentKeys(t *testing.T) {
	l := NewLimiter(Config{Rate: 0.1, Burst: 1})

	// user1 uses its token
	if !l.Allow("user1") {
		t.Error("user1 first call should be allowed")
	}
	// user1 is now denied
	if l.Allow("user1") {
		t.Error("user1 second call should be denied")
	}

	// user2 should still be allowed (independent bucket)
	if !l.Allow("user2") {
		t.Error("user2 first call should be allowed (independent bucket)")
	}
}

func TestLimiter_Reset(t *testing.T) {
	l := NewLimiter(Config{Rate: 0.1, Burst: 1})

	// Exhaust user1
	l.Allow("user1")
	if l.Allow("user1") {
		t.Error("should be denied before reset")
	}

	// Reset clears all state
	l.Reset()

	// user1 should be allowed again
	if !l.Allow("user1") {
		t.Error("should be allowed after reset")
	}
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	l := NewLimiter(Config{Rate: 100.0, Burst: 50})

	var wg sync.WaitGroup
	// Run many goroutines concurrently to test for data races
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "user"
			if id%2 == 0 {
				key = "user-even"
			}
			for j := 0; j < 10; j++ {
				l.Allow(key)
			}
		}(i)
	}

	wg.Wait()
	// Test passes if no race condition or panic
}

func TestLimiter_ZeroBurst(t *testing.T) {
	l := NewLimiter(Config{Rate: 1.0, Burst: 0})

	// With zero burst, no tokens are ever available
	if l.Allow("user1") {
		t.Error("zero burst should deny all requests")
	}
}

func TestLimiter_TokensCappedAtBurst(t *testing.T) {
	l := NewLimiter(Config{Rate: 1000.0, Burst: 2})

	// Even with a very high rate, tokens should be capped at burst
	time.Sleep(10 * time.Millisecond)

	allowed := 0
	for i := 0; i < 5; i++ {
		if l.Allow("user1") {
			allowed++
		}
	}

	if allowed > 2 {
		t.Errorf("tokens should be capped at burst of 2, but %d were allowed", allowed)
	}
}
