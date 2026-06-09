package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/sessions"
	"github.com/redis/go-redis/v9"
	"github.com/tsukao2240/radio_review_go/internal/model"
	"github.com/tsukao2240/radio_review_go/pkg/radiko"
)

// stubRadikoClient は radiko.ClientInterface のスタブ。
type stubRadikoClient struct {
	getAuthTokenFunc func(ctx context.Context, areaID string) (string, error)
}

func (c *stubRadikoClient) GetAuthToken(ctx context.Context, areaID string) (string, error) {
	if c.getAuthTokenFunc != nil {
		return c.getAuthTokenFunc(ctx, areaID)
	}
	return "test-token", nil
}

var _ radiko.ClientInterface = (*stubRadikoClient)(nil)

// stubHLSDownloader は radiko.HLSDownloaderInterface のスタブ。
type stubHLSDownloader struct {
	downloadFunc func(ctx context.Context, authToken, stationID, startTime, endTime, areaID, outputPath string) error
}

func (d *stubHLSDownloader) DownloadTimefree(ctx context.Context, authToken, stationID, startTime, endTime, areaID, outputPath string) error {
	if d.downloadFunc != nil {
		return d.downloadFunc(ctx, authToken, stationID, startTime, endTime, areaID, outputPath)
	}
	return nil
}

var _ radiko.HLSDownloaderInterface = (*stubHLSDownloader)(nil)

// newMiniRedis はテスト用の miniredis クライアントを返す。
func newMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

func TestStartTimefreeRecording_InvalidJSON(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := httptest.NewRequest(http.MethodPost, "/recording/timefree/start", bytes.NewReader([]byte("not-json")))
	rr := httptest.NewRecorder()
	h.StartTimefreeRecording(store)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestStartTimefreeRecording_MissingFields(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	body, _ := json.Marshal(map[string]string{"station_id": "TBS"})
	req := httptest.NewRequest(http.MethodPost, "/recording/timefree/start", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.StartTimefreeRecording(store)(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422", rr.Code)
	}
}

func TestStartTimefreeRecording_Success(t *testing.T) {
	_, rdb := newMiniRedis(t)
	// Use os.MkdirTemp instead of t.TempDir() to avoid race with the background goroutine.
	dir, err := os.MkdirTemp("", "test-recording-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Give the background goroutine a moment to finish before cleanup.
		time.Sleep(50 * time.Millisecond)
		os.RemoveAll(dir)
	})
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, dir)
	store := sessions.NewCookieStore([]byte("test"))

	body, _ := json.Marshal(map[string]string{
		"station_id":   "TBS",
		"start_time":   "20240101100000",
		"end_time":     "20240101110000",
		"program_name": "Jazz Show",
	})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/timefree/start", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.StartTimefreeRecording(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["status"] != "recording" {
		t.Errorf("expected status=recording, got %q", resp["status"])
	}
	if resp["recording_id"] == "" {
		t.Error("expected non-empty recording_id")
	}
}

func TestStartTimefreeRecording_DownloadFailureStoresFailReason(t *testing.T) {
	_, rdb := newMiniRedis(t)
	dir, err := os.MkdirTemp("", "test-recording-fail-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond)
		os.RemoveAll(dir)
	})

	done := make(chan struct{})
	downloadErr := errors.New("playlist returned status 404")
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{
		downloadFunc: func(ctx context.Context, authToken, stationID, startTime, endTime, areaID, outputPath string) error {
			defer close(done)
			return downloadErr
		},
	}, rdb, dir)
	store := sessions.NewCookieStore([]byte("test"))

	body, _ := json.Marshal(map[string]string{
		"station_id":   "TBS",
		"start_time":   "202401011000",
		"end_time":     "202401011100",
		"program_name": "Jazz Show",
	})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/timefree/start", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.StartTimefreeRecording(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for download goroutine")
	}

	var info *model.RecordingInfo
	for i := 0; i < 20; i++ {
		info, err = h.loadRecordingInfo(context.Background(), resp["recording_id"])
		if err == nil && info.Status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("loadRecordingInfo: %v", err)
	}
	if info.Status != "failed" {
		t.Fatalf("status = %q, want failed", info.Status)
	}
	if info.FailReason != downloadErr.Error() {
		t.Fatalf("fail_reason = %q, want %q", info.FailReason, downloadErr.Error())
	}
}

