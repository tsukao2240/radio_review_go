package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/yourname/radio_review_go/internal/model"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{
			input: "ジャズ・ナイト",
			want:  []string{"ジャズ", "ナイト"},
		},
		{
			input: "【特集】音楽の時間（毎週月曜）",
			want:  []string{"特集", "音楽の時間", "毎週月曜"},
		},
		{
			input: "ABC Radio Morning Show",
			want:  nil,
		},
		{
			input: "ラジオ深夜便",
			want:  []string{"ラジオ深夜便"},
		},
		{
			input: "「今夜のニュース」",
			want:  []string{"今夜のニュース"},
		},
		{
			input: "あ", // 1文字は除外
			want:  nil,
		},
	}

	for _, tt := range tests {
		got := cleanTitle(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("cleanTitle(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("cleanTitle(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestRecommendationService_GetTrendingPrograms(t *testing.T) {
	t.Run("高評価レビューが多い番組を返す", func(t *testing.T) {
		recent := time.Now().Add(-1 * time.Hour) // 1時間前

		programRepo := &stubProgramRepo{
			findAllFunc: func(limit, offset int) ([]model.RadioProgram, error) {
				return []model.RadioProgram{
					{ID: 1, StationID: "TBS", Title: "jazz show"},
					{ID: 2, StationID: "LFR", Title: "news program"},
				}, nil
			},
		}
		postRepo := &stubPostRepo{
			findByProgramFunc: func(stationID, programTitle string, limit, offset int) ([]model.Post, error) {
				if programTitle == "jazz show" {
					return []model.Post{
						{ID: 1, Rating: 5.0, CreatedAt: recent},
						{ID: 2, Rating: 4.5, CreatedAt: recent},
					}, nil
				}
				// news program は高評価なし
				return []model.Post{
					{ID: 3, Rating: 2.0, CreatedAt: recent},
				}, nil
			},
		}

		svc := &RecommendationService{
			postRepo:    postRepo,
			programRepo: programRepo,
			favRepo:     &stubFavRepo{},
			redis:       nil,
		}

		result, err := svc.GetTrendingPrograms(7, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("got %d results, want 1 (only jazz show)", len(result))
			return
		}
		if result[0]["title"] != "jazz show" {
			t.Errorf("got title=%q, want 'jazz show'", result[0]["title"])
		}
		if result[0]["recent_reviews_count"] != 2 {
			t.Errorf("got recent_reviews_count=%v, want 2", result[0]["recent_reviews_count"])
		}
	})

	t.Run("limit が適用される", func(t *testing.T) {
		recent := time.Now().Add(-1 * time.Hour)
		programs := make([]model.RadioProgram, 5)
		for i := range programs {
			programs[i] = model.RadioProgram{ID: int64(i + 1), StationID: "TBS", Title: "program"}
		}

		programRepo := &stubProgramRepo{
			findAllFunc: func(limit, offset int) ([]model.RadioProgram, error) {
				return programs, nil
			},
		}
		postRepo := &stubPostRepo{
			findByProgramFunc: func(stationID, programTitle string, limit, offset int) ([]model.Post, error) {
				return []model.Post{{ID: 1, Rating: 5.0, CreatedAt: recent}}, nil
			},
		}

		svc := &RecommendationService{
			postRepo:    postRepo,
			programRepo: programRepo,
			favRepo:     &stubFavRepo{},
			redis:       nil,
		}

		result, err := svc.GetTrendingPrograms(7, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) > 3 {
			t.Errorf("got %d results, want <= 3", len(result))
		}
	})

	t.Run("期間外のレビューは除外される", func(t *testing.T) {
		old := time.Now().Add(-30 * 24 * time.Hour) // 30日前

		programRepo := &stubProgramRepo{
			findAllFunc: func(limit, offset int) ([]model.RadioProgram, error) {
				return []model.RadioProgram{
					{ID: 1, StationID: "TBS", Title: "old show"},
				}, nil
			},
		}
		postRepo := &stubPostRepo{
			findByProgramFunc: func(stationID, programTitle string, limit, offset int) ([]model.Post, error) {
				return []model.Post{
					{ID: 1, Rating: 5.0, CreatedAt: old}, // 7日前の範囲外
				}, nil
			},
		}

		svc := &RecommendationService{
			postRepo:    postRepo,
			programRepo: programRepo,
			favRepo:     &stubFavRepo{},
			redis:       nil,
		}

		result, err := svc.GetTrendingPrograms(7, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("got %d results, want 0 (all reviews too old)", len(result))
		}
	})
}

func TestExtractKeywords(t *testing.T) {
	t.Run("高評価の投稿からキーワードを抽出する", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByUserFunc: func(userID int64, limit, offset int) ([]model.Post, error) {
				return []model.Post{
					{ID: 1, UserID: userID, ProgramTitle: "ジャズ・ナイト", Rating: 4.5},
					{ID: 2, UserID: userID, ProgramTitle: "音楽の時間", Rating: 5.0},
					{ID: 3, UserID: userID, ProgramTitle: "ニュース速報", Rating: 2.0}, // 低評価: 除外
				}, nil
			},
		}
		svc := &RecommendationService{
			postRepo:    postRepo,
			programRepo: &stubProgramRepo{},
			favRepo:     &stubFavRepo{},
			redis:       nil,
		}
		keywords, err := svc.extractKeywords(1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keywords) == 0 {
			t.Error("expected keywords, got none")
		}
	})

	t.Run("投稿なし: nil を返す", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByUserFunc: func(userID int64, limit, offset int) ([]model.Post, error) {
				return nil, nil
			},
		}
		svc := &RecommendationService{
			postRepo:    postRepo,
			programRepo: &stubProgramRepo{},
			favRepo:     &stubFavRepo{},
			redis:       nil,
		}
		keywords, err := svc.extractKeywords(1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if keywords != nil {
			t.Errorf("expected nil keywords, got %v", keywords)
		}
	})

	t.Run("低評価のみ: nil を返す", func(t *testing.T) {
		postRepo := &stubPostRepo{
			findByUserFunc: func(userID int64, limit, offset int) ([]model.Post, error) {
				return []model.Post{
					{ID: 1, ProgramTitle: "ニュース速報", Rating: 3.0},
					{ID: 2, ProgramTitle: "スポーツ情報", Rating: 2.5},
				}, nil
			},
		}
		svc := &RecommendationService{
			postRepo:    postRepo,
			programRepo: &stubProgramRepo{},
			favRepo:     &stubFavRepo{},
			redis:       nil,
		}
		keywords, err := svc.extractKeywords(1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if keywords != nil {
			t.Errorf("expected nil keywords for low-rated posts, got %v", keywords)
		}
	})
}

func TestFindSimilarPrograms(t *testing.T) {
	t.Run("キーワードで番組を検索して平均評価順に返す", func(t *testing.T) {
		programRepo := &stubProgramRepo{
			searchByTitleFunc: func(keyword string, limit, offset int) ([]model.RadioProgram, error) {
				return []model.RadioProgram{
					{ID: 1, StationID: "TBS", Title: "ジャズナイト"},
					{ID: 2, StationID: "LFR", Title: "ジャズ特集"},
				}, nil
			},
		}
		postRepo := &stubPostRepo{
			avgRatingFunc: func(programID int64) (float64, error) {
				if programID == 1 {
					return 4.8, nil
				}
				return 3.5, nil
			},
			findByProgramFunc: func(stationID, programTitle string, limit, offset int) ([]model.Post, error) {
				if programTitle == "ジャズナイト" {
					return []model.Post{{ID: 1}, {ID: 2}}, nil
				}
				return []model.Post{{ID: 3}}, nil
			},
		}
		svc := &RecommendationService{
			postRepo:    postRepo,
			programRepo: programRepo,
			favRepo:     &stubFavRepo{},
			redis:       nil,
		}
		result, err := svc.findSimilarPrograms([]string{"ジャズ"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) == 0 {
			t.Fatal("expected results, got none")
		}
		// 高評価順にソートされているか確認
		if result[0]["title"] != "ジャズナイト" {
			t.Errorf("expected first result to be 'ジャズナイト' (highest avg_rating), got %q", result[0]["title"])
		}
	})
}

func TestRecommendationService_GetPopularPrograms(t *testing.T) {
	t.Run("平均評価の高い番組を返す", func(t *testing.T) {
		programRepo := &stubProgramRepo{
			findAllFunc: func(limit, offset int) ([]model.RadioProgram, error) {
				return []model.RadioProgram{
					{ID: 1, StationID: "TBS", Title: "top show"},
					{ID: 2, StationID: "LFR", Title: "low show"},
				}, nil
			},
		}
		postRepo := &stubPostRepo{
			findByProgramFunc: func(stationID, programTitle string, limit, offset int) ([]model.Post, error) {
				if programTitle == "top show" {
					return []model.Post{{Rating: 5.0}, {Rating: 4.5}}, nil
				}
				return []model.Post{{Rating: 2.0}}, nil
			},
		}

		svc := &RecommendationService{
			postRepo:    postRepo,
			programRepo: programRepo,
			favRepo:     &stubFavRepo{},
			redis:       nil,
		}

		result, err := svc.getPopularPrograms(10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("got %d results, want 2", len(result))
			return
		}
		// 高評価順で top show が先頭のはず
		if result[0]["title"] != "top show" {
			t.Errorf("got first title=%q, want 'top show'", result[0]["title"])
		}
	})
}

func newTestMiniRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestRecommendationService_ClearUserCache(t *testing.T) {
	rdb := newTestMiniRedis(t)

	// Pre-populate cache
	rdb.Set(context.Background(), "recommendations_5", `[{"title":"jazz"}]`, time.Minute)

	svc := &RecommendationService{
		postRepo:    &stubPostRepo{},
		programRepo: &stubProgramRepo{},
		favRepo:     &stubFavRepo{},
		redis:       rdb,
	}

	if err := svc.ClearUserCache(5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's gone
	val, _ := rdb.Get(context.Background(), "recommendations_5").Result()
	if val != "" {
		t.Error("expected cache to be cleared")
	}
}

func TestRecommendationService_GetRecommendations_CacheHit(t *testing.T) {
	rdb := newTestMiniRedis(t)

	// Pre-populate cache
	cached := []map[string]interface{}{{"title": "jazz show", "station_id": "TBS"}}
	b, _ := json.Marshal(cached)
	rdb.Set(context.Background(), "recommendations_1", string(b), time.Minute)

	svc := &RecommendationService{
		postRepo:    &stubPostRepo{},
		programRepo: &stubProgramRepo{},
		favRepo:     &stubFavRepo{},
		redis:       rdb,
	}

	result, err := svc.GetRecommendations(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result from cache, got %d", len(result))
	}
	if result[0]["title"] != "jazz show" {
		t.Errorf("expected title='jazz show', got %v", result[0]["title"])
	}
}

func TestRecommendationService_NewRecommendationService(t *testing.T) {
	svc := NewRecommendationService(nil, nil, nil, nil)
	if svc == nil {
		t.Error("expected non-nil service")
	}
}
