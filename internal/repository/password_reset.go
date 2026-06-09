package repository

import (
	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

// PasswordResetRepository は password_resets テーブルの操作を担当する。
type PasswordResetRepository struct {
	db *sqlx.DB
}

// NewPasswordResetRepository は新しい PasswordResetRepository を返す。
func NewPasswordResetRepository(db *sqlx.DB) *PasswordResetRepository {
	return &PasswordResetRepository{db: db}
}

// Save はトークンを保存する。同一メールアドレスの既存レコードを削除してから INSERT する。
func (r *PasswordResetRepository) Save(email, token string) error {
	_, err := r.db.Exec(
		`DELETE FROM password_resets WHERE email = ?`, email,
	)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`INSERT INTO password_resets (email, token, created_at) VALUES (?, ?, NOW())`,
		email, token,
	)
	return err
}

// FindByEmail は指定メールアドレスのトークンレコードを返す。
func (r *PasswordResetRepository) FindByEmail(email string) (*model.PasswordReset, error) {
	var pr model.PasswordReset
	err := r.db.Get(&pr,
		`SELECT email, token, created_at FROM password_resets WHERE email = ? ORDER BY created_at DESC LIMIT 1`,
		email,
	)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

// Delete は指定メールアドレスのトークンを削除する。
func (r *PasswordResetRepository) Delete(email string) error {
	_, err := r.db.Exec(`DELETE FROM password_resets WHERE email = ?`, email)
	return err
}

var _ PasswordResetRepositoryInterface = (*PasswordResetRepository)(nil)
