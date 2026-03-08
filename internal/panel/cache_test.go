package panel_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"subserver/internal/panel"
)

func TestNewCache_InvalidTTL(t *testing.T) {
	tests := []struct {
		name string
		ttl  time.Duration
	}{
		{"zero TTL", 0},
		{"negative TTL", -1 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := panel.NewCache(tt.ttl)
			if c != nil {
				t.Fatalf("NewCache(%v) = non-nil, want nil", tt.ttl)
			}
		})
	}
}

func TestNewCache_Valid(t *testing.T) {
	c := panel.NewCache(time.Minute)
	if c == nil {
		t.Fatal("NewCache(1m) returned nil, want non-nil")
	}
}

func TestCache_BasicSetGet(t *testing.T) {
	c := panel.NewCache(time.Minute)

	tests := []struct {
		name  string
		key   string
		value map[string]any
	}{
		{"simple string value", "key1", map[string]any{"name": "alice"}},
		{"numeric value", "key2", map[string]any{"count": 42}},
		{"nested map", "key3", map[string]any{"nested": map[string]any{"a": 1}}},
		{"empty map", "key4", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Set(tt.key, tt.value)
			got, ok := c.Get(tt.key)
			if !ok {
				t.Fatalf("Get(%q) returned ok=false, want true", tt.key)
			}
			if len(got) != len(tt.value) {
				t.Fatalf("Get(%q) returned %d entries, want %d", tt.key, len(got), len(tt.value))
			}
		})
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := panel.NewCache(time.Minute)
	val, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("Get on missing key returned ok=true, want false")
	}
	if val != nil {
		t.Fatalf("Get on missing key returned %v, want nil", val)
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := panel.NewCache(50 * time.Millisecond)
	c.Set("ephemeral", map[string]any{"data": "temp"})

	// Should be present immediately.
	if _, ok := c.Get("ephemeral"); !ok {
		t.Fatal("Get immediately after Set returned ok=false, want true")
	}

	// Wait for TTL to expire.
	time.Sleep(80 * time.Millisecond)

	val, ok := c.Get("ephemeral")
	if ok {
		t.Fatalf("Get after TTL expiry returned ok=true with value %v, want ok=false", val)
	}
}

func TestCache_MaxSizeEviction(t *testing.T) {
	c := panel.NewCache(time.Minute, 2)

	// Insert two items — cache is now full.
	c.Set("first", map[string]any{"order": 1})
	// Small sleep to guarantee "first" has the earliest expiration.
	time.Sleep(5 * time.Millisecond)
	c.Set("second", map[string]any{"order": 2})

	// Insert a third item; the oldest ("first") should be evicted.
	c.Set("third", map[string]any{"order": 3})

	if _, ok := c.Get("first"); ok {
		t.Fatal("expected 'first' to be evicted, but it was still present")
	}
	if _, ok := c.Get("second"); !ok {
		t.Fatal("expected 'second' to be present after eviction")
	}
	if _, ok := c.Get("third"); !ok {
		t.Fatal("expected 'third' to be present after eviction")
	}
}

func TestCache_OverwriteKey(t *testing.T) {
	c := panel.NewCache(time.Minute)
	c.Set("key", map[string]any{"v": 1})
	c.Set("key", map[string]any{"v": 2})

	got, ok := c.Get("key")
	if !ok {
		t.Fatal("Get after overwrite returned ok=false")
	}
	if got["v"] != 2 {
		t.Fatalf("Get after overwrite returned v=%v, want 2", got["v"])
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := panel.NewCache(time.Second)
	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("key-%d-%d", id, i)
				c.Set(key, map[string]any{"id": id, "i": i})
				c.Get(key)
			}
		}(g)
	}

	wg.Wait()
	// If we reach here without a race detector failure or panic, the test passes.
}

func TestCache_NilSafety(t *testing.T) {
	var c *panel.Cache

	t.Run("Get on nil cache", func(t *testing.T) {
		val, ok := c.Get("key")
		if ok {
			t.Fatal("Get on nil cache returned ok=true, want false")
		}
		if val != nil {
			t.Fatalf("Get on nil cache returned %v, want nil", val)
		}
	})

	t.Run("Set on nil cache", func(t *testing.T) {
		// Should not panic.
		c.Set("key", map[string]any{"a": 1})
	})
}
