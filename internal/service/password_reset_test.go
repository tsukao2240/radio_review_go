package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

// --- stub: PasswordResetRepository ---

type stubResetRepo struct {
	saveFunc        func(email, token string) error
	findByEmailFunc func(email string) (*model.PasswordReset, error)
	deleteFunc      func(email string) error
}

func (r *stubResetRepo) Save(email, token string) error {
	if r.saveFunc != nil {
		return r.saveFunc(email, token)
	}
	return nil
}
func (r *stubResetRepo) FindByEmail(email string) (*model.PasswordReset, error) {
	if r.findByEmailFunc != nil {
		return r.findByEmailFunc(email)
	}
	return nil, nil
}
func (r *stubResetRepo) Delete(email string) error {
	if r.deleteFunc != nil {
		return r.deleteFunc(email)
	}
	return nil
}

// --- stub: UserRepository ---

type stubUserRepo struct {
	findByIDFunc    func(id int64) (*model.User, error)
	findByEmailFunc func(email string) (*model.User, error)
	createFunc      func(user *model.User) (int64, error)
	updateFunc      func(user *model.User) error
}

func (r *stubUserRepo) FindByID(id int64) (*model.User, error) {
	if r.findByIDFunc != nil {
		return r.findByIDFunc(id)
	}
	return nil, nil
}
func (r *stubUserRepo) FindByEmail(email string) (*model.User, error) {
	if r.findByEmailFunc != nil {
		return r.findByEmailFunc(email)
	}
	return nil, nil
}
func (r *stubUserRepo) FindByFeedToken(token string) (*model.User, error) { return nil, nil }
func (r *stubUserRepo) Create(user *model.User) (int64, error) {
	if r.createFunc != nil {
		return r.createFunc(user)
	}
	return 1, nil
}
func (r *stubUserRepo) Update(user *model.User) error {
	if r.updateFunc != nil {
		return r.updateFunc(user)
	}
	return nil
}

// --- Tests ---

