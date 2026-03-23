package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/service"
)

// stubRadikoService は RadikoApiServiceInterface のスタブ実装。
type stubRadikoService struct {
	getCurrentProgramsFunc func() ([]map[string]interface{}, error)
	getWeeklyScheduleFunc  func(stationID string) ([]map[string]interface{}, error)
	getTwoWeekScheduleFunc func(stationID string) ([]map[string]interface{}, error)
	getProgramDetailsFunc  func(stationID, title string) (map[string]interface{}, error)
}

func (s *stubRadikoService) GetCurrentPrograms() ([]map[string]interface{}, error) {
	if s.getCurrentProgramsFunc != nil {
		return s.getCurrentProgramsFunc()
	}
	return nil, nil
}

func (s *stubRadikoService) GetWeeklySchedule(stationID string) ([]map[string]interface{}, error) {
	if s.getWeeklyScheduleFunc != nil {
		return s.getWeeklyScheduleFunc(stationID)
	}
	return nil, nil
}

func (s *stubRadikoService) GetTwoWeekSchedule(stationID string) ([]map[string]interface{}, error) {
	if s.getTwoWeekScheduleFunc != nil {
		return s.getTwoWeekScheduleFunc(stationID)
	}
	return nil, nil
}

func (s *stubRadikoService) GetProgramDetails(stationID, title string) (map[string]interface{}, error) {
	if s.getProgramDetailsFunc != nil {
		return s.getProgramDetailsFunc(stationID, title)
	}
	return nil, nil
}

// stubSearchService は RadioProgramSearchServiceInterface のスタブ実装。
type stubSearchService struct {
	searchForAPIFunc    func(keyword, cast *string, stationID *string, limit int) ([]model.RadioProgram, error)
	searchByTitleCalled bool
	searchForAPICalled  bool
	lastKeyword         *string
	lastCast            *string
	lastStationID       *string
}

func (s *stubSearchService) SearchByTitle(keyword string, stationID *string) ([]model.RadioProgram, error) {
	s.searchByTitleCalled = true
	return nil, nil
}

func (s *stubSearchService) SearchByCast(cast string, stationID *string) ([]model.RadioProgram, error) {
	return nil, nil
}

func (s *stubSearchService) SearchProgramsWithPosts(keyword string, perPage, page int) ([]model.RadioProgram, int, error) {
	return nil, 0, nil
}

func (s *stubSearchService) SearchForAPI(keyword, cast *string, stationID *string, limit int) ([]model.RadioProgram, error) {
	s.searchForAPICalled = true
	s.lastKeyword = keyword
	s.lastCast = cast
	s.lastStationID = stationID
	if s.searchForAPIFunc != nil {
		return s.searchForAPIFunc(keyword, cast, stationID, limit)
	}
	return []model.RadioProgram{}, nil
}

func (s *stubSearchService) GetAllPrograms(perPage, page int) ([]model.RadioProgram, int, error) {
	return nil, 0, nil
}

// 型チェック：スタブがインターフェースを満たすことを確認する。
var _ service.RadikoApiServiceInterface = (*stubRadikoService)(nil)
var _ service.RadioProgramSearchServiceInterface = (*stubSearchService)(nil)

