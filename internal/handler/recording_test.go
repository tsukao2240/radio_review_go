package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/sessions"
	"github.com/redis/go-redis/v9"
	"github.com/yourname/radio_review_go/internal/model"
	"github.com/yourname/radio_review_go/pkg/radiko"
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
	downloadFunc func(ctx context.Context, authToken, stationID, startTime, endTime, outputPath string) error
}

func (d *stubHLSDownloader) DownloadTimefree(ctx context.Context, authToken, stationID, startTime, endTime, outputPath string) error {
	if d.downloadFunc != nil {
		return d.downloadFunc(ctx, authToken, stationID, startTime, endTime, outputPath)
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
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
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
	key := h.ownerKey(req, store)
	if key != "user_42" {
		t.Errorf("expected user_42, got %q", key)
	}
}

func TestOwnerKey_GuestSession(t *testing.T) {
	_, rdb := newMiniRedis(t)
	h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, t.TempDir())
	store := sessions.NewCookieStore([]byte("test"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	key := h.ownerKey(req, store)
	if len(key) == 0 {
		t.Error("expected non-empty owner key for guest")
	}
}
