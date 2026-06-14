package repository

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

func newRecordingScheduleRepoMock(t *testing.T) (*RecordingScheduleRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	db := sqlx.NewDb(rawDB, "sqlmock")
	return NewRecordingScheduleRepository(db), mock, func() { rawDB.Close() }
}

func recordingScheduleRows() *sqlmock.Rows {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "user_id", "station_id", "program_title", "scheduled_start_time", "scheduled_end_time",
		"status", "recording_id", "error_message", "retry_count", "is_recurring", "recurrence_type", "parent_schedule_id",
		"created_at", "updated_at",
	}).AddRow(int64(3), int64(2), "TBS", "morning show", now, now.Add(time.Hour), "pending", nil, nil, 0, false, nil, nil, now, now)
}

func TestRecordingScheduleRepositoryFindByUser(t *testing.T) {
	repo, mock, cleanup := newRecordingScheduleRepoMock(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM recording_schedules WHERE user_id = ? ORDER BY scheduled_start_time DESC")).
		WithArgs(int64(2)).
		WillReturnRows(recordingScheduleRows())

	got, err := repo.FindByUser(2)
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if len(got) != 1 || got[0].ID != 3 || got[0].StationID != "TBS" {
		t.Fatalf("unexpected schedules: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRecordingScheduleRepositoryFindByID(t *testing.T) {
	repo, mock, cleanup := newRecordingScheduleRepoMock(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM recording_schedules WHERE id = ? LIMIT 1")).
		WithArgs(int64(3)).
		WillReturnRows(recordingScheduleRows())

	got, err := repo.FindByID(3)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ID != 3 || got.UserID != 2 {
		t.Fatalf("unexpected schedule: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRecordingScheduleRepositoryFindPendingBefore(t *testing.T) {
	repo, mock, cleanup := newRecordingScheduleRepoMock(t)
	defer cleanup()

	before := "2026-06-09 12:00:00"
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM recording_schedules
		 WHERE status = 'pending' AND scheduled_start_time <= ?
		 ORDER BY scheduled_start_time ASC`)).
		WithArgs(before).
		WillReturnRows(recordingScheduleRows())

	got, err := repo.FindPendingBefore(before)
	if err != nil {
		t.Fatalf("FindPendingBefore: %v", err)
	}
	if len(got) != 1 || got[0].Status != "pending" {
		t.Fatalf("unexpected schedules: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRecordingScheduleRepositoryCreate(t *testing.T) {
	repo, mock, cleanup := newRecordingScheduleRepoMock(t)
	defer cleanup()

	start := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO recording_schedules
		 (user_id, station_id, program_title, scheduled_start_time, scheduled_end_time,
		  status, recording_id, error_message, is_recurring, recurrence_type, parent_schedule_id,
		  created_at, updated_at)
		 VALUES
		 (?, ?, ?, ?, ?,
		  ?, ?, ?, ?, ?, ?,
		  NOW(), NOW())`)).
		WithArgs(int64(2), "TBS", "morning show", start, end, "pending", nil, nil, false, nil, nil).
		WillReturnResult(sqlmock.NewResult(4, 1))

	id, err := repo.Create(&model.RecordingSchedule{
		UserID:             2,
		StationID:          "TBS",
		ProgramTitle:       "morning show",
		ScheduledStartTime: start,
		ScheduledEndTime:   end,
		Status:             "pending",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != 4 {
		t.Fatalf("id = %d, want 4", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRecordingScheduleRepositoryMutations(t *testing.T) {
	repo, mock, cleanup := newRecordingScheduleRepoMock(t)
	defer cleanup()

	msg := "network error"
	mock.ExpectExec(regexp.QuoteMeta("UPDATE recording_schedules SET status = ?, error_message = ?, updated_at = NOW() WHERE id = ?")).
		WithArgs("failed", &msg, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE recording_schedules
		 SET retry_count = retry_count + 1, status = 'pending', error_message = ?, updated_at = NOW()
		 WHERE id = ?`)).
		WithArgs(&msg, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE recording_schedules SET recording_id = ?, updated_at = NOW() WHERE id = ?")).
		WithArgs("rec-1", int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE recording_schedules SET status = 'cancelled', updated_at = NOW() WHERE id = ?")).
		WithArgs(int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateStatus(3, "failed", &msg); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := repo.IncrementRetryCount(3, &msg); err != nil {
		t.Fatalf("IncrementRetryCount: %v", err)
	}
	if err := repo.SetRecordingID(3, "rec-1"); err != nil {
		t.Fatalf("SetRecordingID: %v", err)
	}
	if err := repo.Cancel(3); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
