package handler

import (
	"sync"
	"time"
)

const defaultMaxClients = 100000

type RateLimiter struct {
	mu          sync.Mutex
	rate        float64
	burst       float64
	maxIdle     time.Duration
	maxClients  int
	clients     map[string]*clientRate
	nextCleanup time.Time
}

type clientRate struct {
	allowance float64
	last      time.Time
	lastSeen  time.Time
}

func NewRateLimiter(rps int, burst int, maxIdle time.Duration, maxClients ...int) *RateLimiter {
	if rps <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = rps
	}
	if maxIdle <= 0 {
		maxIdle = 2 * time.Minute
	}
	mc := defaultMaxClients
	if len(maxClients) > 0 && maxClients[0] > 0 {
		mc = maxClients[0]
	}
	return &RateLimiter{
		rate:        float64(rps),
		burst:       float64(burst),
		maxIdle:     maxIdle,
		maxClients:  mc,
		clients:     make(map[string]*clientRate),
		nextCleanup: time.Now().Add(maxIdle),
	}
}

func (l *RateLimiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.After(l.nextCleanup) {
		cutoff := now.Add(-l.maxIdle)
		for clientKey, entry := range l.clients {
			if entry.lastSeen.Before(cutoff) {
				delete(l.clients, clientKey)
			}
		}
		l.nextCleanup = now.Add(l.maxIdle)
	}

	// If at capacity after cleanup, reject new clients.
	entry, ok := l.clients[key]
	if !ok {
		if len(l.clients) >= l.maxClients {
			return false
		}
		entry = &clientRate{allowance: l.burst, last: now, lastSeen: now}
		l.clients[key] = entry
	}

	elapsed := now.Sub(entry.last).Seconds()
	entry.allowance += elapsed * l.rate
	if entry.allowance > l.burst {
		entry.allowance = l.burst
	}
	entry.last = now
	entry.lastSeen = now

	if entry.allowance < 1 {
		return false
	}
	entry.allowance -= 1
	return true
}
