package service

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/repository"
)

// RadioProgramSearchService は RadioProgramSearchServiceInterface の実装
type RadioProgramSearchService struct {
	repo  repository.RadioProgramRepositoryInterface
	redis *redis.Client
}

// NewRadioProgramSearchService はコンストラクタ
func NewRadioProgramSearchService(repo repository.RadioProgramRepositoryInterface, rdb *redis.Client) *RadioProgramSearchService {
	return &RadioProgramSearchService{
		repo:  repo,
		redis: rdb,
	}
}

// ---- ヘルパ ----

// keywordMD5 は検索キーワードの MD5 ハッシュを16進数文字列で返す
func keywordMD5(s string) string {
	h := md5.New() //nolint:gosec
	if _, err := h.Write([]byte(s)); err != nil {
		log.Printf("keywordMD5 write error: %v", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// searchCacheGet は Redis から []model.RadioProgram を取得する
func searchCacheGet(ctx context.Context, rdb *redis.Client, key string, dest *[]model.RadioProgram) bool {
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(val), dest); err != nil {
		return false
	}
	return true
}

// searchCacheSet は Redis に []model.RadioProgram を保存する
func searchCacheSet(ctx context.Context, rdb *redis.Client, key string, val interface{}, ttl time.Duration) {
	b, err := json.Marshal(val)
	if err != nil {
		log.Printf("searchCacheSet marshal error: %v", err)
		return
	}
	if err := rdb.Set(ctx, key, string(b), ttl).Err(); err != nil {
		log.Printf("searchCacheSet redis error: %v", err)
	}
}

// ---- インタフェース実装 ----

// SearchByTitle はタイトルで番組を検索する（キャッシュ 5 分）
func (s *RadioProgramSearchService) SearchByTitle(keyword string, stationID *string) ([]model.RadioProgram, error) {
	ctx := context.Background()
	cacheKey := "search_programs_" + keywordMD5(keyword)
	if stationID != nil {
		cacheKey += "_" + *stationID
	}

	var cached []model.RadioProgram
	if searchCacheGet(ctx, s.redis, cacheKey, &cached) {
		return cached, nil
	}

	// 件数上限なしで全件取得（limit=1000, offset=0 を上限として使用）
	const maxLimit = 1000
	programs, err := s.repo.SearchByTitle(keyword, maxLimit, 0)
	if err != nil {
		return nil, fmt.Errorf("SearchByTitle repo: %w", err)
	}

	// stationID フィルタ
	if stationID != nil && *stationID != "" {
		filtered := programs[:0]
		for _, p := range programs {
			if p.StationID == *stationID {
				filtered = append(filtered, p)
			}
		}
		programs = filtered
	}

	log.Printf("SearchByTitle: keyword=%s stationID=%v count=%d", keyword, stationID, len(programs))
	searchCacheSet(ctx, s.redis, cacheKey, programs, 5*time.Minute)
	return programs, nil
}

// SearchByCast は出演者で番組を検索する（キャッシュ 5 分）
func (s *RadioProgramSearchService) SearchByCast(cast string, stationID *string) ([]model.RadioProgram, error) {
	ctx := context.Background()
	cacheKey := "search_programs_cast_" + keywordMD5(cast)
	if stationID != nil {
		cacheKey += "_" + *stationID
	}

	var cached []model.RadioProgram
	if searchCacheGet(ctx, s.redis, cacheKey, &cached) {
		return cached, nil
	}

	const maxLimit = 1000
	programs, err := s.repo.SearchByCast(cast, maxLimit, 0)
	if err != nil {
		return nil, fmt.Errorf("SearchByCast repo: %w", err)
	}

	if stationID != nil && *stationID != "" {
		filtered := programs[:0]
		for _, p := range programs {
			if p.StationID == *stationID {
				filtered = append(filtered, p)
			}
		}
		programs = filtered
	}

	log.Printf("SearchByCast: cast=%s stationID=%v count=%d", cast, stationID, len(programs))
	searchCacheSet(ctx, s.redis, cacheKey, programs, 5*time.Minute)
	return programs, nil
}

// SearchProgramsWithPosts はタイトル・出演者でユニオン検索し、ページネーションを返す
func (s *RadioProgramSearchService) SearchProgramsWithPosts(keyword string, perPage, page int) ([]model.RadioProgram, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	// タイトル検索
	byTitle, err := s.repo.SearchByTitle(keyword, 0, 0) // 全件取得
	if err != nil {
		return nil, 0, fmt.Errorf("SearchProgramsWithPosts SearchByTitle: %w", err)
	}

	// 出演者検索
	byCast, err := s.repo.SearchByCast(keyword, 0, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("SearchProgramsWithPosts SearchByCast: %w", err)
	}

	// ユニオン（重複 ID 排除）
	seen := map[int64]struct{}{}
	var all []model.RadioProgram
	for _, p := range byTitle {
		if _, ok := seen[p.ID]; !ok {
			seen[p.ID] = struct{}{}
			all = append(all, p)
		}
	}
	for _, p := range byCast {
		if _, ok := seen[p.ID]; !ok {
			seen[p.ID] = struct{}{}
			all = append(all, p)
		}
	}

	total := len(all)

	// ページネーション
	start := offset
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	log.Printf("SearchProgramsWithPosts: keyword=%s total=%d page=%d", keyword, total, page)
	return all[start:end], total, nil
}

// SearchForAPI は stationID/keyword/cast による複合フィルタ検索を行う
func (s *RadioProgramSearchService) SearchForAPI(keyword, cast *string, stationID *string, limit int) ([]model.RadioProgram, error) {
	if keyword == nil && cast == nil {
		return []model.RadioProgram{}, nil
	}

	seen := map[int64]struct{}{}
	var all []model.RadioProgram

	if keyword != nil && *keyword != "" {
		programs, err := s.repo.SearchByTitle(*keyword, limit, 0)
		if err != nil {
			return nil, fmt.Errorf("SearchForAPI SearchByTitle: %w", err)
		}
		for _, p := range programs {
			if stationID != nil && *stationID != "" && p.StationID != *stationID {
				continue
			}
			if _, ok := seen[p.ID]; !ok {
				seen[p.ID] = struct{}{}
				all = append(all, p)
			}
		}
	}

	if cast != nil && *cast != "" {
		programs, err := s.repo.SearchByCast(*cast, limit, 0)
		if err != nil {
			return nil, fmt.Errorf("SearchForAPI SearchByCast: %w", err)
		}
		for _, p := range programs {
			if stationID != nil && *stationID != "" && p.StationID != *stationID {
				continue
			}
			if _, ok := seen[p.ID]; !ok {
				seen[p.ID] = struct{}{}
				all = append(all, p)
			}
		}
	}

	// limit を適用
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	log.Printf("SearchForAPI: keyword=%v cast=%v stationID=%v count=%d", keyword, cast, stationID, len(all))
	return all, nil
}

// GetAllPrograms は全番組をページネーション付きで返す
func (s *RadioProgramSearchService) GetAllPrograms(perPage, page int) ([]model.RadioProgram, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	total, err := s.repo.CountAll()
	if err != nil {
		return nil, 0, fmt.Errorf("GetAllPrograms CountAll: %w", err)
	}

	programs, err := s.repo.FindAll(perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("GetAllPrograms FindAll: %w", err)
	}

	log.Printf("GetAllPrograms: total=%d page=%d", total, page)
	return programs, total, nil
}
