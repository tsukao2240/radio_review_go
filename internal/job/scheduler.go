// Package job はバックグラウンドジョブのスケジューリングを担当する。
package job

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/pkg/radiko"
)

// Scheduler は全バックグラウンドジョブを管理する。
type Scheduler struct {
	db            *sqlx.DB
	redis         *redis.Client
	radikoClient  radiko.ClientInterface
	hlsDownloader radiko.HLSDownloaderInterface
	storagePath   string
}

// NewScheduler は新しい Scheduler を返す。
func NewScheduler(
	db *sqlx.DB,
	rdb *redis.Client,
	radikoClient radiko.ClientInterface,
	hlsDownloader radiko.HLSDownloaderInterface,
	storagePath string,
) *Scheduler {
	return &Scheduler{
		db:            db,
		redis:         rdb,
		radikoClient:  radikoClient,
		hlsDownloader: hlsDownloader,
		storagePath:   storagePath,
	}
}

// Start は goroutine で全ジョブを開始する。ctx がキャンセルされると全ジョブが停止する。
func (s *Scheduler) Start(ctx context.Context) {
	// 毎分実行
	go s.runEveryMinute(ctx, s.processRecordingSchedules)
	// 5分ごと実行
	go s.runEveryN(ctx, 5*time.Minute, s.checkFavoriteProgramsBroadcast)
	// 毎日5:00実行
	go s.runDailyAt(ctx, 5, 0, s.insertRadioPrograms)
	go s.runDailyAt(ctx, 5, 0, s.deleteDuplicateRecords)
	// 録音情報のRedis TTL失効後に残った音声ファイルを掃除
	go s.runEveryN(ctx, time.Hour, s.sweepOrphanRecordingFiles)
}

// runEveryMinute は毎分 fn を実行するループ。
func (s *Scheduler) runEveryMinute(ctx context.Context, fn func(ctx context.Context)) {
	s.runEveryN(ctx, time.Minute, fn)
}

// runEveryN は interval ごとに fn を実行するループ。
func (s *Scheduler) runEveryN(ctx context.Context, interval time.Duration, fn func(ctx context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}

// runDailyAt は毎日 hour:minute に fn を実行するループ。
func (s *Scheduler) runDailyAt(ctx context.Context, hour, minute int, fn func(ctx context.Context)) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		wait := time.Until(next)

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			fn(ctx)
		}
	}
}

// SweepOrphanRecordingFiles はテスト用に孤児録音ファイル掃除を1回実行する。
func (s *Scheduler) SweepOrphanRecordingFiles(ctx context.Context) {
	s.sweepOrphanRecordingFiles(ctx)
}

func (s *Scheduler) sweepOrphanRecordingFiles(ctx context.Context) {
	activeFiles, err := s.activeRecordingFiles(ctx)
	if err != nil {
		log.Printf("[job] sweepOrphanRecordingFiles: Redis 走査エラー: %v", err)
		return
	}
	cutoff := time.Now().Add(-25 * time.Hour)
	if err := filepath.WalkDir(s.storagePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("[job] sweepOrphanRecordingFiles: walk エラー path=%s: %v", path, err)
			return nil
		}
		if d.IsDir() || filepath.Ext(path) != ".aac" {
			return nil
		}
		if !isRecordingFilePathAllowed(s.storagePath, path) {
			log.Printf("[job] sweepOrphanRecordingFiles: path検証失敗 path=%s", path)
			return nil
		}
		cleanPath, err := filepath.Abs(path)
		if err != nil {
			return nil
		}
		if activeFiles[cleanPath] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			log.Printf("[job] sweepOrphanRecordingFiles: stat エラー path=%s: %v", path, err)
			return nil
		}
		if info.ModTime().After(cutoff) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			log.Printf("[job] sweepOrphanRecordingFiles: remove エラー path=%s: %v", path, err)
			return nil
		}
		log.Printf("[job] sweepOrphanRecordingFiles: 削除 path=%s", path)
		return nil
	}); err != nil {
		log.Printf("[job] sweepOrphanRecordingFiles: WalkDir エラー: %v", err)
	}
}

