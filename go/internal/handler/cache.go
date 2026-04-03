package handler

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

type cacheEntry struct {
	data      *allFetched
	createdAt time.Time
}

type resultCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

func newResultCache(maxSize int, ttl time.Duration) *resultCache {
	return &resultCache{
		entries: make(map[string]*cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// cacheKey rounds coordinates to 3 decimal places (~111 m) to group nearby lookups.
func cacheKey(lat, lon float64) string {
	return fmt.Sprintf("%.3f,%.3f",
		math.Round(lat*1000)/1000,
		math.Round(lon*1000)/1000,
	)
}

func (c *resultCache) get(key string) (*allFetched, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Since(entry.createdAt) > c.ttl {
		return nil, false // expired — lazy eviction
	}
	return entry.data, true
}

func (c *resultCache) set(key string, data *allFetched) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest entry if at capacity
	if len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, e := range c.entries {
			if oldestKey == "" || e.createdAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.createdAt
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}

	c.entries[key] = &cacheEntry{
		data:      data,
		createdAt: time.Now(),
	}
}

// startCleanup runs a background goroutine that sweeps expired entries.
func (c *resultCache) startCleanup(interval time.Duration) {
	go func() {
		for {
			time.Sleep(interval)
			c.mu.Lock()
			now := time.Now()
			evicted := 0
			for k, e := range c.entries {
				if now.Sub(e.createdAt) > c.ttl {
					delete(c.entries, k)
					evicted++
				}
			}
			c.mu.Unlock()
			if evicted > 0 {
				log.Printf("[cache] evicted %d expired entries, %d remaining", evicted, len(c.entries))
			}
		}
	}()
}
