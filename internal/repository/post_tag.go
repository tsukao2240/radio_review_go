package repository

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/yourname/radio_review_go/internal/model"
)

type PostTagRepository struct {
	db *sqlx.DB
}

func NewPostTagRepository(db *sqlx.DB) *PostTagRepository {
	return &PostTagRepository{db: db}
}

func (r *PostTagRepository) FindAll() ([]model.PostTag, error) {
	var tags []model.PostTag
	err := r.db.Select(&tags, "SELECT * FROM post_tags ORDER BY display_order ASC, id ASC")
	if err != nil {
		return nil, fmt.Errorf("repository.PostTagRepository.FindAll: %w", err)
	}
	return tags, nil
}

func (r *PostTagRepository) FindByID(id int64) (*model.PostTag, error) {
	var tag model.PostTag
	err := r.db.Get(&tag, "SELECT * FROM post_tags WHERE id = ? LIMIT 1", id)
	if err != nil {
		return nil, fmt.Errorf("repository.PostTagRepository.FindByID: %w", err)
	}
	return &tag, nil
}

func (r *PostTagRepository) FindByPostID(postID int64) ([]model.PostTag, error) {
	var tags []model.PostTag
	err := r.db.Select(&tags,
		`SELECT pt.* FROM post_tags pt
		 INNER JOIN post_post_tag ppt ON ppt.post_tag_id = pt.id
		 WHERE ppt.post_id = ?
		 ORDER BY pt.display_order ASC, pt.id ASC`,
		postID,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.PostTagRepository.FindByPostID: %w", err)
	}
	return tags, nil
}

func (r *PostTagRepository) AttachToPost(postID, tagID int64) error {
	_, err := r.db.Exec(
		`INSERT IGNORE INTO post_post_tag (post_id, post_tag_id, created_at, updated_at)
		 VALUES (?, ?, NOW(), NOW())`,
		postID, tagID,
	)
	if err != nil {
		return fmt.Errorf("repository.PostTagRepository.AttachToPost: %w", err)
	}
	return nil
}

func (r *PostTagRepository) DetachFromPost(postID, tagID int64) error {
	_, err := r.db.Exec(
		"DELETE FROM post_post_tag WHERE post_id = ? AND post_tag_id = ?",
		postID, tagID,
	)
	if err != nil {
		return fmt.Errorf("repository.PostTagRepository.DetachFromPost: %w", err)
	}
	return nil
}
