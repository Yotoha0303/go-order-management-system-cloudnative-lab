package ratelimit

import (
	"math"
	"sync"
	"time"
)

type Config struct {
	PerClientRate  float64
	PerClientBurst int
	GlobalRate     float64
	GlobalBurst    int
	MaxClients     int
	InactiveTTL    time.Duration
	CleanupEvery   time.Duration
}

func (cfg Config) normalized() Config {
	if cfg.PerClientRate <= 0 {
		cfg.PerClientRate = 50
	}
	if cfg.PerClientBurst <= 0 {
		cfg.PerClientBurst = 100
	}
	if cfg.GlobalRate <= 0 {
		cfg.GlobalRate = 500
	}
	if cfg.GlobalBurst <= 0 {
		cfg.GlobalBurst = 1000
	}
	if cfg.MaxClients <= 0 {
		cfg.MaxClients = 10000
	}
	if cfg.InactiveTTL <= 0 {
		cfg.InactiveTTL = 10 * time.Minute
	}
	if cfg.CleanupEvery <= 0 {
		cfg.CleanupEvery = time.Minute
	}
	return cfg
}

type bucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

type Limiter struct {
	mu sync.Mutex

	cfg         Config
	now         func() time.Time
	global      bucket
	clients     map[string]*bucket
	lastCleanup time.Time
}

func New(cfg Config) *Limiter {
	cfg = cfg.normalized()
	now := time.Now()
	return &Limiter{
		cfg: cfg,
		now: time.Now,
		global: bucket{
			tokens:   float64(cfg.GlobalBurst),
			last:     now,
			lastSeen: now,
		},
		clients:     make(map[string]*bucket),
		lastCleanup: now,
	}
}

func (limiter *Limiter) Allow(key string) (bool, time.Duration) {
	if limiter == nil {
		return true, 0
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := limiter.now()
	limiter.cleanupLocked(now)
	client := limiter.clientLocked(key, now)
	refill(&limiter.global, limiter.cfg.GlobalRate, limiter.cfg.GlobalBurst, now)
	refill(client, limiter.cfg.PerClientRate, limiter.cfg.PerClientBurst, now)
	limiter.global.lastSeen = now
	client.lastSeen = now

	globalWait := waitDuration(limiter.global.tokens, limiter.cfg.GlobalRate)
	clientWait := waitDuration(client.tokens, limiter.cfg.PerClientRate)
	if globalWait > 0 || clientWait > 0 {
		if clientWait > globalWait {
			return false, clientWait
		}
		return false, globalWait
	}

	limiter.global.tokens--
	client.tokens--
	return true, 0
}

func (limiter *Limiter) clientLocked(key string, now time.Time) *bucket {
	if key == "" {
		key = "unknown"
	}
	if existing, ok := limiter.clients[key]; ok {
		return existing
	}
	if len(limiter.clients) >= limiter.cfg.MaxClients {
		limiter.evictOldestLocked()
	}
	created := &bucket{
		tokens:   float64(limiter.cfg.PerClientBurst),
		last:     now,
		lastSeen: now,
	}
	limiter.clients[key] = created
	return created
}

func (limiter *Limiter) cleanupLocked(now time.Time) {
	if now.Sub(limiter.lastCleanup) < limiter.cfg.CleanupEvery {
		return
	}
	cutoff := now.Add(-limiter.cfg.InactiveTTL)
	for key, client := range limiter.clients {
		if client.lastSeen.Before(cutoff) {
			delete(limiter.clients, key)
		}
	}
	limiter.lastCleanup = now
}

func (limiter *Limiter) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for key, client := range limiter.clients {
		if oldestKey == "" || client.lastSeen.Before(oldest) {
			oldestKey = key
			oldest = client.lastSeen
		}
	}
	if oldestKey != "" {
		delete(limiter.clients, oldestKey)
	}
}

func refill(target *bucket, rate float64, burst int, now time.Time) {
	if target.last.IsZero() {
		target.last = now
		target.tokens = float64(burst)
		return
	}
	elapsed := now.Sub(target.last).Seconds()
	if elapsed <= 0 {
		return
	}
	target.tokens = math.Min(float64(burst), target.tokens+elapsed*rate)
	target.last = now
}

func waitDuration(tokens float64, rate float64) time.Duration {
	if tokens >= 1 {
		return 0
	}
	seconds := (1 - tokens) / rate
	if seconds <= 0 {
		return 0
	}
	return time.Duration(math.Ceil(seconds * float64(time.Second)))
}
