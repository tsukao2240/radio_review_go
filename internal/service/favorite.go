package service

import (
	"errors"
	"fmt"

	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/repository"
)

// FavoriteService は FavoriteServiceInterface を実装する。
type FavoriteService struct {
	favRepo repository.FavoriteProgramRepositoryInterface
}

// NewFavoriteService は新しい FavoriteService を返す。
func NewFavoriteService(favRepo repository.FavoriteProgramRepositoryInterface) *FavoriteService {
	return &FavoriteService{
		favRepo: favRepo,
	}
}

// GetByUser はユーザーのお気に入り番組一覧を返す。
func (s *FavoriteService) GetByUser(userID int64) ([]model.FavoriteProgram, error) {
	favs, err := s.favRepo.FindByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("FavoriteService.GetByUser: %w", err)
	}
	return favs, nil
}

// Add はお気に入りを追加する。既に登録済みの場合はエラーを返す。
func (s *FavoriteService) Add(userID int64, stationID, programTitle string, broadcastDay *int) (*model.FavoriteProgram, error) {
	exists, err := s.favRepo.Exists(userID, stationID, programTitle, broadcastDay)
	if err != nil {
		return nil, fmt.Errorf("FavoriteService.Add exists check: %w", err)
	}
	if exists {
		return nil, errors.New("already added to favorites")
	}

	fav := &model.FavoriteProgram{
		UserID:       userID,
		StationID:    stationID,
		ProgramTitle: programTitle,
		BroadcastDay: broadcastDay,
	}

	favID, err := s.favRepo.Create(fav)
	if err != nil {
		return nil, fmt.Errorf("FavoriteService.Add create: %w", err)
	}
	fav.ID = favID

	return fav, nil
}

// Remove はお気に入りを削除する。
func (s *FavoriteService) Remove(userID int64, stationID, programTitle string, broadcastDay *int) error {
	if err := s.favRepo.Delete(userID, stationID, programTitle, broadcastDay); err != nil {
		return fmt.Errorf("FavoriteService.Remove: %w", err)
	}
	return nil
}

// Check はお気に入り登録済みかどうかを確認する。
func (s *FavoriteService) Check(userID int64, stationID, programTitle string, broadcastDay *int) (bool, error) {
	exists, err := s.favRepo.Exists(userID, stationID, programTitle, broadcastDay)
	if err != nil {
		return false, fmt.Errorf("FavoriteService.Check: %w", err)
	}
	return exists, nil
}
