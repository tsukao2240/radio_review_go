package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/radio_review_go/internal/repository"
)

// RecommendationService は RecommendationServiceInterface を実装する。
type RecommendationService struct {
	postRepo    repository.PostRepositoryInterface
	programRepo repository.RadioProgramRepositoryInterface
	favRepo     repository.FavoriteProgramRepositoryInterface
	redis       *redis.Client
}

// NewRecommendationService は新しい RecommendationService を返す。
func NewRecommendationService(
	postRepo repository.PostRepositoryInterface,
	programRepo repository.RadioProgramRepositoryInterface,
	favRepo repository.FavoriteProgramRepositoryInterface,
	rdb *redis.Client,
) *RecommendationService {
	return &RecommendationService{
		postRepo:    postRepo,
		programRepo: programRepo,
		favRepo:     favRepo,
		redis:       rdb,
	}
}

const (
	recommendationCacheTTL = 30 * time.Minute
	recommendationLimit    = 10
)

// GetRecommendations はユーザーへのパーソナライズされたレコメンデーションを返す。
// Redisに30分キャッシュする。結果が空の場合は人気番組を返す。
func (s *RecommendationService) GetRecommendations(userID int64) ([]map[string]interface{}, error) {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("recommendations_%d", userID)

	// キャッシュ確認
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var result []map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(cached), &result); jsonErr == nil {
			return result, nil
		}
	}

	result, err := s.buildRecommendations(userID)
	if err != nil {
		log.Printf("RecommendationService.GetRecommendations error for user %d: %v", userID, err)
		// エラー時は人気番組を返す
		return s.getPopularPrograms(recommendationLimit)
	}

	// キャッシュ保存
	if b, jsonErr := json.Marshal(result); jsonErr == nil {
		if setErr := s.redis.Set(ctx, cacheKey, string(b), recommendationCacheTTL).Err(); setErr != nil {
			log.Printf("RecommendationService: redis set error: %v", setErr)
		}
	}

	return result, nil
}

// GetTrendingPrograms は直近 days 日間で高評価レビューが多いトレンド番組を返す。
// 既存のリポジトリメソッドを組み合わせて実装する。
func (s *RecommendationService) GetTrendingPrograms(days, limit int) ([]map[string]interface{}, error) {
	// 全番組を取得し、それぞれの最近の高評価投稿数と平均評価を集計する
	allPrograms, err := s.programRepo.FindAll(200, 0)
	if err != nil {
		return nil, fmt.Errorf("RecommendationService.GetTrendingPrograms FindAll: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -days)

	type trendEntry struct {
		programID          int64
		title              string
		stationID          string
		cast               *string
		recentReviewsCount int
		avgRating          float64
	}

	var entries []trendEntry
	for _, prog := range allPrograms {
		posts, err := s.postRepo.FindByProgram(prog.StationID, prog.Title, 100, 0)
		if err != nil {
			continue
		}

		recentCount := 0
		var ratingSum float64
		var ratingCount int
		for _, p := range posts {
			if p.Rating >= 4.0 && p.CreatedAt.After(cutoff) {
				recentCount++
			}
			ratingSum += p.Rating
			ratingCount++
		}

		if recentCount == 0 {
			continue
		}

		avgRating := 0.0
		if ratingCount > 0 {
			avgRating = ratingSum / float64(ratingCount)
		}

		entries = append(entries, trendEntry{
			programID:          prog.ID,
			title:              prog.Title,
			stationID:          prog.StationID,
			cast:               prog.Cast,
			recentReviewsCount: recentCount,
			avgRating:          avgRating,
		})
	}

	// recent_reviews_count 降順、avg_rating 降順でソート
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].recentReviewsCount != entries[j].recentReviewsCount {
			return entries[i].recentReviewsCount > entries[j].recentReviewsCount
		}
		return entries[i].avgRating > entries[j].avgRating
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	result := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		m := map[string]interface{}{
			"id":                   e.programID,
			"title":                e.title,
			"station_id":           e.stationID,
			"avg_rating":           fmt.Sprintf("%.1f", e.avgRating),
			"recent_reviews_count": e.recentReviewsCount,
		}
		if e.cast != nil {
			m["cast"] = *e.cast
		}
		result = append(result, m)
	}

	log.Printf("RecommendationService.GetTrendingPrograms: days=%d count=%d", days, len(result))
	return result, nil
}

