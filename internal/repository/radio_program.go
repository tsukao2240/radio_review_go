package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

type RadioProgramRepository struct {
	db *sqlx.DB
}

func NewRadioProgramRepository(db *sqlx.DB) *RadioProgramRepository {
	return &RadioProgramRepository{db: db}
}

func (r *RadioProgramRepository) FindByID(id int64) (*model.RadioProgram, error) {
	return r.FindByIDContext(context.Background(), id)
}

func (r *RadioProgramRepository) FindByIDContext(ctx context.Context, id int64) (*model.RadioProgram, error) {
	var p model.RadioProgram
	err := r.db.GetContext(ctx, &p, "SELECT * FROM radio_programs WHERE id = ? LIMIT 1", id)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.FindByID: %w", err)
	}
	return &p, nil
}

func (r *RadioProgramRepository) FindByStationAndTitle(stationID, title string) (*model.RadioProgram, error) {
	return r.FindByStationAndTitleContext(context.Background(), stationID, title)
}

func (r *RadioProgramRepository) FindByStationAndTitleContext(ctx context.Context, stationID, title string) (*model.RadioProgram, error) {
	var p model.RadioProgram
	err := r.db.GetContext(ctx, &p,
		"SELECT * FROM radio_programs WHERE station_id = ? AND title = ? LIMIT 1",
		stationID, title,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.FindByStationAndTitle: %w", err)
	}
	return &p, nil
}

func (r *RadioProgramRepository) SearchByTitle(keyword string, limit, offset int) ([]model.RadioProgram, error) {
	return r.SearchByTitleContext(context.Background(), keyword, limit, offset)
}

func (r *RadioProgramRepository) SearchByTitleContext(ctx context.Context, keyword string, limit, offset int) ([]model.RadioProgram, error) {
	var programs []model.RadioProgram
	err := r.db.SelectContext(ctx, &programs,
		"SELECT * FROM radio_programs WHERE title LIKE ? ORDER BY id DESC LIMIT ? OFFSET ?",
		"%"+keyword+"%", limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.SearchByTitle: %w", err)
	}
	return programs, nil
}

func (r *RadioProgramRepository) SearchByCast(cast string, limit, offset int) ([]model.RadioProgram, error) {
	return r.SearchByCastContext(context.Background(), cast, limit, offset)
}

func (r *RadioProgramRepository) SearchByCastContext(ctx context.Context, cast string, limit, offset int) ([]model.RadioProgram, error) {
	var programs []model.RadioProgram
	err := r.db.SelectContext(ctx, &programs,
		"SELECT * FROM radio_programs WHERE cast LIKE ? ORDER BY id DESC LIMIT ? OFFSET ?",
		"%"+cast+"%", limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.SearchByCast: %w", err)
	}
	return programs, nil
}

func (r *RadioProgramRepository) CountByTitle(keyword string) (int, error) {
	return r.CountByTitleContext(context.Background(), keyword)
}

func (r *RadioProgramRepository) CountByTitleContext(ctx context.Context, keyword string) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM radio_programs WHERE title LIKE ?",
		"%"+keyword+"%",
	)
	if err != nil {
		return 0, fmt.Errorf("repository.RadioProgramRepository.CountByTitle: %w", err)
	}
	return count, nil
}

func (r *RadioProgramRepository) FindAll(limit, offset int) ([]model.RadioProgram, error) {
	return r.FindAllContext(context.Background(), limit, offset)
}

func (r *RadioProgramRepository) FindAllContext(ctx context.Context, limit, offset int) ([]model.RadioProgram, error) {
	var programs []model.RadioProgram
	err := r.db.SelectContext(ctx, &programs,
		"SELECT * FROM radio_programs ORDER BY id DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.FindAll: %w", err)
	}
	return programs, nil
}

func (r *RadioProgramRepository) CountAll() (int, error) {
	return r.CountAllContext(context.Background())
}

func (r *RadioProgramRepository) CountAllContext(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM radio_programs")
	if err != nil {
		return 0, fmt.Errorf("repository.RadioProgramRepository.CountAll: %w", err)
	}
	return count, nil
}

func (r *RadioProgramRepository) FindPopularSummary(minReviews, limit int) ([]model.ProgramSummary, error) {
	return r.FindPopularSummaryContext(context.Background(), minReviews, limit)
}

func (r *RadioProgramRepository) FindPopularSummaryContext(ctx context.Context, minReviews, limit int) ([]model.ProgramSummary, error) {
	var results []model.ProgramSummary
	err := r.db.SelectContext(ctx, &results, `
		SELECT rp.id, rp.station_id, rp.title, COALESCE(rp.cast, '') AS cast,
		       COALESCE(AVG(p.rating), 0) AS avg_rating,
		       COUNT(CASE WHEN p.id IS NOT NULL THEN 1 END) AS reviews_count,
		       0 AS recent_high_count
		FROM radio_programs rp
		LEFT JOIN posts p ON p.program_id = rp.id
		GROUP BY rp.id, rp.station_id, rp.title, rp.cast
		HAVING reviews_count >= ?
		ORDER BY avg_rating DESC, reviews_count DESC
		LIMIT ?
	`, minReviews, limit)
	if err != nil {
		return nil, fmt.Errorf("repository.FindPopularSummary: %w", err)
	}
	return results, nil
}

func (r *RadioProgramRepository) FindSummaryByIDs(ids []int64) ([]model.ProgramSummary, error) {
	return r.FindSummaryByIDsContext(context.Background(), ids)
}

