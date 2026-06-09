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
	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/repository"
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
func (s *RecommendationService) GetTrendingPrograms(days, limit int) ([]map[string]interface{}, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	summaries, err := s.programRepo.FindTrendingSummary(cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("RecommendationService.GetTrendingPrograms: %w", err)
	}
	result := make([]map[string]interface{}, 0, len(summaries))
	for _, e := range summaries {
		m := map[string]interface{}{
			"id":                   e.ID,
			"title":                e.Title,
			"station_id":           e.StationID,
			"avg_rating":           fmt.Sprintf("%.1f", e.AvgRating),
			"recent_reviews_count": e.RecentHighCount,
		}
		if e.Cast != "" {
			m["cast"] = e.Cast
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
			wordCount[word] += 2 // お気に入りは2倍の重み
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
	var candidateIDs []int64

	for _, kw := range keywords {
		programs, err := s.programRepo.SearchByTitle(kw, recommendationLimit*3, 0)
		if err != nil {
			log.Printf("RecommendationService.findSimilarPrograms SearchByTitle(%q): %v", kw, err)
			continue
		}
		for _, prog := range programs {
			if !seen[prog.ID] {
				seen[prog.ID] = true
				candidateIDs = append(candidateIDs, prog.ID)
			}
		}
	}

	if len(candidateIDs) == 0 {
		return nil, nil
	}

	summaries, err := s.programRepo.FindSummaryByIDs(candidateIDs)
	if err != nil {
		return nil, fmt.Errorf("findSimilarPrograms FindSummaryByIDs: %w", err)
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].AvgRating != summaries[j].AvgRating {
			return summaries[i].AvgRating > summaries[j].AvgRating
		}
		return summaries[i].ReviewsCount > summaries[j].ReviewsCount
	})

	// ステーション多様性: 各ステーション最大2件
	stationCount := make(map[string]int)
	var diverse []model.ProgramSummary
	for _, e := range summaries {
		if stationCount[e.StationID] < 2 {
			diverse = append(diverse, e)
			stationCount[e.StationID]++
		}
		if len(diverse) >= recommendationLimit {
			break
		}
	}

	result := make([]map[string]interface{}, 0, len(diverse))
	for _, e := range diverse {
		m := map[string]interface{}{
			"id":            e.ID,
			"title":         e.Title,
			"station_id":    e.StationID,
			"avg_rating":    fmt.Sprintf("%.1f", e.AvgRating),
			"reviews_count": e.ReviewsCount,
		}
		if e.Cast != "" {
			m["cast"] = e.Cast
		}
		result = append(result, m)
	}
	return result, nil
}

// getPopularPrograms は全期間で評価の高い人気番組を返す。
func (s *RecommendationService) getPopularPrograms(limit int) ([]map[string]interface{}, error) {
	summaries, err := s.programRepo.FindPopularSummary(1, limit)
	if err != nil {
		return nil, fmt.Errorf("RecommendationService.getPopularPrograms: %w", err)
	}
	result := make([]map[string]interface{}, 0, len(summaries))
	for _, e := range summaries {
		m := map[string]interface{}{
			"id":            e.ID,
			"title":         e.Title,
			"station_id":    e.StationID,
			"avg_rating":    fmt.Sprintf("%.1f", e.AvgRating),
			"reviews_count": e.ReviewsCount,
		}
		if e.Cast != "" {
			m["cast"] = e.Cast
		}
		result = append(result, m)
	}
	log.Printf("RecommendationService.getPopularPrograms: count=%d", len(result))
	return result, nil
}
