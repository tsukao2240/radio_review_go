package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type PostLikeRepository struct {
	db *sqlx.DB
}

func NewPostLikeRepository(db *sqlx.DB) *PostLikeRepository {
	return &PostLikeRepository{db: db}
}

func (r *PostLikeRepository) Exists(postID, userID int64) (bool, error) {
	ctx := context.Background()
	var count int
	err := r.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND user_id = ?",
		postID, userID,
	)
	if err != nil {
		return false, fmt.Errorf("repository.PostLikeRepository.Exists: %w", err)
	}
	return count > 0, nil
}

func (r *PostLikeRepository) Create(postID, userID int64) error {
	ctx := context.Background()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO post_likes (post_id, user_id, created_at, updated_at)
		 VALUES (?, ?, NOW(), NOW())`,
		postID, userID,
	)
	if err != nil {
		return fmt.Errorf("repository.PostLikeRepository.Create: %w", err)
	}

	_, err = r.db.ExecContext(ctx,
		"UPDATE posts SET likes_count = likes_count + 1, updated_at = NOW() WHERE id = ?",
		postID,
	)
	if err != nil {
		return fmt.Errorf("repository.PostLikeRepository.Create increment likes_count: %w", err)
	}
	return nil
}

func (r *PostLikeRepository) Delete(postID, userID int64) error {
	ctx := context.Background()
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM post_likes WHERE post_id = ? AND user_id = ?",
		postID, userID,
	)
	if err != nil {
		return fmt.Errorf("repository.PostLikeRepository.Delete: %w", err)
	}

	_, err = r.db.ExecContext(ctx,
		"UPDATE posts SET likes_count = GREATEST(likes_count - 1, 0), updated_at = NOW() WHERE id = ?",
		postID,
	)
	if err != nil {
		return fmt.Errorf("repository.PostLikeRepository.Delete decrement likes_count: %w", err)
	}
	return nil
}

func (r *PostLikeRepository) CountByPost(postID int64) (int, error) {
	ctx := context.Background()
	var count int
	err := r.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM post_likes WHERE post_id = ?",
		postID,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.PostLikeRepository.CountByPost: %w", err)
	}
	return count, nil
}