func (r *RadioProgramRepository) FindSummaryByIDsContext(ctx context.Context, ids []int64) ([]model.ProgramSummary, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(`
		SELECT rp.id, rp.station_id, rp.title, COALESCE(rp.cast, '') AS cast,
		       COALESCE(AVG(p.rating), 0) AS avg_rating,
		       COUNT(CASE WHEN p.id IS NOT NULL THEN 1 END) AS reviews_count,
		       0 AS recent_high_count
		FROM radio_programs rp
		LEFT JOIN posts p ON p.program_id = rp.id
		WHERE rp.id IN (?)
		GROUP BY rp.id, rp.station_id, rp.title, rp.cast
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("repository.FindSummaryByIDs: %w", err)
	}
	query = r.db.Rebind(query)
	var results []model.ProgramSummary
	if err := r.db.SelectContext(ctx, &results, query, args...); err != nil {
		return nil, fmt.Errorf("repository.FindSummaryByIDs: %w", err)
	}
	return results, nil
}

func (r *RadioProgramRepository) FindTrendingSummary(cutoff time.Time, limit int) ([]model.ProgramSummary, error) {
	return r.FindTrendingSummaryContext(context.Background(), cutoff, limit)
}

func (r *RadioProgramRepository) FindTrendingSummaryContext(ctx context.Context, cutoff time.Time, limit int) ([]model.ProgramSummary, error) {
	var results []model.ProgramSummary
	err := r.db.SelectContext(ctx, &results, `
		SELECT rp.id, rp.station_id, rp.title, COALESCE(rp.cast, '') AS cast,
		       AVG(p.rating) AS avg_rating,
		       COUNT(p.id) AS reviews_count,
		       SUM(CASE WHEN p.rating >= 4.0 AND p.created_at >= ? THEN 1 ELSE 0 END) AS recent_high_count
		FROM radio_programs rp
		INNER JOIN posts p ON p.program_id = rp.id
		GROUP BY rp.id, rp.station_id, rp.title, rp.cast
		HAVING recent_high_count > 0
		ORDER BY recent_high_count DESC, avg_rating DESC
		LIMIT ?
	`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("repository.FindTrendingSummary: %w", err)
	}
	return results, nil
}

// Upsert inserts or updates a radio program based on (station_id, title, start).
// Returns the ID of the inserted/updated row.
func (r *RadioProgramRepository) Upsert(program *model.RadioProgram) (int64, error) {
	return r.UpsertContext(context.Background(), program)
}

func (r *RadioProgramRepository) UpsertContext(ctx context.Context, program *model.RadioProgram) (int64, error) {
	res, err := r.db.NamedExecContext(ctx,
		`INSERT INTO radio_programs (station_id, title, cast, start, end, info, url, image)
		 VALUES (:station_id, :title, :cast, :start, :end, :info, :url, :image)
		 ON DUPLICATE KEY UPDATE
		   cast=VALUES(cast), end=VALUES(end), info=VALUES(info),
		   url=VALUES(url), image=VALUES(image)`,
		program,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.RadioProgramRepository.Upsert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.RadioProgramRepository.Upsert LastInsertId: %w", err)
	}
	// ON DUPLICATE KEY UPDATE returns 0 for LastInsertId when row exists; fetch by key.
	if id == 0 {
		var existing model.RadioProgram
		if err := r.db.GetContext(ctx, &existing,
			"SELECT * FROM radio_programs WHERE station_id = ? AND title = ? AND start = ? LIMIT 1",
			program.StationID, program.Title, program.Start,
		); err != nil {
			return 0, fmt.Errorf("repository.RadioProgramRepository.Upsert fetch existing: %w", err)
		}
		id = existing.ID
	}
	return id, nil
}

func (r *RadioProgramRepository) FindByStationAndTitleTx(ctx context.Context, tx *sqlx.Tx, stationID, title string) (*model.RadioProgram, error) {
	var p model.RadioProgram
	err := tx.GetContext(ctx, &p,
		"SELECT * FROM radio_programs WHERE station_id = ? AND title = ? LIMIT 1",
		stationID, title,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.FindByStationAndTitleTx: %w", err)
	}
	return &p, nil
}

func (r *RadioProgramRepository) UpsertTx(ctx context.Context, tx *sqlx.Tx, program *model.RadioProgram) (int64, error) {
	res, err := tx.NamedExecContext(ctx,
		`INSERT INTO radio_programs (station_id, title, cast, start, end, info, url, image)
		 VALUES (:station_id, :title, :cast, :start, :end, :info, :url, :image)
		 ON DUPLICATE KEY UPDATE
		   cast=VALUES(cast), end=VALUES(end), info=VALUES(info),
		   url=VALUES(url), image=VALUES(image)`,
		program,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.RadioProgramRepository.UpsertTx: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.RadioProgramRepository.UpsertTx LastInsertId: %w", err)
	}
	if id == 0 {
		var existing model.RadioProgram
		if err := tx.GetContext(ctx, &existing,
			"SELECT * FROM radio_programs WHERE station_id = ? AND title = ? AND start = ? LIMIT 1",
			program.StationID, program.Title, program.Start,
		); err != nil {
			return 0, fmt.Errorf("repository.RadioProgramRepository.UpsertTx fetch existing: %w", err)
		}
		id = existing.ID
	}
	return id, nil
}
