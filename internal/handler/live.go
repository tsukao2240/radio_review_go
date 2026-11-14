package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/tsukao2240/radio_review_go/internal/middleware"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

// LiveUpdates sends revisions only, never database contents or credentials.
func LiveUpdates(db *sqlx.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		snapshot := func(ctx context.Context) (string, error) {
			hash := sha256.New()
			for _, table := range []string{"favorite_programs", "recording_schedules", "notifications"} {
				rows, err := db.QueryxContext(ctx, "SELECT * FROM "+table+" WHERE user_id = ? ORDER BY id", userID)
				if err != nil {
					return "", err
				}
				for rows.Next() {
					row := map[string]interface{}{}
					if err := rows.MapScan(row); err != nil {
						_ = rows.Close()
						return "", err
					}
					data, _ := json.Marshal(row)
					_, _ = hash.Write(data)
				}
				err = rows.Err()
				_ = rows.Close()
				if err != nil {
					return "", err
				}
			}
			var keys []string
			var cursor uint64
			for {
				batch, next, err := rdb.Scan(ctx, cursor, "recording_*", 100).Result()
				if err != nil {
					return "", err
				}
				keys = append(keys, batch...)
				cursor = next
				if cursor == 0 {
					break
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				raw, err := rdb.Get(ctx, key).Result()
				if err == redis.Nil {
					continue
				}
				if err != nil {
					return "", err
				}
				var info model.RecordingInfo
				if json.Unmarshal([]byte(raw), &info) == nil && info.OwnerKey == fmt.Sprintf("user_%d", userID) {
					_, _ = hash.Write([]byte(key))
					_, _ = hash.Write([]byte(raw))
				}
			}
			return hex.EncodeToString(hash.Sum(nil)), nil
		}
		liveEvents(snapshot, 500*time.Millisecond)(w, r)
	}
}

func liveEvents(snapshot func(context.Context) (string, error), interval time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		previous, err := snapshot(r.Context())
		if err != nil {
			http.Error(w, "Live updates unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		controller := http.NewResponseController(w)
		_ = controller.SetWriteDeadline(time.Time{})
		if _, err := fmt.Fprintf(w, "retry: 1000\nevent: ready\ndata: %s\n\n", previous); err != nil {
			return
		}
		if controller.Flush() != nil {
			return
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				current, err := snapshot(r.Context())
				if err != nil {
					return
				}
				if current == previous {
					continue
				}
				previous = current
				if _, err := fmt.Fprintf(w, "event: changed\ndata: %s\n\n", current); err != nil {
					return
				}
				if controller.Flush() != nil {
					return
				}
			}
		}
	}
}
