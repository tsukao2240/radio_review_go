package handler

import (
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/tsukao2240/radio_review_go/internal/service"
)

// PasswordResetHandler はパスワードリセット関連の HTTP ハンドラーを管理する。
type PasswordResetHandler struct {
	svc service.PasswordResetServiceInterface
}

// NewPasswordResetHandler は新しい PasswordResetHandler を返す。
func NewPasswordResetHandler(svc service.PasswordResetServiceInterface) *PasswordResetHandler {
	return &PasswordResetHandler{svc: svc}
}

// ShowRequestForm は GET /password/reset を処理する。
// パスワードリセット申請フォームを表示する。
func (h *PasswordResetHandler) ShowRequestForm(w http.ResponseWriter, r *http.Request) {
	RenderWithBase(w, r, "web/templates/auth/passwords/email.html", nil)
}

// SendResetLink は POST /password/email を処理する。
// 指定メールアドレスにリセットリンクを送信する。
func (h *PasswordResetHandler) SendResetLink(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストのパースに失敗しました")
		return
	}

	email := r.FormValue("email")
	if email == "" {
		RenderWithBase(w, r, "web/templates/auth/passwords/email.html", map[string]interface{}{
			"Error": "メールアドレスは必須です",
		})
		return
	}

	// 存在しないメールでもエラーを返さない（列挙攻撃対策）
	_ = h.svc.SendResetLink(email)

	success := "パスワードリセットのリンクをメールで送信しました"
	if strings.ToLower(os.Getenv("MAIL_MAILER")) != "smtp" {
		success = "パスワードリセットのリンクをログに出力しました"
	}
	RenderWithBase(w, r, "web/templates/auth/passwords/email.html", map[string]interface{}{
		"Success": success,
	})
}

// ShowResetForm は GET /password/reset/{token} を処理する。
// パスワード再設定フォームを表示する。
func (h *PasswordResetHandler) ShowResetForm(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	email := r.URL.Query().Get("email")

	RenderWithBase(w, r, "web/templates/auth/passwords/reset.html", map[string]interface{}{
		"Token": token,
		"Email": email,
	})
}

// Reset は POST /password/update を処理する。
// トークンを検証して新しいパスワードに更新する。
func (h *PasswordResetHandler) Reset(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "リクエストのパースに失敗しました")
		return
	}

	email := r.FormValue("email")
	token := r.FormValue("token")
	password := r.FormValue("password")
	passwordConfirmation := r.FormValue("password_confirmation")

	if err := h.svc.Reset(email, token, password, passwordConfirmation); err != nil {
		RenderWithBase(w, r, "web/templates/auth/passwords/reset.html", map[string]interface{}{
			"Token": token,
			"Email": email,
			"Error": err.Error(),
		})
		return
	}

	http.Redirect(w, r, "/login?reset=1", http.StatusFound)
}
