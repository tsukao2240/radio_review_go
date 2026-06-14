package repository

import (
	"time"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

// UserRepositoryInterface
type UserRepositoryInterface interface {
	FindByID(id int64) (*model.User, error)
	FindByEmail(email string) (*model.User, error)
	FindByFeedToken(token string) (*model.User, error)
	Create(user *model.User) (int64, error)
	Update(user *model.User) error
}

// PostRepositoryInterface
type PostRepositoryInterface interface {
	FindAll(limit, offset int) ([]model.Post, error)
	Count() (int, error)
	FindByProgram(stationID, programTitle string, limit, offset int) ([]model.Post, error)
	CountByProgram(stationID, programTitle string) (int, error)
	FindByUser(userID int64, limit, offset int) ([]model.Post, error)
	CountByUser(userID int64) (int, error)
	FindByID(id int64) (*model.Post, error)
	FindFiltered(filters map[string]interface{}, limit, offset int) ([]model.Post, error)
	CountFiltered(filters map[string]interface{}) (int, error)
	Create(post *model.Post) (int64, error)
	Update(post *model.Post) error
	Delete(id int64) error
	AverageRating(programID int64) (float64, error)
}

// RadioProgramRepositoryInterface
type RadioProgramRepositoryInterface interface {
	FindByID(id int64) (*model.RadioProgram, error)
	FindByStationAndTitle(stationID, title string) (*model.RadioProgram, error)
	SearchByTitle(keyword string, limit, offset int) ([]model.RadioProgram, error)
	SearchByCast(cast string, limit, offset int) ([]model.RadioProgram, error)
	CountByTitle(keyword string) (int, error)
	FindAll(limit, offset int) ([]model.RadioProgram, error)
	CountAll() (int, error)
	Upsert(program *model.RadioProgram) (int64, error)
	// FindPopularSummary は minReviews 件以上のレビューを持つ番組を
	// 平均評価降順で返す（JOIN集計、N+1を回避）。
	FindPopularSummary(minReviews, limit int) ([]model.ProgramSummary, error)
	FindSummaryByIDs(ids []int64) ([]model.ProgramSummary, error)
	// FindTrendingSummary は cutoff 以降に評価4.0以上のレビューが1件以上ある
	// 番組を最近の高評価数降順で返す（JOIN集計、N+1を回避）。
	FindTrendingSummary(cutoff time.Time, limit int) ([]model.ProgramSummary, error)
}

// FavoriteProgramRepositoryInterface
type FavoriteProgramRepositoryInterface interface {
	FindByUser(userID int64) ([]model.FavoriteProgram, error)
	Exists(userID int64, stationID, programTitle string, broadcastDay *int) (bool, error)
	Create(fav *model.FavoriteProgram) (int64, error)
	Delete(userID int64, stationID, programTitle string, broadcastDay *int) error
}

// RecordingScheduleRepositoryInterface
type RecordingScheduleRepositoryInterface interface {
	FindByUser(userID int64) ([]model.RecordingSchedule, error)
	FindByID(id int64) (*model.RecordingSchedule, error)
	FindPendingBefore(t string) ([]model.RecordingSchedule, error)
	Create(s *model.RecordingSchedule) (int64, error)
	UpdateStatus(id int64, status string, errMsg *string) error
	IncrementRetryCount(id int64, errMsg *string) error
	SetRecordingID(id int64, recordingID string) error
	Cancel(id int64) error
}

// PostTagRepositoryInterface
type PostTagRepositoryInterface interface {
	FindAll() ([]model.PostTag, error)
	FindByID(id int64) (*model.PostTag, error)
	FindByPostID(postID int64) ([]model.PostTag, error)
	AttachToPost(postID, tagID int64) error
	DetachFromPost(postID, tagID int64) error
}

// PostLikeRepositoryInterface
type PostLikeRepositoryInterface interface {
	Exists(postID, userID int64) (bool, error)
	Create(postID, userID int64) error
	Delete(postID, userID int64) error
	CountByPost(postID int64) (int, error)
}

// PostCommentRepositoryInterface
type PostCommentRepositoryInterface interface {
	FindByPost(postID int64) ([]model.PostComment, error)
	FindByID(id int64) (*model.PostComment, error)
	Create(comment *model.PostComment) (int64, error)
	Delete(id int64) error
}

// NotificationRepositoryInterface
type NotificationRepositoryInterface interface {
	FindUnreadByUser(userID int64) ([]model.Notification, error)
	FindAllByUser(userID int64) ([]model.Notification, error)
	Create(notification *model.Notification) (int64, error)
	MarkAsRead(id, userID int64) error
	MarkAllAsRead(userID int64) error
}

// PushSubscriptionRepositoryInterface
type PushSubscriptionRepositoryInterface interface {
	FindByUser(userID int64) ([]model.PushSubscription, error)
	Upsert(subscription *model.PushSubscription) error
	Delete(userID int64, endpoint string) error
}

// PasswordResetRepositoryInterface
type PasswordResetRepositoryInterface interface {
	// Save はトークンを保存する（既存レコードは上書き）。
	Save(email, token string) error
	// FindByEmail は最新のトークンレコードを返す。
	FindByEmail(email string) (*model.PasswordReset, error)
	// Delete は指定メールアドレスのトークンを削除する。
	Delete(email string) error
}
