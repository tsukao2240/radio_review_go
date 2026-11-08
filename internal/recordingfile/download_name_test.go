package recordingfile

import (
	"mime"
	"strings"
	"testing"
)

func TestAttachmentDispositionUnicode(t *testing.T) {
	name := "2026-11-04_0000_TBS_アルコ＆ピース D.C.GARAGE.m4a"
	header := AttachmentDisposition(name)
	if !strings.Contains(header, "filename*=utf-8''") {
		t.Fatalf("missing UTF-8 filename parameter: %s", header)
	}
	for _, r := range header {
		if r > 127 {
			t.Fatal("non-ASCII HTTP header")
		}
	}
	kind, params, err := mime.ParseMediaType(header)
	if err != nil || kind != "attachment" || params["filename"] != name {
		t.Fatal("filename did not round trip")
	}
}
