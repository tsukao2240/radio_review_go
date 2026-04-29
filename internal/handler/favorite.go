package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/service"
)

// FavoriteHandler はお気に入り番組関連の HTTP ハンドラーを管理する。
type FavoriteHandler struct {
	favService   service.FavoriteServiceInterface
	radikoService service.RadikoApiServiceInterface
	store        sessions.Store
}

// NewFavoriteHandler は新しい FavoriteHandler を返す。
func NewFavoriteHandler(favService service.FavoriteServiceInterface, radikoService service.RadikoApiServiceInterface, store sessions.Store) *FavoriteHandler {
	return &FavoriteHandler{
		favService:   favService,
		radikoService: radikoService,
		store:        store,
	}
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
		ID           int64
		StationID    string
		ProgramTitle string
		BroadcastDay interface{}
		CreatedAt    interface{}
		Cast         string
		NextDate     string
	}
	favsWithCast := make([]favWithCast, 0, len(favs))
	for _, f := range favs {
		cast := ""
		nextDate := ""
		if h.radikoService != nil {
			// 次回放送のキャスト・日付を優先、なければ直近タイムフリー放送を使用
			if next := findNextBroadcast(h.radikoService, f.StationID, f.ProgramTitle, f.BroadcastDay); next != nil {
				cast, _ = next["cast"].(string)
				nextDate, _ = next["date"].(string)
			}
			if cast == "" {
				if latest := findLatestTimefree(h.radikoService, f.StationID, f.ProgramTitle); latest != nil {
					cast, _ = latest["cast"].(string)
					nextDate, _ = latest["date"].(string)
				}
			}
		}
		favsWithCast = append(favsWithCast, favWithCast{
			ID:           f.ID,
			StationID:    f.StationID,
			ProgramTitle: f.ProgramTitle,
			BroadcastDay: f.BroadcastDay,
			CreatedAt:    f.CreatedAt,
			Cast:         cast,
			NextDate:     nextDate,
		})
	}

	renderOrJSON(w, r, "web/templates/favorite/index.html", map[string]interface{}{
		"Favorites": favsWithCast,
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

// parseInt は文字列を int に変換するヘルパー。
func parseInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	return n, err
}
