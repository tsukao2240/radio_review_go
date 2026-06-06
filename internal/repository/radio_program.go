package repository

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/yourname/radio_review_go/internal/model"
)

type RadioProgramRepository struct {
	db *sqlx.DB
}

func NewRadioProgramRepository(db *sqlx.DB) *RadioProgramRepository {
	return &RadioProgramRepository{db: db}
}

func (r *RadioProgramRepository) FindByID(id int64) (*model.RadioProgram, error) {
	var p model.RadioProgram
	err := r.db.Get(&p, "SELECT * FROM radio_programs WHERE id = ? LIMIT 1", id)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.FindByID: %w", err)
	}
	return &p, nil
}

func (r *RadioProgramRepository) FindByStationAndTitle(stationID, title string) (*model.RadioProgram, error) {
	var p model.RadioProgram
	err := r.db.Get(&p,
		"SELECT * FROM radio_programs WHERE station_id = ? AND title = ? LIMIT 1",
		stationID, title,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.FindByStationAndTitle: %w", err)
	}
	return &p, nil
}

func (r *RadioProgramRepository) SearchByTitle(keyword string, limit, offset int) ([]model.RadioProgram, error) {
	var programs []model.RadioProgram
	err := r.db.Select(&programs,
		"SELECT * FROM radio_programs WHERE title LIKE ? ORDER BY id DESC LIMIT ? OFFSET ?",
		"%"+keyword+"%", limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.SearchByTitle: %w", err)
	}
	return programs, nil
}

func (r *RadioProgramRepository) SearchByCast(cast string, limit, offset int) ([]model.RadioProgram, error) {
	var programs []model.RadioProgram
	err := r.db.Select(&programs,
		"SELECT * FROM radio_programs WHERE cast LIKE ? ORDER BY id DESC LIMIT ? OFFSET ?",
		"%"+cast+"%", limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.SearchByCast: %w", err)
	}
	return programs, nil
}

func (r *RadioProgramRepository) CountByTitle(keyword string) (int, error) {
	var count int
	err := r.db.Get(&count,
		"SELECT COUNT(*) FROM radio_programs WHERE title LIKE ?",
		"%"+keyword+"%",
	)
	if err != nil {
		return 0, fmt.Errorf("repository.RadioProgramRepository.CountByTitle: %w", err)
	}
	return count, nil
}

func (r *RadioProgramRepository) FindAll(limit, offset int) ([]model.RadioProgram, error) {
	var programs []model.RadioProgram
	err := r.db.Select(&programs,
		"SELECT * FROM radio_programs ORDER BY id DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.RadioProgramRepository.FindAll: %w", err)
	}
	return programs, nil
}

func (r *RadioProgramRepository) CountAll() (int, error) {
	var count int
	err := r.db.Get(&count, "SELECT COUNT(*) FROM radio_programs")
	if err != nil {
		return 0, fmt.Errorf("repository.RadioProgramRepository.CountAll: %w", err)
	}
	return count, nil
}

func (r *RadioProgramRepository) FindPopularSummary(minReviews, limit int) ([]model.ProgramSummary, error) {
	var results []model.ProgramSummary
	err := r.db.Select(&results, `
		SELECT rp.id, rp.station_id, rp.title, COALESCE(rp.cast, '') AS cast,
		       AVG(p.rating) AS avg_rating,
		       COUNT(p.id) AS reviews_count,
		       0 AS recent_high_count
		FROM radio_programs rp
		INNER JOIN posts p ON p.program_id = rp.id
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

func (r *RadioProgramRepository) FindTrendingSummary(cutoff time.Time, limit int) ([]model.ProgramSummary, error) {
	var results []model.ProgramSummary
	err := r.db.Select(&results, `
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
	res, err := r.db.NamedExec(
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
		if err := r.db.Get(&existing,
			"SELECT * FROM radio_programs WHERE station_id = ? AND title = ? AND start = ? LIMIT 1",
			program.StationID, program.Title, program.Start,
		); err != nil {
			return 0, fmt.Errorf("repository.RadioProgramRepository.Upsert fetch existing: %w", err)
		}
		id = existing.ID
	}
	return id, nil
}
