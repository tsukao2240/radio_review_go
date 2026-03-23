package service

import (
	"errors"
	"testing"

	"github.com/yourname/radio_review_go/internal/model"
)

type stubFavRepo struct {
	findByUserFunc func(userID int64) ([]model.FavoriteProgram, error)
	existsFunc     func(userID int64, stationID, programTitle string, broadcastDay *int) (bool, error)
	createFunc     func(fav *model.FavoriteProgram) (int64, error)
	deleteFunc     func(userID int64, stationID, programTitle string, broadcastDay *int) error
}

func (r *stubFavRepo) FindByUser(userID int64) ([]model.FavoriteProgram, error) {
	if r.findByUserFunc != nil {
		return r.findByUserFunc(userID)
	}
	return nil, nil
}
func (r *stubFavRepo) Exists(userID int64, stationID, programTitle string, broadcastDay *int) (bool, error) {
	if r.existsFunc != nil {
		return r.existsFunc(userID, stationID, programTitle, broadcastDay)
	}
	return false, nil
}
func (r *stubFavRepo) Create(fav *model.FavoriteProgram) (int64, error) {
	if r.createFunc != nil {
		return r.createFunc(fav)
	}
	return 1, nil
}
func (r *stubFavRepo) Delete(userID int64, stationID, programTitle string, broadcastDay *int) error {
	if r.deleteFunc != nil {
		return r.deleteFunc(userID, stationID, programTitle, broadcastDay)
	}
	return nil
}

func TestFavoriteService_Add(t *testing.T) {
	t.Run("新規追加: 成功", func(t *testing.T) {
		repo := &stubFavRepo{
			existsFunc: func(_ int64, _, _ string, _ *int) (bool, error) { return false, nil },
			createFunc: func(fav *model.FavoriteProgram) (int64, error) { return 42, nil },
		}
		svc := NewFavoriteService(repo)
		fav, err := svc.Add(1, "TBS", "jazz show", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fav.ID != 42 {
			t.Errorf("expected ID=42, got %d", fav.ID)
		}
		if fav.StationID != "TBS" {
			t.Errorf("expected StationID=TBS, got %s", fav.StationID)
		}
	})

	t.Run("既に登録済み: エラー", func(t *testing.T) {
		repo := &stubFavRepo{
			existsFunc: func(_ int64, _, _ string, _ *int) (bool, error) { return true, nil },
		}
		svc := NewFavoriteService(repo)
		_, err := svc.Add(1, "TBS", "jazz show", nil)
		if err == nil {
			t.Fatal("expected error for duplicate, got nil")
		}
		if err.Error() != "already added to favorites" {
			t.Errorf("unexpected error: %q", err.Error())
		}
	})

	t.Run("exists チェックでDBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo := &stubFavRepo{
			existsFunc: func(_ int64, _, _ string, _ *int) (bool, error) { return false, repoErr },
		}
		svc := NewFavoriteService(repo)
		_, err := svc.Add(1, "TBS", "jazz show", nil)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestFavoriteService_Remove(t *testing.T) {
	t.Run("削除成功", func(t *testing.T) {
		var called bool
		repo := &stubFavRepo{
			deleteFunc: func(_ int64, _, _ string, _ *int) error {
				called = true
				return nil
			},
		}
		svc := NewFavoriteService(repo)
		if err := svc.Remove(1, "TBS", "jazz show", nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !called {
			t.Error("Delete not called")
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("delete error")
		repo := &stubFavRepo{
			deleteFunc: func(_ int64, _, _ string, _ *int) error { return repoErr },
		}
		svc := NewFavoriteService(repo)
		err := svc.Remove(1, "TBS", "jazz show", nil)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}

func TestFavoriteService_Check(t *testing.T) {
	t.Run("登録済みの場合 true", func(t *testing.T) {
		repo := &stubFavRepo{
			existsFunc: func(_ int64, _, _ string, _ *int) (bool, error) { return true, nil },
		}
		svc := NewFavoriteService(repo)
		ok, err := svc.Check(1, "TBS", "jazz show", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("expected true, got false")
		}
	})

	t.Run("未登録の場合 false", func(t *testing.T) {
		repo := &stubFavRepo{
			existsFunc: func(_ int64, _, _ string, _ *int) (bool, error) { return false, nil },
		}
		svc := NewFavoriteService(repo)
		ok, err := svc.Check(1, "TBS", "jazz show", nil)
		if err != nil || ok {
			t.Errorf("expected false/nil, got %v/%v", ok, err)
		}
	})
}

func TestFavoriteService_GetByUser(t *testing.T) {
	t.Run("お気に入り一覧取得: 成功", func(t *testing.T) {
		want := []model.FavoriteProgram{{ID: 1, UserID: 5, StationID: "TBS"}, {ID: 2, UserID: 5, StationID: "QRR"}}
		repo := &stubFavRepo{
			findByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
				return want, nil
			},
		}
		svc := NewFavoriteService(repo)
		got, err := svc.GetByUser(5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 favorites, got %d", len(got))
		}
	})

	t.Run("DBエラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo := &stubFavRepo{
			findByUserFunc: func(userID int64) ([]model.FavoriteProgram, error) {
				return nil, repoErr
			},
		}
		svc := NewFavoriteService(repo)
		_, err := svc.GetByUser(1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}
