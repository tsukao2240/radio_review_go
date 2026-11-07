package job

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	appmetrics "github.com/tsukao2240/radio_review_go/internal/metrics"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

const favoriteRecordingInterval = 5 * time.Minute
const timefreeAvailabilityDelay = 5 * time.Minute

var radikoLocation = time.FixedZone("Asia/Tokyo", 9*60*60)

type favoriteScheduleService interface {
	GetTwoWeekSchedule(stationID string) ([]map[string]interface{}, error)
}

// SetFavoriteScheduleService はお気に入り自動録音用の番組表取得処理を設定する。
func (s *Scheduler) SetFavoriteScheduleService(svc favoriteScheduleService) {
	s.favoriteScheduleService = svc
}

func (s *Scheduler) syncFavoriteRecordings(ctx context.Context) {
	if s.favoriteScheduleService == nil {
		return
	}
	appmetrics.IncRecordingJobRun("favorite_recording_sync")
	var favorites []model.FavoriteProgram
	if err := s.db.SelectContext(ctx, &favorites, "SELECT * FROM favorite_programs ORDER BY created_at DESC"); err != nil {
		slog.Error("[job] favorite recording: select favorites failed", "error", err)
		return
	}
	if err := s.cancelRemovedFavoriteRecordings(ctx, favorites); err != nil {
		slog.Error("[job] favorite recording: cancel removed favorites failed", "error", err)
		return
	}

	// ユーザー・番組ごとではなく、放送局ごとに一度だけ番組表を取得する。
	byStation := make(map[string][]model.FavoriteProgram)
	for _, fav := range favorites {
		byStation[fav.StationID] = append(byStation[fav.StationID], fav)
	}
	now := time.Now()
	for stationID, stationFavorites := range byStation {
		if ctx.Err() != nil {
			return
		}
		schedule, err := s.favoriteScheduleService.GetTwoWeekSchedule(stationID)
		if err != nil {
			slog.Error("[job] favorite recording: fetch schedule failed", "station_id", stationID, "error", err)
			continue
		}
		for _, group := range schedule {
			for _, entry := range favoriteScheduleEntries(group["entries"]) {
				for _, fav := range stationFavorites {
					start, end, ok := favoriteRecordingTimes(fav, entry, now)
					if !ok {
						continue
					}
					if err := s.enqueueFavoriteRecording(ctx, fav, start, end); err != nil {
						slog.Error("[job] favorite recording: enqueue failed", "favorite_id", fav.ID, "error", err)
					}
				}
			}
		}
	}
}

// Redis キャッシュから復元した []interface{} と API 直取得の両方を受け付ける。
func favoriteScheduleEntries(value interface{}) []map[string]interface{} {
	switch entries := value.(type) {
	case []map[string]interface{}:
		return entries
	case []interface{}:
		var result []map[string]interface{}
		for _, entry := range entries {
			if m, ok := entry.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	default:
		return nil
	}
}

func favoriteRecordingTimes(fav model.FavoriteProgram, entry map[string]interface{}, now time.Time) (time.Time, time.Time, bool) {
	if entry["title"] != fav.ProgramTitle {
		return time.Time{}, time.Time{}, false
	}
	if stationID, _ := entry["id"].(string); stationID != "" && stationID != fav.StationID {
		return time.Time{}, time.Time{}, false
	}
	ft, _ := entry["ft"].(string)
	to, _ := entry["to"].(string)
	start, startErr := time.ParseInLocation("20060102150405", ft, radikoLocation)
	end, endErr := time.ParseInLocation("20060102150405", to, radikoLocation)
	if startErr != nil || endErr != nil || !start.Before(end) {
		return time.Time{}, time.Time{}, false
	}
	// 終了済みの放送を番組表から確認して予約する。将来の時刻を固定して予約しない。
	// 新規登録より前の放送は対象外。停止中の取り逃しは7日以内なら回収する。
	if end.Before(fav.CreatedAt) || !end.After(now.AddDate(0, 0, -7)) || end.After(now.Add(-timefreeAvailabilityDelay)) {
		return time.Time{}, time.Time{}, false
	}
	if fav.BroadcastDay != nil && *fav.BroadcastDay != (int(start.Weekday())+6)%7 {
		return time.Time{}, time.Time{}, false
	}
	// DB の loc=Local と揃えて保存し、Radiko へ渡す際に日本時間へ戻す。
	return start.In(time.Local), end.In(time.Local), true
}

func favoriteRecordingKey(userID int64, stationID string, start time.Time) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", userID, stationID, start.In(radikoLocation).Format("20060102150405")))))
}

