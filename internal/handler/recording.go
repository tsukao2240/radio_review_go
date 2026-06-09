package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"github.com/redis/go-redis/v9"
	"github.com/tsukao2240/radio_review_go/internal/middleware"
	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/internal/recordingfile"
	"github.com/tsukao2240/radio_review_go/internal/recordingmeta"
	"github.com/tsukao2240/radio_review_go/pkg/radiko"
)

// RecordingHandler は録音関連のHTTPハンドラーを管理する。
type RecordingHandler struct {
	radikoClient  radiko.ClientInterface
	hlsDownloader radiko.HLSDownloaderInterface
	redisClient   *redis.Client
	storagePath   string
}

// NewRecordingHandler は新しい RecordingHandler を返す。
func NewRecordingHandler(
	radikoClient radiko.ClientInterface,
	hlsDownloader radiko.HLSDownloaderInterface,
	redisClient *redis.Client,
	storagePath string,
) *RecordingHandler {
	return &RecordingHandler{
		radikoClient:  radikoClient,
		hlsDownloader: hlsDownloader,
		redisClient:   redisClient,
		storagePath:   storagePath,
	}
}

// ownerKey はリクエストからオーナーキーを生成する。
// ログイン済みなら "user_{id}"、ゲストならセッションに保存したUUIDベースの "session_{guestID}"。
func (h *RecordingHandler) ownerKey(r *http.Request, w http.ResponseWriter, store sessions.Store) (string, error) {
	session, err := store.Get(r, "radio_review_session")
	if err != nil {
		return "", err
	}
	if userID, ok := session.Values["user_id"].(int64); ok {
		return fmt.Sprintf("user_%d", userID), nil
	}
	if userID, ok := middleware.GetUserID(r.Context()); ok {
		return fmt.Sprintf("user_%d", userID), nil
	}
	guestID, _ := session.Values["guest_owner_id"].(string)
	if guestID == "" {
		guestID = newGuestOwnerID()
		session.Values["guest_owner_id"] = guestID
		if err := session.Save(r, w); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("session_%s", guestID), nil
}

func newGuestOwnerID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// saveRecordingInfo は RecordingInfo を Redis に JSON シリアライズして保存する。
func (h *RecordingHandler) saveRecordingInfo(ctx context.Context, info *model.RecordingInfo, ttl time.Duration) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}
	key := "recording_" + info.RecordingID
	return h.redisClient.Set(ctx, key, string(data), ttl).Err()
}

// loadRecordingInfo は Redis から RecordingInfo を取得してデシリアライズする。
func (h *RecordingHandler) loadRecordingInfo(ctx context.Context, recordingID string) (*model.RecordingInfo, error) {
	key := "recording_" + recordingID
	val, err := h.redisClient.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis.Get %s: %w", key, err)
	}
	var info model.RecordingInfo
	if err := json.Unmarshal([]byte(val), &info); err != nil {
		return nil, fmt.Errorf("json.Unmarshal: %w", err)
	}
	return &info, nil
}

// writeJSON はレスポンスを JSON 形式で書き込む。
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError はエラーレスポンスを JSON 形式で書き込む。
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *RecordingHandler) validateRecordingFilePath(filePath string) bool {
	cleanPath := filepath.Clean(filePath)
	storageRoot := filepath.Clean(h.storagePath) + string(os.PathSeparator)
	return strings.HasPrefix(cleanPath, storageRoot)
}

