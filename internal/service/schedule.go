package service

import (
	"errors"
	"time"

	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/repository"
)

// RecordingScheduleService は RecordingScheduleServiceInterface を実装する。
type RecordingScheduleService struct {
	repo repository.RecordingScheduleRepositoryInterface
}

// NewRecordingScheduleService は新しい RecordingScheduleService を返す。
func NewRecordingScheduleService(repo repository.RecordingScheduleRepositoryInterface) *RecordingScheduleService {
	return &RecordingScheduleService{repo: repo}
}

// GetByUser はユーザーの録音予約一覧を返す。
func (s *RecordingScheduleService) GetByUser(userID int64) ([]model.RecordingSchedule, error) {
	return s.repo.FindByUser(userID)
}

// Schedule は録音予約を作成する。
// startTime・endTime は RFC3339 または "2006-01-02 15:04:05" 形式を受け付ける。
// startTime < endTime でなければエラーを返す。
// isRecurring=true かつ recurrenceType="weekly" の場合は定期録音として登録する。
func (s *RecordingScheduleService) Schedule(userID int64, stationID, programTitle string, startTime, endTime string, isRecurring bool, recurrenceType string) (*model.RecordingSchedule, error) {
	// 時刻パース（RFC3339 を優先し、失敗したら一般的な形式を試みる）
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006/01/02 15:04:05",
	}

	var start, end time.Time
	var parseErr error

	for _, layout := range layouts {
		start, parseErr = time.ParseInLocation(layout, startTime, time.Local)
		if parseErr == nil {
			break
		}
	}
	if parseErr != nil {
		return nil, errors.New("start_time のパースに失敗しました: " + parseErr.Error())
	}

	for _, layout := range layouts {
		end, parseErr = time.ParseInLocation(layout, endTime, time.Local)
		if parseErr == nil {
			break
		}
	}
	if parseErr != nil {
		return nil, errors.New("end_time のパースに失敗しました: " + parseErr.Error())
	}

	// バリデーション: 開始時刻 < 終了時刻
	if !start.Before(end) {
		return nil, errors.New("開始時刻は終了時刻より前でなければなりません")
	}

	var recType *string
	if isRecurring && recurrenceType != "" {
		recType = &recurrenceType
	}

	schedule := &model.RecordingSchedule{
		UserID:             userID,
		StationID:          stationID,
		ProgramTitle:       programTitle,
		ScheduledStartTime: start,
		ScheduledEndTime:   end,
		Status:             "pending",
		IsRecurring:        isRecurring,
		RecurrenceType:     recType,
	}

	id, err := s.repo.Create(schedule)
	if err != nil {
		return nil, err
	}
	schedule.ID = id
	return schedule, nil
}

// Cancel はユーザー本人の録音予約をキャンセルする。
// 他のユーザーの予約はキャンセルできない（403相当エラー）。
func (s *RecordingScheduleService) Cancel(scheduleID, userID int64) error {
	schedule, err := s.repo.FindByID(scheduleID)
	if err != nil {
		return err
	}
	if schedule == nil {
		return errors.New("録音予約が見つかりません")
	}
	if schedule.UserID != userID {
		return errors.New("この録音予約をキャンセルする権限がありません")
	}
	return s.repo.Cancel(scheduleID)
}