func TestStartTimefreeRecording_ResolvesAreaFromStationID(t *testing.T) {
	_, rdb := newMiniRedis(t)
	dir := t.TempDir()
	store := sessions.NewCookieStore([]byte("test"))

	authArea := make(chan string, 1)
	downloadArea := make(chan string, 1)
	h := NewRecordingHandler(&stubRadikoClient{
		getAuthTokenFunc: func(ctx context.Context, areaID string) (string, error) {
			authArea <- areaID
			return "test-token", nil
		},
	}, &stubHLSDownloader{
		downloadFunc: func(ctx context.Context, authToken, stationID, startTime, endTime, areaID, outputPath string) error {
			downloadArea <- areaID
			return nil
		},
	}, rdb, dir)

	body, _ := json.Marshal(map[string]string{
		"station_id":   "OBC",
		"start_time":   "20240101100000",
		"end_time":     "20240101110000",
		"program_name": "Osaka Show",
	})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/timefree/start", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.StartTimefreeRecording(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	if got := <-authArea; got != "JP27" {
		t.Fatalf("auth area = %q, want JP27", got)
	}
	select {
	case got := <-downloadArea:
		if got != "JP27" {
			t.Fatalf("download area = %q, want JP27", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for download")
	}
}

func TestStartTimefree_CoreStoresOwnerAndStartsDownload(t *testing.T) {
	_, rdb := newMiniRedis(t)
	dir := t.TempDir()

	authArea := make(chan string, 1)
	downloadArgs := make(chan map[string]string)
	h := NewRecordingHandler(&stubRadikoClient{
		getAuthTokenFunc: func(ctx context.Context, areaID string) (string, error) {
			authArea <- areaID
			return "core-token", nil
		},
	}, &stubHLSDownloader{
		downloadFunc: func(ctx context.Context, authToken, stationID, startTime, endTime, areaID, outputPath string) error {
			downloadArgs <- map[string]string{
				"authToken":  authToken,
				"stationID":  stationID,
				"startTime":  startTime,
				"endTime":    endTime,
				"areaID":     areaID,
				"outputPath": outputPath,
			}
			return nil
		},
	}, rdb, dir)

	recordingID, err := h.StartTimefree(context.Background(), "user_9", "OBC", "Core Show", "202401011000", "202401011100", "")
	if err != nil {
		t.Fatalf("StartTimefree: %v", err)
	}
	if recordingID == "" {
		t.Fatal("expected recordingID")
	}
	if got := <-authArea; got != "JP27" {
		t.Fatalf("auth area = %q, want JP27", got)
	}

	info, err := h.loadRecordingInfo(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("loadRecordingInfo: %v", err)
	}
	if info.OwnerKey != "user_9" || info.ProgramName != "Core Show" || info.Status != "recording" {
		t.Fatalf("stored info = %#v", info)
	}
	if !strings.Contains(filepath.Base(info.FilePath), "2024-01-01_1000_OBC_Core Show.aac") {
		t.Fatalf("file path = %q", info.FilePath)
	}

	select {
	case got := <-downloadArgs:
		if got["authToken"] != "core-token" || got["areaID"] != "JP27" || got["stationID"] != "OBC" {
			t.Fatalf("download args = %#v", got)
		}
		if got["outputPath"] != info.FilePath {
			t.Fatalf("output path = %q, want %q", got["outputPath"], info.FilePath)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for download")
	}
}

func TestIsProgramRecording(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	ctx := context.Background()

	if err := h.saveRecordingInfo(ctx, &model.RecordingInfo{
		RecordingID: "active",
		ProgramName: "Jazz",
		Status:      "recording",
		OwnerKey:    "user_1",
	}, 2*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := h.saveRecordingInfo(ctx, &model.RecordingInfo{
		RecordingID: "done",
		ProgramName: "News",
		Status:      "completed",
		OwnerKey:    "user_1",
	}, 2*time.Hour); err != nil {
		t.Fatal(err)
	}

	got, err := h.IsProgramRecording(ctx, "user_1", "Jazz")
	if err != nil {
		t.Fatalf("IsProgramRecording: %v", err)
	}
	if !got {
		t.Fatal("expected Jazz recording")
	}
	got, err = h.IsProgramRecording(ctx, "user_1", "News")
	if err != nil {
		t.Fatalf("IsProgramRecording: %v", err)
	}
	if got {
		t.Fatal("expected completed program not recording")
	}
}

func TestRecordingOwnerKey_GuestStableInSession(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test-secret-32-bytes-long"))

	req1 := httptest.NewRequest(http.MethodGet, "/recording/list", nil)
	rr1 := httptest.NewRecorder()
	key1, err := h.ownerKey(req1, rr1, store)
	if err != nil {
		t.Fatalf("ownerKey: %v", err)
	}
	if key1 == "session_" {
		t.Fatal("guest owner key is empty")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/recording/list", nil)
	for _, c := range rr1.Result().Cookies() {
		req2.AddCookie(c)
	}
	rr2 := httptest.NewRecorder()
	key2, err := h.ownerKey(req2, rr2, store)
	if err != nil {
		t.Fatalf("ownerKey second call: %v", err)
	}
	if key1 != key2 {
		t.Fatalf("guest owner key changed: %q != %q", key1, key2)
	}
}

func requestWithGuestOwner(t *testing.T, method, target string, store sessions.Store, ownerID string) *http.Request {
	t.Helper()
	setupReq := httptest.NewRequest(http.MethodGet, "/", nil)
	setupRR := httptest.NewRecorder()
	session, err := store.Get(setupReq, "radio_review_session")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	session.Values["guest_owner_id"] = ownerID
	if err := session.Save(setupReq, setupRR); err != nil {
		t.Fatalf("session.Save: %v", err)
	}

	req := httptest.NewRequest(method, target, nil)
	for _, c := range setupRR.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func requestWithSessionUser(t *testing.T, method, target string, store sessions.Store, userID int64) *http.Request {
	t.Helper()
	setupReq := httptest.NewRequest(http.MethodGet, "/", nil)
	setupRR := httptest.NewRecorder()
	session, err := store.Get(setupReq, "radio_review_session")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	session.Values["user_id"] = userID
	if err := session.Save(setupReq, setupRR); err != nil {
		t.Fatalf("session.Save: %v", err)
	}

	req := httptest.NewRequest(method, target, nil)
	for _, c := range setupRR.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestStopRecording_InvalidJSON(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()
	h.StopRecording(store)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestStopRecording_MissingRecordingID(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	body, _ := json.Marshal(map[string]string{"recording_id": ""})
	req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.StopRecording(store)(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422", rr.Code)
	}
}

func TestStopRecording_NotFound(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	body, _ := json.Marshal(map[string]string{"recording_id": "nonexistent"})
	req := httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.StopRecording(store)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rr.Code)
	}
}

func TestStopRecording_Forbidden(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	// Save a recording owned by user_2
	info := &model.RecordingInfo{
		RecordingID: "test123",
		OwnerKey:    "user_2",
		Status:      "recording",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_test123", string(data), 0)

	body, _ := json.Marshal(map[string]string{"recording_id": "test123"})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body)), 1) // user_1
	rr := httptest.NewRecorder()
	h.StopRecording(store)(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rr.Code)
	}
}

func TestStopRecording_Success(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	// Save recording owned by user_1
	info := &model.RecordingInfo{
		RecordingID: "rec456",
		OwnerKey:    "user_1",
		Status:      "recording",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_rec456", string(data), 0)

	body, _ := json.Marshal(map[string]string{"recording_id": "rec456"})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/stop", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.StopRecording(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestGetRecordingStatus_MissingID(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := httptest.NewRequest(http.MethodGet, "/recording/status", nil)
	rr := httptest.NewRecorder()
	h.GetRecordingStatus(store)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestGetRecordingStatus_NotFound(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := httptest.NewRequest(http.MethodGet, "/recording/status?recording_id=nonexistent", nil)
	rr := httptest.NewRecorder()
	h.GetRecordingStatus(store)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rr.Code)
	}
}

func TestGetRecordingStatus_Forbidden(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	info := &model.RecordingInfo{
		RecordingID: "st_forbid",
		OwnerKey:    "user_2",
		Status:      "completed",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_st_forbid", string(data), 0)

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/status?recording_id=st_forbid", nil), 1) // user_1
	rr := httptest.NewRecorder()
	h.GetRecordingStatus(store)(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rr.Code)
	}
}

func TestGetRecordingStatus_Success(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	info := &model.RecordingInfo{
		RecordingID: "status123",
		OwnerKey:    "user_1",
		Status:      "completed",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_status123", string(data), 0)

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/status?recording_id=status123", nil), 1)
	rr := httptest.NewRecorder()
	h.GetRecordingStatus(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestDownloadRecording_MissingID(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := httptest.NewRequest(http.MethodGet, "/recording/download", nil)
	rr := httptest.NewRecorder()
	h.DownloadRecording(store)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestDownloadRecording_NotCompleted(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	info := &model.RecordingInfo{
		RecordingID: "dl123",
		OwnerKey:    "user_1",
		Status:      "recording",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_dl123", string(data), 0)

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/download?recording_id=dl123", nil), 1)
	rr := httptest.NewRecorder()
	h.DownloadRecording(store)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestDownloadRecording_NotFound(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/download?recording_id=nonexistent", nil), 1)
	rr := httptest.NewRecorder()
	h.DownloadRecording(store)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rr.Code)
	}
}

func TestDownloadRecording_Forbidden(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	info := &model.RecordingInfo{
		RecordingID: "forbid123",
		OwnerKey:    "user_2",
		Status:      "completed",
		FilePath:    "/tmp/file.aac",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_forbid123", string(data), 0)

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/download?recording_id=forbid123", nil), 1) // user_1
	rr := httptest.NewRecorder()
	h.DownloadRecording(store)(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rr.Code)
	}
}

func TestDownloadRecording_FileOpenError(t *testing.T) {
	_, rdb := newMiniRedis(t)
	dir := t.TempDir()
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, dir)
	store := sessions.NewCookieStore([]byte("test"))

	info := &model.RecordingInfo{
		RecordingID: "filerr123",
		OwnerKey:    "user_1",
		Status:      "completed",
		FilePath:    filepath.Join(dir, "file.aac"),
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_filerr123", string(data), 0)

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/download?recording_id=filerr123", nil), 1)
	rr := httptest.NewRecorder()
	h.DownloadRecording(store)(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestDownloadRecording_Success(t *testing.T) {
	dir := t.TempDir()
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, dir)
	store := sessions.NewCookieStore([]byte("test"))

	// Create a real file to serve.
	filePath := filepath.Join(dir, "2026-06-05_0100_TBS_Jazz Show.aac")
	if err := os.WriteFile(filePath, []byte("audio-data"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	info := &model.RecordingInfo{
		RecordingID: "dl_ok",
		OwnerKey:    "user_1",
		Status:      "completed",
		FilePath:    filePath,
		ProgramName: "Jazz Show",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_dl_ok", string(data), 0)

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/download?recording_id=dl_ok", nil), 1)
	rr := httptest.NewRecorder()
	h.DownloadRecording(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="2026-06-05_0100_TBS_Jazz Show.aac"`) {
		t.Fatalf("Content-Disposition = %q", cd)
	}
}

func TestListRecordings_WithRecordings(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	info := &model.RecordingInfo{
		RecordingID: "list001",
		OwnerKey:    "user_1",
		Status:      "completed",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_list001", string(data), 0)

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/list", nil), 1)
	rr := httptest.NewRecorder()
	h.ListRecordings(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestListRecordings_Empty(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/list", nil), 1)
	rr := httptest.NewRecorder()
	h.ListRecordings(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
}

func TestListRecordings_RedisError(t *testing.T) {
	mr, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	mr.Close() // close Redis to trigger scan error

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/list", nil), 1)
	rr := httptest.NewRecorder()
	h.ListRecordings(store)(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestShowHistory_RedisError(t *testing.T) {
	mr, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	mr.Close() // close Redis to trigger scan error

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/history", nil), 1)
	rr := httptest.NewRecorder()
	h.ShowHistory(store)(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestDeleteRecording_MissingID(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	body, _ := json.Marshal(map[string]string{"recording_id": ""})
	req := httptest.NewRequest(http.MethodPost, "/recording/delete", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.DeleteRecording(store)(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422", rr.Code)
	}
}

func TestDeleteRecording_Success(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	info := &model.RecordingInfo{
		RecordingID: "del789",
		OwnerKey:    "user_1",
		Status:      "completed",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_del789", string(data), 0)

	body, _ := json.Marshal(map[string]string{"recording_id": "del789"})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/delete", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.DeleteRecording(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestOwnerKey_WithUserID(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := withUserID(httptest.NewRequest(http.MethodGet, "/", nil), 42)
	rr := httptest.NewRecorder()
	key, err := h.ownerKey(req, rr, store)
	if err != nil {
		t.Fatalf("ownerKey: %v", err)
	}
	if key != "user_42" {
		t.Errorf("expected user_42, got %q", key)
	}
}

func TestOwnerKey_WithSessionUserID(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := requestWithSessionUser(t, http.MethodGet, "/", store, 42)
	rr := httptest.NewRecorder()
	key, err := h.ownerKey(req, rr, store)
	if err != nil {
		t.Fatalf("ownerKey: %v", err)
	}
	if key != "user_42" {
		t.Errorf("expected user_42, got %q", key)
	}
}

func TestOwnerKey_GuestSession(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	key, err := h.ownerKey(req, rr, store)
	if err != nil {
		t.Fatalf("ownerKey: %v", err)
	}
	if len(key) == 0 {
		t.Error("expected non-empty owner key for guest")
	}
}

func TestListRecordings_WithSessionUserReturnsUserRecordings(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	userInfo := &model.RecordingInfo{
		RecordingID: "bulk001",
		OwnerKey:    "user_7",
		Status:      "completed",
		ProgramName: "Bulk Program",
	}
	otherInfo := &model.RecordingInfo{
		RecordingID: "other001",
		OwnerKey:    "user_8",
		Status:      "completed",
		ProgramName: "Other Program",
	}
	userData, _ := json.Marshal(userInfo)
	otherData, _ := json.Marshal(otherInfo)
	rdb.Set(context.Background(), "recording_bulk001", string(userData), 0)
	rdb.Set(context.Background(), "recording_other001", string(otherData), 0)

	req := requestWithSessionUser(t, http.MethodGet, "/recording/list", store, 7)
	rr := httptest.NewRecorder()
	h.ListRecordings(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "bulk001") {
		t.Fatalf("expected bulk recording in response: %s", body)
	}
	if strings.Contains(body, "other001") {
		t.Fatalf("unexpected other owner recording in response: %s", body)
	}
}

func TestShowHistory_WithSessionUserReturnsUserRecordings(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	userInfo := &model.RecordingInfo{
		RecordingID: "bulk_history",
		StationID:   "TBS",
		OwnerKey:    "user_7",
		Status:      "completed",
		ProgramName: "Bulk History Program",
		CreatedAt:   "2026-06-09T12:00:00+09:00",
	}
	otherInfo := &model.RecordingInfo{
		RecordingID: "other_history",
		StationID:   "QRR",
		OwnerKey:    "user_8",
		Status:      "completed",
		ProgramName: "Other History Program",
	}
	userData, _ := json.Marshal(userInfo)
	otherData, _ := json.Marshal(otherInfo)
	rdb.Set(context.Background(), "recording_bulk_history", string(userData), 0)
	rdb.Set(context.Background(), "recording_other_history", string(otherData), 0)

	req := requestWithSessionUser(t, http.MethodGet, "/recording/history", store, 7)
	rr := httptest.NewRecorder()
	h.ShowHistory(store)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "bulk_history") {
		t.Fatalf("expected bulk recording in response: %s", body)
	}
	if strings.Contains(body, "other_history") {
		t.Fatalf("unexpected other owner recording in response: %s", body)
	}
}

func TestShowHistory(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := withUserID(httptest.NewRequest(http.MethodGet, "/recording/history", nil), 1)
	rr := httptest.NewRecorder()
	h.ShowHistory(store)(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestDeleteRecording_NotFound(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	body, _ := json.Marshal(map[string]string{"recording_id": "nonexistent"})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/delete", bytes.NewReader(body)), 1)
	rr := httptest.NewRecorder()
	h.DeleteRecording(store)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rr.Code)
	}
}

func TestDeleteRecording_Forbidden(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	info := &model.RecordingInfo{
		RecordingID: "del_forbid",
		OwnerKey:    "user_2",
		Status:      "completed",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_del_forbid", string(data), 0)

	body, _ := json.Marshal(map[string]string{"recording_id": "del_forbid"})
	req := withUserID(httptest.NewRequest(http.MethodPost, "/recording/delete", bytes.NewReader(body)), 1) // user_1
	rr := httptest.NewRecorder()
	h.DeleteRecording(store)(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rr.Code)
	}
}

func TestDeleteRecording_BadJSON(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := httptest.NewRequest(http.MethodPost, "/recording/delete", bytes.NewReader([]byte("bad-json")))
	rr := httptest.NewRecorder()
	h.DeleteRecording(store)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestStreamRecording_NotFound(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test-secret"))

	req := httptest.NewRequest(http.MethodGet, "/recording/stream?recording_id=notexist", nil)
	w := httptest.NewRecorder()
	h.StreamRecording(store)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestStreamRecording_NotCompleted(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test-secret"))

	info := &model.RecordingInfo{
		RecordingID: "test123",
		Status:      "recording",
		OwnerKey:    "session_guest123",
		FilePath:    "/tmp/test.aac",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_test123", string(data), 0)

	req := requestWithGuestOwner(t, http.MethodGet, "/recording/stream?recording_id=test123", store, "guest123")
	w := httptest.NewRecorder()
	h.StreamRecording(store)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStreamRecording_Forbidden(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test-secret"))

	info := &model.RecordingInfo{
		RecordingID: "test-forbidden",
		Status:      "completed",
		OwnerKey:    "user_99",
		FilePath:    "/tmp/test.aac",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_test-forbidden", string(data), 0)

	req := httptest.NewRequest(http.MethodGet, "/recording/stream?recording_id=test-forbidden", nil)
	w := httptest.NewRecorder()
	h.StreamRecording(store)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestStreamRecording_CompletedServesFile(t *testing.T) {
	_, rdb := newMiniRedis(t)
	dir := t.TempDir()
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, dir)
	store := sessions.NewCookieStore([]byte("test-secret"))

	// テスト用AACファイルを作成
	filePath := filepath.Join(dir, "test.aac")
	if err := os.WriteFile(filePath, []byte("fake aac content"), 0644); err != nil {
		t.Fatal(err)
	}

	info := &model.RecordingInfo{
		RecordingID: "test456",
		Status:      "completed",
		OwnerKey:    "session_guest456",
		FilePath:    filePath,
		ProgramName: "テスト番組",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_test456", string(data), 0)

	req := requestWithGuestOwner(t, http.MethodGet, "/recording/stream?recording_id=test456", store, "guest456")
	w := httptest.NewRecorder()
	h.StreamRecording(store)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "audio/aac" {
		t.Errorf("Content-Type = %q, want audio/aac", ct)
	}
}

func TestStreamRecording_RangeRequest(t *testing.T) {
	_, rdb := newMiniRedis(t)
	dir := t.TempDir()
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, dir)
	store := sessions.NewCookieStore([]byte("test-secret"))

	filePath := filepath.Join(dir, "2026-06-05_0100_TBS_Range.aac")
	if err := os.WriteFile(filePath, []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}

	info := &model.RecordingInfo{
		RecordingID: "range1",
		Status:      "completed",
		OwnerKey:    "session_guest456",
		FilePath:    filePath,
		ProgramName: "Range",
		StationID:   "TBS",
		StartTime:   "20260605010000",
	}
	data, _ := json.Marshal(info)
	rdb.Set(context.Background(), "recording_range1", string(data), 0)

	req := requestWithGuestOwner(t, http.MethodGet, "/recording/stream?recording_id=range1", store, "guest456")
	req.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	h.StreamRecording(store)(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("got %d, want %d", w.Code, http.StatusPartialContent)
	}
	if got := w.Body.String(); got != "2345" {
		t.Fatalf("body = %q, want 2345", got)
	}
	if cr := w.Header().Get("Content-Range"); cr != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q", cr)
	}
	if ar := w.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Fatalf("Accept-Ranges = %q", ar)
	}
}