// StartTimefree はタイムフリー録音を開始し、非同期で HLS ダウンロードを実行する。
func (h *RecordingHandler) StartTimefree(ctx context.Context, ownerKey, stationID, programName, startTime, endTime, areaID string) (string, error) {
	if areaID == "" {
		areaID = radiko.GetAreaIDFromStationID(stationID)
	}

	authToken, err := h.radikoClient.GetAuthToken(ctx, areaID)
	if err != nil {
		return "", fmt.Errorf("Radiko認証トークンの取得に失敗しました: %w", err)
	}

	recordingID := fmt.Sprintf("%d", time.Now().UnixNano())
	filePath := recordingfile.NewPath(h.storagePath, recordingID, startTime, stationID, programName)

	info := &model.RecordingInfo{
		RecordingID: recordingID,
		StationID:   stationID,
		ProgramName: programName,
		StartTime:   startTime,
		EndTime:     endTime,
		Status:      "recording",
		FilePath:    filePath,
		OwnerKey:    ownerKey,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	if err := h.saveRecordingInfo(ctx, info, 2*time.Hour); err != nil {
		return "", fmt.Errorf("録音情報の保存に失敗しました: %w", err)
	}

	go func() {
		bgCtx := context.Background()

		if err := os.MkdirAll(h.storagePath, 0755); err != nil {
			log.Printf("StartTimefree: mkdir storage failed recording_id=%s path=%s: %v", recordingID, h.storagePath, err)
		}

		dlErr := h.hlsDownloader.DownloadTimefree(bgCtx, authToken, stationID, startTime, endTime, areaID, filePath)

		updated, loadErr := h.loadRecordingInfo(bgCtx, recordingID)
		if loadErr != nil {
			log.Printf("StartTimefree: load recording info failed recording_id=%s: %v", recordingID, loadErr)
			return
		}
		if updated.Status == "stopped" {
			return
		}
		if dlErr != nil {
			updated.Status = "failed"
			updated.FailReason = dlErr.Error()
			log.Printf("StartTimefree: download failed recording_id=%s station_id=%s start=%s end=%s: %v", recordingID, stationID, startTime, endTime, dlErr)
		} else {
			if err := recordingmeta.TagAAC(bgCtx, updated); err != nil {
				log.Printf("StartTimefree: metadata tagging skipped recording_id=%s: %v", recordingID, err)
			}
			updated.Status = "completed"
			updated.FailReason = ""
		}
		if err := h.saveRecordingInfo(bgCtx, updated, 2*time.Hour); err != nil {
			log.Printf("StartTimefree: save recording info failed recording_id=%s: %v", recordingID, err)
		}
	}()

	return recordingID, nil
}

// IsProgramRecording は同一オーナーの同一番組が録音中かを返す。
func (h *RecordingHandler) IsProgramRecording(ctx context.Context, ownerKey, programName string) (bool, error) {
	recordings, err := h.listOwnerRecordings(ctx, ownerKey)
	if err != nil {
		return false, err
	}
	for _, rec := range recordings {
		if rec.ProgramName == programName && rec.Status == "recording" {
			return true, nil
		}
	}
	return false, nil
}

// StartTimefreeRecording は POST /recording/timefree/start を処理する。
// タイムフリー録音を開始し、非同期で HLS ダウンロードを実行する。
func (h *RecordingHandler) StartTimefreeRecording(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			StationID   string `json:"station_id"`
			StartTime   string `json:"start_time"`
			EndTime     string `json:"end_time"`
			ProgramName string `json:"program_name"`
			AreaID      string `json:"area_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
			return
		}

		// バリデーション
		if req.StationID == "" || req.StartTime == "" || req.EndTime == "" || req.ProgramName == "" {
			writeError(w, http.StatusUnprocessableEntity, "station_id, start_time, end_time, program_name は必須です")
			return
		}

		ownerKey, err := h.ownerKey(r, w, store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
			return
		}

		recordingID, err := h.StartTimefree(r.Context(), ownerKey, req.StationID, req.ProgramName, req.StartTime, req.EndTime, req.AreaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"recording_id": recordingID,
			"status":       "recording",
			"success":      "true",
		})
	}
}

// StopRecording は POST /recording/stop を処理する。
// 指定した録音の status を "stopped" に更新する。
func (h *RecordingHandler) StopRecording(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RecordingID string `json:"recording_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
			return
		}
		if req.RecordingID == "" {
			writeError(w, http.StatusUnprocessableEntity, "recording_id は必須です")
			return
		}

		info, err := h.loadRecordingInfo(r.Context(), req.RecordingID)
		if err != nil {
			writeError(w, http.StatusNotFound, "録音情報が見つかりません")
			return
		}

		ownerKey, err := h.ownerKey(r, w, store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
			return
		}
		if info.OwnerKey != ownerKey {
			writeError(w, http.StatusForbidden, "アクセス権限がありません")
			return
		}

		info.Status = "stopped"
		if err := h.saveRecordingInfo(r.Context(), info, 2*time.Hour); err != nil {
			writeError(w, http.StatusInternalServerError, "録音情報の更新に失敗しました: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"recording_id": req.RecordingID,
			"status":       "stopped",
		})
	}
}

