package repository

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

type FavoriteProgramRepository struct {
	db *sqlx.DB
}

func NewFavoriteProgramRepository(db *sqlx.DB) *FavoriteProgramRepository {
	return &FavoriteProgramRepository{db: db}
}

func (r *FavoriteProgramRepository) FindByUser(userID int64) ([]model.FavoriteProgram, error) {
	var favs []model.FavoriteProgram
	err := r.db.Select(&favs,
		"SELECT * FROM favorite_programs WHERE user_id = ? ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("repository.FavoriteProgramRepository.FindByUser: %w", err)
	}
	return favs, nil
}

func (r *FavoriteProgramRepository) Exists(userID int64, stationID, programTitle string, broadcastDay *int) (bool, error) {
	var count int
	var err error
	if broadcastDay == nil {
		err = r.db.Get(&count,
			`SELECT COUNT(*) FROM favorite_programs
			 WHERE user_id = ? AND station_id = ? AND program_title = ? AND broadcast_day IS NULL`,
			userID, stationID, programTitle,
		)
	} else {
		err = r.db.Get(&count,
			`SELECT COUNT(*) FROM favorite_programs
			 WHERE user_id = ? AND station_id = ? AND program_title = ? AND broadcast_day = ?`,
			userID, stationID, programTitle, *broadcastDay,
		)
	}
	if err != nil {
		return false, fmt.Errorf("repository.FavoriteProgramRepository.Exists: %w", err)
	}
	return count > 0, nil
}

func (r *FavoriteProgramRepository) Create(fav *model.FavoriteProgram) (int64, error) {
	res, err := r.db.NamedExec(
		`INSERT INTO favorite_programs (user_id, station_id, program_title, broadcast_day, created_at, updated_at)
		 VALUES (:user_id, :station_id, :program_title, :broadcast_day, NOW(), NOW())`,
		fav,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.FavoriteProgramRepository.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.FavoriteProgramRepository.Create LastInsertId: %w", err)
	}
	return id, nil
}

func (r *FavoriteProgramRepository) Delete(userID int64, stationID, programTitle string, broadcastDay *int) error {
	var err error
	if broadcastDay == nil {
		_, err = r.db.Exec(
			`DELETE FROM favorite_programs
			 WHERE user_id = ? AND station_id = ? AND program_title = ? AND broadcast_day IS NULL`,
			userID, stationID, programTitle,
		)
	} else {
		_, err = r.db.Exec(
			`DELETE FROM favorite_programs
			 WHERE user_id = ? AND station_id = ? AND program_title = ? AND broadcast_day = ?`,
			userID, stationID, programTitle, *broadcastDay,
		)
	}
	if err != nil {
		return fmt.Errorf("repository.FavoriteProgramRepository.Delete: %w", err)
	}
	return nil
}
