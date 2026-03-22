package service

import (
	"errors"
	"fmt"

	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/repository"
)

// PostService は PostServiceInterface を実装する。
type PostService struct {
	postRepo    repository.PostRepositoryInterface
	programRepo repository.RadioProgramRepositoryInterface
	tagRepo     repository.PostTagRepositoryInterface
}

// NewPostService は新しい PostService を返す。
func NewPostService(
	postRepo repository.PostRepositoryInterface,
	programRepo repository.RadioProgramRepositoryInterface,
	tagRepo repository.PostTagRepositoryInterface,
) *PostService {
	return &PostService{
		postRepo:    postRepo,
		programRepo: programRepo,
		tagRepo:     tagRepo,
	}
}

// GetAllPosts はページネーション付きで全投稿を返す。
func (s *PostService) GetAllPosts(perPage, page int) ([]model.Post, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	posts, err := s.postRepo.FindAll(perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetAllPosts: %w", err)
	}

	total, err := s.postRepo.Count()
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetAllPosts count: %w", err)
	}

	return posts, total, nil
}

// GetPostsByProgram は放送局IDと番組タイトルで絞り込んだ投稿一覧を返す。
func (s *PostService) GetPostsByProgram(stationID, programTitle string, perPage, page int) ([]model.Post, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	posts, err := s.postRepo.FindByProgram(stationID, programTitle, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsByProgram: %w", err)
	}

	total, err := s.postRepo.CountByProgram(stationID, programTitle)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsByProgram count: %w", err)
	}

	return posts, total, nil
}

// GetPostsByUser はユーザーIDで絞り込んだ投稿一覧を返す。
func (s *PostService) GetPostsByUser(userID int64, perPage, page int) ([]model.Post, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	posts, err := s.postRepo.FindByUser(userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsByUser: %w", err)
	}

	total, err := s.postRepo.CountByUser(userID)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsByUser count: %w", err)
	}

	return posts, total, nil
}

// CreatePost は data map から Post 構造体を組み立てて保存する。
// program_id は radio_programs テーブルを stationID + programTitle で検索し、
// 存在しなければ Upsert する。
func (s *PostService) CreatePost(data map[string]interface{}, userID int64) (*model.Post, error) {
	stationID, _ := data["station_id"].(string)
	programTitle, _ := data["program_title"].(string)

	// radio_programs の program_id を解決
	var programID int64
	program, err := s.programRepo.FindByStationAndTitle(stationID, programTitle)
	if err != nil {
		// 見つからない場合は新規作成
		newProgram := &model.RadioProgram{
			StationID: stationID,
			Title:     programTitle,
		}
		id, upsertErr := s.programRepo.Upsert(newProgram)
		if upsertErr != nil {
			return nil, fmt.Errorf("PostService.CreatePost upsert program: %w", upsertErr)
		}
		programID = id
	} else {
		programID = program.ID
	}

	title, _ := data["title"].(string)
	body, _ := data["body"].(string)

	var rating float64 = 3.0
	switch v := data["rating"].(type) {
	case float64:
		rating = v
	case float32:
		rating = float64(v)
	case int:
		rating = float64(v)
	case int64:
		rating = float64(v)
	}

	post := &model.Post{
		UserID:       userID,
		ProgramID:    programID,
		ProgramTitle: programTitle,
		Title:        title,
		Body:         body,
		Rating:       rating,
	}

	if stationID != "" {
		post.StationID = &stationID
	}

	postID, err := s.postRepo.Create(post)
	if err != nil {
		return nil, fmt.Errorf("PostService.CreatePost: %w", err)
	}
	post.ID = postID

	// タグの付与
	if tagIDsRaw, ok := data["tag_ids"]; ok {
		switch v := tagIDsRaw.(type) {
		case []int64:
			for _, tagID := range v {
				_ = s.tagRepo.AttachToPost(postID, tagID)
			}
		case []interface{}:
			for _, tagRaw := range v {
				switch tid := tagRaw.(type) {
				case int64:
					_ = s.tagRepo.AttachToPost(postID, tid)
				case float64:
					_ = s.tagRepo.AttachToPost(postID, int64(tid))
				}
			}
		}
	}

	return post, nil
}