// GetRecordingStatus は GET /recording/status を処理する。
// 指定した録音の status を JSON で返す。
func (h *RecordingHandler) GetRecordingStatus(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recordingID := r.URL.Query().Get("recording_id")
		if recordingID == "" {
			writeError(w, http.StatusBadRequest, "recording_id は必須です")
			return
		}

		info, err := h.loadRecordingInfo(r.Context(), recordingID)
		if err != nil {
			writeError(w, http.StatusNotFound, "録音情報が見つかりません")
			return
		}

		ownerKey, err := h.ownerKey(r, w, store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
			return
		}
		if info.OwnerKey != ownerKey {
			writeError(w, http.StatusForbidden, "アクセス権限がありません")
			return
		}

		writeJSON(w, http.StatusOK, info)
	}
}

// DownloadRecording は GET /recording/download を処理する。
// 録音ファイルを application/octet-stream で返す。
func (h *RecordingHandler) DownloadRecording(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recordingID := r.URL.Query().Get("recording_id")
		if recordingID == "" {
			writeError(w, http.StatusBadRequest, "recording_id は必須です")
			return
		}

		info, err := h.loadRecordingInfo(r.Context(), recordingID)
		if err != nil {
			writeError(w, http.StatusNotFound, "録音情報が見つかりません")
			return
		}

		ownerKey, err := h.ownerKey(r, w, store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
			return
		}
		if info.OwnerKey != ownerKey {
			writeError(w, http.StatusForbidden, "アクセス権限がありません")
			return
		}

		if info.Status != "completed" {
			writeError(w, http.StatusBadRequest, "録音が完了していません (status: "+info.Status+")")
			return
		}

		if !h.validateRecordingFilePath(info.FilePath) {
			writeError(w, http.StatusForbidden, "不正なファイルパスです")
			return
		}

		f, err := os.Open(info.FilePath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ファイルのオープンに失敗しました: "+err.Error())
			return
		}
		defer func() { _ = f.Close() }()

		downloadName := recordingfile.DisplayName(info.FilePath, info.StartTime, info.StationID, info.ProgramName)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
		http.ServeContent(w, r, downloadName, time.Time{}, f)
	}
}

// StreamRecording は GET /recording/stream を処理する。
// 録音ファイルを audio/aac で返し、Range リクエストによるシークを可能にする。
func (h *RecordingHandler) StreamRecording(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recordingID := r.URL.Query().Get("recording_id")
		if recordingID == "" {
			writeError(w, http.StatusBadRequest, "recording_id は必須です")
			return
		}

		info, err := h.loadRecordingInfo(r.Context(), recordingID)
		if err != nil {
			writeError(w, http.StatusNotFound, "録音情報が見つかりません")
			return
		}

		ownerKey, err := h.ownerKey(r, w, store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
			return
		}
		if info.OwnerKey != ownerKey {
			writeError(w, http.StatusForbidden, "アクセス権限がありません")
			return
		}

		if info.Status != "completed" {
			writeError(w, http.StatusBadRequest, "録音が完了していません (status: "+info.Status+")")
			return
		}

		if !h.validateRecordingFilePath(info.FilePath) {
			writeError(w, http.StatusForbidden, "不正なファイルパスです")
			return
		}

		f, err := os.Open(info.FilePath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ファイルのオープンに失敗しました: "+err.Error())
			return
		}
		defer func() { _ = f.Close() }()

		stat, err := f.Stat()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ファイル情報の取得に失敗しました: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "audio/aac")
		streamName := recordingfile.DisplayName(info.FilePath, info.StartTime, info.StationID, info.ProgramName)
		http.ServeContent(w, r, streamName, stat.ModTime(), f)
	}
}

