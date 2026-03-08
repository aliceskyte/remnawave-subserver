package panel

import (
	"sync"
	"time"
)

const defaultMaxCacheSize = 10000

type Cache struct {
	ttl     time.Duration
	maxSize int
	mu      sync.RWMutex
	items   map[string]cacheEntry
}

type cacheEntry struct {
	expires time.Time
	value   map[string]any
}

func NewCache(ttl time.Duration, maxSize ...int) *Cache {
	if ttl <= 0 {
		return nil
	}
	ms := defaultMaxCacheSize
	if len(maxSize) > 0 && maxSize[0] > 0 {
		ms = maxSize[0]
	}
	return &Cache{
		ttl:     ttl,
		maxSize: ms,
		items:   make(map[string]cacheEntry),
	}
}

func (c *Cache) Get(key string) (map[string]any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expires) {
		c.mu.Lock()
		if current, ok := c.items[key]; ok && time.Now().After(current.expires) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return nil, false
	}
	return entry.value, true
}

func (c *Cache) Set(key string, value map[string]any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict expired entries if at capacity.
	if len(c.items) >= c.maxSize {
		c.sweepExpired()
	}

	// If still at capacity, evict the oldest entry.
	if len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	c.items[key] = cacheEntry{value: value, expires: time.Now().Add(c.ttl)}
}

// sweepExpired removes all expired entries.
func (c *Cache) sweepExpired() {
	now := time.Now()
	for k, entry := range c.items {
		if now.After(entry.expires) {
			delete(c.items, k)
		}
	}
}

// evictOldest removes the entry with the earliest expiration time.
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, entry := range c.items {
		if first || entry.expires.Before(oldestTime) {
			oldestKey = k
			oldestTime = entry.expires
			first = false
		}
	}
	if !first {
		delete(c.items, oldestKey)
	}
}