func (s *Scheduler) enqueueFavoriteRecording(ctx context.Context, fav model.FavoriteProgram, start, end time.Time) error {
	// UNIQUE キーは再起動・複数お気に入り・再走査による二重登録を防ぐ。
	// 既存の手動予約も同じ放送開始日時なら優先する。キャンセル済みは復活させない。
	_, err := s.db.ExecContext(ctx, `INSERT INTO recording_schedules
		(user_id, station_id, program_title, scheduled_start_time, scheduled_end_time,
		 status, auto_recording_key, created_at, updated_at)
		SELECT f.user_id, f.station_id, f.program_title, ?, ?, 'pending', ?, NOW(), NOW()
		FROM favorite_programs f
		LEFT JOIN recording_schedules existing
		  ON existing.user_id = f.user_id AND existing.station_id = f.station_id
		 AND existing.scheduled_start_time = ? AND existing.status <> 'failed'
		 AND existing.auto_recording_key IS NULL
		WHERE f.id = ? AND existing.id IS NULL
		ON DUPLICATE KEY UPDATE scheduled_end_time = IF(recording_schedules.status = 'pending',
		  VALUES(scheduled_end_time), recording_schedules.scheduled_end_time)`,
		start, end, favoriteRecordingKey(fav.UserID, fav.StationID, start), start, fav.ID)
	return err
}

func matchesFavoriteSchedule(fav model.FavoriteProgram, sc model.RecordingSchedule) bool {
	day := (int(sc.ScheduledStartTime.In(radikoLocation).Weekday()) + 6) % 7
	return fav.UserID == sc.UserID && fav.StationID == sc.StationID && fav.ProgramTitle == sc.ProgramTitle &&
		(fav.BroadcastDay == nil || *fav.BroadcastDay == day)
}

