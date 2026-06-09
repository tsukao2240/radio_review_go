package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/repository"
)

const (
	maxReviewBodyLength = 5000
	maxPostTagCount     = 10
)

type txRunner interface {
	RunInTx(context.Context, func(*sqlx.Tx) error) error
}

type txPostRepository interface {
	FindByIDTx(context.Context, *sqlx.Tx, int64) (*model.Post, error)
	CreateTx(context.Context, *sqlx.Tx, *model.Post) (int64, error)
	UpdateTx(context.Context, *sqlx.Tx, *model.Post) error
}

type txRadioProgramRepository interface {
	FindByStationAndTitleTx(context.Context, *sqlx.Tx, string, string) (*model.RadioProgram, error)
	UpsertTx(context.Context, *sqlx.Tx, *model.RadioProgram) (int64, error)
}

type txPostTagRepository interface {
	FindByPostIDTx(context.Context, *sqlx.Tx, int64) ([]model.PostTag, error)
	AttachToPostTx(context.Context, *sqlx.Tx, int64, int64) error
	DetachFromPostTx(context.Context, *sqlx.Tx, int64, int64) error
}

type contextPostRepository interface {
	FindAllContext(context.Context, int, int) ([]model.Post, error)
	CountContext(context.Context) (int, error)
	FindByProgramContext(context.Context, string, string, int, int) ([]model.Post, error)
	CountByProgramContext(context.Context, string, string) (int, error)
	FindByUserContext(context.Context, int64, int, int) ([]model.Post, error)
	CountByUserContext(context.Context, int64) (int, error)
	FindByIDContext(context.Context, int64) (*model.Post, error)
	FindFilteredContext(context.Context, map[string]interface{}, int, int) ([]model.Post, error)
	CountFilteredContext(context.Context, map[string]interface{}) (int, error)
	AverageRatingContext(context.Context, int64) (float64, error)
}

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
	return s.GetAllPostsContext(context.Background(), perPage, page)
}

func (s *PostService) GetAllPostsContext(ctx context.Context, perPage, page int) ([]model.Post, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	posts, err := s.findAllPosts(ctx, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetAllPosts: %w", err)
	}

	total, err := s.countPosts(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetAllPosts count: %w", err)
	}

	return posts, total, nil
}

// GetPostsByProgram は放送局IDと番組タイトルで絞り込んだ投稿一覧を返す。
func (s *PostService) GetPostsByProgram(stationID, programTitle string, perPage, page int) ([]model.Post, int, error) {
	return s.GetPostsByProgramContext(context.Background(), stationID, programTitle, perPage, page)
}

func (s *PostService) GetPostsByProgramContext(ctx context.Context, stationID, programTitle string, perPage, page int) ([]model.Post, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	posts, err := s.findPostsByProgram(ctx, stationID, programTitle, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsByProgram: %w", err)
	}

	total, err := s.countPostsByProgram(ctx, stationID, programTitle)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsByProgram count: %w", err)
	}

	return posts, total, nil
}

// GetPostsByUser はユーザーIDで絞り込んだ投稿一覧を返す。
func (s *PostService) GetPostsByUser(userID int64, perPage, page int) ([]model.Post, int, error) {
	return s.GetPostsByUserContext(context.Background(), userID, perPage, page)
}

func (s *PostService) GetPostsByUserContext(ctx context.Context, userID int64, perPage, page int) ([]model.Post, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	posts, err := s.findPostsByUser(ctx, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsByUser: %w", err)
	}

	total, err := s.countPostsByUser(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsByUser count: %w", err)
	}

	return posts, total, nil
}

// CreatePost は data map から Post 構造体を組み立てて保存する。
// program_id は radio_programs テーブルを stationID + programTitle で検索し、
// 存在しなければ Upsert する。
func (s *PostService) CreatePost(data map[string]interface{}, userID int64) (*model.Post, error) {
	return s.CreatePostContext(context.Background(), data, userID)
}

func (s *PostService) CreatePostContext(ctx context.Context, data map[string]interface{}, userID int64) (*model.Post, error) {
	if err := validatePostInput(data); err != nil {
		return nil, err
	}
	if runner, ok := s.postRepo.(txRunner); ok {
		if postTx, ok := s.postRepo.(txPostRepository); ok {
			if programTx, ok := s.programRepo.(txRadioProgramRepository); ok {
				if tagTx, ok := s.tagRepo.(txPostTagRepository); ok {
					return s.createPostInTx(ctx, runner, postTx, programTx, tagTx, data, userID)
				}
			}
		}
	}
	return s.createPostWithoutTx(ctx, data, userID)
}

func (s *PostService) createPostWithoutTx(ctx context.Context, data map[string]interface{}, userID int64) (*model.Post, error) {
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
	for _, tagID := range extractTagIDs(data) {
		if err := s.tagRepo.AttachToPost(postID, tagID); err != nil {
			return nil, fmt.Errorf("PostService.CreatePost attach tag: %w", err)
		}
	}

	return post, nil
}

