package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSelectedTimefreeArea(t *testing.T) {
	for _, tc := range []struct {
		name, query, cookie, want string
		persist                   bool
	}{
		{name: "default", want: "JP13"},
		{name: "explicit area", query: "JP27", want: "JP27", persist: true},
		{name: "return to tab", cookie: "JP27", want: "JP27"},
		{name: "explicit area wins", query: "JP1", cookie: "JP27", want: "JP1", persist: true},
		{name: "invalid query", query: "invalid", cookie: "JP27", want: "JP27"},
		{name: "invalid cookie", cookie: "invalid", want: "JP13"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/timefree?area="+tc.query, nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: timefreeAreaCookie, Value: tc.cookie})
			}
			rr := httptest.NewRecorder()
			if got := selectedTimefreeArea(rr, req); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
			cookies := rr.Result().Cookies()
			if tc.persist && (len(cookies) != 1 || cookies[0].Value != tc.want || cookies[0].MaxAge <= 0) {
				t.Fatal("selection not persisted")
			}
		})
	}
}
