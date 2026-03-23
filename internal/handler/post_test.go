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

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/service"
)

// stubInteractionService は PostInteractionServiceInterface のスタブ実装。
type stubInteractionService struct {
	likeFunc          func(postID, userID int64) error
	unlikeFunc        func(postID, userID int64) error
	addCommentFunc    func(postID, userID int64, body string) (*model.PostComment, error)
	deleteCommentFunc func(commentID, userID int64) error
	getCommentsFunc   func(postID int64) ([]model.PostComment, error)
	isLikedByFunc     func(postID, userID int64) (bool, error)
}

func (s *stubInteractionService) Like(postID, userID int64) error {
	if s.likeFunc != nil {
		return s.likeFunc(postID, userID)
	}
	return nil
}
func (s *stubInteractionService) Unlike(postID, userID int64) error {
	if s.unlikeFunc != nil {
		return s.unlikeFunc(postID, userID)
	}
	return nil
}
func (s *stubInteractionService) AddComment(postID, userID int64, body string) (*model.PostComment, error) {
	if s.addCommentFunc != nil {
		return s.addCommentFunc(postID, userID, body)
	}
	return &model.PostComment{ID: 1}, nil
}
func (s *stubInteractionService) DeleteComment(commentID, userID int64) error {
	if s.deleteCommentFunc != nil {
		return s.deleteCommentFunc(commentID, userID)
	}
	return nil
}
func (s *stubInteractionService) GetComments(postID int64) ([]model.PostComment, error) {
	if s.getCommentsFunc != nil {
		return s.getCommentsFunc(postID)
	}
	return nil, nil
}
func (s *stubInteractionService) IsLikedBy(postID, userID int64) (bool, error) {
	if s.isLikedByFunc != nil {
		return s.isLikedByFunc(postID, userID)
	}
	return false, nil
}

var _ service.PostInteractionServiceInterface = (*stubInteractionService)(nil)

func newPostHandler() *PostHandler {
	return NewPostHandler(&stubPostService{}, &stubInteractionService{}, sessions.NewCookieStore([]byte("test")))
}

