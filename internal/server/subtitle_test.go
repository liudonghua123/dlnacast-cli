package server

import (
	"strings"
	"testing"
)

func TestSRTToVTT(t *testing.T) {
	tests := []struct {
		name    string
		srt     string
		wantOK  bool
		wantSub []string
	}{
		{
			name:   "basic srt",
			srt:    "1\n00:00:01,000 --> 00:00:03,000\nHello\nWorld\n\n2\n00:00:04,500 --> 00:00:06,000\nSecond",
			wantOK: true,
			wantSub: []string{
				"WEBVTT",
				"00:00:01.000 --> 00:00:03.000",
				"Hello",
				"World",
				"00:00:04.500 --> 00:00:06.000",
				"Second",
			},
		},
		{
			name:    "crlf",
			srt:     "1\r\n00:00:01,000 --> 00:00:03,000\r\nHi\r\n\r\n2\r\n00:00:04,000 --> 00:00:05,000\r\nNext",
			wantOK:  true,
			wantSub: []string{"WEBVTT", "00:00:01.000 --> 00:00:03.000", "Hi", "Next"},
		},
		{
			name:    "no index lines",
			srt:     "00:00:01,000 --> 00:00:03,000\nNoIndex",
			wantOK:  true,
			wantSub: []string{"WEBVTT", "00:00:01.000 --> 00:00:03.000", "NoIndex"},
		},
		{
			name:   "invalid",
			srt:    "not a subtitle file",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SRTToVTT(tt.srt)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("missing %q in:\n%s", sub, got)
				}
			}
		})
	}
}
