package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

type stubFavoriteSchedules struct {
	entries []map[string]interface{}
	calls   map[string]int
	err     error
}

func (s *stubFavoriteSchedules) GetTwoWeekSchedule(stationID string) ([]map[string]interface{}, error) {
	s.calls[stationID]++
	return []map[string]interface{}{{"entries": s.entries}}, s.err
}

func newFavoriteRecordingScheduler(t *testing.T) (*Scheduler, sqlmock.Sqlmock, *miniredis.Miniredis) {
	t.Helper()
	db, mock := newSchedulerSQLMock(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		rdb.Close()
		db.Close()
	})
	return NewScheduler(db, rdb, stubSchedulerRadikoClient{}, stubSchedulerHLSDownloader{}, t.TempDir()), mock, mr
}

func favoriteScheduleRows(schedules ...model.RecordingSchedule) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "user_id", "station_id", "program_title", "scheduled_start_time", "scheduled_end_time", "status", "auto_recording_key", "recording_id", "retry_count", "recording_metadata"})
	for _, sc := range schedules {
		rows.AddRow(sc.ID, sc.UserID, sc.StationID, sc.ProgramTitle, sc.ScheduledStartTime, sc.ScheduledEndTime, sc.Status, sc.AutoRecordingKey, sc.RecordingID, sc.RetryCount, sc.RecordingMetadata)
	}
	return rows
}

func favoriteEntry(title string, start, end time.Time) map[string]interface{} {
	return map[string]interface{}{
		"id": "TBS", "title": title,
		"ft": start.In(radikoLocation).Format("20060102150405"),
		"to": end.In(radikoLocation).Format("20060102150405"),
	}
}

func TestFavoriteRecordingTimes(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, radikoLocation)
	fav := model.FavoriteProgram{UserID: 2, StationID: "TBS", ProgramTitle: "show", CreatedAt: now.AddDate(0, 0, -30)}
	monday, sunday := 0, 6
	tests := []struct {
		name    string
		fav     model.FavoriteProgram
		entry   map[string]interface{}
		want    bool
		wantDay *int
	}{
		{"catch up after power off", fav, favoriteEntry("show", now.Add(-48*time.Hour), now.Add(-47*time.Hour)), true, nil},
		{"upcoming episode is not queued early", fav, favoriteEntry("show", now.Add(time.Hour), now.Add(2*time.Hour)), false, nil},
		{"currently broadcasting", fav, favoriteEntry("show", now.Add(-time.Hour), now.Add(time.Hour)), false, nil},
		{"wait for timefree availability", fav, favoriteEntry("show", now.Add(-time.Hour), now.Add(-time.Minute)), false, nil},
		{"expired", fav, favoriteEntry("show", now.Add(-8*24*time.Hour), now.Add(-7*24*time.Hour)), false, nil},
		{"wrong title", fav, favoriteEntry("other", now.Add(-time.Hour), now), false, nil},
		{"wrong station", fav, map[string]interface{}{"id": "QRR", "title": "show", "ft": "20261108090000", "to": "20261108100000"}, false, nil},
		{"invalid timestamp", fav, map[string]interface{}{"title": "show", "ft": "20261108330000", "to": "bad"}, false, nil},
		{"reversed timestamp", fav, favoriteEntry("show", now, now.Add(-time.Hour)), false, nil},
		{"sunday at 01:00 in Japan", fav, map[string]interface{}{"title": "show", "ft": "20261108010000", "to": "20261108020000"}, true, &sunday},
		{"weekday mismatch", fav, favoriteEntry("show", now.Add(-time.Hour), now.Add(-10*time.Minute)), false, &monday},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantDay != nil {
				tt.fav.BroadcastDay = tt.wantDay
			}
			start, end, ok := favoriteRecordingTimes(tt.fav, tt.entry, now)
			if ok != tt.want {
				t.Fatalf("matched=%v want=%v start=%v end=%v", ok, tt.want, start, end)
			}
			if ok && (start.In(radikoLocation).Format("20060102150405") != tt.entry["ft"] || end.In(radikoLocation).Format("20060102150405") != tt.entry["to"]) {
				t.Fatal("broadcast timestamps changed")
			}
		})
	}
	t.Run("registered after broadcast", func(t *testing.T) {
		fav.CreatedAt = now
		_, _, ok := favoriteRecordingTimes(fav, favoriteEntry("show", now.Add(-2*time.Hour), now.Add(-time.Hour)), now)
		if ok {
			t.Fatal("must not record broadcasts that ended before registration")
		}
	})
}

