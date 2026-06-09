package repository

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

type NotificationRepository struct {
	db *sqlx.DB
}

func NewNotificationRepository(db *sqlx.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) FindUnreadByUser(userID int64) ([]model.Notification, error) {
	var notifications []model.Notification
	err := r.db.Select(&notifications,
		`SELECT * FROM notifications
		 WHERE user_id = ? AND is_read = FALSE
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.NotificationRepository.FindUnreadByUser: %w", err)
	}
	return notifications, nil
}

func (r *NotificationRepository) FindAllByUser(userID int64) ([]model.Notification, error) {
	var notifications []model.Notification
	err := r.db.Select(&notifications,
		"SELECT * FROM notifications WHERE user_id = ? ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.NotificationRepository.FindAllByUser: %w", err)
	}
	return notifications, nil
}

func (r *NotificationRepository) Create(notification *model.Notification) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO notifications (user_id, type, title, message, data, is_read, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, FALSE, NOW(), NOW())`,
		notification.UserID,
		notification.Type,
		notification.Title,
		notification.Message,
		notification.Data,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.NotificationRepository.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.NotificationRepository.Create LastInsertId: %w", err)
	}
	return id, nil
}

func (r *NotificationRepository) MarkAsRead(id, userID int64) error {
	_, err := r.db.Exec(
		`UPDATE notifications
		 SET is_read = TRUE, read_at = NOW(), updated_at = NOW()
		 WHERE id = ? AND user_id = ?`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("repository.NotificationRepository.MarkAsRead: %w", err)
	}
	return nil
}

func (r *NotificationRepository) MarkAllAsRead(userID int64) error {
	_, err := r.db.Exec(
		`UPDATE notifications
		 SET is_read = TRUE, read_at = NOW(), updated_at = NOW()
		 WHERE user_id = ? AND is_read = FALSE`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("repository.NotificationRepository.MarkAllAsRead: %w", err)
	}
	return nil
}
