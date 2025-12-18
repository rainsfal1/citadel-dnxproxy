package proxy

import (
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	cfg "dnsproxy/internal/config"

	"github.com/miekg/dns"
)

type testClock struct {
	now time.Time
}

func (f *testClock) Now() time.Time {
	return f.now
}

func (f *testClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

type upstreamBehavior struct {
	delay time.Duration
	resp  *dns.Msg
	err   error
	hits  *int32
}

func makeResp(q dns.Question, ip string, ttl uint32) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(&dns.Msg{Question: []dns.Question{q}})
	h := dns.RR_Header{
		Name:   q.Name,
		Rrtype: dns.TypeA,
		Class:  dns.ClassINET,
		Ttl:    ttl,
	}
	m.Answer = []dns.RR{&dns.A{Hdr: h, A: net.ParseIP(ip)}}
	return m
}

func stubExchange(clock *testClock, behaviors map[string]*upstreamBehavior) func(req *dns.Msg, addr string, proto string, timeout time.Duration) (*dns.Msg, time.Duration, error) {
	return func(req *dns.Msg, addr string, proto string, timeout time.Duration) (*dns.Msg, time.Duration, error) {
		b, ok := behaviors[addr]
		if !ok {
			return nil, 0, fmt.Errorf("no upstream for %s", addr)
		}
		if b.hits == nil {
			b.hits = new(int32)
		}
		atomic.AddInt32(b.hits, 1)
		if b.err != nil {
			return nil, 0, b.err
		}
		if b.delay > timeout {
			return nil, 0, fmt.Errorf("timeout talking to %s", addr)
		}
		if b.delay > 0 {
			clock.Advance(b.delay)
		}
		if b.resp == nil {
			return nil, 0, fmt.Errorf("no response configured for %s", addr)
		}
		resp := b.resp.Copy()
		resp.Id = req.Id
		return resp, b.delay, nil
	}
}

func TestUpstreamFailoverAndCircuitBreaker(t *testing.T) {
	slowAddr := "slow-upstream"
	fastAddr := "fast-upstream"
	clock := &testClock{now: time.Now()}

	conf := &cfg.Config{
		NameServers:           []cfg.NameServer{{IP: slowAddr}, {IP: fastAddr}},
		UpstreamTimeoutMs:     10,
		UpstreamRetries:       1,
		UpstreamMaxFails:      1,
		UpstreamFailWindowSec: 60,
	}
	resolver := NewUpstreamResolver(conf)
	resolver.now = clock.Now
	resolver.sleep = clock.Advance

	msg := new(dns.Msg)
	msg.SetQuestion("failover.test.", dns.TypeA)
	fastResp := makeResp(msg.Question[0], "203.0.113.10", 5)

	behaviors := map[string]*upstreamBehavior{
		slowAddr: {delay: 50 * time.Millisecond, hits: new(int32)},
		fastAddr: {resp: fastResp, hits: new(int32)},
	}
	resolver.exchange = stubExchange(clock, behaviors)

	resp, err := resolver.Exchange(msg, "udp")
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}
	if len(resp.Answer) == 0 {
		t.Fatalf("expected answer from fast upstream")
	}
	if atomic.LoadInt32(behaviors[fastAddr].hits) != 1 {
		t.Fatalf("expected fast upstream to be hit once, got %d", atomic.LoadInt32(behaviors[fastAddr].hits))
	}
	if atomic.LoadInt32(behaviors[slowAddr].hits) == 0 {
		t.Fatalf("expected slow upstream to receive at least one attempt")
	}
	if resolver.upstreams[0].failures != 1 {
		t.Fatalf("expected slow upstream failures=1, got %d", resolver.upstreams[0].failures)
	}

	// Second request should skip the unhealthy slow upstream (fail count should not increase)
	_, err = resolver.Exchange(msg, "udp")
	if err != nil {
		t.Fatalf("second exchange failed: %v", err)
	}
	if resolver.upstreams[0].failures != 1 {
		t.Fatalf("expected slow upstream failures to stay at 1, got %d", resolver.upstreams[0].failures)
	}
}

