// internal/ui/ui_test.go
package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderProgress(t *testing.T) {
	var b bytes.Buffer
	RenderProgress(&b, 634, 5400, true, 30, "我的视频.mp4")
	out := b.String()

	if !strings.HasPrefix(out, "\r\x1b[K") {
		t.Errorf("missing clear-line prefix: %q", out)
	}
	for _, want := range []string{"▶", "00:10:34", "01:30:00", "11%", "vol:30", "我的视频.mp4"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestRenderProgressPaused(t *testing.T) {
	var b bytes.Buffer
	RenderProgress(&b, 0, 60, false, 0, "x.mp4")
	if !strings.Contains(b.String(), "⏸") {
		t.Errorf("missing pause icon in %q", b.String())
	}
}
