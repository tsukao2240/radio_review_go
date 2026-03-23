package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/service"
)

type stubNotifService struct {
	getUnreadFunc   func(userID int64) ([]model.Notification, error)
	getAllFunc       func(userID int64) ([]model.Notification, error)
	markAsReadFunc  func(id, userID int64) error
	markAllReadFunc func(userID int64) error
}

func (s *stubNotifService) GetUnread(userID int64) ([]model.Notification, error) {
	if s.getUnreadFunc != nil {
		return s.getUnreadFunc(userID)
	}
	return nil, nil
}
func (s *stubNotifService) GetAll(userID int64) ([]model.Notification, error) {
	if s.getAllFunc != nil {
		return s.getAllFunc(userID)
	}
	return nil, nil
}
func (s *stubNotifService) MarkAsRead(id, userID int64) error {
	if s.markAsReadFunc != nil {
		return s.markAsReadFunc(id, userID)
	}
	return nil
}
func (s *stubNotifService) MarkAllAsRead(userID int64) error {
	if s.markAllReadFunc != nil {
		return s.markAllReadFunc(userID)
	}
	return nil
}

var _ service.NotificationServiceInterface = (*stubNotifService)(nil)

func TestNotificationHandler_GetUnread(t *testing.T) {
	t.Run("未読通知を JSON で返す", func(t *testing.T) {
		svc := &stubNotifService{
			getUnreadFunc: func(_ int64) ([]model.Notification, error) {
				return []model.Notification{{ID: 1}, {ID: 2}}, nil
			},
		}
		h := NewNotificationHandler(svc, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/api/notifications/unread", nil), 1)
		rr := httptest.NewRecorder()
		h.GetUnread(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		notifs, _ := resp["notifications"].([]interface{})
		if len(notifs) != 2 {
			t.Errorf("expected 2 notifications, got %d", len(notifs))
		}
	})

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewNotificationHandler(&stubNotifService{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/api/notifications/unread", nil)
		rr := httptest.NewRecorder()
		h.GetUnread(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubNotifService{
			getUnreadFunc: func(_ int64) ([]model.Notification, error) {
				return nil, errors.New("db error")
			},
		}
		h := NewNotificationHandler(svc, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/api/notifications/unread", nil), 1)
		rr := httptest.NewRecorder()
		h.GetUnread(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestNotificationHandler_MarkAsRead(t *testing.T) {
	t.Run("既読化成功: 200", func(t *testing.T) {
		var markedID int64
		svc := &stubNotifService{
			markAsReadFunc: func(id, _ int64) error { markedID = id; return nil },
		}
		h := NewNotificationHandler(svc, nil)
		body, _ := json.Marshal(map[string]int64{"notification_id": 5})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/notifications/mark-read", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.MarkAsRead(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		if markedID != 5 {
			t.Errorf("expected markedID=5, got %d", markedID)
		}
	})

	t.Run("notification_id=0: 400", func(t *testing.T) {
		h := NewNotificationHandler(&stubNotifService{}, nil)
		body, _ := json.Marshal(map[string]int64{"notification_id": 0})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/notifications/mark-read", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.MarkAsRead(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})
}

func TestNotificationHandler_MarkAllAsRead(t *testing.T) {
	t.Run("全既読化: 200", func(t *testing.T) {
		var calledUserID int64
		svc := &stubNotifService{
			markAllReadFunc: func(userID int64) error { calledUserID = userID; return nil },
		}
		h := NewNotificationHandler(svc, nil)
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/notifications/mark-all-read", nil), 7)
		rr := httptest.NewRecorder()
		h.MarkAllAsRead(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		if calledUserID != 7 {
			t.Errorf("expected userID=7, got %d", calledUserID)
		}
	})
}
