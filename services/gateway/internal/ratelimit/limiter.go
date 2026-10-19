// Package ratelimit la token bucket trong bo nho, theo khoa (nguoi dung hoac IP).
//
// Moi replica gateway giu bucket rieng nen gioi han thuc te = gioi han x so replica. Day la
// lop chan tho, re, khong them phu thuoc vao Redis tren moi request; gioi han chinh xac theo
// nguoi va theo su kien thuoc ve antibot (EV-174).
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// idleTTL: bucket khong dung qua lau thi da day lai, xoa di khong doi ket qua.
const idleTTL = 10 * time.Minute

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter cho phep PerSecond request/giay, dot bien toi Burst, tren moi khoa.
type Limiter struct {
	mu        sync.Mutex
	perSecond float64
	burst     float64
	buckets   map[string]*bucket
	now       func() time.Time
	lastSweep time.Time
}

// New tao Limiter. now nil thi dung time.Now.
func New(perSecond float64, burst int, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{
		perSecond: perSecond,
		burst:     float64(burst),
		buckets:   map[string]*bucket{},
		now:       now,
		lastSweep: now(),
	}
}

// Allow lay mot token cua key. Het token thi tra false va thoi gian cho toi khi co token.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(l.burst, b.tokens+elapsed*l.perSecond)
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.perSecond * float64(time.Second))
	return false, wait
}

// sweep xoa bucket nhan roi, toi da mot lan moi phut, de map khong phinh theo so IP da gap.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if now.Sub(b.last) > idleTTL {
			delete(l.buckets, k)
		}
	}
}

// Len tra ve so bucket dang giu (cho test va metric).
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
