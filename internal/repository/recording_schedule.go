package repository

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

type RecordingScheduleRepository struct {
	db *sqlx.DB
}

func NewRecordingScheduleRepository(db *sqlx.DB) *RecordingScheduleRepository {
	return &RecordingScheduleRepository{db: db}
}

func (r *RecordingScheduleRepository) FindByUser(userID int64) ([]model.RecordingSchedule, error) {
	var schedules []model.RecordingSchedule
	err := r.db.Select(&schedules,
		"SELECT * FROM recording_schedules WHERE user_id = ? ORDER BY scheduled_start_time DESC",
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RecordingScheduleRepository.FindByUser: %w", err)
	}
	return schedules, nil
}

func (r *RecordingScheduleRepository) FindByID(id int64) (*model.RecordingSchedule, error) {
	var s model.RecordingSchedule
	err := r.db.Get(&s, "SELECT * FROM recording_schedules WHERE id = ? LIMIT 1", id)
	if err != nil {
		return nil, fmt.Errorf("repository.RecordingScheduleRepository.FindByID: %w", err)
	}
	return &s, nil
}

// FindPendingBefore returns pending schedules whose scheduled_start_time is before or equal to t.
// t should be formatted as a MySQL-compatible datetime string (e.g. "2006-01-02 15:04:05").
func (r *RecordingScheduleRepository) FindPendingBefore(t string) ([]model.RecordingSchedule, error) {
	var schedules []model.RecordingSchedule
	err := r.db.Select(&schedules,
		`SELECT * FROM recording_schedules
		 WHERE status = 'pending' AND scheduled_start_time <= ?
		   AND (next_retry_at IS NULL OR next_retry_at <= ?)
		 ORDER BY scheduled_start_time ASC`,
		t, t,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RecordingScheduleRepository.FindPendingBefore: %w", err)
	}
	return schedules, nil
}

func (r *RecordingScheduleRepository) Create(s *model.RecordingSchedule) (int64, error) {
	res, err := r.db.NamedExec(
		`INSERT INTO recording_schedules
		 (user_id, station_id, program_title, scheduled_start_time, scheduled_end_time,
		  status, recording_id, error_message, is_recurring, recurrence_type, parent_schedule_id,
		  created_at, updated_at)
		 VALUES
		 (:user_id, :station_id, :program_title, :scheduled_start_time, :scheduled_end_time,
		  :status, :recording_id, :error_message, :is_recurring, :recurrence_type, :parent_schedule_id,
		  NOW(), NOW())`,
		s,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.RecordingScheduleRepository.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.RecordingScheduleRepository.Create LastInsertId: %w", err)
	}
	return id, nil
}

func (r *RecordingScheduleRepository) UpdateStatus(id int64, status string, errMsg *string) error {
	_, err := r.db.Exec(
		"UPDATE recording_schedules SET status = ?, error_message = ?, updated_at = NOW() WHERE id = ?",
		status, errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("repository.RecordingScheduleRepository.UpdateStatus: %w", err)
	}
	return nil
}

func (r *RecordingScheduleRepository) IncrementRetryCount(id int64, errMsg *string) error {
	_, err := r.db.Exec(
		`UPDATE recording_schedules
		 SET retry_count = retry_count + 1, status = 'pending', error_message = ?,
		     next_retry_at = DATE_ADD(NOW(), INTERVAL 60 SECOND), updated_at = NOW()
		 WHERE id = ?`,
		errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("repository.RecordingScheduleRepository.IncrementRetryCount: %w", err)
	}
	return nil
}

func (r *RecordingScheduleRepository) SetRecordingID(id int64, recordingID string) error {
	_, err := r.db.Exec(
		"UPDATE recording_schedules SET recording_id = ?, updated_at = NOW() WHERE id = ?",
		recordingID, id,
	)
	if err != nil {
		return fmt.Errorf("repository.RecordingScheduleRepository.SetRecordingID: %w", err)
	}
	return nil
}

func (r *RecordingScheduleRepository) Cancel(id int64) error {
	_, err := r.db.Exec(
		"UPDATE recording_schedules SET status = 'cancelled', updated_at = NOW() WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("repository.RecordingScheduleRepository.Cancel: %w", err)
	}
	return nil
}
