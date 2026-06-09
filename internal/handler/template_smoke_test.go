package handler

// template_smoke_test.go
//
// テンプレートのパース・実行エラーを早期に検出するスモークテスト。
// 通常の handler テストはテンプレートファイルが見つからない場合に JSON へ
// フォールバックするため、{{ $.DateParam }} のような存在しないフィールドへの
// アクセスが実行時まで検出されない問題を解決する。
//
// 各テストはプロジェクトルートへ移動してから実テンプレートファイルを読み込み、
// html/template.ExecuteTemplate を直接呼び出してエラーの有無を確認する。

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tsukao2240/radio_review_go/internal/model"
)

// projectRoot はこのファイルの場所（internal/handler/）から2段上のディレクトリを返す。
// runtime.Caller は _test.go であってもビルド時のソースパスを返すため、
// Chdir に依存せずプロジェクトルートを特定できる。
func projectRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../radio_review_go/internal/handler/template_smoke_test.go
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	return root
}

// buildFuncMap は RenderWithBase と同じ FuncMap を返す。
func buildFuncMap() template.FuncMap {
	htmlTagRe := regexp.MustCompile(`<[^>]+>`)
	weekdays := [...]string{"日", "月", "火", "水", "木", "金", "土"}
	return template.FuncMap{
		"urlpathesc": func(s string) template.URL { return template.URL(url.PathEscape(s)) },
		"striptags": func(s string) string {
			brRe := regexp.MustCompile(`(?i)<br\s*/?>`)
			s = brRe.ReplaceAllString(s, "\n")
			return htmlTagRe.ReplaceAllString(s, "")
		},
		"fmtdate": func(s string) string {
			t, err := time.Parse("20060102", s)
			if err != nil {
				return s
			}
			return fmt.Sprintf("%d/%02d/%02d (%s)", t.Year(), t.Month(), t.Day(), weekdays[t.Weekday()])
		},
	}
}

// executeTemplate はテンプレートをパース・実行してエラーを返す。
func executeTemplate(t *testing.T, root, tmplPath string, data TemplateData) error {
	t.Helper()
	_, err := renderTemplateString(t, root, tmplPath, data)
	return err
}

func renderTemplateString(t *testing.T, root, tmplPath string, data TemplateData) (string, error) {
	t.Helper()
	basePath := filepath.Join(root, "web/templates/layouts/base.html")
	fullPath := filepath.Join(root, tmplPath)

	tmpl, err := template.New("").Funcs(buildFuncMap()).ParseFiles(basePath, fullPath)
	if err != nil {
		return "", fmt.Errorf("ParseFiles(%s): %w", tmplPath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", fmt.Errorf("ExecuteTemplate(%s): %w", tmplPath, err)
	}
	return buf.String(), nil
}

// minimalTD は最低限のフィールドを持つ TemplateData を返す。
func minimalTD(data interface{}) TemplateData {
	return TemplateData{
		User:      nil,
		Nonce:     "test-nonce",
		CSRFToken: "test-csrf",
		Data:      data,
		Flash:     "",
	}
}

// loggedInTD はログイン済みユーザーを持つ TemplateData を返す。
func loggedInTD(data interface{}) TemplateData {
	td := minimalTD(data)
	td.User = &model.User{ID: 1, Name: "テストユーザー"}
	return td
}

// ---------- スモークテスト ----------

func TestTemplateSmoke_ProgramDetail(t *testing.T) {
	root := projectRoot(t)

	// latestBroadcast あり・ProgramID あり・ログイン済み の最もリッチなケース
	data := map[string]interface{}{
		"Entries": []map[string]interface{}{
			{
				"id":    "TBS",
				"title": "テスト番組",
				"cast":  "テストパーソナリティ",
				"image": "/img/test.jpg",
				"info":  "番組説明",
			},
		},
		"LatestBroadcast": map[string]interface{}{
			"date":  "20260405",
			"start": "25:00",
			"end":   "27:00",
			"ft":    "20260405010000",
			"to":    "20260405030000",
		},
		"ProgramID":    int64(42),
		"StationID":    "TBS",
		"ProgramTitle": "テスト番組",
		"DateParam":    "20260405",
	}

	tests := []struct {
		name string
		td   TemplateData
	}{
		{"未ログイン", minimalTD(data)},
		{"ログイン済み", loggedInTD(data)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := renderTemplateString(t, root, "web/templates/radioprogram/detail.html", tt.td)
			if err != nil {
				t.Error(err)
			}
			if !strings.Contains(html, `data-ft="20260405010000"`) || !strings.Contains(html, `data-to="20260405030000"`) {
				t.Error("expected recording button ft/to attributes")
			}
			if !strings.Contains(html, "start_time: startTime") || !strings.Contains(html, "end_time:   endTime") {
				t.Error("expected recording payload to use ft/to variables")
			}
		})
	}
}

