package service

import (
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/repository"
)

// NotificationService は NotificationServiceInterface を実装する。
type NotificationService struct {
	repo repository.NotificationRepositoryInterface
}

// NewNotificationService は新しい NotificationService を返す。
func NewNotificationService(repo repository.NotificationRepositoryInterface) *NotificationService {
	return &NotificationService{repo: repo}
}

// GetUnread は未読通知の一覧を返す。
func (s *NotificationService) GetUnread(userID int64) ([]model.Notification, error) {
	return s.repo.FindUnreadByUser(userID)
}

// GetAll は全通知の一覧を返す。
func (s *NotificationService) GetAll(userID int64) ([]model.Notification, error) {
	return s.repo.FindAllByUser(userID)
}

// MarkAsRead は指定した通知を既読にする。
func (s *NotificationService) MarkAsRead(notificationID, userID int64) error {
	return s.repo.MarkAsRead(notificationID, userID)
}

// MarkAllAsRead はユーザーの全通知を既読にする。
func (s *NotificationService) MarkAllAsRead(userID int64) error {
	return s.repo.MarkAllAsRead(userID)
}
