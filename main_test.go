// main_test.go
package main

import (
	"testing"
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
