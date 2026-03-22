package repository

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/yourname/radio_review_go/internal/model"
)

type PostCommentRepository struct {
	db *sqlx.DB
}

func NewPostCommentRepository(db *sqlx.DB) *PostCommentRepository {
	return &PostCommentRepository{db: db}
}

func (r *PostCommentRepository) FindByPost(postID int64) ([]model.PostComment, error) {
	var comments []model.PostComment
	err := r.db.Select(&comments,
		"SELECT * FROM post_comments WHERE post_id = ? ORDER BY created_at ASC",
		postID,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.PostCommentRepository.FindByPost: %w", err)
	}
	return comments, nil
}

func (r *PostCommentRepository) FindByID(id int64) (*model.PostComment, error) {
	var c model.PostComment
	err := r.db.Get(&c, "SELECT * FROM post_comments WHERE id = ? LIMIT 1", id)
	if err != nil {
		return nil, fmt.Errorf("repository.PostCommentRepository.FindByID: %w", err)
	}
	return &c, nil
}

func (r *PostCommentRepository) Create(comment *model.PostComment) (int64, error) {
	res, err := r.db.NamedExec(
		`INSERT INTO post_comments (post_id, user_id, body, created_at, updated_at)
		 VALUES (:post_id, :user_id, :body, NOW(), NOW())`,
		comment,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.PostCommentRepository.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.PostCommentRepository.Create LastInsertId: %w", err)
	}

	_, err = r.db.Exec(
		"UPDATE posts SET comments_count = comments_count + 1, updated_at = NOW() WHERE id = ?",
		comment.PostID,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.PostCommentRepository.Create increment comments_count: %w", err)
	}
	return id, nil
}

func (r *PostCommentRepository) Delete(id int64) error {
	// Fetch the post_id before deleting so we can decrement the counter.
	var postID int64
	err := r.db.Get(&postID, "SELECT post_id FROM post_comments WHERE id = ? LIMIT 1", id)
	if err != nil {
		return fmt.Errorf("repository.PostCommentRepository.Delete fetch post_id: %w", err)
	}

	_, err = r.db.Exec("DELETE FROM post_comments WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("repository.PostCommentRepository.Delete: %w", err)
	}

	_, err = r.db.Exec(
		"UPDATE posts SET comments_count = GREATEST(comments_count - 1, 0), updated_at = NOW() WHERE id = ?",
		postID,
	)
	if err != nil {
		return fmt.Errorf("repository.PostCommentRepository.Delete decrement comments_count: %w", err)
	}
	return nil
}
