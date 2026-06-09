package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/tsukao2240/radio_review_go/internal/middleware"
	"github.com/tsukao2240/radio_review_go/internal/service"
)

// FavoriteHandler はお気に入り番組関連の HTTP ハンドラーを管理する。
type FavoriteHandler struct {
	favService    service.FavoriteServiceInterface
	radikoService service.RadikoApiServiceInterface
	store         sessions.Store
	recorder      favoriteRecorder
}

type favoriteRecorder interface {
	StartTimefree(ctx context.Context, ownerKey, stationID, programName, startTime, endTime, areaID string) (string, error)
	IsProgramRecording(ctx context.Context, ownerKey, programName string) (bool, error)
}

// NewFavoriteHandler は新しい FavoriteHandler を返す。
func NewFavoriteHandler(favService service.FavoriteServiceInterface, radikoService service.RadikoApiServiceInterface, store sessions.Store) *FavoriteHandler {
	return &FavoriteHandler{
		favService:    favService,
		radikoService: radikoService,
		store:         store,
	}
}

// SetRecorder は一括タイムフリー録音で使用する録音開始処理を設定する。
func (h *FavoriteHandler) SetRecorder(recorder favoriteRecorder) {
	h.recorder = recorder
}

// Index は GET /favorites を処理する。お気に入り一覧。
func (h *FavoriteHandler) Index(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	favs, err := h.favService.GetByUser(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "お気に入り一覧の取得に失敗しました: "+err.Error())
		return
	}

	// 各お気に入りに次回放送のキャスト・日付を付与
	type favWithCast struct {
		ID             int64
		StationID      string
		ProgramTitle   string
		BroadcastDay   interface{}
		CreatedAt      interface{}
		Cast           string
		NextDate       string
		Recordable     bool
		RecProgramName string
		RecDate        string
		RecStart       string
		RecEnd         string
		RecFt          string
		RecTo          string
	}
	favsWithCast := make([]favWithCast, 0, len(favs))
	hasRecordable := false
	for _, f := range favs {
		cast := ""
		nextDate := ""
		recordable := false
		recProgramName := ""
		recDate := ""
		recStart := ""
		recEnd := ""
		recFt := ""
		recTo := ""
		if h.radikoService != nil {
			entries, err := twoWeekEntries(h.radikoService, f.StationID)
			if err == nil {
				latest := findLatestTimefreeFromEntries(entries, f.ProgramTitle, f.BroadcastDay)

				// 次回放送のキャスト・日付を優先、なければ直近タイムフリー放送を使用
				if next := findNextBroadcastFromEntries(entries, f.ProgramTitle, f.BroadcastDay); next != nil {
					cast, _ = next["cast"].(string)
					nextDate, _ = next["date"].(string)
				}
				if cast == "" && latest != nil {
					cast, _ = latest["cast"].(string)
					nextDate, _ = latest["date"].(string)
				}
				if latest != nil {
					recDate, _ = latest["date"].(string)
					recStart, _ = latest["start"].(string)
					recEnd, _ = latest["end"].(string)
					recFt, recTo = recordingTimesFromEntry(latest)
					if recFt != "" && recTo != "" {
						recordable = true
						hasRecordable = true
						recProgramName = f.ProgramTitle
					}
				}
			}
		}
		favsWithCast = append(favsWithCast, favWithCast{
			ID:             f.ID,
			StationID:      f.StationID,
			ProgramTitle:   f.ProgramTitle,
			BroadcastDay:   f.BroadcastDay,
			CreatedAt:      f.CreatedAt,
			Cast:           cast,
			NextDate:       nextDate,
			Recordable:     recordable,
			RecProgramName: recProgramName,
			RecDate:        recDate,
			RecStart:       recStart,
			RecEnd:         recEnd,
			RecFt:          recFt,
			RecTo:          recTo,
		})
	}

	renderOrJSON(w, r, "web/templates/favorite/index.html", map[string]interface{}{
		"Favorites":     favsWithCast,
		"HasRecordable": hasRecordable,
	})
}

