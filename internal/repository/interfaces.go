package repository

import "github.com/yourname/radio_review_go/internal/model"

// UserRepositoryInterface
type UserRepositoryInterface interface {
	FindByID(id int64) (*model.User, error)
	FindByEmail(email string) (*model.User, error)
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
	MarkAsRead(id, userID int64) error
	MarkAllAsRead(userID int64) error
}