func TestTemplateSmoke_FavoriteIndexTimefreeRecordingButton(t *testing.T) {
	root := projectRoot(t)
	now := time.Now()
	favorites := []struct {
		ID             int64
		StationID      string
		ProgramTitle   string
		BroadcastDay   *int
		CreatedAt      time.Time
		Cast           string
		NextDate       string
		Recordable     bool
		RecProgramName string
		RecDate        string
		RecStart       string
		RecEnd         string
		RecFt          string
		RecTo          string
	}{
		{
			ID:             1,
			StationID:      "TBS",
			ProgramTitle:   "録音可能番組",
			CreatedAt:      now,
			Cast:           "出演者",
			NextDate:       "20260601",
			Recordable:     true,
			RecProgramName: "録音可能番組",
			RecDate:        "20260601",
			RecStart:       "10:00",
			RecEnd:         "12:00",
			RecFt:          "20260601100000",
			RecTo:          "20260601120000",
		},
		{
			ID:           2,
			StationID:    "QRR",
			ProgramTitle: "録音不可番組",
			CreatedAt:    now,
		},
	}

	html, err := renderTemplateString(t, root, "web/templates/favorite/index.html", loggedInTD(map[string]interface{}{
		"Favorites":     favorites,
		"HasRecordable": true,
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(html, "タイムフリー録音") {
		t.Error("expected timefree recording button text")
	}
	if !strings.Contains(html, `data-program-name="録音可能番組"`) {
		t.Error("expected recording data-program-name")
	}
	if !strings.Contains(html, `fetch('/recording/timefree/start'`) {
		t.Error("expected timefree recording fetch")
	}
	if !strings.Contains(html, "お気に入りを一括録音") {
		t.Error("expected record-all button text")
	}
	if !strings.Contains(html, `fetch('/favorites/record-all'`) {
		t.Error("expected record-all fetch")
	}
	if !strings.Contains(html, "直近放送日: 2026/06/01 (月)") {
		t.Error("expected latest broadcast date text")
	}
	if strings.Count(html, "直近放送日:") != 1 {
		t.Errorf("latest broadcast date count = %d, want 1", strings.Count(html, "直近放送日:"))
	}
	if strings.Contains(html, `data-program-name="録音不可番組"`) {
		t.Error("unexpected recording button for non-recordable favorite")
	}
}

func TestTemplateSmoke_ProgramDetail_NoLatestBroadcast(t *testing.T) {
	root := projectRoot(t)
	data := map[string]interface{}{
		"Entries": []map[string]interface{}{
			{"id": "TBS", "title": "テスト番組"},
		},
		"LatestBroadcast": nil,
		"ProgramID":       nil,
		"StationID":       "TBS",
		"ProgramTitle":    "テスト番組",
		"DateParam":       "",
	}
	if err := executeTemplate(t, root, "web/templates/radioprogram/detail.html", minimalTD(data)); err != nil {
		t.Error(err)
	}
}

func TestTemplateSmoke_TwoWeekSchedule(t *testing.T) {
	root := projectRoot(t)
	data := map[string]interface{}{
		"StationID": "TBS",
		"Schedule": map[string][]map[string]interface{}{
			"20260405": {
				{"title": "番組A", "date": "20260405", "start": "10:00", "end": "12:00"},
			},
			"20260406": {
				{"title": "番組B", "date": "20260406", "start": "14:00", "end": "15:00"},
			},
		},
		"Dates": []string{"20260405", "20260406"},
	}
	if err := executeTemplate(t, root, "web/templates/radioprogram/twoweek_schedule.html", minimalTD(data)); err != nil {
		t.Error(err)
	}
}

func TestTemplateSmoke_WeeklySchedule(t *testing.T) {
	root := projectRoot(t)
	data := map[string]interface{}{
		"BroadcastName": "TBSラジオ",
		"StationID":     "TBS",
		"Entries": []map[string]interface{}{
			{"title": "週次番組", "date": "20260405", "start": "09:00", "end": "10:00", "cast": "パーソナリティ", "image": ""},
		},
	}
	if err := executeTemplate(t, root, "web/templates/radioprogram/weekly_schedule.html", minimalTD(data)); err != nil {
		t.Error(err)
	}
}

func TestTemplateSmoke_RecentSchedule(t *testing.T) {
	root := projectRoot(t)
	data := map[string]interface{}{
		"Programs": []map[string]interface{}{
			{"title": "現在の番組", "station_id": "TBS", "date": "20260405", "start": "10:00", "end": "11:00", "cast": "MC", "image": ""},
		},
	}
	if err := executeTemplate(t, root, "web/templates/radioprogram/recent_schedule.html", minimalTD(data)); err != nil {
		t.Error(err)
	}
}

func TestTemplateSmoke_Favorites(t *testing.T) {
	root := projectRoot(t)

	now := time.Now()
	type favWithCast struct {
		ID             int64
		StationID      string
		ProgramTitle   string
		BroadcastDay   interface{}
		CreatedAt      interface{}
		Cast           string
		NextDate       string
		Recordable     bool
		RecProgramName string
		RecDate        string
		RecStart       string
		RecEnd         string
	}

	data := map[string]interface{}{
		"Favorites": []favWithCast{
			{
				ID:           1,
				StationID:    "TBS",
				ProgramTitle: "お気に入り番組",
				BroadcastDay: 0, // 月曜
				CreatedAt:    now,
				Cast:         "パーソナリティ",
				NextDate:     "20260407",
			},
		},
		"HasRecordable": false,
	}

	if err := executeTemplate(t, root, "web/templates/favorite/index.html", loggedInTD(data)); err != nil {
		t.Error(err)
	}
}

func TestTemplateSmoke_Favorites_Empty(t *testing.T) {
	root := projectRoot(t)
	data := map[string]interface{}{
		"Favorites": nil,
	}
	if err := executeTemplate(t, root, "web/templates/favorite/index.html", minimalTD(data)); err != nil {
		t.Error(err)
	}
}
