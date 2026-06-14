package repository

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByID(id int64) (*model.User, error) {
	var u model.User
	err := r.db.Get(&u, "SELECT * FROM users WHERE id = ? LIMIT 1", id)
	if err != nil {
		return nil, fmt.Errorf("repository.UserRepository.FindByID: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var u model.User
	err := r.db.Get(&u, "SELECT * FROM users WHERE email = ? LIMIT 1", email)
	if err != nil {
		return nil, fmt.Errorf("repository.UserRepository.FindByEmail: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) FindByFeedToken(token string) (*model.User, error) {
	var u model.User
	err := r.db.Get(&u, "SELECT * FROM users WHERE feed_token = ? LIMIT 1", token)
	if err != nil {
		return nil, fmt.Errorf("repository.UserRepository.FindByFeedToken: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) Create(user *model.User) (int64, error) {
	if user.FeedToken == "" {
		token, err := newFeedToken()
		if err != nil {
			return 0, fmt.Errorf("repository.UserRepository.Create feed token: %w", err)
		}
		user.FeedToken = token
	}
	res, err := r.db.NamedExec(
		`INSERT INTO users (name, email, email_verified_at, password, remember_token, feed_token, created_at, updated_at)
		 VALUES (:name, :email, :email_verified_at, :password, :remember_token, :feed_token, NOW(), NOW())`,
		user,
	)
	if err != nil {
		return 0, fmt.Errorf("repository.UserRepository.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("repository.UserRepository.Create LastInsertId: %w", err)
	}
	return id, nil
}

func (r *UserRepository) Update(user *model.User) error {
	_, err := r.db.NamedExec(
		`UPDATE users SET name=:name, email=:email, password=:password,
		 remember_token=:remember_token, updated_at=NOW() WHERE id=:id`,
		user,
	)
	if err != nil {
		return fmt.Errorf("repository.UserRepository.Update: %w", err)
	}
	return nil
}

func newFeedToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
