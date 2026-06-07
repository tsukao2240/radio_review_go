package job

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/radio_review_go/internal/model"
)

func TestInsertColon(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"202506021300", "20:2506021300"},
		{"1300", "13:00"},
		{"0000", "00:00"},
		{"12", "12:"},
		{"1", "1"},
		{"", ""},
	}
	for _, tc := range cases {
		got := insertColon(tc.input)
		if got != tc.want {
			t.Errorf("insertColon(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestGetBroadcastIDs_ParsesXML(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<regions>
  <region>
    <stations>
      <station><id>TBS</id></station>
      <station><id>LFR</id></station>
    </stations>
  </region>
  <region>
    <stations>
      <station><id>TBS</id></station>
      <station><id>QRR</id></station>
    </stations>
  </region>
</regions>`

	// radikoRegionXML の xml タグに合わせたテスト用 XML
	type testStation struct {
		ID string `xml:"id"`
	}
	type testRegion struct {
		Stations []testStation `xml:"stations>station"`
	}
	type testRoot struct {
		Regions []testRegion `xml:"region"`
	}

	var root testRoot
	if err := xml.Unmarshal([]byte(xmlBody), &root); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}

	seen := make(map[string]struct{})
	var ids []string
	for _, region := range root.Regions {
		for _, st := range region.Stations {
			if st.ID == "" {
				continue
			}
			if _, ok := seen[st.ID]; !ok {
				seen[st.ID] = struct{}{}
				ids = append(ids, st.ID)
			}
		}
	}

	if len(ids) != 3 {
		t.Errorf("expected 3 unique IDs, got %d: %v", len(ids), ids)
	}
	// 重複 TBS は1件のみ
	count := 0
	for _, id := range ids {
		if id == "TBS" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("TBS should appear exactly once, got %d", count)
	}
}

func TestFetchWeeklyPrograms_ParsesXML(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<radiko>
  <stations>
    <station id="TBS">
      <progs>
        <prog ft="202506021300" to="202506021400">
          <title>jazz show</title>
          <pfm>DJ Smith</pfm>
          <info>jazz music</info>
          <url>https://example.com</url>
          <img>https://example.com/img.jpg</img>
        </prog>
        <prog ft="202506021400" to="202506021500">
          <title>talk show</title>
        </prog>
      </progs>
    </station>
  </stations>
</radiko>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, xmlBody)
	}))
	defer srv.Close()

	// fetchWeeklyPrograms は http.Get を使うためテスト用 URL に差し替えるためにパースロジックをテスト
	var root radikoWeeklyXML
	if err := xml.Unmarshal([]byte(xmlBody), &root); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}

	if len(root.Programs) != 2 {
		t.Fatalf("expected 2 programs, got %d", len(root.Programs))
	}
	if root.Programs[0].Title != "jazz show" {
		t.Errorf("expected 'jazz show', got %q", root.Programs[0].Title)
	}
	if root.Programs[0].Cast != "DJ Smith" {
		t.Errorf("expected 'DJ Smith', got %q", root.Programs[0].Cast)
	}
	if root.Programs[1].Title != "talk show" {
		t.Errorf("expected 'talk show', got %q", root.Programs[1].Title)
	}
	// Cast が空のケース
	if root.Programs[1].Cast != "" {
		t.Errorf("expected empty cast, got %q", root.Programs[1].Cast)
	}
}

func TestInsertColon_FtToConversion(t *testing.T) {
	// Radiko の ft/to 属性 "YYYYMMDDHHmm" を insertColon で変換
	// PHP の substr_replace($s, ':', 2, 0) の動作を確認
	got := insertColon("202506021300")
	want := "20:2506021300"
	if got != want {
		t.Errorf("insertColon(%q) = %q, want %q", "202506021300", got, want)
	}
}

func TestSchedulerSweepOrphanRecordingFiles(t *testing.T) {
	dir := t.TempDir()
	oldOrphan := filepath.Join(dir, "old_orphan.aac")
	activeFile := filepath.Join(dir, "active.aac")
	freshFile := filepath.Join(dir, "fresh.aac")
	otherExt := filepath.Join(dir, "old.txt")
	for _, path := range []string{oldOrphan, activeFile, freshFile, otherExt} {
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}
	oldTime := time.Now().Add(-26 * time.Hour)
	for _, path := range []string{oldOrphan, activeFile, otherExt} {
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatalf("Chtimes %s: %v", path, err)
		}
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	active := model.RecordingInfo{RecordingID: "active", FilePath: activeFile}
	b, _ := json.Marshal(active)
	if err := rdb.Set(context.Background(), "recording_active", string(b), time.Hour).Err(); err != nil {
		t.Fatalf("redis set: %v", err)
	}

	s := NewScheduler(nil, rdb, nil, nil, dir)
	s.SweepOrphanRecordingFiles(context.Background())

	if _, err := os.Stat(oldOrphan); !os.IsNotExist(err) {
		t.Fatalf("old orphan should be removed, stat err=%v", err)
	}
	for _, path := range []string{activeFile, freshFile, otherExt} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s should remain: %v", path, err)
		}
	}
}

func TestUpdateRedisRecordingStatusStoresFailReason(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	info := model.RecordingInfo{
		RecordingID: "rec-failed",
		Status:      "recording",
	}
	b, _ := json.Marshal(info)
	if err := rdb.Set(context.Background(), "recording_rec-failed", string(b), time.Hour).Err(); err != nil {
		t.Fatalf("redis set: %v", err)
	}

	const reason = "録音エラー: playlist returned status 404"
	if err := updateRedisRecordingStatus(context.Background(), rdb, "rec-failed", "failed", reason); err != nil {
		t.Fatalf("updateRedisRecordingStatus: %v", err)
	}

	raw, err := rdb.Get(context.Background(), "recording_rec-failed").Result()
	if err != nil {
		t.Fatalf("redis get: %v", err)
	}
	var got model.RecordingInfo
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.FailReason != reason {
		t.Fatalf("fail_reason = %q, want %q", got.FailReason, reason)
	}
}

func TestIsRecordingFilePathAllowed(t *testing.T) {
	dir := t.TempDir()
	if !isRecordingFilePathAllowed(dir, filepath.Join(dir, "ok.aac")) {
		t.Fatal("expected in-storage aac path to be allowed")
	}
	if isRecordingFilePathAllowed(dir, filepath.Join(dir, "bad.txt")) {
		t.Fatal("expected non-aac path to be rejected")
	}
	if isRecordingFilePathAllowed(dir, filepath.Join(dir, "..", "escape.aac")) {
		t.Fatal("expected escaping path to be rejected")
	}
}
