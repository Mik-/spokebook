package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// limiter is a token bucket per key: burst attempts at once, then one more
// every interval.
type limiter struct {
	burst    float64
	interval time.Duration

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxBuckets bounds memory; beyond it, full buckets are dropped.
const maxBuckets = 10_000

func newLimiter(burst int, interval time.Duration) *limiter {
	return &limiter{burst: float64(burst), interval: interval, buckets: make(map[string]*bucket)}
}

// allow takes a token for key and reports whether there was one.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.buckets) >= maxBuckets {
		for k, b := range l.buckets {
			if l.refill(b, now) >= l.burst {
				delete(l.buckets, k)
			}
		}
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = l.refill(b, now)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *limiter) refill(b *bucket, now time.Time) float64 {
	return min(l.burst, b.tokens+float64(now.Sub(b.last))/float64(l.interval))
}

// clientIP is the address the request came from. If it came through one of
// the trusted proxies, it is the rightmost address in X-Forwarded-For that is
// not a trusted proxy; addresses left of it may be forged by the client.
// Without trusted proxies, the limit behind a reverse proxy applies to all
// clients together.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !isTrusted(host, trusted) {
		return host
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(hops[i])
		if hop == "" {
			continue
		}
		if !isTrusted(hop, trusted) {
			return hop
		}
		host = hop
	}
	return host
}

func isTrusted(ip string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ParseTrustedProxies parses a comma-separated list of IP addresses and
// CIDR ranges such as "192.168.1.10, 172.16.0.0/12".
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if strings.Contains(f, "/") {
			p, err := netip.ParsePrefix(f)
			if err != nil {
				return nil, fmt.Errorf("trusted proxy %q: %w", f, err)
			}
			prefixes = append(prefixes, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(f)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q: %w", f, err)
		}
		a = a.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(a, a.BitLen()))
	}
	return prefixes, nil
}
