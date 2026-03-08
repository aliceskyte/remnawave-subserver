package handler_test

import (
	"testing"
	"time"

	"subserver/internal/handler"
)

func TestNewRateLimiter_InvalidRPS(t *testing.T) {
	tests := []struct {
		name string
		rps  int
	}{
		{"zero rps", 0},
		{"negative rps", -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := handler.NewRateLimiter(tt.rps, 1, time.Minute)
			if rl != nil {
				t.Fatalf("NewRateLimiter(%d, ...) = non-nil, want nil", tt.rps)
			}
		})
	}
}

func TestNewRateLimiter_Valid(t *testing.T) {
	rl := handler.NewRateLimiter(10, 5, time.Minute)
	if rl == nil {
		t.Fatal("NewRateLimiter(10, 5, 1m) returned nil, want non-nil")
	}
}

func TestRateLimiter_AllowWithinBurst(t *testing.T) {
	burst := 5
	rl := handler.NewRateLimiter(1, burst, time.Minute)

	for i := 0; i < burst; i++ {
		if !rl.Allow("client1") {
			t.Fatalf("Allow() returned false on request %d, which is within burst of %d", i+1, burst)
		}
	}
}

func TestRateLimiter_ExceedBurstReturnsFalse(t *testing.T) {
	burst := 3
	rl := handler.NewRateLimiter(1, burst, time.Minute)

	// Exhaust burst.
	for i := 0; i < burst; i++ {
		rl.Allow("client1")
	}

	// Next request should be rejected.
	if rl.Allow("client1") {
		t.Fatal("Allow() returned true after burst exhausted, want false")
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	// High rps so tokens refill quickly for a fast test.
	rps := 100
	burst := 1
	rl := handler.NewRateLimiter(rps, burst, time.Minute)

	// Exhaust the single token.
	if !rl.Allow("client1") {
		t.Fatal("first Allow() returned false, want true")
	}
	if rl.Allow("client1") {
		t.Fatal("Allow() after exhaustion returned true, want false")
	}

	// Wait long enough for at least 1 token to refill.
	time.Sleep(50 * time.Millisecond)

	if !rl.Allow("client1") {
		t.Fatal("Allow() after refill wait returned false, want true")
	}
}

func TestRateLimiter_IndependentKeys(t *testing.T) {
	burst := 2
	rl := handler.NewRateLimiter(1, burst, time.Minute)

	// Exhaust burst for client-a.
	for i := 0; i < burst; i++ {
		rl.Allow("client-a")
	}
	if rl.Allow("client-a") {
		t.Fatal("client-a should be rate-limited, but Allow returned true")
	}

	// client-b should still have its full burst available.
	for i := 0; i < burst; i++ {
		if !rl.Allow("client-b") {
			t.Fatalf("client-b Allow() returned false on request %d, want true", i+1)
		}
	}
}

func TestRateLimiter_CleanupIdleClients(t *testing.T) {
	maxIdle := 50 * time.Millisecond
	rl := handler.NewRateLimiter(10, 10, maxIdle)

	// Create a client entry.
	rl.Allow("idle-client")

	// Wait for the idle duration to elapse.
	time.Sleep(80 * time.Millisecond)

	// Trigger cleanup by calling Allow on a different key (also advances past nextCleanup).
	// After cleanup, idle-client's entry should be removed.
	// A subsequent call should behave as a fresh client with full burst.
	for i := 0; i < 10; i++ {
		if !rl.Allow("idle-client") {
			t.Fatalf("idle-client should have been cleaned up and received a fresh burst, but Allow returned false on request %d", i+1)
		}
	}
}

func TestRateLimiter_MaxClientsLimit(t *testing.T) {
	rl := handler.NewRateLimiter(10, 5, time.Minute, 2)

	// Two clients fit within the limit.
	if !rl.Allow("client-1") {
		t.Fatal("client-1 should be allowed (within maxClients)")
	}
	if !rl.Allow("client-2") {
		t.Fatal("client-2 should be allowed (within maxClients)")
	}

	// Third distinct client should be rejected because maxClients=2.
	if rl.Allow("client-3") {
		t.Fatal("client-3 should be rejected (maxClients exceeded), but Allow returned true")
	}

	// Existing clients should still work fine.
	if !rl.Allow("client-1") {
		t.Fatal("existing client-1 should still be allowed after maxClients reached")
	}
	if !rl.Allow("client-2") {
		t.Fatal("existing client-2 should still be allowed after maxClients reached")
	}
}

func TestRateLimiter_NilAllow(t *testing.T) {
	var rl *handler.RateLimiter

	if !rl.Allow("anything") {
		t.Fatal("Allow on nil RateLimiter returned false, want true")
	}
}

func TestRateLimiter_BurstEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		rps         int
		burst       int
		requests    int
		wantAllowed int
	}{
		{"burst of 1", 10, 1, 3, 1},
		{"burst equals rps", 5, 5, 7, 5},
		{"large burst small rps", 1, 10, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := handler.NewRateLimiter(tt.rps, tt.burst, time.Minute)
			allowed := 0
			for i := 0; i < tt.requests; i++ {
				if rl.Allow("test") {
					allowed++
				}
			}
			if allowed != tt.wantAllowed {
				t.Fatalf("got %d allowed out of %d requests, want %d", allowed, tt.requests, tt.wantAllowed)
			}
		})
	}
}
