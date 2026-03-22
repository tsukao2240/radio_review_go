package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"github.com/redis/go-redis/v9"
	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/pkg/radiko"
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
// ログイン済みなら "user_{id}"、ゲストなら "session_{sessionID}"。
func (h *RecordingHandler) ownerKey(r *http.Request, store sessions.Store) string {
	if userID, ok := middleware.GetUserID(r.Context()); ok {
		return fmt.Sprintf("user_%d", userID)
	}
	session, _ := store.Get(r, "radio_review_session")
	return fmt.Sprintf("session_%s", session.ID)
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

		areaID := req.AreaID
		if areaID == "" {
			areaID = "JP13"
		}

		// 認証トークン取得
		authToken, err := h.radikoClient.GetAuthToken(r.Context(), areaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Radiko認証トークンの取得に失敗しました: "+err.Error())
			return
		}

		// recording_id 生成
		recordingID := fmt.Sprintf("%d", time.Now().UnixNano())

		// ファイルパス生成
		safeProgName := strings.ReplaceAll(req.ProgramName, "/", "_")
		fileName := fmt.Sprintf("%s_%s.aac", recordingID, safeProgName)
		filePath := filepath.Join(h.storagePath, fileName)

		ownerKey := h.ownerKey(r, store)

		info := &model.RecordingInfo{
			RecordingID: recordingID,
			StationID:   req.StationID,
			ProgramName: req.ProgramName,
			StartTime:   req.StartTime,
			EndTime:     req.EndTime,
			Status:      "recording",
			FilePath:    filePath,
			OwnerKey:    ownerKey,
			CreatedAt:   time.Now().Format(time.RFC3339),
		}

		if err := h.saveRecordingInfo(r.Context(), info, 2*time.Hour); err != nil {
			writeError(w, http.StatusInternalServerError, "録音情報の保存に失敗しました: "+err.Error())
			return
		}

		// 非同期で HLS ダウンロード実行
		go func() {
			ctx := context.Background()

			// ストレージディレクトリを作成（存在しない場合）
			_ = os.MkdirAll(h.storagePath, 0755)

			dlErr := h.hlsDownloader.DownloadTimefree(ctx, authToken, req.StationID, req.StartTime, req.EndTime, filePath)

			// ステータス更新
			updated, loadErr := h.loadRecordingInfo(ctx, recordingID)
			if loadErr != nil {
				return
			}
			if updated.Status == "stopped" {
				// 停止済みはそのまま
				return
			}
			if dlErr != nil {
				updated.Status = "failed"
			} else {
				updated.Status = "completed"
			}
			_ = h.saveRecordingInfo(ctx, updated, 2*time.Hour)
		}()

		writeJSON(w, http.StatusOK, map[string]string{
			"recording_id": recordingID,
			"status":       "recording",
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

		ownerKey := h.ownerKey(r, store)
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

		ownerKey := h.ownerKey(r, store)
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

		ownerKey := h.ownerKey(r, store)
		if info.OwnerKey != ownerKey {
			writeError(w, http.StatusForbidden, "アクセス権限がありません")
			return
		}

		if info.Status != "completed" {
			writeError(w, http.StatusBadRequest, "録音が完了していません (status: "+info.Status+")")
			return
		}

		f, err := os.Open(info.FilePath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ファイルのオープンに失敗しました: "+err.Error())
			return
		}
		defer f.Close()

		downloadName := info.ProgramName + ".aac"
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
		http.ServeContent(w, r, downloadName, time.Time{}, f)
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
		ownerKey := h.ownerKey(r, store)

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
		ownerKey := h.ownerKey(r, store)

		recordings, err := h.listOwnerRecordings(r.Context(), ownerKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "録音一覧の取得に失敗しました: "+err.Error())
			return
		}

		if recordings == nil {
			recordings = []model.RecordingInfo{}
		}

		RenderWithBase(w, "web/templates/recording/history.html", map[string]interface{}{
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

		ownerKey := h.ownerKey(r, store)
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
			_ = os.Remove(info.FilePath)
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"recording_id": req.RecordingID,
			"status":       "deleted",
		})
	}
}
