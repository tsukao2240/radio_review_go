package handler

import (
	"encoding/json"
	"net/http"

	"github.com/tsukao2240/radio_review_go/internal/middleware"
	"github.com/tsukao2240/radio_review_go/internal/service"
)

type PushHandler struct {
	service service.PushServiceInterface
}

func NewPushHandler(svc service.PushServiceInterface) *PushHandler {
	return &PushHandler{service: svc}
}

func (h *PushHandler) VAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"public_key": h.service.PublicKey(),
		"enabled":    h.service.Enabled(),
	})
}

func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	if !h.service.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "Web Push は無効です")
		return
	}
	var req struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}
	ua := r.UserAgent()
	var userAgent *string
	if ua != "" {
		userAgent = &ua
	}
	if err := h.service.Subscribe(userID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth, userAgent); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}
	if err := h.service.Unsubscribe(userID, req.Endpoint); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
