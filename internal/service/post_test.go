package service

import (
	"errors"
	"testing"

	"github.com/yourname/radio_review_go/internal/model"
)

// --- stub: PostRepository ---

type stubPostRepo struct {
	findAllFunc        func(limit, offset int) ([]model.Post, error)
	countFunc          func() (int, error)
	findByProgramFunc  func(stationID, programTitle string, limit, offset int) ([]model.Post, error)
	countByProgramFunc func(stationID, programTitle string) (int, error)
	findByUserFunc     func(userID int64, limit, offset int) ([]model.Post, error)
	countByUserFunc    func(userID int64) (int, error)
	findByIDFunc       func(id int64) (*model.Post, error)
	findFilteredFunc   func(filters map[string]interface{}, limit, offset int) ([]model.Post, error)
	countFilteredFunc  func(filters map[string]interface{}) (int, error)
	createFunc         func(post *model.Post) (int64, error)
	updateFunc         func(post *model.Post) error
	deleteFunc         func(id int64) error
	avgRatingFunc      func(programID int64) (float64, error)
}

func (r *stubPostRepo) FindAll(limit, offset int) ([]model.Post, error) {
	if r.findAllFunc != nil {
		return r.findAllFunc(limit, offset)
	}
	return nil, nil
}
func (r *stubPostRepo) Count() (int, error) {
	if r.countFunc != nil {
		return r.countFunc()
	}
	return 0, nil
}
func (r *stubPostRepo) FindByProgram(stationID, programTitle string, limit, offset int) ([]model.Post, error) {
	if r.findByProgramFunc != nil {
		return r.findByProgramFunc(stationID, programTitle, limit, offset)
	}
	return nil, nil
}
func (r *stubPostRepo) CountByProgram(stationID, programTitle string) (int, error) {
	if r.countByProgramFunc != nil {
		return r.countByProgramFunc(stationID, programTitle)
	}
	return 0, nil
}
func (r *stubPostRepo) FindByUser(userID int64, limit, offset int) ([]model.Post, error) {
	if r.findByUserFunc != nil {
		return r.findByUserFunc(userID, limit, offset)
	}
	return nil, nil
}
func (r *stubPostRepo) CountByUser(userID int64) (int, error) {
	if r.countByUserFunc != nil {
		return r.countByUserFunc(userID)
	}
	return 0, nil
}
func (r *stubPostRepo) FindByID(id int64) (*model.Post, error) {
	if r.findByIDFunc != nil {
		return r.findByIDFunc(id)
	}
	return nil, nil
}
func (r *stubPostRepo) FindFiltered(filters map[string]interface{}, limit, offset int) ([]model.Post, error) {
	if r.findFilteredFunc != nil {
		return r.findFilteredFunc(filters, limit, offset)
	}
	return nil, nil
}
func (r *stubPostRepo) CountFiltered(filters map[string]interface{}) (int, error) {
	if r.countFilteredFunc != nil {
		return r.countFilteredFunc(filters)
	}
	return 0, nil
}
func (r *stubPostRepo) Create(post *model.Post) (int64, error) {
	if r.createFunc != nil {
		return r.createFunc(post)
	}
	return 1, nil
}
func (r *stubPostRepo) Update(post *model.Post) error {
	if r.updateFunc != nil {
		return r.updateFunc(post)
	}
	return nil
}
func (r *stubPostRepo) Delete(id int64) error {
	if r.deleteFunc != nil {
		return r.deleteFunc(id)
	}
	return nil
}
func (r *stubPostRepo) AverageRating(programID int64) (float64, error) {
	if r.avgRatingFunc != nil {
		return r.avgRatingFunc(programID)
	}
	return 0, nil
}

// --- stub: PostTagRepository ---

type stubTagRepo struct {
	findAllFunc        func() ([]model.PostTag, error)
	findByPostIDFunc   func(postID int64) ([]model.PostTag, error)
	attachToPostFunc   func(postID, tagID int64) error
	detachFromPostFunc func(postID, tagID int64) error
}

func (r *stubTagRepo) FindAll() ([]model.PostTag, error) {
	if r.findAllFunc != nil {
		return r.findAllFunc()
	}
	return nil, nil
}
func (r *stubTagRepo) FindByID(id int64) (*model.PostTag, error) { return nil, nil }
func (r *stubTagRepo) FindByPostID(postID int64) ([]model.PostTag, error) {
	if r.findByPostIDFunc != nil {
		return r.findByPostIDFunc(postID)
	}
	return nil, nil
}
func (r *stubTagRepo) AttachToPost(postID, tagID int64) error {
	if r.attachToPostFunc != nil {
		return r.attachToPostFunc(postID, tagID)
	}
	return nil
}
func (r *stubTagRepo) DetachFromPost(postID, tagID int64) error {
	if r.detachFromPostFunc != nil {
		return r.detachFromPostFunc(postID, tagID)
	}
	return nil
}

