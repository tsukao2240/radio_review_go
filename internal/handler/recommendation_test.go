package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/service"
)

// stubRecommendService は RecommendationServiceInterface のスタブ実装。
type stubRecommendService struct {
	getRecommendationsFunc func(userID int64) ([]map[string]interface{}, error)
	getTrendingFunc        func(days, limit int) ([]map[string]interface{}, error)
	clearUserCacheFunc     func(userID int64) error
}

func (s *stubRecommendService) GetRecommendations(userID int64) ([]map[string]interface{}, error) {
	if s.getRecommendationsFunc != nil {
		return s.getRecommendationsFunc(userID)
	}
	return nil, nil
}
func (s *stubRecommendService) GetTrendingPrograms(days, limit int) ([]map[string]interface{}, error) {
	if s.getTrendingFunc != nil {
		return s.getTrendingFunc(days, limit)
	}
	return nil, nil
}
func (s *stubRecommendService) ClearUserCache(userID int64) error {
	if s.clearUserCacheFunc != nil {
		return s.clearUserCacheFunc(userID)
	}
	return nil
}

var _ service.RecommendationServiceInterface = (*stubRecommendService)(nil)

func TestRecommendationHandler_GetRecommendations(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewRecommendationHandler(&stubRecommendService{}, store)
		req := httptest.NewRequest(http.MethodGet, "/api/recommendations", nil)
		rr := httptest.NewRecorder()
		h.GetRecommendations(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("正常取得: 200", func(t *testing.T) {
		svc := &stubRecommendService{
			getRecommendationsFunc: func(userID int64) ([]map[string]interface{}, error) {
				return []map[string]interface{}{
					{"title": "jazz show", "station_id": "TBS"},
				}, nil
			},
		}
		h := NewRecommendationHandler(svc, store)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/api/recommendations", nil), 1)
		rr := httptest.NewRecorder()
		h.GetRecommendations(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if resp["success"] != true {
			t.Errorf("expected success=true")
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubRecommendService{
			getRecommendationsFunc: func(_ int64) ([]map[string]interface{}, error) {
				return nil, errors.New("service error")
			},
		}
		h := NewRecommendationHandler(svc, store)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/api/recommendations", nil), 1)
		rr := httptest.NewRecorder()
		h.GetRecommendations(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestRecommendationHandler_Refresh(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewRecommendationHandler(&stubRecommendService{}, store)
		req := httptest.NewRequest(http.MethodPost, "/api/recommendations/refresh", nil)
		rr := httptest.NewRecorder()
		h.Refresh(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("正常リフレッシュ: 200", func(t *testing.T) {
		var cacheCleared bool
		svc := &stubRecommendService{
			clearUserCacheFunc: func(_ int64) error {
				cacheCleared = true
				return nil
			},
			getRecommendationsFunc: func(_ int64) ([]map[string]interface{}, error) {
				return []map[string]interface{}{}, nil
			},
		}
		h := NewRecommendationHandler(svc, store)
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/recommendations/refresh", nil), 1)
		rr := httptest.NewRecorder()
		h.Refresh(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		if !cacheCleared {
			t.Error("cache should have been cleared")
		}
	})

	t.Run("キャッシュ削除エラー: 500", func(t *testing.T) {
		svc := &stubRecommendService{
			clearUserCacheFunc: func(_ int64) error {
				return errors.New("redis error")
			},
		}
		h := NewRecommendationHandler(svc, store)
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/recommendations/refresh", nil), 1)
		rr := httptest.NewRecorder()
		h.Refresh(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestRecommendationHandler_Index(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))

	t.Run("未認証: /loginにリダイレクト", func(t *testing.T) {
		h := NewRecommendationHandler(&stubRecommendService{}, store)
		req := httptest.NewRequest(http.MethodGet, "/recommendations", nil)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
	})

	t.Run("認証済み: 200", func(t *testing.T) {
		h := NewRecommendationHandler(&stubRecommendService{}, store)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/recommendations", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}
