package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/service"
)

// stubFavService は FavoriteServiceInterface のスタブ実装。
type stubFavService struct {
	getByUserFunc func(userID int64) ([]model.FavoriteProgram, error)
	addFunc       func(userID int64, stationID, programTitle string, broadcastDay *int) (*model.FavoriteProgram, error)
	removeFunc    func(userID int64, stationID, programTitle string, broadcastDay *int) error
	checkFunc     func(userID int64, stationID, programTitle string, broadcastDay *int) (bool, error)
}

func (s *stubFavService) GetByUser(userID int64) ([]model.FavoriteProgram, error) {
	if s.getByUserFunc != nil {
		return s.getByUserFunc(userID)
	}
	return nil, nil
}
func (s *stubFavService) Add(userID int64, stationID, programTitle string, broadcastDay *int) (*model.FavoriteProgram, error) {
	if s.addFunc != nil {
		return s.addFunc(userID, stationID, programTitle, broadcastDay)
	}
	return &model.FavoriteProgram{ID: 1}, nil
}
func (s *stubFavService) Remove(userID int64, stationID, programTitle string, broadcastDay *int) error {
	if s.removeFunc != nil {
		return s.removeFunc(userID, stationID, programTitle, broadcastDay)
	}
	return nil
}
func (s *stubFavService) Check(userID int64, stationID, programTitle string, broadcastDay *int) (bool, error) {
	if s.checkFunc != nil {
		return s.checkFunc(userID, stationID, programTitle, broadcastDay)
	}
	return false, nil
}

var _ service.FavoriteServiceInterface = (*stubFavService)(nil)

type stubFavoriteRecorder struct {
	startFunc       func(ctx context.Context, ownerKey, stationID, programName, startTime, endTime, areaID string) (string, error)
	isRecordingFunc func(ctx context.Context, ownerKey, programName string) (bool, error)
}

func (s *stubFavoriteRecorder) StartTimefree(ctx context.Context, ownerKey, stationID, programName, startTime, endTime, areaID string) (string, error) {
	if s.startFunc != nil {
		return s.startFunc(ctx, ownerKey, stationID, programName, startTime, endTime, areaID)
	}
	return "rec-1", nil
}

func (s *stubFavoriteRecorder) IsProgramRecording(ctx context.Context, ownerKey, programName string) (bool, error) {
	if s.isRecordingFunc != nil {
		return s.isRecordingFunc(ctx, ownerKey, programName)
	}
	return false, nil
}

// withUserID はテスト用にユーザーIDをコンテキストに埋め込む。
func withUserID(r *http.Request, userID int64) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, userID)
	return r.WithContext(ctx)
}