func TestSyncFavoriteRecordingsCatchesMissedEpisodesAndFetchesStationOnce(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	now := time.Now().Truncate(time.Minute)
	created := now.AddDate(0, 0, -30)
	start := now.Add(-48 * time.Hour)
	end := start.Add(time.Hour)
	upcoming := now.Add(24 * time.Hour)
	svc := &stubFavoriteSchedules{calls: map[string]int{}, entries: []map[string]interface{}{
		favoriteEntry("show", start, end),
		favoriteEntry("show", upcoming, upcoming.Add(time.Hour)),
		favoriteEntry("show", now.Add(-9*24*time.Hour), now.Add(-8*24*time.Hour)),
	}}
	s.SetFavoriteScheduleService(svc)
	mock.ExpectQuery("SELECT \\* FROM favorite_programs").WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "station_id", "program_title", "created_at"}).
		AddRow(int64(1), int64(2), "TBS", "show", created).
		AddRow(int64(2), int64(3), "TBS", "show", created))
	mock.ExpectQuery("SELECT \\* FROM recording_schedules").WillReturnRows(favoriteScheduleRows())
	for _, at := range []time.Time{start} {
		for i, userID := range []int64{2, 3} {
			mock.ExpectExec("INSERT INTO recording_schedules").
				WithArgs(at, at.Add(time.Hour), favoriteRecordingKey(userID, "TBS", at), at, int64(i+1)).
				WillReturnResult(sqlmock.NewResult(int64(i+1), 1))
		}
	}
	s.syncFavoriteRecordings(context.Background())
	if svc.calls["TBS"] != 1 {
		t.Fatalf("station fetched %d times", svc.calls["TBS"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFavoriteRecordingsPreservesPendingOnFetchError(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	s.SetFavoriteScheduleService(&stubFavoriteSchedules{calls: map[string]int{}, err: errors.New("offline")})
	mock.ExpectQuery("SELECT \\* FROM favorite_programs").WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "station_id", "program_title"}).AddRow(int64(1), int64(2), "TBS", "show"))
	mock.ExpectQuery("SELECT \\* FROM recording_schedules").WillReturnRows(favoriteScheduleRows())
	s.syncFavoriteRecordings(context.Background())
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFavoriteRecordingKeyIdentifiesUserStationAndEpisode(t *testing.T) {
	at := time.Now()
	key := favoriteRecordingKey(2, "TBS", at)
	if len(key) != 64 || key != favoriteRecordingKey(2, "TBS", at.UTC()) {
		t.Fatal("key must be stable across time zones")
	}
	for _, other := range []string{favoriteRecordingKey(3, "TBS", at), favoriteRecordingKey(2, "QRR", at), favoriteRecordingKey(2, "TBS", at.AddDate(0, 0, 7))} {
		if key == other {
			t.Fatal("different users/stations/episodes must not share a key")
		}
	}
}

func TestFavoriteScheduleEntriesHandlesRedisJSON(t *testing.T) {
	entry := map[string]interface{}{"title": "show"}
	for _, input := range []interface{}{[]map[string]interface{}{entry}, []interface{}{entry, "invalid"}} {
		got := favoriteScheduleEntries(input)
		if len(got) != 1 || got[0]["title"] != "show" {
			t.Fatalf("entries=%v", got)
		}
	}
}

func TestCancelRemovedFavoriteRecordingsKeepsOtherUsersAndWeekdaysSeparate(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	key := "auto"
	at := time.Date(2026, 11, 8, 1, 0, 0, 0, radikoLocation).In(time.Local)
	day := 6
	kept := model.RecordingSchedule{ID: 1, UserID: 2, StationID: "TBS", ProgramTitle: "show", ScheduledStartTime: at, ScheduledEndTime: at.Add(time.Hour), AutoRecordingKey: &key}
	removed := kept
	removed.ID, removed.UserID = 2, 3
	mock.ExpectQuery("SELECT \\* FROM recording_schedules").WillReturnRows(favoriteScheduleRows(kept, removed))
	mock.ExpectExec("UPDATE recording_schedules.*SET status = 'cancelled'").WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	err := s.cancelRemovedFavoriteRecordings(context.Background(), []model.FavoriteProgram{{UserID: 2, StationID: "TBS", ProgramTitle: "show", BroadcastDay: &day}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverInterruptedFavoriteRecordingsRequeuesAfterPowerOff(t *testing.T) {
	s, mock, mr := newFavoriteRecordingScheduler(t)
	sc := *testRecordingSchedule()
	key, recordingID := "auto", "sched_interrupted"
	sc.AutoRecordingKey, sc.RecordingID = &key, &recordingID
	sc.Status = "recording"
	mr.Set("recording_"+recordingID, `{"recording_id":"sched_interrupted","status":"recording"}`)
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'recording'").WillReturnRows(favoriteScheduleRows(sc))
	mock.ExpectExec("UPDATE recording_schedules.*SET status = 'pending', recording_id = NULL, next_retry_at = NULL").
		WithArgs(sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.recoverInterruptedFavoriteRecordings(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := mr.Get("recording_" + recordingID)
	if err != nil || !strings.Contains(raw, `"status":"failed"`) {
		t.Fatalf("old recording must not remain in progress: %s, %v", raw, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectAutomaticClaim(mock sqlmock.Sqlmock, sc *model.RecordingSchedule, registered bool) {
	mock.ExpectExec("UPDATE recording_schedules SET status = \\?.*WHERE id = \\? AND status = 'pending'").
		WithArgs("recording", nil, sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	count := 0
	if registered {
		count = 1
	}
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM favorite_programs").WithArgs(sc.UserID, sc.StationID, sc.ProgramTitle, (int(sc.ScheduledStartTime.In(radikoLocation).Weekday())+6)%7).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func TestAutomaticRecordingExpiresInsteadOfDownloadingAfterLongShutdown(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	sc := testRecordingSchedule()
	key := "auto"
	sc.AutoRecordingKey = &key
	sc.ScheduledStartTime = time.Now().AddDate(0, 0, -9)
	sc.ScheduledEndTime = sc.ScheduledStartTime.Add(time.Hour)
	notifier := &stubRecordingNotificationService{}
	s.SetNotificationService(notifier)
	expectAutomaticClaim(mock, sc, true)
	mock.ExpectExec("UPDATE recording_schedules SET status = \\?").WithArgs("expired", sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	s.startScheduledRecording(context.Background(), sc)
	if notifier.failed != 1 {
		t.Fatal("expired recording should notify once")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticRecordingDoesNotStartAfterFavoriteRemoval(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	sc := testRecordingSchedule()
	key := "auto"
	sc.AutoRecordingKey = &key
	expectAutomaticClaim(mock, sc, false)
	mock.ExpectExec("UPDATE recording_schedules SET status = \\?").WithArgs("cancelled", nil, sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	s.startScheduledRecording(context.Background(), sc)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestScheduledRecordingClaimSkipsAlreadyClaimedOrCancelled(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	sc := testRecordingSchedule()
	mock.ExpectExec("UPDATE recording_schedules SET status = \\?.*AND status = 'pending'").WithArgs("recording", nil, sc.ID).WillReturnResult(sqlmock.NewResult(0, 0))
	s.startScheduledRecording(context.Background(), sc)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticRecordingReusesManualAudioAndPersistsMetadata(t *testing.T) {
	s, mock, mr := newFavoriteRecordingScheduler(t)
	sc := testRecordingSchedule()
	sc.ScheduledStartTime = sc.ScheduledStartTime.Truncate(time.Minute)
	key := "auto"
	sc.AutoRecordingKey = &key
	path := s.storagePath + "/manual.m4a"
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	info := model.RecordingInfo{RecordingID: "manual", StationID: sc.StationID, StartTime: sc.ScheduledStartTime.In(radikoLocation).Format("200601021504"), OwnerKey: fmt.Sprintf("user_%d", sc.UserID), Status: "completed", FilePath: path}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mr.Set("recording_manual", string(raw))
	mr.SetTTL("recording_manual", time.Hour)
	expectAutomaticClaim(mock, sc, true)
	mock.ExpectExec("UPDATE recording_schedules SET status = 'completed', recording_id = \\?, recording_metadata = \\?").WithArgs("manual", sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	s.startScheduledRecording(context.Background(), sc)
	if mr.TTL("recording_manual") != 0 {
		t.Fatal("adopted recording must survive until the configured retention cleanup")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type capturingFavoriteDownloader struct {
	start, end string
}

func (d *capturingFavoriteDownloader) DownloadTimefree(ctx context.Context, authToken, stationID, start, end, areaID, outputPath string) error {
	d.start, d.end = start, end
	return os.WriteFile(outputPath, []byte("audio"), 0644)
}

func TestAutomaticRecordingDownloadsJapaneseBroadcastTimesAndKeepsResult(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // FFmpegなしのフォールバックで音声処理を外部依存から分離する。
	s, mock, mr := newFavoriteRecordingScheduler(t)
	sc := testRecordingSchedule()
	key := "auto"
	sc.AutoRecordingKey = &key
	sc.ScheduledStartTime = time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Minute)
	sc.ScheduledEndTime = sc.ScheduledStartTime.Add(time.Hour)
	downloader := &capturingFavoriteDownloader{}
	s.hlsDownloader = downloader
	expectAutomaticClaim(mock, sc, true)
	mock.ExpectExec("UPDATE recording_schedules SET recording_id = \\?").WithArgs(sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE recording_schedules SET status = 'completed', recording_id = \\?, recording_metadata = \\?").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	s.startScheduledRecording(context.Background(), sc)
	if downloader.start != sc.ScheduledStartTime.In(radikoLocation).Format("200601021504") || downloader.end != sc.ScheduledEndTime.In(radikoLocation).Format("200601021504") {
		t.Fatalf("wrong Radiko timestamps: %s %s", downloader.start, downloader.end)
	}
	keys := mr.Keys()
	if len(keys) != 1 || mr.TTL(keys[0]) != 0 {
		t.Fatalf("metadata must persist: %v", keys)
	}
	value, _ := mr.Get(keys[0])
	var info model.RecordingInfo
	if err := json.Unmarshal([]byte(value), &info); err != nil || info.Status != "completed" || info.OwnerKey != fmt.Sprintf("user_%d", sc.UserID) {
		t.Fatalf("invalid saved result: %s, %v", value, err)
	}
	if _, err := os.Stat(info.FilePath); err != nil {
		t.Fatalf("recorded audio missing: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFindPendingBeforeWaitsForBroadcastEndAndRetryDeadline(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	cutoff := "2026-11-08 12:00:00"
	mock.ExpectQuery("SELECT \\* FROM recording_schedules WHERE status = 'pending' AND scheduled_end_time <= \\? AND \\(next_retry_at IS NULL OR next_retry_at <= \\?\\)").
		WithArgs(cutoff, cutoff).WillReturnRows(favoriteScheduleRows())
	if _, err := findPendingBefore(s.db, cutoff); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticRecordingKeepsRetryingAfterManualRetryLimit(t *testing.T) {
	s, mock, _ := newFavoriteRecordingScheduler(t)
	sc := testRecordingSchedule()
	key := "auto"
	sc.AutoRecordingKey = &key
	sc.RetryCount = maxScheduledRecordingRetries + 10
	s.radikoClient = stubSchedulerRadikoClient{err: errors.New("offline")}
	expectAutomaticClaim(mock, sc, true)
	mock.ExpectExec("UPDATE recording_schedules SET retry_count = retry_count \\+ 1, status = 'pending', error_message = \\?, next_retry_at = \\?").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	s.startScheduledRecording(context.Background(), sc)
	if favoriteRecordingRetryDelay(sc.RetryCount) != time.Hour || favoriteRecordingRetryDelay(0) != time.Minute {
		t.Fatal("automatic retries must back off from one minute to at most one hour")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreFavoriteRecordingMetadataAfterRedisLoss(t *testing.T) {
	s, mock, mr := newFavoriteRecordingScheduler(t)
	sc := *testRecordingSchedule()
	key := "auto"
	sc.AutoRecordingKey = &key
	sc.Status = "completed"
	path := s.storagePath + "/recorded.m4a"
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	info := model.RecordingInfo{RecordingID: "saved", StationID: sc.StationID, OwnerKey: fmt.Sprintf("user_%d", sc.UserID), Status: "completed", FilePath: path, CreatedAt: time.Now().Format(time.RFC3339)}
	metadata, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(metadata)
	sc.RecordingMetadata = &raw
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'completed' AND recording_metadata IS NOT NULL").WillReturnRows(favoriteScheduleRows(sc))
	if err := s.restoreFavoriteRecordingMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("recording_saved") || mr.TTL("recording_saved") != 0 {
		t.Fatal("completed recording must be restored from the database without a TTL")
	}
	// 音声を手動削除した後は、古いDBスナップショットから一覧を復活させない。
	mr.FlushAll()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT \\* FROM recording_schedules").WillReturnRows(favoriteScheduleRows(sc))
	if err := s.restoreFavoriteRecordingMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mr.Exists("recording_saved") {
		t.Fatal("deleted audio must not be resurrected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverCompletedDownloadDoesNotDownloadAgainAfterRestart(t *testing.T) {
	s, mock, mr := newFavoriteRecordingScheduler(t)
	sc := *testRecordingSchedule()
	key, id := "auto", "saved"
	sc.AutoRecordingKey, sc.RecordingID, sc.Status = &key, &id, "recording"
	path := s.storagePath + "/saved.m4a"
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	info := model.RecordingInfo{RecordingID: id, StationID: sc.StationID, OwnerKey: fmt.Sprintf("user_%d", sc.UserID), Status: "completed", FilePath: path}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mr.Set("recording_"+id, string(raw))
	mock.ExpectQuery("SELECT \\* FROM recording_schedules.*status = 'recording'").WillReturnRows(favoriteScheduleRows(sc))
	mock.ExpectExec("UPDATE recording_schedules SET status = 'completed', recording_id = \\?, recording_metadata = \\?").WithArgs(id, string(raw), sc.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.recoverInterruptedFavoriteRecordings(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
