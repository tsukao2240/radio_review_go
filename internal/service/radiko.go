package service

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/radio_review_go/internal/repository"
)

// RadikoApiService は RadikoApiServiceInterface の実装
type RadikoApiService struct {
	redis      *redis.Client
	radikoRepo repository.RadioProgramRepositoryInterface
}

// NewRadikoApiService はコンストラクタ
func NewRadikoApiService(rdb *redis.Client, radikoRepo repository.RadioProgramRepositoryInterface) *RadikoApiService {
	return &RadikoApiService{
		redis:      rdb,
		radikoRepo: radikoRepo,
	}
}

// ---- XML 構造体 ----

type radikoXML struct {
	Stations []radikoStation `xml:"stations>station"`
}

type radikoStation struct {
	ID    string       `xml:"id,attr"`
	Name  string       `xml:"name"`
	Progs []radikoProg `xml:"progs>prog"`
}

type radikoProg struct {
	Ft    string `xml:"ft,attr"`
	To    string `xml:"to,attr"`
	Ftl   string `xml:"ftl,attr"`
	Tol   string `xml:"tol,attr"`
	Title string `xml:"title"`
	Pfm   string `xml:"pfm"`
	Img   string `xml:"img"`
	Desc  string `xml:"desc"`
	Info  string `xml:"info"`
	URL   string `xml:"url"`
}

// ---- ヘルパ ----

// getCurrentDate は午前5時を基準に今日の日付(YYYYMMDD)を返す
func getCurrentDate() string {
	now := time.Now()
	if now.Hour() < 5 {
		now = now.AddDate(0, 0, -1)
	}
	return now.Format("20060102")
}

// insertColon は "HHMM" → "HH:MM" に変換する
func insertColon(t string) string {
	if len(t) < 4 {
		return t
	}
	return t[:2] + ":" + t[2:]
}

// fetchXML は URL から XML を取得してデコードする
func fetchXML(url string, v interface{}) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("http.Get %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("io.ReadAll: %w", err)
	}

	if err := xml.Unmarshal(body, v); err != nil {
		return fmt.Errorf("xml.Unmarshal: %w", err)
	}
	return nil
}

// cacheGetRadiko は Redis から JSON をデコードして返す
func cacheGetRadiko(ctx context.Context, rdb *redis.Client, key string, dest interface{}) bool {
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(val), dest); err != nil {
		return false
	}
	return true
}

// cacheSetRadiko は Redis に JSON エンコードして保存する
func cacheSetRadiko(ctx context.Context, rdb *redis.Client, key string, val interface{}, ttl time.Duration) {
	b, err := json.Marshal(val)
	if err != nil {
		log.Printf("cacheSetRadiko marshal error: %v", err)
		return
	}
	if err := rdb.Set(ctx, key, string(b), ttl).Err(); err != nil {
		log.Printf("cacheSetRadiko redis error: %v", err)
	}
}

