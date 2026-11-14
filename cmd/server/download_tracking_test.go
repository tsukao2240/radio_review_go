package main

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
	"testing"
)

func TestRecordDownloadCompletionKeepsScheduleAndMarksDownloaded(t *testing.T) {
	for _, fail := range []bool{false, true} {
		conn, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		db := sqlx.NewDb(conn, "mysql")
		expected := mock.ExpectExec(`UPDATE recording_schedules.*SET recording_metadata = JSON_SET.*status.*downloaded.*file_path.*downloaded_at.*WHERE recording_id = \? AND status = 'completed'`).WithArgs(sqlmock.AnyArg(), "saved-recording")
		if fail {
			expected.WillReturnError(errors.New("database unavailable"))
		} else {
			expected.WillReturnResult(sqlmock.NewResult(0, 1))
		}
		err = recordDownloadCompletion(context.Background(), db, &model.RecordingInfo{RecordingID: "saved-recording"})
		if (err != nil) != fail {
			t.Fatal("database errors must prevent cleanup")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
	}
}
