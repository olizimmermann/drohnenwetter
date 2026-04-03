package handler

import (
	"sync"
	"testing"
	"time"
)

func TestCacheKeyRounding(t *testing.T) {
	tests := []struct {
		lat1, lon1 float64
		lat2, lon2 float64
		sameKey    bool
	}{
		{52.52014, 13.40500, 52.52019, 13.40500, true},  // ~0.5m apart
		{52.520, 13.405, 52.521, 13.405, false},          // ~111m apart
		{52.5205, 13.4050, 52.5205, 13.4050, true},       // identical
		{-33.8688, 151.2093, -33.8688, 151.2093, true},   // southern hemisphere
	}

	for _, tc := range tests {
		k1 := cacheKey(tc.lat1, tc.lon1)
		k2 := cacheKey(tc.lat2, tc.lon2)
		if (k1 == k2) != tc.sameKey {
			t.Errorf("cacheKey(%.5f,%.5f)=%s vs cacheKey(%.5f,%.5f)=%s — sameKey=%v want %v",
				tc.lat1, tc.lon1, k1, tc.lat2, tc.lon2, k2, k1 == k2, tc.sameKey)
		}
	}
}

func TestCacheMiss(t *testing.T) {
	c := newResultCache(100, time.Minute)
	_, ok := c.get("52.520,13.405")
	if ok {
		t.Error("expected miss on empty cache")
	}
}

func TestCacheHit(t *testing.T) {
	c := newResultCache(100, time.Minute)
	data := &allFetched{}
	c.set("52.520,13.405", data)
	got, ok := c.get("52.520,13.405")
	if !ok {
		t.Fatal("expected hit")
	}
	if got != data {
		t.Error("returned different pointer")
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	c := newResultCache(100, 10*time.Millisecond)
	c.set("key", &allFetched{})
	time.Sleep(20 * time.Millisecond)
	_, ok := c.get("key")
	if ok {
		t.Error("expected miss after TTL expiry")
	}
}

func TestCacheMaxSize(t *testing.T) {
	c := newResultCache(3, time.Minute)

	c.set("a", &allFetched{})
	time.Sleep(time.Millisecond)
	c.set("b", &allFetched{})
	time.Sleep(time.Millisecond)
	c.set("c", &allFetched{})
	time.Sleep(time.Millisecond)

	// Adding a 4th should evict the oldest ("a")
	c.set("d", &allFetched{})

	if _, ok := c.get("a"); ok {
		t.Error("expected 'a' to be evicted")
	}
	if _, ok := c.get("b"); !ok {
		t.Error("expected 'b' to still exist")
	}
	if _, ok := c.get("d"); !ok {
		t.Error("expected 'd' to exist")
	}
}

func TestCacheConcurrent(t *testing.T) {
	c := newResultCache(1000, time.Minute)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		key := cacheKey(float64(i)/100, float64(i)/100)
		go func() {
			defer wg.Done()
			c.set(key, &allFetched{})
		}()
		go func() {
			defer wg.Done()
			c.get(key)
		}()
	}
	wg.Wait()
}
