package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

type PostRepository struct {
	db *sqlx.DB
}

func NewPostRepository(db *sqlx.DB) *PostRepository {
	return &PostRepository{db: db}
}

func (r *PostRepository) FindAll(limit, offset int) ([]model.Post, error) {
	return r.FindAllContext(context.Background(), limit, offset)
}

func (r *PostRepository) FindAllContext(ctx context.Context, limit, offset int) ([]model.Post, error) {
	var posts []model.Post
	err := r.db.SelectContext(ctx, &posts,
		"SELECT * FROM posts ORDER BY created_at DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.PostRepository.FindAll: %w", err)
	}
	return posts, nil
}

func (r *PostRepository) Count() (int, error) {
	return r.CountContext(context.Background())
}

func (r *PostRepository) CountContext(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM posts")
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.Count: %w", err)
	}
	return count, nil
}

func (r *PostRepository) FindByProgram(stationID, programTitle string, limit, offset int) ([]model.Post, error) {
	return r.FindByProgramContext(context.Background(), stationID, programTitle, limit, offset)
}

func (r *PostRepository) FindByProgramContext(ctx context.Context, stationID, programTitle string, limit, offset int) ([]model.Post, error) {
	var posts []model.Post
	err := r.db.SelectContext(ctx, &posts,
		"SELECT * FROM posts WHERE station_id = ? AND program_title = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
		stationID, programTitle, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.PostRepository.FindByProgram: %w", err)
	}
	return posts, nil
}

func (r *PostRepository) CountByProgram(stationID, programTitle string) (int, error) {
	return r.CountByProgramContext(context.Background(), stationID, programTitle)
}

func (r *PostRepository) CountByProgramContext(ctx context.Context, stationID, programTitle string) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM posts WHERE station_id = ? AND program_title = ?",
		stationID, programTitle,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.CountByProgram: %w", err)
	}
	return count, nil
}

func (r *PostRepository) FindByUser(userID int64, limit, offset int) ([]model.Post, error) {
	return r.FindByUserContext(context.Background(), userID, limit, offset)
}

func (r *PostRepository) FindByUserContext(ctx context.Context, userID int64, limit, offset int) ([]model.Post, error) {
	var posts []model.Post
	err := r.db.SelectContext(ctx, &posts,
		"SELECT * FROM posts WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.PostRepository.FindByUser: %w", err)
	}
	return posts, nil
}

func (r *PostRepository) CountByUser(userID int64) (int, error) {
	return r.CountByUserContext(context.Background(), userID)
}

func (r *PostRepository) CountByUserContext(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM posts WHERE user_id = ?",
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.CountByUser: %w", err)
	}
	return count, nil
}

func (r *PostRepository) FindByID(id int64) (*model.Post, error) {
	return r.FindByIDContext(context.Background(), id)
}

func (r *PostRepository) FindByIDContext(ctx context.Context, id int64) (*model.Post, error) {
	var post model.Post
	err := r.db.GetContext(ctx, &post, "SELECT * FROM posts WHERE id = ? LIMIT 1", id)
	if err != nil {
		return nil, fmt.Errorf("repository.PostRepository.FindByID: %w", err)
	}
	return &post, nil
}

// FindFiltered applies dynamic filter conditions.
// Supported filter keys: stationID (string), tagID (int64), minRating (float64), keyword (string).
func (r *PostRepository) FindFiltered(filters map[string]interface{}, limit, offset int) ([]model.Post, error) {
	return r.FindFilteredContext(context.Background(), filters, limit, offset)
}

func (r *PostRepository) FindFilteredContext(ctx context.Context, filters map[string]interface{}, limit, offset int) ([]model.Post, error) {
	query, args := buildPostFilterQuery("SELECT p.*", filters)
	query += " ORDER BY p.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var posts []model.Post
	err := r.db.SelectContext(ctx, &posts, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repository.PostRepository.FindFiltered: %w", err)
	}
	return posts, nil
}

func (r *PostRepository) CountFiltered(filters map[string]interface{}) (int, error) {
	return r.CountFilteredContext(context.Background(), filters)
}

func (r *PostRepository) CountFilteredContext(ctx context.Context, filters map[string]interface{}) (int, error) {
	query, args := buildPostFilterQuery("SELECT COUNT(*)", filters)

	var count int
	err := r.db.GetContext(ctx, &count, query, args...)
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.CountFiltered: %w", err)
	}
	return count, nil
}

