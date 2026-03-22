package handler

import "github.com/yourname/radio_review_go/internal/model"

// TemplateData は各ページテンプレートに渡す共通データ構造体。
type TemplateData struct {
	User      *model.User  // ログイン中のユーザー（未ログインは nil）
	Nonce     string       // CSP の script-src nonce
	CSRFToken string       // CSRF トークン（セッションから）
	Data      interface{}  // ページ固有のデータ
	Flash     string       // フラッシュメッセージ
}
