package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter 基于滑动窗口的请求限流器。
type RateLimiter struct {
	mu        sync.Mutex
	windows   map[string]*windowState
	maxReqs   int
	windowMs  int64
	perIP     bool
}

type windowState struct {
	count       int
	windowStart int64
}

// NewRateLimiter 创建限流器。maxReqs 为每分钟最大请求数，perIP=true 按 IP 限流。
func NewRateLimiter(maxReqs int, perIP bool) *RateLimiter {
	return &RateLimiter{
		windows:  make(map[string]*windowState),
		maxReqs:  maxReqs,
		windowMs: 60 * 1000,
		perIP:    perIP,
	}
}

// Middleware 返回限流中间件。
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := "global"
		if rl.perIP {
			key = extractIP(r.RemoteAddr)
		}

		now := time.Now().UnixMilli()
		rl.mu.Lock()
		ws, ok := rl.windows[key]
		if !ok {
			ws = &windowState{}
			rl.windows[key] = ws
		}
		if now-ws.windowStart >= rl.windowMs {
			ws.count = 0
			ws.windowStart = now
		}
		if ws.count >= rl.maxReqs {
			rl.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			http.Error(w, "请求过于频繁", http.StatusTooManyRequests)
			return
		}
		ws.count++
		rl.mu.Unlock()

		next(w, r)
	}
}

// extractIP 从 RemoteAddr 中提取 IP（去掉端口）。
func extractIP(remoteAddr string) string {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return ip
}
