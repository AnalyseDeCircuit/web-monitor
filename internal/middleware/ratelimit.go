// Package middleware 提供HTTP中间件
package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter 限流器结构
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter 创建新的限流器
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
	}
}

// GetLimiter 获取或创建指定key的限流器
func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, exists := rl.limiters[key]; exists {
		return limiter
	}

	limiter := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[key] = limiter
	return limiter
}

// Cleanup 清理过期的限流器
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 这里可以添加清理逻辑，但为了简单起见，我们暂时不实现
	// 在实际生产环境中，应该定期清理长时间未使用的限流器
}

// RateLimitMiddleware 限流中间件
func RateLimitMiddleware(rl *RateLimiter, keyFunc func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			limiter := rl.GetLimiter(key)

			if !limiter.Allow() {
				http.Error(w, "请求过于频繁，请稍后再试", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// IPBasedKey 基于IP地址生成key
func IPBasedKey(r *http.Request) string {
	// 获取真实IP（考虑代理）
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = forwarded
	}
	return ip
}

// UserBasedKey 基于用户ID生成key
func UserBasedKey(r *http.Request) string {
	// 这里可以从JWT令牌或会话中获取用户ID
	// 暂时返回IP作为fallback
	return IPBasedKey(r)
}

// DefaultRateLimiter 默认限流器配置
var DefaultRateLimiter = NewRateLimiter(rate.Limit(10), 20) // 10 req/s, burst 20