func TestBroadcastHandlerSearch(t *testing.T) {
	t.Run("no keyword returns 200 without calling SearchForAPI", func(t *testing.T) {
		radikoSvc := &stubRadikoService{}
		searchSvc := &stubSearchService{}
		h := NewBroadcastHandler(radikoSvc, searchSvc)

		req := httptest.NewRequest(http.MethodGet, "/search", nil)
		rr := httptest.NewRecorder()
		h.Search(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
		}
		if searchSvc.searchForAPICalled {
			t.Error("SearchForAPI should not be called when no keyword is provided")
		}
	})

	t.Run("keyword param calls SearchForAPI and returns 200", func(t *testing.T) {
		radikoSvc := &stubRadikoService{}
		searchSvc := &stubSearchService{}
		h := NewBroadcastHandler(radikoSvc, searchSvc)

		req := httptest.NewRequest(http.MethodGet, "/search?keyword=jazz", nil)
		rr := httptest.NewRecorder()
		h.Search(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
		}
		if !searchSvc.searchForAPICalled {
			t.Error("SearchForAPI should be called when keyword is provided")
		}
		if searchSvc.lastKeyword == nil || *searchSvc.lastKeyword != "jazz" {
			t.Errorf("SearchForAPI called with wrong keyword: %v", searchSvc.lastKeyword)
		}
	})

	t.Run("cast param calls SearchForAPI", func(t *testing.T) {
		radikoSvc := &stubRadikoService{}
		searchSvc := &stubSearchService{}
		h := NewBroadcastHandler(radikoSvc, searchSvc)

		req := httptest.NewRequest(http.MethodGet, "/search?cast=SomePerson", nil)
		rr := httptest.NewRecorder()
		h.Search(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
		}
		if !searchSvc.searchForAPICalled {
			t.Error("SearchForAPI should be called when cast is provided")
		}
		if searchSvc.lastCast == nil || *searchSvc.lastCast != "SomePerson" {
			t.Errorf("SearchForAPI called with wrong cast: %v", searchSvc.lastCast)
		}
	})

	t.Run("search result is included in JSON response when no template", func(t *testing.T) {
		radikoSvc := &stubRadikoService{}
		searchSvc := &stubSearchService{
			searchForAPIFunc: func(keyword, cast *string, stationID *string, limit int) ([]model.RadioProgram, error) {
				return []model.RadioProgram{
					{ID: 1, StationID: "TBS", Title: "Jazz Program"},
				}, nil
			},
		}
		h := NewBroadcastHandler(radikoSvc, searchSvc)

		req := httptest.NewRequest(http.MethodGet, "/search?keyword=jazz", nil)
		rr := httptest.NewRecorder()
		h.Search(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
		}

		// template does not exist in test, so JSON fallback is used
		contentType := rr.Header().Get("Content-Type")
		if contentType == "application/json" {
			var result map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
				t.Fatalf("decode JSON: %v", err)
			}
			if result["Keyword"] != "jazz" {
				t.Errorf("keyword in response: %v", result["Keyword"])
			}
		}
	})

	t.Run("SearchForAPI error returns 500", func(t *testing.T) {
		radikoSvc := &stubRadikoService{}
		searchSvc := &stubSearchService{
			searchForAPIFunc: func(keyword, cast *string, stationID *string, limit int) ([]model.RadioProgram, error) {
				return nil, errSearchFailed
			},
		}
		h := NewBroadcastHandler(radikoSvc, searchSvc)

		req := httptest.NewRequest(http.MethodGet, "/search?keyword=error", nil)
		rr := httptest.NewRecorder()
		h.Search(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got status %d, want %d", rr.Code, http.StatusInternalServerError)
		}
	})
}

// errSearchFailed はテスト用エラー。
var errSearchFailed = &searchError{"search service error"}

type searchError struct{ msg string }

func (e *searchError) Error() string { return e.msg }

