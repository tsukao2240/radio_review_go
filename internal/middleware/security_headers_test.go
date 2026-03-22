package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := SecurityHeaders(nextHandler)

	makeRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	t.Run("X-Frame-Options is set to SAMEORIGIN", func(t *testing.T) {
		rr := makeRequest()
		got := rr.Header().Get("X-Frame-Options")
		if got != "SAMEORIGIN" {
			t.Errorf("got %q, want %q", got, "SAMEORIGIN")
		}
	})

	t.Run("X-Content-Type-Options is set to nosniff", func(t *testing.T) {
		rr := makeRequest()
		got := rr.Header().Get("X-Content-Type-Options")
		if got != "nosniff" {
			t.Errorf("got %q, want %q", got, "nosniff")
		}
	})

	t.Run("X-XSS-Protection is set", func(t *testing.T) {
		rr := makeRequest()
		got := rr.Header().Get("X-XSS-Protection")
		if got != "1; mode=block" {
			t.Errorf("got %q, want %q", got, "1; mode=block")
		}
	})

	t.Run("Referrer-Policy is set", func(t *testing.T) {
		rr := makeRequest()
		got := rr.Header().Get("Referrer-Policy")
		if got != "strict-origin-when-cross-origin" {
			t.Errorf("got %q, want %q", got, "strict-origin-when-cross-origin")
		}
	})

	t.Run("Content-Security-Policy is set", func(t *testing.T) {
		rr := makeRequest()
		got := rr.Header().Get("Content-Security-Policy")
		if got == "" {
			t.Error("Content-Security-Policy header is empty")
		}
	})

	t.Run("CSP contains nonce", func(t *testing.T) {
		rr := makeRequest()
		csp := rr.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "'nonce-") {
			t.Errorf("CSP does not contain nonce: %q", csp)
		}
	})

	t.Run("nonce differs between requests", func(t *testing.T) {
		var nonces []string
		for i := 0; i < 5; i++ {
			rr := makeRequest()
			csp := rr.Header().Get("Content-Security-Policy")
			// extract nonce value from "nonce-<value>"
			idx := strings.Index(csp, "'nonce-")
			if idx == -1 {
				t.Fatalf("nonce not found in CSP: %q", csp)
			}
			rest := csp[idx+len("'nonce-"):]
			end := strings.Index(rest, "'")
			if end == -1 {
				t.Fatalf("closing quote not found in CSP: %q", csp)
			}
			nonces = append(nonces, rest[:end])
		}

		seen := make(map[string]bool)
		for _, n := range nonces {
			if seen[n] {
				t.Errorf("duplicate nonce found: %q", n)
			}
			seen[n] = true
		}
	})

	t.Run("nonce is stored in request context", func(t *testing.T) {
		var capturedNonce string
		captureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedNonce = GetNonce(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		SecurityHeaders(captureHandler).ServeHTTP(rr, req)

		if capturedNonce == "" {
			t.Error("nonce was not stored in context")
		}

		// verify the nonce in context matches the one in CSP header
		csp := rr.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "'nonce-"+capturedNonce+"'") {
			t.Errorf("context nonce %q not found in CSP %q", capturedNonce, csp)
		}
	})

	t.Run("response status 200 on success", func(t *testing.T) {
		rr := makeRequest()
		if rr.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
		}
	})
}

func TestGetNonce(t *testing.T) {
	t.Run("returns empty string when nonce not in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		got := GetNonce(req.Context())
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}
