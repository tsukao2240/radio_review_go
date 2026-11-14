package handler

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLiveEventsSignalsChangeAndStopsOnDisconnect(t *testing.T) {
	var version atomic.Int64
	server := httptest.NewServer(liveEvents(func(context.Context) (string, error) {
		if version.Load() == 0 {
			return "before", nil
		}
		return "after", nil
	}, 5*time.Millisecond))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("not a live event stream")
	}
	reader := bufio.NewReader(response.Body)
	readEvent := func() string {
		var b strings.Builder
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				t.Fatal(e)
			}
			b.WriteString(line)
			if line == "\n" {
				return b.String()
			}
		}
	}
	if !strings.Contains(readEvent(), "event: ready") {
		t.Fatal("missing initial state")
	}
	version.Store(1)
	changed := readEvent()
	if !strings.Contains(changed, "event: changed") || !strings.Contains(changed, "data: after") {
		t.Fatal("missing state change")
	}
	cancel()
}
