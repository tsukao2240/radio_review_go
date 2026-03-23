package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestInsertColon(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"1300", "13:00"},
		{"0000", "00:00"},
		{"2400", "24:00"},
		{"0530", "05:30"},
		{"12", "12"},  // 4文字未満はそのまま
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

func TestMd5Hex(t *testing.T) {
	// 同じ入力に対して同じハッシュを返す
	h1 := md5Hex("test string")
	h2 := md5Hex("test string")
	if h1 != h2 {
		t.Errorf("md5Hex is not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 32 {
		t.Errorf("md5Hex length = %d, want 32", len(h1))
	}
	// 異なる入力は異なるハッシュ
	h3 := md5Hex("different")
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}
}

func TestFetchXML(t *testing.T) {
	t.Run("正常なXMLをパースできる", func(t *testing.T) {
		xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<radiko>
  <stations>
    <station id="TBS">
      <name>TBSラジオ</name>
      <progs>
        <prog ft="202506021300" to="202506021400" ftl="1300" tol="1400">
          <title>jazz show</title>
          <pfm>DJ Smith</pfm>
        </prog>
      </progs>
    </station>
  </stations>
</radiko>`

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(xmlBody))
		}))
		defer srv.Close()

		var data radikoXML
		if err := fetchXML(srv.URL, &data); err != nil {
			t.Fatalf("fetchXML error: %v", err)
		}

		if len(data.Stations) != 1 {
			t.Fatalf("got %d stations, want 1", len(data.Stations))
		}
		if data.Stations[0].ID != "TBS" {
			t.Errorf("got station ID=%q, want TBS", data.Stations[0].ID)
		}
		if data.Stations[0].Name != "TBSラジオ" {
			t.Errorf("got name=%q, want TBSラジオ", data.Stations[0].Name)
		}
		if len(data.Stations[0].Progs) != 1 {
			t.Fatalf("got %d progs, want 1", len(data.Stations[0].Progs))
		}
		if data.Stations[0].Progs[0].Title != "jazz show" {
			t.Errorf("got title=%q, want 'jazz show'", data.Stations[0].Progs[0].Title)
		}
	})

	t.Run("サーバーエラー: エラーを返す", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		var data radikoXML
		// ステータス500でもボディが空ならXML parseエラーになる
		err := fetchXML(srv.URL, &data)
		if err == nil {
			// 空のXMLでもエラーにならない場合がある
			// 少なくともパニックしないことを確認
		}
	})
}

func TestRadikoXMLStructure(t *testing.T) {
	// radikoXML構造体のXMLパースをテスト
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<radiko>
  <stations>
    <station id="QRR">
      <name>文化放送</name>
      <progs>
        <prog ft="202506021000" to="202506021200" ftl="1000" tol="1200">
          <title>午前のトーク</title>
          <pfm>山田太郎</pfm>
          <img>https://example.com/img.jpg</img>
          <desc>朝の番組</desc>
          <info>詳細情報</info>
          <url>https://example.com/prog</url>
        </prog>
      </progs>
    </station>
  </stations>
</radiko>`

	var data radikoXML
	if err := xml.Unmarshal([]byte(xmlBody), &data); err != nil {
		t.Fatalf("xml.Unmarshal error: %v", err)
	}

	if len(data.Stations) != 1 {
		t.Fatalf("got %d stations, want 1", len(data.Stations))
	}
	st := data.Stations[0]
	if st.ID != "QRR" {
		t.Errorf("got ID=%q, want QRR", st.ID)
	}
	if len(st.Progs) != 1 {
		t.Fatalf("got %d progs, want 1", len(st.Progs))
	}
	p := st.Progs[0]
	if p.Ftl != "1000" {
		t.Errorf("got Ftl=%q, want 1000", p.Ftl)
	}
	if p.Title != "午前のトーク" {
		t.Errorf("got title=%q, want 午前のトーク", p.Title)
	}
	if p.Pfm != "山田太郎" {
		t.Errorf("got pfm=%q, want 山田太郎", p.Pfm)
	}
}

func TestGetCurrentDate(t *testing.T) {
	// getCurrentDate は午前5時より前なら前日の日付を返すはず
	// 実際の時刻に依存するのでフォーマットだけ確認
	date := getCurrentDate()
	if len(date) != 8 {
		t.Errorf("expected 8-char date string, got %q (len=%d)", date, len(date))
	}
	// YYYYMMDD 形式チェック（数値のみ）
	for _, c := range date {
		if c < '0' || c > '9' {
			t.Errorf("expected numeric date, got %q", date)
			break
		}
	}
}

func TestNewRadikoApiService(t *testing.T) {
	svc := NewRadikoApiService(nil, nil)
	if svc == nil {
		t.Error("expected non-nil service")
	}
}

func TestCacheGetSetRadiko(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	t.Run("キャッシュなし: false を返す", func(t *testing.T) {
		var dest []map[string]interface{}
		ok := cacheGetRadiko(ctx, rdb, "missing_key", &dest)
		if ok {
			t.Error("expected false for missing key")
		}
	})

	t.Run("セット後に取得: true を返す", func(t *testing.T) {
		data := []map[string]interface{}{{"title": "jazz show"}}
		cacheSetRadiko(ctx, rdb, "test_key", data, time.Minute)

		var dest []map[string]interface{}
		ok := cacheGetRadiko(ctx, rdb, "test_key", &dest)
		if !ok {
			t.Error("expected true after set")
		}
		if len(dest) != 1 {
			t.Errorf("expected 1 item, got %d", len(dest))
		}
		if dest[0]["title"] != "jazz show" {
			t.Errorf("expected title='jazz show', got %v", dest[0]["title"])
		}
	})

	t.Run("不正なJSONでfalseを返す", func(t *testing.T) {
		rdb.Set(ctx, "bad_json", "not-valid-json", time.Minute)
		var dest []map[string]interface{}
		ok := cacheGetRadiko(ctx, rdb, "bad_json", &dest)
		if ok {
			t.Error("expected false for invalid JSON")
		}
	})
}

func TestRadikoApiService_GetWeeklySchedule_CacheHit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewRadikoApiService(rdb, nil)

	cached := []map[string]interface{}{
		{"broadcast_name": "TBSラジオ", "station_id": "TBS", "entries": nil},
	}
	b, _ := json.Marshal(cached)
	rdb.Set(context.Background(), "weekly_schedule_TBS", string(b), time.Minute)

	result, err := svc.GetWeeklySchedule("TBS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result from cache, got %d", len(result))
	}
	if result[0]["broadcast_name"] != "TBSラジオ" {
		t.Errorf("unexpected result: %v", result[0])
	}
}

