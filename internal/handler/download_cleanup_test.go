package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/tsukao2240/radio_review_go/internal/model"
)

type failedDownloadWriter struct{ *httptest.ResponseRecorder }

func (w failedDownloadWriter) Write(p []byte) (int, error) { return 0, errors.New("connection lost") }

func TestDownloadWaitsForSavedFileAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name, method, rangeHeader                            string
		writeFailure, dbFailure, ack, badReceipt, wrongOwner bool
		want                                                 int
	}{
		{name: "full transfer alone keeps original", method: "GET"},
		{name: "saved acknowledgement deletes", method: "GET", ack: true, want: 200},
		{name: "partial cannot acknowledge", method: "GET", rangeHeader: "bytes=0-1", ack: true, want: 409},
		{name: "head cannot acknowledge", method: "HEAD", ack: true, want: 409},
		{name: "disconnect cannot acknowledge", method: "GET", writeFailure: true, ack: true, want: 409},
		{name: "database unavailable keeps original", method: "GET", dbFailure: true, ack: true, want: 503},
		{name: "forged receipt rejected", method: "GET", badReceipt: true, ack: true, want: 409},
		{name: "different owner rejected", method: "GET", wrongOwner: true, ack: true, want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mr, rdb := newMiniRedis(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "test.m4a")
			if err := os.WriteFile(path, []byte("audio data"), 0600); err != nil {
				t.Fatal(err)
			}
			h := NewRecordingHandler(&stubRadikoClient{}, &stubHLSDownloader{}, rdb, dir)
			info := &model.RecordingInfo{RecordingID: "cleanup", OwnerKey: "user_1", Status: "completed", FilePath: path}
			if err := h.saveRecordingInfo(context.Background(), info, 0); err != nil {
				t.Fatal(err)
			}
			calls := 0
			h.SetDownloadCompletion(func(ctx context.Context, got *model.RecordingInfo) error {
				calls++
				if got.RecordingID != "cleanup" {
					t.Fatal("wrong recording")
				}
				if tc.dbFailure {
					return errors.New("database unavailable")
				}
				return nil
			})
			store := sessions.NewCookieStore([]byte("test"))
			req := withUserID(httptest.NewRequest(tc.method, "/recording/download?recording_id=cleanup", nil), 1)
			if tc.rangeHeader != "" {
				req.Header.Set("Range", tc.rangeHeader)
			}
			rr := httptest.NewRecorder()
			var writer http.ResponseWriter = rr
			if tc.writeFailure {
				writer = failedDownloadWriter{rr}
			}
			h.DownloadRecording(store)(writer, req)
			if _, err := os.Stat(path); err != nil {
				t.Fatal("GET removed original before local save")
			}
			if calls != 0 {
				t.Fatal("GET persisted a save that was not acknowledged")
			}
			if !tc.ack {
				return
			}
			receipt := rr.Header().Get("X-Download-Receipt")
			if tc.badReceipt {
				receipt = "forged"
			}
			body, _ := json.Marshal(map[string]string{"recording_id": "cleanup", "receipt": receipt})
			userID := int64(1)
			if tc.wrongOwner {
				userID = 2
			}
			ack := withUserID(httptest.NewRequest("POST", "/recording/download/complete", strings.NewReader(string(body))), userID)
			result := httptest.NewRecorder()
			h.CompleteSavedDownload(store)(result, ack)
			if result.Code != tc.want {
				t.Fatalf("ack status=%d want=%d", result.Code, tc.want)
			}
			_, err := os.Stat(path)
			if tc.want != 200 && err != nil {
				t.Fatal("failed acknowledgement removed original")
			}
			if tc.want == 200 {
				if !os.IsNotExist(err) || mr.Exists("recording_cleanup") || calls != 1 {
					t.Fatal("save completion did not persist then remove")
				}
				replay := httptest.NewRecorder()
				again := withUserID(httptest.NewRequest("POST", "/recording/download/complete", strings.NewReader(string(body))), 1)
				h.CompleteSavedDownload(store)(replay, again)
				if replay.Code != 200 || calls != 1 {
					t.Fatal("acknowledgement retry is not idempotent")
				}
			}
		})
	}
}
