package service

import (
	"encoding/json"
	"fmt"

	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/repository"
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
	_, err = s.repo.Create(&model.Notification{
		UserID:  sc.UserID,
		Type:    "recording_complete",
		Title:   "録音完了",
		Message: fmt.Sprintf("「%s」の録音が完了しました", sc.ProgramTitle),
		Data:    &dataStr,
	})
	return err
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
	_, err = s.repo.Create(&model.Notification{
		UserID:  sc.UserID,
		Type:    "recording_failed",
		Title:   "録音失敗",
		Message: fmt.Sprintf("「%s」の録音に失敗しました: %s", sc.ProgramTitle, errMsg),
		Data:    &dataStr,
	})
	return err
}

// MarkAsRead は指定した通知を既読にする。
func (s *NotificationService) MarkAsRead(notificationID, userID int64) error {
	return s.repo.MarkAsRead(notificationID, userID)
}

// MarkAllAsRead はユーザーの全通知を既読にする。
func (s *NotificationService) MarkAllAsRead(userID int64) error {
	return s.repo.MarkAllAsRead(userID)
}