// listOwnerRecordings は自分の owner_key に一致する録音一覧を Redis SCAN で取得する。
func (h *RecordingHandler) listOwnerRecordings(ctx context.Context, ownerKey string) ([]model.RecordingInfo, error) {
	var cursor uint64
	var recordings []model.RecordingInfo

	for {
		keys, nextCursor, err := h.redisClient.Scan(ctx, cursor, "recording_*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("redis.Scan: %w", err)
		}

		for _, key := range keys {
			val, err := h.redisClient.Get(ctx, key).Result()
			if err != nil {
				continue
			}
			var info model.RecordingInfo
			if err := json.Unmarshal([]byte(val), &info); err != nil {
				continue
			}
			if info.OwnerKey == ownerKey {
				recordings = append(recordings, info)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return recordings, nil
}

// ListRecordings は GET /recording/list を処理する。
// 自分の録音一覧を JSON で返す。
func (h *RecordingHandler) ListRecordings(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerKey, err := h.ownerKey(r, w, store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
			return
		}

		recordings, err := h.listOwnerRecordings(r.Context(), ownerKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "録音一覧の取得に失敗しました: "+err.Error())
			return
		}

		if recordings == nil {
			recordings = []model.RecordingInfo{}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"recordings": recordings,
		})
	}
}

// ShowHistory は GET /recording/history を処理する。
// 録音履歴を HTML テンプレートで返す。テンプレートが存在しない場合は JSON で代替する。
func (h *RecordingHandler) ShowHistory(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerKey, err := h.ownerKey(r, w, store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
			return
		}

		recordings, err := h.listOwnerRecordings(r.Context(), ownerKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "録音一覧の取得に失敗しました: "+err.Error())
			return
		}

		if recordings == nil {
			recordings = []model.RecordingInfo{}
		}

		RenderWithBase(w, r, "web/templates/recording/history.html", map[string]interface{}{
			"recordings": recordings,
		})
	}
}

// DeleteRecording は POST /recording/delete を処理する。
// Redis キーと録音ファイルを削除する。
func (h *RecordingHandler) DeleteRecording(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RecordingID string `json:"recording_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
			return
		}
		if req.RecordingID == "" {
			writeError(w, http.StatusUnprocessableEntity, "recording_id は必須です")
			return
		}

		info, err := h.loadRecordingInfo(r.Context(), req.RecordingID)
		if err != nil {
			writeError(w, http.StatusNotFound, "録音情報が見つかりません")
			return
		}

		ownerKey, err := h.ownerKey(r, w, store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "セッションの保存に失敗しました")
			return
		}
		if info.OwnerKey != ownerKey {
			writeError(w, http.StatusForbidden, "アクセス権限がありません")
			return
		}

		// Redis キーを削除
		key := "recording_" + req.RecordingID
		if err := h.redisClient.Del(r.Context(), key).Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "録音情報の削除に失敗しました: "+err.Error())
			return
		}

		// 録音ファイルを削除（エラーは無視）
		if info.FilePath != "" {
			if !h.validateRecordingFilePath(info.FilePath) {
				writeError(w, http.StatusForbidden, "不正なファイルパスです")
				return
			}
			_ = os.Remove(info.FilePath)
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"recording_id": req.RecordingID,
			"status":       "deleted",
		})
	}
}