// --- Tests ---

func TestPostService_GetAllPosts(t *testing.T) {
	t.Run("投稿一覧と件数を返す", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findAllFunc: func(limit, offset int) ([]model.Post, error) {
				return []model.Post{{ID: 1}, {ID: 2}}, nil
			},
			countFunc: func() (int, error) { return 10, nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		posts, total, err := svc.GetAllPosts(20, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(posts) != 2 {
			t.Errorf("got %d posts, want 2", len(posts))
		}
		if total != 10 {
			t.Errorf("got total=%d, want 10", total)
		}
	})

	t.Run("page < 1 は 1 に補正される", func(t *testing.T) {
		var capturedOffset int
		postRepo := &stubPostRepo{
			findAllFunc: func(limit, offset int) ([]model.Post, error) {
				capturedOffset = offset
				return nil, nil
			},
			countFunc: func() (int, error) { return 0, nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, _, err := svc.GetAllPosts(20, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedOffset != 0 {
			t.Errorf("expected offset=0, got %d", capturedOffset)
		}
	})

	t.Run("FindAll エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		postRepo := &stubPostRepo{
			findAllFunc: func(_, _ int) ([]model.Post, error) { return nil, repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, _, err := svc.GetAllPosts(20, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_GetPostsByUser(t *testing.T) {
	t.Run("ユーザーの投稿一覧を返す", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByUserFunc: func(userID int64, limit, offset int) ([]model.Post, error) {
				return []model.Post{{ID: 10, UserID: userID}}, nil
			},
			countByUserFunc: func(userID int64) (int, error) { return 5, nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		posts, total, err := svc.GetPostsByUser(42, 20, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(posts) != 1 || posts[0].UserID != 42 {
			t.Errorf("unexpected posts: %v", posts)
		}
		if total != 5 {
			t.Errorf("got total=%d, want 5", total)
		}
	})

	t.Run("page < 1 は 1 に補正される", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByUserFunc:  func(userID int64, limit, offset int) ([]model.Post, error) { return nil, nil },
			countByUserFunc: func(userID int64) (int, error) { return 0, nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, _, err := svc.GetPostsByUser(1, 20, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("FindByUser エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		postRepo := &stubPostRepo{
			findByUserFunc: func(userID int64, limit, offset int) ([]model.Post, error) {
				return nil, repoErr
			},
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, _, err := svc.GetPostsByUser(1, 20, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})

	t.Run("CountByUser エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("count error")
		postRepo := &stubPostRepo{
			findByUserFunc:  func(userID int64, limit, offset int) ([]model.Post, error) { return nil, nil },
			countByUserFunc: func(userID int64) (int, error) { return 0, repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, _, err := svc.GetPostsByUser(1, 20, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_CreatePost(t *testing.T) {
	t.Run("新規番組を Upsert して投稿を作成", func(t *testing.T) {
		programRepo := &stubProgramRepo{
			findByStationAndTitleFunc: func(_, _ string) (*model.RadioProgram, error) {
				return nil, errors.New("not found")
			},
			upsertFunc: func(p *model.RadioProgram) (int64, error) { return 99, nil },
		}
		postRepo := &stubPostRepo{
			createFunc: func(post *model.Post) (int64, error) { return 7, nil },
		}
		svc := NewPostService(postRepo, programRepo, &stubTagRepo{})

		data := map[string]interface{}{
			"station_id":    "TBS",
			"program_title": "jazz show",
			"title":         "good show",
			"body":          "loved it",
			"rating":        float64(4.5),
		}
		post, err := svc.CreatePost(data, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if post.ID != 7 {
			t.Errorf("got ID=%d, want 7", post.ID)
		}
		if post.ProgramID != 99 {
			t.Errorf("got ProgramID=%d, want 99", post.ProgramID)
		}
		if post.Rating != 4.5 {
			t.Errorf("got rating=%.1f, want 4.5", post.Rating)
		}
	})

	t.Run("既存番組が見つかった場合はそのIDを使用", func(t *testing.T) {
		programRepo := &stubProgramRepo{
			findByStationAndTitleFunc: func(_, _ string) (*model.RadioProgram, error) {
				return &model.RadioProgram{ID: 55}, nil
			},
		}
		postRepo := &stubPostRepo{
			createFunc: func(post *model.Post) (int64, error) { return 1, nil },
		}
		svc := NewPostService(postRepo, programRepo, &stubTagRepo{})
		data := map[string]interface{}{
			"station_id": "QRR", "program_title": "news",
			"title": "t", "body": "b", "rating": float64(3.0),
		}
		post, err := svc.CreatePost(data, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if post.ProgramID != 55 {
			t.Errorf("got ProgramID=%d, want 55", post.ProgramID)
		}
	})

	t.Run("tag_ids[]が付与されて AttachToPost が呼ばれる", func(t *testing.T) {
		var attached []int64
		postRepo := &stubPostRepo{
			createFunc: func(post *model.Post) (int64, error) { return 1, nil },
		}
		tagRepo := &stubTagRepo{
			attachToPostFunc: func(postID, tagID int64) error {
				attached = append(attached, tagID)
				return nil
			},
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, tagRepo)
		data := map[string]interface{}{
			"station_id": "TBS", "program_title": "p",
			"title": "t", "body": "b",
			"tag_ids": []interface{}{float64(1), float64(2)},
		}
		_, err := svc.CreatePost(data, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(attached) != 2 {
			t.Errorf("expected 2 tags attached, got %d", len(attached))
		}
	})
}

func TestPostService_CreatePost_UpsertError(t *testing.T) {
	repoErr := errors.New("upsert error")
	programRepo := &stubProgramRepo{
		findByStationAndTitleFunc: func(_, _ string) (*model.RadioProgram, error) {
			return nil, errors.New("not found")
		},
		upsertFunc: func(p *model.RadioProgram) (int64, error) { return 0, repoErr },
	}
	svc := NewPostService(&stubPostRepo{}, programRepo, &stubTagRepo{})
	data := map[string]interface{}{"station_id": "TBS", "program_title": "jazz", "title": "t", "body": "b"}
	_, err := svc.CreatePost(data, 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repoErr, got %v", err)
	}
}

func TestPostService_CreatePost_CreateError(t *testing.T) {
	repoErr := errors.New("create error")
	postRepo := &stubPostRepo{
		createFunc: func(post *model.Post) (int64, error) { return 0, repoErr },
	}
	svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
	data := map[string]interface{}{"station_id": "TBS", "program_title": "jazz", "title": "t", "body": "b"}
	_, err := svc.CreatePost(data, 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repoErr, got %v", err)
	}
}

func TestPostService_CreatePost_RatingTypeVariants(t *testing.T) {
	for _, rating := range []interface{}{float32(4.0), int(4), int64(4)} {
		postRepo := &stubPostRepo{
			createFunc: func(post *model.Post) (int64, error) { return 1, nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		data := map[string]interface{}{
			"station_id": "TBS", "program_title": "jazz", "title": "t", "body": "b",
			"rating": rating,
		}
		post, err := svc.CreatePost(data, 1)
		if err != nil {
			t.Errorf("rating %T: unexpected error: %v", rating, err)
		}
		if post.Rating != 4.0 {
			t.Errorf("rating %T: got %.1f, want 4.0", rating, post.Rating)
		}
	}
}

func TestPostService_CreatePost_TagIDsInt64Slice(t *testing.T) {
	var attached []int64
	postRepo := &stubPostRepo{
		createFunc: func(post *model.Post) (int64, error) { return 1, nil },
	}
	tagRepo := &stubTagRepo{
		attachToPostFunc: func(postID, tagID int64) error {
			attached = append(attached, tagID)
			return nil
		},
	}
	svc := NewPostService(postRepo, &stubProgramRepo{}, tagRepo)
	data := map[string]interface{}{
		"station_id": "TBS", "program_title": "p", "title": "t", "body": "b",
		"tag_ids": []int64{5, 6, 7},
	}
	_, err := svc.CreatePost(data, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attached) != 3 {
		t.Errorf("expected 3 attached, got %d", len(attached))
	}
}

func TestPostService_UpdatePost(t *testing.T) {
	t.Run("正常更新", func(t *testing.T) {
		var updated *model.Post
		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) {
				return &model.Post{ID: id, UserID: 1, Title: "old"}, nil
			},
			updateFunc: func(post *model.Post) error {
				updated = post
				return nil
			},
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		data := map[string]interface{}{
			"user_id": int64(1),
			"title":   "new title",
			"body":    "new body",
			"rating":  float64(5.0),
		}
		if err := svc.UpdatePost(1, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Title != "new title" {
			t.Errorf("got title=%q, want 'new title'", updated.Title)
		}
	})

	t.Run("他ユーザーの投稿: unauthorized エラー", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) {
				return &model.Post{ID: id, UserID: 99}, nil
			},
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		err := svc.UpdatePost(1, map[string]interface{}{"user_id": int64(1)})
		if err == nil {
			t.Fatal("expected error for unauthorized update")
		}
	})

	t.Run("FindByID エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("not found")
		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) { return nil, repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		if err := svc.UpdatePost(1, map[string]interface{}{"user_id": int64(1)}); !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})

	t.Run("Update エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("update error")
		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) {
				return &model.Post{ID: id, UserID: 1}, nil
			},
			updateFunc: func(post *model.Post) error { return repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		if err := svc.UpdatePost(1, map[string]interface{}{"user_id": int64(1)}); !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_DeletePost(t *testing.T) {
	t.Run("正常削除", func(t *testing.T) {
		var deletedID int64
		postRepo := &stubPostRepo{
			deleteFunc: func(id int64) error {
				deletedID = id
				return nil
			},
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		if err := svc.DeletePost(5); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deletedID != 5 {
			t.Errorf("got deletedID=%d, want 5", deletedID)
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("delete error")
		postRepo := &stubPostRepo{
			deleteFunc: func(_ int64) error { return repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		if err := svc.DeletePost(1); !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_GetAllTags(t *testing.T) {
	t.Run("全タグを返す", func(t *testing.T) {
		tagRepo := &stubTagRepo{
			findAllFunc: func() ([]model.PostTag, error) {
				return []model.PostTag{{ID: 1, Name: "音楽"}, {ID: 2, Name: "トーク"}}, nil
			},
		}
		svc := NewPostService(&stubPostRepo{}, &stubProgramRepo{}, tagRepo)
		tags, err := svc.GetAllTags()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tags) != 2 {
			t.Errorf("got %d tags, want 2", len(tags))
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		tagRepo := &stubTagRepo{
			findAllFunc: func() ([]model.PostTag, error) { return nil, repoErr },
		}
		svc := NewPostService(&stubPostRepo{}, &stubProgramRepo{}, tagRepo)
		_, err := svc.GetAllTags()
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_GetPostsByProgram(t *testing.T) {
	t.Run("番組別投稿一覧と件数を返す", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByProgramFunc: func(stationID, programTitle string, limit, offset int) ([]model.Post, error) {
				return []model.Post{{ID: 1}, {ID: 2}}, nil
			},
			countByProgramFunc: func(stationID, programTitle string) (int, error) { return 7, nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		posts, total, err := svc.GetPostsByProgram("TBS", "jazz show", 20, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(posts) != 2 {
			t.Errorf("got %d posts, want 2", len(posts))
		}
		if total != 7 {
			t.Errorf("got total=%d, want 7", total)
		}
	})

	t.Run("FindByProgram エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		postRepo := &stubPostRepo{
			findByProgramFunc: func(_, _ string, _, _ int) ([]model.Post, error) { return nil, repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, _, err := svc.GetPostsByProgram("TBS", "jazz show", 20, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})

	t.Run("CountByProgram エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("count error")
		postRepo := &stubPostRepo{
			findByProgramFunc:  func(_, _ string, _, _ int) ([]model.Post, error) { return nil, nil },
			countByProgramFunc: func(_, _ string) (int, error) { return 0, repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, _, err := svc.GetPostsByProgram("TBS", "jazz show", 20, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_GetPostsFiltered(t *testing.T) {
	t.Run("フィルタ付き投稿一覧を返す", func(t *testing.T) {
		filters := map[string]interface{}{"station_id": "TBS", "min_rating": 4.0}
		postRepo := &stubPostRepo{
			findFilteredFunc: func(f map[string]interface{}, limit, offset int) ([]model.Post, error) {
				return []model.Post{{ID: 1}, {ID: 2}, {ID: 3}}, nil
			},
			countFilteredFunc: func(f map[string]interface{}) (int, error) { return 15, nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		posts, total, err := svc.GetPostsFiltered(filters, 10, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(posts) != 3 {
			t.Errorf("got %d posts, want 3", len(posts))
		}
		if total != 15 {
			t.Errorf("got total=%d, want 15", total)
		}
	})

	t.Run("FindFiltered エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("filter error")
		postRepo := &stubPostRepo{
			findFilteredFunc: func(_ map[string]interface{}, _, _ int) ([]model.Post, error) { return nil, repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, _, err := svc.GetPostsFiltered(nil, 10, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_GetAverageRatingByProgram(t *testing.T) {
	t.Run("平均評価を返す", func(t *testing.T) {
		postRepo := &stubPostRepo{
			avgRatingFunc: func(programID int64) (float64, error) { return 4.2, nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		avg, err := svc.GetAverageRatingByProgram(1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if avg != 4.2 {
			t.Errorf("got avg=%.1f, want 4.2", avg)
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("avg error")
		postRepo := &stubPostRepo{
			avgRatingFunc: func(_ int64) (float64, error) { return 0, repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, err := svc.GetAverageRatingByProgram(1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_GetPostByID(t *testing.T) {
	t.Run("IDで投稿を返す", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) {
				return &model.Post{ID: id, Title: "my post"}, nil
			},
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		post, err := svc.GetPostByID(5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if post.ID != 5 {
			t.Errorf("got ID=%d, want 5", post.ID)
		}
		if post.Title != "my post" {
			t.Errorf("got Title=%q, want 'my post'", post.Title)
		}
	})

	t.Run("not-found エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("not found")
		postRepo := &stubPostRepo{
			findByIDFunc: func(_ int64) (*model.Post, error) { return nil, repoErr },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		_, err := svc.GetPostByID(99)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestPostService_GetPostsFiltered_CountError(t *testing.T) {
	repoErr := errors.New("count error")
	postRepo := &stubPostRepo{
		findFilteredFunc: func(_ map[string]interface{}, _, _ int) ([]model.Post, error) {
			return []model.Post{{ID: 1}}, nil
		},
		countFilteredFunc: func(_ map[string]interface{}) (int, error) { return 0, repoErr },
	}
	svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
	_, _, err := svc.GetPostsFiltered(nil, 10, 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repoErr, got %v", err)
	}
}

func TestPostService_UpdatePost_UserIDTypeVariants(t *testing.T) {
	// float64 user_id (matches post.UserID=1 when uid=1.0)
	t.Run("float64 user_id: 正常更新", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) {
				return &model.Post{ID: id, UserID: 1}, nil
			},
			updateFunc: func(post *model.Post) error { return nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		data := map[string]interface{}{
			"user_id": float64(1),
			"title":   "updated",
			"rating":  int(4),
		}
		if err := svc.UpdatePost(1, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("int user_id and int64 rating: 正常更新", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) {
				return &model.Post{ID: id, UserID: 1}, nil
			},
			updateFunc: func(post *model.Post) error { return nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		data := map[string]interface{}{
			"user_id": int(1),
			"title":   "updated",
			"rating":  int64(4),
		}
		if err := svc.UpdatePost(1, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("float32 rating: 正常更新", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) {
				return &model.Post{ID: id, UserID: 1}, nil
			},
			updateFunc: func(post *model.Post) error { return nil },
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, &stubTagRepo{})
		data := map[string]interface{}{
			"user_id": int64(1),
			"rating":  float32(3.5),
		}
		if err := svc.UpdatePost(1, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestPostService_UpdatePost_TagReplace(t *testing.T) {
	t.Run("tag_ids が渡された場合: 既存タグ削除後に新規タグを付与", func(t *testing.T) {
		var detachedTags []int64
		var attachedTags []int64

		postRepo := &stubPostRepo{
			findByIDFunc: func(id int64) (*model.Post, error) {
				return &model.Post{ID: id, UserID: 1}, nil
			},
			updateFunc: func(post *model.Post) error { return nil },
		}
		tagRepo := &stubTagRepo{
			findByPostIDFunc: func(postID int64) ([]model.PostTag, error) {
				return []model.PostTag{{ID: 10}, {ID: 20}}, nil
			},
			detachFromPostFunc: func(postID, tagID int64) error {
				detachedTags = append(detachedTags, tagID)
				return nil
			},
			attachToPostFunc: func(postID, tagID int64) error {
				attachedTags = append(attachedTags, tagID)
				return nil
			},
		}
		svc := NewPostService(postRepo, &stubProgramRepo{}, tagRepo)
		data := map[string]interface{}{
			"user_id": int64(1),
			"title":   "updated title",
			"tag_ids": []interface{}{float64(30), float64(40), float64(50)},
		}
		if err := svc.UpdatePost(1, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 既存の2タグが detach されたか確認
		if len(detachedTags) != 2 {
			t.Errorf("expected 2 detached tags, got %d", len(detachedTags))
		}
		// 新規3タグが attach されたか確認
		if len(attachedTags) != 3 {
			t.Errorf("expected 3 attached tags, got %d", len(attachedTags))
		}
	})
}
