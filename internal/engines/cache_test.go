package engines

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engines.json")
	infos := []Info{
		{ID: FFmpeg, Name: "ffmpeg", Available: true, Version: "7.1", Path: "/usr/bin/ffmpeg"},
		{ID: Rembg, Name: "rembg", Available: false, Hint: "pip install rembg", Err: errors.New("not found")},
	}
	if err := SaveCache(path, infos); err != nil {
		t.Fatal(err)
	}
	got := LoadCache(path, 24*time.Hour)
	if len(got) != 2 {
		t.Fatalf("cache = %+v", got)
	}
	if got[0].ID != FFmpeg || !got[0].Available || got[0].Version != "7.1" {
		t.Errorf("ffmpeg = %+v", got[0])
	}
	if got[1].Err == nil || got[1].Hint == "" {
		t.Errorf("error metadata lost: %+v", got[1])
	}
}

func TestCacheExpires(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engines.json")
	if err := SaveCache(path, []Info{{ID: FFmpeg, Available: true}}); err != nil {
		t.Fatal(err)
	}
	if got := LoadCache(path, time.Nanosecond); len(got) != 0 {
		t.Fatalf("stale cache returned %d entries", len(got))
	}
}
