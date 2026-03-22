package repository

import (
	"fmt"

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
