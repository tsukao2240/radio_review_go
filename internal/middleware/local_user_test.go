package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalUserAccess(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want int
	}{
		{"127.0.0.1:1234", http.StatusNoContent},
		{"[::1]:1234", http.StatusNoContent},
		{"192.168.1.20:1234", http.StatusForbidden},
		{"invalid", http.StatusForbidden},
	} {
		t.Run(tc.addr, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://localhost/favorites", nil)
			req.RemoteAddr = tc.addr
			rr := httptest.NewRecorder()
			LocalUser(42)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id, ok := GetUserID(r.Context())
				if !ok || id != 42 {
					t.Fatal("local owner missing")
				}
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d", rr.Code, tc.want)
			}
		})
	}
}
