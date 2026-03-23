package service

import (
	"errors"
	"testing"

	"github.com/yourname/radio_review_go/internal/model"
)

// stubProgramRepo は RadioProgramRepositoryInterface のスタブ実装。
type stubProgramRepo struct {
	searchByTitleFunc func(keyword string, limit, offset int) ([]model.RadioProgram, error)
	searchByCastFunc  func(cast string, limit, offset int) ([]model.RadioProgram, error)
	countAllFunc      func() (int, error)
	findAllFunc       func(limit, offset int) ([]model.RadioProgram, error)
}

func (r *stubProgramRepo) FindByID(id int64) (*model.RadioProgram, error) { return nil, nil }
func (r *stubProgramRepo) FindByStationAndTitle(stationID, title string) (*model.RadioProgram, error) {
	return nil, nil
}
func (r *stubProgramRepo) SearchByTitle(keyword string, limit, offset int) ([]model.RadioProgram, error) {
	if r.searchByTitleFunc != nil {
		return r.searchByTitleFunc(keyword, limit, offset)
	}
	return nil, nil
}
func (r *stubProgramRepo) SearchByCast(cast string, limit, offset int) ([]model.RadioProgram, error) {
	if r.searchByCastFunc != nil {
		return r.searchByCastFunc(cast, limit, offset)
	}
	return nil, nil
}
func (r *stubProgramRepo) CountByTitle(keyword string) (int, error) { return 0, nil }
func (r *stubProgramRepo) CountAll() (int, error) {
	if r.countAllFunc != nil {
		return r.countAllFunc()
	}
	return 0, nil
}
func (r *stubProgramRepo) FindAll(limit, offset int) ([]model.RadioProgram, error) {
	if r.findAllFunc != nil {
		return r.findAllFunc(limit, offset)
	}
	return nil, nil
}
func (r *stubProgramRepo) Upsert(p *model.RadioProgram) (int64, error) { return 0, nil }
func (r *stubProgramRepo) DeleteDuplicates() error                     { return nil }
func (r *stubProgramRepo) FindByUserPosts(userID int64) ([]model.RadioProgram, error) { return nil, nil }

func TestSearchForAPI_NilKeywordAndCast(t *testing.T) {
	svc := &RadioProgramSearchService{repo: &stubProgramRepo{}, redis: nil}
	result, err := svc.SearchForAPI(nil, nil, nil, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestSearchForAPI_KeywordOnly(t *testing.T) {
	programs := []model.RadioProgram{
		{ID: 1, StationID: "TBS", Title: "jazz show"},
		{ID: 2, StationID: "LFR", Title: "jazz night"},
	}
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return programs, nil
		},
	}
	svc := &RadioProgramSearchService{repo: repo, redis: nil}
	kw := "jazz"
	result, err := svc.SearchForAPI(&kw, nil, nil, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestSearchForAPI_StationIDFilter(t *testing.T) {
	programs := []model.RadioProgram{
		{ID: 1, StationID: "TBS", Title: "jazz show"},
		{ID: 2, StationID: "LFR", Title: "jazz night"},
	}
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return programs, nil
		},
	}
	svc := &RadioProgramSearchService{repo: repo, redis: nil}
	kw := "jazz"
	station := "TBS"
	result, err := svc.SearchForAPI(&kw, nil, &station, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result after station filter, got %d", len(result))
	}
	if result[0].StationID != "TBS" {
		t.Errorf("expected TBS, got %s", result[0].StationID)
	}
}

func TestSearchForAPI_DeduplicatesUnion(t *testing.T) {
	castVal := "DJ Jazz"
	shared := model.RadioProgram{ID: 1, StationID: "TBS", Title: "radio show", Cast: &castVal}
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return []model.RadioProgram{shared}, nil
		},
		searchByCastFunc: func(cast string, limit, offset int) ([]model.RadioProgram, error) {
			return []model.RadioProgram{shared}, nil
		},
	}
	svc := &RadioProgramSearchService{repo: repo, redis: nil}
	kw := "radio"
	cast := "DJ Jazz"
	result, err := svc.SearchForAPI(&kw, &cast, nil, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 deduplicated result, got %d", len(result))
	}
}

func TestSearchForAPI_LimitApplied(t *testing.T) {
	programs := []model.RadioProgram{
		{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
	}
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return programs, nil
		},
	}
	svc := &RadioProgramSearchService{repo: repo, redis: nil}
	kw := "test"
	result, err := svc.SearchForAPI(&kw, nil, nil, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results (limit), got %d", len(result))
	}
}

func TestSearchForAPI_RepoError(t *testing.T) {
	repoErr := errors.New("db error")
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return nil, repoErr
		},
	}
	svc := &RadioProgramSearchService{repo: repo, redis: nil}
	kw := "test"
	_, err := svc.SearchForAPI(&kw, nil, nil, 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repoErr, got %v", err)
	}
}

func TestSearchProgramsWithPosts_Pagination(t *testing.T) {
	all := []model.RadioProgram{
		{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
	}
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return all, nil
		},
		searchByCastFunc: func(cast string, limit, offset int) ([]model.RadioProgram, error) {
			return nil, nil
		},
	}
	svc := &RadioProgramSearchService{repo: repo, redis: nil}

	result, total, err := svc.SearchProgramsWithPosts("test", 2, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 items on page 1, got %d", len(result))
	}

	result2, _, err := svc.SearchProgramsWithPosts("test", 2, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result2) != 1 {
		t.Errorf("expected 1 item on last page, got %d", len(result2))
	}
}
