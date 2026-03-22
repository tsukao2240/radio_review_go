package handler

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"

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

// RenderWithBase は base.html + tmplPath を ParseFiles して "base" テンプレートを実行する。
// テンプレートファイルが存在しない場合は JSON にフォールバックする。
func RenderWithBase(w http.ResponseWriter, tmplPath string, data interface{}) {
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("RenderWithBase ExecuteTemplate error (%s): %v", tmplPath, err)
	}
}
