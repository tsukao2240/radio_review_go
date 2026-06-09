package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tsukao2240/radio_review_go/internal/repository"
)

// パスワードリセットトークンの有効期限（60分）
const passwordResetTTL = 60 * time.Minute

// PasswordResetServiceInterface はパスワードリセットのサービスインターフェース。
type PasswordResetServiceInterface interface {
	// SendResetLink はリセット用トークンを生成してメールを送信（またはログ出力）する。
	// 指定メールアドレスのユーザーが存在しない場合もエラーを返さない（列挙攻撃対策）。
	SendResetLink(email string) error
	// Reset はトークンを検証して新しいパスワードに更新する。
	Reset(email, token, newPassword, newPasswordConfirmation string) error
}

// PasswordResetService はパスワードリセットのビジネスロジックを担当する。
type PasswordResetService struct {
	resetRepo repository.PasswordResetRepositoryInterface
	userRepo  repository.UserRepositoryInterface
	mailer    Mailer
}

// NewPasswordResetService は新しい PasswordResetService を返す。
func NewPasswordResetService(
	resetRepo repository.PasswordResetRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
) *PasswordResetService {
	return NewPasswordResetServiceWithMailer(resetRepo, userRepo, &LogMailer{})
}

// NewPasswordResetServiceWithMailer は Mailer を注入して新しい PasswordResetService を返す。
func NewPasswordResetServiceWithMailer(
	resetRepo repository.PasswordResetRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	mailer Mailer,
) *PasswordResetService {
	if mailer == nil {
		mailer = &LogMailer{}
	}
	return &PasswordResetService{
		resetRepo: resetRepo,
		userRepo:  userRepo,
		mailer:    mailer,
	}
}

// SendResetLink はリセットトークンを生成して保存し、メール送信（ログ出力）する。
func (s *PasswordResetService) SendResetLink(email string) error {
	// ユーザー存在確認（存在しなくてもエラーを返さない＝列挙攻撃対策）
	user, err := s.userRepo.FindByEmail(email)
	if err != nil || user == nil {
		log.Printf("[PasswordReset] メールアドレスが見つかりません（無視）: %s", email)
		return nil
	}

	// 32バイトのランダムトークン生成
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("トークン生成エラー: %w", err)
	}
	token := hex.EncodeToString(b)

	if err := s.resetRepo.Save(email, token); err != nil {
		return fmt.Errorf("トークン保存エラー: %w", err)
	}

	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "http://localhost:8080"
	}
	resetURL := fmt.Sprintf("%s/password/reset/%s?email=%s", appURL, token, email)
	body := fmt.Sprintf("以下のURLからパスワードを再設定してください。\n\n%s\n\nこのリンクの有効期限は60分です。", resetURL)
	if err := s.mailer.Send(email, "パスワードリセット", body); err != nil {
		return fmt.Errorf("メール送信エラー: %w", err)
	}

	return nil
}

// Reset はトークンを検証して新しいパスワードに更新する。
func (s *PasswordResetService) Reset(email, token, newPassword, newPasswordConfirmation string) error {
	if newPassword == "" || newPasswordConfirmation == "" {
		return errors.New("パスワードは必須です")
	}
	if newPassword != newPasswordConfirmation {
		return errors.New("パスワードと確認用パスワードが一致しません")
	}
	if len(newPassword) < 8 {
		return errors.New("パスワードは8文字以上で入力してください")
	}

	// トークン取得
	pr, err := s.resetRepo.FindByEmail(email)
	if err != nil || pr == nil {
		return errors.New("無効なパスワードリセットトークンです")
	}

	// トークン一致確認
	if pr.Token != token {
		return errors.New("無効なパスワードリセットトークンです")
	}

	// 有効期限確認（60分）
	if time.Since(pr.CreatedAt) > passwordResetTTL {
		if err := s.resetRepo.Delete(email); err != nil {
			log.Printf("[PasswordReset] expired token delete error: %v", err)
		}
		return errors.New("パスワードリセットトークンの有効期限が切れています。再度リセットを申請してください")
	}

	// ユーザー取得
	user, err := s.userRepo.FindByEmail(email)
	if err != nil || user == nil {
		return errors.New("ユーザーが見つかりません")
	}

	// パスワードハッシュ化
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return fmt.Errorf("パスワードのハッシュ化に失敗しました: %w", err)
	}

	user.Password = string(hashed)
	if err := s.userRepo.Update(user); err != nil {
		return fmt.Errorf("パスワードの更新に失敗しました: %w", err)
	}

	// 使用済みトークンを削除
	if err := s.resetRepo.Delete(email); err != nil {
		log.Printf("[PasswordReset] used token delete error: %v", err)
	}

	return nil
}

var _ PasswordResetServiceInterface = (*PasswordResetService)(nil)
