package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/sessions"
	"github.com/redis/go-redis/v9"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

func downloadReceiptKey(id, receipt string) string { return "download_receipt:" + id + ":" + receipt }

// CompleteSavedDownload removes audio only after the client has closed its saved file.
func (h *RecordingHandler) CompleteSavedDownload(store sessions.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RecordingID string `json:"recording_id"`
			Receipt     string `json:"receipt"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || req.RecordingID == "" || req.Receipt == "" {
			writeError(w, http.StatusBadRequest, "保存完了情報が不正です")
			return
		}
		owner, err := h.ownerKey(r, w, store)
		if err != nil {
			writeRecordingAccessError(w, r)
			return
		}
		raw, err := h.redisClient.Get(r.Context(), downloadReceiptKey(req.RecordingID, req.Receipt)).Result()
		if err != nil {
			writeError(w, http.StatusConflict, "ダウンロード完了の確認を待っています")
			return
		}
		var delivered model.RecordingInfo
		if json.Unmarshal([]byte(raw), &delivered) != nil || delivered.RecordingID != req.RecordingID || delivered.OwnerKey != owner {
			writeError(w, http.StatusForbidden, "アクセス権限がありません")
			return
		}
		current, err := h.loadRecordingInfo(r.Context(), req.RecordingID)
		if errors.Is(err, redis.Nil) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "downloaded"})
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "録音情報を確認できません")
			return
		}
		if current.OwnerKey != owner || current.FilePath != delivered.FilePath || current.Status != "completed" || !h.validateRecordingFilePath(current.FilePath) {
			writeError(w, http.StatusConflict, "録音情報が変更されたためファイルを保持しました")
			return
		}
		if h.downloadCompletion == nil {
			writeError(w, http.StatusServiceUnavailable, "保存済み状態を記録できません")
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
		defer cancel()
		if err := h.downloadCompletion(ctx, current); err != nil {
			writeError(w, http.StatusServiceUnavailable, "保存済み状態の記録に失敗したため録音を保持しました")
			return
		}
		if err := os.Remove(current.FilePath); err != nil && !os.IsNotExist(err) {
			writeError(w, http.StatusInternalServerError, "保存済み録音の削除に失敗しました")
			return
		}
		if err := h.redisClient.Del(ctx, "recording_"+req.RecordingID).Err(); err != nil {
			writeError(w, http.StatusServiceUnavailable, "録音一覧の更新に失敗しました")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "downloaded"})
	}
}
