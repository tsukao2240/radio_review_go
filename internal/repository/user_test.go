package repository

import (
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

func newUserRepoMock(t *testing.T) (*UserRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	db := sqlx.NewDb(rawDB, "sqlmock")
	return NewUserRepository(db), mock, func() { rawDB.Close() }
}

func userRows() *sqlmock.Rows {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "name", "email", "email_verified_at", "password", "remember_token", "feed_token", "created_at", "updated_at",
	}).AddRow(int64(7), "alice", "alice@example.com", nil, "hash", nil, "feed-token", now, now)
}

func TestUserRepositoryFindByID(t *testing.T) {
	repo, mock, cleanup := newUserRepoMock(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users WHERE id = ? LIMIT 1")).
		WithArgs(int64(7)).
		WillReturnRows(userRows())

	got, err := repo.FindByID(7)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ID != 7 || got.Email != "alice@example.com" {
		t.Fatalf("unexpected user: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepositoryFindByEmail(t *testing.T) {
	repo, mock, cleanup := newUserRepoMock(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users WHERE email = ? LIMIT 1")).
		WithArgs("alice@example.com").
		WillReturnRows(userRows())

	got, err := repo.FindByEmail("alice@example.com")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if got.ID != 7 || got.Name != "alice" {
		t.Fatalf("unexpected user: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepositoryFindByFeedToken(t *testing.T) {
	repo, mock, cleanup := newUserRepoMock(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users WHERE feed_token = ? LIMIT 1")).
		WithArgs("feed-token").
		WillReturnRows(userRows())

	got, err := repo.FindByFeedToken("feed-token")
	if err != nil {
		t.Fatalf("FindByFeedToken: %v", err)
	}
	if got.ID != 7 || got.FeedToken != "feed-token" {
		t.Fatalf("unexpected user: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepositoryCreate(t *testing.T) {
	repo, mock, cleanup := newUserRepoMock(t)
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users (name, email, email_verified_at, password, remember_token, feed_token, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`)).
		WithArgs("alice", "alice@example.com", nil, "hash", nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(9, 1))

	user := &model.User{
		Name:     "alice",
		Email:    "alice@example.com",
		Password: "hash",
	}
	id, err := repo.Create(user)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != 9 {
		t.Fatalf("id = %d, want 9", id)
	}
	if len(user.FeedToken) != 64 {
		t.Fatalf("FeedToken length = %d, want 64", len(user.FeedToken))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepositoryUpdate(t *testing.T) {
	repo, mock, cleanup := newUserRepoMock(t)
	defer cleanup()

	token := "remember"
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET name=?, email=?, password=?,
		 remember_token=?, updated_at=NOW() WHERE id=?`)).
		WithArgs("alice", "alice@example.com", "hash2", &token, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Update(&model.User{
		ID:            7,
		Name:          "alice",
		Email:         "alice@example.com",
		Password:      "hash2",
		RememberToken: &token,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUserRepositoryFindByIDError(t *testing.T) {
	repo, mock, cleanup := newUserRepoMock(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users WHERE id = ? LIMIT 1")).
		WithArgs(int64(99)).
		WillReturnError(sql.ErrNoRows)

	if _, err := repo.FindByID(99); err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