func TestBroadcastHandler_GetCurrentSchedule(t *testing.T) {
	t.Run("正常: 200", func(t *testing.T) {
		svc := &stubRadikoService{
			getCurrentProgramsFunc: func() ([]map[string]interface{}, error) {
				return []map[string]interface{}{{"station_id": "TBS", "title": "jazz"}}, nil
			},
		}
		h := NewBroadcastHandler(svc, &stubSearchService{})
		req := httptest.NewRequest(http.MethodGet, "/schedule", nil)
		rr := httptest.NewRecorder()
		h.GetCurrentSchedule(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubRadikoService{
			getCurrentProgramsFunc: func() ([]map[string]interface{}, error) {
				return nil, errors.New("fetch error")
			},
		}
		h := NewBroadcastHandler(svc, &stubSearchService{})
		req := httptest.NewRequest(http.MethodGet, "/schedule", nil)
		rr := httptest.NewRecorder()
		h.GetCurrentSchedule(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestBroadcastHandler_GetWeeklySchedule(t *testing.T) {
	t.Run("正常: 200", func(t *testing.T) {
		svc := &stubRadikoService{
			getWeeklyScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
				return []map[string]interface{}{{"broadcast_name": "TBSラジオ", "entries": []map[string]interface{}{}}}, nil
			},
		}
		h := NewBroadcastHandler(svc, &stubSearchService{})
		r := chi.NewRouter()
		r.Get("/schedule/{station_id}", h.GetWeeklySchedule)
		req := httptest.NewRequest(http.MethodGet, "/schedule/TBS", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubRadikoService{
			getWeeklyScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
				return nil, errors.New("fetch error")
			},
		}
		h := NewBroadcastHandler(svc, &stubSearchService{})
		r := chi.NewRouter()
		r.Get("/schedule/{station_id}", h.GetWeeklySchedule)
		req := httptest.NewRequest(http.MethodGet, "/schedule/TBS", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestBroadcastHandler_ShowProgramDetail(t *testing.T) {
	t.Run("正常: 200", func(t *testing.T) {
		svc := &stubRadikoService{
			getProgramDetailsFunc: func(stationID, title string) (map[string]interface{}, error) {
				return map[string]interface{}{"title": title, "station_id": stationID}, nil
			},
		}
		h := NewBroadcastHandler(svc, &stubSearchService{})
		r := chi.NewRouter()
		r.Get("/list/{station_id}/{title}", h.ShowProgramDetail)
		req := httptest.NewRequest(http.MethodGet, "/list/TBS/jazz+show", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubRadikoService{
			getProgramDetailsFunc: func(stationID, title string) (map[string]interface{}, error) {
				return nil, errors.New("fetch error")
			},
		}
		h := NewBroadcastHandler(svc, &stubSearchService{})
		r := chi.NewRouter()
		r.Get("/list/{station_id}/{title}", h.ShowProgramDetail)
		req := httptest.NewRequest(http.MethodGet, "/list/TBS/jazz+show", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestBroadcastHandler_GetTwoWeekScheduleByStation(t *testing.T) {
	t.Run("正常: 200", func(t *testing.T) {
		svc := &stubRadikoService{
			getTwoWeekScheduleFunc: func(stationID string) ([]map[string]interface{}, error) {
				return []map[string]interface{}{{"broadcast_name": "TBSラジオ", "entries": []map[string]interface{}{}}}, nil
			},
		}
		h := NewBroadcastHandler(svc, &stubSearchService{})
		r := chi.NewRouter()
		r.Get("/timefree/{station_id}", h.GetTwoWeekScheduleByStation)
		req := httptest.NewRequest(http.MethodGet, "/timefree/TBS", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}

func TestBroadcastHandler_GetTwoWeekScheduleSelect(t *testing.T) {
	// fetchStationListForHandler は実際の Radiko API を呼ぶが、エラー時でも 200 を返す
	h := NewBroadcastHandler(&stubRadikoService{}, &stubSearchService{})
	req := httptest.NewRequest(http.MethodGet, "/timefree?area=JP13", nil)
	rr := httptest.NewRecorder()
	h.GetTwoWeekScheduleSelect(rr, req)
	// エラー時でも 200 OK（空の station list で表示）
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestRespondJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	data := map[string]string{"key": "value"}
	respondJSON(rr, data)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "value") {
		t.Errorf("expected 'value' in body, got %q", rr.Body.String())
	}
}

func TestRespondJSON_EncodeError(t *testing.T) {
	rr := httptest.NewRecorder()
	// channels cannot be JSON encoded → triggers the error log path
	respondJSON(rr, make(chan int))
	// Should not panic; Content-Type is still set
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}
