package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/service"
)

// MypageHandler はマイページ関連の HTTP ハンドラーを管理する。
type MypageHandler struct {
	postService service.PostServiceInterface
	store       sessions.Store
}

// NewMypageHandler は新しい MypageHandler を返す。
func NewMypageHandler(postService service.PostServiceInterface, store sessions.Store) *MypageHandler {
	return &MypageHandler{
		postService: postService,
		store:       store,
	}
}

// Index は GET /my を処理する。自分のレビュー一覧。
func (h *MypageHandler) Index(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	perPage := 20

	posts, total, err := h.postService.GetPostsByUser(userID, perPage, page)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "レビュー一覧の取得に失敗しました: "+err.Error())
		return
	}

	renderOrJSON(w, "web/templates/mypage/index.html", map[string]interface{}{
		"posts":   posts,
		"total":   total,
		"page":    page,
		"perPage": perPage,
	})
}

// Edit は GET /my/edit/{program_id} を処理する。レビュー編集フォーム。
func (h *MypageHandler) Edit(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	postIDStr := chi.URLParam(r, "program_id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "無効なIDです")
		return
	}

	post, err := h.postService.GetPostByID(postID)
	if err != nil {
		writeError(w, http.StatusNotFound, "レビューが見つかりません")
		return
	}

	// 自分の投稿のみ編集可能
	if post.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	tags, _ := h.postService.GetAllTags()

	renderOrJSON(w, "web/templates/mypage/edit.html", map[string]interface{}{
		"post": post,
		"tags": tags,
	})
}

// Update は POST /my/edit/{program_id} を処理する。レビュー更新。
func (h *MypageHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	postIDStr := chi.URLParam(r, "program_id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "無効なIDです")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "フォームのパースに失敗しました")
		return
	}

	ratingStr := r.FormValue("rating")
	rating, _ := strconv.ParseFloat(ratingStr, 64)
	if rating == 0 {
		rating = 3.0
	}

	data := map[string]interface{}{
		"user_id": userID,
		"title":   r.FormValue("title"),
		"body":    r.FormValue("body"),
		"rating":  rating,
	}

	// タグIDの取得
	tagIDStrs := r.Form["tag_ids[]"]
	if len(tagIDStrs) == 0 {
		tagIDStrs = r.Form["tag_ids"]
	}
	var tagIDs []interface{}
	for _, s := range tagIDStrs {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			tagIDs = append(tagIDs, float64(id))
		}
	}
	data["tag_ids"] = tagIDs

	if err := h.postService.UpdatePost(postID, data); err != nil {
		writeError(w, http.StatusInternalServerError, "レビューの更新に失敗しました: "+err.Error())
		return
	}

	http.Redirect(w, r, "/my", http.StatusFound)
}

// Destroy は POST /my を処理する。レビュー削除。
func (h *MypageHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "フォームのパースに失敗しました")
		return
	}

	postIDStr := r.FormValue("post_id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "無効な post_id です")
		return
	}

	// 自分の投稿か確認
	post, err := h.postService.GetPostByID(postID)
	if err != nil {
		writeError(w, http.StatusNotFound, "レビューが見つかりません")
		return
	}

	if post.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := h.postService.DeletePost(postID); err != nil {
		writeError(w, http.StatusInternalServerError, "レビューの削除に失敗しました: "+err.Error())
		return
	}

	http.Redirect(w, r, "/my", http.StatusFound)
}
