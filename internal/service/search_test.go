package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/yourname/radio_review_go/internal/model"
)

// stubProgramRepo は RadioProgramRepositoryInterface のスタブ実装。
type stubProgramRepo struct {
	searchByTitleFunc         func(keyword string, limit, offset int) ([]model.RadioProgram, error)
	searchByCastFunc          func(cast string, limit, offset int) ([]model.RadioProgram, error)
	countAllFunc              func() (int, error)
	findAllFunc               func(limit, offset int) ([]model.RadioProgram, error)
	findByStationAndTitleFunc func(stationID, title string) (*model.RadioProgram, error)
	upsertFunc                func(program *model.RadioProgram) (int64, error)
}

func (r *stubProgramRepo) FindByID(id int64) (*model.RadioProgram, error) { return nil, nil }
func (r *stubProgramRepo) FindByStationAndTitle(stationID, title string) (*model.RadioProgram, error) {
	if r.findByStationAndTitleFunc != nil {
		return r.findByStationAndTitleFunc(stationID, title)
	}
	return nil, errors.New("not found")
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
func (r *stubProgramRepo) Upsert(p *model.RadioProgram) (int64, error) {
	if r.upsertFunc != nil {
		return r.upsertFunc(p)
	}
	return 1, nil
}

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
	shared := model.RadioProgram{ID: 1, StationID: "TBS", Title: "radio show", Cast: "DJ Jazz"}
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

func TestKeywordMD5(t *testing.T) {
	t.Run("32文字の16進数を返す", func(t *testing.T) {
		result := keywordMD5("jazz")
		if len(result) != 32 {
			t.Errorf("expected 32 chars, got %d: %q", len(result), result)
		}
	})

	t.Run("決定的: 同じ入力は同じ出力", func(t *testing.T) {
		a := keywordMD5("radio")
		b := keywordMD5("radio")
		if a != b {
			t.Errorf("expected same output, got %q and %q", a, b)
		}
	})

	t.Run("異なる入力は異なる出力", func(t *testing.T) {
		a := keywordMD5("jazz")
		b := keywordMD5("news")
		if a == b {
			t.Errorf("expected different outputs, got same: %q", a)
		}
	})
}

func TestGetAllPrograms(t *testing.T) {
	t.Run("番組一覧と件数を返す", func(t *testing.T) {
		repo := &stubProgramRepo{
			countAllFunc: func() (int, error) { return 5, nil },
			findAllFunc: func(limit, offset int) ([]model.RadioProgram, error) {
				return []model.RadioProgram{{ID: 1}, {ID: 2}}, nil
			},
		}
		svc := &RadioProgramSearchService{repo: repo, redis: nil}
		programs, total, err := svc.GetAllPrograms(10, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(programs) != 2 {
			t.Errorf("got %d programs, want 2", len(programs))
		}
		if total != 5 {
			t.Errorf("got total=%d, want 5", total)
		}
	})

	t.Run("page < 1 は 1 に補正される", func(t *testing.T) {
		var capturedOffset int
		repo := &stubProgramRepo{
			countAllFunc: func() (int, error) { return 0, nil },
			findAllFunc: func(limit, offset int) ([]model.RadioProgram, error) {
				capturedOffset = offset
				return nil, nil
			},
		}
		svc := &RadioProgramSearchService{repo: repo, redis: nil}
		_, _, err := svc.GetAllPrograms(10, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedOffset != 0 {
			t.Errorf("expected offset=0, got %d", capturedOffset)
		}
	})

	t.Run("CountAll エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("count error")
		repo := &stubProgramRepo{
			countAllFunc: func() (int, error) { return 0, repoErr },
		}
		svc := &RadioProgramSearchService{repo: repo, redis: nil}
		_, _, err := svc.GetAllPrograms(10, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})

	t.Run("FindAll エラー: 伝播", func(t *testing.T) {
		repoErr := errors.New("findall error")
		repo := &stubProgramRepo{
			countAllFunc: func() (int, error) { return 3, nil },
			findAllFunc: func(_, _ int) ([]model.RadioProgram, error) { return nil, repoErr },
		}
		svc := &RadioProgramSearchService{repo: repo, redis: nil}
		_, _, err := svc.GetAllPrograms(10, 1)
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
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

func newSearchMiniRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestNewRadioProgramSearchService(t *testing.T) {
	svc := NewRadioProgramSearchService(nil, nil)
	if svc == nil {
		t.Error("expected non-nil service")
	}
}

func TestSearchCacheGetSet(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	ctx := context.Background()

	t.Run("存在しないキー: false", func(t *testing.T) {
		var dest []model.RadioProgram
		ok := searchCacheGet(ctx, rdb, "missing", &dest)
		if ok {
			t.Error("expected false for missing key")
		}
	})

	t.Run("セット後に取得: true", func(t *testing.T) {
		data := []model.RadioProgram{{ID: 1, Title: "jazz show"}}
		searchCacheSet(ctx, rdb, "test_key", data, time.Minute)

		var dest []model.RadioProgram
		ok := searchCacheGet(ctx, rdb, "test_key", &dest)
		if !ok {
			t.Error("expected true after set")
		}
		if len(dest) != 1 || dest[0].Title != "jazz show" {
			t.Errorf("unexpected result: %v", dest)
		}
	})

	t.Run("不正なJSON: false", func(t *testing.T) {
		rdb.Set(ctx, "bad_json", "not-json", time.Minute)
		var dest []model.RadioProgram
		ok := searchCacheGet(ctx, rdb, "bad_json", &dest)
		if ok {
			t.Error("expected false for invalid JSON")
		}
	})
}

func TestSearchByTitle_CacheHit(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repo := &stubProgramRepo{}
	svc := NewRadioProgramSearchService(repo, rdb)

	cached := []model.RadioProgram{{ID: 1, Title: "jazz show"}, {ID: 2, Title: "jazz night"}}
	cacheKey := "search_programs_" + keywordMD5("jazz")
	searchCacheSet(context.Background(), rdb, cacheKey, cached, time.Minute)

	result, err := svc.SearchByTitle("jazz", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results from cache, got %d", len(result))
	}
}

func TestSearchByTitle_CacheMiss(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return []model.RadioProgram{{ID: 3, Title: "morning jazz", StationID: "TBS"}}, nil
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	result, err := svc.SearchByTitle("jazz", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestSearchByTitle_StationFilter(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return []model.RadioProgram{
				{ID: 1, Title: "jazz show", StationID: "TBS"},
				{ID: 2, Title: "jazz night", StationID: "QRR"},
			}, nil
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	stationID := "TBS"
	result, err := svc.SearchByTitle("jazz", &stationID)
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

func TestSearchByCast_CacheMiss(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repo := &stubProgramRepo{
		searchByCastFunc: func(cast string, limit, offset int) ([]model.RadioProgram, error) {
			return []model.RadioProgram{{ID: 1, Title: "morning show", StationID: "TBS"}}, nil
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	result, err := svc.SearchByCast("DJ Smith", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestSearchByCast_WithStationFilter(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repo := &stubProgramRepo{
		searchByCastFunc: func(cast string, limit, offset int) ([]model.RadioProgram, error) {
			return []model.RadioProgram{
				{ID: 1, Title: "show A", StationID: "TBS"},
				{ID: 2, Title: "show B", StationID: "QRR"},
			}, nil
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	stationID := "TBS"
	result, err := svc.SearchByCast("DJ Smith", &stationID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].StationID != "TBS" {
		t.Errorf("expected 1 TBS result, got %v", result)
	}
}

func TestSearchByCast_RepoError(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repoErr := errors.New("db error")
	repo := &stubProgramRepo{
		searchByCastFunc: func(cast string, limit, offset int) ([]model.RadioProgram, error) {
			return nil, repoErr
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	_, err := svc.SearchByCast("DJ Smith", nil)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repoErr, got %v", err)
	}
}

func TestSearchProgramsWithPosts_Basic(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return []model.RadioProgram{
				{ID: 1, Title: "jazz show"},
				{ID: 2, Title: "jazz night"},
			}, nil
		},
		searchByCastFunc: func(cast string, limit, offset int) ([]model.RadioProgram, error) {
			return []model.RadioProgram{
				{ID: 2, Title: "jazz night"}, // duplicate to test dedup
				{ID: 3, Title: "another show"},
			}, nil
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	result, total, err := svc.SearchProgramsWithPosts("jazz", 10, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total=3 (dedup), got %d", total)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d", len(result))
	}
}

func TestSearchProgramsWithPosts_TitleError(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repoErr := errors.New("title search error")
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return nil, repoErr
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	_, _, err := svc.SearchProgramsWithPosts("jazz", 10, 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repoErr, got %v", err)
	}
}

func TestSearchProgramsWithPosts_CastError(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	repoErr := errors.New("cast search error")
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return nil, nil
		},
		searchByCastFunc: func(cast string, limit, offset int) ([]model.RadioProgram, error) {
			return nil, repoErr
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	_, _, err := svc.SearchProgramsWithPosts("jazz", 10, 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repoErr, got %v", err)
	}
}

func TestSearchProgramsWithPosts_PaginationPage2(t *testing.T) {
	rdb := newSearchMiniRedis(t)
	programs := make([]model.RadioProgram, 15)
	for i := range programs {
		programs[i] = model.RadioProgram{ID: int64(i + 1), Title: "show"}
	}
	repo := &stubProgramRepo{
		searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
			return programs, nil
		},
		searchByCastFunc: func(cast string, limit, offset int) ([]model.RadioProgram, error) {
			return nil, nil
		},
	}
	svc := NewRadioProgramSearchService(repo, rdb)

	result, total, err := svc.SearchProgramsWithPosts("show", 5, 2) // page 2
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 15 {
		t.Errorf("expected total=15, got %d", total)
	}
	if len(result) != 5 {
		t.Errorf("expected 5 results for page 2, got %d", len(result))
	}
}