// Store は POST /favorites を処理する。お気に入り追加。
func (h *FavoriteHandler) Store(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		StationID    string `json:"station_id"`
		ProgramTitle string `json:"program_title"`
		BroadcastDay *int   `json:"broadcast_day"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}

	if req.StationID == "" || req.ProgramTitle == "" {
		writeError(w, http.StatusUnprocessableEntity, "station_id と program_title は必須です")
		return
	}

	fav, err := h.favService.Add(userID, req.StationID, req.ProgramTitle, req.BroadcastDay)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "お気に入りの追加に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "ok",
		"favorite": fav,
	})
}

// Destroy は POST /favorites/delete を処理する。お気に入り削除。
func (h *FavoriteHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		StationID    string `json:"station_id"`
		ProgramTitle string `json:"program_title"`
		BroadcastDay *int   `json:"broadcast_day"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}

	if req.StationID == "" || req.ProgramTitle == "" {
		writeError(w, http.StatusUnprocessableEntity, "station_id と program_title は必須です")
		return
	}

	if err := h.favService.Remove(userID, req.StationID, req.ProgramTitle, req.BroadcastDay); err != nil {
		writeError(w, http.StatusInternalServerError, "お気に入りの削除に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// Check は GET /favorites/check を処理する。お気に入り確認。
func (h *FavoriteHandler) Check(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeJSON(w, http.StatusOK, map[string]bool{"is_favorite": false})
		return
	}

	stationID := r.URL.Query().Get("station_id")
	programTitle := r.URL.Query().Get("program_title")

	if stationID == "" || programTitle == "" {
		writeError(w, http.StatusBadRequest, "station_id と program_title は必須です")
		return
	}

	// broadcast_day はオプション
	var broadcastDay *int
	if bdStr := r.URL.Query().Get("broadcast_day"); bdStr != "" {
		bd, err := parseInt(bdStr)
		if err == nil {
			broadcastDay = &bd
		}
	}

	exists, err := h.favService.Check(userID, stationID, programTitle, broadcastDay)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "お気に入り確認に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"is_favorite": exists})
}

type recordAllFavoriteItem struct {
	ProgramName string `json:"program_name"`
	StationID   string `json:"station_id"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	RecordingID string `json:"recording_id,omitempty"`
}

type recordAllFavoritesResponse struct {
	Total   int                     `json:"total"`
	Started int                     `json:"started"`
	Skipped int                     `json:"skipped"`
	Items   []recordAllFavoriteItem `json:"items"`
}

// RecordAll は POST /favorites/record-all を処理する。
func (h *FavoriteHandler) RecordAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if h.recorder == nil || h.radikoService == nil {
		writeError(w, http.StatusInternalServerError, "一括録音の初期化が完了していません")
		return
	}

	favs, err := h.favService.GetByUser(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "お気に入り一覧の取得に失敗しました: "+err.Error())
		return
	}

	ownerKey := fmt.Sprintf("user_%d", userID)
	resp := recordAllFavoritesResponse{
		Total: len(favs),
		Items: make([]recordAllFavoriteItem, 0, len(favs)),
	}

	for _, f := range favs {
		item := recordAllFavoriteItem{
			ProgramName: f.ProgramTitle,
			StationID:   f.StationID,
			Status:      "skipped",
		}

		entries, err := twoWeekEntries(h.radikoService, f.StationID)
		if err != nil {
			item.Reason = "番組表の取得に失敗しました"
			resp.Skipped++
			resp.Items = append(resp.Items, item)
			continue
		}

		latest := findLatestTimefreeFromEntries(entries, f.ProgramTitle, f.BroadcastDay)
		if latest == nil {
			item.Reason = "録音可能なタイムフリー放送がありません"
			resp.Skipped++
			resp.Items = append(resp.Items, item)
			continue
		}

		startTime, endTime := recordingTimesFromEntry(latest)
		if startTime == "" || endTime == "" {
			item.Reason = "録音対象の時刻情報が不足しています"
			resp.Skipped++
			resp.Items = append(resp.Items, item)
			continue
		}

		recording, err := h.recorder.IsProgramRecording(r.Context(), ownerKey, f.ProgramTitle)
		if err != nil {
			item.Reason = "録音状態の確認に失敗しました"
			resp.Skipped++
			resp.Items = append(resp.Items, item)
			continue
		}
		if recording {
			item.Reason = "同じ番組が録音中です"
			resp.Skipped++
			resp.Items = append(resp.Items, item)
			continue
		}

		recordingID, err := h.recorder.StartTimefree(
			r.Context(),
			ownerKey,
			f.StationID,
			f.ProgramTitle,
			startTime,
			endTime,
			"",
		)
		if err != nil {
			item.Reason = err.Error()
			resp.Skipped++
			resp.Items = append(resp.Items, item)
			continue
		}

		item.Status = "started"
		item.Reason = ""
		item.RecordingID = recordingID
		resp.Started++
		resp.Items = append(resp.Items, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

// parseInt は文字列を int に変換するヘルパー。
func parseInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	return n, err
}

func recordingTimesFromEntry(entry map[string]interface{}) (string, string) {
	ft, _ := entry["ft"].(string)
	to, _ := entry["to"].(string)
	if len(ft) == 14 && len(to) == 14 {
		return ft, to
	}

	date, _ := entry["date"].(string)
	start, _ := entry["start"].(string)
	end, _ := entry["end"].(string)
	if date == "" || start == "" || end == "" {
		return "", ""
	}
	return date + strings.ReplaceAll(start, ":", ""), date + strings.ReplaceAll(end, ":", "")
}
