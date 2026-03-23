package service

import (
	"testing"
	"time"

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
