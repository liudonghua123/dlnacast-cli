// main_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"dlnacast-cli/internal/server"
)

func TestClassifyMedia(t *testing.T) {
	tests := []struct {
		in      string
		wantURL bool
		wantTyp string
	}{
		{"http://example.com/a.mp4", true, "video"},
		{"https://example.com/pic.jpg", true, "image"},
		{"video.mp4", false, "video"},
		{"song.mp3", false, "audio"},
		{"photo.png", false, "image"},
		{"movie.mkv", false, "video"},
		{"C:\\dir\\video.avi", false, "video"},
		{"sound.flac", false, "audio"},
		{"unknown.xyz", false, "video"}, // 默认 video
	}
	for _, tt := range tests {
		f := classifyMedia(tt.in)
		isURL := f.URL != ""
		if isURL != tt.wantURL {
			t.Errorf("%s: isURL = %v, want %v", tt.in, isURL, tt.wantURL)
		}
		if f.Type != tt.wantTyp {
			t.Errorf("%s: type = %q, want %q", tt.in, f.Type, tt.wantTyp)
		}
	}
}

func TestApplySubtitles(t *testing.T) {
	dir := t.TempDir()
	subA := filepath.Join(dir, "a.srt")
	subB := filepath.Join(dir, "b.srt")
	for _, p := range []string{subA, subB} {
		if err := os.WriteFile(p, []byte("1\n00:00:00,000 --> 00:00:01,000\ntest\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("两个媒体对应两个字幕", func(t *testing.T) {
		files := []*server.File{
			{Path: "v1.mp4", Type: "video"},
			{Path: "v2.mp4", Type: "video"},
		}
		if err := applySubtitles(files, subA+","+subB); err != nil {
			t.Fatalf("applySubtitles: %v", err)
		}
		if files[0].SubPath != subA {
			t.Errorf("files[0].SubPath = %q, want %q", files[0].SubPath, subA)
		}
		if files[1].SubPath != subB {
			t.Errorf("files[1].SubPath = %q, want %q", files[1].SubPath, subB)
		}
	})

	t.Run("单个字幕应用到第一个视频", func(t *testing.T) {
		files := []*server.File{
			{Path: "s1.mp3", Type: "audio"},
			{Path: "v1.mp4", Type: "video"},
		}
		if err := applySubtitles(files, subA); err != nil {
			t.Fatalf("applySubtitles: %v", err)
		}
		if files[0].SubPath != "" {
			t.Errorf("files[0].SubPath = %q, want empty", files[0].SubPath)
		}
		if files[1].SubPath != subA {
			t.Errorf("files[1].SubPath = %q, want %q", files[1].SubPath, subA)
		}
	})

	t.Run("字幕数量与媒体数量不一致报错", func(t *testing.T) {
		files := []*server.File{
			{Path: "v1.mp4", Type: "video"},
		}
		if err := applySubtitles(files, subA+","+subB); err == nil {
			t.Error("expected error for subtitle/media count mismatch, got nil")
		}
	})

	t.Run("字幕文件不存在报错", func(t *testing.T) {
		files := []*server.File{
			{Path: "v1.mp4", Type: "video"},
		}
		if err := applySubtitles(files, filepath.Join(dir, "missing.srt")); err == nil {
			t.Error("expected error for missing subtitle file, got nil")
		}
	})
}
