package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("requests within limit pass through", func(t *testing.T) {
		limit := 3
		mw := RateLimit(limit, time.Minute)
		handler := mw(okHandler)

		for i := 1; i <= limit; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("request %d: got status %d, want %d", i, rr.Code, http.StatusOK)
			}
		}
	})

	t.Run("request exceeding limit returns 429", func(t *testing.T) {
		limit := 2
		mw := RateLimit(limit, time.Minute)
		handler := mw(okHandler)

		// consume the limit
		for i := 0; i < limit; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "10.0.0.1:9999"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}

		// next request should be rejected
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("got status %d, want %d", rr.Code, http.StatusTooManyRequests)
		}
	})

	t.Run("different IPs have independent counters", func(t *testing.T) {
		limit := 1
		mw := RateLimit(limit, time.Minute)
		handler := mw(okHandler)

		for _, ip := range []string{"1.2.3.4:1000", "5.6.7.8:2000"} {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = ip
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("IP %s: got status %d, want %d", ip, rr.Code, http.StatusOK)
			}
		}
	})

	t.Run("X-Forwarded-For is respected", func(t *testing.T) {
		limit := 1
		mw := RateLimit(limit, time.Minute)
		handler := mw(okHandler)

		// First request with X-Forwarded-For: consume the limit
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:80"
		req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("first request: got %d, want %d", rr.Code, http.StatusOK)
		}

		// Second request from same forwarded IP should be rejected
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		req2.RemoteAddr = "127.0.0.1:80"
		req2.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
		rr2 := httptest.NewRecorder()
		handler.ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusTooManyRequests {
			t.Errorf("second request: got %d, want %d", rr2.Code, http.StatusTooManyRequests)
		}
	})

	t.Run("window reset allows requests again", func(t *testing.T) {
		limit := 1
		window := 50 * time.Millisecond
		mw := RateLimit(limit, window)
		handler := mw(okHandler)

		ip := "172.16.0.1:4321"

		// consume the limit
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		req1.RemoteAddr = ip
		rr1 := httptest.NewRecorder()
		handler.ServeHTTP(rr1, req1)
		if rr1.Code != http.StatusOK {
			t.Fatalf("first request: got %d, want %d", rr1.Code, http.StatusOK)
		}

		// exceed the limit
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		req2.RemoteAddr = ip
		rr2 := httptest.NewRecorder()
		handler.ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusTooManyRequests {
			t.Fatalf("second request: got %d, want %d", rr2.Code, http.StatusTooManyRequests)
		}

		// wait for the window to expire
		time.Sleep(window + 10*time.Millisecond)

		// should pass again after window reset
		req3 := httptest.NewRequest(http.MethodGet, "/", nil)
		req3.RemoteAddr = ip
		rr3 := httptest.NewRecorder()
		handler.ServeHTTP(rr3, req3)
		if rr3.Code != http.StatusOK {
			t.Errorf("after reset: got %d, want %d", rr3.Code, http.StatusOK)
		}
	})
}

func TestExtractIP(t *testing.T) {
	t.Run("uses RemoteAddr when no X-Forwarded-For", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.10:5678"
		got := extractIP(req)
		if got != "192.168.1.10" {
			t.Errorf("got %q, want %q", got, "192.168.1.10")
		}
	})

	t.Run("uses first IP from X-Forwarded-For", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:80"
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1, 10.0.0.2")
		got := extractIP(req)
		if got != "203.0.113.1" {
			t.Errorf("got %q, want %q", got, "203.0.113.1")
		}
	})
}
