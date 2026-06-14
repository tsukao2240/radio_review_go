package model

import "time"

// PasswordReset - password_resets テーブル
type PasswordReset struct {
	Email     string    `db:"email"`
	Token     string    `db:"token"`
	CreatedAt time.Time `db:"created_at"`
}

// User - users テーブル
type User struct {
	ID              int64      `db:"id"`
	Name            string     `db:"name"`
	Email           string     `db:"email"`
	EmailVerifiedAt *time.Time `db:"email_verified_at"`
	Password        string     `db:"password"`
	RememberToken   *string    `db:"remember_token"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
}

// Post - posts テーブル（レビュー）
type Post struct {
	ID            int64     `db:"id"`
	UserID        int64     `db:"user_id"`
	ProgramID     int64     `db:"program_id"`
	ProgramTitle  string    `db:"program_title"`
	StationID     *string   `db:"station_id"`
	Title         string    `db:"title"`
	Body          string    `db:"body"`
	Rating        float64   `db:"rating"` // 1.0~5.0, default 3.0
	LikesCount    int       `db:"likes_count"`
	CommentsCount int       `db:"comments_count"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// RadioProgram - radio_programs テーブル（Radikoキャッシュ）
// PHPモデルは timestamps=false だが実テーブルにはカラムが存在する
type RadioProgram struct {
	ID        int64      `db:"id"`
	StationID string     `db:"station_id"`
	Title     string     `db:"title"`
	Cast      string     `db:"cast"`  // NOT NULL DEFAULT ''
	Start     string     `db:"start"` // HH:MM 形式
	End       string     `db:"end"`   // HH:MM 形式
	Info      *string    `db:"info"`
	URL       *string    `db:"url"`
	Image     *string    `db:"image"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// FavoriteProgram - favorite_programs テーブル
type FavoriteProgram struct {
	ID           int64     `db:"id"`
	UserID       int64     `db:"user_id"`
	StationID    string    `db:"station_id"`
	ProgramTitle string    `db:"program_title"`
	BroadcastDay *int      `db:"broadcast_day"` // 0=月~6=日, NULL可
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// RecordingSchedule - recording_schedules テーブル
type RecordingSchedule struct {
	ID                 int64     `db:"id"`
	UserID             int64     `db:"user_id"`
	StationID          string    `db:"station_id"`
	ProgramTitle       string    `db:"program_title"`
	ScheduledStartTime time.Time `db:"scheduled_start_time"`
	ScheduledEndTime   time.Time `db:"scheduled_end_time"`
	Status             string    `db:"status"` // pending/recording/completed/failed/cancelled
	RecordingID        *string   `db:"recording_id"`
	ErrorMessage       *string   `db:"error_message"`
	RetryCount         int       `db:"retry_count"`
	IsRecurring        bool      `db:"is_recurring"`       // 定期録音フラグ
	RecurrenceType     *string   `db:"recurrence_type"`    // "weekly" など
	ParentScheduleID   *int64    `db:"parent_schedule_id"` // 前回スケジュールのID
	CreatedAt          time.Time `db:"created_at"`
	UpdatedAt          time.Time `db:"updated_at"`
}

// PostTag - post_tags テーブル
type PostTag struct {
	ID           int64  `db:"id"`
	Name         string `db:"name"`
	DisplayOrder int    `db:"display_order"`
}

// PostLike - post_likes テーブル
type PostLike struct {
	ID        int64     `db:"id"`
	PostID    int64     `db:"post_id"`
	UserID    int64     `db:"user_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// PostComment - post_comments テーブル
type PostComment struct {
	ID        int64     `db:"id"`
	PostID    int64     `db:"post_id"`
	UserID    int64     `db:"user_id"`
	Body      string    `db:"body"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// Notification - notifications テーブル
type Notification struct {
	ID        int64      `db:"id"`
	UserID    int64      `db:"user_id"`
	Type      string     `db:"type"`
	Title     string     `db:"title"`
	Message   string     `db:"message"`
	Data      *string    `db:"data"` // JSON文字列
	IsRead    bool       `db:"is_read"`
	ReadAt    *time.Time `db:"read_at"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt time.Time  `db:"updated_at"`
}

// PushSubscription - push_subscriptions テーブル
type PushSubscription struct {
	ID           int64     `db:"id"`
	UserID       int64     `db:"user_id"`
	Endpoint     string    `db:"endpoint"`
	EndpointHash string    `db:"endpoint_hash"`
	P256dh       string    `db:"p256dh"`
	Auth         string    `db:"auth"`
	UserAgent    *string   `db:"user_agent"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// ProgramSummary はレコメンド/トレンド集計用の軽量番組情報。
// radio_programs と posts の JOIN 集計結果をマッピングする。
type ProgramSummary struct {
	ID              int64   `db:"id"`
	StationID       string  `db:"station_id"`
	Title           string  `db:"title"`
	Cast            string  `db:"cast"`
	AvgRating       float64 `db:"avg_rating"`
	ReviewsCount    int     `db:"reviews_count"`
	RecentHighCount int     `db:"recent_high_count"`
}

// RecordingInfo - Redisに保存する録音情報（JSONシリアライズ）
type RecordingInfo struct {
	RecordingID string `json:"recording_id"`
	StationID   string `json:"station_id"`
	ProgramName string `json:"program_name"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Status      string `json:"status"` // recording/completed/failed/stopped
	FilePath    string `json:"file_path"`
	FailReason  string `json:"fail_reason,omitempty"`
	// OwnerKey: ログイン済みは "user_{id}"、ゲストは "session_{id}"
	OwnerKey  string `json:"owner_key"`
	CreatedAt string `json:"created_at"`
}
