package proxy

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	cfg "dnsproxy/internal/config"

	"github.com/miekg/dns"
)

type upstreamState struct {
	addr     string
	failures int
	lastFail time.Time
}

type cacheEntry struct {
	msg       *dns.Msg
	expiresAt time.Time
	negative  bool
}

type responseCache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
}

func newResponseCache() *responseCache {
	return &responseCache{
		items: make(map[string]cacheEntry),
	}
}

func (c *responseCache) get(key string, now time.Time) (*dns.Msg, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		return nil, false
	}
	return entry.msg.Copy(), true
}

func (c *responseCache) put(key string, msg *dns.Msg, ttl time.Duration, negative bool, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cacheEntry{
		msg:       msg.Copy(),
		expiresAt: now.Add(ttl),
		negative:  negative,
	}
}

func (c *responseCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]cacheEntry)
}

type UpstreamResolver struct {
	upstreams        []upstreamState
	timeout          time.Duration
	retries          int
	maxFails         int
	failWindow       time.Duration
	cache            *responseCache
	cacheEnabled     bool
	cacheTTLOvride   time.Duration
	negativeCacheTTL time.Duration
	now              func() time.Time
	sleep            func(time.Duration)
	exchange         func(req *dns.Msg, addr string, proto string, timeout time.Duration) (*dns.Msg, time.Duration, error)
}

func NewUpstreamResolver(conf *cfg.Config) *UpstreamResolver {
	us := make([]upstreamState, 0, len(conf.NameServers))
	for _, ns := range conf.NameServers {
		us = append(us, upstreamState{addr: ns.IP})
	}
	return &UpstreamResolver{
		upstreams:        us,
		timeout:          time.Duration(conf.UpstreamTimeoutMs) * time.Millisecond,
		retries:          conf.UpstreamRetries,
		maxFails:         conf.UpstreamMaxFails,
		failWindow:       time.Duration(conf.UpstreamFailWindowSec) * time.Second,
		cache:            newResponseCache(),
		cacheEnabled:     conf.CacheEnabled,
		cacheTTLOvride:   time.Duration(conf.CacheTTLOverrideSec) * time.Second,
		negativeCacheTTL: time.Duration(conf.NegativeCacheTTL) * time.Second,
		now:              time.Now,
		sleep:            time.Sleep,
		exchange: func(req *dns.Msg, addr string, proto string, timeout time.Duration) (*dns.Msg, time.Duration, error) {
			c := &dns.Client{Net: proto, Timeout: timeout}
			return c.Exchange(req, addr)
		},
	}
}

func cacheKey(q dns.Question) string {
	return fmt.Sprintf("%s|%d", strings.ToLower(q.Name), q.Qtype)
}

func minTTL(msg *dns.Msg) time.Duration {
	if len(msg.Answer) == 0 {
		return 0
	}
	min := msg.Answer[0].Header().Ttl
	for _, a := range msg.Answer[1:] {
		if a.Header().Ttl < min {
			min = a.Header().Ttl
		}
	}
	return time.Duration(min) * time.Second
}

func (u *UpstreamResolver) Exchange(req *dns.Msg, proto string) (*dns.Msg, error) {
	now := u.now()
	q := req.Question[0]
	key := cacheKey(q)
	if u.cacheEnabled {
		if cached, ok := u.cache.get(key, now); ok {
			cached.Id = req.Id
			return cached, nil
		}
	}

	tries := u.retries
	if tries < 1 {
		tries = 1
	}
	var lastErr error
	for i := 0; i < tries; i++ {
		for idx := range u.upstreams {
			if u.isUnhealthy(&u.upstreams[idx], now) {
				continue
			}
			resp, _, err := u.exchange(req, u.upstreams[idx].addr, proto, u.timeout)
			if err != nil {
				lastErr = err
				u.markFail(&u.upstreams[idx], now)
				continue
			}
			u.markSuccess(&u.upstreams[idx])
			if u.cacheEnabled {
				ttl := minTTL(resp)
				if u.cacheTTLOvride > 0 {
					ttl = u.cacheTTLOvride
				}
				if ttl > 0 {
					u.cache.put(key, resp, ttl, false, now)
				}
			}
			return resp, nil
		}
		u.sleep(10 * time.Millisecond)
	}
	return nil, fmt.Errorf("upstream resolution failed: %w", lastErr)
}

func (u *UpstreamResolver) markFail(s *upstreamState, now time.Time) {
	s.failures++
	s.lastFail = now
}

func (u *UpstreamResolver) markSuccess(s *upstreamState) {
	s.failures = 0
}

func (u *UpstreamResolver) isUnhealthy(s *upstreamState, now time.Time) bool {
	if u.maxFails == 0 {
		return false
	}
	if s.failures >= u.maxFails && now.Sub(s.lastFail) < u.failWindow {
		return true
	}
	if now.Sub(s.lastFail) >= u.failWindow {
		s.failures = 0
	}
	return false
}

// NegativeCache stores negative responses (NXDOMAIN/REFUSED).
func (u *UpstreamResolver) CacheNegative(req *dns.Msg, rcode int) {
	if !u.cacheEnabled || u.negativeCacheTTL <= 0 {
		return
	}
	now := u.now()
	key := cacheKey(req.Question[0])
	msg := new(dns.Msg)
	msg.SetRcode(req, rcode)
	u.cache.put(key, msg, u.negativeCacheTTL, true, now)
}

func (u *UpstreamResolver) ClearCache() {
	u.cache.clear()
}

var errNoHealthy = errors.New("no healthy upstreams")
