package handler

import (
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/service"
)

// RecommendationHandler はレコメンデーション関連の HTTP ハンドラーを管理する。
type RecommendationHandler struct {
	service service.RecommendationServiceInterface
	store   sessions.Store
}

// NewRecommendationHandler は新しい RecommendationHandler を返す。
func NewRecommendationHandler(svc service.RecommendationServiceInterface, store sessions.Store) *RecommendationHandler {
	return &RecommendationHandler{
		service: svc,
		store:   store,
	}
}

// Index は GET /recommendations を処理する。レコメンデーションページを表示する。
func (h *RecommendationHandler) Index(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	recommendations, err := h.service.GetRecommendations(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "レコメンデーションの取得に失敗しました: "+err.Error())
		return
	}

	trending, err := h.service.GetTrendingPrograms(7, 10)
	if err != nil {
		// トレンド取得失敗は致命的ではないので空スライスで続行
		trending = []map[string]interface{}{}
	}

	renderOrJSON(w, r, "web/templates/recommendations/index.html", map[string]interface{}{
		"Recommendations": recommendations,
		"Trending":        trending,
	})
}

// GetRecommendations は GET /api/recommendations を処理する。レコメンデーションを JSON で返す。
func (h *RecommendationHandler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	recommendations, err := h.service.GetRecommendations(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "レコメンデーションの取得に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    recommendations,
	})
}

// Refresh は POST /api/recommendations/refresh を処理する。
// キャッシュを削除して最新のレコメンデーションを再取得し JSON で返す。
func (h *RecommendationHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.service.ClearUserCache(userID); err != nil {
		writeError(w, http.StatusInternalServerError, "キャッシュの削除に失敗しました: "+err.Error())
		return
	}

	recommendations, err := h.service.GetRecommendations(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "レコメンデーションの取得に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "レコメンデーションを更新しました",
		"data":    recommendations,
	})
}
