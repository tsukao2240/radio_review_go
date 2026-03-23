package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

// withUserID はテスト用にユーザーIDをコンテキストに埋め込む。
func withUserID(r *http.Request, userID int64) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, userID)
	return r.WithContext(ctx)
}

func TestFavoriteHandler_Store(t *testing.T) {
	t.Run("正常追加: 200", func(t *testing.T) {
		svc := &stubFavService{}
		h := NewFavoriteHandler(svc, nil)

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
		h := NewFavoriteHandler(&stubFavService{}, nil)
		body, _ := json.Marshal(map[string]interface{}{"program_title": "jazz"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Store(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil)
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
		h := NewFavoriteHandler(svc, nil)
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
		h := NewFavoriteHandler(&stubFavService{}, nil)
		body, _ := json.Marshal(map[string]interface{}{"station_id": "TBS", "program_title": "jazz"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites/delete", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("station_id 未指定: 422", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil)
		body, _ := json.Marshal(map[string]interface{}{"program_title": "jazz"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/favorites/delete", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422", rr.Code)
		}
	})
}

func TestFavoriteHandler_Check(t *testing.T) {
	t.Run("登録済み: is_favorite=true", func(t *testing.T) {
		svc := &stubFavService{
			checkFunc: func(_ int64, _, _ string, _ *int) (bool, error) { return true, nil },
		}
		h := NewFavoriteHandler(svc, nil)
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
		h := NewFavoriteHandler(&stubFavService{}, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/favorites/check?station_id=TBS", nil), 1)
		rr := httptest.NewRecorder()
		h.Check(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", rr.Code)
		}
	})

	t.Run("未認証: is_favorite=false", func(t *testing.T) {
		h := NewFavoriteHandler(&stubFavService{}, nil)
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