func (s *Scheduler) activeRecordingFiles(ctx context.Context) (map[string]bool, error) {
	active := make(map[string]bool)
	var cursor uint64
	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "recording_*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			raw, err := s.redis.Get(ctx, key).Result()
			if err != nil {
				continue
			}
			var info model.RecordingInfo
			if err := json.Unmarshal([]byte(raw), &info); err != nil || info.FilePath == "" {
				continue
			}
			if !isRecordingFilePathAllowed(s.storagePath, info.FilePath) {
				continue
			}
			absPath, err := filepath.Abs(info.FilePath)
			if err == nil {
				active[absPath] = true
			}
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return active, nil
}

func isRecordingFilePathAllowed(storagePath, path string) bool {
	if filepath.Ext(path) != ".aac" {
		return false
	}
	storageAbs, err := filepath.Abs(storagePath)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(storageAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel != "." && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// ---------------------------------------------------------------------------
// InsertRadioPrograms: 毎日5:00 に全局の週間番組表を取得して radio_programs に UPSERT する
// ---------------------------------------------------------------------------

// radikoRegionXML は全局一覧 XML のルート要素。
// 実際の XML: <region><stations><station><id>HBC</id>...
type radikoRegionXML struct {
	Stations []radikoStation `xml:"stations>station"`
}

// radikoStation はステーション要素。
type radikoStation struct {
	ID string `xml:"id"`
}

// radikoWeeklyXML は週間番組表 XML のルート要素。
type radikoWeeklyXML struct {
	Programs []radikoProgNode `xml:"stations>station>progs>prog"`
}

// radikoProgNode は番組要素。
// ftl/tol はローカル時刻の "HHmm" 形式（例: "0500"）。PHP版も同じ属性を使用。
type radikoProgNode struct {
	Ftl   string `xml:"ftl,attr"`
	Tol   string `xml:"tol,attr"`
	Title string `xml:"title"`
	Cast  string `xml:"pfm"`
	Info  string `xml:"info"`
	URL   string `xml:"url"`
	Image string `xml:"img"`
}

// getBroadcastIDs は Radiko から全局 ID の一覧を取得する。
func getBroadcastIDs() ([]string, error) {
	const regionURL = "http://radiko.jp/v3/station/region/full.xml"
	resp, err := http.Get(regionURL) //nolint:gosec // 外部 API 取得
	if err != nil {
		return nil, fmt.Errorf("getBroadcastIDs: GET %s: %w", regionURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("getBroadcastIDs: ReadAll: %w", err)
	}

	var root radikoRegionXML
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("getBroadcastIDs: xml.Unmarshal: %w", err)
	}

	seen := make(map[string]struct{})
	var ids []string
	for _, st := range root.Stations {
		if st.ID == "" {
			continue
		}
		if _, ok := seen[st.ID]; !ok {
			seen[st.ID] = struct{}{}
			ids = append(ids, st.ID)
		}
	}
	return ids, nil
}

// fetchWeeklyPrograms は指定局の週間番組表を Radiko から取得して model.RadioProgram スライスを返す。
func fetchWeeklyPrograms(stationID string) ([]model.RadioProgram, error) {
	url := "http://radiko.jp/v3/program/station/weekly/" + stationID + ".xml"
	resp, err := http.Get(url) //nolint:gosec // 外部 API 取得
	if err != nil {
		return nil, fmt.Errorf("fetchWeeklyPrograms %s: GET: %w", stationID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetchWeeklyPrograms %s: ReadAll: %w", stationID, err)
	}

	var root radikoWeeklyXML
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("fetchWeeklyPrograms %s: xml.Unmarshal: %w", stationID, err)
	}

	programs := make([]model.RadioProgram, 0, len(root.Programs))
	for _, node := range root.Programs {
		p := model.RadioProgram{
			StationID: stationID,
			Title:     node.Title,
			Cast:      node.Cast,
			Start:     insertColon(node.Ftl),
			End:       insertColon(node.Tol),
		}
		if node.Info != "" {
			p.Info = &node.Info
		}
		if node.URL != "" {
			p.URL = &node.URL
		}
		if node.Image != "" {
			p.Image = &node.Image
		}
		programs = append(programs, p)
	}
	return programs, nil
}

// insertColon は "HHmm" → "HH:MM" に変換する。
// PHP版の substr_replace($str, ':', 2, 0) と同等。
// Radiko の ftl/tol 属性は "HHmm" の4桁文字列（例: "0500" → "05:00"）。
func insertColon(s string) string {
	if len(s) < 2 {
		return s
	}
	return s[:2] + ":" + s[2:]
}

// InsertRadioPrograms は全局の週間番組表を取得して radio_programs テーブルに UPSERT する（外部からも呼び出し可能）。
func (s *Scheduler) InsertRadioPrograms(ctx context.Context) {
	s.insertRadioPrograms(ctx)
}

// insertRadioPrograms は全局の週間番組表を取得して radio_programs テーブルに UPSERT する。
func (s *Scheduler) insertRadioPrograms(ctx context.Context) {
	log.Println("[job] insertRadioPrograms: 開始")

	ids, err := getBroadcastIDs()
	if err != nil {
		log.Printf("[job] insertRadioPrograms: getBroadcastIDs エラー: %v", err)
		return
	}

	// 全局の番組を収集（放送局・番組名・キャストで重複除去）
	type dedupKey struct {
		stationID, title, cast string
	}
	seen := make(map[dedupKey]struct{})
	var allPrograms []model.RadioProgram

	for _, stationID := range ids {
		programs, err := fetchWeeklyPrograms(stationID)
		if err != nil {
			log.Printf("[job] insertRadioPrograms: %s の番組取得エラー: %v", stationID, err)
			continue
		}
		for _, p := range programs {
			k := dedupKey{p.StationID, p.Title, p.Cast}
			if _, ok := seen[k]; !ok {
				seen[k] = struct{}{}
				allPrograms = append(allPrograms, p)
			}
		}
	}

	if len(allPrograms) == 0 {
		log.Println("[job] insertRadioPrograms: 取得できた番組が 0 件のため終了")
		return
	}

	// 1000 件ずつバッチ UPSERT（放送局・番組名・キャストが一致すれば UPDATE）
	const batchSize = 1000
	total := len(allPrograms)
	upserted := 0

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}
		batch := allPrograms[i:end]

		for _, p := range batch {
			prog := p
			if err := upsertRadioProgram(s.db, &prog); err != nil {
				log.Printf("[job] insertRadioPrograms: upsert エラー (station=%s title=%s): %v",
					prog.StationID, prog.Title, err)
				continue
			}
			upserted++
		}
	}

	log.Printf("[job] insertRadioPrograms: 完了 (upsert 件数: %d / %d)", upserted, total)
}

