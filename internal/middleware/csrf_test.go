package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
)

func TestCSRFProtection(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-32-bytes-long"))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := CSRFProtection(store)(next)

	setupReq := httptest.NewRequest(http.MethodGet, "/", nil)
	setupRR := httptest.NewRecorder()
	token := EnsureCSRFToken(setupReq, setupRR, store)
	if token == "" {
		t.Fatal("expected token")
	}
	cookies := setupRR.Result().Cookies()

	t.Run("GET bypasses validation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("got %d, want 204", rr.Code)
		}
	})

	t.Run("POST rejects missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true}`))
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("got %d, want 403", rr.Code)
		}
		if body, _ := io.ReadAll(req.Body); string(body) != `{"ok":true}` {
			t.Fatalf("body was consumed: %q", string(body))
		}
	})

	t.Run("POST accepts X-CSRF-TOKEN header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true}`))
		req.Header.Set("X-CSRF-TOKEN", token)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("got %d, want 204", rr.Code)
		}
	})

	t.Run("POST accepts _csrf form field", func(t *testing.T) {
		form := url.Values{"_csrf": {token}}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("got %d, want 204", rr.Code)
		}
	})
}