func (s *Scheduler) cancelRemovedFavoriteRecordings(ctx context.Context, favorites []model.FavoriteProgram) error {
	var pending []model.RecordingSchedule
	if err := s.db.SelectContext(ctx, &pending, `SELECT * FROM recording_schedules
		WHERE auto_recording_key IS NOT NULL AND status = 'pending'`); err != nil {
		return err
	}
	for _, sc := range pending {
		found := false
		for _, fav := range favorites {
			if matchesFavoriteSchedule(fav, sc) {
				found = true
				break
			}
		}
		if !found {
			if _, err := s.db.ExecContext(ctx, `UPDATE recording_schedules
				SET status = 'cancelled', updated_at = NOW() WHERE id = ? AND status = 'pending'`, sc.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// デスクトップで一つのアプリを起動する前提。前回の強制終了時に残った録音を再開する。
// ダウンロードは部分ファイルへの継ぎ足しではなく、同じ放送を最初から再取得する。
func (s *Scheduler) recoverInterruptedFavoriteRecordings(ctx context.Context) error {
	var interrupted []model.RecordingSchedule
	if err := s.db.SelectContext(ctx, &interrupted, `SELECT * FROM recording_schedules
		WHERE auto_recording_key IS NOT NULL AND status = 'recording'`); err != nil {
		return err
	}
	for _, sc := range interrupted {
		const reason = "前回のアプリ終了により中断。起動後に再取得します"
		if sc.RecordingID != nil {
			// 音声取得が終わりDB更新の直前に終了した場合は、完了情報だけ復旧する。
			raw, err := s.redis.Get(ctx, "recording_"+*sc.RecordingID).Result()
			var info model.RecordingInfo
			if err == nil && json.Unmarshal([]byte(raw), &info) == nil && info.Status == "completed" && s.favoriteRecordingFileExists(sc, info) {
				if err := s.completeFavoriteRecording(ctx, sc.ID, &info); err != nil {
					return err
				}
				continue
			}
			_ = updateRedisRecordingStatus(ctx, s.redis, *sc.RecordingID, "failed", reason)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE recording_schedules
			SET status = 'pending', recording_id = NULL, next_retry_at = NULL,
			    error_message = ?, updated_at = NOW()
			WHERE id = ? AND status = 'recording'`, reason, sc.ID); err != nil {
			return err
		}
	}
	return nil
}

// 録音音声は storage に、完了情報はDBに残す。Redisは一覧表示のキャッシュとして復元できる。
func (s *Scheduler) completeFavoriteRecording(ctx context.Context, scheduleID int64, info *model.RecordingInfo) error {
	metadata, err := json.Marshal(info)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE recording_schedules
		SET status = 'completed', recording_id = ?, recording_metadata = ?,
		    error_message = NULL, next_retry_at = NULL, updated_at = NOW() WHERE id = ?`,
		info.RecordingID, string(metadata), scheduleID)
	return err
}

func (s *Scheduler) favoriteRecordingFileExists(sc model.RecordingSchedule, info model.RecordingInfo) bool {
	if info.RecordingID == "" || info.Status != "completed" || info.OwnerKey != fmt.Sprintf("user_%d", sc.UserID) ||
		info.StationID != sc.StationID || !isRecordingFilePathAllowed(s.storagePath, info.FilePath) {
		return false
	}
	stat, err := os.Stat(info.FilePath)
	return err == nil && !stat.IsDir()
}

func (s *Scheduler) restoreFavoriteRecordingMetadata(ctx context.Context) error {
	var completed []model.RecordingSchedule
	if err := s.db.SelectContext(ctx, &completed, `SELECT * FROM recording_schedules
		WHERE auto_recording_key IS NOT NULL AND status = 'completed' AND recording_metadata IS NOT NULL`); err != nil {
		return err
	}
	for _, sc := range completed {
		if sc.RecordingMetadata == nil {
			continue
		}
		var info model.RecordingInfo
		if json.Unmarshal([]byte(*sc.RecordingMetadata), &info) != nil || !s.favoriteRecordingFileExists(sc, info) {
			continue
		}
		if err := saveRecordingInfo(ctx, s.redis, info.RecordingID, &info); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) favoriteScheduleStillRegistered(ctx context.Context, sc *model.RecordingSchedule) (bool, error) {
	var count int
	day := (int(sc.ScheduledStartTime.In(radikoLocation).Weekday()) + 6) % 7
	err := s.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM favorite_programs
		WHERE user_id = ? AND station_id = ? AND program_title = ?
		  AND (broadcast_day IS NULL OR broadcast_day = ?)`, sc.UserID, sc.StationID, sc.ProgramTitle, day)
	return count > 0, err
}

func favoriteRecordingRetryDelay(retryCount int) time.Duration {
	delays := [...]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour}
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount >= len(delays) {
		retryCount = len(delays) - 1
	}
	return delays[retryCount]
}

func (s *Scheduler) retryFavoriteRecording(sc *model.RecordingSchedule, reason string) {
	next := time.Now().Add(favoriteRecordingRetryDelay(sc.RetryCount))
	_, err := s.db.Exec(`UPDATE recording_schedules
		SET retry_count = retry_count + 1, status = 'pending', error_message = ?,
		    next_retry_at = ?, updated_at = NOW() WHERE id = ?`, reason, next, sc.ID)
	if err != nil {
		slog.Error("[job] favorite recording: retry update failed", "schedule_id", sc.ID, "error", err)
		return
	}
	slog.Info("[job] favorite recording: queued for retry", "schedule_id", sc.ID, "next_retry_at", next, "retry_count", sc.RetryCount+1)
}

// 手動録音済みの放送も自動予約に紐付け、同じ音声を取得し直さない。
func (s *Scheduler) reuseFavoriteRecording(ctx context.Context, sc *model.RecordingSchedule) (bool, error) {
	ownerKey := fmt.Sprintf("user_%d", sc.UserID)
	start := sc.ScheduledStartTime.In(radikoLocation).Format("20060102150405")
	var cursor uint64
	var inProgress bool
	for {
		keys, next, err := s.redis.Scan(ctx, cursor, "recording_*", 100).Result()
		if err != nil {
			return false, err
		}
		for _, key := range keys {
			raw, err := s.redis.Get(ctx, key).Result()
			if err != nil {
				continue
			}
			var info model.RecordingInfo
			if json.Unmarshal([]byte(raw), &info) != nil || info.OwnerKey != ownerKey || info.StationID != sc.StationID {
				continue
			}
			ft := info.StartTime
			if len(ft) == 12 {
				ft += "00"
			}
			if ft != start {
				continue
			}
			if info.Status == "recording" {
				inProgress = true
			}
			if info.Status != "completed" || info.RecordingID == "" {
				continue
			}
			if !isRecordingFilePathAllowed(s.storagePath, info.FilePath) {
				continue
			}
			if _, err := os.Stat(info.FilePath); err != nil {
				continue
			}
			if err := saveRecordingInfo(ctx, s.redis, info.RecordingID, &info); err != nil {
				return false, err
			}
			return true, s.completeFavoriteRecording(ctx, sc.ID, &info)
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	if inProgress {
		_, err := s.db.ExecContext(ctx, `UPDATE recording_schedules
			SET status = 'pending', next_retry_at = DATE_ADD(NOW(), INTERVAL 60 SECOND), updated_at = NOW()
			WHERE id = ?`, sc.ID)
		return true, err
	}
	return false, nil
}
