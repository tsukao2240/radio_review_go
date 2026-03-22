package repository

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/yourname/radio_review_go/internal/model"
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

func (r *UserRepository) Create(user *model.User) (int64, error) {
	res, err := r.db.NamedExec(
		`INSERT INTO users (name, email, email_verified_at, password, remember_token, created_at, updated_at)
		 VALUES (:name, :email, :email_verified_at, :password, :remember_token, NOW(), NOW())`,
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