func TestPasswordResetService_SendResetLink(t *testing.T) {
	t.Run("メールが存在しない場合はエラーを返さない（列挙攻撃対策）", func(t *testing.T) {
		userRepo := &stubUserRepo{
			findByEmailFunc: func(email string) (*model.User, error) {
				return nil, errors.New("not found")
			},
		}
		svc := NewPasswordResetService(&stubResetRepo{}, userRepo)
		if err := svc.SendResetLink("notfound@example.com"); err != nil {
			t.Errorf("expected nil error for nonexistent email, got %v", err)
		}
	})

	t.Run("正常: トークン保存を呼ぶ", func(t *testing.T) {
		var savedEmail, savedToken string
		userRepo := &stubUserRepo{
			findByEmailFunc: func(email string) (*model.User, error) {
				return &model.User{ID: 1, Email: email}, nil
			},
		}
		resetRepo := &stubResetRepo{
			saveFunc: func(email, token string) error {
				savedEmail = email
				savedToken = token
				return nil
			},
		}
		svc := NewPasswordResetService(resetRepo, userRepo)
		if err := svc.SendResetLink("user@example.com"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if savedEmail != "user@example.com" {
			t.Errorf("got email=%q", savedEmail)
		}
		if len(savedToken) != 64 { // 32 bytes = 64 hex chars
			t.Errorf("got token length=%d, want 64", len(savedToken))
		}
	})

	t.Run("Mailer 注入: リセットURLを送信する", func(t *testing.T) {
		mailer := &captureMailer{}
		userRepo := &stubUserRepo{
			findByEmailFunc: func(email string) (*model.User, error) {
				return &model.User{ID: 1, Email: email}, nil
			},
		}
		svc := NewPasswordResetServiceWithMailer(&stubResetRepo{}, userRepo, mailer)
		if err := svc.SendResetLink("user@example.com"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mailer.to != "user@example.com" {
			t.Fatalf("got to=%q", mailer.to)
		}
		if !strings.Contains(mailer.body, "/password/reset/") {
			t.Fatalf("reset URL missing from body: %q", mailer.body)
		}
	})

	t.Run("Save エラー: 伝播", func(t *testing.T) {
		saveErr := errors.New("save error")
		userRepo := &stubUserRepo{
			findByEmailFunc: func(email string) (*model.User, error) {
				return &model.User{ID: 1}, nil
			},
		}
		resetRepo := &stubResetRepo{
			saveFunc: func(_, _ string) error { return saveErr },
		}
		svc := NewPasswordResetService(resetRepo, userRepo)
		err := svc.SendResetLink("user@example.com")
		if !errors.Is(err, saveErr) {
			t.Errorf("expected saveErr, got %v", err)
		}
	})
}

func TestPasswordResetService_Reset(t *testing.T) {
	validToken := "abc123"

	t.Run("正常リセット", func(t *testing.T) {
		var updatedUser *model.User
		resetRepo := &stubResetRepo{
			findByEmailFunc: func(email string) (*model.PasswordReset, error) {
				return &model.PasswordReset{Email: email, Token: validToken, CreatedAt: time.Now()}, nil
			},
		}
		userRepo := &stubUserRepo{
			findByEmailFunc: func(email string) (*model.User, error) {
				return &model.User{ID: 1, Email: email}, nil
			},
			updateFunc: func(user *model.User) error {
				updatedUser = user
				return nil
			},
		}
		svc := NewPasswordResetService(resetRepo, userRepo)
		err := svc.Reset("user@example.com", validToken, "newpass123", "newpass123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updatedUser == nil {
			t.Fatal("user was not updated")
		}
		// パスワードはハッシュ化されているはず
		if updatedUser.Password == "newpass123" {
			t.Error("password should be hashed, not plaintext")
		}
	})

	t.Run("パスワード不一致: エラー", func(t *testing.T) {
		svc := NewPasswordResetService(&stubResetRepo{}, &stubUserRepo{})
		err := svc.Reset("u@e.com", validToken, "pass1234", "different")
		if err == nil {
			t.Fatal("expected error for mismatched passwords")
		}
	})

	t.Run("パスワードが8文字未満: エラー", func(t *testing.T) {
		svc := NewPasswordResetService(&stubResetRepo{}, &stubUserRepo{})
		err := svc.Reset("u@e.com", validToken, "short", "short")
		if err == nil {
			t.Fatal("expected error for short password")
		}
	})

	t.Run("トークン不一致: エラー", func(t *testing.T) {
		resetRepo := &stubResetRepo{
			findByEmailFunc: func(email string) (*model.PasswordReset, error) {
				return &model.PasswordReset{Email: email, Token: "correct_token", CreatedAt: time.Now()}, nil
			},
		}
		svc := NewPasswordResetService(resetRepo, &stubUserRepo{})
		err := svc.Reset("u@e.com", "wrong_token", "newpass123", "newpass123")
		if err == nil {
			t.Fatal("expected error for wrong token")
		}
	})

	t.Run("有効期限切れ: エラーを返しトークン削除", func(t *testing.T) {
		var deleted bool
		resetRepo := &stubResetRepo{
			findByEmailFunc: func(email string) (*model.PasswordReset, error) {
				// 2時間前に作成されたトークン（60分TTLを超過）
				return &model.PasswordReset{
					Email:     email,
					Token:     validToken,
					CreatedAt: time.Now().Add(-2 * time.Hour),
				}, nil
			},
			deleteFunc: func(email string) error {
				deleted = true
				return nil
			},
		}
		svc := NewPasswordResetService(resetRepo, &stubUserRepo{})
		err := svc.Reset("u@e.com", validToken, "newpass123", "newpass123")
		if err == nil {
			t.Fatal("expected error for expired token")
		}
		if !deleted {
			t.Error("expired token should be deleted")
		}
	})

	t.Run("トークンが見つからない: エラー", func(t *testing.T) {
		resetRepo := &stubResetRepo{
			findByEmailFunc: func(email string) (*model.PasswordReset, error) {
				return nil, errors.New("not found")
			},
		}
		svc := NewPasswordResetService(resetRepo, &stubUserRepo{})
		err := svc.Reset("u@e.com", validToken, "newpass123", "newpass123")
		if err == nil {
			t.Fatal("expected error when token not found")
		}
	})
}
