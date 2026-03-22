package service

import (
	"errors"
	"testing"

	"github.com/yourname/radio_review_go/internal/model"
)

// --- PostLikeRepositoryInterface スタブ ---

type stubLikeRepo struct {
	existsFunc func(postID, userID int64) (bool, error)
	createFunc func(postID, userID int64) error
	deleteFunc func(postID, userID int64) error
	countFunc  func(postID int64) (int, error)
}

func (r *stubLikeRepo) Exists(postID, userID int64) (bool, error) {
	if r.existsFunc != nil {
		return r.existsFunc(postID, userID)
	}
	return false, nil
}

func (r *stubLikeRepo) Create(postID, userID int64) error {
	if r.createFunc != nil {
		return r.createFunc(postID, userID)
	}
	return nil
}

func (r *stubLikeRepo) Delete(postID, userID int64) error {
	if r.deleteFunc != nil {
		return r.deleteFunc(postID, userID)
	}
	return nil
}

func (r *stubLikeRepo) CountByPost(postID int64) (int, error) {
	if r.countFunc != nil {
		return r.countFunc(postID)
	}
	return 0, nil
}

// --- PostCommentRepositoryInterface スタブ ---

type stubCommentRepo struct {
	findByPostFunc func(postID int64) ([]model.PostComment, error)
	findByIDFunc   func(id int64) (*model.PostComment, error)
	createFunc     func(comment *model.PostComment) (int64, error)
	deleteFunc     func(id int64) error
}

func (r *stubCommentRepo) FindByPost(postID int64) ([]model.PostComment, error) {
	if r.findByPostFunc != nil {
		return r.findByPostFunc(postID)
	}
	return nil, nil
}

func (r *stubCommentRepo) FindByID(id int64) (*model.PostComment, error) {
	if r.findByIDFunc != nil {
		return r.findByIDFunc(id)
	}
	return nil, nil
}

func (r *stubCommentRepo) Create(comment *model.PostComment) (int64, error) {
	if r.createFunc != nil {
		return r.createFunc(comment)
	}
	return 1, nil
}

func (r *stubCommentRepo) Delete(id int64) error {
	if r.deleteFunc != nil {
		return r.deleteFunc(id)
	}
	return nil
}

// --- テスト ---

func TestPostInteractionService_Like(t *testing.T) {
	t.Run("not yet liked: succeeds", func(t *testing.T) {
		likeRepo := &stubLikeRepo{
			existsFunc: func(postID, userID int64) (bool, error) {
				return false, nil
			},
		}
		svc := NewPostInteractionService(likeRepo, &stubCommentRepo{})

		if err := svc.Like(1, 10); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("already liked: returns error", func(t *testing.T) {
		likeRepo := &stubLikeRepo{
			existsFunc: func(postID, userID int64) (bool, error) {
				return true, nil
			},
		}
		svc := NewPostInteractionService(likeRepo, &stubCommentRepo{})

		err := svc.Like(1, 10)
		if err == nil {
			t.Error("expected error for duplicate like, got nil")
		}
		if err.Error() != "already liked" {
			t.Errorf("unexpected error message: %q", err.Error())
		}
	})

	t.Run("repository error is propagated", func(t *testing.T) {
		repoErr := errors.New("db connection error")
		likeRepo := &stubLikeRepo{
			existsFunc: func(postID, userID int64) (bool, error) {
				return false, repoErr
			},
		}
		svc := NewPostInteractionService(likeRepo, &stubCommentRepo{})

		err := svc.Like(1, 10)
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected wrapped repoErr, got %v", err)
		}
	})
}

func TestPostInteractionService_Unlike(t *testing.T) {
	t.Run("liked: succeeds", func(t *testing.T) {
		var deletedPostID, deletedUserID int64
		likeRepo := &stubLikeRepo{
			deleteFunc: func(postID, userID int64) error {
				deletedPostID = postID
				deletedUserID = userID
				return nil
			},
		}
		svc := NewPostInteractionService(likeRepo, &stubCommentRepo{})

		if err := svc.Unlike(5, 20); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if deletedPostID != 5 || deletedUserID != 20 {
			t.Errorf("Delete called with (%d, %d), want (5, 20)", deletedPostID, deletedUserID)
		}
	})

	t.Run("repository error is propagated", func(t *testing.T) {
		repoErr := errors.New("delete failed")
		likeRepo := &stubLikeRepo{
			deleteFunc: func(postID, userID int64) error {
				return repoErr
			},
		}
		svc := NewPostInteractionService(likeRepo, &stubCommentRepo{})

		err := svc.Unlike(1, 10)
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected wrapped repoErr, got %v", err)
		}
	})
}

