package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a simple in-memory, per-key sliding-window limiter. It
// fits the single-binary model: no external store, no dependency. It is
// used to slow down credential guessing on the login endpoint.
//
// rateLimiter, basit bir bellek-içi, anahtar-başına kayan-pencere
// sınırlayıcıdır. Tek-binary modeline uyar: harici depo yok, bağımlılık
// yok. Giriş uç noktasında kimlik bilgisi tahminini yavaşlatmak için
// kullanılır.
type rateLimiter struct {
	mu        sync.Mutex
	hits      map[string][]time.Time
	limit     int
	window    time.Duration
	lastSweep time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		hits:   make(map[string][]time.Time),
		limit:  limit,
		window: window,
	}
}

// reset forgets every recorded attempt, under the limiter's own lock, so a
// limiter shared with running goroutines is never replaced underneath them.
func (l *rateLimiter) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits = make(map[string][]time.Time)
	l.lastSweep = time.Time{}
}

// allow records an attempt for key and reports whether it is within the
// limit. Timestamps older than the window are pruned on each call.
// allow, key için bir deneme kaydeder ve limitin içinde olup olmadığını
// bildirir. Pencereden eski zaman damgaları her çağrıda budanır.
func (l *rateLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Traffic from one-off keys (for example, scanned source addresses) must
	// not make the map grow forever. Sweep the complete map at most once per
	// window; the hot path still only prunes the current key.
	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= l.window {
		for candidate, timestamps := range l.hits {
			kept := timestamps[:0]
			for _, timestamp := range timestamps {
				if timestamp.After(cutoff) {
					kept = append(kept, timestamp)
				}
			}
			if len(kept) == 0 {
				delete(l.hits, candidate)
				continue
			}
			l.hits[candidate] = kept
		}
		l.lastSweep = now
	}

	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= l.limit {
		l.hits[key] = recent
		return false
	}

	l.hits[key] = append(recent, now)
	return true
}

// clientIP extracts the remote IP without the port. Behind a trusted
// reverse proxy this would consult X-Forwarded-For; direct exposure uses
// RemoteAddr as the source of truth.
// clientIP, uzak IP'yi port olmadan çıkarır. Güvenilir bir ters vekil
// arkasında X-Forwarded-For'a bakılırdı; doğrudan erişimde RemoteAddr
// doğruluğun kaynağıdır.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// blocked reports whether key has reached its limit in the current window,
// without recording an attempt. The API token layer counts only failed
// sign-ins (allow on a failure) and asks blocked before every attempt.
// blocked, bir deneme kaydetmeden anahtarın sınıra ulaşıp ulaşmadığını söyler.
func (l *rateLimiter) blocked(key string) bool {
	cutoff := time.Now().Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := 0
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent++
		}
	}
	return recent >= l.limit
}
