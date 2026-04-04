// internal/middleware/ratelimit.go
package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter 限流中间件
type RateLimiter struct {
	limiters sync.Map // map[string]*rate.Limiter
	rps      rate.Limit
	burst    int
}

// NewRateLimiter 创建限流器
// rps: 每秒请求数, burst: 突发容量
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		rps:   rate.Limit(rps),
		burst: burst,
	}

	// 定期清理过期 limiter（避免内存泄漏）
	go rl.cleanup()

	slog.Info("rate limiter initialized", "rps", rps, "burst", burst)

	return rl
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	if limiter, ok := rl.limiters.Load(key); ok {
		return limiter.(*rate.Limiter)
	}

	limiter := rate.NewLimiter(rl.rps, rl.burst)
	rl.limiters.Store(key, limiter)
	return limiter
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			slog.Warn("rate limit exceeded", "ip", ip, "path", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":{"code":429,"message":"rate limit exceeded","status":"RESOURCE_EXHAUSTED"}}`,
				http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractIP(r *http.Request) string {
	// 1. 检查 X-Forwarded-For（代理/负载均衡场景）
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// 2. 检查 X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// 3. 直接连接
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// cleanup 定期清理闲置的 limiter
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		count := 0

		rl.limiters.Range(func(key, value interface{}) bool {
			limiter := value.(*rate.Limiter)

			// 如果 limiter 已经闲置（令牌桶满），删除
			if limiter.Tokens() >= float64(rl.burst) {
				rl.limiters.Delete(key)
				count++
			}
			return true
		})

		if count > 0 {
			slog.Debug("rate limiter cleanup", "deleted", count)
		}
	}
}