// ClearUserCache はユーザーのレコメンデーションキャッシュを削除する。
func (s *RecommendationService) ClearUserCache(userID int64) error {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("recommendations_%d", userID)
	if err := s.redis.Del(ctx, cacheKey).Err(); err != nil {
		return fmt.Errorf("RecommendationService.ClearUserCache: %w", err)
	}
	log.Printf("RecommendationService: cache cleared for user %d", userID)
	return nil
}

// buildRecommendations はキャッシュなしでレコメンデーションを構築する。
func (s *RecommendationService) buildRecommendations(userID int64) ([]map[string]interface{}, error) {
	keywords, err := s.extractKeywords(userID)
	if err != nil {
		return nil, fmt.Errorf("extractKeywords: %w", err)
	}

	if len(keywords) == 0 {
		log.Printf("RecommendationService: no user history for user %d, returning popular programs", userID)
		return s.getPopularPrograms(recommendationLimit)
	}

	result, err := s.findSimilarPrograms(keywords)
	if err != nil {
		return nil, fmt.Errorf("findSimilarPrograms: %w", err)
	}

	log.Printf("RecommendationService: recommendations built for user %d, keywords=%d results=%d",
		userID, len(keywords), len(result))

	if len(result) == 0 {
		return s.getPopularPrograms(recommendationLimit)
	}

	return result, nil
}

// extractKeywords はユーザーのお気に入りと高評価レビューからキーワードを抽出する。
func (s *RecommendationService) extractKeywords(userID int64) ([]string, error) {
	wordCount := make(map[string]int)

	// お気に入り番組のタイトルからキーワードを抽出
	favs, err := s.favRepo.FindByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("favRepo.FindByUser: %w", err)
	}
	for _, fav := range favs {
		for _, word := range cleanTitle(fav.ProgramTitle) {
			wordCount[word]++
		}
	}

	// 高評価（4.0以上）レビューの番組タイトルからキーワードを抽出
	posts, err := s.postRepo.FindByUser(userID, 50, 0)
	if err != nil {
		return nil, fmt.Errorf("postRepo.FindByUser: %w", err)
	}
	for _, p := range posts {
		if p.Rating >= 4.0 {
			for _, word := range cleanTitle(p.ProgramTitle) {
				wordCount[word]++
			}
		}
	}

	if len(wordCount) == 0 {
		return nil, nil
	}

	// 頻度降順でソートして上位10キーワードを返す
	type wordFreq struct {
		word  string
		count int
	}
	freqs := make([]wordFreq, 0, len(wordCount))
	for w, c := range wordCount {
		freqs = append(freqs, wordFreq{w, c})
	}
	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i].count > freqs[j].count
	})

	limit := 10
	if len(freqs) < limit {
		limit = len(freqs)
	}
	keywords := make([]string, limit)
	for i := 0; i < limit; i++ {
		keywords[i] = freqs[i].word
	}
	return keywords, nil
}

// cleanTitle はタイトルから日本語キーワードを抽出する。
// 記号を除去して2文字以上の日本語連続文字列を返す。
var (
	symbolRe   = regexp.MustCompile(`[「」『』【】（）()\x{301c}\x{ff5e}・、。！？\s]+`)
	japaneseRe = regexp.MustCompile(`[\x{3041}-\x{3096}\x{30A1}-\x{30F6}\x{30FC}\x{4e00}-\x{9fff}\x{3005}]{2,}`)
)

func cleanTitle(title string) []string {
	cleaned := symbolRe.ReplaceAllString(title, " ")
	cleaned = strings.TrimSpace(cleaned)
	return japaneseRe.FindAllString(cleaned, -1)
}

