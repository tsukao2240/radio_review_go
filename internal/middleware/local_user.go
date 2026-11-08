package middleware

import (
	"context"
	"net"
	"net/http"
)

// LocalUser assigns the existing desktop owner's data to every local request.
func LocalUser(userID int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil || !net.ParseIP(host).IsLoopback() {
				http.Error(w, "Local access only", http.StatusForbidden)
				return
			}
			ctx := context.WithValue(r.Context(), UserContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