// upsertRadioProgram は (station_id, title, cast) をキーに INSERT ... ON DUPLICATE KEY UPDATE する。
func upsertRadioProgram(db *sqlx.DB, program *model.RadioProgram) error {
	_, err := db.NamedExec(
		`INSERT INTO radio_programs (station_id, title, cast, start, end, info, url, image)
		 VALUES (:station_id, :title, :cast, :start, :end, :info, :url, :image)
		 ON DUPLICATE KEY UPDATE
		   start = VALUES(start),
		   end   = VALUES(end),
		   info  = VALUES(info),
		   url   = VALUES(url),
		   image = VALUES(image)`,
		program,
	)
	return err
}

// ---------------------------------------------------------------------------
// DeleteDuplicateRecords: 毎日5:00 に radio_programs の重複レコードを削除する
// ---------------------------------------------------------------------------

// deleteDuplicateRecords はタイトルが重複している radio_programs レコードのうち
// min(id) 以外を削除する。
func (s *Scheduler) deleteDuplicateRecords(ctx context.Context) {
	log.Println("[job] deleteDuplicateRecords: 開始")

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM radio_programs
		 WHERE id NOT IN (
		   SELECT min_id FROM (
		     SELECT MIN(t1.id) AS min_id
		     FROM radio_programs AS t1
		     GROUP BY t1.title
		   ) AS t2
		 )`,
	)
	if err != nil {
		log.Printf("[job] deleteDuplicateRecords: DELETE エラー: %v", err)
		return
	}

	log.Println("[job] deleteDuplicateRecords: 完了")
}

// ---------------------------------------------------------------------------
// ProcessRecordingSchedules: 毎分実行 — pending かつ開始時刻到来のスケジュールを録音開始する
// ---------------------------------------------------------------------------

// processRecordingSchedules は status=pending かつ scheduled_start_time <= NOW() の
// スケジュールを取得し、各スケジュールの録音を goroutine で開始する。
func (s *Scheduler) processRecordingSchedules(ctx context.Context) {
	now := time.Now().Format("2006-01-02 15:04:05")

	schedules, err := findPendingBefore(s.db, now)
	if err != nil {
		log.Printf("[job] processRecordingSchedules: FindPendingBefore エラー: %v", err)
		return
	}

	if len(schedules) == 0 {
		return
	}

	log.Printf("[job] processRecordingSchedules: %d 件の録音予約を処理", len(schedules))

	for _, sc := range schedules {
		sc := sc // ループ変数コピー
		go s.startScheduledRecording(ctx, &sc)
	}
}

// startScheduledRecording は録音スケジュールの録音を開始し、完了・失敗を DB に反映する。
func (s *Scheduler) startScheduledRecording(ctx context.Context, sc *model.RecordingSchedule) {
	log.Printf("[job] 録音開始: id=%d station=%s title=%s", sc.ID, sc.StationID, sc.ProgramTitle)

	// ステータスを recording に更新
	if err := updateScheduleStatus(s.db, sc.ID, "recording", nil); err != nil {
		log.Printf("[job] startScheduledRecording: recording ステータス更新エラー (id=%d): %v", sc.ID, err)
		return
	}

	// 録音 ID 生成
	recordingID := fmt.Sprintf("sched_%d_%d", sc.ID, time.Now().Unix())

	areaID := radiko.GetAreaIDFromStationID(sc.StationID)
	authToken, err := s.radikoClient.GetAuthToken(ctx, areaID)
	if err != nil {
		errMsg := fmt.Sprintf("認証トークン取得エラー: %v", err)
		log.Printf("[job] startScheduledRecording (id=%d): %s", sc.ID, errMsg)
		_ = updateScheduleStatus(s.db, sc.ID, "failed", &errMsg)
		_ = createRecordingFailedNotification(s.db, sc, errMsg)
		return
	}

	// 出力ファイルパス作成
	startFmt := sc.ScheduledStartTime.Format("200601021504")
	endFmt := sc.ScheduledEndTime.Format("200601021504")
	fileName := fmt.Sprintf("%s_%s_%s.aac", sc.StationID, startFmt, endFmt)
	outputPath := filepath.Join(s.storagePath, fileName)

	if err := os.MkdirAll(s.storagePath, 0755); err != nil {
		errMsg := fmt.Sprintf("保存ディレクトリ作成エラー: %v", err)
		log.Printf("[job] startScheduledRecording (id=%d): %s", sc.ID, errMsg)
		_ = updateScheduleStatus(s.db, sc.ID, "failed", &errMsg)
		_ = createRecordingFailedNotification(s.db, sc, errMsg)
		return
	}

	// 録音情報を Redis に保存
	ownerKey := fmt.Sprintf("user_%d", sc.UserID)
	info := &model.RecordingInfo{
		RecordingID: recordingID,
		StationID:   sc.StationID,
		ProgramName: sc.ProgramTitle,
		StartTime:   startFmt,
		EndTime:     endFmt,
		Status:      "recording",
		FilePath:    outputPath,
		OwnerKey:    ownerKey,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}
	if err := saveRecordingInfo(ctx, s.redis, recordingID, info); err != nil {
		log.Printf("[job] startScheduledRecording (id=%d): Redis 保存エラー: %v", sc.ID, err)
		// Redis 保存失敗は録音処理を止める理由にはしない
	}

	// スケジュールに録音 ID を保存
	if err := setScheduleRecordingID(s.db, sc.ID, recordingID); err != nil {
		log.Printf("[job] startScheduledRecording (id=%d): recording_id 保存エラー: %v", sc.ID, err)
	}

	// HLS ダウンロード（タイムフリー録音）
	if err := s.hlsDownloader.DownloadTimefree(ctx, authToken, sc.StationID, startFmt, endFmt, areaID, outputPath); err != nil {
		errMsg := fmt.Sprintf("録音エラー: %v", err)
		log.Printf("[job] startScheduledRecording (id=%d): %s", sc.ID, errMsg)
		_ = updateScheduleStatus(s.db, sc.ID, "failed", &errMsg)
		_ = updateRedisRecordingStatus(ctx, s.redis, recordingID, "failed", errMsg)
		_ = createRecordingFailedNotification(s.db, sc, errMsg)
		return
	}

	// 完了処理
	_ = updateScheduleStatus(s.db, sc.ID, "completed", nil)
	_ = updateRedisRecordingStatus(ctx, s.redis, recordingID, "completed", "")
	_ = createRecordingStartNotification(s.db, sc, recordingID)

	log.Printf("[job] 録音完了: id=%d recording_id=%s", sc.ID, recordingID)

	// 定期録音の場合は次回スケジュールを自動生成する
	if sc.IsRecurring && sc.RecurrenceType != nil {
		if err := s.createNextRecurringSchedule(sc); err != nil {
			log.Printf("[job] 次回定期録音スケジュール生成エラー (id=%d): %v", sc.ID, err)
		}
	}
}

// createNextRecurringSchedule は定期録音の次回スケジュールを DB に INSERT する。
// recurrence_type="weekly" の場合は 7 日後に次回スケジュールを作成する。
func (s *Scheduler) createNextRecurringSchedule(sc *model.RecordingSchedule) error {
	var nextStart, nextEnd time.Time

	switch *sc.RecurrenceType {
	case "weekly":
		nextStart = sc.ScheduledStartTime.Add(7 * 24 * time.Hour)
		nextEnd = sc.ScheduledEndTime.Add(7 * 24 * time.Hour)
	default:
		return fmt.Errorf("未対応の recurrence_type: %s", *sc.RecurrenceType)
	}

	parentID := sc.ID
	recType := *sc.RecurrenceType
	next := &model.RecordingSchedule{
		UserID:             sc.UserID,
		StationID:          sc.StationID,
		ProgramTitle:       sc.ProgramTitle,
		ScheduledStartTime: nextStart,
		ScheduledEndTime:   nextEnd,
		Status:             "pending",
		IsRecurring:        true,
		RecurrenceType:     &recType,
		ParentScheduleID:   &parentID,
	}

	res, err := s.db.NamedExec(
		`INSERT INTO recording_schedules
		 (user_id, station_id, program_title, scheduled_start_time, scheduled_end_time,
		  status, is_recurring, recurrence_type, parent_schedule_id, created_at, updated_at)
		 VALUES
		 (:user_id, :station_id, :program_title, :scheduled_start_time, :scheduled_end_time,
		  :status, :is_recurring, :recurrence_type, :parent_schedule_id, NOW(), NOW())`,
		next,
	)
	if err != nil {
		return fmt.Errorf("createNextRecurringSchedule INSERT: %w", err)
	}

	newID, _ := res.LastInsertId()
	log.Printf("[job] 次回定期録音スケジュール生成: id=%d station=%s title=%s start=%s",
		newID, sc.StationID, sc.ProgramTitle, nextStart.Format("2006-01-02 15:04"))
	return nil
}

// findPendingBefore は status=pending かつ scheduled_start_time <= t のスケジュールを返す。
func findPendingBefore(db *sqlx.DB, t string) ([]model.RecordingSchedule, error) {
	var schedules []model.RecordingSchedule
	err := db.Select(&schedules,
		`SELECT * FROM recording_schedules
		 WHERE status = 'pending' AND scheduled_start_time <= ?
		 ORDER BY scheduled_start_time ASC`,
		t,
	)
	if err != nil {
		return nil, err
	}
	return schedules, nil
}

// updateScheduleStatus は録音スケジュールのステータスとエラーメッセージを更新する。
func updateScheduleStatus(db *sqlx.DB, id int64, status string, errMsg *string) error {
	_, err := db.Exec(
		"UPDATE recording_schedules SET status = ?, error_message = ?, updated_at = NOW() WHERE id = ?",
		status, errMsg, id,
	)
	return err
}

// setScheduleRecordingID は録音スケジュールに録音 ID を設定する。
func setScheduleRecordingID(db *sqlx.DB, id int64, recordingID string) error {
	_, err := db.Exec(
		"UPDATE recording_schedules SET recording_id = ?, updated_at = NOW() WHERE id = ?",
		recordingID, id,
	)
	return err
}

// saveRecordingInfo は RecordingInfo を Redis に JSON で保存する（TTL: 24 時間）。
func saveRecordingInfo(ctx context.Context, rdb *redis.Client, recordingID string, info *model.RecordingInfo) error {
	b, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}
	return rdb.Set(ctx, "recording_"+recordingID, string(b), 24*time.Hour).Err()
}

// updateRedisRecordingStatus は Redis に保存済みの RecordingInfo の status を更新する。
func updateRedisRecordingStatus(ctx context.Context, rdb *redis.Client, recordingID, status, failReason string) error {
	key := "recording_" + recordingID
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("redis.Get: %w", err)
	}
	var info model.RecordingInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return fmt.Errorf("json.Unmarshal: %w", err)
	}
	info.Status = status
	info.FailReason = failReason
	b, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return rdb.Set(ctx, key, string(b), ttl).Err()
}

// createRecordingStartNotification は録音開始通知を notifications テーブルに INSERT する。
func createRecordingStartNotification(db *sqlx.DB, sc *model.RecordingSchedule, recordingID string) error {
	dataJSON, _ := json.Marshal(map[string]string{
		"station_id":   sc.StationID,
		"recording_id": recordingID,
	})
	dataStr := string(dataJSON)
	n := &model.Notification{
		UserID:  sc.UserID,
		Type:    "recording_start",
		Title:   "録音開始",
		Message: fmt.Sprintf("「%s」の録音を開始しました", sc.ProgramTitle),
		Data:    &dataStr,
	}
	return insertNotification(db, n)
}

// createRecordingFailedNotification は録音失敗通知を notifications テーブルに INSERT する。
func createRecordingFailedNotification(db *sqlx.DB, sc *model.RecordingSchedule, errMsg string) error {
	dataJSON, _ := json.Marshal(map[string]string{
		"station_id": sc.StationID,
		"error":      errMsg,
	})
	dataStr := string(dataJSON)
	n := &model.Notification{
		UserID:  sc.UserID,
		Type:    "recording_failed",
		Title:   "録音失敗",
		Message: fmt.Sprintf("「%s」の録音に失敗しました: %s", sc.ProgramTitle, errMsg),
		Data:    &dataStr,
	}
	return insertNotification(db, n)
}

// insertNotification は notifications テーブルにレコードを INSERT する。
func insertNotification(db *sqlx.DB, n *model.Notification) error {
	_, err := db.Exec(
		`INSERT INTO notifications (user_id, type, title, message, data, is_read, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, FALSE, NOW(), NOW())`,
		n.UserID, n.Type, n.Title, n.Message, n.Data,
	)
	return err
}

// ---------------------------------------------------------------------------
// CheckFavoriteProgramsBroadcast: 5分ごと — お気に入り番組が今後5分以内に放送開始するか確認
// ---------------------------------------------------------------------------

// checkFavoriteProgramsBroadcast は全ユーザーのお気に入り番組を確認し、
// 5分以内に放送開始する番組のユーザーへ通知を作成する。
func (s *Scheduler) checkFavoriteProgramsBroadcast(ctx context.Context) {
	log.Println("[job] checkFavoriteProgramsBroadcast: 開始")

	// 全お気に入り番組を取得
	var favorites []model.FavoriteProgram
	if err := s.db.SelectContext(ctx, &favorites,
		"SELECT * FROM favorite_programs ORDER BY created_at DESC",
	); err != nil {
		log.Printf("[job] checkFavoriteProgramsBroadcast: SELECT エラー: %v", err)
		return
	}

	if len(favorites) == 0 {
		log.Println("[job] checkFavoriteProgramsBroadcast: お気に入り番組が登録されていません")
		return
	}

	now := time.Now()
	// "HHmm" 形式で比較（PHP の Carbon::now()->format('Hi') に相当）
	nowHHmm := now.Format("1504")
	fiveMinLater := now.Add(5 * time.Minute).Format("1504")

	notifCount := 0

	for _, fav := range favorites {
		fav := fav

		// radio_programs から番組情報を取得（station_id + title で検索）
		var program model.RadioProgram
		err := s.db.GetContext(ctx, &program,
			"SELECT * FROM radio_programs WHERE station_id = ? AND title = ? LIMIT 1",
			fav.StationID, fav.ProgramTitle,
		)
		if err != nil {
			// 番組が見つからない場合はスキップ
			continue
		}

		// start カラムの末尾4文字が "HHmm" 形式になっている前提で比較
		// （start カラムは "YYYYMMDDHHmm" や "HH:mm" 等の形式が混在しうるため末尾4文字を使用）
		progStartRaw := program.Start
		var progHHmm string
		if len(progStartRaw) >= 5 && progStartRaw[len(progStartRaw)-3] == ':' {
			// "HH:mm" 形式 → コロンを除去
			progHHmm = progStartRaw[len(progStartRaw)-5:len(progStartRaw)-3] +
				progStartRaw[len(progStartRaw)-2:]
		} else if len(progStartRaw) >= 4 {
			// "...HHmm" 形式 → 末尾4文字
			progHHmm = progStartRaw[len(progStartRaw)-4:]
		} else {
			continue
		}

		// 5分以内に放送開始する番組を検出
		if progHHmm >= nowHHmm && progHHmm <= fiveMinLater {
			if err := createFavoriteBroadcastNotification(s.db, &fav, &program); err != nil {
				log.Printf("[job] checkFavoriteProgramsBroadcast: 通知作成エラー (fav_id=%d): %v",
					fav.ID, err)
				continue
			}
			notifCount++
			log.Printf("[job] 放送通知: user_id=%d station=%s title=%s",
				fav.UserID, fav.StationID, fav.ProgramTitle)
		}
	}

	log.Printf("[job] checkFavoriteProgramsBroadcast: 完了 (通知: %d 件)", notifCount)
}

// createFavoriteBroadcastNotification はお気に入り番組の放送開始通知を notifications に INSERT する。
func createFavoriteBroadcastNotification(db *sqlx.DB, fav *model.FavoriteProgram, program *model.RadioProgram) error {
	dataJSON, _ := json.Marshal(map[string]string{
		"station_id":    fav.StationID,
		"program_title": fav.ProgramTitle,
	})
	dataStr := string(dataJSON)
	n := &model.Notification{
		UserID:  fav.UserID,
		Type:    "favorite_broadcast",
		Title:   "お気に入り番組の放送開始",
		Message: fmt.Sprintf("「%s」(%s) がまもなく放送開始します", fav.ProgramTitle, fav.StationID),
		Data:    &dataStr,
	}
	return insertNotification(db, n)
}

// ---------------------------------------------------------------------------
// cmd/server/main.go の main() に以下を追加:
//
//   scheduler := job.NewScheduler(db, rdb, radikoClient, hlsDownloader, storagePath)
//   go scheduler.Start(ctx)
//
// ※ ctx は signal.NotifyContext 等で作成したキャンセル可能なコンテキストを使用すること。
// ---------------------------------------------------------------------------