// findSimilarPrograms はキーワードに類似する番組を検索して平均評価順に返す。
func (s *RecommendationService) findSimilarPrograms(keywords []string) ([]map[string]interface{}, error) {
	seen := make(map[int64]bool)
	type programEntry struct {
		id        int64
		title     string
		stationID string
		cast      *string
		avgRating float64
		reviews   int
	}
	var entries []programEntry

	for _, kw := range keywords {
		programs, err := s.programRepo.SearchByTitle(kw, recommendationLimit*2, 0)
		if err != nil {
			log.Printf("RecommendationService.findSimilarPrograms SearchByTitle(%q): %v", kw, err)
			continue
		}
		for _, prog := range programs {
			if seen[prog.ID] {
				continue
			}
			seen[prog.ID] = true

			avgRating, err := s.postRepo.AverageRating(prog.ID)
			if err != nil {
				avgRating = 0
			}
			posts, err := s.postRepo.FindByProgram(prog.StationID, prog.Title, 100, 0)
			reviewCount := 0
			if err == nil {
				reviewCount = len(posts)
			}

			entries = append(entries, programEntry{
				id:        prog.ID,
				title:     prog.Title,
				stationID: prog.StationID,
				cast:      prog.Cast,
				avgRating: avgRating,
				reviews:   reviewCount,
			})
		}
		if len(seen) >= recommendationLimit {
			break
		}
	}

	// 平均評価降順、レビュー数降順でソート
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].avgRating != entries[j].avgRating {
			return entries[i].avgRating > entries[j].avgRating
		}
		return entries[i].reviews > entries[j].reviews
	})

	if len(entries) > recommendationLimit {
		entries = entries[:recommendationLimit]
	}

	result := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		m := map[string]interface{}{
			"id":            e.id,
			"title":         e.title,
			"station_id":    e.stationID,
			"avg_rating":    fmt.Sprintf("%.1f", e.avgRating),
			"reviews_count": e.reviews,
		}
		if e.cast != nil {
			m["cast"] = *e.cast
		}
		result = append(result, m)
	}
	return result, nil
}

// getPopularPrograms は全期間で評価の高い人気番組を返す。
func (s *RecommendationService) getPopularPrograms(limit int) ([]map[string]interface{}, error) {
	allPrograms, err := s.programRepo.FindAll(200, 0)
	if err != nil {
		return nil, fmt.Errorf("RecommendationService.getPopularPrograms FindAll: %w", err)
	}

	type popularEntry struct {
		id        int64
		title     string
		stationID string
		cast      *string
		avgRating float64
		reviews   int
	}

	var entries []popularEntry
	for _, prog := range allPrograms {
		posts, err := s.postRepo.FindByProgram(prog.StationID, prog.Title, 100, 0)
		if err != nil || len(posts) < 1 {
			continue
		}
		var sum float64
		for _, p := range posts {
			sum += p.Rating
		}
		avg := sum / float64(len(posts))

		entries = append(entries, popularEntry{
			id:        prog.ID,
			title:     prog.Title,
			stationID: prog.StationID,
			cast:      prog.Cast,
			avgRating: avg,
			reviews:   len(posts),
		})
	}

	// 平均評価降順、レビュー数降順でソート
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].avgRating != entries[j].avgRating {
			return entries[i].avgRating > entries[j].avgRating
		}
		return entries[i].reviews > entries[j].reviews
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	result := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		m := map[string]interface{}{
			"id":            e.id,
			"title":         e.title,
			"station_id":    e.stationID,
			"avg_rating":    fmt.Sprintf("%.1f", e.avgRating),
			"reviews_count": e.reviews,
		}
		if e.cast != nil {
			m["cast"] = *e.cast
		}
		result = append(result, m)
	}

	log.Printf("RecommendationService.getPopularPrograms: count=%d", len(result))
	return result, nil
}