func TestUpstreamCachingAndExpiry(t *testing.T) {
	addr := "cache-upstream"
	clock := &testClock{now: time.Now()}
	conf := &cfg.Config{
		NameServers:           []cfg.NameServer{{IP: addr}},
		UpstreamTimeoutMs:     100,
		UpstreamRetries:       1,
		UpstreamMaxFails:      0,
		CacheEnabled:          true,
		CacheTTLOverrideSec:   1,
		NegativeCacheTTL:      1,
		UpstreamFailWindowSec: 30,
	}
	resolver := NewUpstreamResolver(conf)
	resolver.now = clock.Now
	resolver.sleep = clock.Advance
	msg := new(dns.Msg)
	msg.SetQuestion("cache.test.", dns.TypeA)
	resp := makeResp(msg.Question[0], "198.51.100.10", 5)
	behaviors := map[string]*upstreamBehavior{
		addr: {resp: resp, hits: new(int32)},
	}
	resolver.exchange = stubExchange(clock, behaviors)

	if _, err := resolver.Exchange(msg, "udp"); err != nil {
		t.Fatalf("first exchange failed: %v", err)
	}
	if atomic.LoadInt32(behaviors[addr].hits) != 1 {
		t.Fatalf("expected first exchange to hit upstream once, got %d", atomic.LoadInt32(behaviors[addr].hits))
	}

	if _, err := resolver.Exchange(msg, "udp"); err != nil {
		t.Fatalf("cached exchange failed: %v", err)
	}
	if atomic.LoadInt32(behaviors[addr].hits) != 1 {
		t.Fatalf("expected cached exchange to avoid upstream, got %d hits", atomic.LoadInt32(behaviors[addr].hits))
	}

	clock.Advance(1200 * time.Millisecond)

	if _, err := resolver.Exchange(msg, "udp"); err != nil {
		t.Fatalf("post-expiry exchange failed: %v", err)
	}
	if atomic.LoadInt32(behaviors[addr].hits) != 2 {
		t.Fatalf("expected cache expiry to trigger second upstream hit, got %d", atomic.LoadInt32(behaviors[addr].hits))
	}
}

func TestUpstreamNegativeCache(t *testing.T) {
	addr := "neg-upstream"
	clock := &testClock{now: time.Now()}
	conf := &cfg.Config{
		NameServers:           []cfg.NameServer{{IP: addr}},
		UpstreamTimeoutMs:     100,
		UpstreamRetries:       1,
		UpstreamMaxFails:      0,
		CacheEnabled:          true,
		NegativeCacheTTL:      1,
		UpstreamFailWindowSec: 30,
	}
	resolver := NewUpstreamResolver(conf)
	resolver.now = clock.Now
	resolver.sleep = clock.Advance
	resp := makeResp(dns.Question{Name: "negative.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}, "192.0.2.10", 5)
	behaviors := map[string]*upstreamBehavior{
		addr: {resp: resp, hits: new(int32)},
	}
	resolver.exchange = stubExchange(clock, behaviors)

	msg := new(dns.Msg)
	msg.SetQuestion("negative.test.", dns.TypeA)

	resolver.CacheNegative(msg, dns.RcodeServerFailure)

	respMsg, err := resolver.Exchange(msg, "udp")
	if err != nil {
		t.Fatalf("exchange failed with cached negative: %v", err)
	}
	if respMsg.Rcode != dns.RcodeServerFailure {
		t.Fatalf("expected SERVFAIL from negative cache, got %d", respMsg.Rcode)
	}
	if atomic.LoadInt32(behaviors[addr].hits) != 0 {
		t.Fatalf("expected no upstream hits when serving from negative cache, got %d", atomic.LoadInt32(behaviors[addr].hits))
	}

	clock.Advance(1100 * time.Millisecond)

	_, _ = resolver.Exchange(msg, "udp")
	if atomic.LoadInt32(behaviors[addr].hits) == 0 {
		t.Fatalf("expected upstream to be hit after negative cache expiry")
	}
}
