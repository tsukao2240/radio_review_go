package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/service"
)

// stubScheduleService は RecordingScheduleServiceInterface のスタブ実装。
type stubScheduleService struct {
	getByUserFunc func(userID int64) ([]model.RecordingSchedule, error)
	scheduleFunc  func(userID int64, stationID, programTitle, startTime, endTime string, isRecurring bool, recurrenceType string) (*model.RecordingSchedule, error)
	cancelFunc    func(scheduleID, userID int64) error
}

func (s *stubScheduleService) GetByUser(userID int64) ([]model.RecordingSchedule, error) {
	if s.getByUserFunc != nil {
		return s.getByUserFunc(userID)
	}
	return nil, nil
}
func (s *stubScheduleService) Schedule(userID int64, stationID, programTitle, startTime, endTime string, isRecurring bool, recurrenceType string) (*model.RecordingSchedule, error) {
	if s.scheduleFunc != nil {
		return s.scheduleFunc(userID, stationID, programTitle, startTime, endTime, isRecurring, recurrenceType)
	}
	return &model.RecordingSchedule{ID: 1}, nil
}
func (s *stubScheduleService) Cancel(scheduleID, userID int64) error {
	if s.cancelFunc != nil {
		return s.cancelFunc(scheduleID, userID)
	}
	return nil
}

var _ service.RecordingScheduleServiceInterface = (*stubScheduleService)(nil)

func TestScheduleHandler_Index(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))

	t.Run("未認証: /loginにリダイレクト", func(t *testing.T) {
		h := NewScheduleHandler(&stubScheduleService{}, store)
		req := httptest.NewRequest(http.MethodGet, "/recording/schedules", nil)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
	})

	t.Run("認証済み: 200", func(t *testing.T) {
		svc := &stubScheduleService{
			getByUserFunc: func(userID int64) ([]model.RecordingSchedule, error) {
				return []model.RecordingSchedule{{ID: 1}}, nil
			},
		}
		h := NewScheduleHandler(svc, store)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/schedules", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubScheduleService{
			getByUserFunc: func(_ int64) ([]model.RecordingSchedule, error) {
				return nil, errors.New("db error")
			},
		}
		h := NewScheduleHandler(svc, store)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/schedules", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestScheduleHandler_Store(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewScheduleHandler(&stubScheduleService{}, store)
		body, _ := json.Marshal(map[string]string{
			"station_id": "TBS", "program_title": "jazz",
			"start_time": "2026-04-01 10:00:00", "end_time": "2026-04-01 11:00:00",
		})
		req := httptest.NewRequest(http.MethodPost, "/recording/schedule", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Store(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("必須フィールド不足: 422", func(t *testing.T) {
		h := NewScheduleHandler(&stubScheduleService{}, store)
		body, _ := json.Marshal(map[string]string{"station_id": "TBS"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Store(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})

	t.Run("正常予約: 201", func(t *testing.T) {
		svc := &stubScheduleService{
			scheduleFunc: func(_ int64, _, _, _, _ string, _ bool, _ string) (*model.RecordingSchedule, error) {
				return &model.RecordingSchedule{ID: 5}, nil
			},
		}
		h := NewScheduleHandler(svc, store)
		body, _ := json.Marshal(map[string]string{
			"station_id":    "TBS",
			"program_title": "jazz show",
			"start_time":    "2026-04-01 10:00:00",
			"end_time":      "2026-04-01 11:00:00",
		})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Store(rr, req)
		if rr.Code != http.StatusCreated {
			t.Errorf("got %d, want 201", rr.Code)
		}
	})

	t.Run("フォーム送信でも動作する", func(t *testing.T) {
		svc := &stubScheduleService{}
		h := NewScheduleHandler(svc, store)
		form := url.Values{
			"station_id":    {"TBS"},
			"program_title": {"jazz show"},
			"start_time":    {"2026-04-01 10:00:00"},
			"end_time":      {"2026-04-01 11:00:00"},
		}
		req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule", strings.NewReader(form.Encode())), 1)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Store(rr, req)
		if rr.Code != http.StatusCreated {
			t.Errorf("got %d, want 201", rr.Code)
		}
	})
}

func TestScheduleHandler_Cancel(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewScheduleHandler(&stubScheduleService{}, store)
		body, _ := json.Marshal(map[string]int64{"schedule_id": 1})
		req := httptest.NewRequest(http.MethodPost, "/recording/schedule/cancel", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Cancel(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("正常キャンセル: 200", func(t *testing.T) {
		h := NewScheduleHandler(&stubScheduleService{}, store)
		body, _ := json.Marshal(map[string]int64{"schedule_id": 1})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule/cancel", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Cancel(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("schedule_id=0: 422", func(t *testing.T) {
		h := NewScheduleHandler(&stubScheduleService{}, store)
		body, _ := json.Marshal(map[string]int64{"schedule_id": 0})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule/cancel", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Cancel(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})

	t.Run("存在しない予約: 404", func(t *testing.T) {
		svc := &stubScheduleService{
			cancelFunc: func(_, _ int64) error {
				return errors.New("録音予約が見つかりません")
			},
		}
		h := NewScheduleHandler(svc, store)
		body, _ := json.Marshal(map[string]int64{"schedule_id": 99})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule/cancel", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Cancel(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("got %d, want 404", rr.Code)
		}
	})
}

func TestScheduleHandler_Cancel_Forbidden(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))
	svc := &stubScheduleService{
		cancelFunc: func(_, _ int64) error {
			return errors.New("この録音予約をキャンセルする権限がありません")
		},
	}
	h := NewScheduleHandler(svc, store)
	body, _ := json.Marshal(map[string]int64{"schedule_id": 99})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule/cancel", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.Cancel(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rr.Code)
	}
}

func TestScheduleHandler_Cancel_DefaultError(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))
	svc := &stubScheduleService{
		cancelFunc: func(_, _ int64) error {
			return errors.New("unexpected error")
		},
	}
	h := NewScheduleHandler(svc, store)
	body, _ := json.Marshal(map[string]int64{"schedule_id": 99})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule/cancel", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.Cancel(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestScheduleHandler_Cancel_FormEncoded(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))
	h := NewScheduleHandler(&stubScheduleService{}, store)
	form := url.Values{"schedule_id": {"5"}}
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/schedule/cancel", strings.NewReader(form.Encode())), 1)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Cancel(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}
