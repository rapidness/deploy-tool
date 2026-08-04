package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(3, false)
	called := 0
	h := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	})
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		h(w, r)
	}
	if called != 3 {
		t.Errorf("expected 3 calls, got %d", called)
	}
}

func TestRateLimiterBlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(2, false)
	h := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		h(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("request %d should be allowed, got %d", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	h(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestRateLimiterPerIP(t *testing.T) {
	rl := NewRateLimiter(1, true)
	h := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r1.RemoteAddr = "1.2.3.4:1234"
	h(w1, r1)
	if w1.Code != http.StatusOK {
		t.Errorf("IP1 first request should be allowed, got %d", w1.Code)
	}
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r2.RemoteAddr = "1.2.3.4:5678"
	h(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 second request should be rate limited, got %d", w2.Code)
	}
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r3.RemoteAddr = "5.6.7.8:9999"
	h(w3, r3)
	if w3.Code != http.StatusOK {
		t.Errorf("IP2 first request should be allowed, got %d", w3.Code)
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	rl := NewRateLimiter(1, false)
	rl.windowMs = 100
	h := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	h(w1, r1)
	if w1.Code != http.StatusOK {
		t.Errorf("first request should be allowed")
	}
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	h(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be rate limited")
	}
	time.Sleep(150 * time.Millisecond)
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	h(w3, r3)
	if w3.Code != http.StatusOK {
		t.Errorf("request after window reset should be allowed")
	}
}
