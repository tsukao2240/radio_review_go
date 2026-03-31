package service

import "github.com/yourname/radio_review_go/internal/model"

// RadikoApiServiceInterface - Radiko APIとの連携
type RadikoApiServiceInterface interface {
	// GetWeeklySchedule は指定放送局の週間番組表を返す（キャッシュTTL: 30分）
	GetWeeklySchedule(stationID string) ([]map[string]interface{}, error)
	// GetTwoWeekSchedule は2週間分の番組表を返す（タイムフリー録音用）
	GetTwoWeekSchedule(stationID string) ([]map[string]interface{}, error)
	// GetCurrentPrograms は現在放送中の番組一覧を返す
	GetCurrentPrograms() ([]map[string]interface{}, error)
	// GetProgramDetails は番組詳細を返す
	GetProgramDetails(stationID, title string) (map[string]interface{}, error)
}

// PostServiceInterface - レビュー投稿のビジネスロジック
type PostServiceInterface interface {
	GetAllPosts(perPage, page int) ([]model.Post, int, error)
	GetPostsByProgram(stationID, programTitle string, perPage, page int) ([]model.Post, int, error)
	GetPostsByUser(userID int64, perPage, page int) ([]model.Post, int, error)
	CreatePost(data map[string]interface{}, userID int64) (*model.Post, error)
	UpdatePost(postID int64, data map[string]interface{}) error
	DeletePost(postID int64) error
	GetPostByID(postID int64) (*model.Post, error)
	GetPostsFiltered(filters map[string]interface{}, perPage, page int) ([]model.Post, int, error)
	GetAverageRatingByProgram(programID int64) (float64, error)
	GetAllTags() ([]model.PostTag, error)
}

// PostInteractionServiceInterface - いいね・コメント操作
type PostInteractionServiceInterface interface {
	Like(postID, userID int64) error
	Unlike(postID, userID int64) error
	AddComment(postID, userID int64, body string) (*model.PostComment, error)
	DeleteComment(commentID, userID int64) error
	GetComments(postID int64) ([]model.PostComment, error)
	IsLikedBy(postID, userID int64) (bool, error)
}

// RadioProgramSearchServiceInterface - 番組検索
type RadioProgramSearchServiceInterface interface {
	// SearchByTitle はタイトルで番組を検索（キャッシュTTL: 5分）
	SearchByTitle(keyword string, stationID *string) ([]model.RadioProgram, error)
	SearchByCast(cast string, stationID *string) ([]model.RadioProgram, error)
	SearchProgramsWithPosts(keyword string, perPage, page int) ([]model.RadioProgram, int, error)
	SearchForAPI(keyword, cast *string, stationID *string, limit int) ([]model.RadioProgram, error)
	GetAllPrograms(perPage, page int) ([]model.RadioProgram, int, error)
}

// FavoriteServiceInterface - お気に入り番組管理
type FavoriteServiceInterface interface {
	GetByUser(userID int64) ([]model.FavoriteProgram, error)
	Add(userID int64, stationID, programTitle string, broadcastDay *int) (*model.FavoriteProgram, error)
	Remove(userID int64, stationID, programTitle string, broadcastDay *int) error
	Check(userID int64, stationID, programTitle string, broadcastDay *int) (bool, error)
}

// NotificationServiceInterface - 通知管理
type NotificationServiceInterface interface {
	GetUnread(userID int64) ([]model.Notification, error)
	GetAll(userID int64) ([]model.Notification, error)
	MarkAsRead(notificationID, userID int64) error
	MarkAllAsRead(userID int64) error
}

// RecommendationServiceInterface - レコメンデーション機能
type RecommendationServiceInterface interface {
	// GetRecommendations はユーザーへのパーソナライズされたレコメンデーションを返す（Redisキャッシュ30分）。
	GetRecommendations(userID int64) ([]map[string]interface{}, error)
	// GetTrendingPrograms は直近 days 日間で高評価レビューが多いトレンド番組を返す。
	GetTrendingPrograms(days, limit int) ([]map[string]interface{}, error)
	// ClearUserCache はユーザーのレコメンデーションキャッシュを削除する。
	ClearUserCache(userID int64) error
}

// RecordingScheduleServiceInterface - 録音予約管理
type RecordingScheduleServiceInterface interface {
	GetByUser(userID int64) ([]model.RecordingSchedule, error)
	// Schedule は録音予約を作成する。isRecurring=true かつ recurrenceType="weekly" の場合は定期録音。
	Schedule(userID int64, stationID, programTitle string, startTime, endTime string, isRecurring bool, recurrenceType string) (*model.RecordingSchedule, error)
	Cancel(scheduleID, userID int64) error
}
