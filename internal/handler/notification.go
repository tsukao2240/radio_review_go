package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/service"
)

// NotificationHandler は通知関連の HTTP ハンドラーを管理する。
type NotificationHandler struct {
	service service.NotificationServiceInterface
	store   sessions.Store
}

// NewNotificationHandler は新しい NotificationHandler を返す。
func NewNotificationHandler(svc service.NotificationServiceInterface, store sessions.Store) *NotificationHandler {
	return &NotificationHandler{
		service: svc,
		store:   store,
	}
}

// Index は GET /notifications を処理する。
// 通知一覧ページを HTML テンプレートで返す。
func (h *NotificationHandler) Index(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	notifications, err := h.service.GetAll(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "通知の取得に失敗しました: "+err.Error())
		return
	}

	RenderWithBase(w, r, "web/templates/notifications/index.html", map[string]interface{}{
		"Notifications": notifications,
	})
}

// GetUnread は GET /api/notifications/unread を処理する。
// 未読通知一覧を JSON で返す。
func (h *NotificationHandler) GetUnread(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}

	notifications, err := h.service.GetUnread(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "未読通知の取得に失敗しました: "+err.Error())
		return
	}

	if notifications == nil {
		notifications = []model.Notification{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": notifications,
	})
}

// GetAll は GET /api/notifications/all を処理する。
// 全通知一覧を JSON で返す。
func (h *NotificationHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}

	notifications, err := h.service.GetAll(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "通知の取得に失敗しました: "+err.Error())
		return
	}

	if notifications == nil {
		notifications = []model.Notification{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": notifications,
	})
}

// MarkAsRead は POST /api/notifications/mark-read を処理する。
// リクエストボディ: {"notification_id": 1}
func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}

	var req struct {
		NotificationID int64 `json:"notification_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}
	if req.NotificationID == 0 {
		writeError(w, http.StatusUnprocessableEntity, "notification_id は必須です")
		return
	}

	if err := h.service.MarkAsRead(req.NotificationID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "既読処理に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":         true,
		"notification_id": req.NotificationID,
	})
}

// MarkAllAsRead は POST /api/notifications/mark-all-read を処理する。
// 全通知を既読にする。
func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}

	if err := h.service.MarkAllAsRead(userID); err != nil {
		writeError(w, http.StatusInternalServerError, "全既読処理に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}
