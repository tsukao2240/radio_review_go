package radiko

import (
	"strconv"
	"strings"
	"testing"
)

func TestGetAreaIDFromStationID(t *testing.T) {
	tests := []struct {
		stationID string
		want      string
	}{
		{stationID: "TBS", want: "JP13"},
		{stationID: "OBC", want: "JP27"},
		{stationID: "HBC", want: "JP1"},
		{stationID: "UNKNOWN", want: "JP13"},
	}

	for _, tt := range tests {
		t.Run(tt.stationID, func(t *testing.T) {
			got := GetAreaIDFromStationID(tt.stationID)
			if got != tt.want {
				t.Fatalf("GetAreaIDFromStationID(%q) = %q, want %q", tt.stationID, got, tt.want)
			}
		})
	}
}

func TestGenerateGPSLocation(t *testing.T) {
	got := generateGPSLocation("JP27")
	parts := strings.Split(got, ",")
	if len(parts) != 3 || parts[2] != "gps" {
		t.Fatalf("location = %q, want lat,long,gps", got)
	}
	lat, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		t.Fatalf("parse lat: %v", err)
	}
	lng, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		t.Fatalf("parse lng: %v", err)
	}
	base := areaCoordinates["JP27"]
	if lat < base[0]-0.025 || lat > base[0]+0.025 {
		t.Fatalf("lat = %f, want within JP27 jitter range", lat)
	}
	if lng < base[1]-0.025 || lng > base[1]+0.025 {
		t.Fatalf("lng = %f, want within JP27 jitter range", lng)
	}
}
