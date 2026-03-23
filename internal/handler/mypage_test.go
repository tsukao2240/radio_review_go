package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/service"
)

// stubPostService は PostServiceInterface のスタブ実装。
type stubPostService struct {
	getAllPostsFunc         func(perPage, page int) ([]model.Post, int, error)
	getPostsByUserFunc     func(userID int64, perPage, page int) ([]model.Post, int, error)
	getPostByIDFunc        func(postID int64) (*model.Post, error)
	updatePostFunc         func(postID int64, data map[string]interface{}) error
	deletePostFunc         func(postID int64) error
	createPostFunc         func(data map[string]interface{}, userID int64) (*model.Post, error)
	getPostsByProgramFunc  func(stationID, programTitle string, perPage, page int) ([]model.Post, int, error)
	getPostsFilteredFunc   func(filters map[string]interface{}, perPage, page int) ([]model.Post, int, error)
	getAvgRatingFunc       func(programID int64) (float64, error)
	getAllTagsFunc          func() ([]model.PostTag, error)
}

func (s *stubPostService) GetAllPosts(perPage, page int) ([]model.Post, int, error) {
	if s.getAllPostsFunc != nil {
		return s.getAllPostsFunc(perPage, page)
	}
	return nil, 0, nil
}
func (s *stubPostService) GetPostsByUser(userID int64, perPage, page int) ([]model.Post, int, error) {
	if s.getPostsByUserFunc != nil {
		return s.getPostsByUserFunc(userID, perPage, page)
	}
	return nil, 0, nil
}
func (s *stubPostService) GetPostByID(postID int64) (*model.Post, error) {
	if s.getPostByIDFunc != nil {
		return s.getPostByIDFunc(postID)
	}
	return nil, nil
}
func (s *stubPostService) UpdatePost(postID int64, data map[string]interface{}) error {
	if s.updatePostFunc != nil {
		return s.updatePostFunc(postID, data)
	}
	return nil
}
func (s *stubPostService) DeletePost(postID int64) error {
	if s.deletePostFunc != nil {
		return s.deletePostFunc(postID)
	}
	return nil
}
func (s *stubPostService) CreatePost(data map[string]interface{}, userID int64) (*model.Post, error) {
	if s.createPostFunc != nil {
		return s.createPostFunc(data, userID)
	}
	return &model.Post{ID: 1}, nil
}
func (s *stubPostService) GetPostsByProgram(stationID, programTitle string, perPage, page int) ([]model.Post, int, error) {
	if s.getPostsByProgramFunc != nil {
		return s.getPostsByProgramFunc(stationID, programTitle, perPage, page)
	}
	return nil, 0, nil
}
func (s *stubPostService) GetPostsFiltered(filters map[string]interface{}, perPage, page int) ([]model.Post, int, error) {
	if s.getPostsFilteredFunc != nil {
		return s.getPostsFilteredFunc(filters, perPage, page)
	}
	return nil, 0, nil
}
func (s *stubPostService) GetAverageRatingByProgram(programID int64) (float64, error) {
	if s.getAvgRatingFunc != nil {
		return s.getAvgRatingFunc(programID)
	}
	return 0, nil
}
func (s *stubPostService) GetAllTags() ([]model.PostTag, error) {
	if s.getAllTagsFunc != nil {
		return s.getAllTagsFunc()
	}
	return nil, nil
}

var _ service.PostServiceInterface = (*stubPostService)(nil)

