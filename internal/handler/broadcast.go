package handler

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/radio_review_go/internal/repository"
	"github.com/yourname/radio_review_go/internal/service"
)

// fetchXMLHandler は URL から XML を取得してデコードするハンドラパッケージ内ヘルパー
func fetchXMLHandler(url string, v interface{}) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("http.Get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("io.ReadAll: %w", err)
	}

	if err := xml.Unmarshal(body, v); err != nil {
		return fmt.Errorf("xml.Unmarshal: %w", err)
	}
	return nil
}

// BroadcastHandler は放送スケジュール関連のHTTPハンドラー
type BroadcastHandler struct {
	radikoService service.RadikoApiServiceInterface
	searchService service.RadioProgramSearchServiceInterface
	programRepo   repository.RadioProgramRepositoryInterface
}

// NewBroadcastHandler はコンストラクタ
func NewBroadcastHandler(
	radikoService service.RadikoApiServiceInterface,
	searchService service.RadioProgramSearchServiceInterface,
	programRepo repository.RadioProgramRepositoryInterface,
) *BroadcastHandler {
	return &BroadcastHandler{
		radikoService: radikoService,
		searchService: searchService,
		programRepo:   programRepo,
	}
}

// ---- ヘルパ ----

// renderTemplate はテンプレートを描画する。base.html を含めてパースし "base" テンプレートを実行する。
func renderTemplate(w http.ResponseWriter, r *http.Request, tmplPath string, data interface{}) {
	RenderWithBase(w, r, tmplPath, data)
}

// respondJSON は JSON レスポンスを返す共通ヘルパー
func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("respondJSON encode error: %v", err)
	}
}

// ---- エンドポイント ----

