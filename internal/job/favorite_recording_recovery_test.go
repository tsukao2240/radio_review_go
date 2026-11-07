package job

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

func TestRecoveryPreservesInterruptedAttemptUntilRedisIsAvailable(t *testing.T) {
	s, mock, mr := newFavoriteRecordingScheduler(t)
	sc := *testRecordingSchedule()
	key, id := "auto", "interrupted"
	sc.AutoRecordingKey, sc.RecordingID, sc.Status = &key, &id, "recording"
	mr.Set("recording_"+id, `{"recording_id":"interrupted","status":"recording"}`)
	mr.SetError("LOADING Redis is starting")
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'recording'").WillReturnRows(favoriteScheduleRows(sc))
	if err := s.recoverInterruptedFavoriteRecordings(context.Background()); err == nil || !strings.Contains(err.Error(), "LOADING") {
		t.Fatalf("Redis failure must leave the attempt recoverable, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	mr.SetError("")
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'recording'").WillReturnRows(favoriteScheduleRows(sc))
	mock.ExpectExec("UPDATE recording_schedules.*SET status = 'pending', recording_id = NULL").
		WithArgs(sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.recoverInterruptedFavoriteRecordings(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := mr.Get("recording_" + id)
	if err != nil || !strings.Contains(raw, `"status":"failed"`) {
		t.Fatalf("abandoned attempt must stop blocking a new download: %s, %v", raw, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSweepProtectsCompletedAudioWithoutRedisMetadata(t *testing.T) {
	for _, databaseUnavailable := range []bool{false, true} {
		t.Run(fmt.Sprintf("database_unavailable_%v", databaseUnavailable), func(t *testing.T) {
			s, mock, _ := newFavoriteRecordingScheduler(t)
			path := filepath.Join(s.storagePath, "saved.m4a")
			orphan := filepath.Join(s.storagePath, "orphan.m4a")
			old := time.Now().Add(-26 * time.Hour)
			for _, p := range []string{path, orphan} {
				if err := os.WriteFile(p, []byte("audio"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(p, old, old); err != nil {
					t.Fatal(err)
				}
			}
			sc := *testRecordingSchedule()
			key := "auto"
			sc.AutoRecordingKey, sc.Status = &key, "completed"
			info := model.RecordingInfo{RecordingID: "saved", StationID: sc.StationID, OwnerKey: fmt.Sprintf("user_%d", sc.UserID), Status: "completed", FilePath: path}
			metadata, err := json.Marshal(info)
			if err != nil {
				t.Fatal(err)
			}
			raw := string(metadata)
			sc.RecordingMetadata = &raw
			query := mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'completed' AND recording_metadata IS NOT NULL")
			if databaseUnavailable {
				query.WillReturnError(errors.New("database temporarily unavailable"))
			} else {
				query.WillReturnRows(favoriteScheduleRows(sc))
			}
			s.sweepOrphanRecordingFiles(context.Background())
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("completed audio must survive Redis loss: %v", err)
			}
			_, err = os.Stat(orphan)
			if databaseUnavailable && err != nil {
				t.Fatalf("cleanup must wait until database records can be checked: %v", err)
			}
			if !databaseUnavailable && !os.IsNotExist(err) {
				t.Fatalf("unreferenced old audio should still be cleaned: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type datetimeNear struct{ want time.Time }

func (a datetimeNear) Match(value driver.Value) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	at, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	return err == nil && at.Sub(a.want) > -2*time.Second && at.Sub(a.want) < 2*time.Second
}

func TestProcessRecordingSchedulesUsesCurrentTimeForRetryDeadline(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM recording_schedules WHERE status = 'pending'").
		WithArgs(datetimeNear{now.Add(-timefreeAvailabilityDelay)}, datetimeNear{now}).
		WillReturnRows(favoriteScheduleRows())
	s.processRecordingSchedules(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type blockingFavoriteDownloader struct {
	started chan struct{}
	release chan struct{}
}

func (d *blockingFavoriteDownloader) DownloadTimefree(ctx context.Context, _, _, _, _, _, path string) error {
	close(d.started)
	select {
	case <-d.release:
		return os.WriteFile(path, []byte("audio"), 0644)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestRecoveryLeavesActiveDownloadRunning(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	s, mock, _ := newFavoriteRecordingScheduler(t)
	sc := testRecordingSchedule()
	key := "auto"
	sc.AutoRecordingKey = &key
	sc.ScheduledStartTime = time.Now().Add(-2 * time.Hour).Truncate(time.Minute)
	sc.ScheduledEndTime = sc.ScheduledStartTime.Add(time.Hour)
	d := &blockingFavoriteDownloader{started: make(chan struct{}), release: make(chan struct{})}
	s.hlsDownloader = d
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	expectAutomaticClaim(mock, sc, true)
	mock.ExpectExec("UPDATE recording_schedules SET recording_id = \\?").WithArgs(sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.startScheduledRecording(ctx, sc)
	}()
	select {
	case <-d.started:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	stored := *sc
	stored.Status = "recording"
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'recording'").WillReturnRows(favoriteScheduleRows(stored))
	err := s.recoverInterruptedFavoriteRecordings(ctx)
	if err != nil {
		cancel()
		<-done
		t.Fatalf("reconciliation must skip a live worker: %v", err)
	}
	mock.ExpectExec("UPDATE recording_schedules SET status = 'completed', recording_id = \\?, recording_metadata = \\?").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	close(d.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not finish")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconciliationRestoresRedisMetadataAfterRuntimeLoss(t *testing.T) {
	s, mock, mr := newFavoriteRecordingScheduler(t)
	s.SetFavoriteScheduleService(&stubFavoriteSchedules{calls: map[string]int{}})
	sc := *testRecordingSchedule()
	key := "auto"
	sc.AutoRecordingKey, sc.Status = &key, "completed"
	path := filepath.Join(s.storagePath, "saved.m4a")
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	info := model.RecordingInfo{RecordingID: "saved", StationID: sc.StationID, OwnerKey: fmt.Sprintf("user_%d", sc.UserID), Status: "completed", FilePath: path}
	metadata, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(metadata)
	sc.RecordingMetadata = &raw
	for i := 0; i < 2; i++ {
		mr.FlushAll()
		mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'recording'").WillReturnRows(favoriteScheduleRows())
		mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'completed' AND recording_metadata IS NOT NULL").WillReturnRows(favoriteScheduleRows(sc))
		mock.ExpectQuery("SELECT \\* FROM favorite_programs").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'pending'").WillReturnRows(favoriteScheduleRows())
		mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'pending'").WillReturnRows(favoriteScheduleRows())
		s.reconcileFavoriteRecordings(context.Background())
		if !mr.Exists("recording_saved") || mr.TTL("recording_saved") != 0 {
			t.Fatalf("reconciliation %d did not restore the recording", i)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconciliationRecoversAfterRetryStateWriteFails(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	s.SetFavoriteScheduleService(&stubFavoriteSchedules{calls: map[string]int{}})
	sc := testRecordingSchedule()
	key := "auto"
	sc.AutoRecordingKey = &key
	s.radikoClient = stubSchedulerRadikoClient{err: errors.New("network unavailable")}
	expectAutomaticClaim(mock, sc, true)
	mock.ExpectExec("UPDATE recording_schedules SET retry_count = retry_count \\+ 1").WillReturnError(errors.New("database unavailable"))
	s.startScheduledRecording(context.Background(), sc)

	stored := *sc
	stored.Status = "recording"
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'recording'").WillReturnRows(favoriteScheduleRows(stored))
	mock.ExpectExec("UPDATE recording_schedules.*SET status = 'pending', recording_id = NULL").WithArgs(sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'completed' AND recording_metadata IS NOT NULL").WillReturnRows(favoriteScheduleRows())
	mock.ExpectQuery("SELECT \\* FROM favorite_programs").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'pending'").WillReturnRows(favoriteScheduleRows())
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'pending'").WillReturnRows(favoriteScheduleRows())
	s.reconcileFavoriteRecordings(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("failed state update must recover without restarting the app: %v", err)
	}
}