func TestFavoriteHandler_Store(t *testing.T) {
	t.Run("正常追加: 200", func(t *testing.T) {
		svc := &stubFavService{}
		h := NewFavoriteHandler(svc, nil, nil)

		body, _ := json.Marshal(map[string]interface{}{
			"station_id":    "TBS",
			"program_title": "jazz show",
		})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites", bytes.NewReader(body)), 1)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.Store(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("station_id 未指定: 422", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		body, _ := json.Marshal(map[string]interface{}{"program_title": "jazz"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Store(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		body, _ := json.Marshal(map[string]interface{}{"station_id": "TBS", "program_title": "jazz"})
		req := httptest.NewRequest(http.MethodPost, "/favorites", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Store(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubFavService{
			addFunc: func(_ int64, _, _ string, _ *int) (*model.FavoriteProgram, error) {
				return nil, errors.New("db error")
			},
		}
		h := NewFavoriteHandler(svc, nil, nil)
		body, _ := json.Marshal(map[string]interface{}{"station_id": "TBS", "program_title": "jazz"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Store(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestFavoriteHandler_Destroy(t *testing.T) {
	t.Run("正常削除: 200", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		body, _ := json.Marshal(map[string]interface{}{"station_id": "TBS", "program_title": "jazz"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites/delete", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("station_id 未指定: 422", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		body, _ := json.Marshal(map[string]interface{}{"program_title": "jazz"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites/delete", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		body, _ := json.Marshal(map[string]interface{}{"station_id": "TBS", "program_title": "jazz"})
		req := httptest.NewRequest(http.MethodPost, "/favorites/delete", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("不正なJSON: 400", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites/delete", strings.NewReader("bad-json")), 1)
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubFavService{
			removeFunc: func(_ int64, _, _ string, _ *int) error {
				return errors.New("db error")
			},
		}
		h := NewFavoriteHandler(svc, nil, nil)
		body, _ := json.Marshal(map[string]interface{}{"station_id": "TBS", "program_title": "jazz"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites/delete", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestFavoriteHandler_Check(t *testing.T) {
	t.Run("登録済み: is_favorite=true", func(t *testing.T) {
		svc := &stubFavService{
			checkFunc: func(_ int64, _, _ string, _ *int) (bool, error) { return true, nil },
		}
		h := NewFavoriteHandler(svc, nil, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites/check?station_id=TBS&program_title=jazz", nil), 1)
		rr := httptest.NewRecorder()
		h.Check(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		var resp map[string]bool
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if !resp["is_favorite"] {
			t.Error("expected is_favorite=true")
		}
	})

	t.Run("パラメータ不足: 400", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites/check?station_id=TBS", nil), 1)
		rr := httptest.NewRecorder()
		h.Check(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubFavService{
			checkFunc: func(_ int64, _, _ string, _ *int) (bool, error) {
				return false, errors.New("db error")
			},
		}
		h := NewFavoriteHandler(svc, nil, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites/check?station_id=TBS&program_title=jazz", nil), 1)
		rr := httptest.NewRecorder()
		h.Check(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})

	t.Run("未認証: is_favorite=false", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		req := httptest.NewRequest(http.MethodGet, "/favorites/check?station_id=TBS&program_title=jazz", nil)
		rr := httptest.NewRecorder()
		h.Check(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		var resp map[string]bool
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp["is_favorite"] {
			t.Error("unauthenticated should return is_favorite=false")
		}
	})
}

func TestFavoriteHandler_RecordAll(t *testing.T) {
	now := time.Now()
	matchingEnd := now.AddDate(0, 0, -2)
	matchingStart := matchingEnd.Add(-1 * time.Hour)
	otherEnd := now.AddDate(0, 0, -1)
	otherStart := otherEnd.Add(-1 * time.Hour)
	broadcastDay := (int(matchingStart.Weekday()) + 6) % 7

	favs := []model.FavoriteProgram{
		{ID: 1, UserID: 7, StationID: "TBS", ProgramTitle: "daily", BroadcastDay: &broadcastDay, CreatedAt: now},
		{ID: 2, UserID: 7, StationID: "TBS", ProgramTitle: "missing", CreatedAt: now},
		{ID: 3, UserID: 7, StationID: "QRR", ProgramTitle: "duplicate", CreatedAt: now},
	}

	svc := &stubFavService{
		getByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
			if userID != 7 {
				t.Fatalf("userID = %d, want 7", userID)
			}
			return favs, nil
		},
	}
	radiko := &stubRadikoService{
		getTwoWeekScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
			switch stationID {
			case "TBS":
				return []map[string]interface{}{
					{
						"entries": []map[string]interface{}{
							{
								"title": "daily",
								"date":  matchingStart.Format("20060102"),
								"start": matchingStart.Format("15:04"),
								"end":   matchingEnd.Format("15:04"),
								"ft":    matchingStart.Format("20060102150405"),
								"to":    matchingEnd.Format("20060102150405"),
							},
							{
								"title": "daily",
								"date":  otherStart.Format("20060102"),
								"start": otherStart.Format("15:04"),
								"end":   otherEnd.Format("15:04"),
								"ft":    otherStart.Format("20060102150405"),
								"to":    otherEnd.Format("20060102150405"),
							},
						},
					},
				}, nil
			case "QRR":
				return []map[string]interface{}{
					{
						"entries": []map[string]interface{}{
							{
								"title": "duplicate",
								"date":  otherStart.Format("20060102"),
								"start": otherStart.Format("15:04"),
								"end":   otherEnd.Format("15:04"),
								"ft":    otherStart.Format("20060102150405"),
								"to":    otherEnd.Format("20060102150405"),
							},
						},
					},
				}, nil
			default:
				return nil, nil
			}
		},
	}

	var starts []map[string]string
	recorder := &stubFavoriteRecorder{
		isRecordingFunc: func(ctx context.Context, ownerKey, programName string) (bool, error) {
			if ownerKey != "user_7" {
				t.Fatalf("ownerKey = %q, want user_7", ownerKey)
			}
			return programName == "duplicate", nil
		},
		startFunc: func(ctx context.Context, ownerKey, stationID, programName, startTime, endTime, areaID string) (string, error) {
			starts = append(starts, map[string]string{
				"ownerKey":    ownerKey,
				"stationID":   stationID,
				"programName": programName,
				"startTime":   startTime,
				"endTime":     endTime,
				"areaID":      areaID,
			})
			return "rec-daily", nil
		},
	}

	h := NewFavoriteHandler(svc, radiko, nil)
	h.SetRecorder(recorder)
	req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites/record-all", nil), 7)
	rr := httptest.NewRecorder()
	h.RecordAll(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	var resp recordAllFavoritesResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Total != 3 || resp.Started != 1 || resp.Skipped != 2 {
		t.Fatalf("summary = %#v", resp)
	}
	if len(starts) != 1 {
		t.Fatalf("starts len = %d, want 1", len(starts))
	}
	wantStart := matchingStart.Format("20060102150405")
	wantEnd := matchingEnd.Format("20060102150405")
	if starts[0]["programName"] != "daily" || starts[0]["startTime"] != wantStart || starts[0]["endTime"] != wantEnd {
		t.Fatalf("start args = %#v", starts[0])
	}
	if resp.Items[0].Status != "started" || resp.Items[0].RecordingID != "rec-daily" {
		t.Fatalf("first item = %#v", resp.Items[0])
	}
	if resp.Items[1].Status != "skipped" || resp.Items[1].Reason == "" {
		t.Fatalf("second item = %#v", resp.Items[1])
	}
	if resp.Items[2].Status != "skipped" || resp.Items[2].Reason != "同じ番組が録音中です" {
		t.Fatalf("third item = %#v", resp.Items[2])
	}
}

func TestFavoriteHandler_RecordAll_UsesRadikoFtToForLateNightProgram(t *testing.T) {
	now := time.Now()
	start := now.AddDate(0, 0, -2)
	start = time.Date(start.Year(), start.Month(), start.Day(), 1, 0, 0, 0, time.Local)
	end := start.Add(2 * time.Hour)

	svc := &stubFavService{
		getByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
			return []model.FavoriteProgram{{
				ID:           1,
				UserID:       userID,
				StationID:    "LFR",
				ProgramTitle: "late night",
				CreatedAt:    now,
			}}, nil
		},
	}
	radiko := &stubRadikoService{
		getTwoWeekScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
			return []map[string]interface{}{
				{
					"entries": []map[string]interface{}{
						{
							"title": "late night",
							"date":  start.Format("20060102"),
							"start": "25:00",
							"end":   "27:00",
							"ft":    start.Format("20060102150405"),
							"to":    end.Format("20060102150405"),
						},
					},
				},
			}, nil
		},
	}

	var gotStart, gotEnd string
	recorder := &stubFavoriteRecorder{
		startFunc: func(ctx context.Context, ownerKey, stationID, programName, startTime, endTime, areaID string) (string, error) {
			gotStart = startTime
			gotEnd = endTime
			return "rec-late", nil
		},
	}

	h := NewFavoriteHandler(svc, radiko, nil)
	h.SetRecorder(recorder)
	req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites/record-all", nil), 7)
	rr := httptest.NewRecorder()
	h.RecordAll(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	if gotStart != start.Format("20060102150405") || gotEnd != end.Format("20060102150405") {
		t.Fatalf("recording times = %q/%q, want %q/%q", gotStart, gotEnd, start.Format("20060102150405"), end.Format("20060102150405"))
	}
	if gotStart == start.Format("20060102")+"2500" {
		t.Fatalf("recording start used display time: %q", gotStart)
	}
}

func TestFavoriteHandler_RecordAll_Unauthorized(t *testing.T) {
	h := NewFavoriteHandler(&stubFavService{}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/favorites/record-all", nil)
	rr := httptest.NewRecorder()
	h.RecordAll(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rr.Code)
	}
}

func TestFavoriteHandler_Index(t *testing.T) {
	t.Run("未認証: /loginにリダイレクト", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil, nil)
		req := httptest.NewRequest(http.MethodGet, "/favorites", nil)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/login" {
			t.Errorf("got Location=%q, want /login", loc)
		}
	})
	t.Run("認証済み: 200", func(t *testing.T) {
		svc := &stubFavService{
			getByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
				return []model.FavoriteProgram{{ID: 1}}, nil
			},
		}
		h := NewFavoriteHandler(svc, nil, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubFavService{
			getByUserFunc: func(_ int64) ([]model.FavoriteProgram, error) {
				return nil, errors.New("db error")
			},
		}
		h := NewFavoriteHandler(svc, nil, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
	t.Run("タイムフリーあり: 録音情報を付与", func(t *testing.T) {
		createdAt := time.Now()
		svc := &stubFavService{
			getByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
				return []model.FavoriteProgram{{
					ID:           1,
					UserID:       userID,
					StationID:    "TBS",
					ProgramTitle: "jazz",
					CreatedAt:    createdAt,
				}}, nil
			},
		}

		calls := 0
		end := time.Now().Add(-1 * time.Hour)
		start := end.Add(-2 * time.Hour)
		radiko := &stubRadikoService{
			getTwoWeekScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
				calls++
				return []map[string]interface{}{
					{
						"entries": []map[string]interface{}{
							{
								"title": "jazz",
								"cast":  "DJ",
								"date":  start.Format("20060102"),
								"start": start.Format("15:04"),
								"end":   end.Format("15:04"),
								"to":    end.Format("20060102150405"),
							},
						},
					},
				}, nil
			},
		}
		h := NewFavoriteHandler(svc, radiko, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		if calls != 1 {
			t.Errorf("GetTwoWeekSchedule calls = %d, want 1", calls)
		}

		var resp struct {
			Favorites []struct {
				Cast           string
				Recordable     bool
				RecProgramName string
				RecDate        string
				RecStart       string
				RecEnd         string
				RecFt          string
				RecTo          string
			}
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if len(resp.Favorites) != 1 {
			t.Fatalf("favorites len = %d, want 1", len(resp.Favorites))
		}
		got := resp.Favorites[0]
		if !got.Recordable {
			t.Error("expected Recordable=true")
		}
		if got.Cast != "DJ" {
			t.Errorf("Cast = %q, want DJ", got.Cast)
		}
		if got.RecProgramName != "jazz" || got.RecDate != start.Format("20060102") || got.RecStart != start.Format("15:04") || got.RecEnd != end.Format("15:04") {
			t.Errorf("recording fields = %#v", got)
		}
	})
	t.Run("タイムフリーあり: 24時以降はft/toを録音用に付与", func(t *testing.T) {
		now := time.Now()
		start := now.AddDate(0, 0, -2)
		start = time.Date(start.Year(), start.Month(), start.Day(), 1, 0, 0, 0, time.Local)
		end := start.Add(2 * time.Hour)
		svc := &stubFavService{
			getByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
				return []model.FavoriteProgram{{
					ID:           1,
					UserID:       userID,
					StationID:    "LFR",
					ProgramTitle: "late night",
					CreatedAt:    now,
				}}, nil
			},
		}
		radiko := &stubRadikoService{
			getTwoWeekScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
				return []map[string]interface{}{
					{
						"entries": []map[string]interface{}{
							{
								"title": "late night",
								"date":  start.Format("20060102"),
								"start": "25:00",
								"end":   "27:00",
								"ft":    start.Format("20060102150405"),
								"to":    end.Format("20060102150405"),
							},
						},
					},
				}, nil
			},
		}
		h := NewFavoriteHandler(svc, radiko, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		var resp struct {
			Favorites []struct {
				Recordable bool
				RecStart   string
				RecEnd     string
				RecFt      string
				RecTo      string
			}
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if len(resp.Favorites) != 1 {
			t.Fatalf("favorites len = %d, want 1", len(resp.Favorites))
		}
		got := resp.Favorites[0]
		if !got.Recordable || got.RecStart != "25:00" || got.RecEnd != "27:00" {
			t.Fatalf("recording display fields = %#v", got)
		}
		if got.RecFt != start.Format("20060102150405") || got.RecTo != end.Format("20060102150405") {
			t.Fatalf("recording ft/to = %#v", got)
		}
	})
	t.Run("タイムフリーあり: broadcast_dayの曜日を録音対象にする", func(t *testing.T) {
		now := time.Now()
		matchingEnd := now.AddDate(0, 0, -2)
		matchingStart := matchingEnd.Add(-1 * time.Hour)
		otherEnd := now.AddDate(0, 0, -1)
		otherStart := otherEnd.Add(-1 * time.Hour)
		broadcastDay := (int(matchingStart.Weekday()) + 6) % 7

		svc := &stubFavService{
			getByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
				return []model.FavoriteProgram{{
					ID:           1,
					UserID:       userID,
					StationID:    "TBS",
					ProgramTitle: "daily",
					BroadcastDay: &broadcastDay,
					CreatedAt:    now,
				}}, nil
			},
		}

		radiko := &stubRadikoService{
			getTwoWeekScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
				return []map[string]interface{}{
					{
						"entries": []map[string]interface{}{
							{
								"title": "daily",
								"cast":  "Matching DJ",
								"date":  matchingStart.Format("20060102"),
								"start": matchingStart.Format("15:04"),
								"end":   matchingEnd.Format("15:04"),
								"ft":    matchingStart.Format("20060102150405"),
								"to":    matchingEnd.Format("20060102150405"),
							},
							{
								"title": "daily",
								"cast":  "Other DJ",
								"date":  otherStart.Format("20060102"),
								"start": otherStart.Format("15:04"),
								"end":   otherEnd.Format("15:04"),
								"ft":    otherStart.Format("20060102150405"),
								"to":    otherEnd.Format("20060102150405"),
							},
						},
					},
				}, nil
			},
		}
		h := NewFavoriteHandler(svc, radiko, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}

		var resp struct {
			Favorites []struct {
				Cast       string
				Recordable bool
				RecDate    string
				RecStart   string
				RecEnd     string
			}
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if len(resp.Favorites) != 1 {
			t.Fatalf("favorites len = %d, want 1", len(resp.Favorites))
		}
		got := resp.Favorites[0]
		if !got.Recordable {
			t.Error("expected Recordable=true")
		}
		if got.Cast != "Matching DJ" {
			t.Errorf("Cast = %q, want Matching DJ", got.Cast)
		}
		if got.RecDate != matchingStart.Format("20060102") || got.RecStart != matchingStart.Format("15:04") || got.RecEnd != matchingEnd.Format("15:04") {
			t.Errorf("recording fields = %#v", got)
		}
	})
	t.Run("タイムフリーなし: 録音不可", func(t *testing.T) {
		svc := &stubFavService{
			getByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
				return []model.FavoriteProgram{{
					ID:           1,
					UserID:       userID,
					StationID:    "TBS",
					ProgramTitle: "jazz",
					CreatedAt:    time.Now(),
				}}, nil
			},
		}

		calls := 0
		end := time.Now().AddDate(0, 0, -8)
		radiko := &stubRadikoService{
			getTwoWeekScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
				calls++
				return []map[string]interface{}{
					{
						"entries": []map[string]interface{}{
							{
								"title": "jazz",
								"cast":  "DJ",
								"date":  end.Format("20060102"),
								"start": "10:00",
								"end":   "12:00",
								"to":    end.Format("20060102150405"),
							},
						},
					},
				}, nil
			},
		}
		h := NewFavoriteHandler(svc, radiko, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		if calls != 1 {
			t.Errorf("GetTwoWeekSchedule calls = %d, want 1", calls)
		}

		var resp struct {
			Favorites []struct {
				Recordable bool
			}
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if len(resp.Favorites) != 1 {
			t.Fatalf("favorites len = %d, want 1", len(resp.Favorites))
		}
		if resp.Favorites[0].Recordable {
			t.Error("expected Recordable=false")
		}
	})
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
		isErr bool
	}{
		{"42", 42, false},
		{"0", 0, false},
		{"-1", -1, false},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := parseInt(tt.input)
		if tt.isErr && err == nil {
			t.Errorf("parseInt(%q): expected error, got nil", tt.input)
		}
		if !tt.isErr && err != nil {
			t.Errorf("parseInt(%q): unexpected error %v", tt.input, err)
		}
		if !tt.isErr && got != tt.want {
			t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
