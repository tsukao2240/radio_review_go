package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/service"
)

// PostHandler はレビュー投稿関連の HTTP ハンドラーを管理する。
type PostHandler struct {
	postService        service.PostServiceInterface
	interactionService service.PostInteractionServiceInterface
	store              sessions.Store
}

// NewPostHandler は新しい PostHandler を返す。
func NewPostHandler(
	postService service.PostServiceInterface,
	interactionService service.PostInteractionServiceInterface,
	store sessions.Store,
) *PostHandler {
	return &PostHandler{
		postService:        postService,
		interactionService: interactionService,
		store:              store,
	}
}

// renderOrJSON はテンプレートをレンダリングし、存在しない場合は JSON で代替する。
func renderOrJSON(w http.ResponseWriter, tmplPath string, data interface{}) {
	RenderWithBase(w, tmplPath, data)
}

// IndexPrograms は GET /program を処理する。番組一覧（検索付き）。
func (h *PostHandler) IndexPrograms(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	perPage := 20

	keyword := r.URL.Query().Get("keyword")

	var filters map[string]interface{}
	if keyword != "" {
		filters = map[string]interface{}{"keyword": keyword}
	}

	posts, total, err := h.postService.GetPostsFiltered(filters, perPage, page)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "番組一覧の取得に失敗しました: "+err.Error())
		return
	}

	tags, _ := h.postService.GetAllTags()

	renderOrJSON(w, "web/templates/post/index.html", map[string]interface{}{
		"posts":   posts,
		"total":   total,
		"page":    page,
		"perPage": perPage,
		"keyword": keyword,
		"tags":    tags,
	})
}

// ShowReviewForm は GET /review/{id} を処理する。レビュー投稿フォーム（要認証）。
func (h *PostHandler) ShowReviewForm(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	programIDStr := chi.URLParam(r, "id")
	programID, err := strconv.ParseInt(programIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "無効な番組IDです")
		return
	}

	tags, _ := h.postService.GetAllTags()

	renderOrJSON(w, "web/templates/post/create.html", map[string]interface{}{
		"programID": programID,
		"tags":      tags,
	})
}

// CreateReview は POST /review/{id} を処理する。レビュー投稿（要認証）。
func (h *PostHandler) CreateReview(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
		"station_id":    r.FormValue("station_id"),
		"program_title": r.FormValue("program_title"),
		"title":         r.FormValue("title"),
		"body":          r.FormValue("body"),
		"rating":        rating,
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
	if len(tagIDs) > 0 {
		data["tag_ids"] = tagIDs
	}

	post, err := h.postService.CreatePost(data, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "レビューの投稿に失敗しました: "+err.Error())
		return
	}

	stationID := r.FormValue("station_id")
	title := r.FormValue("program_title")
	_ = post

	http.Redirect(w, r, "/list/"+stationID+"/"+title, http.StatusFound)
}

// ListAllReviews は GET /review/list を処理する。全レビュー一覧。
func (h *PostHandler) ListAllReviews(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	perPage := 20

	posts, total, err := h.postService.GetAllPosts(perPage, page)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "レビュー一覧の取得に失敗しました: "+err.Error())
		return
	}

	renderOrJSON(w, "web/templates/post/list_all.html", map[string]interface{}{
		"posts":   posts,
		"total":   total,
		"page":    page,
		"perPage": perPage,
	})
}

// ListReviewsByProgram は GET /list/{station_id}/{title}/review を処理する。番組別レビュー。
func (h *PostHandler) ListReviewsByProgram(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "station_id")
	title := chi.URLParam(r, "title")

	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	perPage := 20

	posts, total, err := h.postService.GetPostsByProgram(stationID, title, perPage, page)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "レビュー一覧の取得に失敗しました: "+err.Error())
		return
	}

	renderOrJSON(w, "web/templates/post/list_each.html", map[string]interface{}{
		"posts":     posts,
		"total":     total,
		"page":      page,
		"perPage":   perPage,
		"stationID": stationID,
		"title":     title,
	})
}

// GetProgramRating は GET /program/{program_id}/rating を処理する。平均評価 JSON。
func (h *PostHandler) GetProgramRating(w http.ResponseWriter, r *http.Request) {
	programIDStr := chi.URLParam(r, "program_id")
	programID, err := strconv.ParseInt(programIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "無効な番組IDです")
		return
	}

	avg, err := h.postService.GetAverageRatingByProgram(programID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "平均評価の取得に失敗しました: "+err.Error())
		return
	}

	// 件数を取得するためフィルタなしで番組別に数える
	_, count, err := h.postService.GetPostsByProgram("", "", 1, 1)
	_ = err

	// 番組IDフィルタで件数を取得
	filters := map[string]interface{}{"program_id": programID}
	_, ratingCount, err := h.postService.GetPostsFiltered(filters, 1, 1)
	if err != nil {
		ratingCount = 0
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"average_rating": avg,
		"count":          ratingCount,
	})
	_ = count
}

// LikePost は POST /api/posts/like を処理する。いいね追加（要認証）。
func (h *PostHandler) LikePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		PostID int64 `json:"post_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}

	if err := h.interactionService.Like(req.PostID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "いいねの追加に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// UnlikePost は POST /api/posts/unlike を処理する。いいね削除（要認証）。
func (h *PostHandler) UnlikePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		PostID int64 `json:"post_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}

	if err := h.interactionService.Unlike(req.PostID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "いいねの削除に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// AddComment は POST /api/posts/comment を処理する。コメント追加（要認証）。
func (h *PostHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		PostID int64  `json:"post_id"`
		Body   string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}

	comment, err := h.interactionService.AddComment(req.PostID, userID, req.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "コメントの追加に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "ok",
		"comment": comment,
	})
}

// DeleteComment は POST /api/posts/comment/delete を処理する。コメント削除（要認証）。
func (h *PostHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		CommentID int64 `json:"comment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}

	if err := h.interactionService.DeleteComment(req.CommentID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "コメントの削除に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// GetComments は GET /api/posts/comments を処理する。コメント一覧（post_id クエリパラメータ）。
func (h *PostHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	postIDStr := r.URL.Query().Get("post_id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "無効な post_id です")
		return
	}

	comments, err := h.interactionService.GetComments(postID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "コメント一覧の取得に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"comments": comments,
	})
}

// CheckLike は GET /api/posts/check-like を処理する。いいね確認。
func (h *PostHandler) CheckLike(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeJSON(w, http.StatusOK, map[string]bool{"is_liked": false})
		return
	}

	postIDStr := r.URL.Query().Get("post_id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "無効な post_id です")
		return
	}

	liked, err := h.interactionService.IsLikedBy(postID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "いいね確認に失敗しました: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"is_liked": liked})
}
