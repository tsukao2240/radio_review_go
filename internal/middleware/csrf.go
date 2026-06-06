package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/gorilla/sessions"
)

const csrfSessionKey = "_csrf"

func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// EnsureCSRFToken returns the session CSRF token, creating and saving it if needed.
func EnsureCSRFToken(r *http.Request, w http.ResponseWriter, store sessions.Store) string {
	session, err := store.Get(r, sessionName)
	if err != nil {
		return ""
	}
	if token, ok := session.Values[csrfSessionKey].(string); ok && token != "" {
		return token
	}
	token := newCSRFToken()
	if token == "" {
		return ""
	}
	session.Values[csrfSessionKey] = token
	if err := session.Save(r, w); err != nil {
		return ""
	}
	return token
}

// CSRFProtection validates unsafe requests against the token stored in session.
func CSRFProtection(store sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
				next.ServeHTTP(w, r)
				return
			}

			session, err := store.Get(r, sessionName)
			if err != nil {
				http.Error(w, "csrf token invalid", http.StatusForbidden)
				return
			}
			expected, _ := session.Values[csrfSessionKey].(string)
			actual := csrfTokenFromRequest(r)
			if expected == "" || actual == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
				http.Error(w, "csrf token invalid", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func csrfTokenFromRequest(r *http.Request) string {
	if token := r.Header.Get("X-CSRF-TOKEN"); token != "" {
		return token
	}
	if token := r.Header.Get("X-CSRF-Token"); token != "" {
		return token
	}
	if err := r.ParseForm(); err != nil {
		return ""
	}
	return r.FormValue("_csrf")
}