func TestMypageHandler_Index(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))

	t.Run("未認証: /loginにリダイレクト", func(t *testing.T) {
		h := NewMypageHandler(&stubPostService{}, store)
		req := httptest.NewRequest(http.MethodGet, "/my", nil)
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
		svc := &stubPostService{
			getPostsByUserFunc: func(userID int64, perPage, page int) ([]model.Post, int, error) {
				return []model.Post{{ID: 1, UserID: userID}}, 1, nil
			},
		}
		h := NewMypageHandler(svc, store)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/my", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubPostService{
			getPostsByUserFunc: func(_ int64, _, _ int) ([]model.Post, int, error) {
				return nil, 0, errors.New("db error")
			},
		}
		h := NewMypageHandler(svc, store)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/my", nil), 1)
		rr := httptest.NewRecorder()
		h.Index(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestMypageHandler_Edit(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))

	t.Run("未認証: /loginにリダイレクト", func(t *testing.T) {
		h := NewMypageHandler(&stubPostService{}, store)
		r := chi.NewRouter()
		r.Get("/my/edit/{program_id}", h.Edit)
		req := httptest.NewRequest(http.MethodGet, "/my/edit/1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
	})

	t.Run("他ユーザーの投稿: 403", func(t *testing.T) {
		svc := &stubPostService{
			getPostByIDFunc: func(postID int64) (*model.Post, error) {
				return &model.Post{ID: postID, UserID: 99}, nil // 別ユーザーの投稿
			},
		}
		h := NewMypageHandler(svc, store)
		r := chi.NewRouter()
		r.Get("/my/edit/{program_id}", h.Edit)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/my/edit/1", nil), 1)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("got %d, want 403", rr.Code)
		}
	})

	t.Run("投稿が見つからない: 404", func(t *testing.T) {
		svc := &stubPostService{
			getPostByIDFunc: func(postID int64) (*model.Post, error) {
				return nil, errors.New("not found")
			},
		}
		h := NewMypageHandler(svc, store)
		r := chi.NewRouter()
		r.Get("/my/edit/{program_id}", h.Edit)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/my/edit/1", nil), 1)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("got %d, want 404", rr.Code)
		}
	})

	t.Run("自分の投稿: 200", func(t *testing.T) {
		svc := &stubPostService{
			getPostByIDFunc: func(postID int64) (*model.Post, error) {
				return &model.Post{ID: postID, UserID: 1}, nil
			},
		}
		h := NewMypageHandler(svc, store)
		r := chi.NewRouter()
		r.Get("/my/edit/{program_id}", h.Edit)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/my/edit/1", nil), 1)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}

func TestMypageHandler_Update(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))
	FlashStore = store

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewMypageHandler(&stubPostService{}, store)
		r := chi.NewRouter()
		r.Post("/my/edit/{program_id}", h.Update)
		req := httptest.NewRequest(http.MethodPost, "/my/edit/1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("正常更新: /myにリダイレクト", func(t *testing.T) {
		svc := &stubPostService{
			updatePostFunc: func(postID int64, data map[string]interface{}) error { return nil },
		}
		h := NewMypageHandler(svc, store)
		r := chi.NewRouter()
		r.Post("/my/edit/{program_id}", h.Update)

		form := url.Values{
			"title":  {"new title"},
			"body":   {"new body"},
			"rating": {"4.0"},
		}
		req := withUserID(httptest.NewRequest(http.MethodPost, "/my/edit/1", strings.NewReader(form.Encode())), 1)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/my" {
			t.Errorf("got Location=%q, want /my", loc)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubPostService{
			updatePostFunc: func(_ int64, _ map[string]interface{}) error {
				return errors.New("update error")
			},
		}
		h := NewMypageHandler(svc, store)
		r := chi.NewRouter()
		r.Post("/my/edit/{program_id}", h.Update)

		form := url.Values{"title": {"t"}, "body": {"b"}, "rating": {"3.0"}}
		req := withUserID(httptest.NewRequest(http.MethodPost, "/my/edit/1", strings.NewReader(form.Encode())), 1)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestMypageHandler_Destroy(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))
	FlashStore = store

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewMypageHandler(&stubPostService{}, store)
		form := url.Values{"post_id": {"1"}}
		req := httptest.NewRequest(http.MethodPost, "/my", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("他ユーザーの投稿削除: 403", func(t *testing.T) {
		svc := &stubPostService{
			getPostByIDFunc: func(postID int64) (*model.Post, error) {
				return &model.Post{ID: postID, UserID: 99}, nil
			},
		}
		h := NewMypageHandler(svc, store)
		form := url.Values{"post_id": {"1"}}
		req := withUserID(httptest.NewRequest(http.MethodPost, "/my", strings.NewReader(form.Encode())), 1)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("got %d, want 403", rr.Code)
		}
	})

	t.Run("正常削除: /myにリダイレクト", func(t *testing.T) {
		svc := &stubPostService{
			getPostByIDFunc: func(postID int64) (*model.Post, error) {
				return &model.Post{ID: postID, UserID: 1}, nil
			},
			deletePostFunc: func(postID int64) error { return nil },
		}
		h := NewMypageHandler(svc, store)
		form := url.Values{"post_id": {"1"}}
		req := withUserID(httptest.NewRequest(http.MethodPost, "/my", strings.NewReader(form.Encode())), 1)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		h.Destroy(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/my" {
			t.Errorf("got Location=%q, want /my", loc)
		}
	})
}