func TestPostHandler_ListAllReviews(t *testing.T) {
	t.Run("正常: 200", func(t *testing.T) {
		svc := &stubPostService{
			getAllPostsFunc: func(_, _ int) ([]model.Post, int, error) {
				return []model.Post{{ID: 1}}, 1, nil
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/review/list", nil)
		rr := httptest.NewRecorder()
		h.ListAllReviews(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubPostService{
			getAllPostsFunc: func(_, _ int) ([]model.Post, int, error) {
				return nil, 0, errors.New("db error")
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/review/list", nil)
		rr := httptest.NewRecorder()
		h.ListAllReviews(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestPostHandler_CreateReview(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test"))
	FlashStore = store

	t.Run("未認証: 401", func(t *testing.T) {
		h := NewPostHandler(&stubPostService{}, &stubInteractionService{}, store)
		r := chi.NewRouter()
		r.Post("/review/{id}", h.CreateReview)
		req := httptest.NewRequest(http.MethodPost, "/review/1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("正常投稿: リダイレクト", func(t *testing.T) {
		svc := &stubPostService{
			createPostFunc: func(data map[string]interface{}, userID int64) (*model.Post, error) {
				return &model.Post{ID: 1}, nil
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, store)
		r := chi.NewRouter()
		r.Post("/review/{id}", h.CreateReview)

		form := url.Values{
			"station_id":    {"TBS"},
			"program_title": {"jazz show"},
			"title":         {"great"},
			"body":          {"loved it"},
			"rating":        {"4.5"},
		}
		req := withUserID(httptest.NewRequest(http.MethodPost, "/review/1", strings.NewReader(form.Encode())), 1)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubPostService{
			createPostFunc: func(_ map[string]interface{}, _ int64) (*model.Post, error) {
				return nil, errors.New("create error")
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, store)
		r := chi.NewRouter()
		r.Post("/review/{id}", h.CreateReview)

		form := url.Values{"title": {"t"}, "body": {"b"}, "rating": {"3.0"}}
		req := withUserID(httptest.NewRequest(http.MethodPost, "/review/1", strings.NewReader(form.Encode())), 1)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestPostHandler_LikePost(t *testing.T) {
	t.Run("未認証: 401", func(t *testing.T) {
		h := newPostHandler()
		req := httptest.NewRequest(http.MethodPost, "/api/posts/like", nil)
		rr := httptest.NewRecorder()
		h.LikePost(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("正常いいね: 200", func(t *testing.T) {
		svc := &stubInteractionService{}
		h := NewPostHandler(&stubPostService{}, svc, nil)
		body, _ := json.Marshal(map[string]int64{"post_id": 5})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/posts/like", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.LikePost(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubInteractionService{
			likeFunc: func(_, _ int64) error { return errors.New("like error") },
		}
		h := NewPostHandler(&stubPostService{}, svc, nil)
		body, _ := json.Marshal(map[string]int64{"post_id": 5})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/posts/like", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.LikePost(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestPostHandler_UnlikePost(t *testing.T) {
	t.Run("正常いいね取消: 200", func(t *testing.T) {
		h := NewPostHandler(&stubPostService{}, &stubInteractionService{}, nil)
		body, _ := json.Marshal(map[string]int64{"post_id": 5})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/posts/unlike", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.UnlikePost(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}

func TestPostHandler_AddComment(t *testing.T) {
	t.Run("未認証: 401", func(t *testing.T) {
		h := newPostHandler()
		req := httptest.NewRequest(http.MethodPost, "/api/posts/comment", nil)
		rr := httptest.NewRecorder()
		h.AddComment(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})

	t.Run("正常コメント追加: 200", func(t *testing.T) {
		svc := &stubInteractionService{
			addCommentFunc: func(postID, userID int64, body string) (*model.PostComment, error) {
				return &model.PostComment{ID: 10, Body: body}, nil
			},
		}
		h := NewPostHandler(&stubPostService{}, svc, nil)
		reqBody, _ := json.Marshal(map[string]interface{}{"post_id": 1, "body": "great comment"})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/posts/comment", bytes.NewReader(reqBody)), 1)
		rr := httptest.NewRecorder()
		h.AddComment(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}

func TestPostHandler_GetComments(t *testing.T) {
	t.Run("コメント一覧: 200", func(t *testing.T) {
		svc := &stubInteractionService{
			getCommentsFunc: func(postID int64) ([]model.PostComment, error) {
				return []model.PostComment{{ID: 1, Body: "good"}}, nil
			},
		}
		h := NewPostHandler(&stubPostService{}, svc, nil)
		req := httptest.NewRequest(http.MethodGet, "/api/posts/comments?post_id=1", nil)
		rr := httptest.NewRecorder()
		h.GetComments(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})

	t.Run("post_id 不正: 400", func(t *testing.T) {
		h := newPostHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/posts/comments?post_id=invalid", nil)
		rr := httptest.NewRecorder()
		h.GetComments(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", rr.Code)
		}
	})
}

func TestPostHandler_CheckLike(t *testing.T) {
	t.Run("未認証: is_liked=false", func(t *testing.T) {
		h := newPostHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/posts/check-like?post_id=1", nil)
		rr := httptest.NewRecorder()
		h.CheckLike(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		var resp map[string]bool
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp["is_liked"] {
			t.Error("unauthenticated should return is_liked=false")
		}
	})

	t.Run("いいね済み: is_liked=true", func(t *testing.T) {
		svc := &stubInteractionService{
			isLikedByFunc: func(_, _ int64) (bool, error) { return true, nil },
		}
		h := NewPostHandler(&stubPostService{}, svc, nil)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/api/posts/check-like?post_id=1", nil), 1)
		rr := httptest.NewRecorder()
		h.CheckLike(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		var resp map[string]bool
		json.NewDecoder(rr.Body).Decode(&resp)
		if !resp["is_liked"] {
			t.Error("expected is_liked=true")
		}
	})
}

func TestPostHandler_GetProgramRating(t *testing.T) {
	t.Run("正常: 200", func(t *testing.T) {
		svc := &stubPostService{
			getAvgRatingFunc: func(programID int64) (float64, error) {
				return 4.2, nil
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, nil)
		r := chi.NewRouter()
		r.Get("/program/{program_id}/rating", h.GetProgramRating)
		req := httptest.NewRequest(http.MethodGet, "/program/1/rating", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if resp["average_rating"] != 4.2 {
			t.Errorf("got average_rating=%v, want 4.2", resp["average_rating"])
		}
	})
}

func TestPostHandler_IndexPrograms(t *testing.T) {
	t.Run("正常: 200", func(t *testing.T) {
		svc := &stubPostService{
			getPostsFilteredFunc: func(_ map[string]interface{}, _, _ int) ([]model.Post, int, error) {
				return []model.Post{{ID: 1}}, 1, nil
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/program", nil)
		rr := httptest.NewRecorder()
		h.IndexPrograms(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubPostService{
			getPostsFilteredFunc: func(_ map[string]interface{}, _, _ int) ([]model.Post, int, error) {
				return nil, 0, errors.New("db error")
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/program", nil)
		rr := httptest.NewRecorder()
		h.IndexPrograms(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestPostHandler_ShowReviewForm(t *testing.T) {
	t.Run("未認証: /loginにリダイレクト", func(t *testing.T) {
		h := newPostHandler()
		r := chi.NewRouter()
		r.Get("/review/{id}", h.ShowReviewForm)
		req := httptest.NewRequest(http.MethodGet, "/review/1", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusFound {
			t.Errorf("got %d, want 302", rr.Code)
		}
	})
	t.Run("認証済み: 200", func(t *testing.T) {
		h := newPostHandler()
		r := chi.NewRouter()
		r.Get("/review/{id}", h.ShowReviewForm)
		req := withUserID(httptest.NewRequest(http.MethodGet, "/review/1", nil), 1)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
}

func TestPostHandler_ListReviewsByProgram(t *testing.T) {
	t.Run("正常: 200", func(t *testing.T) {
		svc := &stubPostService{
			getPostsByProgramFunc: func(stationID, programTitle string, perPage, page int) ([]model.Post, int, error) {
				return []model.Post{{ID: 1}}, 1, nil
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, nil)
		r := chi.NewRouter()
		r.Get("/list/{station_id}/{title}/review", h.ListReviewsByProgram)
		req := httptest.NewRequest(http.MethodGet, "/list/TBS/jazz+show/review", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubPostService{
			getPostsByProgramFunc: func(_, _ string, _, _ int) ([]model.Post, int, error) {
				return nil, 0, errors.New("db error")
			},
		}
		h := NewPostHandler(svc, &stubInteractionService{}, nil)
		r := chi.NewRouter()
		r.Get("/list/{station_id}/{title}/review", h.ListReviewsByProgram)
		req := httptest.NewRequest(http.MethodGet, "/list/TBS/jazz+show/review", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestPostHandler_DeleteComment(t *testing.T) {
	t.Run("未認証: 401", func(t *testing.T) {
		h := newPostHandler()
		body, _ := json.Marshal(map[string]int64{"comment_id": 1})
		req := httptest.NewRequest(http.MethodPost, "/api/posts/comment/delete", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.DeleteComment(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", rr.Code)
		}
	})
	t.Run("正常削除: 200", func(t *testing.T) {
		svc := &stubInteractionService{
			deleteCommentFunc: func(commentID, userID int64) error { return nil },
		}
		h := NewPostHandler(&stubPostService{}, svc, nil)
		body, _ := json.Marshal(map[string]int64{"comment_id": 5})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/posts/comment/delete", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.DeleteComment(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rr.Code)
		}
	})
	t.Run("サービスエラー: 500", func(t *testing.T) {
		svc := &stubInteractionService{
			deleteCommentFunc: func(_, _ int64) error { return errors.New("delete error") },
		}
		h := NewPostHandler(&stubPostService{}, svc, nil)
		body, _ := json.Marshal(map[string]int64{"comment_id": 5})
		req := withUserID(httptest.NewRequest(http.MethodPost, "/api/posts/comment/delete", bytes.NewReader(body)), 1)
		rr := httptest.NewRecorder()
		h.DeleteComment(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", rr.Code)
		}
	})
}

func TestPostHandler_UnlikePost_Success(t *testing.T) {
	svc := &stubInteractionService{
		unlikeFunc: func(_, _ int64) error { return nil },
	}
	h := NewPostHandler(&stubPostService{}, svc, nil)
	body, _ := json.Marshal(map[string]int64{"post_id": 3})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/posts/unlike", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.UnlikePost(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestPostHandler_UnlikePost_ServiceError(t *testing.T) {
	svc := &stubInteractionService{
		unlikeFunc: func(_, _ int64) error { return errors.New("unlike error") },
	}
	h := NewPostHandler(&stubPostService{}, svc, nil)
	body, _ := json.Marshal(map[string]int64{"post_id": 3})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/api/posts/unlike", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.UnlikePost(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestPostHandler_GetComments_ServiceError(t *testing.T) {
	svc := &stubInteractionService{
		getCommentsFunc: func(_ int64) ([]model.PostComment, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewPostHandler(&stubPostService{}, svc, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/posts/comments?post_id=1", nil)
	rr := httptest.NewRecorder()
	h.GetComments(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}
