package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"

	"github.com/gorilla/sessions"
	"github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/internal/service"
)

// ScheduleHandler は録音予約関連の HTTP ハンドラーを管理する。
type ScheduleHandler struct {
	service service.RecordingScheduleServiceInterface
	store   sessions.Store
}

// NewScheduleHandler は新しい ScheduleHandler を返す。
func NewScheduleHandler(svc service.RecordingScheduleServiceInterface, store sessions.Store) *ScheduleHandler {
	return &ScheduleHandler{
		service: svc,
		store:   store,
	}
}

// Index は GET /recording/schedules を処理する。
// 録音予約一覧ページを HTML テンプレートで返す。
func (h *ScheduleHandler) Index(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	schedules, err := h.service.GetByUser(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "録音予約の取得に失敗しました: "+err.Error())
		return
	}

	if schedules == nil {
		schedules = []model.RecordingSchedule{}
	}

	tmpl, tmplErr := template.ParseFiles(
		"web/templates/layouts/base.html",
		"web/templates/recording/schedules.html",
	)
	if tmplErr != nil {
		writeError(w, http.StatusInternalServerError, "テンプレートのパースに失敗しました: "+tmplErr.Error())
		return
	}

	data := TemplateData{
		Data: map[string]interface{}{
			"Schedules": schedules,
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		writeError(w, http.StatusInternalServerError, "テンプレートのレンダリングに失敗しました: "+err.Error())
	}
}

// Store は POST /recording/schedule を処理する。
// 新規録音予約を作成する。
// リクエストボディ: {"station_id": "TBS", "program_title": "...", "start_time": "...", "end_time": "..."}
func (h *ScheduleHandler) Store(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}

	var req struct {
		StationID    string `json:"station_id"`
		ProgramTitle string `json:"program_title"`
		StartTime    string `json:"start_time"`
		EndTime      string `json:"end_time"`
	}

	// フォームデータ or JSON を両方受け付ける
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/x-www-form-urlencoded" || r.Header.Get("Content-Type") == "multipart/form-data" {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "リクエストのパースに失敗しました")
			return
		}
		req.StationID = r.FormValue("station_id")
		req.ProgramTitle = r.FormValue("program_title")
		req.StartTime = r.FormValue("start_time")
		req.EndTime = r.FormValue("end_time")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
			return
		}
	}

	if req.StationID == "" || req.ProgramTitle == "" || req.StartTime == "" || req.EndTime == "" {
		writeError(w, http.StatusUnprocessableEntity, "station_id, program_title, start_time, end_time は必須です")
		return
	}

	schedule, err := h.service.Schedule(userID, req.StationID, req.ProgramTitle, req.StartTime, req.EndTime)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"success":  true,
		"schedule": schedule,
	})
}

// Cancel は POST /recording/schedule/cancel を処理する。
// 録音予約をキャンセルする。
// リクエストボディ: {"schedule_id": 1}
func (h *ScheduleHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}

	var req struct {
		ScheduleID int64 `json:"schedule_id"`
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "application/x-www-form-urlencoded" {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "リクエストのパースに失敗しました")
			return
		}
		id, parseErr := strconv.ParseInt(r.FormValue("schedule_id"), 10, 64)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "schedule_id のパースに失敗しました")
			return
		}
		req.ScheduleID = id
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
			return
		}
	}

	if req.ScheduleID == 0 {
		writeError(w, http.StatusUnprocessableEntity, "schedule_id は必須です")
		return
	}

	if err := h.service.Cancel(req.ScheduleID, userID); err != nil {
		switch err.Error() {
		case "録音予約が見つかりません":
			writeError(w, http.StatusNotFound, err.Error())
		case "この録音予約をキャンセルする権限がありません":
			writeError(w, http.StatusForbidden, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "キャンセルに失敗しました: "+err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"schedule_id": req.ScheduleID,
	})
}