// UpdatePost は自分の投稿のみ更新可能（postID で取得して userID を確認）。
func (s *PostService) UpdatePost(postID int64, data map[string]interface{}) error {
	post, err := s.postRepo.FindByID(postID)
	if err != nil {
		return fmt.Errorf("PostService.UpdatePost find: %w", err)
	}

	// userID チェック
	if userID, ok := data["user_id"]; ok {
		var uid int64
		switch v := userID.(type) {
		case int64:
			uid = v
		case float64:
			uid = int64(v)
		case int:
			uid = int64(v)
		}
		if uid != 0 && post.UserID != uid {
			return errors.New("unauthorized: cannot update other user's post")
		}
	}

	if title, ok := data["title"].(string); ok {
		post.Title = title
	}
	if body, ok := data["body"].(string); ok {
		post.Body = body
	}
	switch v := data["rating"].(type) {
	case float64:
		post.Rating = v
	case float32:
		post.Rating = float64(v)
	case int:
		post.Rating = float64(v)
	case int64:
		post.Rating = float64(v)
	}

	if err := s.postRepo.Update(post); err != nil {
		return fmt.Errorf("PostService.UpdatePost: %w", err)
	}

	// タグの再付与（渡された場合のみ）
	if tagIDsRaw, ok := data["tag_ids"]; ok {
		// 既存タグを全削除してから再付与
		existingTags, _ := s.tagRepo.FindByPostID(postID)
		for _, t := range existingTags {
			_ = s.tagRepo.DetachFromPost(postID, t.ID)
		}

		switch v := tagIDsRaw.(type) {
		case []int64:
			for _, tagID := range v {
				_ = s.tagRepo.AttachToPost(postID, tagID)
			}
		case []interface{}:
			for _, tagRaw := range v {
				switch tid := tagRaw.(type) {
				case int64:
					_ = s.tagRepo.AttachToPost(postID, tid)
				case float64:
					_ = s.tagRepo.AttachToPost(postID, int64(tid))
				}
			}
		}
	}

	return nil
}

// DeletePost は投稿を削除する。
func (s *PostService) DeletePost(postID int64) error {
	if err := s.postRepo.Delete(postID); err != nil {
		return fmt.Errorf("PostService.DeletePost: %w", err)
	}
	return nil
}

// GetPostByID は投稿をIDで取得する。
func (s *PostService) GetPostByID(postID int64) (*model.Post, error) {
	post, err := s.postRepo.FindByID(postID)
	if err != nil {
		return nil, fmt.Errorf("PostService.GetPostByID: %w", err)
	}
	return post, nil
}

// GetPostsFiltered はフィルタ条件付きで投稿一覧を返す。
// filters: station_id, tag_id, min_rating, keyword, sort (popular/latest)
func (s *PostService) GetPostsFiltered(filters map[string]interface{}, perPage, page int) ([]model.Post, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	posts, err := s.postRepo.FindFiltered(filters, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsFiltered: %w", err)
	}

	total, err := s.postRepo.CountFiltered(filters)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsFiltered count: %w", err)
	}

	return posts, total, nil
}

// GetAverageRatingByProgram は番組IDに対する平均評価を返す。
func (s *PostService) GetAverageRatingByProgram(programID int64) (float64, error) {
	avg, err := s.postRepo.AverageRating(programID)
	if err != nil {
		return 0, fmt.Errorf("PostService.GetAverageRatingByProgram: %w", err)
	}
	return avg, nil
}

// GetAllTags は全タグ一覧を返す。
func (s *PostService) GetAllTags() ([]model.PostTag, error) {
	tags, err := s.tagRepo.FindAll()
	if err != nil {
		return nil, fmt.Errorf("PostService.GetAllTags: %w", err)
	}
	return tags, nil
}