// UpdatePost は自分の投稿のみ更新可能（postID で取得して userID を確認）。
func (s *PostService) UpdatePost(postID int64, data map[string]interface{}) error {
	return s.UpdatePostContext(context.Background(), postID, data)
}

func (s *PostService) UpdatePostContext(ctx context.Context, postID int64, data map[string]interface{}) error {
	if err := validatePostInput(data); err != nil {
		return err
	}
	if runner, ok := s.postRepo.(txRunner); ok {
		if postTx, ok := s.postRepo.(txPostRepository); ok {
			if tagTx, ok := s.tagRepo.(txPostTagRepository); ok {
				return s.updatePostInTx(ctx, runner, postTx, tagTx, postID, data)
			}
		}
	}
	return s.updatePostWithoutTx(ctx, postID, data)
}

func (s *PostService) updatePostWithoutTx(ctx context.Context, postID int64, data map[string]interface{}) error {
	post, err := s.findPostByID(ctx, postID)
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
		existingTags, err := s.tagRepo.FindByPostID(postID)
		if err != nil {
			return fmt.Errorf("PostService.UpdatePost find tags: %w", err)
		}
		for _, t := range existingTags {
			if err := s.tagRepo.DetachFromPost(postID, t.ID); err != nil {
				return fmt.Errorf("PostService.UpdatePost detach tag: %w", err)
			}
		}
		for _, tagID := range extractTagIDs(map[string]interface{}{"tag_ids": tagIDsRaw}) {
			if err := s.tagRepo.AttachToPost(postID, tagID); err != nil {
				return fmt.Errorf("PostService.UpdatePost attach tag: %w", err)
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
	post, err := s.findPostByID(context.Background(), postID)
	if err != nil {
		return nil, fmt.Errorf("PostService.GetPostByID: %w", err)
	}
	return post, nil
}

// GetPostsFiltered はフィルタ条件付きで投稿一覧を返す。
// filters: station_id, tag_id, min_rating, keyword, sort (popular/latest)
func (s *PostService) GetPostsFiltered(filters map[string]interface{}, perPage, page int) ([]model.Post, int, error) {
	return s.GetPostsFilteredContext(context.Background(), filters, perPage, page)
}

func (s *PostService) GetPostsFilteredContext(ctx context.Context, filters map[string]interface{}, perPage, page int) ([]model.Post, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	posts, err := s.findFilteredPosts(ctx, filters, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsFiltered: %w", err)
	}

	total, err := s.countFilteredPosts(ctx, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("PostService.GetPostsFiltered count: %w", err)
	}

	return posts, total, nil
}

// GetAverageRatingByProgram は番組IDに対する平均評価を返す。
func (s *PostService) GetAverageRatingByProgram(programID int64) (float64, error) {
	avg, err := s.averageRating(context.Background(), programID)
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

func validatePostInput(data map[string]interface{}) error {
	if body, ok := data["body"].(string); ok && len([]rune(body)) > maxReviewBodyLength {
		return fmt.Errorf("review body must be %d characters or fewer", maxReviewBodyLength)
	}
	if len(extractTagIDs(data)) > maxPostTagCount {
		return fmt.Errorf("tag count must be %d or fewer", maxPostTagCount)
	}
	return nil
}

func extractTagIDs(data map[string]interface{}) []int64 {
	tagIDsRaw, ok := data["tag_ids"]
	if !ok {
		return nil
	}
	var tagIDs []int64
	switch v := tagIDsRaw.(type) {
	case []int64:
		tagIDs = append(tagIDs, v...)
	case []interface{}:
		for _, tagRaw := range v {
			switch tid := tagRaw.(type) {
			case int64:
				tagIDs = append(tagIDs, tid)
			case int:
				tagIDs = append(tagIDs, int64(tid))
			case float64:
				tagIDs = append(tagIDs, int64(tid))
			}
		}
	}
	return tagIDs
}

func (s *PostService) createPostInTx(ctx context.Context, runner txRunner, postTx txPostRepository, programTx txRadioProgramRepository, tagTx txPostTagRepository, data map[string]interface{}, userID int64) (*model.Post, error) {
	var created *model.Post
	err := runner.RunInTx(ctx, func(tx *sqlx.Tx) error {
		stationID, _ := data["station_id"].(string)
		programTitle, _ := data["program_title"].(string)

		var programID int64
		program, err := programTx.FindByStationAndTitleTx(ctx, tx, stationID, programTitle)
		if err != nil {
			id, upsertErr := programTx.UpsertTx(ctx, tx, &model.RadioProgram{StationID: stationID, Title: programTitle})
			if upsertErr != nil {
				return fmt.Errorf("PostService.CreatePost upsert program: %w", upsertErr)
			}
			programID = id
		} else {
			programID = program.ID
		}

		post := buildPostFromData(data, userID, programID)
		postID, err := postTx.CreateTx(ctx, tx, post)
		if err != nil {
			return fmt.Errorf("PostService.CreatePost: %w", err)
		}
		post.ID = postID
		for _, tagID := range extractTagIDs(data) {
			if err := tagTx.AttachToPostTx(ctx, tx, postID, tagID); err != nil {
				return fmt.Errorf("PostService.CreatePost attach tag: %w", err)
			}
		}
		created = post
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *PostService) updatePostInTx(ctx context.Context, runner txRunner, postTx txPostRepository, tagTx txPostTagRepository, postID int64, data map[string]interface{}) error {
	return runner.RunInTx(ctx, func(tx *sqlx.Tx) error {
		post, err := postTx.FindByIDTx(ctx, tx, postID)
		if err != nil {
			return fmt.Errorf("PostService.UpdatePost find: %w", err)
		}
		if err := authorizePostUpdate(post, data); err != nil {
			return err
		}
		applyPostUpdates(post, data)
		if err := postTx.UpdateTx(ctx, tx, post); err != nil {
			return fmt.Errorf("PostService.UpdatePost: %w", err)
		}
		if tagIDsRaw, ok := data["tag_ids"]; ok {
			existingTags, err := tagTx.FindByPostIDTx(ctx, tx, postID)
			if err != nil {
				return fmt.Errorf("PostService.UpdatePost find tags: %w", err)
			}
			for _, t := range existingTags {
				if err := tagTx.DetachFromPostTx(ctx, tx, postID, t.ID); err != nil {
					return fmt.Errorf("PostService.UpdatePost detach tag: %w", err)
				}
			}
			for _, tagID := range extractTagIDs(map[string]interface{}{"tag_ids": tagIDsRaw}) {
				if err := tagTx.AttachToPostTx(ctx, tx, postID, tagID); err != nil {
					return fmt.Errorf("PostService.UpdatePost attach tag: %w", err)
				}
			}
		}
		return nil
	})
}

func buildPostFromData(data map[string]interface{}, userID, programID int64) *model.Post {
	stationID, _ := data["station_id"].(string)
	programTitle, _ := data["program_title"].(string)
	title, _ := data["title"].(string)
	body, _ := data["body"].(string)

	post := &model.Post{
		UserID:       userID,
		ProgramID:    programID,
		ProgramTitle: programTitle,
		Title:        title,
		Body:         body,
		Rating:       ratingFromData(data),
	}
	if stationID != "" {
		post.StationID = &stationID
	}
	return post
}

func ratingFromData(data map[string]interface{}) float64 {
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
	return rating
}

func authorizePostUpdate(post *model.Post, data map[string]interface{}) error {
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
	return nil
}

func applyPostUpdates(post *model.Post, data map[string]interface{}) {
	if title, ok := data["title"].(string); ok {
		post.Title = title
	}
	if body, ok := data["body"].(string); ok {
		post.Body = body
	}
	post.Rating = ratingFromData(map[string]interface{}{"rating": post.Rating})
	if _, ok := data["rating"]; ok {
		post.Rating = ratingFromData(data)
	}
}

func (s *PostService) findAllPosts(ctx context.Context, limit, offset int) ([]model.Post, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.FindAllContext(ctx, limit, offset)
	}
	return s.postRepo.FindAll(limit, offset)
}

func (s *PostService) countPosts(ctx context.Context) (int, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.CountContext(ctx)
	}
	return s.postRepo.Count()
}

func (s *PostService) findPostsByProgram(ctx context.Context, stationID, programTitle string, limit, offset int) ([]model.Post, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.FindByProgramContext(ctx, stationID, programTitle, limit, offset)
	}
	return s.postRepo.FindByProgram(stationID, programTitle, limit, offset)
}

func (s *PostService) countPostsByProgram(ctx context.Context, stationID, programTitle string) (int, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.CountByProgramContext(ctx, stationID, programTitle)
	}
	return s.postRepo.CountByProgram(stationID, programTitle)
}

func (s *PostService) findPostsByUser(ctx context.Context, userID int64, limit, offset int) ([]model.Post, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.FindByUserContext(ctx, userID, limit, offset)
	}
	return s.postRepo.FindByUser(userID, limit, offset)
}

func (s *PostService) countPostsByUser(ctx context.Context, userID int64) (int, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.CountByUserContext(ctx, userID)
	}
	return s.postRepo.CountByUser(userID)
}

func (s *PostService) findPostByID(ctx context.Context, postID int64) (*model.Post, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.FindByIDContext(ctx, postID)
	}
	return s.postRepo.FindByID(postID)
}

func (s *PostService) findFilteredPosts(ctx context.Context, filters map[string]interface{}, limit, offset int) ([]model.Post, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.FindFilteredContext(ctx, filters, limit, offset)
	}
	return s.postRepo.FindFiltered(filters, limit, offset)
}

func (s *PostService) countFilteredPosts(ctx context.Context, filters map[string]interface{}) (int, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.CountFilteredContext(ctx, filters)
	}
	return s.postRepo.CountFiltered(filters)
}

func (s *PostService) averageRating(ctx context.Context, programID int64) (float64, error) {
	if repo, ok := s.postRepo.(contextPostRepository); ok {
		return repo.AverageRatingContext(ctx, programID)
	}
	return s.postRepo.AverageRating(programID)
}