// md5Hex は文字列の MD5 ハッシュを16進数文字列で返す
func md5Hex(s string) string {
	h := md5.New() //nolint:gosec
	if _, err := h.Write([]byte(s)); err != nil {
		log.Printf("md5Hex write error: %v", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ---- インタフェース実装 ----

// GetWeeklySchedule は指定放送局の週間番組表を返す（キャッシュ 30 分）
func (s *RadikoApiService) GetWeeklySchedule(stationID string) ([]map[string]interface{}, error) {
	ctx := context.Background()
	cacheKey := "weekly_schedule_" + stationID

	var cached []map[string]interface{}
	if cacheGetRadiko(ctx, s.redis, cacheKey, &cached) {
		return cached, nil
	}

	today := getCurrentDate()
	url := "http://radiko.jp/v3/program/station/weekly/" + stationID + ".xml"

	var data radikoXML
	if err := fetchXML(url, &data); err != nil {
		return nil, fmt.Errorf("GetWeeklySchedule fetchXML: %w", err)
	}

	var entries []map[string]interface{}
	weekSet := map[string]struct{}{}
	broadcastName := ""

	for _, st := range data.Stations {
		if broadcastName == "" {
			broadcastName = st.Name
		}
		for _, prog := range st.Progs {
			programDate := ""
			if len(prog.Ft) >= 8 {
				programDate = prog.Ft[:8]
			}
			if programDate < today {
				continue
			}
			startTime := insertColon(prog.Ftl)
			// 24時以降の今日の番組を除外
			if programDate == today && len(startTime) >= 2 {
				var h int
				fmt.Sscanf(startTime[:2], "%d", &h)
				if h >= 24 {
					continue
				}
			}
			weekSet[programDate] = struct{}{}
			entries = append(entries, map[string]interface{}{
				"id":         st.ID,
				"station_id": st.ID,
				"date":       programDate,
				"title":      prog.Title,
				"cast":       prog.Pfm,
				"start":      startTime,
				"end":        insertColon(prog.Tol),
			})
		}
	}

	thisWeek := make([]string, 0, len(weekSet))
	for d := range weekSet {
		thisWeek = append(thisWeek, d)
	}

	result := []map[string]interface{}{
		{
			"entries":        entries,
			"thisWeek":       thisWeek,
			"broadcast_name": broadcastName,
			"station_id":     stationID,
		},
	}

	log.Printf("GetWeeklySchedule: station=%s programs=%d", stationID, len(entries))
	cacheSetRadiko(ctx, s.redis, cacheKey, result, 30*time.Minute)
	return result, nil
}

// GetTwoWeekSchedule は2週間分の番組表を返す（タイムフリー録音用、キャッシュ 30 分）
func (s *RadikoApiService) GetTwoWeekSchedule(stationID string) ([]map[string]interface{}, error) {
	ctx := context.Background()
	cacheKey := "radiko_two_week_schedule_" + stationID

	var cached []map[string]interface{}
	if cacheGetRadiko(ctx, s.redis, cacheKey, &cached) {
		return cached, nil
	}

	// 7日前の日付 (午前5時基準)
	base := time.Now()
	if base.Hour() < 5 {
		base = base.AddDate(0, 0, -1)
	}
	startDate := base.AddDate(0, 0, -7).Format("20060102")

	var entries []map[string]interface{}
	broadcastName := ""

	// 週間XML から未来分を取得
	weekURL := "http://radiko.jp/v3/program/station/weekly/" + stationID + ".xml"
	var weekData radikoXML
	if err := fetchXML(weekURL, &weekData); err != nil {
		return nil, fmt.Errorf("GetTwoWeekSchedule fetchXML weekly: %w", err)
	}

	for _, st := range weekData.Stations {
		if broadcastName == "" {
			broadcastName = st.Name
		}
		for _, prog := range st.Progs {
			programDate := ""
			if len(prog.Ft) >= 8 {
				programDate = prog.Ft[:8]
			}
			if programDate < startDate {
				continue
			}
			entries = append(entries, map[string]interface{}{
				"id":         st.ID,
				"station_id": st.ID,
				"date":       programDate,
				"title":      prog.Title,
				"cast":       prog.Pfm,
				"start":      insertColon(prog.Ftl),
				"end":        insertColon(prog.Tol),
				"ft":         prog.Ft,
				"to":         prog.To,
				"img":        prog.Img,
				"desc":       prog.Desc,
			})
		}
	}

	// 各日付のエンドポイントから過去分を補完（日付ごと API）
	for i := -7; i <= 0; i++ {
		targetDate := base.AddDate(0, 0, i)
		dateStr := targetDate.Format("20060102")
		dayURL := fmt.Sprintf("http://radiko.jp/v3/program/station/date/%s/%s.xml", dateStr, stationID)

		var dayData radikoXML
		if err := fetchXML(dayURL, &dayData); err != nil {
			log.Printf("GetTwoWeekSchedule: failed to fetch day %s: %v", dateStr, err)
			continue
		}
		for _, st := range dayData.Stations {
			if broadcastName == "" {
				broadcastName = st.Name
			}
			for _, prog := range st.Progs {
				// 週間取得と ft が重複するエントリをスキップ
				duplicate := false
				for _, e := range entries {
					if e["ft"] == prog.Ft && e["id"] == st.ID {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
				programDate := ""
				if len(prog.Ft) >= 8 {
					programDate = prog.Ft[:8]
				}
				entries = append(entries, map[string]interface{}{
					"id":    st.ID,
					"date":  programDate,
					"title": prog.Title,
					"cast":  prog.Pfm,
					"start": insertColon(prog.Ftl),
					"end":   insertColon(prog.Tol),
					"ft":    prog.Ft,
					"to":    prog.To,
					"img":   prog.Img,
					"desc":  prog.Desc,
				})
			}
		}
	}

	result := []map[string]interface{}{
		{
			"entries":        entries,
			"broadcast_name": broadcastName,
			"station_id":     stationID,
		},
	}

	log.Printf("GetTwoWeekSchedule: station=%s programs=%d", stationID, len(entries))
	cacheSetRadiko(ctx, s.redis, cacheKey, result, 30*time.Minute)
	return result, nil
}

// GetCurrentPrograms は現在放送中の全局番組を返す（キャッシュ 5 分）
func (s *RadikoApiService) GetCurrentPrograms() ([]map[string]interface{}, error) {
	ctx := context.Background()
	cacheKey := "radiko_current_programs"

	var cached []map[string]interface{}
	if cacheGetRadiko(ctx, s.redis, cacheKey, &cached) {
		return cached, nil
	}

	var allEntries []map[string]interface{}
	seenStations := map[string]struct{}{}

	for i := 1; i < 48; i++ {
		url := fmt.Sprintf("http://radiko.jp/v3/program/now/JP%d.xml", i)
		var data radikoXML
		if err := fetchXML(url, &data); err != nil {
			log.Printf("GetCurrentPrograms: JP%d fetch error: %v", i, err)
			continue
		}
		for _, st := range data.Stations {
			if _, exists := seenStations[st.Name]; exists {
				continue
			}
			seenStations[st.Name] = struct{}{}

			title := ""
			cast := ""
			startTime := ""
			endTime := ""
			progURL := ""
			progDate := ""
			if len(st.Progs) > 0 {
				p := st.Progs[0]
				title = p.Title
				cast = p.Pfm
				startTime = insertColon(p.Ftl)
				endTime = insertColon(p.Tol)
				progURL = p.URL
				if len(p.Ft) >= 8 {
					progDate = p.Ft[:8]
				}
			}

			allEntries = append(allEntries, map[string]interface{}{
				"station_id": st.ID,
				"station":    st.Name,
				"title":      title,
				"cast":       cast,
				"date":       progDate,
				"start":      startTime,
				"end":        endTime,
				"url":        progURL,
			})
		}
	}

	log.Printf("GetCurrentPrograms: total=%d", len(allEntries))
	cacheSetRadiko(ctx, s.redis, cacheKey, allEntries, 5*time.Minute)
	return allEntries, nil
}

// GetProgramDetails は番組詳細を返す（キャッシュ 30 分）
func (s *RadikoApiService) GetProgramDetails(stationID, title string) (map[string]interface{}, error) {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("radiko_program_details_%s_%s", stationID, md5Hex(title))

	var cached map[string]interface{}
	if cacheGetRadiko(ctx, s.redis, cacheKey, &cached) {
		return cached, nil
	}

	url := "http://radiko.jp/v3/program/station/weekly/" + stationID + ".xml"
	var data radikoXML
	if err := fetchXML(url, &data); err != nil {
		return nil, fmt.Errorf("GetProgramDetails fetchXML: %w", err)
	}

	var result map[string]interface{}
	for _, st := range data.Stations {
		for _, prog := range st.Progs {
			if prog.Title == title {
				result = map[string]interface{}{
					"id":    st.ID,
					"title": prog.Title,
					"cast":  prog.Pfm,
					"image": prog.Img,
					"desc":  prog.Desc,
					"info":  prog.Info,
				}
				break
			}
		}
		if result != nil {
			break
		}
	}

	if result == nil {
		result = map[string]interface{}{}
	}

	log.Printf("GetProgramDetails: station=%s title=%s found=%v", stationID, title, len(result) > 0)
	cacheSetRadiko(ctx, s.redis, cacheKey, result, 30*time.Minute)
	return result, nil
}
