package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
)

func TestCSRFSafeGETRepairsInvalidCookie(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-32-bytes-long"))
	req := httptest.NewRequest("GET", "/api/csrf", nil)
	req.AddCookie(&http.Cookie{Name: sessionName, Value: "expired-or-foreign-signature"})
	rr := httptest.NewRecorder()
	CSRFToken(store)(rr, req)
	if rr.Code != 200 {
		t.Fatal("safe token refresh failed")
	}
	var body map[string]string
	if json.Unmarshal(rr.Body.Bytes(), &body) != nil || body["csrf_token"] == "" {
		t.Fatal("fresh token missing")
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session refresh must not be cached")
	}
	handler := CSRFProtection(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, valid := range []bool{true, false} {
		post := httptest.NewRequest("POST", "/favorites", strings.NewReader("{}"))
		for _, cookie := range rr.Result().Cookies() {
			post.AddCookie(cookie)
		}
		token := body["csrf_token"]
		if !valid {
			token = "wrong-token"
		}
		post.Header.Set("X-CSRF-TOKEN", token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, post)
		if valid && response.Code != 204 {
			t.Fatal("repaired session cannot perform an operation")
		}
		if !valid && response.Code != 403 {
			t.Fatal("invalid CSRF token was accepted")
		}
	}
}
