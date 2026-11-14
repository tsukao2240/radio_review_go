package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
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
		// CookieStore returns a fresh usable session together with its decode error.
		// Replace stale/foreign cookies on a safe GET rather than trapping the user in 403.
		if session == nil {
			return ""
		}
		session.Values = make(map[interface{}]interface{})
		session.IsNew = true
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
				writeCSRFError(w)
				return
			}
			expected, _ := session.Values[csrfSessionKey].(string)
			actual := csrfTokenFromRequest(r)
			if expected == "" || actual == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
				writeCSRFError(w)
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

func writeCSRFError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": "csrf_token_invalid", "error": "画面の接続情報を更新してください"})
}

// CSRFToken refreshes a signed session token without accepting an unsafe request.
func CSRFToken(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		token := EnsureCSRFToken(r, w, store)
		if token == "" {
			http.Error(w, "Session unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"csrf_token": token})
	}
}