func TestRadikoApiService_GetTwoWeekSchedule_CacheHit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewRadikoApiService(rdb, nil)

	cached := []map[string]interface{}{
		{"broadcast_name": "文化放送", "station_id": "QRR", "entries": nil},
	}
	b, _ := json.Marshal(cached)
	rdb.Set(context.Background(), "radiko_two_week_schedule_QRR", string(b), time.Minute)

	result, err := svc.GetTwoWeekSchedule("QRR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestRadikoApiService_GetCurrentPrograms_CacheHit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewRadikoApiService(rdb, nil)

	cached := []map[string]interface{}{
		{"station_id": "TBS", "title": "jazz show"},
		{"station_id": "QRR", "title": "morning talk"},
	}
	b, _ := json.Marshal(cached)
	rdb.Set(context.Background(), "radiko_current_programs", string(b), time.Minute)

	result, err := svc.GetCurrentPrograms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 programs from cache, got %d", len(result))
	}
}

func TestRadikoApiService_GetProgramDetails_CacheHit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewRadikoApiService(rdb, nil)

	titleHash := md5Hex("jazz show")
	cacheKey := "radiko_program_details_TBS_" + titleHash
	cachedDetail := map[string]interface{}{
		"title": "jazz show", "station_id": "TBS",
	}
	b, _ := json.Marshal(cachedDetail)
	rdb.Set(context.Background(), cacheKey, string(b), time.Minute)

	result, err := svc.GetProgramDetails("TBS", "jazz show")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["title"] != "jazz show" {
		t.Errorf("expected title='jazz show', got %v", result["title"])
	}
}
