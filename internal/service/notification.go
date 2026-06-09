package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/repository"
)

type PushSender interface {
	SendToUser(ctx context.Context, userID int64, payload PushPayload) error
}

// NotificationService は NotificationServiceInterface を実装する。
type NotificationService struct {
	repo       repository.NotificationRepositoryInterface
	pushSender PushSender
}

// NewNotificationService は新しい NotificationService を返す。
func NewNotificationService(repo repository.NotificationRepositoryInterface) *NotificationService {
	return &NotificationService{repo: repo}
}

func (s *NotificationService) SetPushSender(pushSender PushSender) {
	s.pushSender = pushSender
}

// GetUnread は未読通知の一覧を返す。
func (s *NotificationService) GetUnread(userID int64) ([]model.Notification, error) {
	return s.repo.FindUnreadByUser(userID)
}

// GetAll は全通知の一覧を返す。
func (s *NotificationService) GetAll(userID int64) ([]model.Notification, error) {
	return s.repo.FindAllByUser(userID)
}

func (s *NotificationService) Create(notification *model.Notification) (int64, error) {
	return s.repo.Create(notification)
}

func (s *NotificationService) CreateRecordingCompleted(sc *model.RecordingSchedule, recordingID string) error {
	dataJSON, err := json.Marshal(map[string]string{
		"station_id":   sc.StationID,
		"recording_id": recordingID,
	})
	if err != nil {
		return fmt.Errorf("notification recording completed data: %w", err)
	}
	dataStr := string(dataJSON)
	notification := &model.Notification{
		UserID:  sc.UserID,
		Type:    "recording_complete",
		Title:   "録音完了",
		Message: fmt.Sprintf("「%s」の録音が完了しました", sc.ProgramTitle),
		Data:    &dataStr,
	}
	id, err := s.repo.Create(notification)
	if err != nil {
		return err
	}
	notification.ID = id
	s.sendPush(notification, map[string]interface{}{
		"station_id":   sc.StationID,
		"recording_id": recordingID,
	})
	return nil
}

func (s *NotificationService) CreateRecordingFailed(sc *model.RecordingSchedule, errMsg string) error {
	dataJSON, err := json.Marshal(map[string]string{
		"station_id": sc.StationID,
		"error":      errMsg,
	})
	if err != nil {
		return fmt.Errorf("notification recording failed data: %w", err)
	}
	dataStr := string(dataJSON)
	notification := &model.Notification{
		UserID:  sc.UserID,
		Type:    "recording_failed",
		Title:   "録音失敗",
		Message: fmt.Sprintf("「%s」の録音に失敗しました: %s", sc.ProgramTitle, errMsg),
		Data:    &dataStr,
	}
	id, err := s.repo.Create(notification)
	if err != nil {
		return err
	}
	notification.ID = id
	s.sendPush(notification, map[string]interface{}{
		"station_id": sc.StationID,
		"error":      errMsg,
	})
	return nil
}

func (s *NotificationService) sendPush(notification *model.Notification, data map[string]interface{}) {
	if s.pushSender == nil || notification == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.pushSender.SendToUser(ctx, notification.UserID, PushPayload{
		Title: notification.Title,
		Body:  notification.Message,
		ID:    notification.ID,
		Type:  notification.Type,
		URL:   "/notifications",
		Data:  data,
	}); err != nil {
		slog.Warn("push notification failed", "user_id", notification.UserID, "notification_id", notification.ID, "error", err)
	}
}

// MarkAsRead は指定した通知を既読にする。
func (s *NotificationService) MarkAsRead(notificationID, userID int64) error {
	return s.repo.MarkAsRead(notificationID, userID)
}

// MarkAllAsRead はユーザーの全通知を既読にする。
func (s *NotificationService) MarkAllAsRead(userID int64) error {
	return s.repo.MarkAllAsRead(userID)
}
