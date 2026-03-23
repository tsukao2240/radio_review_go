package service

import (
	"errors"
	"testing"

	"github.com/yourname/radio_review_go/internal/model"
)

type stubNotifRepo struct {
	findUnreadFunc    func(userID int64) ([]model.Notification, error)
	findAllFunc       func(userID int64) ([]model.Notification, error)
	markAsReadFunc    func(id, userID int64) error
	markAllReadFunc   func(userID int64) error
}

func (r *stubNotifRepo) FindUnreadByUser(userID int64) ([]model.Notification, error) {
	if r.findUnreadFunc != nil {
		return r.findUnreadFunc(userID)
	}
	return nil, nil
}
func (r *stubNotifRepo) FindAllByUser(userID int64) ([]model.Notification, error) {
	if r.findAllFunc != nil {
		return r.findAllFunc(userID)
	}
	return nil, nil
}
func (r *stubNotifRepo) MarkAsRead(id, userID int64) error {
	if r.markAsReadFunc != nil {
		return r.markAsReadFunc(id, userID)
	}
	return nil
}
func (r *stubNotifRepo) MarkAllAsRead(userID int64) error {
	if r.markAllReadFunc != nil {
		return r.markAllReadFunc(userID)
	}
	return nil
}

func TestNotificationService_GetUnread(t *testing.T) {
	t.Run("未読通知を返す", func(t *testing.T) {
		notifs := []model.Notification{{ID: 1}, {ID: 2}}
		repo := &stubNotifRepo{
			findUnreadFunc: func(userID int64) ([]model.Notification, error) {
				return notifs, nil
			},
		}
		svc := NewNotificationService(repo)
		result, err := svc.GetUnread(1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2, got %d", len(result))
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo := &stubNotifRepo{
			findUnreadFunc: func(userID int64) ([]model.Notification, error) { return nil, repoErr },
		}
		svc := NewNotificationService(repo)
		_, err := svc.GetUnread(1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestNotificationService_MarkAsRead(t *testing.T) {
	t.Run("既読化: 成功", func(t *testing.T) {
		var gotID, gotUserID int64
		repo := &stubNotifRepo{
			markAsReadFunc: func(id, userID int64) error {
				gotID, gotUserID = id, userID
				return nil
			},
		}
		svc := NewNotificationService(repo)
		if err := svc.MarkAsRead(5, 10); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if gotID != 5 || gotUserID != 10 {
			t.Errorf("MarkAsRead called with (%d,%d), want (5,10)", gotID, gotUserID)
		}
	})
}

func TestNotificationService_MarkAllAsRead(t *testing.T) {
	t.Run("全既読化: 成功", func(t *testing.T) {
		var gotUserID int64
		repo := &stubNotifRepo{
			markAllReadFunc: func(userID int64) error {
				gotUserID = userID
				return nil
			},
		}
		svc := NewNotificationService(repo)
		if err := svc.MarkAllAsRead(7); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if gotUserID != 7 {
			t.Errorf("MarkAllAsRead called with %d, want 7", gotUserID)
		}
	})
}

func TestNotificationService_GetAll(t *testing.T) {
	t.Run("全通知を返す", func(t *testing.T) {
		notifs := []model.Notification{{ID: 1}, {ID: 2}, {ID: 3}}
		repo := &stubNotifRepo{
			findAllFunc: func(userID int64) ([]model.Notification, error) {
				return notifs, nil
			},
		}
		svc := NewNotificationService(repo)
		result, err := svc.GetAll(5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("expected 3, got %d", len(result))
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo := &stubNotifRepo{
			findAllFunc: func(userID int64) ([]model.Notification, error) { return nil, repoErr },
		}
		svc := NewNotificationService(repo)
		_, err := svc.GetAll(5)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}