func TestPostInteractionService_AddComment(t *testing.T) {
	t.Run("valid body: returns comment", func(t *testing.T) {
		commentRepo := &stubCommentRepo{
			createFunc: func(comment *model.PostComment) (int64, error) {
				return 42, nil
			},
		}
		svc := NewPostInteractionService(&stubLikeRepo{}, commentRepo)

		comment, err := svc.AddComment(3, 7, "Great show!")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if comment == nil {
			t.Fatal("expected comment, got nil")
		}
		if comment.ID != 42 {
			t.Errorf("comment.ID = %d, want 42", comment.ID)
		}
		if comment.PostID != 3 {
			t.Errorf("comment.PostID = %d, want 3", comment.PostID)
		}
		if comment.UserID != 7 {
			t.Errorf("comment.UserID = %d, want 7", comment.UserID)
		}
		if comment.Body != "Great show!" {
			t.Errorf("comment.Body = %q, want %q", comment.Body, "Great show!")
		}
	})

	t.Run("empty body: returns error", func(t *testing.T) {
		svc := NewPostInteractionService(&stubLikeRepo{}, &stubCommentRepo{})

		_, err := svc.AddComment(1, 1, "")
		if err == nil {
			t.Error("expected error for empty body, got nil")
		}
		if err.Error() != "comment body is required" {
			t.Errorf("unexpected error message: %q", err.Error())
		}
	})

	t.Run("repository error is propagated", func(t *testing.T) {
		repoErr := errors.New("insert failed")
		commentRepo := &stubCommentRepo{
			createFunc: func(comment *model.PostComment) (int64, error) {
				return 0, repoErr
			},
		}
		svc := NewPostInteractionService(&stubLikeRepo{}, commentRepo)

		_, err := svc.AddComment(1, 1, "test comment")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected wrapped repoErr, got %v", err)
		}
	})
}

func TestPostInteractionService_DeleteComment(t *testing.T) {
	t.Run("own comment: succeeds", func(t *testing.T) {
		commentRepo := &stubCommentRepo{
			findByIDFunc: func(id int64) (*model.PostComment, error) {
				return &model.PostComment{ID: id, UserID: 10}, nil
			},
		}
		svc := NewPostInteractionService(&stubLikeRepo{}, commentRepo)

		if err := svc.DeleteComment(1, 10); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("other user's comment: returns unauthorized error", func(t *testing.T) {
		commentRepo := &stubCommentRepo{
			findByIDFunc: func(id int64) (*model.PostComment, error) {
				// comment owned by userID=99
				return &model.PostComment{ID: id, UserID: 99}, nil
			},
		}
		svc := NewPostInteractionService(&stubLikeRepo{}, commentRepo)

		err := svc.DeleteComment(1, 10) // userID=10 tries to delete
		if err == nil {
			t.Error("expected unauthorized error, got nil")
		}
		if err.Error() != "unauthorized: cannot delete other user's comment" {
			t.Errorf("unexpected error message: %q", err.Error())
		}
	})

	t.Run("comment not found: repository error propagated", func(t *testing.T) {
		repoErr := errors.New("comment not found")
		commentRepo := &stubCommentRepo{
			findByIDFunc: func(id int64) (*model.PostComment, error) {
				return nil, repoErr
			},
		}
		svc := NewPostInteractionService(&stubLikeRepo{}, commentRepo)

		err := svc.DeleteComment(999, 10)
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected wrapped repoErr, got %v", err)
		}
	})
}
