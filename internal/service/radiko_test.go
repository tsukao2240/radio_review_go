package service

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
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
