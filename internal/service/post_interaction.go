package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/repository"
)

const maxCommentBodyLength = 1000

type contextPostCommentRepository interface {
	CreateContext(context.Context, *model.PostComment) (int64, error)
}

// PostInteractionService は PostInteractionServiceInterface を実装する。
type PostInteractionService struct {
	likeRepo    repository.PostLikeRepositoryInterface
	commentRepo repository.PostCommentRepositoryInterface
}

// NewPostInteractionService は新しい PostInteractionService を返す。
func NewPostInteractionService(
	likeRepo repository.PostLikeRepositoryInterface,
	commentRepo repository.PostCommentRepositoryInterface,
) *PostInteractionService {
	return &PostInteractionService{
		likeRepo:    likeRepo,
		commentRepo: commentRepo,
	}
}

// Like は投稿にいいねを追加する。重複いいねはエラーを返す。
func (s *PostInteractionService) Like(postID, userID int64) error {
	exists, err := s.likeRepo.Exists(postID, userID)
	if err != nil {
		return fmt.Errorf("PostInteractionService.Like exists check: %w", err)
	}
	if exists {
		return errors.New("already liked")
	}

	if err := s.likeRepo.Create(postID, userID); err != nil {
		return fmt.Errorf("PostInteractionService.Like create: %w", err)
	}
	return nil
}

// Unlike は投稿からいいねを削除する。
func (s *PostInteractionService) Unlike(postID, userID int64) error {
	if err := s.likeRepo.Delete(postID, userID); err != nil {
		return fmt.Errorf("PostInteractionService.Unlike: %w", err)
	}
	return nil
}

// AddComment は投稿にコメントを追加する。
func (s *PostInteractionService) AddComment(postID, userID int64, body string) (*model.PostComment, error) {
	return s.AddCommentContext(context.Background(), postID, userID, body)
}

func (s *PostInteractionService) AddCommentContext(ctx context.Context, postID, userID int64, body string) (*model.PostComment, error) {
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("comment body is required")
	}
	if len([]rune(body)) > maxCommentBodyLength {
		return nil, fmt.Errorf("comment body must be %d characters or fewer", maxCommentBodyLength)
	}

	comment := &model.PostComment{
		PostID: postID,
		UserID: userID,
		Body:   body,
	}

	var (
		commentID int64
		err       error
	)
	if repo, ok := s.commentRepo.(contextPostCommentRepository); ok {
		commentID, err = repo.CreateContext(ctx, comment)
	} else {
		commentID, err = s.commentRepo.Create(comment)
	}
	if err != nil {
		return nil, fmt.Errorf("PostInteractionService.AddComment: %w", err)
	}
	comment.ID = commentID

	return comment, nil
}

// DeleteComment はコメントを削除する。投稿者本人のみ削除可能。
func (s *PostInteractionService) DeleteComment(commentID, userID int64) error {
	comment, err := s.commentRepo.FindByID(commentID)
	if err != nil {
		return fmt.Errorf("PostInteractionService.DeleteComment find: %w", err)
	}

	if comment.UserID != userID {
		return errors.New("unauthorized: cannot delete other user's comment")
	}

	if err := s.commentRepo.Delete(commentID); err != nil {
		return fmt.Errorf("PostInteractionService.DeleteComment: %w", err)
	}
	return nil
}

// GetComments は投稿に紐づくコメント一覧を返す。
func (s *PostInteractionService) GetComments(postID int64) ([]model.PostComment, error) {
	comments, err := s.commentRepo.FindByPost(postID)
	if err != nil {
		return nil, fmt.Errorf("PostInteractionService.GetComments: %w", err)
	}
	return comments, nil
}

// IsLikedBy は指定ユーザーがその投稿にいいねしているか確認する。
func (s *PostInteractionService) IsLikedBy(postID, userID int64) (bool, error) {
	exists, err := s.likeRepo.Exists(postID, userID)
	if err != nil {
		return false, fmt.Errorf("PostInteractionService.IsLikedBy: %w", err)
	}
	return exists, nil
}