// GetCurrentSchedule は現在放送中の番組一覧を返す
// GET /schedule
func (h *BroadcastHandler) GetCurrentSchedule(w http.ResponseWriter, r *http.Request) {
	results, err := h.radikoService.GetCurrentPrograms()
	if err != nil {
		log.Printf("GetCurrentSchedule error: %v", err)
		http.Error(w, "failed to fetch current programs", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Results": results,
	}
	renderTemplate(w, r, "web/templates/radioprogram/recent_schedule.html", data)
}

// GetWeeklySchedule は指定放送局の週間番組表を返す
// GET /schedule/{station_id}
func (h *BroadcastHandler) GetWeeklySchedule(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "station_id")
	if stationID == "" {
		http.Error(w, "station_id is required", http.StatusBadRequest)
		return
	}

	schedule, err := h.radikoService.GetWeeklySchedule(stationID)
	if err != nil {
		log.Printf("GetWeeklySchedule error: stationID=%s %v", stationID, err)
		http.Error(w, "failed to fetch weekly schedule", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"BroadcastName": "",
		"Entries":       []map[string]interface{}{},
		"ThisWeek":      []string{},
	}
	if len(schedule) > 0 {
		item := schedule[0]
		data["BroadcastName"] = item["broadcast_name"]
		data["Entries"] = item["entries"]
		data["ThisWeek"] = item["thisWeek"]
	}
	renderTemplate(w, r, "web/templates/radioprogram/weekly_schedule.html", data)
}

// GetTwoWeekScheduleSelect は2週間番組表の放送局選択画面を返す
// GET /timefree
func (h *BroadcastHandler) GetTwoWeekScheduleSelect(w http.ResponseWriter, r *http.Request) {
	areaID := r.URL.Query().Get("area")
	if areaID == "" {
		areaID = "JP13"
	}

	stations, err := fetchStationListForHandler(areaID)
	if err != nil {
		log.Printf("GetTwoWeekScheduleSelect fetchStationList error: %v", err)
		stations = []map[string]interface{}{}
	}

	data := map[string]interface{}{
		"Stations":     stations,
		"Areas":        getAreaList(),
		"SelectedArea": areaID,
	}
	renderTemplate(w, r, "web/templates/radioprogram/twoweek_select.html", data)
}

// GetTwoWeekScheduleByStation は指定放送局の2週間番組表を返す
// GET /timefree/{station_id}
func (h *BroadcastHandler) GetTwoWeekScheduleByStation(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "station_id")
	if stationID == "" {
		http.Error(w, "station_id is required", http.StatusBadRequest)
		return
	}
	areaID := r.URL.Query().Get("area")
	if areaID == "" {
		areaID = "JP13"
	}

	schedule, err := h.radikoService.GetTwoWeekSchedule(stationID)
	if err != nil {
		log.Printf("GetTwoWeekScheduleByStation error: stationID=%s %v", stationID, err)
		http.Error(w, "failed to fetch two-week schedule", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"BroadcastName": "",
		"Entries":       []map[string]interface{}{},
		"Dates":         []string{},
		"SelectedArea":  areaID,
	}
	if len(schedule) > 0 {
		item := schedule[0]
		data["BroadcastName"] = item["broadcast_name"]

		// Redis から JSON 経由で返る場合は []interface{} になるため両方に対応する
		var entries []map[string]interface{}
		switch v := item["entries"].(type) {
		case []map[string]interface{}:
			entries = v
		case []interface{}:
			for _, raw := range v {
				if m, ok := raw.(map[string]interface{}); ok {
					entries = append(entries, m)
				}
			}
		}

		data["Entries"] = entries
		// 日付一覧を entries から収集
		dateSet := map[string]struct{}{}
		for _, e := range entries {
			if d, ok := e["date"].(string); ok && d != "" {
				dateSet[d] = struct{}{}
			}
		}
		dates := make([]string, 0, len(dateSet))
		for d := range dateSet {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		data["Dates"] = dates
	}
	renderTemplate(w, r, "web/templates/radioprogram/twoweek_schedule.html", data)
}

// ShowProgramDetail は番組詳細を返す
// GET /list/{station_id}/{title}
func (h *BroadcastHandler) ShowProgramDetail(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "station_id")
	title := chi.URLParam(r, "title")

	if stationID == "" || title == "" {
		http.Error(w, "station_id and title are required", http.StatusBadRequest)
		return
	}

	detail, err := h.radikoService.GetProgramDetails(stationID, title)
	if err != nil {
		log.Printf("ShowProgramDetail error: stationID=%s title=%s %v", stationID, title, err)
		http.Error(w, "failed to fetch program details", http.StatusInternalServerError)
		return
	}

	// DBから program_id を取得（レビュー投稿ボタン用）
	var programID int64
	if prog, dbErr := h.programRepo.FindByStationAndTitle(stationID, title); dbErr == nil && prog != nil {
		programID = prog.ID
	}

	// dateParam がある場合: その日のエントリを基準に
	//   - cast・image・desc を上書き
	//   - 同じキャストの直近タイムフリー対象放送を録音ボタン用に使う
	// dateParam がない場合: 直近タイムフリー対象放送を使う
	dateParam := r.URL.Query().Get("date")
	var latestBroadcast map[string]interface{}
	if dateParam != "" {
		if entry := entryForDate(h.radikoService, stationID, title, dateParam); entry != nil {
			overwriteDetailFromEntry(detail, entry)
			cast, _ := entry["cast"].(string)
			latestBroadcast = findLatestTimefreeWithCast(h.radikoService, stationID, title, cast, nil)
		}
	}
	if latestBroadcast == nil {
		latestBroadcast = findLatestTimefree(h.radikoService, stationID, title, nil)
		if latestBroadcast != nil {
			overwriteDetailFromEntry(detail, latestBroadcast)
		}
	}

	data := map[string]interface{}{
		"Entries":         []map[string]interface{}{detail},
		"LatestBroadcast": latestBroadcast,
		"ProgramID":       programID,
		"StationID":       stationID,
		"ProgramTitle":    title,
		"DateParam":       dateParam,
	}
	renderTemplate(w, r, "web/templates/radioprogram/detail.html", data)
}

// findLatestTimefree は2週間番組表からタイムフリー再生可能な直近放送を返す。
// 放送終了済み（過去）かつ7日以内のものを対象とし、最新のものを返す。
// broadcastDay が nil でない場合は指定曜日（0=月〜6=日）のみを対象にする。
func findLatestTimefree(svc service.RadikoApiServiceInterface, stationID, title string, broadcastDay *int) map[string]interface{} {
	entries, err := twoWeekEntries(svc, stationID)
	if err != nil {
		log.Printf("findLatestTimefree: GetTwoWeekSchedule error: %v", err)
		return nil
	}
	return findLatestTimefreeFromEntries(entries, title, broadcastDay)
}

func twoWeekEntries(svc service.RadikoApiServiceInterface, stationID string) ([]map[string]interface{}, error) {
	schedule, err := svc.GetTwoWeekSchedule(stationID)
	if err != nil || len(schedule) == 0 {
		return nil, err
	}
	var entries []map[string]interface{}
	switch v := schedule[0]["entries"].(type) {
	case []map[string]interface{}:
		entries = v
	case []interface{}:
		for _, raw := range v {
			if m, ok := raw.(map[string]interface{}); ok {
				entries = append(entries, m)
			}
		}
	}
	return entries, nil
}

func findLatestTimefreeFromEntries(entries []map[string]interface{}, title string, broadcastDay *int) map[string]interface{} {
	now := time.Now()
	timefreeLimitDate := now.AddDate(0, 0, -7)

	var latestBroadcast map[string]interface{}
	var latestEndTime time.Time

	for _, entry := range entries {
		if entry["title"] != title {
			continue
		}
		// "to" は "20260321050000" 形式
		toStr, _ := entry["to"].(string)
		if len(toStr) < 12 {
			continue
		}
		programEndTime, err := time.ParseInLocation("20060102150405", toStr, time.Local)
		if err != nil {
			continue
		}
		// 放送終了済み かつ 7日以内
		if !programEndTime.Before(now) || !programEndTime.After(timefreeLimitDate) {
			continue
		}
		if broadcastDay != nil {
			wd, ok := entryWeekday(entry)
			if !ok || wd != *broadcastDay {
				continue
			}
		}
		if latestBroadcast == nil || programEndTime.After(latestEndTime) {
			latestBroadcast = entry
			latestEndTime = programEndTime
		}
	}

	return latestBroadcast
}

func entryWeekday(entry map[string]interface{}) (int, bool) {
	ftStr, _ := entry["ft"].(string)
	if len(ftStr) < 12 {
		return 0, false
	}
	t, err := time.ParseInLocation("20060102150405", ftStr, time.Local)
	if err != nil {
		return 0, false
	}
	return (int(t.Weekday()) + 6) % 7, true
}

// entryForDate は指定日付・タイトルに一致する2週間番組表のエントリを返す。
func entryForDate(svc service.RadikoApiServiceInterface, stationID, title, date string) map[string]interface{} {
	schedule, err := svc.GetTwoWeekSchedule(stationID)
	if err != nil || len(schedule) == 0 {
		return nil
	}
	var entries []map[string]interface{}
	switch v := schedule[0]["entries"].(type) {
	case []map[string]interface{}:
		entries = v
	case []interface{}:
		for _, raw := range v {
			if m, ok := raw.(map[string]interface{}); ok {
				entries = append(entries, m)
			}
		}
	}
	for _, e := range entries {
		if e["title"] == title && e["date"] == date {
			log.Printf("entryForDate: found station=%s title=%s date=%s cast=%q", stationID, title, date, e["cast"])
			return e
		}
	}
	log.Printf("entryForDate: no entry found for station=%s title=%s date=%s", stationID, title, date)
	return nil
}

// findLatestTimefreeWithCast は指定キャストと一致する直近タイムフリー対象放送を返す。
// 一致するものがなければキャスト不問で findLatestTimefree と同等の結果を返す。
func findLatestTimefreeWithCast(svc service.RadikoApiServiceInterface, stationID, title, cast string, broadcastDay *int) map[string]interface{} {
	entries, err := twoWeekEntries(svc, stationID)
	if err != nil {
		return nil
	}
	now := time.Now()
	limit := now.AddDate(0, 0, -7)
	var best map[string]interface{}
	var bestTime time.Time
	for _, e := range entries {
		if e["title"] != title {
			continue
		}
		if cast != "" {
			if c, _ := e["cast"].(string); c != cast {
				continue
			}
		}
		toStr, _ := e["to"].(string)
		if len(toStr) < 12 {
			continue
		}
		t, err := time.ParseInLocation("20060102150405", toStr, time.Local)
		if err != nil || !t.Before(now) || !t.After(limit) {
			continue
		}
		if broadcastDay != nil {
			wd, ok := entryWeekday(e)
			if !ok || wd != *broadcastDay {
				continue
			}
		}
		if best == nil || t.After(bestTime) {
			best = e
			bestTime = t
		}
	}
	return best
}

// overwriteDetailFromEntry は日付別エントリの cast・image・desc で detail を上書きする。
// info は2週間番組表に含まれないため GetProgramDetails の値を維持する。
func findNextBroadcastFromEntries(entries []map[string]interface{}, title string, broadcastDay *int) map[string]interface{} {
	now := time.Now()
	var next map[string]interface{}
	var nextTime time.Time
	for _, e := range entries {
		if e["title"] != title {
			continue
		}
		ftStr, _ := e["ft"].(string)
		if len(ftStr) < 12 {
			continue
		}
		t, err := time.ParseInLocation("20060102150405", ftStr, time.Local)
		if err != nil || !t.After(now) {
			continue
		}
		if broadcastDay != nil {
			// 曜日フィルタ: 0=月〜6=日
			wd := (int(t.Weekday()) + 6) % 7
			if wd != *broadcastDay {
				continue
			}
		}
		if next == nil || t.Before(nextTime) {
			next = e
			nextTime = t
		}
	}
	return next
}

func overwriteDetailFromEntry(detail, entry map[string]interface{}) {
	if cast, ok := entry["cast"].(string); ok && cast != "" {
		detail["cast"] = cast
	}
	if img, ok := entry["img"].(string); ok && img != "" {
		detail["image"] = img
	}
	if desc, ok := entry["desc"].(string); ok && desc != "" {
		detail["desc"] = desc
	}
}

// Search は番組検索を行う
// GET /search?keyword=...&cast=...&station_id=...
func (h *BroadcastHandler) Search(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")
	cast := r.URL.Query().Get("cast")
	stationIDParam := r.URL.Query().Get("station_id")

	var stationID *string
	if stationIDParam != "" {
		stationID = &stationIDParam
	}

	var keywordPtr, castPtr *string
	if keyword != "" {
		keywordPtr = &keyword
	}
	if cast != "" {
		castPtr = &cast
	}

	data := map[string]interface{}{
		"Keyword":   keyword,
		"Cast":      cast,
		"StationID": stationIDParam,
		"Results":   nil,
		"Total":     0,
	}

	// どちらのパラメータもない場合はフォームのみ表示
	if keywordPtr == nil && castPtr == nil {
		renderTemplate(w, r, "web/templates/search/index.html", data)
		return
	}

	programs, err := h.searchService.SearchForAPI(keywordPtr, castPtr, stationID, 100)
	if err != nil {
		log.Printf("Search error: keyword=%s cast=%s stationID=%v %v", keyword, cast, stationID, err)
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}

	data["Results"] = programs
	data["Total"] = len(programs)
	renderTemplate(w, r, "web/templates/search/index.html", data)
}

// ---- 内部ヘルパー（放送局リスト・エリアリスト） ----

// fetchStationListForHandler は指定エリアの放送局リストを Radiko API から取得する
func fetchStationListForHandler(areaID string) ([]map[string]interface{}, error) {
	url := "http://radiko.jp/v3/station/list/" + areaID + ".xml"

	type stationXML struct {
		ID   string `xml:"id"`
		Name string `xml:"name"`
	}
	type stationListXML struct {
		Stations []stationXML `xml:"station"`
	}

	var data stationListXML
	if err := fetchXMLHandler(url, &data); err != nil {
		return nil, err
	}

	stations := make([]map[string]interface{}, 0, len(data.Stations))
	for _, st := range data.Stations {
		if st.ID == "" || st.Name == "" {
			continue
		}
		stations = append(stations, map[string]interface{}{
			"id":   st.ID,
			"name": st.Name,
		})
	}
	return stations, nil
}

// getAreaList は都道府県エリアの一覧を返す
func getAreaList() map[string]string {
	return map[string]string{
		"JP1":  "北海道",
		"JP2":  "青森県",
		"JP3":  "岩手県",
		"JP4":  "宮城県",
		"JP5":  "秋田県",
		"JP6":  "山形県",
		"JP7":  "福島県",
		"JP8":  "茨城県",
		"JP9":  "栃木県",
		"JP10": "群馬県",
		"JP11": "埼玉県",
		"JP12": "千葉県",
		"JP13": "東京都",
		"JP14": "神奈川県",
		"JP15": "新潟県",
		"JP16": "富山県",
		"JP17": "石川県",
		"JP18": "福井県",
		"JP19": "山梨県",
		"JP20": "長野県",
		"JP21": "岐阜県",
		"JP22": "静岡県",
		"JP23": "愛知県",
		"JP24": "三重県",
		"JP25": "滋賀県",
		"JP26": "京都府",
		"JP27": "大阪府",
		"JP28": "兵庫県",
		"JP29": "奈良県",
		"JP30": "和歌山県",
		"JP31": "鳥取県",
		"JP32": "島根県",
		"JP33": "岡山県",
		"JP34": "広島県",
		"JP35": "山口県",
		"JP36": "徳島県",
		"JP37": "香川県",
		"JP38": "愛媛県",
		"JP39": "高知県",
		"JP40": "福岡県",
		"JP41": "佐賀県",
		"JP42": "長崎県",
		"JP43": "熊本県",
		"JP44": "大分県",
		"JP45": "宮崎県",
		"JP46": "鹿児島県",
		"JP47": "沖縄県",
	}
}
