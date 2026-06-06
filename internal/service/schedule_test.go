package service

import (
	"errors"
	"testing"
	"time"

	"github.com/yourname/radio_review_go/internal/model"
)

type stubScheduleRepo struct {
	findByUserFunc   func(userID int64) ([]model.RecordingSchedule, error)
	findByIDFunc     func(id int64) (*model.RecordingSchedule, error)
	findPendingFunc  func(t string) ([]model.RecordingSchedule, error)
	createFunc       func(s *model.RecordingSchedule) (int64, error)
	updateStatusFunc func(id int64, status string, errMsg *string) error
	cancelFunc       func(id int64) error
}

func (r *stubScheduleRepo) FindByUser(userID int64) ([]model.RecordingSchedule, error) {
	if r.findByUserFunc != nil {
		return r.findByUserFunc(userID)
	}
	return nil, nil
}
func (r *stubScheduleRepo) FindByID(id int64) (*model.RecordingSchedule, error) {
	if r.findByIDFunc != nil {
		return r.findByIDFunc(id)
	}
	return nil, nil
}
func (r *stubScheduleRepo) FindPendingBefore(t string) ([]model.RecordingSchedule, error) {
	if r.findPendingFunc != nil {
		return r.findPendingFunc(t)
	}
	return nil, nil
}
func (r *stubScheduleRepo) Create(s *model.RecordingSchedule) (int64, error) {
	if r.createFunc != nil {
		return r.createFunc(s)
	}
	return 1, nil
}
func (r *stubScheduleRepo) UpdateStatus(id int64, status string, errMsg *string) error {
	if r.updateStatusFunc != nil {
		return r.updateStatusFunc(id, status, errMsg)
	}
	return nil
}
func (r *stubScheduleRepo) SetRecordingID(id int64, recordingID string) error { return nil }
func (r *stubScheduleRepo) Cancel(id int64) error {
	if r.cancelFunc != nil {
		return r.cancelFunc(id)
	}
	return nil
}

func TestRecordingScheduleService_Schedule(t *testing.T) {
	start := time.Now().Add(time.Hour)
	end := start.Add(30 * time.Minute)

	t.Run("正常な時刻: 予約成功", func(t *testing.T) {
		repo := &stubScheduleRepo{
			createFunc: func(s *model.RecordingSchedule) (int64, error) { return 10, nil },
		}
		svc := NewRecordingScheduleService(repo)
		schedule, err := svc.Schedule(1, "TBS", "jazz show",
			start.Format(time.RFC3339), end.Format(time.RFC3339), false, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if schedule.ID != 10 {
			t.Errorf("expected ID=10, got %d", schedule.ID)
		}
		if schedule.Status != "pending" {
			t.Errorf("expected status=pending, got %s", schedule.Status)
		}
	})

	t.Run("開始 >= 終了: エラー", func(t *testing.T) {
		svc := NewRecordingScheduleService(&stubScheduleRepo{})
		_, err := svc.Schedule(1, "TBS", "jazz show",
			end.Format(time.RFC3339), start.Format(time.RFC3339), false, "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("不正な時刻フォーマット: エラー", func(t *testing.T) {
		svc := NewRecordingScheduleService(&stubScheduleRepo{})
		_, err := svc.Schedule(1, "TBS", "jazz show", "not-a-time", end.Format(time.RFC3339), false, "")
		if err == nil {
			t.Fatal("expected parse error, got nil")
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("insert failed")
		repo := &stubScheduleRepo{
			createFunc: func(s *model.RecordingSchedule) (int64, error) { return 0, repoErr },
		}
		svc := NewRecordingScheduleService(repo)
		_, err := svc.Schedule(1, "TBS", "jazz show",
			start.Format(time.RFC3339), end.Format(time.RFC3339), false, "")
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestRecordingScheduleService_Cancel(t *testing.T) {
	t.Run("本人の予約: キャンセル成功", func(t *testing.T) {
		var cancelCalled bool
		repo := &stubScheduleRepo{
			findByIDFunc: func(id int64) (*model.RecordingSchedule, error) {
				return &model.RecordingSchedule{ID: id, UserID: 1}, nil
			},
			cancelFunc: func(id int64) error {
				cancelCalled = true
				return nil
			},
		}
		svc := NewRecordingScheduleService(repo)
		if err := svc.Cancel(5, 1); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !cancelCalled {
			t.Error("Cancel not called")
		}
	})

	t.Run("他ユーザーの予約: 権限エラー", func(t *testing.T) {
		repo := &stubScheduleRepo{
			findByIDFunc: func(id int64) (*model.RecordingSchedule, error) {
				return &model.RecordingSchedule{ID: id, UserID: 99}, nil
			},
		}
		svc := NewRecordingScheduleService(repo)
		err := svc.Cancel(5, 1) // userID=1 が userID=99 の予約をキャンセルしようとする
		if err == nil {
			t.Fatal("expected unauthorized error, got nil")
		}
	})

	t.Run("予約が見つからない: エラー", func(t *testing.T) {
		repo := &stubScheduleRepo{
			findByIDFunc: func(id int64) (*model.RecordingSchedule, error) {
				return nil, nil
			},
		}
		svc := NewRecordingScheduleService(repo)
		err := svc.Cancel(999, 1)
		if err == nil {
			t.Fatal("expected not found error, got nil")
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo := &stubScheduleRepo{
			findByIDFunc: func(id int64) (*model.RecordingSchedule, error) {
				return nil, repoErr
			},
		}
		svc := NewRecordingScheduleService(repo)
		err := svc.Cancel(1, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestRecordingScheduleService_GetByUser(t *testing.T) {
	t.Run("予約一覧取得: 成功", func(t *testing.T) {
		want := []model.RecordingSchedule{{ID: 1, UserID: 3}, {ID: 2, UserID: 3}}
		repo := &stubScheduleRepo{
			findByUserFunc: func(userID int64) ([]model.RecordingSchedule, error) {
				return want, nil
			},
		}
		svc := NewRecordingScheduleService(repo)
		got, err := svc.GetByUser(3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 schedules, got %d", len(got))
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo := &stubScheduleRepo{
			findByUserFunc: func(userID int64) ([]model.RecordingSchedule, error) {
				return nil, repoErr
			},
		}
		svc := NewRecordingScheduleService(repo)
		_, err := svc.GetByUser(1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}