// buildPostFilterQuery constructs a SELECT query with optional JOIN and WHERE clauses.
func buildPostFilterQuery(selectClause string, filters map[string]interface{}) (string, []interface{}) {
	var joins []string
	var wheres []string
	var args []interface{}

	from := " FROM posts p"

	if tagID, ok := filters["tagID"]; ok {
		joins = append(joins, "INNER JOIN post_post_tag ppt ON ppt.post_id = p.id AND ppt.post_tag_id = ?")
		args = append(args, tagID)
	}

	if stationID, ok := filters["stationID"]; ok {
		wheres = append(wheres, "p.station_id = ?")
		args = append(args, stationID)
	}

	if minRating, ok := filters["minRating"]; ok {
		wheres = append(wheres, "p.rating >= ?")
		args = append(args, minRating)
	}

	if keyword, ok := filters["keyword"]; ok {
		wheres = append(wheres, "(p.title LIKE ? OR p.body LIKE ? OR p.program_title LIKE ?)")
		kw := "%" + keyword.(string) + "%"
		args = append(args, kw, kw, kw)
	}

	query := selectClause + from
	if len(joins) > 0 {
		query += " " + strings.Join(joins, " ")
	}
	if len(wheres) > 0 {
		query += " WHERE " + strings.Join(wheres, " AND ")
	}

	return query, args
}

func (r *PostRepository) Create(post *model.Post) (int64, error) {
	return r.CreateContext(context.Background(), post)
}

func (r *PostRepository) CreateContext(ctx context.Context, post *model.Post) (int64, error) {
	res, err := r.db.NamedExecContext(ctx,
		`INSERT INTO posts (user_id, program_id, program_title, station_id, title, body, rating,
		 likes_count, comments_count, created_at, updated_at)
		 VALUES (:user_id, :program_id, :program_title, :station_id, :title, :body, :rating,
		 :likes_count, :comments_count, NOW(), NOW())`,
		post,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.Create LastInsertId: %w", err)
	}
	return id, nil
}

func (r *PostRepository) Update(post *model.Post) error {
	return r.UpdateContext(context.Background(), post)
}

func (r *PostRepository) UpdateContext(ctx context.Context, post *model.Post) error {
	_, err := r.db.NamedExecContext(ctx,
		`UPDATE posts SET program_id=:program_id, program_title=:program_title,
		 station_id=:station_id, title=:title, body=:body, rating=:rating, updated_at=NOW()
		 WHERE id=:id`,
		post,
	)
	if err != nil {
		return fmt.Errorf("repository.PostRepository.Update: %w", err)
	}
	return nil
}

func (r *PostRepository) Delete(id int64) error {
	_, err := r.db.ExecContext(context.Background(), "DELETE FROM posts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("repository.PostRepository.Delete: %w", err)
	}
	return nil
}

func (r *PostRepository) AverageRating(programID int64) (float64, error) {
	return r.AverageRatingContext(context.Background(), programID)
}

func (r *PostRepository) AverageRatingContext(ctx context.Context, programID int64) (float64, error) {
	var avg float64
	err := r.db.GetContext(ctx, &avg,
		"SELECT COALESCE(AVG(rating), 0) FROM posts WHERE program_id = ?",
		programID,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.AverageRating: %w", err)
	}
	return avg, nil
}

func (r *PostRepository) RunInTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("repository.PostRepository.BeginTxx: %w", err)
	}
	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("%w; rollback: %v", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repository.PostRepository.Commit: %w", err)
	}
	return nil
}

func (r *PostRepository) FindByIDTx(ctx context.Context, tx *sqlx.Tx, id int64) (*model.Post, error) {
	var post model.Post
	err := tx.GetContext(ctx, &post, "SELECT * FROM posts WHERE id = ? LIMIT 1", id)
	if err != nil {
		return nil, fmt.Errorf("repository.PostRepository.FindByIDTx: %w", err)
	}
	return &post, nil
}

func (r *PostRepository) CreateTx(ctx context.Context, tx *sqlx.Tx, post *model.Post) (int64, error) {
	res, err := tx.NamedExecContext(ctx,
		`INSERT INTO posts (user_id, program_id, program_title, station_id, title, body, rating,
		 likes_count, comments_count, created_at, updated_at)
		 VALUES (:user_id, :program_id, :program_title, :station_id, :title, :body, :rating,
		 :likes_count, :comments_count, NOW(), NOW())`,
		post,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.CreateTx: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.PostRepository.CreateTx LastInsertId: %w", err)
	}
	return id, nil
}

func (r *PostRepository) UpdateTx(ctx context.Context, tx *sqlx.Tx, post *model.Post) error {
	_, err := tx.NamedExecContext(ctx,
		`UPDATE posts SET program_id=:program_id, program_title=:program_title,
		 station_id=:station_id, title=:title, body=:body, rating=:rating, updated_at=NOW()
		 WHERE id=:id`,
		post,
	)
	if err != nil {
		return fmt.Errorf("repository.PostRepository.UpdateTx: %w", err)
	}
	return nil
}
