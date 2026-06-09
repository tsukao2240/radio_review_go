package repository

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

func newPushSubscriptionRepoMock(t *testing.T) (*PushSubscriptionRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	db := sqlx.NewDb(rawDB, "sqlmock")
	return NewPushSubscriptionRepository(db), mock, func() { rawDB.Close() }
}

func pushSubscriptionRows() *sqlmock.Rows {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "user_id", "endpoint", "endpoint_hash", "p256dh", "auth", "user_agent", "created_at", "updated_at",
	}).AddRow(int64(1), int64(7), "https://push.example/sub", EndpointHash("https://push.example/sub"), "p256dh", "auth", "ua", now, now)
}

func TestPushSubscriptionRepositoryFindByUser(t *testing.T) {
	repo, mock, cleanup := newPushSubscriptionRepoMock(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM push_subscriptions
		 WHERE user_id = ?
		 ORDER BY updated_at DESC`)).
		WithArgs(int64(7)).
		WillReturnRows(pushSubscriptionRows())

	got, err := repo.FindByUser(7)
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if len(got) != 1 || got[0].Endpoint != "https://push.example/sub" {
		t.Fatalf("unexpected subscriptions: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPushSubscriptionRepositoryUpsert(t *testing.T) {
	repo, mock, cleanup := newPushSubscriptionRepoMock(t)
	defer cleanup()

	ua := "ua"
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO push_subscriptions (user_id, endpoint, endpoint_hash, p256dh, auth, user_agent, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
		 ON DUPLICATE KEY UPDATE
		   endpoint = VALUES(endpoint),
		   p256dh = VALUES(p256dh),
		   auth = VALUES(auth),
		   user_agent = VALUES(user_agent),
		   updated_at = NOW()`)).
		WithArgs(int64(7), "https://push.example/sub", EndpointHash("https://push.example/sub"), "p256dh", "auth", &ua).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Upsert(&model.PushSubscription{
		UserID:    7,
		Endpoint:  "https://push.example/sub",
		P256dh:    "p256dh",
		Auth:      "auth",
		UserAgent: &ua,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPushSubscriptionRepositoryDelete(t *testing.T) {
	repo, mock, cleanup := newPushSubscriptionRepoMock(t)
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM push_subscriptions WHERE user_id = ? AND endpoint_hash = ?")).
		WithArgs(int64(7), EndpointHash("https://push.example/sub")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Delete(7, "https://push.example/sub"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
