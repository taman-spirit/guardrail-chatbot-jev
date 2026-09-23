package guardrail

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// VolatileStateKeys are dropped from the cache key by default. A session puts the turn number and
// a running risk score in here, and both change on every turn, so keying on them would mean the
// cache never hits for exactly the repeated messages it exists to serve.
var VolatileStateKeys = map[string]bool{"deployment_context": true}

// CacheKey is a stable key for one check.
//
// Deployment context is dropped by default, because it is per-request context rather than the
// content being judged. If a deployment's metadata genuinely changes what a verdict should be,
// pass an empty ignore set and accept the lower hit rate. A nil ignore set means
// VolatileStateKeys.
//
// Repeat messages are common in a deployed chatbot, and a verdict is a pure function of the
// policy, the surface and the content. The key carries the policy id, so publishing a new pack
// invalidates every entry without anyone having to remember to flush.
func CacheKey(policyID string, surface Surface, state State, subset string, ignore map[string]bool) string {
	if ignore == nil {
		ignore = VolatileStateKeys
	}
	if subset == "" {
		subset = SubsetFull
	}
	kept := make(State, len(state))
	for k, v := range state {
		if !ignore[k] {
			kept[k] = v
		}
	}
	material := strings.Join([]string{policyID, string(surface), subset, string(marshalNoEscape(kept))}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

// VerdictCache is anything that can remember a verdict. Bring your own Redis by implementing it.
type VerdictCache interface {
	Get(key string) (Verdict, bool)
	Put(key string, v Verdict)
}

// LRUCache is an in-process LRU with a TTL, safe to share across goroutines.
//
// Degraded verdicts are never stored. A verdict produced because Jev was unreachable says
// nothing about the content, and caching one would turn a brief outage into a lasting wrong
// answer for that exact message.
type LRUCache struct {
	capacity int
	ttl      time.Duration

	mu      sync.Mutex
	order   *list.List
	entries map[string]*list.Element
	hits    int
	misses  int
}

type cacheEntry struct {
	key      string
	storedAt time.Time
	verdict  Verdict
}

// NewLRUCache holds up to capacity verdicts for ttl each. Zero values take the defaults of 4096
// and five minutes; a negative ttl means entries never expire.
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	if capacity <= 0 {
		capacity = 4096
	}
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &LRUCache{capacity: capacity, ttl: ttl, order: list.New(), entries: map[string]*list.Element{}}
}

// Get returns a cached verdict, marked as cached.
func (c *LRUCache) Get(key string) (Verdict, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		c.misses++
		return Verdict{}, false
	}
	entry := el.Value.(*cacheEntry)
	if c.ttl > 0 && time.Since(entry.storedAt) > c.ttl {
		c.order.Remove(el)
		delete(c.entries, key)
		c.misses++
		return Verdict{}, false
	}
	c.order.MoveToFront(el)
	c.hits++
	v := entry.verdict
	v.Cached = true
	v.LatencyMS = 0
	return v, true
}

// Put stores a verdict, evicting the least recently used entry when full.
func (c *LRUCache) Put(key string, v Verdict) {
	if v.Degraded {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		el.Value = &cacheEntry{key: key, storedAt: time.Now(), verdict: v}
		c.order.MoveToFront(el)
		return
	}
	c.entries[key] = c.order.PushFront(&cacheEntry{key: key, storedAt: time.Now(), verdict: v})
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*cacheEntry).key)
	}
}

// Clear drops every entry.
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order.Init()
	c.entries = map[string]*list.Element{}
}

// Len is the number of entries held.
func (c *LRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// CacheStats is a snapshot of the cache's counters.
type CacheStats struct {
	Size     int     `json:"size"`
	Capacity int     `json:"capacity"`
	Hits     int     `json:"hits"`
	Misses   int     `json:"misses"`
	HitRate  float64 `json:"hit_rate"`
}

// Stats is a snapshot of the cache's counters.
func (c *LRUCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := CacheStats{Size: c.order.Len(), Capacity: c.capacity, Hits: c.hits, Misses: c.misses}
	if total := c.hits + c.misses; total > 0 {
		s.HitRate = round(float64(c.hits)/float64(total), 4)
	}
	return s
}
