package handler

import "net/http"

const timefreeAreaCookie = "radio_review_timefree_area"

func selectedTimefreeArea(w http.ResponseWriter, r *http.Request) string {
	areas := getAreaList()
	requested := r.URL.Query().Get("area")
	if _, ok := areas[requested]; ok {
		existing, err := r.Cookie(timefreeAreaCookie)
		if err != nil || existing.Value != requested {
			http.SetCookie(w, &http.Cookie{Name: timefreeAreaCookie, Value: requested, Path: "/", MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil})
		}
		return requested
	}
	if remembered, err := r.Cookie(timefreeAreaCookie); err == nil {
		if _, ok := areas[remembered.Value]; ok {
			return remembered.Value
		}
	}
	return "JP13"
}
