package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/sessions"
	appmiddleware "github.com/yourname/radio_review_go/internal/middleware"
	"github.com/yourname/radio_review_go/internal/model"
)

const baseTmpl = "web/templates/layouts/base.html"

// TemplateData は各ページテンプレートに渡す共通データ構造体。
type TemplateData struct {
	User      *model.User // ログイン中のユーザー（未ログインは nil）
	Nonce     string      // CSP の script-src nonce
	CSRFToken string      // CSRF トークン（セッションから）
	Data      interface{} // ページ固有のデータ
	Flash     string      // フラッシュメッセージ
}

// ResolveUser はリクエストからログインユーザーを取得するための注入可能な関数。
// main.go で初期化される。
var ResolveUser func(r *http.Request) *model.User

// FlashStore はフラッシュメッセージ用セッションストア。main.go で初期化される。
var FlashStore sessions.Store

const flashSessionName = "radio_review_flash"
const flashKey = "flash"

// SetFlash はセッションにフラッシュメッセージを保存する。
func SetFlash(r *http.Request, w http.ResponseWriter, message string) {
	if FlashStore == nil {
		return
	}
	sess, err := FlashStore.Get(r, flashSessionName)
	if err != nil {
		return
	}
	sess.Values[flashKey] = message
	_ = sess.Save(r, w)
}

// getAndClearFlash はセッションからフラッシュメッセージを取得してクリアする。
func getAndClearFlash(r *http.Request, w http.ResponseWriter) string {
	if FlashStore == nil {
		return ""
	}
	sess, err := FlashStore.Get(r, flashSessionName)
	if err != nil {
		return ""
	}
	val, ok := sess.Values[flashKey]
	if !ok {
		return ""
	}
	msg, _ := val.(string)
	delete(sess.Values, flashKey)
	_ = sess.Save(r, w)
	return msg
}

// generateCSRFToken はリクエストごとにランダムな CSRF トークンを生成する。
func generateCSRFToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}

// RenderWithBase は base.html + tmplPath を ParseFiles して "base" テンプレートを実行する。
// テンプレートファイルが存在しない場合は JSON にフォールバックする。
func RenderWithBase(w http.ResponseWriter, r *http.Request, tmplPath string, data interface{}) {
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(data); encErr != nil {
			log.Printf("RenderWithBase json encode error: %v", encErr)
		}
		return
	}

	t, err := template.ParseFiles(baseTmpl, tmplPath)
	if err != nil {
		log.Printf("RenderWithBase ParseFiles error (%s): %v", tmplPath, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	var user *model.User
	if ResolveUser != nil {
		user = ResolveUser(r)
	}

	nonce := appmiddleware.GetNonce(r.Context())
	csrfToken := generateCSRFToken()

	td := TemplateData{
		User:      user,
		Nonce:     nonce,
		CSRFToken: csrfToken,
		Data:      data,
		Flash:     getAndClearFlash(r, w),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", td); err != nil {
		log.Printf("RenderWithBase ExecuteTemplate error (%s): %v", tmplPath, err)
	}
}
